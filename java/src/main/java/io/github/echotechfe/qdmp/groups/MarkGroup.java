package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.MarkAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkAddRequest;
import io.github.echotechfe.qdmp.generated.MarkDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkList200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.util.Map;
import java.util.Objects;

/**
 * {@code marks} group: {@code POST /mark/v1/add}, {@code GET /mark/v1/me/list}, {@code GET
 * /mark/v1/me/search}, {@code GET /mark/v1/me/detail}. "standard" scheme, mandatory user token.
 */
public final class MarkGroup {

  private static final RouteMeta.Entry ADD_ROUTE = RouteMeta.get("markAdd");
  private static final RouteMeta.Entry LIST_ROUTE = RouteMeta.get("markList");
  private static final RouteMeta.Entry SEARCH_ROUTE = RouteMeta.get("markSearch");
  private static final RouteMeta.Entry DETAIL_ROUTE = RouteMeta.get("markDetail");

  private final QdmpTransport transport;

  public MarkGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Adds a mark (optionally with a rating) for a SPU.
   *
   * @param ctx the calling context, carrying the required access token
   * @param request the SPU to mark and optional rating
   * @return the created mark's ID
   */
  public MarkAdd200ResponseAllOfData add(QdmpContext ctx, MarkAddRequest request) {
    return transport.post(
        ADD_ROUTE.getPath(),
        AuthScheme.fromWireValue(ADD_ROUTE.getAuthScheme()),
        ctx,
        request,
        MarkAdd200ResponseAllOfData.class);
  }

  /**
   * Lists the current user's marks, paginated.
   *
   * @param ctx the calling context, carrying the required access token
   * @param limit page size (max 100)
   * @param offset page offset (max 100)
   * @return the mark list and total count
   */
  public MarkList200ResponseAllOfData list(QdmpContext ctx, String limit, String offset) {
    Objects.requireNonNull(limit, "limit must not be null");
    Objects.requireNonNull(offset, "offset must not be null");
    return transport.get(
        LIST_ROUTE.getPath(),
        AuthScheme.fromWireValue(LIST_ROUTE.getAuthScheme()),
        ctx,
        Map.of("limit", limit, "offset", offset),
        MarkList200ResponseAllOfData.class);
  }

  /**
   * Searches the current user's marks by category, paginated.
   *
   * @param ctx the calling context, carrying the required access token
   * @param params the search filters
   * @return the mark list and total count
   */
  public MarkSearch200ResponseAllOfData search(QdmpContext ctx, MarkSearchParams params) {
    return transport.get(
        SEARCH_ROUTE.getPath(),
        AuthScheme.fromWireValue(SEARCH_ROUTE.getAuthScheme()),
        ctx,
        params.toQueryMap(),
        MarkSearch200ResponseAllOfData.class);
  }

  /**
   * Fetches a single mark's details, including its paginated history.
   *
   * @param ctx the calling context, carrying the required access token
   * @param id the mark spu info ID
   * @param limit history page size (max 100)
   * @param offset history page offset (max 100)
   * @return the mark details
   */
  public MarkDetail200ResponseAllOfData detail(
      QdmpContext ctx, String id, String limit, String offset) {
    Objects.requireNonNull(id, "id must not be null");
    Objects.requireNonNull(limit, "limit must not be null");
    Objects.requireNonNull(offset, "offset must not be null");
    return transport.get(
        DETAIL_ROUTE.getPath(),
        AuthScheme.fromWireValue(DETAIL_ROUTE.getAuthScheme()),
        ctx,
        Map.of("id", id, "limit", limit, "offset", offset),
        MarkDetail200ResponseAllOfData.class);
  }
}
