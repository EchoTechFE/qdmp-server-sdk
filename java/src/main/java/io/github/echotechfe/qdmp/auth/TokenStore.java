package io.github.echotechfe.qdmp.auth;

import java.util.Optional;

/**
 * Pluggable persistence for the app-level (CLIENT_CREDENTIALS) token managed by {@link QdmpAuth}.
 * The default is an in-process {@link InMemoryTokenStore}; multi-instance deployments should supply
 * an external implementation (e.g. Redis-backed) so that instances share one cached token instead
 * of each independently racing to refresh it against the qdmp API.
 *
 * <p>User-level tokens (from {@code AUTHORIZATION_CODE}) are never stored here -- see {@link
 * QdmpAuth#code2Session}.
 */
public interface TokenStore {

  /**
   * Returns the currently stored app-level token, if any.
   *
   * @return the cached token, or {@link Optional#empty()} if none has been stored yet
   */
  Optional<CachedAppToken> get();

  /**
   * Replaces the stored app-level token.
   *
   * @param token the token to store
   */
  void set(CachedAppToken token);

  /** Removes any stored app-level token. */
  void clear();
}
