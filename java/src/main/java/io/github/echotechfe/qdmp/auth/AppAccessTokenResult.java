package io.github.echotechfe.qdmp.auth;

/**
 * The full app credential returned by {@link QdmpAuth#getAppAccessToken()}: access token, expiry,
 * refresh token, and open ID.
 *
 * <p>The {@code CLIENT_CREDENTIALS} response carries the same four fields the user-authorization
 * response does, so all four are handed to the caller rather than just the access token --
 * otherwise the expiry (needed to schedule anything around it) and the refresh token would be
 * silently dropped. Two of them behave differently from their user-authorization counterparts and
 * are therefore not validated as non-empty: {@code openId} is an empty string on this path (the
 * credential represents the app, not a user), and {@code refreshToken}, while present in practice,
 * is passed through as-is. The SDK itself never uses that refresh token -- it renews the app
 * credential by re-running {@code CLIENT_CREDENTIALS} 300 seconds before expiry -- but the caller
 * is free to.
 *
 * <p>Hand-written (not generated) so that {@link #toString()} can omit the actual token values --
 * unlike the openapi-generator DTOs, whose generated {@code toString()} prints every field verbatim
 * and would otherwise write plaintext credentials into anything that logs this object (e.g. {@code
 * logger.info("credential={}", credential)}).
 */
public final class AppAccessTokenResult {

  private final String accessToken;
  private final long expiresAtEpochSeconds;
  private final String refreshToken;
  private final String openId;

  /**
   * Creates a new app credential.
   *
   * @param accessToken the app-level access token
   * @param expiresAtEpochSeconds the absolute expiry of {@code accessToken}, as Unix seconds (not
   *     milliseconds, and not a relative "seconds remaining" duration)
   * @param refreshToken the refresh token reported alongside it, passed through unvalidated
   * @param openId the open ID reported alongside it -- an empty string on this path
   */
  public AppAccessTokenResult(
      String accessToken, long expiresAtEpochSeconds, String refreshToken, String openId) {
    this.accessToken = accessToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
    this.refreshToken = refreshToken;
    this.openId = openId;
  }

  /**
   * Returns the app-level access token.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return accessToken;
  }

  /**
   * Returns the absolute expiry of the access token, as Unix seconds.
   *
   * @return the expiry, in epoch seconds
   */
  public long getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds;
  }

  /**
   * Returns the refresh token reported alongside the access token, exactly as the server sent it.
   *
   * @return the refresh token, or {@code null}/empty if the server omitted it
   */
  public String getRefreshToken() {
    return refreshToken;
  }

  /**
   * Returns the open ID reported alongside the access token, exactly as the server sent it.
   *
   * @return the open ID -- an empty string on the app-credential path
   */
  public String getOpenId() {
    return openId;
  }

  @Override
  public String toString() {
    return "AppAccessTokenResult{accessToken=[REDACTED], refreshToken=[REDACTED],"
        + " expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + ", openId="
        + sanitizeForToString(openId)
        + "}";
  }

  // openId is not validated at all on this path (it is empty by design), so it can carry
  // server-controlled control characters such as embedded CR/LF. This class exists specifically so
  // logging it is safe, so toString() must never let openId forge extra log lines. getOpenId()
  // intentionally still returns the raw value -- this only affects the debug rendering.
  private static String sanitizeForToString(String value) {
    if (value == null) {
      return null;
    }
    StringBuilder sanitized = new StringBuilder(value.length());
    for (int i = 0; i < value.length(); i++) {
      char c = value.charAt(i);
      sanitized.append(c < 0x20 || c == 0x7f ? ' ' : c);
    }
    return sanitized.toString();
  }
}
