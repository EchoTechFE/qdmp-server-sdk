package qdmp_test

// Contract exercised by this file:
//
//	qdmpCtx := qdmp.Context{AccessToken: token}   // 凭证每次调用显式传
//	res, err := asUser.WishSpu.Add(ctx, generated.WishAddJSONBody{Ids: []string{...}, Type: ...})
//	res, err := asUser.WishSpu.Cancel(ctx, generated.WishCancelJSONBody{Ids: []string{...}, Type: ...})
//	res, err := asUser.WishSpu.List(ctx, generated.WishListParams{Offset: "0", Limit: "20"})
//
// qdmp.WishAddResult{SuccessCount string}
// qdmp.WishCancelResult{SuccessCount string}
// qdmp.WishListResult{Items []qdmp.WishItem, TotalCount string}
// wishspu.add / wishspu.cancel / wishspu.list all have
// x-qdmp-token-required=true (shared/generated/route-meta.json).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func TestWishSpuAdd_Success(t *testing.T) {
	const token = "wishspu-add-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/wishspu/v1/add" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		ids, ok := body["ids"].([]any)
		if !ok || len(ids) != 2 || ids[0] != "spu-1" || ids[1] != "spu-2" {
			t.Fatalf("ids = %v, want [spu-1 spu-2]", body["ids"])
		}
		if body["type"] != "WISH_SPU_TYPE_SPU" {
			t.Fatalf("type = %v, want WISH_SPU_TYPE_SPU", body["type"])
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-wish-1", map[string]any{
			"successCount": "2",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WishSpu.Add(context.Background(), qdmp.Context{AccessToken: token}, generated.WishAddJSONBody{
		Ids:  []string{"spu-1", "spu-2"},
		Type: generated.WishAddJSONBodyTypeWISHSPUTYPESPU,
	})
	if err != nil {
		t.Fatalf("WishSpu.Add() error = %v, want nil", err)
	}
	if res.SuccessCount != "2" {
		t.Fatalf("SuccessCount = %q, want %q", res.SuccessCount, "2")
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

func TestWishSpuAdd_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WishSpu.Add(context.Background(), qdmp.Context{AccessToken: ""}, generated.WishAddJSONBody{
		Ids:  []string{"spu-1"},
		Type: generated.WishAddJSONBodyTypeWISHSPUTYPESPU,
	})
	if err == nil {
		t.Fatalf("WishSpu.Add() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("WishSpu.Add() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

func TestWishSpuCancel_Success(t *testing.T) {
	const token = "wishspu-cancel-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/wishspu/v1/cancel" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-wish-2", map[string]any{
			"successCount": "1",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WishSpu.Cancel(context.Background(), qdmp.Context{AccessToken: token}, generated.WishCancelJSONBody{
		Ids:  []string{"spu-1"},
		Type: generated.WishCancelJSONBodyTypeWISHSPUTYPESPU,
	})
	if err != nil {
		t.Fatalf("WishSpu.Cancel() error = %v, want nil", err)
	}
	if res.SuccessCount != "1" {
		t.Fatalf("SuccessCount = %q, want %q", res.SuccessCount, "1")
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestWishSpuList_Success also verifies the required (non-pointer) offset
// and limit query params are always sent, unlike the optional typeId.
func TestWishSpuList_Success(t *testing.T) {
	const token = "wishspu-list-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/wishspu/v1/list" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("offset"); got != "0" {
			t.Fatalf("offset query param = %q, want %q", got, "0")
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Fatalf("limit query param = %q, want %q", got, "20")
		}
		if _, ok := r.URL.Query()["typeId"]; ok {
			t.Fatalf("typeId query param present, want absent when nil")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-wish-3", map[string]any{
			"items": []map[string]any{
				{
					"id":        "wish-1",
					"spu":       nil,
					"typeId":    "type-1",
					"markAt":    "1785508950",
					"createdAt": "1785500000",
				},
			},
			"totalCount": "1",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WishSpu.List(context.Background(), qdmp.Context{AccessToken: token}, generated.WishListParams{
		Offset: "0",
		Limit:  "20",
	})
	if err != nil {
		t.Fatalf("WishSpu.List() error = %v, want nil", err)
	}
	if res.TotalCount != "1" || len(res.Items) != 1 || res.Items[0].ID != "wish-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

func TestWishSpuList_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WishSpu.List(context.Background(), qdmp.Context{AccessToken: ""}, generated.WishListParams{Offset: "0", Limit: "20"})
	if err == nil {
		t.Fatalf("WishSpu.List() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("WishSpu.List() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}
