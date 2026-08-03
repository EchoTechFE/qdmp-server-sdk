package io.github.echotechfe.qdmp.auth;

/**
 * What a {@link UserCredentialOptions#getOnRefresh()} callback receives after the SDK has renewed a
 * user-authorization credential: the new access token and its new expiry, and nothing else.
 *
 * <p>There is deliberately no refresh token here. {@code POST /auth/v1/refresh} returns only {@code
 * accessToken}/{@code expiresAt}; the refresh token the caller originally supplied stays valid and
 * unchanged, so a callback persisting this back to storage should leave its stored refresh token
 * alone.
 *
 * <p>{@link #toString()} omits the token value, so logging the callback argument cannot leak a live
 * credential.
 */
public final class RefreshedUserToken {

  private final String accessToken;
  private final long expiresAtEpochSeconds;

  /**
   * Creates a new renewal notification.
   *
   * @param accessToken the newly issued user access token
   * @param expiresAtEpochSeconds the absolute expiry of {@code accessToken}, as Unix seconds (not
   *     milliseconds, and not a relative "seconds remaining" duration)
   */
  public RefreshedUserToken(String accessToken, long expiresAtEpochSeconds) {
    this.accessToken = accessToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
  }

  /**
   * Returns the newly issued user access token.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return accessToken;
  }

  /**
   * Returns the absolute expiry of the new access token, as Unix seconds.
   *
   * @return the expiry, in epoch seconds
   */
  public long getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds;
  }

  @Override
  public String toString() {
    return "RefreshedUserToken{accessToken=[REDACTED], expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + "}";
  }
}
