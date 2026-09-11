package io.github.echotechfe.qdmp.groups;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.github.echotechfe.qdmp.QdmpClient;
import io.github.echotechfe.qdmp.QdmpContext;
import io.github.echotechfe.qdmp.generated.CommentCreateRequest;
import io.github.echotechfe.qdmp.generated.CommentLikeRequest;
import io.github.echotechfe.qdmp.generated.CommentReplyRequest;
import io.github.echotechfe.qdmp.testsupport.TestClients;
import java.io.IOException;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class PostCommentGroupTest {
  private MockWebServer server;

  @BeforeEach
  void startServer() throws IOException {
    server = new MockWebServer();
    server.start();
  }

  @AfterEach
  void stopServer() throws IOException {
    server.shutdown();
  }

  @Test
  void postEndpoints_useDocumentedMethodsQueriesAndMapData() throws Exception {
    server.enqueue(ok("{\"id\":\"1\"}"));
    server.enqueue(ok("{\"items\":[]}"));
    server.enqueue(ok("{\"items\":[]}"));
    QdmpClient client = TestClients.create(server);
    assertThat(client.post().detail(QdmpContext.of("token"), "1")).containsEntry("id", "1");
    client.post().list(QdmpContext.of("token"), "0", null);
    client.post().myList(QdmpContext.of("token"), null, "20");
    assertThat(server.takeRequest().getPath()).isEqualTo("/post/v1/detail?postId=1");
    assertThat(server.takeRequest().getPath()).isEqualTo("/post/v1/list?offset=0");
    assertThat(server.takeRequest().getPath()).isEqualTo("/post/v1/me/list?limit=20");
  }

  @Test
  void commentEndpoints_useNumericPathsOptionalQueriesBodiesAndDtos() throws Exception {
    server.enqueue(ok("{\"id\":\"2\"}"));
    server.enqueue(ok("{\"id\":\"3\"}"));
    server.enqueue(new MockResponse().setBody("{\"code\":\"0\",\"message\":\"ok\"}"));
    server.enqueue(ok("{\"items\":[],\"count\":0}"));
    server.enqueue(ok("{\"items\":[],\"hasMore\":false,\"count\":0}"));
    QdmpClient client = TestClients.create(server);
    assertThat(
            client
                .comment()
                .create(
                    QdmpContext.of("token"), new CommentCreateRequest().postId("1").content("hi"))
                .getId())
        .isEqualTo("2");
    assertThat(
            client
                .comment()
                .reply(QdmpContext.of("token"), "1", new CommentReplyRequest().content("reply"))
                .getId())
        .isEqualTo("3");
    client.comment().like(QdmpContext.of("token"), "1", new CommentLikeRequest().liked(true));
    assertThat(client.comment().postComments(QdmpContext.of("token"), "1", null, "0").getCount())
        .isZero();
    assertThat(client.comment().replies(QdmpContext.of("token"), "1", "10", "next").getHasMore())
        .isFalse();
    assertThat(server.takeRequest().getMethod()).isEqualTo("POST");
    assertThat(server.takeRequest().getPath()).isEqualTo("/comment/1/reply");
    assertThat(server.takeRequest().getPath()).isEqualTo("/comment/1/like");
    assertThat(server.takeRequest().getPath()).isEqualTo("/post/1/comments?offset=0");
    assertThat(server.takeRequest().getPath()).isEqualTo("/comment/1/replies?limit=10&cursor=next");
  }

  @Test
  void postAndCommentGroups_rejectNullContextBeforeNetwork() {
    QdmpClient client = TestClients.create(server);
    assertThatThrownBy(() -> client.post().detail(null, "1"))
        .isInstanceOf(NullPointerException.class);
    assertThatThrownBy(() -> client.comment().like(null, "1", new CommentLikeRequest().liked(true)))
        .isInstanceOf(NullPointerException.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void commentEndpoints_rejectDotSegmentIdsBeforeNetwork() {
    QdmpClient client = TestClients.create(server);
    assertThatThrownBy(
            () -> client.comment().reply(QdmpContext.of("token"), "..", new CommentReplyRequest()))
        .isInstanceOf(IllegalArgumentException.class);
    assertThatThrownBy(
            () -> client.comment().like(QdmpContext.of("token"), "..", new CommentLikeRequest()))
        .isInstanceOf(IllegalArgumentException.class);
    assertThatThrownBy(
            () -> client.comment().postComments(QdmpContext.of("token"), "..", null, null))
        .isInstanceOf(IllegalArgumentException.class);
    assertThatThrownBy(() -> client.comment().replies(QdmpContext.of("token"), "..", null, null))
        .isInstanceOf(IllegalArgumentException.class);
    assertThatThrownBy(
            () -> client.comment().reply(QdmpContext.of("token"), null, new CommentReplyRequest()))
        .isInstanceOf(NullPointerException.class);
    assertThatThrownBy(
            () -> client.comment().like(QdmpContext.of("token"), null, new CommentLikeRequest()))
        .isInstanceOf(NullPointerException.class);
    assertThatThrownBy(
            () -> client.comment().postComments(QdmpContext.of("token"), null, null, null))
        .isInstanceOf(NullPointerException.class);
    assertThatThrownBy(() -> client.comment().replies(QdmpContext.of("token"), null, null, null))
        .isInstanceOf(NullPointerException.class);
    assertThat(server.getRequestCount()).isEqualTo(0);
  }

  @Test
  void like_acceptsSuccessfulResponseWithEmptyData() {
    server.enqueue(ok("{}"));
    QdmpClient client = TestClients.create(server);

    client.comment().like(QdmpContext.of("token"), "1", new CommentLikeRequest().liked(true));

    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  private static MockResponse ok(String data) {
    return new MockResponse().setBody("{\"code\":\"0\",\"message\":\"ok\",\"data\":" + data + "}");
  }
}
