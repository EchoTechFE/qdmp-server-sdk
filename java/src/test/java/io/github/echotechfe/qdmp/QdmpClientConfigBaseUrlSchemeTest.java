package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.Test;

/**
 * {@link QdmpClientConfig.Builder#baseUrl(String)} currently accepts any syntactically valid URL
 * without inspecting its scheme -- {@code http://} and {@code https://} are treated identically.
 * Since every business/auth call carries the app secret and/or an access token as a request header,
 * configuring an {@code http://} base URL (attacker-controlled or just misconfigured) would send
 * those credentials in plaintext over the network.
 *
 * <p>The required fix: {@link QdmpClientConfig.Builder#build()} must reject a base URL whose scheme
 * is {@code http} unless the host is loopback/localhost (127.0.0.1, "localhost" case-insensitively,
 * or any {@code 127.*} address) -- the loopback exception exists because {@link
 * okhttp3.mockwebserver.MockWebServer}, used throughout this test suite, always listens on {@code
 * http://127.0.0.1:PORT}.
 */
class QdmpClientConfigBaseUrlSchemeTest {

  @Test
  void build_withHttpNonLoopbackBaseUrl_throwsIllegalArgumentException() {
    assertThatThrownBy(
            () ->
                QdmpClientConfig.builder()
                    .appId("a")
                    .appSecret("s")
                    .baseUrl("http://evil.example.com")
                    .build())
        .isInstanceOf(IllegalArgumentException.class);
  }

  @Test
  void build_withHttpLoopbackIpBaseUrl_doesNotThrow() {
    assertThatCode(
            () ->
                QdmpClientConfig.builder()
                    .appId("a")
                    .appSecret("s")
                    .baseUrl("http://127.0.0.1:12345")
                    .build())
        .doesNotThrowAnyException();
  }

  @Test
  void build_withHttpLocalhostBaseUrl_doesNotThrow() {
    assertThatCode(
            () ->
                QdmpClientConfig.builder()
                    .appId("a")
                    .appSecret("s")
                    .baseUrl("http://localhost:12345")
                    .build())
        .doesNotThrowAnyException();
  }

  @Test
  void build_withHttpsBaseUrl_doesNotThrow() {
    assertThatCode(
            () ->
                QdmpClientConfig.builder()
                    .appId("a")
                    .appSecret("s")
                    .baseUrl("https://example.com")
                    .build())
        .doesNotThrowAnyException();
  }

  /**
   * {@code isLoopbackOrLocalhost} currently checks {@code lowerHost.startsWith("127.")}, a bare
   * string-prefix test rather than validating that the host is a genuine dotted-decimal IPv4
   * literal. {@code 127.attacker.com} is a syntactically valid hostname -- not an IP address at all
   * -- that DNS could resolve to any attacker-controlled server, yet it passes this check today and
   * is treated as "loopback", letting an {@code http://} base URL past the HTTPS requirement and
   * sending app secret/access token headers in plaintext to that attacker host.
   */
  @Test
  void build_withHttp127PrefixedNonIpHostname_throwsIllegalArgumentException() {
    assertThatThrownBy(
            () ->
                QdmpClientConfig.builder()
                    .appId("a")
                    .appSecret("s")
                    .baseUrl("http://127.attacker.com")
                    .build())
        .isInstanceOf(IllegalArgumentException.class);
  }
}
