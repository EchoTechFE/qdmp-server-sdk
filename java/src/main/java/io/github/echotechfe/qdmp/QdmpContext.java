package io.github.echotechfe.qdmp;

import java.util.Objects;

/**
 * Carries the per-call access token required by every user-scoped business method (e.g. {@code
 * client.mark().add(ctx, request)}). {@link #of} is the single gate that guarantees a missing/blank
 * token fails locally, before any HTTP request is attempted.
 */
public final class QdmpContext {

  private final AccessTokenSource tokenSource;

  private QdmpContext(AccessTokenSource tokenSource) {
    this.tokenSource = tokenSource;
  }

  /**
   * Creates a context wrapping the given access token. The SDK does not distinguish here whether
   * the token is app-level ({@code CLIENT_CREDENTIALS}) or user-level ({@code AUTHORIZATION_CODE})
   * -- that choice belongs to the caller.
   *
   * @param accessToken the token to send with the call; must not be {@code null} or blank, and must
   *     contain only characters that are safe to send verbatim as an HTTP header value
   * @return a new context wrapping {@code accessToken}
   * @throws NullPointerException if {@code accessToken} is {@code null}
   * @throws IllegalArgumentException if {@code accessToken} is empty, all whitespace, or contains a
   *     character that cannot be safely sent as an HTTP header value
   */
  public static QdmpContext of(String accessToken) {
    Objects.requireNonNull(accessToken, "accessToken must not be null");
    if (accessToken.isBlank()) {
      throw new IllegalArgumentException("accessToken must not be blank");
    }
    requireHeaderSafe(accessToken);
    return new QdmpContext(new StaticAccessTokenSource(accessToken));
  }

  // The context handed to the business groups by QdmpUserClient: instead of a fixed token it
  // carries
  // the live user-authorization credential, so QdmpTransport can renew it (proactively, or after an
  // HTTP 401) without any group method knowing about renewal at all. Package-private because
  // QdmpUserClient is the only legitimate caller -- external callers go through
  // QdmpClient#withUserCredential.
  static QdmpContext boundTo(AccessTokenSource tokenSource) {
    Objects.requireNonNull(tokenSource, "tokenSource must not be null");
    return new QdmpContext(tokenSource);
  }

  AccessTokenSource tokenSource() {
    return tokenSource;
  }

  // Whitelists printable ASCII (0x20-0x7E) before the token ever reaches QdmpTransport's OkHttp
  // Request.Builder#header calls, rejecting everything else -- including tab (0x09) and the whole
  // Latin-1 Supplement range (0x80-0xFF, e.g. accented characters). A blacklist that only rejects
  // control characters and non-Latin-1 code points is not strict enough: OkHttp's own header
  // validation is stricter still (tab or 0x20-0x7E only), and for header names outside its
  // hardcoded redaction list (Authorization/Cookie/Proxy-Authorization/Set-Cookie --
  // "access-token"/"x-openapi-access-token" are not on it) it echoes the *entire* invalid header
  // value into its thrown IllegalArgumentException message. Whitelisting here means that path is
  // never reached. Package-private (not private) so QdmpTransport can reuse the exact same check
  // for tokens passed directly to it, bypassing QdmpContext entirely. The message below
  // intentionally never includes the token itself.
  static void requireHeaderSafe(String accessToken) {
    for (int i = 0; i < accessToken.length(); i++) {
      char c = accessToken.charAt(i);
      boolean isPrintableAscii = c >= 0x20 && c <= 0x7e;
      if (!isPrintableAscii) {
        throw new IllegalArgumentException(
            "accessToken contains a character that cannot be safely sent as an HTTP header value"
                + " (this message intentionally omits the token itself)");
      }
    }
  }

  /**
   * Returns the access token this context currently carries, without triggering a renewal.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return tokenSource.currentAccessToken();
  }
}
