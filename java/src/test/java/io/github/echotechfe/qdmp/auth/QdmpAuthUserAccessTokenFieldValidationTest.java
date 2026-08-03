package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@link QdmpAuth#getUserAccessToken}'s Javadoc promises "the full session: access token, refresh
 * token, expiry, and open ID", but the current implementation's malformed-body guard ({@code
 * requireNonMalformed}) only checks {@code accessToken}/{@code expiresAt} -- it never looks at
 * {@code data.refreshToken} or {@code data.openId} at all. A response missing either field is
 * currently handed back to the caller as a session object with a silently-{@code null} (or empty)
 * refresh token / open ID, contradicting the documented contract. Both fields must be validated the
 * same way {@code accessToken}/{@code expiresAt} already are: missing or empty must throw.
 */
class QdmpAuthUserAccessTokenFieldValidationTest {

  private MockWebServer server;

  @BeforeEach
  void startServer() throws IOException {
    server = new MockWebServer();
    server.start();
  }

  @AfterEach
  void stopServer() throws IOException {
    server.shutdown();
  }

  @Test
  void getUserAccessToken_missingRefreshToken_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1900000000\",\"openId\":\"open-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getUserAccessToken("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void getUserAccessToken_emptyRefreshToken_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1900000000\",\"refreshToken\":\"\","
                    + "\"openId\":\"open-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getUserAccessToken("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void getUserAccessToken_missingOpenId_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1900000000\",\"refreshToken\":\"ref\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getUserAccessToken("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void getUserAccessToken_emptyOpenId_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1900000000\",\"refreshToken\":\"ref\",\"openId\":\"\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getUserAccessToken("some-code"))
        .isInstanceOf(RuntimeException.class);
  }
}
