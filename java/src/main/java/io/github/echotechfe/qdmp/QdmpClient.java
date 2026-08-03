package io.github.echotechfe.qdmp;

import io.github.echotechfe.qdmp.auth.QdmpAuth;
import io.github.echotechfe.qdmp.auth.UserCredentialOptions;
import io.github.echotechfe.qdmp.auth.UserCredentialSession;
import io.github.echotechfe.qdmp.groups.GenaiGroup;
import io.github.echotechfe.qdmp.groups.IslandGroup;
import io.github.echotechfe.qdmp.groups.MarkGroup;
import io.github.echotechfe.qdmp.groups.SpuGroup;
import io.github.echotechfe.qdmp.groups.TagGroup;
import io.github.echotechfe.qdmp.groups.UserGroup;
import io.github.echotechfe.qdmp.groups.WishSpuGroup;
import java.time.Clock;
import okhttp3.OkHttpClient;

/**
 * Entry point for the qdmp Server SDK. Exposes {@link #auth()} for app/user credential management,
 * {@link #withUserCredential} for a client that keeps one user-authorization credential alive on
 * its own, and one accessor per business group (e.g. {@link #mark()}), each of which routes through
 * a single shared {@link QdmpTransport} built from the given {@link QdmpClientConfig}.
 */
public final class QdmpClient {

  private final QdmpAuth auth;
  private final Clock clock;
  private final UserGroup user;
  private final IslandGroup island;
  private final SpuGroup spu;
  private final TagGroup tag;
  private final MarkGroup mark;
  private final WishSpuGroup wishspu;
  private final GenaiGroup genai;

  /**
   * Creates a new client from the given configuration, wiring a single shared HTTP transport into
   * the auth module and every business group.
   *
   * @param config the client configuration
   */
  public QdmpClient(QdmpClientConfig config) {
    OkHttpClient httpClient =
        new OkHttpClient.Builder().followRedirects(false).followSslRedirects(false).build();
    QdmpTransport transport =
        new QdmpTransport(
            httpClient, config.getBaseUrl(), config.getQdmpVersion(), config.getAppId());
    this.auth =
        new QdmpAuth(
            transport,
            config.getAppId(),
            config.getAppSecret(),
            config.getTokenStore(),
            config.getClock());
    this.clock = config.getClock();
    this.user = new UserGroup(transport);
    this.island = new IslandGroup(transport);
    this.spu = new SpuGroup(transport);
    this.tag = new TagGroup(transport);
    this.mark = new MarkGroup(transport);
    this.wishspu = new WishSpuGroup(transport);
    this.genai = new GenaiGroup(transport);
  }

  public QdmpAuth auth() {
    return auth;
  }

  /**
   * Binds an already-obtained user-authorization credential to a client that keeps it alive: its
   * business methods take no {@link QdmpContext}, and the SDK renews the credential ahead of a
   * known expiry and once more after an HTTP 401 that reports the token invalid/expired, retrying
   * the failed call.
   *
   * <p>The returned client shares this client's transport and groups; this client's own {@code
   * user().me(ctx)}-style API is unaffected.
   *
   * @param options the credential (access token plus the refresh token to renew it with), its
   *     optional expiry, and an optional callback to persist each renewal
   * @return a client bound to that credential
   */
  public QdmpUserClient withUserCredential(UserCredentialOptions options) {
    return new QdmpUserClient(this, new UserCredentialSession(auth, clock, options));
  }

  public UserGroup user() {
    return user;
  }

  public IslandGroup island() {
    return island;
  }

  public SpuGroup spu() {
    return spu;
  }

  public TagGroup tag() {
    return tag;
  }

  public MarkGroup mark() {
    return mark;
  }

  public WishSpuGroup wishspu() {
    return wishspu;
  }

  public GenaiGroup genai() {
    return genai;
  }
}
