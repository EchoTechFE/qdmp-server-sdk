package io.github.echotechfe.qdmp.auth;

import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code code2Session}/{@code refreshToken} currently validate {@code data.expiresAt} only via
 * {@code requireNonMalformed}, which checks for {@code null}/empty -- it never parses the string as
 * a number. That is inconsistent with the CLIENT_CREDENTIALS caching path ({@code
 * toCachedAppToken}), which does call {@code Long.parseLong(data.getExpiresAt())} and turns a
 * {@link NumberFormatException} into a typed {@link
 * io.github.echotechfe.qdmp.errors.QdmpTransportException}. A non-numeric or negative {@code
 * expiresAt} must be rejected on every one of the three auth response paths, not just
 * CLIENT_CREDENTIALS -- and {@code toCachedAppToken}'s existing {@code Long.parseLong} call must
 * also be tightened to reject negative values, which it currently accepts silently (an existing
 * bug, not just a gap in the new code2Session/refreshToken paths).
 */
class QdmpAuthExpiresAtValidationTest {

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
  void code2Session_nonNumericExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"garbage\",\"refreshToken\":\"ref\","
                    + "\"openId\":\"open-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void code2Session_negativeExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"-100\",\"refreshToken\":\"ref\",\"openId\":\"open-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void refreshToken_nonNumericExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"garbage\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("some-refresh-token"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void refreshToken_negativeExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"-100\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("some-refresh-token"))
        .isInstanceOf(RuntimeException.class);
  }

  /**
   * Regression test (codex convergence-check finding): a non-negative but already-expired {@code
   * expiresAt} (e.g. {@code "1"}, 1970-01-01) passes {@code parseExpiresAt} (which only rejects
   * negative/non-numeric values) and must still be rejected by {@code requireNotAlreadyExpired} --
   * otherwise a caller could persist or start relying on a session/token that is already dead on
   * arrival.
   */
  @Test
  void code2Session_alreadyExpiredExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1\",\"refreshToken\":\"ref\",\"openId\":\"open-1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().code2Session("some-code"))
        .isInstanceOf(RuntimeException.class);
  }

  @Test
  void refreshToken_alreadyExpiredExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
                    + "\"expiresAt\":\"1\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().refreshToken("some-refresh-token"))
        .isInstanceOf(RuntimeException.class);
  }

  /**
   * Existing bug in the CLIENT_CREDENTIALS caching path: {@code toCachedAppToken} calls {@code
   * Long.parseLong(data.getExpiresAt())}, which happily accepts a negative number instead of
   * rejecting it -- so a malformed/negative expiry currently gets cached and returned to the caller
   * as if it were valid.
   */
  @Test
  void getAccessToken_negativeExpiresAt_throws() {
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"app-token\","
                    + "\"expiresAt\":\"-100\"}}"));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getAccessToken()).isInstanceOf(RuntimeException.class);
  }
}
