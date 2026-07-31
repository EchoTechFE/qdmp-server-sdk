package io.github.echotechfe.qdmp.testsupport;

import io.github.echotechfe.qdmp.auth.CachedAppToken;
import io.github.echotechfe.qdmp.auth.TokenStore;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * A {@link TokenStore} test double that delegates to an in-memory slot but counts how many times
 * {@link #set} was invoked, so single-flight-refresh tests can assert the app-level token was only
 * fetched (and persisted) once despite concurrent callers.
 */
public final class RecordingTokenStore implements TokenStore {

  private final AtomicInteger setCallCount = new AtomicInteger();
  private volatile CachedAppToken current;

  @Override
  public Optional<CachedAppToken> get() {
    return Optional.ofNullable(current);
  }

  @Override
  public void set(CachedAppToken token) {
    setCallCount.incrementAndGet();
    this.current = token;
  }

  @Override
  public void clear() {
    this.current = null;
  }

  public int getSetCallCount() {
    return setCallCount.get();
  }
}
