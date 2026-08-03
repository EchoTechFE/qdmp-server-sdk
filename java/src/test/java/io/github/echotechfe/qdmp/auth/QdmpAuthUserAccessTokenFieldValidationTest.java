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
 * {@link QdmpAuth#getUserAccessToken} hands back a complete user-authorization credential: access
 * token, refresh token, expiry, and open ID. These tests lock in that {@code refreshToken} and
 * {@code openId} are validated exactly the way {@code accessToken}/{@code expiresAt} already are --
 * a business-success envelope missing or emptying either one must throw rather than be handed to
 * the caller as a credential carrying a silently-{@code null} (or empty) field.
 *
 * <p>Note this is deliberately stricter than the app-credential path, where an empty {@code openId}
 * is the normal, captured server behaviour and {@code refreshToken} is not required.
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
