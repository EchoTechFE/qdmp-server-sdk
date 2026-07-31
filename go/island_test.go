package qdmp_test

// Contract exercised by this file:
//
//	asUser := client.WithAccessToken(token)
//	res, err := asUser.Island.Detail(ctx, generated.IslandDetailParams{Id: "..."})
//
// qdmp.IslandDetailResult{Island qdmp.IslandInfo}
// island.detail has x-qdmp-token-required=true (shared/generated/route-meta.json).

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func TestIslandDetail_Success(t *testing.T) {
	const token = "island-detail-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/island/v1/detail" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "island-1" {
			t.Fatalf("id query param = %q, want %q", got, "island-1")
		}
		if got := r.Header.Get("access-token"); got != token {
			t.Fatalf("access-token header = %q, want %q", got, token)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-island-1", map[string]any{
			"island": map[string]any{
				"id":     "island-1",
				"name":   "哪吒岛",
				"image":  "https://example.com/island1.png",
				"joined": true,
			},
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken(token).Island.Detail(context.Background(), generated.IslandDetailParams{Id: "island-1"})
	if err != nil {
		t.Fatalf("Island.Detail() error = %v, want nil", err)
	}
	if res.Island.ID != "island-1" || res.Island.Name != "哪吒岛" || !res.Island.Joined {
		t.Fatalf("unexpected result: %+v", res)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestIslandDetail_MissingAccessToken_NoRequestSent: island.detail is
// tokenRequired=true per shared/generated/route-meta.json, so an empty
// token must fail locally without ever reaching the network — even though
// this operation is confirmed to also work with an app-level token, the SDK
// still requires the caller to explicitly pass one.
func TestIslandDetail_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken("").Island.Detail(context.Background(), generated.IslandDetailParams{Id: "island-1"})
	if err == nil {
		t.Fatalf("Island.Detail() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("Island.Detail() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

// TestIslandDetail_GatewayEnvelopeError exercises the gateway-style error
// envelope ({code, message, details}, no data/requestId) against island.detail
// too, not only the operation where it was first captured.
func TestIslandDetail_GatewayEnvelopeError(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayEnvelope(w, http.StatusInternalServerError, 13, "query parse error")
	}))
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken("some-token").Island.Detail(context.Background(), generated.IslandDetailParams{Id: "island-1"})
	if err == nil {
		t.Fatalf("Island.Detail() error = nil, want an error for the gateway-envelope failure shape")
	}
	if res != nil {
		t.Fatalf("Island.Detail() result = %+v, want nil on failure", res)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "13" || apiErr.RequestID != "" {
		t.Fatalf("apiErr = %+v, want Code=13 and empty RequestID", apiErr)
	}
}
