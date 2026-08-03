package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import io.github.echotechfe.qdmp.generated.UserMe200ResponseAllOfData;

/** {@code user} group: {@code GET /user/v1/me} ("standard" scheme, mandatory user token). */
public final class UserGroup {

  private static final RouteMeta.Entry ME_ROUTE = RouteMeta.get("userMe");

  private final QdmpTransport transport;

  public UserGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Fetches the current authorized user's profile. Identity comes entirely from {@code ctx}'s
   * access token; there are no request parameters.
   *
   * @param ctx the calling context, carrying the required access token
   * @return the user's profile
   */
  public UserMe200ResponseAllOfData me(QdmpContext ctx) {
    return transport.get(
        ME_ROUTE.getPath(),
        AuthScheme.fromWireValue(ME_ROUTE.getAuthScheme()),
        ctx,
        null,
        UserMe200ResponseAllOfData.class);
  }
}
