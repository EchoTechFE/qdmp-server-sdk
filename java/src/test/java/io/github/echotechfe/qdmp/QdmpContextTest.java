package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.Test;

/**
 * {@code QdmpContext.of(accessToken)} is the single required-token gate for every user-scoped call
 * ({@code client.mark().add(ctx, ...)}, {@code client.user().me(ctx)}, ...). A missing/blank token
 * must fail locally, before any HTTP request is attempted.
 */
class QdmpContextTest {

  @Test
  void of_withNonBlankToken_exposesItVerbatim() {
    QdmpContext ctx = QdmpContext.of("user-access-token-123");

    assertThat(ctx.getAccessToken()).isEqualTo("user-access-token-123");
  }

  @Test
  void of_withNullToken_throwsNullPointerException() {
    assertThatThrownBy(() -> QdmpContext.of(null)).isInstanceOf(NullPointerException.class);
  }

  @Test
  void of_withEmptyToken_throwsIllegalArgumentException() {
    assertThatThrownBy(() -> QdmpContext.of("")).isInstanceOf(IllegalArgumentException.class);
  }

  @Test
  void of_withBlankToken_throwsIllegalArgumentException() {
    assertThatThrownBy(() -> QdmpContext.of("   ")).isInstanceOf(IllegalArgumentException.class);
  }

  /**
   * A token containing a raw newline would otherwise reach OkHttp's own header-value validation,
   * which echoes the full (invalid) header value -- including the token -- into its exception
   * message for any header other than Authorization/Cookie/Proxy-Authorization/Set-Cookie (neither
   * "access-token" nor "x-openapi-access-token" is on that redaction list). QdmpContext.of must
   * reject this locally first, and the resulting message must never contain the token.
   */
  @Test
  void of_withEmbeddedNewline_throwsIllegalArgumentException_withoutLeakingToken() {
    String tokenWithNewline = "user-access-token\nX-Injected-Header: evil";

    assertThatThrownBy(() -> QdmpContext.of(tokenWithNewline))
        .isInstanceOf(IllegalArgumentException.class)
        .satisfies(e -> assertThat(e.getMessage()).doesNotContain(tokenWithNewline));
  }

  @Test
  void of_withCarriageReturn_throwsIllegalArgumentException() {
    assertThatThrownBy(() -> QdmpContext.of("token\rwith-cr"))
        .isInstanceOf(IllegalArgumentException.class);
  }

  @Test
  void of_withNonLatin1Character_throwsIllegalArgumentException() {
    assertThatThrownBy(() -> QdmpContext.of("token-中文"))
        .isInstanceOf(IllegalArgumentException.class);
  }
}
