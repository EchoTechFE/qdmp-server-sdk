package qdmp_test

// Contract exercised by this file:
//
//	asUser := client.WithAccessToken(token)
//	res, err := asUser.Mark.Add(ctx, generated.MarkAddJSONBody{SpuId: "...", Rating: &...})
//
// qdmp.MarkAddResult{ID string}
// mark.add has x-qdmp-token-required=true (shared/generated/route-meta.json).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// TestMarkAdd_Success verifies the happy path and that the JSON request
// body actually sent on the wire matches the openapi-defined shape:
// spuId as a bare string (int64 wire format), rating.value as a number.
func TestMarkAdd_Success(t *testing.T) {
	const token = "user-access-token-mark"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mark/v1/add" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("access-token"); got != token {
			t.Fatalf("access-token header = %q, want %q", got, token)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["spuId"] != "spu-99887766" {
			t.Fatalf("spuId = %v, want the raw string %q", body["spuId"], "spu-99887766")
		}
		rating, ok := body["rating"].(map[string]any)
		if !ok {
			t.Fatalf("rating missing or wrong shape: %+v", body)
		}
		if v, ok := rating["value"].(float64); !ok || v != 5 {
			t.Fatalf("rating.value = %v, want 5", rating["value"])
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-mark-1", map[string]any{
			"id": "mark-1001",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	value := 5
	res, err := client.WithAccessToken(token).Mark.Add(context.Background(), generated.MarkAddJSONBody{
		SpuId: "spu-99887766",
		Rating: &struct {
			Value *int `json:"value,omitempty"`
		}{Value: &value},
	})
	if err != nil {
		t.Fatalf("Mark.Add() error = %v, want nil", err)
	}
	if res.ID != "mark-1001" {
		t.Fatalf("Mark.Add() result = %+v, want ID = mark-1001", res)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestMarkAdd_MissingAccessToken_NoRequestSent verifies the same
// fail-fast-locally guarantee as user.me, on a POST + required-body
// endpoint (mark.add is also tokenRequired=true).
func TestMarkAdd_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken("").Mark.Add(context.Background(), generated.MarkAddJSONBody{SpuId: "spu-1"})
	if err == nil {
		t.Fatalf("Mark.Add() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("Mark.Add() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

// TestMarkAdd_HTTP401Code10005_DoesNotLeakAccessToken combines the 401/10005
// failure case with the secret-redaction requirement on a write endpoint.
func TestMarkAdd_HTTP401Code10005_DoesNotLeakAccessToken(t *testing.T) {
	const token = "mark-add-secret-token"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusUnauthorized, float64(10005), "access_token 格式错误或已吊销", "req-mark-401", nil)
	}))
	client := newTestClient(t, srv.URL)

	res, err := client.WithAccessToken(token).Mark.Add(context.Background(), generated.MarkAddJSONBody{SpuId: "spu-2"})
	if err == nil {
		t.Fatalf("Mark.Add() error = nil, want an error for HTTP 401 / code=10005")
	}
	if res != nil {
		t.Fatalf("Mark.Add() result = %+v, want nil on failure", res)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10005" || apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("apiErr = %+v, want Code=10005 HTTPStatus=401", apiErr)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("err.Error() leaks the accessToken: %q", err.Error())
	}
}
