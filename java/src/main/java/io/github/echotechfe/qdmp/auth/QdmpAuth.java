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
 * <p>The app credential ({@code CLIENT_CREDENTIALS}, via {@link #getAppAccessToken()}) is cached
 * and renewed automatically: cached values are reused until fewer than {@link
 * #REFRESH_BUFFER_SECONDS} remain before expiry, renewals are single-flighted under a lock so
 * concurrent callers never trigger more than one network call, and persistence goes through the
 * pluggable {@link TokenStore}.
 *
 * <p>User-authorization credentials ({@code AUTHORIZATION_CODE}, via {@link #getUserAccessToken}
 * and {@link #refreshToken}) are never cached by the SDK -- a server process serves many end users
 * at once and has no way to know, on its own, which one a given call belongs to. Callers must
 * persist these themselves, or hand them to {@link
 * io.github.echotechfe.qdmp.QdmpClient#withUserCredential} to have the SDK keep one alive for the
 * duration of a request.
 */
public final class QdmpAuth {

  /**
   * Proactive renewal buffer, in seconds: a credential is renewed once fewer than this many seconds
   * remain before its reported expiry, rather than waiting for it to actually expire. Shared with
   * {@link UserCredentialSession}, which applies the same buffer to user-authorization credentials.
   */
  static final long REFRESH_BUFFER_SECONDS = 300;

  // All three auth endpoints authenticate with appId/appSecret (or the refresh token) in the
  // request body -- their route metadata declares authScheme "noAuth", so no token header is
  // attached at all. Named rather than passed as a bare `null` because QdmpTransport overloads its
  // credential parameter (String vs QdmpContext), which a bare `null` literal would leave
  // ambiguous.
  private static final String NO_ACCESS_TOKEN = null;

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
   * Returns a valid app credential, reusing/renewing the cached one as needed.
   *
   * <p>The object returned by the cached path and the object returned by the freshly-exchanged path
   * are built from the same {@link CachedAppToken} by {@link #toAppAccessTokenResult}, so both
   * carry the full set of fields -- a caller can never tell which path served it apart by looking
   * at what is populated.
   *
   * @return the app credential: access token, expiry, refresh token, and open ID
   */
  public AppAccessTokenResult getAppAccessToken() {
    CachedAppToken cached = tokenStore.get().orElse(null);
    if (cached != null && hasSufficientTimeRemaining(cached)) {
      return toAppAccessTokenResult(cached);
    }
    synchronized (refreshLock) {
      CachedAppToken recheck = tokenStore.get().orElse(null);
      if (recheck != null && hasSufficientTimeRemaining(recheck)) {
        return toAppAccessTokenResult(recheck);
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
              NO_ACCESS_TOKEN,
              request,
              AuthToken200ResponseAllOfData.class);
      CachedAppToken fresh = toCachedAppToken(data);
      // A non-negative but already-past (or about-to-expire) expiresAt passes
      // toCachedAppToken/parseExpiresAt's numeric validation, but a token that is already stale
      // the moment it arrives must never be cached or handed back as if it were usable -- apply
      // the exact same freshness bar used to decide whether a *cached* token still needs
      // refreshing.
      if (!hasSufficientTimeRemaining(fresh)) {
        throw new QdmpTransportException(
            "qdmp POST /auth/v1/token returned an access token that is already expired or too"
                + " close to expiry (expiresAt=\""
                + data.getExpiresAt()
                + "\"); refusing to cache or return it");
      }
      tokenStore.set(fresh);
      return toAppAccessTokenResult(fresh);
    }
  }

  private static AppAccessTokenResult toAppAccessTokenResult(CachedAppToken token) {
    return new AppAccessTokenResult(
        token.getAccessToken(),
        token.getExpiresAtEpochSeconds(),
        token.getRefreshToken(),
        token.getOpenId());
  }

  // The server reported code:"0" (success), but the SDK still has to guard against a
  // structurally malformed body: a null/empty `accessToken` or a non-numeric/negative `expiresAt`
  // would otherwise surface as a bare NullPointerException/NumberFormatException instead of one of
  // the SDK's typed exceptions, or worse -- silently cache/return an empty or negative-lifetime
  // access token.
  private static CachedAppToken toCachedAppToken(AuthToken200ResponseAllOfData data) {
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/token");
    long expiresAtEpochSeconds = parseExpiresAt(data.getExpiresAt(), "POST /auth/v1/token");
    // Unlike the user-authorization path, refreshToken/openId are deliberately not required here:
    // the CLIENT_CREDENTIALS response reports an empty openId (the credential represents the app,
    // not a user), and its refreshToken -- which the SDK never uses, since it renews by re-running
    // CLIENT_CREDENTIALS -- is passed through exactly as received rather than validated.
    return new CachedAppToken(
        data.getAccessToken(), expiresAtEpochSeconds, data.getRefreshToken(), data.getOpenId());
  }

  /**
   * Exchanges a one-time authorization code for a user-authorization credential. Never cached by
   * the SDK -- persist the result yourself if you need it beyond this call.
   *
   * @param code the one-time authorization code obtained by the app/front end; required and must be
   *     non-empty for grantType {@code AUTHORIZATION_CODE}
   * @return the full credential: access token, refresh token, expiry, and open ID
   * @throws QdmpValidationError if {@code code} is {@code null} or empty
   */
  public UserAccessTokenResult getUserAccessToken(String code) {
    if (code == null || code.isEmpty()) {
      throw new QdmpValidationError(
          "getUserAccessToken: \"code\" is required for grantType=AUTHORIZATION_CODE and must be a"
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
            NO_ACCESS_TOKEN,
            request,
            AuthToken200ResponseAllOfData.class);
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/token");
    long expiresAtEpochSeconds = parseExpiresAt(data.getExpiresAt(), "POST /auth/v1/token");
    requireNonEmptyField(data.getRefreshToken(), "refreshToken", "POST /auth/v1/token");
    requireNonEmptyField(data.getOpenId(), "openId", "POST /auth/v1/token");
    requireNotAlreadyExpired(expiresAtEpochSeconds, "POST /auth/v1/token");
    return new UserAccessTokenResult(
        data.getAccessToken(), data.getRefreshToken(), expiresAtEpochSeconds, data.getOpenId());
  }

  /**
   * Exchanges a user-level refresh token for a new access token. Never cached by the SDK.
   *
   * @param refreshToken the refresh token previously obtained from {@link #getUserAccessToken};
   *     required and must be non-empty
   * @return the new access token and its expiry
   * @throws QdmpValidationError if {@code refreshToken} is {@code null} or empty
   */
  public RefreshTokenResult refreshToken(String refreshToken) {
    if (refreshToken == null || refreshToken.isEmpty()) {
      throw new QdmpValidationError(
          "refreshToken: \"refreshToken\" is required and must be a non-empty string");
    }
    AuthRefreshRequest request = new AuthRefreshRequest().refreshToken(refreshToken);
    AuthRefresh200ResponseAllOfData data =
        transport.post(
            AUTH_REFRESH_ROUTE.getPath(),
            AuthScheme.fromWireValue(AUTH_REFRESH_ROUTE.getAuthScheme()),
            NO_ACCESS_TOKEN,
            request,
            AuthRefresh200ResponseAllOfData.class);
    requireNonMalformed(
        data == null,
        data == null ? null : data.getAccessToken(),
        data == null ? null : data.getExpiresAt(),
        "POST /auth/v1/refresh");
    long expiresAtEpochSeconds = parseExpiresAt(data.getExpiresAt(), "POST /auth/v1/refresh");
    requireNotAlreadyExpired(expiresAtEpochSeconds, "POST /auth/v1/refresh");
    return new RefreshTokenResult(data.getAccessToken(), expiresAtEpochSeconds);
  }

  // Shared guard for all three auth endpoints
  // (CLIENT_CREDENTIALS/getUserAccessToken/refreshToken): the server reported code:"0" (success),
  // but the SDK still has to protect callers from a structurally malformed body -- a null `data` or
  // a null/empty `accessToken`/`expiresAt` must never be handed back as a silently-broken
  // credential object.
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

  // Shared numeric guard for all three auth endpoints: `expiresAt` must parse as a non-negative
  // long. A non-numeric value would otherwise surface as a bare NumberFormatException, and a
  // negative value -- while technically parseable -- describes an already-expired (or nonsensical)
  // token and must never be cached/returned as if it were valid.
  private static long parseExpiresAt(String expiresAt, String routeDescription) {
    long parsed;
    try {
      parsed = Long.parseLong(expiresAt);
    } catch (NumberFormatException e) {
      throw new QdmpTransportException(
          "qdmp " + routeDescription + " returned a non-numeric expiresAt: \"" + expiresAt + "\"",
          e);
    }
    if (parsed < 0) {
      throw new QdmpTransportException(
          "qdmp " + routeDescription + " returned a negative expiresAt: \"" + expiresAt + "\"");
    }
    return parsed;
  }

  // getUserAccessToken's Javadoc promises "the full credential: access token, refresh token,
  // expiry, and open ID" -- unlike accessToken/expiresAt (checked by requireNonMalformed),
  // refreshToken/openId were never validated at all, so a response missing either field would
  // silently hand back a credential object with a null/empty refresh token or open ID. This guard
  // is user-authorization-only: on the CLIENT_CREDENTIALS path openId is empty by design (see
  // toCachedAppToken).
  private static void requireNonEmptyField(
      String value, String fieldName, String routeDescription) {
    if (value == null || value.isEmpty()) {
      throw new QdmpTransportException(
          "qdmp "
              + routeDescription
              + " reported code=\"0\" (success) but returned a malformed body (missing/empty "
              + fieldName
              + ")");
    }
  }

  // getUserAccessToken/refreshToken are one-shot exchanges the SDK never caches (unlike the
  // CLIENT_CREDENTIALS path, guarded by hasSufficientTimeRemaining's stricter
  // REFRESH_BUFFER_SECONDS buffer), so there is no cache-freshness policy to apply here -- but a
  // non-negative expiresAt can still already be in the past (e.g. "1", 1970-01-01; parseExpiresAt
  // only rejects negative/non-numeric values). Handing back a credential that is already dead on
  // arrival would let a caller persist (or start relying on) a permanently-broken one.
  private void requireNotAlreadyExpired(long expiresAtEpochSeconds, String routeDescription) {
    if (expiresAtEpochSeconds <= clock.instant().getEpochSecond()) {
      throw new QdmpTransportException(
          "qdmp "
              + routeDescription
              + " returned an already-expired expiresAt: "
              + expiresAtEpochSeconds);
    }
  }

  private boolean hasSufficientTimeRemaining(CachedAppToken token) {
    long remaining = token.getExpiresAtEpochSeconds() - clock.instant().getEpochSecond();
    return remaining > REFRESH_BUFFER_SECONDS;
  }
}
