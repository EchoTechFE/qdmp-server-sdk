package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;

/**
 * The default, process-local {@link TokenStore} implementation used when no external store (e.g.
 * Redis-backed, for multi-instance deployments) is configured.
 */
class InMemoryTokenStoreTest {

  @Test
  void get_beforeAnyTokenStored_isEmpty() {
    TokenStore store = new InMemoryTokenStore();

    assertThat(store.get()).isEmpty();
  }

  @Test
  void set_thenGet_returnsTheSameToken() {
    TokenStore store = new InMemoryTokenStore();
    CachedAppToken token =
        new CachedAppToken("app-level-token", 1_900_000_000L, "app-refresh-token", "");

    store.set(token);

    assertThat(store.get()).isPresent();
    assertThat(store.get().get().getAccessToken()).isEqualTo("app-level-token");
    assertThat(store.get().get().getExpiresAtEpochSeconds()).isEqualTo(1_900_000_000L);
  }

  @Test
  void clear_removesTheStoredToken() {
    TokenStore store = new InMemoryTokenStore();
    store.set(new CachedAppToken("app-level-token", 1_900_000_000L, "app-refresh-token", ""));

    store.clear();

    assertThat(store.get()).isEmpty();
  }

  @Test
  void separateInstances_doNotShareState() {
    TokenStore a = new InMemoryTokenStore();
    TokenStore b = new InMemoryTokenStore();

    a.set(new CachedAppToken("token-a", 1_900_000_000L, "app-refresh-token", ""));

    assertThat(b.get()).isEmpty();
  }
}
