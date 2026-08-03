package qdmp_test

// Contract exercised by this file:
//
//	qdmpCtx := qdmp.Context{AccessToken: token}   // 凭证每次调用显式传
//	res, err := client.Tag.Detail(ctx, qdmpCtx, generated.TagDetailParams{Id: "..."})
//	res, err := client.Tag.Search(ctx, qdmpCtx, generated.TagSearchParams{Keyword: ...})
//
// qdmp.TagDetailResult{Tag qdmp.TagInfo}
// qdmp.TagSearchResult{Items []qdmp.TagInfo, TotalCount string}
// tag.detail / tag.search both have x-qdmp-token-required=true.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func TestTagDetail_Success(t *testing.T) {
	const token = "tag-detail-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/tag/v1/detail" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "tag-1" {
			t.Fatalf("id query param = %q, want %q", got, "tag-1")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-tag-1", map[string]any{
			"tag": map[string]any{
				"id":       "tag-1",
				"name":     "手办",
				"image":    "https://example.com/tag1.png",
				"typeId":   "type-1",
				"typeName": "商品类型",
				"island": map[string]any{
					"id":     "island-9",
					"name":   "哪吒岛",
					"image":  "https://example.com/island9.png",
					"joined": false,
				},
			},
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.Tag.Detail(context.Background(), qdmp.Context{AccessToken: token}, generated.TagDetailParams{Id: "tag-1"})
	if err != nil {
		t.Fatalf("Tag.Detail() error = %v, want nil", err)
	}
	if res.Tag.ID != "tag-1" || res.Tag.Name != "手办" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Tag.Island == nil || res.Tag.Island.ID != "island-9" {
		t.Fatalf("unexpected embedded island: %+v", res.Tag.Island)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

func TestTagDetail_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.Tag.Detail(context.Background(), qdmp.Context{AccessToken: ""}, generated.TagDetailParams{Id: "tag-1"})
	if err == nil {
		t.Fatalf("Tag.Detail() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("Tag.Detail() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

// TestTagSearch_Success verifies the search query params (including the
// repeated filterOptions array param) are sent correctly.
func TestTagSearch_Success(t *testing.T) {
	const token = "tag-search-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/tag/v1/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("keyword"); got != "哪吒" {
			t.Fatalf("keyword query param = %q, want %q", got, "哪吒")
		}
		if got := r.URL.Query()["filterOptions"]; len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
			t.Fatalf("filterOptions query params = %v, want [f1 f2]", got)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-tag-2", map[string]any{
			"items": []map[string]any{
				{"id": "tag-1", "name": "手办", "image": "", "typeId": "type-1", "typeName": "商品类型"},
			},
			"totalCount": "1",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	keyword := "哪吒"
	filterOptions := []string{"f1", "f2"}
	res, err := client.Tag.Search(context.Background(), qdmp.Context{AccessToken: token}, generated.TagSearchParams{
		Keyword:       &keyword,
		FilterOptions: &filterOptions,
	})
	if err != nil {
		t.Fatalf("Tag.Search() error = %v, want nil", err)
	}
	if res.TotalCount != "1" || len(res.Items) != 1 || res.Items[0].ID != "tag-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestTagSearch_GatewayEnvelopeError: tag.search is the operation where the
// gateway-style envelope was first captured (code=13, "query parse error"),
// so this test exercises it directly against that operation.
func TestTagSearch_GatewayEnvelopeError(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayEnvelope(w, http.StatusInternalServerError, 13, "query parse error")
	}))
	client := newTestClient(t, srv.URL)

	res, err := client.Tag.Search(context.Background(), qdmp.Context{AccessToken: "some-token"}, generated.TagSearchParams{})
	if err == nil {
		t.Fatalf("Tag.Search() error = nil, want an error for the gateway-envelope failure shape")
	}
	if res != nil {
		t.Fatalf("Tag.Search() result = %+v, want nil on failure", res)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "13" || apiErr.RequestID != "" {
		t.Fatalf("apiErr = %+v, want Code=13 and empty RequestID", apiErr)
	}
}
