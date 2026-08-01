package io.github.echotechfe.qdmp.errors;

import static org.assertj.core.api.Assertions.assertThat;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.assertj.core.api.Assertions;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@link QdmpApiError#toString()} currently interpolates the server-reported {@code message} (and
 * {@code requestId}) verbatim, with no escaping or length bound. A server (or a man-in-the-middle,
 * or simply a buggy upstream) that returns a {@code message} containing raw {@code \r\n} can inject
 * fake log lines into anything that logs the exception via {@code toString()}/{@code getMessage()},
 * and an unbounded-length {@code message} (e.g. hundreds of KB) would be written to logs in full.
 * Both the control-character injection and the length must be sanitized/bounded before they reach
 * {@code toString()}/{@code getMessage()}.
 */
class QdmpApiErrorSanitizationTest {

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
  void message_withEmbeddedCrLf_isSanitized_inToStringAndGetMessage() {
    String injectedMessage = "line1\r\nFAKE injected log line";
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":10002,\"message\":\""
                    + injectedMessage.replace("\r", "\\r").replace("\n", "\\n")
                    + "\",\"requestId\":\"req-1\"}"));
    QdmpClient client = TestClients.create(server);

    QdmpApiError error =
        (QdmpApiError) Assertions.catchThrowable(() -> client.auth().getAccessToken());

    assertThat(error).isNotNull();
    assertThat(error.toString()).doesNotContain("\r").doesNotContain("\n");
    assertThat(String.valueOf(error.getMessage())).doesNotContain("\r").doesNotContain("\n");
  }

  @Test
  void veryLongMessage_isBounded_inToString() {
    String hugeMessage = "x".repeat(200_000);
    server.enqueue(
        new MockResponse()
            .setResponseCode(200)
            .setBody(
                "{\"code\":10002,\"message\":\"" + hugeMessage + "\",\"requestId\":\"req-1\"}"));
    QdmpClient client = TestClients.create(server);

    QdmpApiError error =
        (QdmpApiError) Assertions.catchThrowable(() -> client.auth().getAccessToken());

    assertThat(error).isNotNull();
    assertThat(error.toString().length())
        .as("QdmpApiError#toString() must bound the server-controlled message length")
        .isLessThan(10_000);
  }

  /**
   * {@link QdmpApiError}'s constructor sanitizes {@code message} and {@code requestId} via {@code
   * sanitize(...)} before storing them, but stores {@code code} completely verbatim -- {@code code}
   * is just as server-controlled (see class Javadoc: "the qdmp API is known to return codes outside
   * the documented set") and is echoed by both {@link QdmpApiError#getCode()} and {@link
   * QdmpApiError#toString()}, so an embedded {@code \r\n} in {@code code} can inject fake log lines
   * exactly like an unsanitized {@code message} would.
   */
  @Test
  void code_withEmbeddedCrLf_isSanitized_inToStringAndGetCode() {
    QdmpApiError error = new QdmpApiError("1\r\nFAKE injected", "msg", null, 200);

    assertThat(error.toString()).doesNotContain("\r").doesNotContain("\n");
    assertThat(error.getCode()).doesNotContain("\r").doesNotContain("\n");
  }
}
