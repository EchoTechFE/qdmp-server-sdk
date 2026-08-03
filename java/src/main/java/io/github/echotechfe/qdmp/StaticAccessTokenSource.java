package io.github.echotechfe.qdmp;

/**
 * The {@link AccessTokenSource} behind a {@link QdmpContext#of(String)} context: one fixed token,
 * validated once at construction, that the SDK can neither renew nor replace. An HTTP 401 on this
 * path is therefore surfaced to the caller as-is -- there is nothing to retry with.
 */
final class StaticAccessTokenSource implements AccessTokenSource {

  private final String accessToken;

  StaticAccessTokenSource(String accessToken) {
    this.accessToken = accessToken;
  }

  @Override
  public String tokenForRequest() {
    return accessToken;
  }

  @Override
  public String currentAccessToken() {
    return accessToken;
  }

  @Override
  public String renewAfterUnauthorized(String attemptedAccessToken) {
    return null;
  }
}
