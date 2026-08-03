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
 * {@code auth.getAppAccessToken()} manages the app credential (CLIENT_CREDENTIALS): cache,
 * single-flight refresh, pluggable {@link TokenStore}, and the two real response-envelope shapes
 * confirmed against the live API (see shared/openapi.yaml BusinessEnvelopeBase /
 * GatewayErrorEnvelope descriptions).
 */
class QdmpAuthGetAppAccessTokenTest {

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

    AppAccessTokenResult credential = client.auth().getAppAccessToken();

    assertThat(credential.getAccessToken()).isEqualTo("app-token-1");
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

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
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

    AppAccessTokenResult credential = client.auth().getAppAccessToken();

    assertThat(credential.getAccessToken()).isEqualTo("app-token-numeric");
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

    AppAccessTokenResult first = client.auth().getAppAccessToken();
    AppAccessTokenResult second = client.auth().getAppAccessToken();

    assertThat(first.getAccessToken()).isEqualTo("app-token-1");
    assertThat(second.getAccessToken()).isEqualTo("app-token-1");
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

    assertThat(client.auth().getAppAccessToken().getAccessToken()).isEqualTo("token-a");
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

    AppAccessTokenResult refreshed = client.auth().getAppAccessToken();

    assertThat(refreshed.getAccessToken()).isEqualTo("token-b");
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
                results.add(client.auth().getAppAccessToken().getAccessToken());
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
    tokenStore.set(new CachedAppToken("seeded-token", 9_999_999_999L, "seeded-refresh-token", ""));
    QdmpClient client = TestClients.create(server, tokenStore);

    AppAccessTokenResult credential = client.auth().getAppAccessToken();

    assertThat(credential.getAccessToken()).isEqualTo("seeded-token");
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  /**
   * The cache-hit path and the freshly-exchanged path must hand back an equally complete
   * credential: if the cached entry only carried accessToken/expiresAt, a caller served from the
   * cache would silently lose the refreshToken/openId a caller served by a live exchange gets, and
   * could not tell the two apart other than by the missing fields.
   */
  @Test
  void cachedCredential_carriesEveryFieldTheFreshlyExchangedOneDoes() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"app-token-1\",\"expiresAt\":\"9999999999\","
                    + "\"refreshToken\":\"app-refresh-token-1\",\"openId\":\"\"}}"));
    QdmpClient client = TestClients.create(server);

    AppAccessTokenResult fresh = client.auth().getAppAccessToken();
    AppAccessTokenResult cached = client.auth().getAppAccessToken();

    assertThat(server.getRequestCount())
        .as("the second call must be served from cache")
        .isEqualTo(1);
    assertThat(fresh.getAccessToken()).isEqualTo("app-token-1");
    assertThat(fresh.getExpiresAtEpochSeconds()).isEqualTo(9_999_999_999L);
    assertThat(fresh.getRefreshToken()).isEqualTo("app-refresh-token-1");
    assertThat(fresh.getOpenId()).isEmpty();
    assertThat(cached.getAccessToken()).isEqualTo(fresh.getAccessToken());
    assertThat(cached.getExpiresAtEpochSeconds()).isEqualTo(fresh.getExpiresAtEpochSeconds());
    assertThat(cached.getRefreshToken()).isEqualTo(fresh.getRefreshToken());
    assertThat(cached.getOpenId()).isEqualTo(fresh.getOpenId());
  }

  /**
   * The app-credential response reports an empty {@code openId} and is not contractually required
   * to report a {@code refreshToken} at all, so -- unlike the user-authorization path -- neither
   * field may be validated as non-empty. A response carrying only accessToken/expiresAt must still
   * succeed.
   */
  @Test
  void missingRefreshTokenAndOpenId_areToleratedOnTheAppCredentialPath() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"app-token-1\",\"expiresAt\":\"9999999999\"}}"));
    QdmpClient client = TestClients.create(server);

    AppAccessTokenResult credential = client.auth().getAppAccessToken();

    assertThat(credential.getAccessToken()).isEqualTo("app-token-1");
    assertThat(credential.getRefreshToken()).isNull();
    assertThat(credential.getOpenId()).isNull();
  }

  @Test
  void businessEnvelope_nonZeroCode_throwsQdmpApiError_httpStatus200() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody("{\"code\":10002,\"message\":\"密钥不匹配\",\"requestId\":\"req-err\"}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
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

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
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

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
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

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
        .isInstanceOf(QdmpTransportException.class)
        .isNotInstanceOf(NumberFormatException.class);
  }

  /**
   * A fresh {@code POST /auth/v1/token} response with code:"0" (success) but an {@code expiresAt}
   * that is already in the past (here, 1 second past the Unix epoch -- non-negative, so {@link
   * QdmpAuth}'s existing negative-expiresAt guard does not catch it) must not be cached/returned as
   * if it were a usable token: with {@code REFRESH_BUFFER_SECONDS=300}, any real clock reports zero
   * (in fact deeply negative) time remaining before expiry. The freshly-fetched token is never
   * checked against that same freshness requirement before being cached, so today it is persisted
   * into the {@link TokenStore} and handed back to the caller as if valid.
   */
  @Test
  void freshResponse_withAlreadyExpiredExpiresAt_isRejected_andNeverCached() throws Exception {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":"
                    + "{\"accessToken\":\"stale-token\",\"expiresAt\":\"1\"}}"));
    RecordingTokenStore tokenStore = new RecordingTokenStore();
    QdmpClient client = TestClients.create(server, tokenStore);

    assertThatThrownBy(() -> client.auth().getAppAccessToken())
        .isInstanceOf(QdmpTransportException.class);
    assertThat(tokenStore.getSetCallCount()).isZero();
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
            org.assertj.core.api.Assertions.catchThrowable(() -> client.auth().getAppAccessToken());

    assertThat(error).isNotNull();
    assertThat(error.toString()).doesNotContain(secretAppSecret);
    assertThat(String.valueOf(error.getMessage())).doesNotContain(secretAppSecret);
  }
}
