package io.github.echotechfe.qdmp.auth;

import io.github.echotechfe.qdmp.errors.QdmpValidationError;
import java.util.OptionalLong;
import java.util.function.Consumer;

/**
 * The user-authorization credential handed to {@code QdmpClient.withUserCredential(...)}, together
 * with what the SDK needs in order to keep it alive: the refresh token to renew it with, optionally
 * the expiry it should renew ahead of, and optionally a callback to persist each renewal.
 *
 * <p>{@link Builder#build()} is the single gate: a missing/empty access or refresh token fails
 * right there with a {@link QdmpValidationError}, before any client is created and long before any
 * HTTP request could be attempted. The refresh token is mandatory precisely because without it
 * there is nothing to renew with -- a caller that only has a static token should use {@code
 * QdmpContext.of(token)} against the root client instead.
 *
 * <p>{@link #toString()} omits both token values, so logging the options cannot leak a live
 * credential.
 */
public final class UserCredentialOptions {

  private final String accessToken;
  private final String refreshToken;
  private final Long expiresAtEpochSeconds;
  private final Consumer<RefreshedUserToken> onRefresh;

  private UserCredentialOptions(Builder builder) {
    this.accessToken = builder.accessToken;
    this.refreshToken = builder.refreshToken;
    this.expiresAtEpochSeconds = builder.expiresAtEpochSeconds;
    this.onRefresh = builder.onRefresh;
  }

  /**
   * Starts building a set of options.
   *
   * @return a new builder
   */
  public static Builder builder() {
    return new Builder();
  }

  /**
   * Returns the user access token to start from.
   *
   * @return the access token
   */
  public String getAccessToken() {
    return accessToken;
  }

  /**
   * Returns the refresh token used to renew the access token.
   *
   * @return the refresh token
   */
  public String getRefreshToken() {
    return refreshToken;
  }

  /**
   * Returns the absolute expiry of the supplied access token, as Unix seconds.
   *
   * @return the expiry in epoch seconds, or {@link OptionalLong#empty()} if the caller did not
   *     supply one -- in which case there is no proactive renewal and the credential is only
   *     renewed reactively, after an HTTP 401
   */
  public OptionalLong getExpiresAtEpochSeconds() {
    return expiresAtEpochSeconds == null
        ? OptionalLong.empty()
        : OptionalLong.of(expiresAtEpochSeconds);
  }

  /**
   * Returns the callback invoked after every successful renewal.
   *
   * @return the callback, or {@code null} if none was supplied
   */
  public Consumer<RefreshedUserToken> getOnRefresh() {
    return onRefresh;
  }

  @Override
  public String toString() {
    return "UserCredentialOptions{accessToken=[REDACTED], refreshToken=[REDACTED],"
        + " expiresAtEpochSeconds="
        + expiresAtEpochSeconds
        + ", onRefresh="
        + (onRefresh == null ? "none" : "set")
        + "}";
  }

  /** Builder for {@link UserCredentialOptions}. */
  public static final class Builder {

    private String accessToken;
    private String refreshToken;
    private Long expiresAtEpochSeconds;
    private Consumer<RefreshedUserToken> onRefresh;

    private Builder() {}

    /**
     * Sets the user access token to start from.
     *
     * @param accessToken the access token; required and must be non-empty
     * @return this builder
     */
    public Builder accessToken(String accessToken) {
      this.accessToken = accessToken;
      return this;
    }

    /**
     * Sets the refresh token the SDK renews the access token with.
     *
     * @param refreshToken the refresh token; required and must be non-empty
     * @return this builder
     */
    public Builder refreshToken(String refreshToken) {
      this.refreshToken = refreshToken;
      return this;
    }

    /**
     * Sets the absolute expiry of the supplied access token.
     *
     * @param expiresAtEpochSeconds the expiry as Unix seconds (not milliseconds, and not a relative
     *     "seconds remaining" duration); leave unset if it is unknown
     * @return this builder
     */
    public Builder expiresAt(long expiresAtEpochSeconds) {
      this.expiresAtEpochSeconds = expiresAtEpochSeconds;
      return this;
    }

    /**
     * Sets the callback invoked after every successful renewal, so the caller can persist the new
     * access token and expiry.
     *
     * @param onRefresh the callback; a {@link RuntimeException} it throws propagates to the caller
     *     of the business method that triggered the renewal
     * @return this builder
     */
    public Builder onRefresh(Consumer<RefreshedUserToken> onRefresh) {
      this.onRefresh = onRefresh;
      return this;
    }

    /**
     * Validates and builds the options.
     *
     * @return the built options
     * @throws QdmpValidationError if the access token or the refresh token is {@code null} or empty
     */
    public UserCredentialOptions build() {
      if (accessToken == null || accessToken.isEmpty()) {
        throw new QdmpValidationError(
            "withUserCredential: \"accessToken\" is required and must be a non-empty string");
      }
      if (refreshToken == null || refreshToken.isEmpty()) {
        throw new QdmpValidationError(
            "withUserCredential: \"refreshToken\" is required and must be a non-empty string --"
                + " without it the SDK cannot renew the credential");
      }
      return new UserCredentialOptions(this);
    }
  }
}
