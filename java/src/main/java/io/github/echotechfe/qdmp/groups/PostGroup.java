package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.util.LinkedHashMap;
import java.util.Map;

/** Post queries return extensible maps because the public documentation does not define data. */
public final class PostGroup {
  private final QdmpTransport transport;

  public PostGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  public Map<String, Object> detail(QdmpContext ctx, String postId) {
    return get("postDetail", ctx, query("postId", postId));
  }

  public Map<String, Object> list(QdmpContext ctx, String offset, String limit) {
    return get("postList", ctx, query("offset", offset, "limit", limit));
  }

  public Map<String, Object> myList(QdmpContext ctx, String offset, String limit) {
    return get("postMyList", ctx, query("offset", offset, "limit", limit));
  }

  private Map<String, Object> get(String operationId, QdmpContext ctx, Map<String, Object> query) {
    RouteMeta.Entry route = RouteMeta.get(operationId);
    @SuppressWarnings("unchecked")
    Map<String, Object> result =
        transport.get(
            route.getPath(),
            AuthScheme.fromWireValue(route.getAuthScheme()),
            ctx,
            query,
            Map.class);
    return result;
  }

  private static Map<String, Object> query(String... entries) {
    Map<String, Object> result = new LinkedHashMap<>();
    for (int i = 0; i < entries.length; i += 2) {
      if (entries[i + 1] != null) {
        result.put(entries[i], entries[i + 1]);
      }
    }
    return result;
  }
}
