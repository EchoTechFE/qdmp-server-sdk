package io.github.echotechfe.qdmp;

import io.github.echotechfe.qdmp.auth.QdmpAuth;
import io.github.echotechfe.qdmp.groups.GenaiGroup;
import io.github.echotechfe.qdmp.groups.IslandGroup;
import io.github.echotechfe.qdmp.groups.MarkGroup;
import io.github.echotechfe.qdmp.groups.SpuGroup;
import io.github.echotechfe.qdmp.groups.TagGroup;
import io.github.echotechfe.qdmp.groups.UserGroup;
import io.github.echotechfe.qdmp.groups.WishSpuGroup;
import okhttp3.OkHttpClient;

/**
 * Entry point for the qdmp Server SDK. Exposes {@link #auth()} for app-level/user-level token
 * management and one accessor per business group (e.g. {@link #mark()}), each of which routes
 * through a single shared {@link QdmpTransport} built from the given {@link QdmpClientConfig}.
 */
public final class QdmpClient {

  private final QdmpAuth auth;
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
    OkHttpClient httpClient = new OkHttpClient();
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
