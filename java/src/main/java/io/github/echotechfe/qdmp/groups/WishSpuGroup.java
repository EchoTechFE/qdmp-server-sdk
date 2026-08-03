package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import io.github.echotechfe.qdmp.generated.WishAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.WishAddRequest;
import io.github.echotechfe.qdmp.generated.WishCancelRequest;
import io.github.echotechfe.qdmp.generated.WishList200ResponseAllOfData;

/**
 * {@code wishspu} group: {@code POST /wishspu/v1/add}, {@code POST /wishspu/v1/cancel}, {@code GET
 * /wishspu/v1/list}. "standard" scheme, mandatory user token.
 */
public final class WishSpuGroup {

  private static final RouteMeta.Entry ADD_ROUTE = RouteMeta.get("wishAdd");
  private static final RouteMeta.Entry CANCEL_ROUTE = RouteMeta.get("wishCancel");
  private static final RouteMeta.Entry LIST_ROUTE = RouteMeta.get("wishList");

  private final QdmpTransport transport;

  public WishSpuGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Batch-adds SPUs to the current user's wish list.
   *
   * @param ctx the calling context, carrying the required access token
   * @param request the target IDs and type
   * @return the number of successful additions
   */
  public WishAdd200ResponseAllOfData add(QdmpContext ctx, WishAddRequest request) {
    return transport.post(
        ADD_ROUTE.getPath(),
        AuthScheme.fromWireValue(ADD_ROUTE.getAuthScheme()),
        ctx,
        request,
        WishAdd200ResponseAllOfData.class);
  }

  /**
   * Batch-removes SPUs from the current user's wish list.
   *
   * @param ctx the calling context, carrying the required access token
   * @param request the target IDs and type
   * @return the number of successful removals
   */
  public WishAdd200ResponseAllOfData cancel(QdmpContext ctx, WishCancelRequest request) {
    return transport.post(
        CANCEL_ROUTE.getPath(),
        AuthScheme.fromWireValue(CANCEL_ROUTE.getAuthScheme()),
        ctx,
        request,
        WishAdd200ResponseAllOfData.class);
  }

  /**
   * Lists the current user's wish list, paginated.
   *
   * @param ctx the calling context, carrying the required access token
   * @param params the pagination/filter parameters
   * @return the wish list and total count
   */
  public WishList200ResponseAllOfData list(QdmpContext ctx, WishListParams params) {
    return transport.get(
        LIST_ROUTE.getPath(),
        AuthScheme.fromWireValue(LIST_ROUTE.getAuthScheme()),
        ctx,
        params.toQueryMap(),
        WishList200ResponseAllOfData.class);
  }
}
