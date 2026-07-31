package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.IslandDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.util.Map;

/**
 * {@code island} group: {@code GET /island/v1/detail} ("standard" scheme, mandatory user token).
 */
public final class IslandGroup {

  private static final RouteMeta.Entry DETAIL_ROUTE = RouteMeta.get("islandDetail");

  private final QdmpTransport transport;

  public IslandGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Fetches an island's details. Confirmed callable with an app-level ({@code CLIENT_CREDENTIALS})
   * token in real testing, unlike most other "standard" endpoints -- see {@code
   * shared/openapi.yaml}.
   *
   * @param ctx the calling context, carrying the required access token
   * @param id the island ID
   * @return the island details
   */
  public IslandDetail200ResponseAllOfData detail(QdmpContext ctx, String id) {
    return transport.get(
        DETAIL_ROUTE.getPath(),
        AuthScheme.fromWireValue(DETAIL_ROUTE.getAuthScheme()),
        ctx.getAccessToken(),
        Map.of("id", id),
        IslandDetail200ResponseAllOfData.class);
  }
}
