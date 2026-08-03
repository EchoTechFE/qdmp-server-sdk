package io.github.echotechfe.qdmp.auth;

import java.util.Optional;

/**
 * Pluggable persistence for the app credential (CLIENT_CREDENTIALS) managed by {@link QdmpAuth}.
 * The default is an in-process {@link InMemoryTokenStore}; multi-instance deployments should supply
 * an external implementation (e.g. Redis-backed) so that instances share one cached token instead
 * of each independently racing to refresh it against the qdmp API.
 *
 * <p>User-authorization credentials (from {@code AUTHORIZATION_CODE}) are never stored here -- see
 * {@link QdmpAuth#getUserAccessToken}.
 */
public interface TokenStore {

  /**
   * Returns the currently stored app credential, if any.
   *
   * @return the cached token, or {@link Optional#empty()} if none has been stored yet
   */
  Optional<CachedAppToken> get();

  /**
   * Replaces the stored app credential.
   *
   * @param token the credential to store
   */
  void set(CachedAppToken token);

  /** Removes any stored app credential. */
  void clear();
}
