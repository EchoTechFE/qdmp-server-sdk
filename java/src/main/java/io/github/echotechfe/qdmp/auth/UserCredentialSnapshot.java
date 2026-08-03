package io.github.echotechfe.qdmp.auth;

import java.util.OptionalLong;

/**
 * An immutable point-in-time view of the user-authorization credential a {@link
 * UserCredentialSession} is holding, as handed out by {@code QdmpUserClient.credential()}.
 *
 * <p>Immutable on purpose: a caller that grabs a snapshot in order to persist it must not have it
 * mutate underneath them when a later call renews the credential. A renewal replaces the session's
 * snapshot with a new object; every snapshot already handed out keeps describing the credential as
 * it was at the moment it was taken.
 *
 * <p>{@link #toString()} omits the token value, so logging a snapshot cannot leak a live
 * credential.
 */
public final class UserCredentialSnapshot {

  private final String accessToken;
  private final String refreshToken;
  private final Long expiresAtEpochSeconds;

  UserCredentialSnapshot(String accessToken, String refreshToken, Long expiresAtEpochSeconds) {
    this.accessToken = accessToken;
    this.refreshToken = refreshToken;
    this.expiresAtEpochSeconds = expiresAtEpochSeconds;
  }

  /**
   * Returns the access token held at the moment this snapshot was taken.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return accessToken;
  }

  /**
   * Returns the refresh token backing this credential. It never changes across renewals -- {@code
   * POST /auth/v1/refresh} issues no new one -- but it is exposed here so that persisting a
   * credential only needs this snapshot, rather than also reaching back to whatever the caller
   * originally passed to {@code withUserCredential}.
   *
   * @return the refresh token
   */
  public String getRefreshToken() {
    return refreshToken;
  }

  /**
   * Returns the absolute expiry of that access token, as Unix seconds.
   *
   * @return the expiry in epoch seconds, or {@link OptionalLong#empty()} if it is unknown -- the
   *     caller never supplied one and no renewal has happened yet
   */
  public OptionalLong getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds == null
        ? OptionalLong.empty()
        : OptionalLong.of(expiresAtEpochSeconds);
  }

  @Override
  public String toString() {
    return "UserCredentialSnapshot{accessToken=[REDACTED], refreshToken=[REDACTED],"
        + " expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + "}";
  }
}
