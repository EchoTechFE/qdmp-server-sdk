package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.GenaiDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerate200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequest;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.util.Map;

/**
 * {@code genai} group: {@code POST /genai/v1/generate} and {@code GET /genai/v1/detail}. The one
 * "genai" scheme group -- uses {@code x-openapi-access-token}/{@code x-openapi-app-id} instead of
 * the "standard" scheme's {@code access-token}/{@code x-echo-qdmp-version}.
 */
public final class GenaiGroup {

  private static final RouteMeta.Entry GENERATE_ROUTE = RouteMeta.get("genaiGenerate");
  private static final RouteMeta.Entry DETAIL_ROUTE = RouteMeta.get("genaiDetail");

  private final QdmpTransport transport;

  public GenaiGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  /**
   * Creates an asynchronous GenAI generation task, dispatched through the upstream AI gateway.
   *
   * @param ctx the calling context, carrying the required GenAI access token
   * @param request the model, conversation contents, and optional generation settings
   * @return the created task's ID
   */
  public GenaiGenerate200ResponseAllOfData generate(QdmpContext ctx, GenaiGenerateRequest request) {
    return transport.post(
        GENERATE_ROUTE.getPath(),
        AuthScheme.fromWireValue(GENERATE_ROUTE.getAuthScheme()),
        ctx.getAccessToken(),
        request,
        GenaiGenerate200ResponseAllOfData.class);
  }

  /**
   * Queries the status/result of an asynchronous GenAI generation task.
   *
   * @param ctx the calling context, carrying the required GenAI access token
   * @param id the task ID returned by {@link #generate}
   * @return the task's current status and, once completed, its result
   */
  public GenaiDetail200ResponseAllOfData detail(QdmpContext ctx, String id) {
    return transport.get(
        DETAIL_ROUTE.getPath(),
        AuthScheme.fromWireValue(DETAIL_ROUTE.getAuthScheme()),
        ctx.getAccessToken(),
        Map.of("id", id),
        GenaiDetail200ResponseAllOfData.class);
  }
}
