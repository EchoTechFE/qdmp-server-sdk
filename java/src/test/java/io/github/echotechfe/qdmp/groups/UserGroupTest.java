package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.generated.UserMe200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code user.me} (GET /user/v1/me): "standard" auth scheme, mandatory user-level access-token, no
 * request body/params -- identity comes entirely from the access-token header.
 */
class UserGroupTest {

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
  void me_missingAccessToken_failsLocally_sendsNoRequest() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);
    assertThatThrownBy(() -> QdmpContext.of("")).isInstanceOf(IllegalArgumentException.class);

    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void me_success_sendsStandardHeaders_andParsesBusinessEnvelope() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\",\"data\":{"
                    + "\"id\":\"user-1\",\"nickname\":\"哪吒\",\"avatar\":\"https://x/avatar.png\"}}"));
    QdmpClient client = TestClients.create(server);
    QdmpContext ctx = QdmpContext.of("user-access-token");

    UserMe200ResponseAllOfData me = client.user().me(ctx);

    assertThat(me.getId()).isEqualTo("user-1");
    assertThat(me.getNickname()).isEqualTo("哪吒");
    RecordedRequest request = server.takeRequest();
    assertThat(request.getMethod()).isEqualTo("GET");
    assertThat(request.getPath()).isEqualTo("/user/v1/me");
    assertThat(request.getHeader("access-token")).isEqualTo("user-access-token");
    assertThat(request.getHeader("x-echo-qdmp-version")).isEqualTo("1.0");
    // GenAI-only headers must never leak onto a "standard" scheme call.
    assertThat(request.getHeader("x-openapi-access-token")).isNull();
  }

  @Test
  void me_qdmpVersionHeader_isConfigurable() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"user-1\"}}"));
    QdmpClient client =
        new QdmpClient(
            io.github.echotechfe.qdmp.QdmpClientConfig.builder()
                .appId(TestClients.APP_ID)
                .appSecret(TestClients.APP_SECRET)
                .baseUrl(server.url("/").toString())
                .qdmpVersion("2.0")
                .build());

    client.user().me(QdmpContext.of("user-access-token"));

    RecordedRequest request = server.takeRequest();
    assertThat(request.getHeader("x-echo-qdmp-version")).isEqualTo("2.0");
  }

  @Test
  void me_success_toleratesUnknownServerFields_forwardCompatibility() throws Exception {
    // Real, reproduced failure mode: the server adding a field to `data` that this SDK's
    // generated model doesn't know about must not break parsing of a code=0 success response.
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"user-1\","
                    + "\"brandNewServerField\":{\"nested\":true}}}"));
    QdmpClient client = TestClients.create(server);

    UserMe200ResponseAllOfData me = client.user().me(QdmpContext.of("user-access-token"));

    assertThat(me.getId()).isEqualTo("user-1");
  }

  @Test
  void me_userNotFound_businessCode20000_throwsQdmpApiError() {
    // Real, documented-outside-x-error-codes example: an app-level token has no bound user.
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":20000,\"message\":\"用户不存在\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.user().me(QdmpContext.of("app-level-token")))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("20000"));
  }

  @Test
  void me_http401_code10005_accessTokenRevoked_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(401)
            .setBody("{\"code\":10005,\"message\":\"access_token 格式错误或已吊销\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.user().me(QdmpContext.of("revoked-token")))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10005");
              assertThat(err.getHttpStatus()).isEqualTo(401);
            });
  }

  @Test
  void me_gatewayErrorEnvelope_hasNoDataOrRequestId() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":2,\"message\":\"rpc error: openid is required\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.user().me(QdmpContext.of("app-level-token")))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("2");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }

  @Test
  void me_failure_doesNotLeakAccessTokenInExceptionMessageOrToString() {
    String sensitiveToken = "Sensitive-Access-Token-Must-Not-Leak";
    server.enqueue(
        new MockResponse()
            .setResponseCode(401)
            .setBody("{\"code\":10005,\"message\":\"access_token 格式错误或已吊销\"}"));
    QdmpClient client = TestClients.create(server);

    QdmpApiError error =
        (QdmpApiError)
            org.assertj.core.api.Assertions.catchThrowable(
                () -> client.user().me(QdmpContext.of(sensitiveToken)));

    assertThat(error).isNotNull();
    assertThat(error.toString()).doesNotContain(sensitiveToken);
    assertThat(String.valueOf(error.getMessage())).doesNotContain(sensitiveToken);
  }
}
