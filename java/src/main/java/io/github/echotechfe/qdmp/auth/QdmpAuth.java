package io.github.echotechfe.qdmp.auth;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import io.github.echotechfe.qdmp.errors.QdmpValidationError;
import io.github.echotechfe.qdmp.generated.AuthRefresh200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.AuthRefreshRequest;
import io.github.echotechfe.qdmp.generated.AuthToken200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.AuthTokenRequest;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.time.Clock;

/**
 * Token lifecycle management, per {@code POST /auth/v1/token} and {@code POST /auth/v1/refresh}.
 *
 * <p>The app-level token ({@code CLIENT_CREDENTIALS}, via {@link #getAccessToken()}) is cached and
 * refreshed automatically: cached values are reused until fewer than {@link
 * #REFRESH_BUFFER_SECONDS} remain before expiry, refreshes are single-flighted under a lock so
 * concurrent callers never trigger more than one network call, and persistence goes through the
 * pluggable {@link TokenStore}.
 *
 * <p>User-level tokens ({@code AUTHORIZATION_CODE}, via {@link #code2Session} and {@link
 * #refreshToken}) are never cached by the SDK -- a server process serves many end users at once and
 * has no way to know, on its own, which one a given call belongs to. Callers must persist these
 * themselves.
 */
public final class QdmpAuth {

  /**
   * Proactive refresh buffer, in seconds: the app-level token is refreshed once fewer than this
   * many seconds remain before its reported expiry, rather than waiting for it to actually expire.
   */
  private static final long REFRESH_BUFFER_SECONDS = 300;

  private static final RouteMeta.Entry AUTH_TOKEN_ROUTE = RouteMeta.get("authToken");
  private static final RouteMeta.Entry AUTH_REFRESH_ROUTE = RouteMeta.get("authRefresh");

  private final QdmpTransport transport;
  private final String appId;
  private final String appSecret;
  private final TokenStore tokenStore;
  private final Clock clock;
  private final Object refreshLock = new Object();

  /**
   * Creates a new auth module.
   *
   * @param transport the shared HTTP transport
   * @param appId the qdmp app ID
   * @param appSecret the qdmp app secret
   * @param tokenStore where the app-level token is cached
   * @param clock the clock used to evaluate token expiry (overridable for tests)
   */
  public QdmpAuth(
      QdmpTransport transport, String appId, String appSecret, TokenStore tokenStore, Clock clock) {
    this.transport = transport;
    this.appId = appId;
    this.appSecret = appSecret;
    this.tokenStore = tokenStore;
    this.clock = clock;
  }

  /**
   * Returns a valid app-level access token, reusing/refreshing the cached one as needed.
   *
   * @return the app-level access token
   */
  public String getAccessToken() {
    CachedAppToken cached = tokenStore.get().orElse(null);
    if (cached != null && hasSufficientTimeRemaining(cached)) {
      return cached.getAccessToken();
    }
    synchronized (refreshLock) {
      CachedAppToken recheck = tokenStore.get().orElse(null);
      if (recheck != null && hasSufficientTimeRemaining(recheck)) {
        return recheck.getAccessToken();
      }
      AuthTokenRequest request =
          new AuthTokenRequest()
              .grantType(AuthTokenRequest.GrantTypeEnum.CLIENT_CREDENTIALS)
              .appId(appId)
              .appSecret(appSecret);
      AuthToken200ResponseAllOfData data =
          transport.post(
              AUTH_TOKEN_ROUTE.getPath(),
              AuthScheme.fromWireValue(AUTH_TOKEN_ROUTE.getAuthScheme()),
              null,
              request,
              AuthToken200ResponseAllOfData.class);
      CachedAppToken fresh = toCachedAppToken(data);
      tokenStore.set(fresh);
      return fresh.getAccessToken();
    }
  }

  // The server reported code:"0" (success), but the SDK still has to guard against a
  // structurally malformed body: a null/empty `accessToken` or a non-numeric `expiresAt` would
  // otherwise surface as a bare NullPointerException/NumberFormatException instead of one of the
  // SDK's typed exceptions, or worse -- silently cache/return an empty access token.
  private static CachedAppToken toCachedAppToken(AuthToken200ResponseAllOfData data) {
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/token");
    long expiresAtEpochSeconds;
    try {
      expiresAtEpochSeconds = Long.parseLong(data.getExpiresAt());
    } catch (NumberFormatException e) {
      throw new QdmpTransportException(
          "qdmp POST /auth/v1/token returned a non-numeric expiresAt: \""
              + data.getExpiresAt()
              + "\"",
          e);
    }
    return new CachedAppToken(data.getAccessToken(), expiresAtEpochSeconds);
  }

  /**
   * Exchanges a one-time {@code AUTHORIZATION_CODE} login code for a user-level session. Never
   * cached by the SDK -- persist the result yourself if you need it beyond this call.
   *
   * @param code the login code, e.g. from {@code qd.login()}; required and must be non-empty for
   *     grantType {@code AUTHORIZATION_CODE}
   * @return the full session: access token, refresh token, expiry, and open ID
   * @throws QdmpValidationError if {@code code} is {@code null} or empty
   */
  public AuthToken200ResponseAllOfData code2Session(String code) {
    if (code == null || code.isEmpty()) {
      throw new QdmpValidationError(
          "code2Session: \"code\" is required for grantType=AUTHORIZATION_CODE and must be a"
              + " non-empty string");
    }
    AuthTokenRequest request =
        new AuthTokenRequest()
            .grantType(AuthTokenRequest.GrantTypeEnum.AUTHORIZATION_CODE)
            .appId(appId)
            .appSecret(appSecret)
            .code(code);
    AuthToken200ResponseAllOfData data =
        transport.post(
            AUTH_TOKEN_ROUTE.getPath(),
            AuthScheme.fromWireValue(AUTH_TOKEN_ROUTE.getAuthScheme()),
            null,
            request,
            AuthToken200ResponseAllOfData.class);
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/token");
    return data;
  }

  /**
   * Exchanges a user-level refresh token for a new access token. Never cached by the SDK.
   *
   * @param refreshToken the refresh token previously obtained from {@link #code2Session}; required
   *     and must be non-empty
   * @return the new access token and its expiry
   * @throws QdmpValidationError if {@code refreshToken} is {@code null} or empty
   */
  public AuthRefresh200ResponseAllOfData refreshToken(String refreshToken) {
    if (refreshToken == null || refreshToken.isEmpty()) {
      throw new QdmpValidationError(
          "refreshToken: \"refreshToken\" is required and must be a non-empty string");
    }
    AuthRefreshRequest request = new AuthRefreshRequest().refreshToken(refreshToken);
    AuthRefresh200ResponseAllOfData data =
        transport.post(
            AUTH_REFRESH_ROUTE.getPath(),
            AuthScheme.fromWireValue(AUTH_REFRESH_ROUTE.getAuthScheme()),
            null,
            request,
            AuthRefresh200ResponseAllOfData.class);
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/refresh");
    return data;
  }

  // Shared guard for all three auth endpoints (CLIENT_CREDENTIALS/code2Session/refreshToken): the
  // server reported code:"0" (success), but the SDK still has to protect callers from a
  // structurally malformed body -- a null `data` or a null/empty `accessToken`/`expiresAt` must
  // never be handed back as a silently-broken session/token object.
  private static void requireNonMalformed(
      boolean dataIsNull, String accessToken, String expiresAt, String routeDescription) {
    if (dataIsNull
        || accessToken == null
        || accessToken.isEmpty()
        || expiresAt == null
        || expiresAt.isEmpty()) {
      throw new QdmpTransportException(
          "qdmp "
              + routeDescription
              + " reported code=\"0\" (success) but returned a malformed body (missing"
              + " data/accessToken/expiresAt)");
    }
  }

  private boolean hasSufficientTimeRemaining(CachedAppToken token) {
    long remaining = token.getExpiresAtEpochSeconds() - clock.instant().getEpochSecond();
    return remaining > REFRESH_BUFFER_SECONDS;
  }
}
