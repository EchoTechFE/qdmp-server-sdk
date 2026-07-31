package io.github.echotechfe.qdmp.auth;

import java.util.Objects;

/**
 * An app-level access token (obtained via {@code CLIENT_CREDENTIALS}) together with its absolute
 * expiry, as cached by {@link QdmpAuth#getAccessToken()}.
 */
public final class CachedAppToken {

  private final String accessToken;
  private final long expiresAtEpochSeconds;

  /**
   * Creates a new cached token.
   *
   * @param accessToken the app-level access token
   * @param expiresAtEpochSeconds the absolute expiry, as Unix seconds (not milliseconds, and not a
   *     relative "seconds remaining" duration)
   */
  public CachedAppToken(String accessToken, long expiresAtEpochSeconds) {
    this.accessToken = accessToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
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
   * Returns the absolute expiry of this token, as Unix seconds.
   *
   * @return the expiry, in epoch seconds
   */
  public long getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds;
  }

  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    if (!(o instanceof CachedAppToken)) {
      return false;
    }
    CachedAppToken other = (CachedAppToken) o;
    return expiresAtEpochSeconds == other.expiresAtEpochSeconds
        && Objects.equals(accessToken, other.accessToken);
  }

  @Override
  public int hashCode() {
    return Objects.hash(accessToken, expiresAtEpochSeconds);
  }
}
