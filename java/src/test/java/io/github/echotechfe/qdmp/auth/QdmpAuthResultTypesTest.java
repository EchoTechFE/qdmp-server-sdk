package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThat;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code getUserAccessToken}/{@code refreshToken} currently return the raw openapi-generator DTOs
 * {@code AuthToken200ResponseAllOfData}/{@code AuthRefresh200ResponseAllOfData}. Those generated
 * classes' {@code toString()} is produced by the openapi-generator template and prints every field
 * verbatim, including {@code accessToken}/{@code refreshToken} -- so a caller doing the extremely
 * common {@code logger.info("session={}", session)} would write the plaintext token straight into
 * logs. Generated files can't be hand-edited (they get overwritten by the next {@code ./gradlew
 * codegen} run), so the fix is to introduce hand-written result types, matching the naming already
 * used by the Go SDK: {@link UserAccessTokenResult}
 * (accessToken/refreshToken/expiresAtEpochSeconds/openId) and {@link RefreshTokenResult}
 * (accessToken/expiresAtEpochSeconds), neither of which may print the actual token values from
 * {@code toString()}.
 */
class QdmpAuthResultTypesTest {

  private static final String SENTINEL_ACCESS_TOKEN = "sentinel-access-token-xyz";
  private static final String SENTINEL_REFRESH_TOKEN = "sentinel-refresh-token-abc";

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
  void getUserAccessToken_returnsUserAccessTokenResult_withMatchingGetters() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                    + SENTINEL_ACCESS_TOKEN
                    + "\",\"expiresAt\":\"1900000000\",\"refreshToken\":\""
                    + SENTINEL_REFRESH_TOKEN
                    + "\",\"openId\":\"open-id-1\"}}"));
    QdmpClient client = TestClients.create(server);

    UserAccessTokenResult session = client.auth().getUserAccessToken("some-code");

    assertThat(session.getAccessToken()).isEqualTo(SENTINEL_ACCESS_TOKEN);
    assertThat(session.getRefreshToken()).isEqualTo(SENTINEL_REFRESH_TOKEN);
    assertThat(session.getExpiresAtEpochSeconds()).isEqualTo(1900000000L);
    assertThat(session.getOpenId()).isEqualTo("open-id-1");
  }

  @Test
  void getUserAccessToken_toString_doesNotLeakAccessTokenOrRefreshToken() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                    + SENTINEL_ACCESS_TOKEN
                    + "\",\"expiresAt\":\"1900000000\",\"refreshToken\":\""
                    + SENTINEL_REFRESH_TOKEN
                    + "\",\"openId\":\"open-id-1\"}}"));
    QdmpClient client = TestClients.create(server);

    UserAccessTokenResult session = client.auth().getUserAccessToken("some-code");

    assertThat(session.toString()).doesNotContain(SENTINEL_ACCESS_TOKEN);
    assertThat(session.toString()).doesNotContain(SENTINEL_REFRESH_TOKEN);
  }

  /**
   * Regression test (codex convergence-check finding): {@code openId} is only validated as
   * non-empty (see {@code QdmpAuth.requireNonEmptyField}), so a server-controlled value containing
   * embedded CR/LF would, without this fix, survive verbatim into {@code toString()} and forge
   * extra log lines via {@code logger.info("session={}", session)} -- the exact log-injection class
   * of bug already fixed for message/code/requestId elsewhere. {@code getOpenId()} must still
   * return the raw value unchanged; only the debug rendering is sanitized.
   */
  @Test
  void getUserAccessToken_toString_doesNotLeakRawCrLfInOpenId() {
    String injectedOpenId = "open-id-1\\r\\nFAKE injected log line";
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                    + SENTINEL_ACCESS_TOKEN
                    + "\",\"expiresAt\":\"1900000000\",\"refreshToken\":\""
                    + SENTINEL_REFRESH_TOKEN
                    + "\",\"openId\":\""
                    + injectedOpenId
                    + "\"}}"));
    QdmpClient client = TestClients.create(server);

    UserAccessTokenResult session = client.auth().getUserAccessToken("some-code");

    assertThat(session.toString()).doesNotContain("\r").doesNotContain("\n");
    // Reverse-direction guard: getOpenId() must still return the raw value untouched.
    assertThat(session.getOpenId()).isEqualTo("open-id-1\r\nFAKE injected log line");
  }

  @Test
  void refreshToken_returnsRefreshTokenResult_withMatchingGetters() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                    + SENTINEL_ACCESS_TOKEN
                    + "\",\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    RefreshTokenResult result = client.auth().refreshToken("some-refresh-token");

    assertThat(result.getAccessToken()).isEqualTo(SENTINEL_ACCESS_TOKEN);
    assertThat(result.getExpiresAtEpochSeconds()).isEqualTo(1900000000L);
  }

  @Test
  void refreshToken_toString_doesNotLeakAccessToken() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                    + SENTINEL_ACCESS_TOKEN
                    + "\",\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    RefreshTokenResult result = client.auth().refreshToken("some-refresh-token");

    assertThat(result.toString()).doesNotContain(SENTINEL_ACCESS_TOKEN);
  }
}
