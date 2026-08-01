package io.github.echotechfe.qdmp;

import io.github.echotechfe.qdmp.auth.InMemoryTokenStore;
import io.github.echotechfe.qdmp.auth.TokenStore;
import java.net.URI;
import java.net.URISyntaxException;
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

  // Package-private (not private) so QdmpTransport can apply this exact same loopback/localhost
  // check to a baseUrl passed directly to its own public constructor -- that constructor is
  // reachable without ever going through this builder (see the comment on QdmpTransport's
  // constructor), so it must not rely on this class being the only caller.
  //
  // Deliberately a strict dotted-decimal IPv4 literal check, not a String#startsWith("127.")
  // prefix match: a hostname like "127.attacker.com" starts with "127." but is not an IP literal
  // at all -- it is an ordinary domain name that DNS can resolve to any attacker-controlled
  // server, which would defeat the whole point of this loopback exception. It also deliberately
  // does NOT delegate to InetAddress.getByName(host).isLoopbackAddress(): that method performs a
  // real DNS resolution, which is both unwanted here (we only want to know whether the string
  // itself is an IPv4 loopback literal, not where a hostname happens to resolve today) and would
  // itself be a vector for the same class of bug.
  static boolean isLoopbackOrLocalhost(String host) {
    if (host == null) {
      return false;
    }
    if ("localhost".equalsIgnoreCase(host)) {
      return true;
    }
    return isIpv4LoopbackLiteral(host);
  }

  private static boolean isIpv4LoopbackLiteral(String host) {
    String[] octets = host.split("\\.", -1);
    if (octets.length != 4 || !"127".equals(octets[0])) {
      return false;
    }
    for (String octet : octets) {
      if (!isValidIpv4Octet(octet)) {
        return false;
      }
    }
    return true;
  }

  private static boolean isValidIpv4Octet(String octet) {
    if (octet.isEmpty() || octet.length() > 3) {
      return false;
    }
    for (int i = 0; i < octet.length(); i++) {
      if (!Character.isDigit(octet.charAt(i))) {
        return false;
      }
    }
    int value = Integer.parseInt(octet);
    return value >= 0 && value <= 255;
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
      requireHttpsOrLoopback(baseUrl);
      return new QdmpClientConfig(this);
    }

    // Every business/auth call carries the app secret and/or an access token as a request header,
    // so an http:// base URL (attacker-controlled or just misconfigured) would send those
    // credentials in plaintext over the network. loopback/localhost is exempted because
    // okhttp3.mockwebserver.MockWebServer, used throughout this SDK's own test suite, always
    // listens on http://127.0.0.1:PORT.
    private static void requireHttpsOrLoopback(String baseUrl) {
      URI uri;
      try {
        uri = new URI(baseUrl);
      } catch (URISyntaxException e) {
        throw new IllegalArgumentException("invalid baseUrl: " + baseUrl, e);
      }
      if (!"http".equalsIgnoreCase(uri.getScheme())) {
        return;
      }
      if (!isLoopbackOrLocalhost(uri.getHost())) {
        throw new IllegalArgumentException(
            "baseUrl must use https:// unless the host is loopback/localhost, got: " + baseUrl);
      }
    }
  }
}
