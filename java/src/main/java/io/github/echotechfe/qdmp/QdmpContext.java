package io.github.echotechfe.qdmp;

import java.util.Objects;

/**
 * Carries the per-call access token required by every user-scoped business method (e.g. {@code
 * client.mark().add(ctx, request)}). {@link #of} is the single gate that guarantees a missing/blank
 * token fails locally, before any HTTP request is attempted.
 */
public final class QdmpContext {

  private final String accessToken;

  private QdmpContext(String accessToken) {
    this.accessToken = accessToken;
  }

  /**
   * Creates a context wrapping the given access token. The SDK does not distinguish here whether
   * the token is app-level ({@code CLIENT_CREDENTIALS}) or user-level ({@code AUTHORIZATION_CODE})
   * -- that choice belongs to the caller.
   *
   * @param accessToken the token to send with the call; must not be {@code null} or blank, and must
   *     contain only characters that are safe to send verbatim as an HTTP header value
   * @return a new context wrapping {@code accessToken}
   * @throws NullPointerException if {@code accessToken} is {@code null}
   * @throws IllegalArgumentException if {@code accessToken} is empty, all whitespace, or contains a
   *     character that cannot be safely sent as an HTTP header value
   */
  public static QdmpContext of(String accessToken) {
    Objects.requireNonNull(accessToken, "accessToken must not be null");
    if (accessToken.isBlank()) {
      throw new IllegalArgumentException("accessToken must not be blank");
    }
    requireHeaderSafe(accessToken);
    return new QdmpContext(accessToken);
  }

  // Rejects control characters and anything outside ISO-8859-1 before the token ever reaches
  // QdmpTransport's OkHttp Request.Builder#header calls. Without this gate, a token with e.g. an
  // embedded newline or a non-Latin1 character would be rejected by OkHttp's own header
  // validation -- but OkHttp only redacts Authorization/Cookie/Proxy-Authorization/Set-Cookie in
  // that exception's message, so "access-token"/"x-openapi-access-token" would otherwise have the
  // full (invalid) token value echoed straight into the thrown IllegalArgumentException. The
  // message below intentionally never includes the token itself.
  private static void requireHeaderSafe(String accessToken) {
    for (int i = 0; i < accessToken.length(); i++) {
      char c = accessToken.charAt(i);
      boolean isControlCharacter = c <= 0x1f || c == 0x7f;
      boolean isOutsideLatin1 = c > 0xff;
      if (isControlCharacter || isOutsideLatin1) {
        throw new IllegalArgumentException(
            "accessToken contains a character that cannot be safely sent as an HTTP header value"
                + " (this message intentionally omits the token itself)");
      }
    }
  }

  public String getAccessToken() {
    return accessToken;
  }
}
