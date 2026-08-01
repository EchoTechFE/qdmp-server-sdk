package io.github.echotechfe.qdmp.auth;

/**
 * The new access token returned by {@link QdmpAuth#refreshToken(String)}.
 *
 * <p>Hand-written (not generated) so that {@link #toString()} can omit the actual token value --
 * unlike the openapi-generator DTOs, whose generated {@code toString()} prints every field verbatim
 * and would otherwise write plaintext credentials into anything that logs this object (e.g. {@code
 * logger.info("result={}", result)}).
 */
public final class RefreshTokenResult {

  private final String accessToken;
  private final long expiresAtEpochSeconds;

  /**
   * Creates a new refresh result.
   *
   * @param accessToken the new user-level access token
   * @param expiresAtEpochSeconds the absolute expiry of {@code accessToken}, as Unix seconds (not
   *     milliseconds, and not a relative "seconds remaining" duration)
   */
  public RefreshTokenResult(String accessToken, long expiresAtEpochSeconds) {
    this.accessToken = accessToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
  }

  /**
   * Returns the new user-level access token.
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

  @Override
  public String toString() {
    return "RefreshTokenResult{accessToken=[REDACTED], expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + "}";
  }
}
