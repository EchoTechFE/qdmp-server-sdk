package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import io.github.echotechfe.qdmp.testsupport.MutableClock;
import io.github.echotechfe.qdmp.testsupport.RecordingTokenStore;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code auth.getAccessToken()} manages the app-level (CLIENT_CREDENTIALS) token: cache,
 * single-flight refresh, pluggable {@link TokenStore}, and the two real response-envelope shapes
 * confirmed against the live API (see shared/openapi.yaml BusinessEnvelopeBase /
 * GatewayErrorEnvelope descriptions).
 */
class QdmpAuthGetAccessTokenTest {

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
  void firstCall_sendsClientCredentialsRequest_andReturnsAccessToken() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\","
                    + "\"data\":{\"accessToken\":\"app-token-1\",\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    String token = client.auth().getAccessToken();

    assertThat(token).isEqualTo("app-token-1");
    RecordedRequest request = server.takeRequest();
    assertThat(request.getMethod()).isEqualTo("POST");
    assertThat(request.getPath()).isEqualTo("/auth/v1/token");
    JsonNode body = JSON.readTree(request.getBody().readUtf8());
    assertThat(body.get("grantType").asText()).isEqualTo("CLIENT_CREDENTIALS");
    assertThat(body.get("appId").asText()).isEqualTo(TestClients.APP_ID);
    assertThat(body.get("appSecret").asText()).isEqualTo(TestClients.APP_SECRET);
  }

  @Test
  void emptyAccessToken_isRejected_andNeverCached() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"requestId\":\"req-1\","
                    + "\"data\":{\"accessToken\":\"\",\"expiresAt\":\"1900000000\"}}"));
    RecordingTokenStore tokenStore = new RecordingTokenStore();
    QdmpClient client = TestClients.create(server, tokenStore);

    assertThatThrownBy(() -> client.auth().getAccessToken())
        .isInstanceOf(QdmpTransportException.class);
    assertThat(tokenStore.getSetCallCount()).isZero();
  }

  @Test
  void numericZeroCode_isTreatedAsSuccessJustLikeStringZero() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":0,\"message\":\"ok\",\"requestId\":\"req-1\","
                    + "\"data\":{\"accessToken\":\"app-token-numeric\","
                    + "\"expiresAt\":\"1900000000\"}}"));
    QdmpClient client = TestClients.create(server);

    String token = client.auth().getAccessToken();

    assertThat(token).isEqualTo("app-token-numeric");
  }

  @Test
  void secondCallBeforeExpiry_isServedFromCache_noSecondRequest() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"app-token-1\",\"expiresAt\":\"9999999999\"}}"));
    QdmpClient client = TestClients.create(server);

    String first = client.auth().getAccessToken();
    String second = client.auth().getAccessToken();

    assertThat(first).isEqualTo("app-token-1");
    assertThat(second).isEqualTo("app-token-1");
    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  @Test
  void withinRefreshBuffer_triggersProactiveRefresh_beforeExpiry() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    long firstExpiresAt = t0.getEpochSecond() + 400; // 400s out: outside the 300s buffer
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"token-a\",\"expiresAt\":\""
                    + firstExpiresAt
                    + "\"}}"));
    QdmpClient client = TestClients.create(server, null, clock);

    assertThat(client.auth().getAccessToken()).isEqualTo("token-a");
    assertThat(server.getRequestCount()).isEqualTo(1);

    // Advance past (expiresAt - 300s buffer): 400s - 150s = 250s remaining < 300s buffer.
    clock.advanceSeconds(150);
    long secondExpiresAt = clock.instant().getEpochSecond() + 400;
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"token-b\",\"expiresAt\":\""
                    + secondExpiresAt
                    + "\"}}"));

    String refreshed = client.auth().getAccessToken();

    assertThat(refreshed).isEqualTo("token-b");
    assertThat(server.getRequestCount()).isEqualTo(2);
  }

  @Test
  void concurrentCalls_singleFlight_onlyOneRequestReachesTheServer() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBodyDelay(200, TimeUnit.MILLISECONDS)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"single-flight-token\",\"expiresAt\":\"9999999999\"}}"));
    RecordingTokenStore tokenStore = new RecordingTokenStore();
    QdmpClient client = TestClients.create(server, tokenStore);
    int threadCount = 8;
    ExecutorService pool = Executors.newFixedThreadPool(threadCount);
    CountDownLatch ready = new CountDownLatch(threadCount);
    CountDownLatch start = new CountDownLatch(1);
    CountDownLatch done = new CountDownLatch(threadCount);
    List<String> results = new CopyOnWriteArrayList<>();

    try {
      for (int i = 0; i < threadCount; i++) {
        pool.submit(
            () -> {
              ready.countDown();
              try {
                start.await();
                results.add(client.auth().getAccessToken());
              } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
              } finally {
                done.countDown();
              }
            });
      }
      ready.await();
      start.countDown();
      boolean finished = done.await(10, TimeUnit.SECONDS);

      assertThat(finished).as("all callers finished within timeout").isTrue();
      assertThat(server.getRequestCount())
          .as(
              "only a single CLIENT_CREDENTIALS request should be sent despite %d concurrent"
                  + " callers",
              threadCount)
          .isEqualTo(1);
      assertThat(results).hasSize(threadCount).containsOnly("single-flight-token");
      assertThat(tokenStore.getSetCallCount())
          .as("the token should be persisted into the store exactly once")
          .isEqualTo(1);
    } finally {
      pool.shutdownNow();
    }
  }

  @Test
  void prePopulatedTokenStore_isUsedWithoutAnyNetworkCall() throws Exception {
    RecordingTokenStore tokenStore = new RecordingTokenStore();
    tokenStore.set(new CachedAppToken("seeded-token", 9_999_999_999L));
    QdmpClient client = TestClients.create(server, tokenStore);

    String token = client.auth().getAccessToken();

    assertThat(token).isEqualTo("seeded-token");
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void businessEnvelope_nonZeroCode_throwsQdmpApiError_httpStatus200() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":10002,\"message\":\"密钥不匹配\",\"requestId\":\"req-err\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAccessToken())
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10002");
              assertThat(err.getHttpStatus()).isEqualTo(200);
              assertThat(err.getRequestId()).isEqualTo("req-err");
            });
  }

  @Test
  void gatewayErrorEnvelope_hasNoDataOrRequestId_stillParsesAndThrows() {
    // Real shape observed for e.g. tag/v1/search bad params: {code, message, details}, no
    // data/requestId.
    server.enqueue(
        new MockResponse()
            .setResponseCode(500)
            .setBody("{\"code\":13,\"message\":\"query parse error\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAccessToken())
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("13");
              assertThat(err.getHttpStatus()).isEqualTo(500);
            });
  }

  /**
   * A malformed success response (code:"0" but no {@code data}) must not surface as a bare
   * NullPointerException -- it should be one of the SDK's typed exceptions.
   */
  @Test
  void successCode_withNullData_throwsQdmpTransportException_notBareNpe() {
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":\"0\",\"message\":\"ok\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAccessToken())
        .isInstanceOf(QdmpTransportException.class)
        .isNotInstanceOf(NullPointerException.class);
  }

  /**
   * A malformed success response with a non-numeric {@code expiresAt} must not surface as a bare
   * NumberFormatException -- it should be one of the SDK's typed exceptions.
   */
  @Test
  void successCode_withNonNumericExpiresAt_throwsQdmpTransportException_notBareNfe() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"tok\",\"expiresAt\":\"not-a-number\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAccessToken())
        .isInstanceOf(QdmpTransportException.class)
        .isNotInstanceOf(NumberFormatException.class);
  }

  @Test
  void failure_doesNotLeakAppSecretInExceptionMessageOrToString() {
    String secretAppSecret = "S3cr3t-App-Secret-Must-Not-Leak";
    server.enqueue(
        new MockResponse().setResponseCode(200).setBody("{\"code\":10002,\"message\":\"密钥不匹配\"}"));
    QdmpClient client =
        new QdmpClient(
            io.github.echotechfe.qdmp.QdmpClientConfig.builder()
                .appId(TestClients.APP_ID)
                .appSecret(secretAppSecret)
                .baseUrl(server.url("/").toString())
                .build());

    QdmpApiError error =
        (QdmpApiError)
            org.assertj.core.api.Assertions.catchThrowable(() -> client.auth().getAccessToken());

    assertThat(error).isNotNull();
    assertThat(error.toString()).doesNotContain(secretAppSecret);
    assertThat(String.valueOf(error.getMessage())).doesNotContain(secretAppSecret);
  }
}
