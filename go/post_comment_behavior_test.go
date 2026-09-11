package qdmp_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func stringPtr(value string) *string { return &value }

func TestPostAndCommentOperations_MapWireContractAndDecodeData(t *testing.T) {
	expected := []struct {
		method, path, query string
		body                bool
		data                any
	}{
		{http.MethodPost, "/mark/v1/batch/add", "", true, map[string]any{"result": map[string]string{"1": "m1"}}},
		{http.MethodGet, "/post/v1/detail", "postId=1", false, map[string]any{"id": "1"}},
		{http.MethodGet, "/post/v1/list", "limit=20&offset=0", false, map[string]any{"items": []any{}}},
		{http.MethodGet, "/post/v1/me/list", "", false, map[string]any{"items": []any{}}},
		{http.MethodPost, "/comment", "", true, map[string]any{"id": "c1"}},
		{http.MethodPost, "/comment/1/reply", "", true, map[string]any{"id": "c2"}},
		{http.MethodPost, "/comment/1/like", "", true, nil},
		{http.MethodGet, "/post/1/comments", "limit=10&offset=0", false, map[string]any{"items": []any{}, "count": 0}},
		{http.MethodGet, "/comment/1/replies", "cursor=next&limit=10", false, map[string]any{"items": []any{}, "count": 0, "hasMore": false, "cursor": "next"}},
	}
	request := 0
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if request >= len(expected) {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
		want := expected[request]
		request++
		if r.Method != want.method || r.URL.EscapedPath() != want.path || r.URL.RawQuery != want.query {
			t.Fatalf("request = %s %s?%s, want %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery, want.method, want.path, want.query)
		}
		if want.body && r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("missing JSON body")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "", want.data)
	}))
	c := newTestClient(t, srv.URL)
	ctx := context.Background()
	q := qdmp.Context{AccessToken: "token"}
	if result, err := c.Mark.BatchAdd(ctx, q, generated.MarkBatchAddJSONRequestBody{SpuIds: []string{"1"}}); err != nil || result.Result["1"] != "m1" {
		t.Fatalf("batch add = %#v, %v", result, err)
	}
	if result, err := c.Post.Detail(ctx, q, "1"); err != nil || result["id"] != "1" {
		t.Fatalf("detail = %#v, %v", result, err)
	}
	if _, err := c.Post.List(ctx, q, generated.PostListParams{Offset: stringPtr("0"), Limit: stringPtr("20")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Post.MyList(ctx, q, generated.PostMyListParams{}); err != nil {
		t.Fatal(err)
	}
	if result, err := c.Comment.Create(ctx, q, generated.CommentCreateJSONRequestBody{PostId: "1", Content: "hi"}); err != nil || result.ID != "c1" {
		t.Fatalf("create = %#v, %v", result, err)
	}
	if result, err := c.Comment.Reply(ctx, q, "1", generated.CommentReplyJSONRequestBody{Content: "hi"}); err != nil || result.ID != "c2" {
		t.Fatalf("reply = %#v, %v", result, err)
	}
	if err := c.Comment.Like(ctx, q, "1", generated.CommentLikeJSONRequestBody{Liked: true}); err != nil {
		t.Fatal(err)
	}
	if result, err := c.Comment.PostComments(ctx, q, "1", generated.PostCommentsParams{Limit: stringPtr("10"), Offset: stringPtr("0")}); err != nil || result.Count != 0 {
		t.Fatalf("comments = %#v, %v", result, err)
	}
	if result, err := c.Comment.Replies(ctx, q, "1", generated.CommentRepliesParams{Limit: stringPtr("10"), Cursor: stringPtr("next")}); err != nil || result.Cursor != "next" {
		t.Fatalf("replies = %#v, %v", result, err)
	}
	if request != len(expected) {
		t.Fatalf("requests = %d, want %d", request, len(expected))
	}
}

func TestPostAndCommentOperations_RejectDotSegmentIDsBeforeRequest(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) { t.Fatalf("unexpected request: %s", r.URL) })
	c := newTestClient(t, startServer(t, counter).URL)
	ctx, q := context.Background(), qdmp.Context{AccessToken: "token"}
	for _, call := range []func() error{
		func() error {
			_, err := c.Comment.Reply(ctx, q, "..", generated.CommentReplyJSONRequestBody{})
			return err
		},
		func() error { return c.Comment.Like(ctx, q, "..", generated.CommentLikeJSONRequestBody{}) },
		func() error {
			_, err := c.Comment.PostComments(ctx, q, "..", generated.PostCommentsParams{})
			return err
		},
		func() error { _, err := c.Comment.Replies(ctx, q, "..", generated.CommentRepliesParams{}); return err },
	} {
		if err := call(); err == nil {
			t.Fatal("invalid ID unexpectedly succeeded")
		}
	}
	if counter.Count() != 0 {
		t.Fatalf("requests=%d", counter.Count())
	}
}

func TestPostAndCommentOperations_RejectMissingTokenBeforeRequest(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) { t.Fatalf("unexpected request: %s", r.URL) })
	c := newTestClient(t, startServer(t, counter).URL)
	_, err := c.Post.List(context.Background(), qdmp.Context{}, generated.PostListParams{})
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) || counter.Count() != 0 {
		t.Fatalf("err=%v requests=%d", err, counter.Count())
	}
}
