package qdmp_test

// Contract exercised by this file:
//
//	asUser := client.WithAccessToken(token)
//	res, err := asUser.Spu.Search(ctx, generated.SpuSearchParams{Keyword: ...})
//
// qdmp.SpuSearchResult{Items []qdmp.SpuSearchItem, TotalCount string}
// qdmp.SpuSearchItem{ID, Name, Image, TypeID, TypeName, WishCount, MarkCount string}
//
// generated.SpuSearchParams bundles the query fields with the "standard"
// authScheme header fields (AccessToken, XEchoQdmpVersion) because that is
// what oapi-codegen produced from shared/openapi.yaml (header params and
// query params both land on the operation's Params struct). The SDK must
// NOT trust whatever the caller happened to put in those two header fields
// — the real access-token comes from WithAccessToken(), and the real
// x-echo-qdmp-version comes from ClientOptions.QdmpVersion (default "1.0").
// TestSpuSearch_Success deliberately fills them with decoy values to prove
// the SDK ignores them and derives the real headers itself.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func TestSpuSearch_Success_IgnoresCallerSuppliedHeaderFields(t *testing.T) {
	const realToken = "spu-search-real-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/spu/v1/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("keyword"); got != "哪吒" {
			t.Fatalf("keyword query param = %q, want %q", got, "哪吒")
		}
		if got := r.Header.Get("access-token"); got != realToken {
			t.Fatalf("access-token header = %q, want the real derived-client token %q (not the caller-supplied decoy)", got, realToken)
		}
		if got := r.Header.Get("x-echo-qdmp-version"); got != "1.0" {
			t.Fatalf("x-echo-qdmp-version header = %q, want the client's configured default %q (not the caller-supplied decoy)", got, "1.0")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-spu-1", map[string]any{
			"items": []map[string]any{
				{
					"id":        "spu-1",
					"name":      "哪吒手办",
					"image":     "https://example.com/spu1.png",
					"typeId":    "type-1",
					"typeName":  "手办",
					"wishCount": "42",
					"markCount": "7",
				},
			},
			"totalCount": "1",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	keyword := "哪吒"
	res, err := client.WithAccessToken(realToken).Spu.Search(context.Background(), generated.SpuSearchParams{
		Keyword: &keyword,
		// Deliberately wrong: the SDK must not read these back off the
		// params struct when building the actual HTTP request headers.
		AccessToken:      "decoy-token-from-caller-should-be-ignored",
		XEchoQdmpVersion: "decoy-version-from-caller-should-be-ignored",
	})
	if err != nil {
		t.Fatalf("Spu.Search() error = %v, want nil", err)
	}
	if res.TotalCount != "1" {
		t.Fatalf("TotalCount = %q, want %q", res.TotalCount, "1")
	}
	if len(res.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(res.Items))
	}
	item := res.Items[0]
	if item.ID != "spu-1" || item.Name != "哪吒手办" || item.WishCount != "42" || item.MarkCount != "7" {
		t.Fatalf("unexpected item: %+v", item)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestSpuSearch_MissingAccessToken_NoRequestSent: spu.search is
// tokenRequired=true per shared/generated/route-meta.json (v1 default:
// no silent app-token fallback), so an empty token must fail locally.
func TestSpuSearch_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken("").Spu.Search(context.Background(), generated.SpuSearchParams{})
	if err == nil {
		t.Fatalf("Spu.Search() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("Spu.Search() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

// TestSpuSearch_GatewayEnvelopeError exercises the second, distinct
// response envelope confirmed by real traffic capture: {code, message,
// details} with no data/requestId — observed for e.g. tag/v1/search
// (code=13 "query parse error"). request() parses both envelope shapes
// through one shared code path for every "standard"-scheme endpoint, so
// this is exercised here against spu.search too, not only against the
// operation where the shape was first captured.
func TestSpuSearch_GatewayEnvelopeError(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayEnvelope(w, http.StatusInternalServerError, 13, "query parse error")
	}))
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken("some-token").Spu.Search(context.Background(), generated.SpuSearchParams{})
	if err == nil {
		t.Fatalf("Spu.Search() error = nil, want an error for the gateway-envelope failure shape")
	}
	if res != nil {
		t.Fatalf("Spu.Search() result = %+v, want nil on failure", res)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "13" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "13")
	}
	if apiErr.Message != "query parse error" {
		t.Fatalf("apiErr.Message = %q, want %q", apiErr.Message, "query parse error")
	}
	if apiErr.RequestID != "" {
		t.Fatalf("apiErr.RequestID = %q, want empty (the gateway envelope has no requestId field)", apiErr.RequestID)
	}
}
