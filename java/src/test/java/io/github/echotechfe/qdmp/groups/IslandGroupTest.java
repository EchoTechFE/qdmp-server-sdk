package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.IslandDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code island.detail} (GET /island/v1/detail): "standard" scheme. Confirmed callable with an
 * app-level (CLIENT_CREDENTIALS) token in real testing, unlike most other "standard" endpoints.
 */
class IslandGroupTest {

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
  void detail_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void detail_success_sendsStandardHeaders_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\",\"data\":"
                    + "{\"island\":{\"id\":\"island-1\",\"name\":\"哪吒岛\",\"joined\":true}}}"));
    QdmpClient client = TestClients.create(server);

    IslandDetail200ResponseAllOfData result =
        client.island().detail(QdmpContext.of("user-access-token"), "island-1");

    assertThat(result.getIsland().getId()).isEqualTo("island-1");
    assertThat(result.getIsland().getName()).isEqualTo("哪吒岛");
    assertThat(result.getIsland().getJoined()).isTrue();
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("GET");
    assertThat(recorded.getRequestUrl().encodedPath()).isEqualTo("/island/v1/detail");
    assertThat(recorded.getRequestUrl().queryParameter("id")).isEqualTo("island-1");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
    assertThat(recorded.getHeader("x-echo-qdmp-version")).isEqualTo("1.0");
  }

  @Test
  void detail_appLevelTokenAlsoWorks_realWorldConfirmedException() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"island\":{\"id\":\"island-2\"}}}"));
    QdmpClient client = TestClients.create(server);

    IslandDetail200ResponseAllOfData result =
        client.island().detail(QdmpContext.of("app-level-token"), "island-2");

    assertThat(result.getIsland().getId()).isEqualTo("island-2");
  }

  @Test
  void detail_notFound_gatewayErrorEnvelope_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":13,\"message\":\"query parse error\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.island().detail(QdmpContext.of("user-access-token"), "bad-id"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("13");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }
}
