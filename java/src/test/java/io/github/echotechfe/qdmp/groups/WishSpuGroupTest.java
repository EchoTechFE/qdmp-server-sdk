package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.WishAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.WishAddRequest;
import io.github.echotechfe.qdmp.generated.WishCancelRequest;
import io.github.echotechfe.qdmp.generated.WishList200ResponseAllOfData;
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
 * {@code wishspu.add}/{@code wishspu.cancel} (POST) and {@code wishspu.list} (GET
 * /wishspu/v1/list). {@code cancel} binds its response to the same {@link
 * WishAdd200ResponseAllOfData} type as {@code add} (both return {@code successCount}); this is
 * exercised explicitly below since it is an easy copy-paste mistake to bind to the wrong type.
 */
class WishSpuGroupTest {

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
            .setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"successCount\":\"2\"}}"));
    QdmpClient client = TestClients.create(server);
    WishAddRequest request =
        new WishAddRequest()
            .ids(List.of("spu-1", "spu-2"))
            .type(WishAddRequest.TypeEnum.WISH_SPU_TYPE_SPU);

    WishAdd200ResponseAllOfData result =
        client.wishspu().add(QdmpContext.of("user-access-token"), request);

    assertThat(result.getSuccessCount()).isEqualTo("2");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("POST");
    assertThat(recorded.getPath()).isEqualTo("/wishspu/v1/add");
    assertThat(recorded.getHeader("access-token")).isEqualTo("user-access-token");
    JsonNode body = JSON.readTree(recorded.getBody().readUtf8());
    assertThat(body.get("ids")).extracting(JsonNode::asText).containsExactly("spu-1", "spu-2");
    assertThat(body.get("type").asText()).isEqualTo("WISH_SPU_TYPE_SPU");
  }

  @Test
  void cancel_success_bindsToWishAddResponseType_notConfusedWithList() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"successCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);
    WishCancelRequest request =
        new WishCancelRequest()
            .ids(List.of("spu-1"))
            .type(WishCancelRequest.TypeEnum.WISH_SPU_TYPE_SPU);

    WishAdd200ResponseAllOfData result =
        client.wishspu().cancel(QdmpContext.of("user-access-token"), request);

    assertThat(result.getSuccessCount()).isEqualTo("1");
    RecordedRequest recorded = server.takeRequest();
    assertThat(recorded.getMethod()).isEqualTo("POST");
    assertThat(recorded.getPath()).isEqualTo("/wishspu/v1/cancel");
    JsonNode body = JSON.readTree(recorded.getBody().readUtf8());
    assertThat(body.get("ids")).extracting(JsonNode::asText).containsExactly("spu-1");
  }

  @Test
  void list_success_serializesQueryParams() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"items\":["
                    + "{\"id\":\"wish-1\"}],\"totalCount\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);
    WishListParams params = WishListParams.builder().offset("0").limit("20").typeId("100").build();

    WishList200ResponseAllOfData result =
        client.wishspu().list(QdmpContext.of("user-access-token"), params);

    assertThat(result.getTotalCount()).isEqualTo("1");
    assertThat(result.getItems()).hasSize(1);
    RecordedRequest recorded = server.takeRequest();
    HttpUrl url = recorded.getRequestUrl();
    assertThat(url.encodedPath()).isEqualTo("/wishspu/v1/list");
    assertThat(url.queryParameter("offset")).isEqualTo("0");
    assertThat(url.queryParameter("limit")).isEqualTo("20");
    assertThat(url.queryParameter("typeId")).isEqualTo("100");
  }

  @Test
  void list_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void add_gatewayErrorEnvelope_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":2,\"message\":\"rpc error: openid is required\"}"));
    QdmpClient client = TestClients.create(server);
    WishAddRequest request =
        new WishAddRequest().ids(List.of("spu-1")).type(WishAddRequest.TypeEnum.WISH_SPU_TYPE_SPU);

    assertThatThrownBy(() -> client.wishspu().add(QdmpContext.of("app-level-token"), request))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("2"));
  }
}
