package io.github.echotechfe.qdmp;

/**
 * The header pair a business endpoint expects, taken from that operation's {@code
 * x-qdmp-auth-scheme} extension in {@code shared/openapi.yaml} (mirrored in {@code
 * shared/generated/route-meta.json}).
 */
public enum AuthScheme {
  /**
   * {@code access-token} + {@code x-echo-qdmp-version}; used by user/island/spu/tag/mark/wishspu.
   */
  STANDARD,

  /** {@code x-openapi-access-token} + {@code x-openapi-app-id}; used only by the genai group. */
  GENAI,

  /** No token headers; used only by the two {@code auth.*} endpoints. */
  NO_AUTH;

  /**
   * Maps the wire-level {@code authScheme} string used by {@code shared/generated/route-meta.json}
   * (and, ultimately, each operation's {@code x-qdmp-auth-scheme} extension in {@code
   * shared/openapi.yaml}) to its enum value. Business groups use this instead of hardcoding an
   * {@link AuthScheme} literal alongside a hardcoded path, so a spec change to an operation's auth
   * scheme surfaces here rather than silently drifting from the route metadata table.
   *
   * @param wireValue one of {@code "standard"}, {@code "genai"}, {@code "noAuth"}
   * @return the corresponding {@link AuthScheme}
   * @throws IllegalArgumentException if {@code wireValue} is none of the above
   */
  public static AuthScheme fromWireValue(String wireValue) {
    switch (wireValue) {
      case "standard":
        return STANDARD;
      case "genai":
        return GENAI;
      case "noAuth":
        return NO_AUTH;
      default:
        throw new IllegalArgumentException("unknown qdmp auth scheme: " + wireValue);
    }
  }
}
