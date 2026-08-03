package io.github.echotechfe.qdmp.auth;

import io.github.echotechfe.qdmp.AccessTokenSource;
import java.time.Clock;
import java.util.OptionalLong;
import java.util.function.Consumer;

/**
 * Keeps one user-authorization credential alive across calls: renews it {@link
 * QdmpAuth#REFRESH_BUFFER_SECONDS} seconds ahead of a known expiry, renews it again if a call comes
 * back as an HTTP 401 saying the token is invalid/expired, and hands the resulting token back to
 * {@code QdmpTransport} through {@link AccessTokenSource}.
 *
 * <p>Public only because {@code QdmpUserClient} lives in a different package -- callers get one of
 * these by calling {@code QdmpClient.withUserCredential(...)}, never by constructing it.
 *
 * <p>Renewals are single-flighted: every renewal happens while holding {@code renewLock}, and both
 * entry points re-check the state after acquiring it, so several concurrent business calls that all
 * notice the same stale token produce exactly one {@code POST /auth/v1/refresh} and exactly one
 * {@code onRefresh} invocation. The refresh token itself never changes -- the endpoint does not
 * return a new one -- so it is held in a final field.
 */
public final class UserCredentialSession implements AccessTokenSource {

  private final QdmpAuth auth;
  private final Clock clock;
  private final String refreshToken;
  private final Consumer<RefreshedUserToken> onRefresh;
  private final Object renewLock = new Object();

  private volatile UserCredentialSnapshot current;

  /**
   * Creates a session around an already-obtained user-authorization credential.
   *
   * @param auth the auth module used to perform {@code POST /auth/v1/refresh}
   * @param clock the clock used to decide whether the credential is close enough to expiry to renew
   *     proactively
   * @param options the credential and renewal callback, already validated by {@link
   *     UserCredentialOptions.Builder#build()}
   */
  public UserCredentialSession(QdmpAuth auth, Clock clock, UserCredentialOptions options) {
    this.auth = auth;
    this.clock = clock;
    this.refreshToken = options.getRefreshToken();
    this.onRefresh = options.getOnRefresh();
    OptionalLong expiresAt = options.getExpiresAtEpochSeconds();
    this.current =
        new UserCredentialSnapshot(
            options.getAccessToken(),
            this.refreshToken,
            expiresAt.isPresent() ? expiresAt.getAsLong() : null);
  }

  /**
   * Returns an immutable view of the credential as it stands right now.
   *
   * @return the current snapshot; later renewals replace it rather than mutating it
   */
  public UserCredentialSnapshot snapshot() {
    return current;
  }

  @Override
  public String currentAccessToken() {
    return current.getAccessToken();
  }

  @Override
  public String tokenForRequest() {
    UserCredentialSnapshot snapshot = current;
    if (!needsProactiveRenewal(snapshot)) {
      return snapshot.getAccessToken();
    }
    synchronized (renewLock) {
      // Re-check under the lock: while this caller was waiting, another one may have renewed
      // already, in which case its result is reused instead of a second network call being made.
      UserCredentialSnapshot rechecked = current;
      if (!needsProactiveRenewal(rechecked)) {
        return rechecked.getAccessToken();
      }
      return renew().getAccessToken();
    }
  }

  @Override
  public String renewAfterUnauthorized(String attemptedAccessToken) {
    synchronized (renewLock) {
      UserCredentialSnapshot rechecked = current;
      if (!rechecked.getAccessToken().equals(attemptedAccessToken)) {
        // A concurrent caller already renewed after this request went out, so the 401 was about a
        // token that is no longer current. Retry with theirs rather than renewing a second time.
        return rechecked.getAccessToken();
      }
      return renew().getAccessToken();
    }
  }

  // Always called while holding renewLock. A failing refresh (10007/10008, or anything else)
  // propagates untouched: the caller of the business method is the one who has to go get a fresh
  // authorization code, and no business request may be attempted in the meantime.
  private UserCredentialSnapshot renew() {
    RefreshTokenResult result = auth.refreshToken(refreshToken);
    // The refresh token is carried over unchanged: POST /auth/v1/refresh issues no new one.
    UserCredentialSnapshot renewed =
        new UserCredentialSnapshot(
            result.getAccessToken(), refreshToken, result.getExpiresAtEpochSeconds());
    // Published before onRefresh runs, deliberately: if the callback throws (e.g. its database
    // write
    // fails) the business call fails with that error, but the token the server already issued is
    // kept -- discarding it would waste a perfectly usable credential and force another renewal on
    // the next call.
    current = renewed;
    if (onRefresh != null) {
      onRefresh.accept(
          new RefreshedUserToken(result.getAccessToken(), result.getExpiresAtEpochSeconds()));
    }
    return renewed;
  }

  private boolean needsProactiveRenewal(UserCredentialSnapshot snapshot) {
    OptionalLong expiresAt = snapshot.getExpiresAtEpochSeconds();
    if (!expiresAt.isPresent()) {
      // Expiry unknown: there is nothing to renew ahead of, so the credential is only ever renewed
      // reactively, when a call actually comes back 401.
      return false;
    }
    long remaining = expiresAt.getAsLong() - clock.instant().getEpochSecond();
    return remaining <= QdmpAuth.REFRESH_BUFFER_SECONDS;
  }
}
