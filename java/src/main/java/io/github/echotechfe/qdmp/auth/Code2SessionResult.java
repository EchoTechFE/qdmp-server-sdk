package io.github.echotechfe.qdmp.auth;

/**
 * The full user session returned by {@link QdmpAuth#code2Session(String)}: access token, refresh
 * token, expiry, and open ID.
 *
 * <p>Hand-written (not generated) so that {@link #toString()} can omit the actual token values --
 * unlike the openapi-generator DTOs, whose generated {@code toString()} prints every field verbatim
 * and would otherwise write plaintext credentials into anything that logs this object (e.g. {@code
 * logger.info("session={}", session)}).
 */
public final class Code2SessionResult {

  private final String accessToken;
  private final String refreshToken;
  private final long expiresAtEpochSeconds;
  private final String openId;

  /**
   * Creates a new session result.
   *
   * @param accessToken the user-level access token
   * @param refreshToken the refresh token, exchangeable via {@link QdmpAuth#refreshToken(String)}
   * @param expiresAtEpochSeconds the absolute expiry of {@code accessToken}, as Unix seconds (not
   *     milliseconds, and not a relative "seconds remaining" duration)
   * @param openId the user's open ID, unique within this app
   */
  public Code2SessionResult(
      String accessToken, String refreshToken, long expiresAtEpochSeconds, String openId) {
    this.accessToken = accessToken;
    this.refreshToken = refreshToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
    this.openId = openId;
  }

  /**
   * Returns the user-level access token.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return accessToken;
  }

  /**
   * Returns the refresh token.
   *
   * @return the refresh token
   */
  public String getRefreshToken() {
    return refreshToken;
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
   * Returns the user's open ID.
   *
   * @return the open ID
   */
  public String getOpenId() {
    return openId;
  }

  @Override
  public String toString() {
    return "Code2SessionResult{accessToken=[REDACTED], refreshToken=[REDACTED],"
        + " expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + ", openId="
        + sanitizeForToString(openId)
        + "}";
  }

  // openId is only validated as non-empty (see QdmpAuth.requireNonEmptyField) -- it can still
  // contain server-controlled control characters (e.g. embedded CR/LF). This class exists
  // specifically so logging it is safe, so toString() must never let openId forge extra log lines
  // the way an unsanitized value could. getOpenId() intentionally still returns the raw value --
  // this only affects the debug rendering.
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
