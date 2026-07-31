package qdmp_test

// Shared helpers for the black-box test suite in this directory.
//
// The suite is written as an external test package (qdmp_test) on purpose:
// it only exercises the public API described by shared/openapi.yaml, never
// internal fields, so it stays valid across whatever internal structure the
// implementation phase chooses.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// businessEnvelope writes the "normal" business response shape confirmed by
// real traffic capture: {code, message, requestId, data}. code is passed as
// `any` because real responses have been observed with both a numeric code
// (0, 10008, 20000...) and a string code ("0") — the SDK must treat both
// the same way, so tests exercise both.
func businessEnvelope(w http.ResponseWriter, status int, code any, message string, requestID string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"code":    code,
		"message": message,
	}
	if requestID != "" {
		body["requestId"] = requestID
	}
	if data != nil {
		body["data"] = data
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		panic(err)
	}
}

// gatewayEnvelope writes the second, gRPC-gateway-style error shape
// confirmed by real traffic capture: {code, message, details} — no data,
// no requestId. Observed for tag/v1/search (code 13, "query parse error")
// and mark/v1/me/list (code 2, "openid is required"). This shape shares the
// same `request()` parsing path as businessEnvelope for every authScheme,
// so it is exercised generically against the representative endpoints
// under test here, not only the two operations where it was first observed.
func gatewayEnvelope(w http.ResponseWriter, status int, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"code":    code,
		"message": message,
		"details": nil,
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		panic(err)
	}
}

// requestCounter wraps an http.HandlerFunc and counts how many times it was
// invoked, so tests can assert "the mock server was hit exactly N times"
// (used both for single-flight assertions and for "must not send a request
// at all when the token is missing" assertions).
type requestCounter struct {
	count   int64
	handler http.HandlerFunc
}

func newRequestCounter(handler http.HandlerFunc) *requestCounter {
	return &requestCounter{handler: handler}
}

func (c *requestCounter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&c.count, 1)
	c.handler(w, r)
}

func (c *requestCounter) Count() int64 {
	return atomic.LoadInt64(&c.count)
}

// newTestClient builds a *qdmp.Client pointed at an httptest.Server via the
// BaseURL override, with the fixed test appId/appSecret used throughout
// this suite. It never touches local.env or any real network endpoint.
func newTestClient(t *testing.T, baseURL string) *qdmp.Client {
	t.Helper()
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "test-app-id",
		AppSecret: "test-app-secret-do-not-leak",
		BaseURL:   baseURL,
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() returned unexpected error: %v", err)
	}
	return client
}

// startServer is a small httptest.NewServer wrapper that registers cleanup.
func startServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
