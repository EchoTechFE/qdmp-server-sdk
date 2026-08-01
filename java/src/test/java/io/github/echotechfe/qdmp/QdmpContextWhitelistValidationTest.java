package io.github.echotechfe.qdmp;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.Test;

/**
 * {@code QdmpContext.of(accessToken)}'s current {@code requireHeaderSafe} check is a blacklist: it
 * only rejects control characters ({@code <= 0x1F}, {@code == 0x7F}) and anything above {@code
 * 0xFF}, which means it lets through both {@code 0x09} (tab) and the entire Latin-1 Supplement
 * range ({@code 0x80-0xFF}, e.g. {@code é}, U+00E9).
 *
 * <p>That matters because OkHttp 4.12's own {@code Headers} validation is stricter than this SDK's
 * check (it only accepts tab or {@code 0x20-0x7E}), and for header names outside its hardcoded
 * redaction list (Authorization/Cookie/Proxy-Authorization/Set-Cookie -- neither {@code
 * access-token} nor {@code x-openapi-access-token} is on it), OkHttp echoes the *entire* invalid
 * header value into its thrown {@link IllegalArgumentException} message. A token containing e.g.
 * {@code é} or a tab currently sails past {@link QdmpContext#of}, then gets rejected by OkHttp with
 * the full token value verbatim in the exception message -- a credential leak into logs/traces.
 *
 * <p>The required fix is a whitelist: only printable ASCII ({@code 0x20-0x7E}) may pass. This test
 * class documents that requirement directly against {@link QdmpContext#of}.
 */
class QdmpContextWhitelistValidationTest {

  @Test
  void of_withLatin1SupplementCharacter_throwsIllegalArgumentException_withoutLeakingToken() {
    String tokenWithLatin1Supplement = "prefixéSECRET";

    assertThatThrownBy(() -> QdmpContext.of(tokenWithLatin1Supplement))
        .isInstanceOf(IllegalArgumentException.class)
        .satisfies(
            e -> {
              assertThat(e.getMessage()).doesNotContain("SECRET");
              assertThat(e.getMessage()).doesNotContain("é");
            });
  }

  @Test
  void of_withEmbeddedTab_throwsIllegalArgumentException_withoutLeakingToken() {
    String tokenWithTab = "prefix\tSECRET";

    assertThatThrownBy(() -> QdmpContext.of(tokenWithTab))
        .isInstanceOf(IllegalArgumentException.class)
        .satisfies(e -> assertThat(e.getMessage()).doesNotContain("SECRET"));
  }

  @Test
  void of_withPlainAsciiToken_isAccepted() {
    QdmpContext ctx = QdmpContext.of("app-token-abc-123");

    assertThat(ctx.getAccessToken()).isEqualTo("app-token-abc-123");
  }
}
