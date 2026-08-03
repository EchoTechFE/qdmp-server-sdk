package io.github.echotechfe.qdmp;

/**
 * Supplies the access token a single {@link QdmpTransport} call attaches, and decides whether an
 * HTTP 401 that call ran into is worth exactly one retry with a renewed token.
 *
 * <p>Exists so that the "renew on 401, retry once" policy lives in one place -- {@link
 * QdmpTransport} -- instead of being repeated in every method of every business group. A {@link
 * QdmpContext} built from a plain token via {@link QdmpContext#of(String)} is backed by a source
 * that never renews anything; a context handed out by {@link
 * QdmpClient#withUserCredential(io.github.echotechfe.qdmp.auth.UserCredentialOptions)} is backed by
 * one that renews the user-authorization credential through {@code POST /auth/v1/refresh}.
 *
 * <p>Implementations must be safe for concurrent use: several business calls can share one source
 * and hit its methods from different threads at the same time, and a renewal triggered by one of
 * them must not be duplicated by the others.
 */
public interface AccessTokenSource {

  /**
   * Returns the token to attach to a request that is about to be sent, renewing it first if it is
   * known to be at (or near) its expiry.
   *
   * @return the access token to send; never {@code null}
   */
  String tokenForRequest();

  /**
   * Returns the currently held token without triggering any renewal.
   *
   * @return the access token held right now
   */
  String currentAccessToken();

  /**
   * Reacts to a request that came back as an HTTP 401 carrying an access-token error code.
   *
   * @param attemptedAccessToken the token the failed request actually carried, so an implementation
   *     can tell "my token is stale" apart from "a concurrent caller already renewed it"
   * @return the token the request should be retried once with, or {@code null} to propagate the
   *     original error without retrying
   */
  String renewAfterUnauthorized(String attemptedAccessToken);
}
