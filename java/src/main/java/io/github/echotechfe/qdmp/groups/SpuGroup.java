package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import io.github.echotechfe.qdmp.generated.SpuDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.SpuSearch200ResponseAllOfData;
import java.util.Map;

/** {@code spu} group: {@code GET /spu/v1/detail} and {@code GET /spu/v1/search}. */
public final class SpuGroup {

  private static final RouteMeta.Entry DETAIL_ROUTE = RouteMeta.get("spuDetail");
  private static final RouteMeta.Entry SEARCH_ROUTE = RouteMeta.get("spuSearch");

  private final QdmpTransport transport;

  public SpuGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Fetches a single SPU's details.
   *
   * @param ctx the calling context, carrying the required access token
   * @param id the SPU ID
   * @return the SPU details
   */
  public SpuDetail200ResponseAllOfData detail(QdmpContext ctx, String id) {
    return transport.get(
        DETAIL_ROUTE.getPath(),
        AuthScheme.fromWireValue(DETAIL_ROUTE.getAuthScheme()),
        ctx.getAccessToken(),
        Map.of("id", id),
        SpuDetail200ResponseAllOfData.class);
  }

  /**
   * Searches SPUs by keyword/category/IP-tag filters. Confirmed callable with an app-level ({@code
   * CLIENT_CREDENTIALS}) token in real testing -- see {@code shared/openapi.yaml}.
   *
   * @param ctx the calling context, carrying the required access token
   * @param params the search filters
   * @return the matching SPUs and total count
   */
  public SpuSearch200ResponseAllOfData search(QdmpContext ctx, SpuSearchParams params) {
    return transport.get(
        SEARCH_ROUTE.getPath(),
        AuthScheme.fromWireValue(SEARCH_ROUTE.getAuthScheme()),
        ctx.getAccessToken(),
        params.toQueryMap(),
        SpuSearch200ResponseAllOfData.class);
  }
}
