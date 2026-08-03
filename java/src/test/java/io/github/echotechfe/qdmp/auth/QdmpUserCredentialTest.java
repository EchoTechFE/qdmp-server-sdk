package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpUserClient;
import io.github.echotechfe.qdmp.errors.QdmpApiError;
import io.github.echotechfe.qdmp.errors.QdmpValidationError;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequest;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequestContentsInner;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequestContentsInnerPartsInner;
import io.github.echotechfe.qdmp.generated.UserMe200ResponseAllOfData;
import io.github.echotechfe.qdmp.testsupport.MutableClock;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code qdmp.withUserCredential(UserCredentialOptions)} (CONTRACT.md §2): a user-authorization
 * credential wrapper that, unlike the one-shot {@code auth().getUserAccessToken}/{@code
 * auth().refreshToken} calls, keeps the access token alive across calls -- refreshing it 300s ahead
 * of a known expiry, and reactively on an HTTP 401 with business code 10005/10006, retrying the
 * failed business call exactly once with the new token.
 */
class QdmpUserCredentialTest {

  private static final ObjectMapper JSON = new ObjectMapper();
  private static final String REFRESH_PATH = "/auth/v1/refresh";
  private static final String ME_PATH = "/user/v1/me";
  private static final String GENAI_GENERATE_PATH = "/genai/v1/generate";
  private static final String SENTINEL_ACCESS_TOKEN = "sentinel-user-access-token-xyz";
  private static final String SENTINEL_REFRESH_TOKEN = "sentinel-user-refresh-token-abc";

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

  // ---- MockResponse builders -------------------------------------------------

  private static MockResponse refreshSuccess(String accessToken, long expiresAtEpochSeconds) {
    return new MockResponse()
        .setResponseCode(200)
        .setBody(
            "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\""
                + accessToken
                + "\",\"expiresAt\":\""
                + expiresAtEpochSeconds
                + "\"}}");
  }

  private static MockResponse refreshFailure(int businessCode) {
    return new MockResponse()
        .setResponseCode(200)
        .setBody("{\"code\":" + businessCode + ",\"message\":\"refreshToken invalid\"}");
  }

  private static MockResponse meSuccess(String userId) {
    return new MockResponse()
        .setResponseCode(200)
        .setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"" + userId + "\"}}");
  }

  private static MockResponse http401(int businessCode) {
    return new MockResponse()
        .setResponseCode(401)
        .setBody("{\"code\":" + businessCode + ",\"message\":\"access_token invalid\"}");
  }

  private static MockResponse businessError200(int businessCode) {
    return new MockResponse()
        .setResponseCode(200)
        .setBody("{\"code\":" + businessCode + ",\"message\":\"business error\"}");
  }

  private static MockResponse serverError5xx() {
    return new MockResponse()
        .setResponseCode(500)
        .setBody("{\"code\":13,\"message\":\"internal\"}");
  }

  private static MockResponse genaiSuccess(String taskId) {
    return new MockResponse()
        .setResponseCode(200)
        .setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"id\":\"" + taskId + "\"}}");
  }

  private static GenaiGenerateRequest genaiRequest() {
    return new GenaiGenerateRequest()
        .model("google/gemini-3.1-flash-image-preview")
        .addContentsItem(
            new GenaiGenerateRequestContentsInner()
                .role("user")
                .addPartsItem(new GenaiGenerateRequestContentsInnerPartsInner().text("hello")));
  }

  // ---- 2.3.1 proactive refresh (300s buffer, same as app credential) --------

  @Test
  void proactiveRefresh_whenExpiresAtWithinBuffer_refreshesBeforeBusinessCall() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    long almostExpired = t0.getEpochSecond() + 200; // inside the 300s buffer
    server.enqueue(refreshSuccess("refreshed-token-1", t0.getEpochSecond() + 7200));
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("refresh-token-1")
                .expiresAt(almostExpired)
                .build());

    UserMe200ResponseAllOfData me = uc.user().me();

    assertThat(me.getId()).isEqualTo("user-1");
    assertThat(server.getRequestCount()).isEqualTo(2);
    RecordedRequest refreshRequest = server.takeRequest();
    assertThat(refreshRequest.getPath()).isEqualTo(REFRESH_PATH);
    assertThat(refreshRequest.getHeader("access-token")).isNull();
    JsonNode refreshBody = JSON.readTree(refreshRequest.getBody().readUtf8());
    assertThat(refreshBody.get("refreshToken").asText()).isEqualTo("refresh-token-1");
    RecordedRequest meRequest = server.takeRequest();
    assertThat(meRequest.getPath()).isEqualTo(ME_PATH);
    assertThat(meRequest.getHeader("access-token")).isEqualTo("refreshed-token-1");
  }

  @Test
  void noProactiveRefresh_whenExpiresAtOutsideBuffer_callsBusinessDirectly() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    long farFromExpiry = t0.getEpochSecond() + 400; // outside the 300s buffer
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("still-valid-token")
                .refreshToken("refresh-token-1")
                .expiresAt(farFromExpiry)
                .build());

    uc.user().me();

    assertThat(server.getRequestCount()).isEqualTo(1);
    RecordedRequest request = server.takeRequest();
    assertThat(request.getPath()).isEqualTo(ME_PATH);
  }

  @Test
  void noExpiresAt_skipsProactiveRefresh_reliesOnlyOn401() throws Exception {
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("no-expiry-known-token")
                .refreshToken("refresh-token-1")
                .build());

    uc.user().me();

    assertThat(server.getRequestCount()).isEqualTo(1);
    assertThat(server.takeRequest().getPath()).isEqualTo(ME_PATH);
  }

  // ---- 2.3.2 / 2.3.3 refresh action + onRefresh callback ---------------------

  @Test
  void onRefresh_isCalledWithNewAccessTokenAndExpiry_afterProactiveRefresh() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    long expiresAt = t0.getEpochSecond() + 200;
    long newExpiresAt = t0.getEpochSecond() + 7200;
    server.enqueue(refreshSuccess("new-token-for-callback", newExpiresAt));
    server.enqueue(meSuccess("user-1"));
    AtomicReference<RefreshedUserToken> captured = new AtomicReference<>();
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("refresh-token-1")
                .expiresAt(expiresAt)
                .onRefresh(captured::set)
                .build());

    uc.user().me();

    assertThat(captured.get()).isNotNull();
    assertThat(captured.get().getAccessToken()).isEqualTo("new-token-for-callback");
    assertThat(captured.get().getExpiresAtEpochSeconds()).isEqualTo(newExpiresAt);
  }

  @Test
  void refreshToken_staysUnchanged_acrossMultipleRefreshes() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    server.enqueue(refreshSuccess("token-round-1", t0.getEpochSecond() + 350));
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("original-refresh-token")
                .expiresAt(t0.getEpochSecond() + 200)
                .build());
    uc.user().me();
    final RecordedRequest firstRefresh = server.takeRequest();
    server.takeRequest(); // the me() call following the first refresh

    // Advance past the second token's buffer too, forcing a second proactive refresh.
    clock.advanceSeconds(100);
    server.enqueue(refreshSuccess("token-round-2", clock.instant().getEpochSecond() + 7200));
    server.enqueue(meSuccess("user-1"));

    uc.user().me();
    RecordedRequest secondRefresh = server.takeRequest();

    JsonNode firstBody = JSON.readTree(firstRefresh.getBody().readUtf8());
    JsonNode secondBody = JSON.readTree(secondRefresh.getBody().readUtf8());
    assertThat(firstBody.get("refreshToken").asText()).isEqualTo("original-refresh-token");
    assertThat(secondBody.get("refreshToken").asText()).isEqualTo("original-refresh-token");
  }

  // ---- 2.3.4 / 2.3.5 401 retry, exactly once ---------------------------------

  @Test
  void retry401_code10006_refreshesOnce_andRetriesBusinessCallExactlyOnce() throws Exception {
    server.enqueue(http401(10006));
    server.enqueue(refreshSuccess("token-after-401", 9_999_999_999L));
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("stale-token")
                .refreshToken("refresh-token-1")
                .build());

    UserMe200ResponseAllOfData me = uc.user().me();

    assertThat(me.getId()).isEqualTo("user-1");
    assertThat(server.getRequestCount()).isEqualTo(3);
    RecordedRequest firstMe = server.takeRequest();
    assertThat(firstMe.getPath()).isEqualTo(ME_PATH);
    assertThat(firstMe.getHeader("access-token")).isEqualTo("stale-token");
    RecordedRequest refresh = server.takeRequest();
    assertThat(refresh.getPath()).isEqualTo(REFRESH_PATH);
    RecordedRequest retriedMe = server.takeRequest();
    assertThat(retriedMe.getPath()).isEqualTo(ME_PATH);
    assertThat(retriedMe.getHeader("access-token")).isEqualTo("token-after-401");
  }

  @Test
  void retry401_code10005_refreshesOnce_andRetriesBusinessCallExactlyOnce() throws Exception {
    server.enqueue(http401(10005));
    server.enqueue(refreshSuccess("token-after-401-b", 9_999_999_999L));
    server.enqueue(meSuccess("user-1"));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("stale-token")
                .refreshToken("refresh-token-1")
                .build());

    uc.user().me();

    assertThat(server.getRequestCount()).isEqualTo(3);
  }

  @Test
  void retry401_secondFailureAfterRetry_propagatesImmediately_noSecondRefresh() throws Exception {
    server.enqueue(http401(10006));
    server.enqueue(refreshSuccess("token-still-rejected", 9_999_999_999L));
    server.enqueue(http401(10006));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("stale-token")
                .refreshToken("refresh-token-1")
                .build());

    assertThatThrownBy(uc.user()::me)
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10006");
              assertThat(err.getHttpStatus()).isEqualTo(401);
            });
    // Exactly: first business attempt + one refresh + retried business attempt. No further
    // refresh/retry loop even though the retried call failed with the same 401/10006 shape.
    assertThat(server.getRequestCount()).isEqualTo(3);
  }

  // ---- 2.3.6 non-retryable failures must never trigger a refresh ------------

  @Test
  void http5xx_neverTriggersRefresh() {
    server.enqueue(serverError5xx());
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("some-token")
                .refreshToken("refresh-token-1")
                .build());

    assertThatThrownBy(uc.user()::me).isInstanceOf(QdmpApiError.class);

    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  @Test
  void nonAuthBusinessErrorAtHttp200_neverTriggersRefresh() {
    server.enqueue(businessError200(10002));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("some-token")
                .refreshToken("refresh-token-1")
                .build());

    assertThatThrownBy(uc.user()::me)
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10002"));
    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  @Test
  void http401_withUnrelatedBusinessCode_neverTriggersRefresh() {
    // 401 alone is not the refresh trigger -- only 401 + {10005,10006} is. A 401 carrying some
    // other business code must be surfaced as-is.
    server.enqueue(http401(10001));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("some-token")
                .refreshToken("refresh-token-1")
                .build());

    assertThatThrownBy(uc.user()::me)
        .isInstanceOf(QdmpApiError.class)
        .satisfies(
            e -> {
              QdmpApiError err = (QdmpApiError) e;
              assertThat(err.getCode()).isEqualTo("10001");
              assertThat(err.getHttpStatus()).isEqualTo(401);
            });
    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  // ---- 2.3.7 refresh itself failing -----------------------------------------

  @Test
  void refreshItselfFails_propagatesRefreshError_andNeverRetriesBusinessCall() {
    server.enqueue(http401(10006));
    server.enqueue(refreshFailure(10008));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("stale-token")
                .refreshToken("dead-refresh-token")
                .build());

    assertThatThrownBy(uc.user()::me)
        .isInstanceOf(QdmpApiError.class)
        .satisfies(e -> assertThat(((QdmpApiError) e).getCode()).isEqualTo("10008"));
    // Only the original business call plus the failed refresh -- no second business attempt.
    assertThat(server.getRequestCount()).isEqualTo(2);
  }

  // ---- 2.3.3 onRefresh throwing must not roll back the already-updated token ----

  private static final class BoomException extends RuntimeException {
    BoomException(String message) {
      super(message);
    }
  }

  @Test
  void onRefreshThrows_businessCallFails_butNewTokenIsPersistedForNextCall() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    server.enqueue(refreshSuccess("post-refresh-token", t0.getEpochSecond() + 7200));
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("refresh-token-1")
                .expiresAt(t0.getEpochSecond() + 100)
                .onRefresh(
                    t -> {
                      throw new BoomException("db write failed");
                    })
                .build());

    assertThatThrownBy(uc.user()::me).isInstanceOf(BoomException.class);
    // The refresh itself must have happened (and only once); the business call must never have
    // been sent because onRefresh's failure aborts the whole call.
    assertThat(server.getRequestCount()).isEqualTo(1);
    assertThat(server.takeRequest().getPath()).isEqualTo(REFRESH_PATH);

    // A second call, with the clock now far past the old expiry, must go straight to the
    // business endpoint using the token that was already swapped in -- proving it was not rolled
    // back just because onRefresh blew up.
    server.enqueue(meSuccess("user-1"));
    uc.user().me();

    assertThat(server.getRequestCount()).isEqualTo(2);
    RecordedRequest secondCall = server.takeRequest();
    assertThat(secondCall.getPath()).isEqualTo(ME_PATH);
    assertThat(secondCall.getHeader("access-token")).isEqualTo("post-refresh-token");
  }

  // ---- 2.3.8 concurrent single-flight refresh --------------------------------

  @Test
  void concurrentBusinessCalls_refreshHappensOnlyOnce() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    int threadCount = 8;
    server.enqueue(
        refreshSuccess("single-flight-user-token", t0.getEpochSecond() + 7200)
            .setBodyDelay(200, TimeUnit.MILLISECONDS));
    for (int i = 0; i < threadCount; i++) {
      server.enqueue(meSuccess("user-1"));
    }
    AtomicInteger onRefreshCount = new AtomicInteger();
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("refresh-token-1")
                .expiresAt(t0.getEpochSecond() + 100)
                .onRefresh(t -> onRefreshCount.incrementAndGet())
                .build());
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
                results.add(uc.user().me().getId());
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
      assertThat(results).hasSize(threadCount).containsOnly("user-1");
      assertThat(onRefreshCount.get()).as("onRefresh must fire exactly once").isEqualTo(1);

      int refreshRequestCount = 0;
      int meRequestCount = 0;
      for (int i = 0; i < server.getRequestCount(); i++) {
        RecordedRequest request = server.takeRequest();
        if (REFRESH_PATH.equals(request.getPath())) {
          refreshRequestCount++;
        } else if (ME_PATH.equals(request.getPath())) {
          meRequestCount++;
        }
      }
      assertThat(refreshRequestCount)
          .as(
              "only a single refresh request should reach the server despite %d concurrent callers",
              threadCount)
          .isEqualTo(1);
      assertThat(meRequestCount).isEqualTo(threadCount);
    } finally {
      pool.shutdownNow();
    }
  }

  // ---- 2.3.9 credential() is a read-only snapshot ----------------------------

  @Test
  void credential_reflectsLatestToken_andEarlierSnapshotIsUnaffectedByLaterRefresh() {
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken("refresh-token-1")
                .build());

    var snapshotBeforeRefresh = uc.credential();
    assertThat(snapshotBeforeRefresh.getAccessToken()).isEqualTo("initial-token");

    server.enqueue(http401(10006));
    server.enqueue(refreshSuccess("refreshed-op-token", 9_999_999_999L));
    server.enqueue(meSuccess("user-1"));
    uc.user().me();

    var snapshotAfterRefresh = uc.credential();
    assertThat(snapshotAfterRefresh.getAccessToken()).isEqualTo("refreshed-op-token");
    // The object obtained before the refresh must not have mutated in place.
    assertThat(snapshotBeforeRefresh.getAccessToken()).isEqualTo("initial-token");
  }

  // ---- 2.2 local input validation --------------------------------------------

  @Test
  void missingRefreshToken_null_failsLocally_sendsNoHttpRequest() {
    QdmpClient qdmp = TestClients.create(server);

    assertThatThrownBy(
            () -> {
              QdmpUserClient uc =
                  qdmp.withUserCredential(
                      UserCredentialOptions.builder()
                          .accessToken("at-valid")
                          .refreshToken(null)
                          .build());
              uc.user().me();
            })
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void emptyRefreshToken_failsLocally_sendsNoHttpRequest() {
    QdmpClient qdmp = TestClients.create(server);

    assertThatThrownBy(
            () -> {
              QdmpUserClient uc =
                  qdmp.withUserCredential(
                      UserCredentialOptions.builder()
                          .accessToken("at-valid")
                          .refreshToken("")
                          .build());
              uc.user().me();
            })
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void missingAccessToken_null_failsLocally_sendsNoHttpRequest() {
    QdmpClient qdmp = TestClients.create(server);

    assertThatThrownBy(
            () -> {
              QdmpUserClient uc =
                  qdmp.withUserCredential(
                      UserCredentialOptions.builder()
                          .accessToken(null)
                          .refreshToken("rt-valid")
                          .build());
              uc.user().me();
            })
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void emptyAccessToken_failsLocally_sendsNoHttpRequest() {
    QdmpClient qdmp = TestClients.create(server);

    assertThatThrownBy(
            () -> {
              QdmpUserClient uc =
                  qdmp.withUserCredential(
                      UserCredentialOptions.builder()
                          .accessToken("")
                          .refreshToken("rt-valid")
                          .build());
              uc.user().me();
            })
        .isInstanceOf(QdmpValidationError.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  // ---- 2.5 genai group: same auto-renewal, genai-specific headers -----------

  @Test
  void genai_retry401_exactlyOnce_stillUsesGenaiHeaders_notStandardHeaders() throws Exception {
    server.enqueue(http401(10006));
    server.enqueue(refreshSuccess("genai-token-after-401", 9_999_999_999L));
    server.enqueue(genaiSuccess("task-1"));
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("stale-genai-token")
                .refreshToken("refresh-token-1")
                .build());

    uc.genai().generate(genaiRequest());

    assertThat(server.getRequestCount()).isEqualTo(3);
    RecordedRequest firstAttempt = server.takeRequest();
    assertThat(firstAttempt.getPath()).isEqualTo(GENAI_GENERATE_PATH);
    assertThat(firstAttempt.getHeader("x-openapi-access-token")).isEqualTo("stale-genai-token");
    RecordedRequest refresh = server.takeRequest();
    assertThat(refresh.getPath()).isEqualTo(REFRESH_PATH);
    RecordedRequest retriedAttempt = server.takeRequest();
    assertThat(retriedAttempt.getPath()).isEqualTo(GENAI_GENERATE_PATH);
    assertThat(retriedAttempt.getHeader("x-openapi-access-token"))
        .isEqualTo("genai-token-after-401");
    assertThat(retriedAttempt.getHeader("access-token")).isNull();
  }

  // ---- 2.3.10 debug printing must never leak the credential ------------------

  @Test
  void userCredentialOptions_toString_doesNotLeakAccessTokenOrRefreshToken() {
    UserCredentialOptions options =
        UserCredentialOptions.builder()
            .accessToken(SENTINEL_ACCESS_TOKEN)
            .refreshToken(SENTINEL_REFRESH_TOKEN)
            .build();

    assertThat(options.toString()).doesNotContain(SENTINEL_ACCESS_TOKEN);
    assertThat(options.toString()).doesNotContain(SENTINEL_REFRESH_TOKEN);
  }

  @Test
  void credential_toString_doesNotLeakAccessToken() {
    QdmpClient qdmp = TestClients.create(server);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken(SENTINEL_ACCESS_TOKEN)
                .refreshToken(SENTINEL_REFRESH_TOKEN)
                .build());

    var snapshot = uc.credential();

    assertThat(snapshot.toString()).doesNotContain(SENTINEL_ACCESS_TOKEN);
  }

  @Test
  void refreshedUserToken_toString_doesNotLeakAccessToken() throws Exception {
    Instant t0 = Instant.parse("2030-01-01T00:00:00Z");
    MutableClock clock = MutableClock.startingAt(t0);
    server.enqueue(refreshSuccess(SENTINEL_ACCESS_TOKEN, t0.getEpochSecond() + 7200));
    server.enqueue(meSuccess("user-1"));
    AtomicReference<RefreshedUserToken> captured = new AtomicReference<>();
    QdmpClient qdmp = TestClients.create(server, null, clock);
    QdmpUserClient uc =
        qdmp.withUserCredential(
            UserCredentialOptions.builder()
                .accessToken("initial-token")
                .refreshToken(SENTINEL_REFRESH_TOKEN)
                .expiresAt(t0.getEpochSecond() + 100)
                .onRefresh(captured::set)
                .build());

    uc.user().me();

    assertThat(captured.get().toString()).doesNotContain(SENTINEL_ACCESS_TOKEN);
  }
}
