package io.github.echotechfe.qdmp.auth;

import java.util.Optional;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Default, process-local {@link TokenStore} implementation. Each instance holds its own state; it
 * does not share tokens across separate {@code QdmpClient} instances or processes.
 */
public final class InMemoryTokenStore implements TokenStore {

  private final AtomicReference<CachedAppToken> current = new AtomicReference<>();

  @Override
  public Optional<CachedAppToken> get() {
    return Optional.ofNullable(current.get());
  }

  @Override
  public void set(CachedAppToken token) {
    current.set(token);
  }

  @Override
  public void clear() {
    current.set(null);
  }
}
