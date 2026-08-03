package io.github.echotechfe.qdmp.auth;

import java.util.Objects;

/**
 * The app credential (obtained via {@code CLIENT_CREDENTIALS}) as cached by {@link
 * QdmpAuth#getAppAccessToken()}.
 *
 * <p>Holds every field the server returned -- access token, expiry, refresh token, open ID -- and
 * not just the two the SDK itself needs, so that a call served from the cache hands the caller
 * exactly the same {@link AppAccessTokenResult} a freshly-exchanged one does. Storing only
 * accessToken/expiresAt would make the cached path silently return an object missing two fields.
 */
public final class CachedAppToken {

  private final String accessToken;
  private final long expiresAtEpochSeconds;
  private final String refreshToken;
  private final String openId;

  /**
   * Creates a new cached credential.
   *
   * @param accessToken the app-level access token
   * @param expiresAtEpochSeconds the absolute expiry, as Unix seconds (not milliseconds, and not a
   *     relative "seconds remaining" duration)
   * @param refreshToken the refresh token the server reported alongside it, passed through
   *     unvalidated
   * @param openId the open ID the server reported alongside it -- an empty string on this path
   */
  public CachedAppToken(
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
   * Returns the absolute expiry of this token, as Unix seconds.
   *
   * @return the expiry, in epoch seconds
   */
  public long getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds;
  }

  /**
   * Returns the refresh token reported alongside the access token.
   *
   * @return the refresh token, or {@code null}/empty if the server omitted it
   */
  public String getRefreshToken() {
    return refreshToken;
  }

  /**
   * Returns the open ID reported alongside the access token.
   *
   * @return the open ID -- an empty string on the app-credential path
   */
  public String getOpenId() {
    return openId;
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
        && Objects.equals(accessToken, other.accessToken)
        && Objects.equals(refreshToken, other.refreshToken)
        && Objects.equals(openId, other.openId);
  }

  @Override
  public int hashCode() {
    return Objects.hash(accessToken, expiresAtEpochSeconds, refreshToken, openId);
  }
}
