package io.github.echotechfe.qdmp.testsupport;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpClientConfig;
import io.github.echotechfe.qdmp.auth.TokenStore;
import java.time.Clock;
import okhttp3.mockwebserver.MockWebServer;

/** Builds {@link QdmpClient} instances wired to a local {@link MockWebServer} for tests. */
public final class TestClients {

  public static final String APP_ID = "test-app-id";
  public static final String APP_SECRET = "test-app-secret";

  private TestClients() {}

  public static QdmpClient create(MockWebServer server) {
    return create(server, null, null);
  }

  public static QdmpClient create(MockWebServer server, TokenStore tokenStore) {
    return create(server, tokenStore, null);
  }

  /**
   * Builds a {@link QdmpClient} wired to {@code server}, optionally overriding the token store
   * and/or clock used for app-level token caching.
   *
   * @param server the local mock server to point the client's base URL at
   * @param tokenStore the token store to use, or {@code null} to use the default in-memory one
   * @param clock the clock to use for token expiry checks, or {@code null} to use the system clock
   * @return a configured {@link QdmpClient}
   */
  public static QdmpClient create(MockWebServer server, TokenStore tokenStore, Clock clock) {
    QdmpClientConfig.Builder builder =
        QdmpClientConfig.builder()
            .appId(APP_ID)
            .appSecret(APP_SECRET)
            .baseUrl(server.url("/").toString());
    if (tokenStore != null) {
      builder.tokenStore(tokenStore);
    }
    if (clock != null) {
      builder.clock(clock);
    }
    return new QdmpClient(builder.build());
  }
}
