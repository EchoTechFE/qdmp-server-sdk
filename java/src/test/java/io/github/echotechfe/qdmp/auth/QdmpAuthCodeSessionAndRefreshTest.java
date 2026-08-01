package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import io.github.echotechfe.qdmp.errors.QdmpValidationError;
import io.github.echotechfe.qdmp.generated.AuthRefresh200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.AuthToken200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code auth.code2Session(code)} (AUTHORIZATION_CODE grant) and {@code
 * auth.refreshToken(refreshToken)} are one-shot calls: the SDK never caches the resulting
 * user-level tokens, unlike the app-level CLIENT_CREDENTIALS flow covered in {@link
 * QdmpAuthGetAccessTokenTest}.
 */
class QdmpAuthCodeSessionAndRefreshTest {

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
  void code2Session_success_sendsAuthorizationCodeGrant_andReturnsFullSession() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\",\"data\":{"
                    + "\"accessToken\":\"user-access-token\",\"expiresAt\":\"1900000000\","
                    + "\"refreshToken\":\"user-refresh-token\",\"openId\":\"open-id-1\"}}"));
    QdmpClient client = TestClients.create(server);

    AuthToken200ResponseAllOfData session = client.auth().code2Session("wx-login-code-abc");

    assertThat(session.getAccessToken()).isEqualTo("user-access-token");
    assertThat(session.getExpiresAt()).isEqualTo("1900000000");
    assertThat(session.getRefreshToken()).isEqualTo("user-refresh-token");
    assertThat(session.getOpenId()).isEqualTo("open-id-1");
    RecordedRequest request = server.takeRequest();
    JsonNode body = JSON.readTree(request.getBody().readUtf8());
    assertThat(body.get("grantType").asText()).isEqualTo("AUTHORIZATION_CODE");
    assertThat(body.get("code").asText()).isEqualTo("wx-login-code-abc");
  }

  @Test
  void code2Session_isNeverCached_repeatedCallsAlwaysHitTheNetwork() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"tok-1\","
                    + "\"expiresAt\":\"9999999999\",\"refreshToken\":\"r-1\","
                    + "\"openId\":\"o-1\"}}"));
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"tok-2\","
                    + "\"expiresAt\":\"9999999999\",\"refreshToken\":\"r-2\","
                    + "\"openId\":\"o-2\"}}"));
    QdmpClient client = TestClients.create(server);

    AuthToken200ResponseAllOfData first = client.auth().code2Session("code-1");
    AuthToken200ResponseAllOfData second = client.auth().code2Session("code-2");

    assertThat(first.getAccessToken()).isEqualTo("tok-1");
    assertThat(second.getAccessToken()).isEqualTo("tok-2");
    assertThat(server.getRequestCount()).isEqualTo(2);
  }

  @Test
  void code2Session_invalidCode_throwsQdmpApiError() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":10003,\"message\":\"授权码错误\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("bad-code"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10003"));
  }

  @Test
  void refreshToken_success_returnsNewAccessToken() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"new-token\","
                    + "\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    AuthRefresh200ResponseAllOfData result = client.auth().refreshToken("some-refresh-token");

    assertThat(result.getAccessToken()).isEqualTo("new-token");
    RecordedRequest request = server.takeRequest();
    JsonNode body = JSON.readTree(request.getBody().readUtf8());
    assertThat(body.get("refreshToken").asText()).isEqualTo("some-refresh-token");
  }

  /**
   * Real, actually-observed behavior: an expired refreshToken is reported as HTTP 200 with {@code
   * code:10008} -- not as an HTTP error. The SDK must treat this as a failure by inspecting the
   * body, never by trusting the HTTP status code alone.
   */
  @Test
  void refreshToken_expired_isHttp200ButMustStillThrow() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":10008,\"message\":\"refreshToken超过有效期\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("expired-refresh-token"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10008");
              assertThat(err.getHttpStatus()).isEqualTo(200);
            });
  }

  /**
   * grantType=AUTHORIZATION_CODE requires {@code code} (design decision 3): a missing/empty value
   * must fail locally with {@link QdmpValidationError} before any request is sent, matching the
   * Node SDK's {@code QdmpValidationError} check (see node/src/auth.ts).
   */
  @Test
  void code2Session_nullCode_throwsQdmpValidationError_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session(null))
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void code2Session_emptyCode_throwsQdmpValidationError_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session(""))
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void refreshToken_nullRefreshToken_throwsQdmpValidationError_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken(null))
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void refreshToken_emptyRefreshToken_throwsQdmpValidationError_sendsNoRequest() {
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken(""))
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void refreshToken_invalid_throwsWithCode10007() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":10007,\"message\":\"refreshToken错误\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("garbage"))
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10007"));
  }

  /**
   * {@code code2Session} must reject a malformed "success" body the same way {@code
   * getAccessToken()}'s {@code toCachedAppToken} already does (see {@link
   * QdmpAuthGetAccessTokenTest#successCode_withNullData_throwsQdmpTransportException_notBareNpe}):
   * code:"0" but no {@code data} at all must never be handed back to the caller as a silent {@code
   * null} return value -- it must surface as a {@link QdmpTransportException}.
   */
  @Test
  void code2Session_successCode_withNullData_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":\"0\",\"message\":\"ok\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("wx-login-code-abc"))
        .isInstanceOf(QdmpTransportException.class);
  }

  /**
   * Same malformed-success guard as above, but for the more subtle case: {@code data} is present
   * but its {@code accessToken} is missing. The caller must never receive a session object whose
   * {@code accessToken} is silently {@code null}.
   */
  @Test
  void code2Session_successCode_withDataMissingAccessToken_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{"
                    + "\"expiresAt\":\"1900000000\",\"refreshToken\":\"user-refresh-token\","
                    + "\"openId\":\"open-id-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("wx-login-code-abc"))
        .isInstanceOf(QdmpTransportException.class);
  }

  /**
   * Same guard for {@code refreshToken}: code:"0" but no {@code data} at all must not be returned
   * to the caller as a silent {@code null}.
   */
  @Test
  void refreshToken_successCode_withNullData_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":\"0\",\"message\":\"ok\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("some-refresh-token"))
        .isInstanceOf(QdmpTransportException.class);
  }

  /**
   * Same guard for {@code refreshToken}: {@code data} present but {@code accessToken} missing must
   * not be returned to the caller as an object whose {@code accessToken} is silently {@code null}.
   */
  @Test
  void refreshToken_successCode_withDataMissingAccessToken_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("some-refresh-token"))
        .isInstanceOf(QdmpTransportException.class);
  }
}
