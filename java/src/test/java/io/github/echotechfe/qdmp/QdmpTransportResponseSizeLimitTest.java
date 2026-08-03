package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@code QdmpTransport#readBody} currently calls {@code body.string()} with no size limit at all --
 * a malicious or malfunctioning endpoint can return an arbitrarily large response body and the SDK
 * will buffer the entire thing into memory before even attempting to parse it. This test drives a
 * real {@code getUserAccessToken} call against a response whose body is otherwise perfectly
 * well-formed (valid JSON, valid accessToken/expiresAt/refreshToken/openId) except for one padding
 * field that balloons it to roughly 11 MB, to prove the requirement is about size, not malformed
 * content: today this call succeeds (no size cap exists), which is exactly the gap this test should
 * catch until a response-size limit is enforced and the oversized response is rejected instead of
 * parsed.
 */
class QdmpTransportResponseSizeLimitTest {

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
  void oversizedResponseBody_isRejected_ratherThanFullyBufferedAndParsed() {
    String padding = "a".repeat(11 * 1024 * 1024);
    String body =
        "{\"code\":\"0\",\"message\":\"ok\",\"data\":{\"accessToken\":\"acc\","
            + "\"expiresAt\":\"1900000000\",\"refreshToken\":\"ref\",\"openId\":\"open-1\","
            + "\"padding\":\""
            + padding
            + "\"}}";
    server.enqueue(new MockResponse().setResponseCode(200).setBody(body));
    QdmpClient client = TestClients.create(server);

    assertThatThrownBy(() -> client.auth().getUserAccessToken("some-code"))
        .as("an ~11MB response body must be rejected, not fully buffered into memory and parsed")
        .isInstanceOf(RuntimeException.class);
  }
}
