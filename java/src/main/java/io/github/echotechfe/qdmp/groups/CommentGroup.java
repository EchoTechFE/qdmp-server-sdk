package io.github.echotechfe.qdmp.groups;

import io.github.echotechfe.qdmp.AuthScheme;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.QdmpTransport;
import io.github.echotechfe.qdmp.generated.CommentCreateRequest;
import io.github.echotechfe.qdmp.generated.CommentCreateResponseAllOfData;
import io.github.echotechfe.qdmp.generated.CommentLikeRequest;
import io.github.echotechfe.qdmp.generated.CommentRepliesResponseAllOfData;
import io.github.echotechfe.qdmp.generated.CommentReplyRequest;
import io.github.echotechfe.qdmp.generated.PostCommentsResponseAllOfData;
import io.github.echotechfe.qdmp.generated.RouteMeta;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/** Comment operations and pagination. */
public final class CommentGroup {
  private final QdmpTransport transport;

  public CommentGroup(QdmpTransport transport) {
    this.transport = transport;
  }

  public CommentCreateResponseAllOfData create(QdmpContext ctx, CommentCreateRequest request) {
    return post("commentCreate", ctx, null, request, CommentCreateResponseAllOfData.class);
  }

  public CommentCreateResponseAllOfData reply(
      QdmpContext ctx, String commentId, CommentReplyRequest request) {
    return post("commentReply", ctx, commentId, request, CommentCreateResponseAllOfData.class);
  }

  /** Likes or unlikes a comment. */
  public void like(QdmpContext ctx, String commentId, CommentLikeRequest request) {
    post("commentLike", ctx, commentId, request, Void.class);
  }

  /** Lists comments on a post. */
  public PostCommentsResponseAllOfData postComments(
      QdmpContext ctx, String postId, String limit, String offset) {
    return get(
        "postComments",
        ctx,
        "postId",
        postId,
        query("limit", limit, "offset", offset),
        PostCommentsResponseAllOfData.class);
  }

  /** Lists replies to a comment. */
  public CommentRepliesResponseAllOfData replies(
      QdmpContext ctx, String commentId, String limit, String cursor) {
    return get(
        "commentReplies",
        ctx,
        "commentId",
        commentId,
        query("limit", limit, "cursor", cursor),
        CommentRepliesResponseAllOfData.class);
  }

  private <T> T post(
      String operationId, QdmpContext ctx, String commentId, Object body, Class<T> type) {
    RouteMeta.Entry route = RouteMeta.get(operationId);
    return transport.post(
        path(route, "commentId", commentId),
        AuthScheme.fromWireValue(route.getAuthScheme()),
        ctx,
        body,
        type);
  }

  private <T> T get(
      String operationId,
      QdmpContext ctx,
      String parameter,
      String id,
      Map<String, Object> query,
      Class<T> type) {
    RouteMeta.Entry route = RouteMeta.get(operationId);
    return transport.get(
        path(route, parameter, id),
        AuthScheme.fromWireValue(route.getAuthScheme()),
        ctx,
        query,
        type);
  }

  private static String path(RouteMeta.Entry route, String parameter, String value) {
    String placeholder = "{" + parameter + "}";
    if (!route.getPath().contains(placeholder)) {
      return route.getPath();
    }
    Objects.requireNonNull(value, parameter + " must not be null");
    validatePositiveId(parameter, value);
    return route.getPath().replace(placeholder, value);
  }

  private static void validatePositiveId(String name, String value) {
    if (!value.matches("^[1-9][0-9]*$")) {
      throw new IllegalArgumentException(name + " must be a positive integer ID");
    }
  }

  private static Map<String, Object> query(Object... entries) {
    Map<String, Object> result = new LinkedHashMap<>();
    for (int i = 0; i < entries.length; i += 2) {
      if (entries[i + 1] != null) {
        result.put((String) entries[i], entries[i + 1]);
      }
    }
    return result;
  }
}
