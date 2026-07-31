package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.TagDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.TagSearch200ResponseAllOfData;
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

/** {@code tag.detail} (GET /tag/v1/detail) and {@code tag.search} (GET /tag/v1/search). */
class TagGroupTest {

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
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"tag\":{\"id\":\"tag-1\",\"name\":\"敖丙\"}}}"));
    QdmpClient client = TestClients.create(server);

    TagDetail200ResponseAllOfData result =
        client.tag().detail(QdmpContext.of("user-access-token"), "tag-1");

    assertThat(result.getTag().getId()).isEqualTo("tag-1");
    assertThat(result.getTag().getName()).isEqualTo("敖丙");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("GET");
    assertThat(recorded.getRequestUrl().encodedPath()).isEqualTo("/tag/v1/detail");
    assertThat(recorded.getRequestUrl().queryParameter("id")).isEqualTo("tag-1");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
  }

  @Test
  void search_success_serializesQueryParams() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"items\":["
                    + "{\"id\":\"tag-1\",\"name\":\"敖丙\"}],\"totalCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);
    TagSearchParams params =
        TagSearchParams.builder()
            .keyword("敖丙")
            .typeId("100")
            .filterOptions(List.of("1", "2"))
            .build();

    TagSearch200ResponseAllOfData result =
        client.tag().search(QdmpContext.of("user-access-token"), params);

    assertThat(result.getTotalCount()).isEqualTo("1");
    assertThat(result.getItems()).hasSize(1);
    assertThat(result.getItems().get(0).getName()).isEqualTo("敖丙");
    RecordedRequest recorded = server.takeRequest();
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/tag/v1/search");
    assertThat(url.queryParameter("keyword")).isEqualTo("敖丙");
    assertThat(url.queryParameter("typeId")).isEqualTo("100");
    assertThat(url.queryParameterValues("filterOptions")).containsExactly("1", "2");
  }

  @Test
  void search_badParams_gatewayErrorEnvelope_queryParseError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":13,\"message\":\"query parse error\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(
            () ->
                client
                    .tag()
                    .search(QdmpContext.of("user-access-token"), TagSearchParams.builder().build()))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("13");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }
}
