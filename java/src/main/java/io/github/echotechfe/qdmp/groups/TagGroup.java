package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import io.github.echotechfe.qdmp.generated.TagDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.TagSearch200ResponseAllOfData;
import java.util.Map;

/** {@code tag} group: {@code GET /tag/v1/detail} and {@code GET /tag/v1/search}. */
public final class TagGroup {

  private static final RouteMeta.Entry DETAIL_ROUTE = RouteMeta.get("tagDetail");
  private static final RouteMeta.Entry SEARCH_ROUTE = RouteMeta.get("tagSearch");

  private final QdmpTransport transport;

  public TagGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Fetches a single tag's details.
   *
   * @param ctx the calling context, carrying the required access token
   * @param id the tag ID
   * @return the tag details
   */
  public TagDetail200ResponseAllOfData detail(QdmpContext ctx, String id) {
    return transport.get(
        DETAIL_ROUTE.getPath(),
        AuthScheme.fromWireValue(DETAIL_ROUTE.getAuthScheme()),
        ctx,
        Map.of("id", id),
        TagDetail200ResponseAllOfData.class);
  }

  /**
   * Searches tags by keyword/category filters.
   *
   * @param ctx the calling context, carrying the required access token
   * @param params the search filters
   * @return the matching tags and total count
   */
  public TagSearch200ResponseAllOfData search(QdmpContext ctx, TagSearchParams params) {
    return transport.get(
        SEARCH_ROUTE.getPath(),
        AuthScheme.fromWireValue(SEARCH_ROUTE.getAuthScheme()),
        ctx,
        params.toQueryMap(),
        TagSearch200ResponseAllOfData.class);
  }
}
