package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.GenaiDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerate200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequest;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequestContentsInner;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequestContentsInnerPartsInner;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import java.util.List;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code genai.generate} (POST /genai/v1/generate): the one "genai" auth scheme, with its own
 * independent header pair (x-openapi-access-token / x-openapi-app-id) instead of the "standard"
 * scheme's (access-token / x-echo-qdmp-version).
 */
class GenaiGroupTest {

  private static final ObjectMapper JSON = new ObjectMapper();

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
  void generate_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void generate_success_usesGenaiHeaders_notStandardHeaders() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\","
                    + "\"data\":{\"id\":\"task-1\"}}"));
    QdmpClient client = TestClients.create(server);
    GenaiGenerateRequest request =
        new GenaiGenerateRequest()
            .model("google/gemini-3.1-flash-image-preview")
            .addContentsItem(
                new GenaiGenerateRequestContentsInner()
                    .role("user")
                    .addPartsItem(new GenaiGenerateRequestContentsInnerPartsInner().text("hello")));

    GenaiGenerate200ResponseAllOfData result =
        client.genai().generate(QdmpContext.of("genai-access-token"), request);

    assertThat(result.getId()).isEqualTo("task-1");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("POST");
    assertThat(recorded.getPath()).isEqualTo("/genai/v1/generate");
    assertThat(recorded.getHeader("x-openapi-access-token")).isEqualTo("genai-access-token");
    assertThat(recorded.getHeader("x-openapi-app-id")).isEqualTo(TestClients.APP_ID);
    // The "standard" scheme's headers must never appear on a genai call.
    assertThat(recorded.getHeader("access-token")).isNull();
    assertThat(recorded.getHeader("x-echo-qdmp-version")).isNull();

    JsonNode body = JSON.readTree(recorded.getBody().readUtf8());
    assertThat(body.get("model").asText()).isEqualTo("google/gemini-3.1-flash-image-preview");
    assertThat(body.get("contents").get(0).get("role").asText()).isEqualTo("user");
    assertThat(body.get("contents").get(0).get("parts").get(0).get("text").asText())
        .isEqualTo("hello");
  }

  @Test
  void generate_businessCodeNonZero_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":10005,\"message\":\"令牌无效\"}"));
    QdmpClient client = TestClients.create(server);
    GenaiGenerateRequest request =
        new GenaiGenerateRequest()
            .model("google/gemini-3.1-flash-image-preview")
            .contents(List.of(new GenaiGenerateRequestContentsInner().role("user")));

    assertThatThrownBy(() -> client.genai().generate(QdmpContext.of("bad-genai-token"), request))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10005"));
  }

  @Test
  void detail_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void detail_success_usesGenaiHeaders_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"task-1\","
                    + "\"status\":\"COMPLETED\",\"response\":{\"text\":\"hi\"}}}"));
    QdmpClient client = TestClients.create(server);

    GenaiDetail200ResponseAllOfData result =
        client.genai().detail(QdmpContext.of("genai-access-token"), "task-1");

    assertThat(result.getId()).isEqualTo("task-1");
    assertThat(result.getStatus()).isEqualTo(GenaiDetail200ResponseAllOfData.StatusEnum.COMPLETED);
    assertThat(result.getResponse()).containsEntry("text", "hi");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("GET");
    assertThat(recorded.getRequestUrl().encodedPath()).isEqualTo("/genai/v1/detail");
    assertThat(recorded.getRequestUrl().queryParameter("id")).isEqualTo("task-1");
    assertThat(recorded.getHeader("x-openapi-access-token")).isEqualTo("genai-access-token");
    assertThat(recorded.getHeader("access-token")).isNull();
  }

  @Test
  void detail_businessCodeNonZero_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":10005,\"message\":\"令牌无效\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.genai().detail(QdmpContext.of("bad-genai-token"), "task-1"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10005"));
  }
}
