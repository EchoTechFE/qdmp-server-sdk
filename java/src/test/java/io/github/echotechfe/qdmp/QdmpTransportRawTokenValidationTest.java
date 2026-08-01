package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.generated.AuthToken200ResponseAllOfData;
import java.io.IOException;
import okhttp3.OkHttpClient;
import okhttp3.mockwebserver.MockWebServer;
import org.assertj.core.api.Assertions;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@link QdmpTransport} is a {@code public final class} with a public constructor and public {@code
 * get}/{@code post} methods that accept a bare {@code String accessToken} straight from the caller
 * -- {@link QdmpContext}'s header-safety validation ({@link QdmpContext#of(String)}/{@code
 * requireHeaderSafe}) is never consulted on this path. A caller that bypasses {@link QdmpContext}
 * and constructs {@link QdmpTransport} directly (it is public only because business groups and auth
 * live in separate packages, not because direct use is supported/safe) can therefore push a token
 * containing arbitrary characters -- including raw control characters -- straight into OkHttp's
 * header machinery.
 *
 * <p>Empirically confirmed against okhttp 4.12.0: {@code new
 * Request.Builder().header("access-token", "raw\nSECRET-TOKEN-DO-NOT-LEAK")} throws {@code
 * IllegalArgumentException: Unexpected char 0x0a at 3 in access-token value: raw\nSECRET-TOKEN...},
 * i.e. OkHttp's own header validation embeds the *entire* raw (invalid) header value -- including
 * the secret token -- into its exception message for header names outside its hardcoded redaction
 * list (Authorization/Cookie/Proxy-Authorization/Set-Cookie; neither {@code access-token} nor
 * {@code x-openapi-access-token} is on it). So merely failing before the request is sent is not
 * enough: bypassing {@link QdmpContext} and calling {@link QdmpTransport} directly with an unsafe
 * token today still throws, but with the token leaked into the exception message. {@link
 * QdmpTransport} must therefore perform the same header-safety validation {@link QdmpContext#of}
 * does, with the same token-free exception message, rather than letting OkHttp's own validation
 * (and its token-echoing message) be reached at all.
 */
class QdmpTransportRawTokenValidationTest {

  private static final String SENTINEL = "SECRET-TOKEN-DO-NOT-LEAK";

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
  void get_withRawControlCharacterToken_failsBeforeAnyRequestIsSent_withoutLeakingToken() {
    QdmpTransport transport =
        new QdmpTransport(new OkHttpClient(), server.url("/").toString(), "1.0", "app-id");
    String unsafeToken = "raw\n" + SENTINEL;

    Throwable thrown =
        Assertions.catchThrowable(
            () ->
                transport.get(
                    "/user/v1/me",
                    AuthScheme.STANDARD,
                    unsafeToken,
                    null,
                    AuthToken200ResponseAllOfData.class));

    assertThat(thrown)
        .as("an unsafe raw token bypassing QdmpContext must still be rejected")
        .isInstanceOf(RuntimeException.class);
    assertThat(server.getRequestCount())
        .as("an unsafe raw token must be rejected locally before any HTTP request is attempted")
        .isEqualTo(0);
    assertThat(String.valueOf(thrown.getMessage()))
        .as(
            "the exception message must never leak the raw token -- OkHttp's own header"
                + " validation would otherwise echo it verbatim for a header name like"
                + " access-token that isn't on OkHttp's redaction list")
        .doesNotContain(SENTINEL);
  }

  /**
   * {@link QdmpTransport}'s public constructor only rejects a base URL that {@link
   * okhttp3.HttpUrl#parse} cannot parse at all -- it never checks the scheme. {@link
   * QdmpClientConfig.Builder#build()} enforces https:// (except for loopback/localhost) precisely
   * because every business/auth call carries the app secret and/or an access token as a request
   * header, but a caller that bypasses {@link QdmpClient}/{@link QdmpClientConfig} and constructs
   * {@link QdmpTransport} directly (it is public only because groups/auth live in separate
   * packages, not because direct use is supported/safe) can hand it a plain {@code http://} base
   * URL, and every subsequent {@code get}/{@code post} call would then send those credentials in
   * plaintext. {@link QdmpTransport}'s constructor must therefore enforce the same
   * https-or-loopback rule {@link QdmpClientConfig.Builder#build()} does, rather than relying
   * entirely on {@link QdmpClientConfig} being the only path that ever constructs it.
   */
  @Test
  void constructor_withHttpNonLoopbackBaseUrl_throwsIllegalArgumentException() {
    assertThatThrownBy(
            () -> new QdmpTransport(new OkHttpClient(), "http://evil.example.com", "1.0", "app-id"))
        .isInstanceOf(IllegalArgumentException.class);
  }
}
