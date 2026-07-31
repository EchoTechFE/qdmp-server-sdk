package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.MarkAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkAddRequest;
import io.github.echotechfe.qdmp.generated.MarkAddRequestRating;
import io.github.echotechfe.qdmp.generated.MarkDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkList200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.HttpUrl;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** {@code mark.add} (POST /mark/v1/add): "standard" scheme, mandatory user-level access-token. */
class MarkGroupTest {

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
  void add_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void add_success_serializesRequestBody_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\","
                    + "\"data\":{\"id\":\"mark-1\"}}"));
    QdmpClient client = TestClients.create(server);
    MarkAddRequest request =
        new MarkAddRequest().spuId("spu-42").rating(new MarkAddRequestRating().value(5));

    MarkAdd200ResponseAllOfData result =
        client.mark().add(QdmpContext.of("user-access-token"), request);

    assertThat(result.getId()).isEqualTo("mark-1");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("POST");
    assertThat(recorded.getPath()).isEqualTo("/mark/v1/add");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
    assertThat(recorded.getHeader("x-echo-qdmp-version")).isEqualTo("1.0");
    JsonNode body = JSON.readTree(recorded.getBody().readUtf8());
    assertThat(body.get("spuId").asText()).isEqualTo("spu-42");
    assertThat(body.get("rating").get("value").asInt()).isEqualTo(5);
  }

  @Test
  void add_http401_code10005_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(401)
            .setBody("{\"code\":10005,\"message\":\"access_token 格式错误或已吊销\"}"));
    QdmpClient client = TestClients.create(server);
    MarkAddRequest request = new MarkAddRequest().spuId("spu-1");

    assertThatThrownBy(() -> client.mark().add(QdmpContext.of("revoked-token"), request))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10005");
              assertThat(err.getHttpStatus()).isEqualTo(401);
            });
  }

  @Test
  void add_gatewayErrorEnvelope_openidRequired_throwsQdmpApiError() {
    // Real, observed shape: app-level token used against a user-scoped endpoint.
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":2,\"message\":\"rpc error: openid is required\"}"));
    QdmpClient client = TestClients.create(server);
    MarkAddRequest request = new MarkAddRequest().spuId("spu-1");

    assertThatThrownBy(() -> client.mark().add(QdmpContext.of("app-level-token"), request))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("2");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }

  @Test
  void list_success_serializesQueryParams_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"items\":["
                    + "{\"id\":\"mark-1\"}],\"totalCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);

    MarkList200ResponseAllOfData result =
        client.mark().list(QdmpContext.of("user-access-token"), "10", "0");

    assertThat(result.getTotalCount()).isEqualTo("1");
    assertThat(result.getItems()).hasSize(1);
    RecordedRequest recorded = server.takeRequest();
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/mark/v1/me/list");
    assertThat(url.queryParameter("limit")).isEqualTo("10");
    assertThat(url.queryParameter("offset")).isEqualTo("0");
  }

  @Test
  void list_nullLimit_throwsNullPointerExceptionWithMessage_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.mark().list(QdmpContext.of("user-access-token"), null, "0"))
        .isInstanceOf(NullPointerException.class)
        .hasMessageContaining("limit");
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void search_success_serializesQueryParams_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"items\":["
                    + "{\"id\":\"mark-1\"}],\"totalCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);
    MarkSearchParams params =
        MarkSearchParams.builder().typeId("100").limit("10").offset("0").build();

    MarkSearch200ResponseAllOfData result =
        client.mark().search(QdmpContext.of("user-access-token"), params);

    assertThat(result.getTotalCount()).isEqualTo("1");
    RecordedRequest recorded = server.takeRequest();
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/mark/v1/me/search");
    assertThat(url.queryParameter("typeId")).isEqualTo("100");
    assertThat(url.queryParameter("limit")).isEqualTo("10");
    assertThat(url.queryParameter("offset")).isEqualTo("0");
  }

  @Test
  void detail_success_serializesQueryParams_andParsesResponse() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"mark-1\","
                    + "\"typeName\":\"手办\",\"hasMore\":false}}"));
    QdmpClient client = TestClients.create(server);

    MarkDetail200ResponseAllOfData result =
        client.mark().detail(QdmpContext.of("user-access-token"), "mark-1", "10", "0");

    assertThat(result.getId()).isEqualTo("mark-1");
    assertThat(result.getTypeName()).isEqualTo("手办");
    assertThat(result.getHasMore()).isFalse();
    RecordedRequest recorded = server.takeRequest();
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/mark/v1/me/detail");
    assertThat(url.queryParameter("id")).isEqualTo("mark-1");
    assertThat(url.queryParameter("limit")).isEqualTo("10");
    assertThat(url.queryParameter("offset")).isEqualTo("0");
  }

  @Test
  void detail_nullId_throwsNullPointerExceptionWithMessage_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(
            () -> client.mark().detail(QdmpContext.of("user-access-token"), null, "10", "0"))
        .isInstanceOf(NullPointerException.class)
        .hasMessageContaining("id");
    assertThat(server.getRequestCount()).isEqualTo(0);
  }
}
