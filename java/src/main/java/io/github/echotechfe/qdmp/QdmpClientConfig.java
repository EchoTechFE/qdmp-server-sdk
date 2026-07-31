package io.github.echotechfe.qdmp;

import io.github.echotechfe.qdmp.auth.InMemoryTokenStore;
import io.github.echotechfe.qdmp.auth.TokenStore;
import java.time.Clock;
import java.util.Objects;

/**
 * Immutable configuration for a {@link QdmpClient}: app credentials, the target host, the {@code
 * x-echo-qdmp-version} header value, and the pluggable pieces ({@link TokenStore}, {@link Clock})
 * used by {@link io.github.echotechfe.qdmp.auth.QdmpAuth} to manage the app-level token.
 */
public final class QdmpClientConfig {

  private static final String DEFAULT_BASE_URL = "https://openapi.qiandao.com";
  private static final String DEFAULT_QDMP_VERSION = "1.0";

  private final String appId;
  private final String appSecret;
  private final String baseUrl;
  private final String qdmpVersion;
  private final TokenStore tokenStore;
  private final Clock clock;

  private QdmpClientConfig(Builder builder) {
    this.appId = builder.appId;
    this.appSecret = builder.appSecret;
    this.baseUrl = builder.baseUrl;
    this.qdmpVersion = builder.qdmpVersion;
    this.tokenStore = builder.tokenStore;
    this.clock = builder.clock;
  }

  /**
   * Creates a new configuration builder.
   *
   * @return a new {@link Builder}
   */
  public static Builder builder() {
    return new Builder();
  }

  public String getAppId() {
    return appId;
  }

  public String getAppSecret() {
    return appSecret;
  }

  public String getBaseUrl() {
    return baseUrl;
  }

  public String getQdmpVersion() {
    return qdmpVersion;
  }

  public TokenStore getTokenStore() {
    return tokenStore;
  }

  public Clock getClock() {
    return clock;
  }

  /** Builder for {@link QdmpClientConfig}. */
  public static final class Builder {

    private String appId;
    private String appSecret;
    private String baseUrl = DEFAULT_BASE_URL;
    private String qdmpVersion = DEFAULT_QDMP_VERSION;
    private TokenStore tokenStore = new InMemoryTokenStore();
    private Clock clock = Clock.systemUTC();

    private Builder() {}

    public Builder appId(String appId) {
      this.appId = appId;
      return this;
    }

    public Builder appSecret(String appSecret) {
      this.appSecret = appSecret;
      return this;
    }

    public Builder baseUrl(String baseUrl) {
      this.baseUrl = baseUrl;
      return this;
    }

    public Builder qdmpVersion(String qdmpVersion) {
      this.qdmpVersion = qdmpVersion;
      return this;
    }

    public Builder tokenStore(TokenStore tokenStore) {
      this.tokenStore = tokenStore;
      return this;
    }

    public Builder clock(Clock clock) {
      this.clock = clock;
      return this;
    }

    /**
     * Validates required fields and builds the configuration.
     *
     * @return a new {@link QdmpClientConfig}
     */
    public QdmpClientConfig build() {
      Objects.requireNonNull(appId, "appId must not be null");
      Objects.requireNonNull(appSecret, "appSecret must not be null");
      Objects.requireNonNull(baseUrl, "baseUrl must not be null");
      Objects.requireNonNull(qdmpVersion, "qdmpVersion must not be null");
      Objects.requireNonNull(tokenStore, "tokenStore must not be null");
      Objects.requireNonNull(clock, "clock must not be null");
      return new QdmpClientConfig(this);
    }
  }
}
