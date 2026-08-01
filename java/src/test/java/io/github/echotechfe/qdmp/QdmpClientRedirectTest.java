package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.errors.QdmpTransportException;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@link QdmpClient} builds its {@link okhttp3.OkHttpClient} with default settings, which follow
 * 3xx redirects transparently. That is unsafe for an SDK carrying secret credentials in headers
 * (access-token / x-openapi-access-token): a redirect could point at an attacker-controlled host
 * that would then receive those headers. The SDK must never follow a redirect -- any 3xx response
 * from the server must surface as a {@link QdmpTransportException} instead, without leaking the
 * access token that was used for the original request into the exception's message.
 */
class QdmpClientRedirectTest {

  private static final String SENTINEL_TOKEN = "sentinel-secret-token";

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

  /**
   * A bare {@code new OkHttpClient()} would transparently follow this 302 to the second enqueued
   * response and return a perfectly normal 200 success payload -- so this response is queued as a
   * trap: if the SDK ever starts following redirects again, this test starts passing for the wrong
   * reason (silently returning data) instead of failing, unless we also assert only one request was
   * ever sent.
   */
  @Test
  void redirectResponse_isNotFollowed_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(302)
            .setHeader("Location", server.url("/user/v1/me-redirected").toString()));
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{"
                    + "\"id\":\"user-1\",\"nickname\":\"should-not-be-reached\"}}"));
    QdmpClient client = TestClients.create(server);
    QdmpContext ctx = QdmpContext.of(SENTINEL_TOKEN);

    assertThatThrownBy(() -> client.user().me(ctx)).isInstanceOf(QdmpTransportException.class);
    assertThat(server.getRequestCount())
        .as("the redirect target must never be requested -- the SDK must not follow it")
        .isEqualTo(1);
  }

  @Test
  void redirectResponse_exceptionMessage_doesNotLeakAccessToken() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(302)
            .setHeader("Location", server.url("/user/v1/me-redirected").toString()));
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{"
                    + "\"id\":\"user-1\",\"nickname\":\"should-not-be-reached\"}}"));
    QdmpClient client = TestClients.create(server);
    QdmpContext ctx = QdmpContext.of(SENTINEL_TOKEN);

    QdmpTransportException exception =
        (QdmpTransportException)
            org.assertj.core.api.Assertions.catchThrowable(() -> client.user().me(ctx));

    assertThat(exception).isNotNull();
    assertThat(exception.getMessage()).doesNotContain(SENTINEL_TOKEN);
  }

  /**
   * Same contract for a different 3xx status code -- any redirect must be rejected, not just 302.
   */
  @Test
  void permanentRedirectResponse_isNotFollowed_throwsQdmpTransportException() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(301)
            .setHeader("Location", server.url("/user/v1/me-redirected").toString()));
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{"
                    + "\"id\":\"user-1\",\"nickname\":\"should-not-be-reached\"}}"));
    QdmpClient client = TestClients.create(server);
    QdmpContext ctx = QdmpContext.of(SENTINEL_TOKEN);

    assertThatThrownBy(() -> client.user().me(ctx)).isInstanceOf(QdmpTransportException.class);
    assertThat(server.getRequestCount())
        .as("the redirect target must never be requested -- the SDK must not follow it")
        .isEqualTo(1);
  }
}
