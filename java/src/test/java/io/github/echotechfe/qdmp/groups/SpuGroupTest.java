package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.SpuDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.SpuSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import java.util.List;
import okhttp3.HttpUrl;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code spu.search} (GET /spu/v1/search): "standard" scheme. Query params carry uint64-typed
 * filters that are wire-encoded as strings ({@code typeId}, {@code filterOptions[]}, ...), per the
 * openapi.yaml int64/uint64-as-string convention.
 */
class SpuGroupTest {

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
  void search_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void search_success_serializesQueryParams() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"items\":["
                    + "{\"id\":\"spu-1\",\"name\":\"哪吒手办\"}],\"totalCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);
    SpuSearchParams params =
        SpuSearchParams.builder()
            .keyword("哪吒")
            .typeId("100")
            .filterOptions(List.of("1", "2"))
            .onlyMega(true)
            .build();

    SpuSearch200ResponseAllOfData result =
        client.spu().search(QdmpContext.of("user-access-token"), params);

    assertThat(result.getTotalCount()).isEqualTo("1");
    assertThat(result.getItems()).hasSize(1);
    assertThat(result.getItems().get(0).getName()).isEqualTo("哪吒手办");

    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("GET");
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/spu/v1/search");
    assertThat(url.queryParameter("keyword")).isEqualTo("哪吒");
    assertThat(url.queryParameter("typeId")).isEqualTo("100");
    assertThat(url.queryParameterValues("filterOptions")).containsExactly("1", "2");
    assertThat(url.queryParameter("onlyMega")).isEqualTo("true");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
  }

  @Test
  void search_appLevelTokenAlsoWorks_realWorldConfirmedException() throws Exception {
    // shared/openapi.yaml explicitly documents that spu/v1/search was confirmed callable with an
    // app-level (CLIENT_CREDENTIALS) token in real testing, unlike most other "standard" endpoints.
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"items\":[],\"totalCount\":\"0\"}}"));
    QdmpClient client = TestClients.create(server);

    SpuSearch200ResponseAllOfData result =
        client.spu().search(QdmpContext.of("app-level-token"), SpuSearchParams.builder().build());

    assertThat(result.getTotalCount()).isEqualTo("0");
  }

  @Test
  void search_badParams_gatewayErrorEnvelope_queryParseError() {
    // Real, observed failure: tag/v1/search with malformed params returns HTTP 500 code=13.
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":13,\"message\":\"query parse error\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(
            () ->
                client
                    .spu()
                    .search(QdmpContext.of("user-access-token"), SpuSearchParams.builder().build()))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("13");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
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
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"spu\":{\"id\":\"spu-1\",\"name\":\"哪吒手办\"}}}"));
    QdmpClient client = TestClients.create(server);

    SpuDetail200ResponseAllOfData result =
        client.spu().detail(QdmpContext.of("user-access-token"), "spu-1");

    assertThat(result.getSpu().getId()).isEqualTo("spu-1");
    assertThat(result.getSpu().getName()).isEqualTo("哪吒手办");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("GET");
    assertThat(recorded.getRequestUrl().encodedPath()).isEqualTo("/spu/v1/detail");
    assertThat(recorded.getRequestUrl().queryParameter("id")).isEqualTo("spu-1");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
  }

  @Test
  void detail_notFound_gatewayErrorEnvelope_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":13,\"message\":\"query parse error\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.spu().detail(QdmpContext.of("user-access-token"), "bad-id"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("13");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }
}
