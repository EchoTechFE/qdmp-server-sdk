package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// AuthService.Code2Session, AuthService.RefreshToken, and the cached
// GetAccessToken (via exchangeClientCredentials) currently only check that
// AccessToken is non-empty; none of them validate that ExpiresAt actually
// parses as a valid, non-negative integer. exchangeClientCredentials (see
// auth.go ~line 177) does call strconv.ParseInt on it, but happily accepts a
// negative value, which would make cachedValid's
// `entry.ExpiresAt-tokenRefreshBufferSeconds <= now` comparison misbehave
// (a negative ExpiresAt is always "expired", but a poisoned/corrupted
// response should be rejected outright rather than silently limping along).
//
// This file requires: any expiresAt that fails to parse as a non-negative
// base-10 int64 must produce a non-nil error from Code2Session/RefreshToken/
// GetAccessToken, and — critically for GetAccessToken — a rejected exchange
// must never be written through to the TokenStore, so the very next call
// still issues a real HTTP request instead of treating the poisoned result as
// a fresh cache hit.

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

// TestAuthCode2Session_NonNumericExpiresAtRejected drives Code2Session
// through a response with an accessToken/refreshToken/openId all populated,
// but an expiresAt that isn't a number at all.
func TestAuthCode2Session_NonNumericExpiresAtRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-c2s-1", map[string]any{
			"accessToken":  "tok",
			"refreshToken": "r",
			"expiresAt":    "garbage",
			"openId":       "o",
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because expiresAt %q does not parse as a number", "garbage")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthCode2Session_NegativeExpiresAtRejected drives Code2Session through
// a response whose expiresAt parses fine as an integer but is negative — a
// value that must never be treated as a usable absolute Unix-seconds expiry.
func TestAuthCode2Session_NegativeExpiresAtRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-c2s-2", map[string]any{
			"accessToken":  "tok",
			"refreshToken": "r",
			"expiresAt":    "-100",
			"openId":       "o",
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because expiresAt is negative (-100)")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthRefreshToken_NonNumericExpiresAtRejected is RefreshToken's
// counterpart to TestAuthCode2Session_NonNumericExpiresAtRejected.
func TestAuthRefreshToken_NonNumericExpiresAtRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-rt-1", map[string]any{
			"accessToken": "tok",
			"expiresAt":   "garbage",
		})
	}))
	client := newTestClient(t, srv.URL)

	result, err := client.Auth.RefreshToken(context.Background(), "a-valid-refresh-token")
	if err == nil {
		t.Fatalf("RefreshToken() error = nil, want an error because expiresAt %q does not parse as a number", "garbage")
	}
	if result != nil {
		t.Fatalf("RefreshToken() result = %+v, want nil on failure", result)
	}
}

// TestAuthRefreshToken_NegativeExpiresAtRejected is RefreshToken's
// counterpart to TestAuthCode2Session_NegativeExpiresAtRejected.
func TestAuthRefreshToken_NegativeExpiresAtRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-rt-2", map[string]any{
			"accessToken": "tok",
			"expiresAt":   "-100",
		})
	}))
	client := newTestClient(t, srv.URL)

	result, err := client.Auth.RefreshToken(context.Background(), "a-valid-refresh-token")
	if err == nil {
		t.Fatalf("RefreshToken() error = nil, want an error because expiresAt is negative (-100)")
	}
	if result != nil {
		t.Fatalf("RefreshToken() result = %+v, want nil on failure", result)
	}
}

// TestAuthGetAccessToken_NegativeExpiresAtRejectedAndNotCached is the most
// important test in this file: it proves a rejected (negative-expiresAt)
// app-level token exchange is never written through to the TokenStore. The
// mock server returns a poisoned response (expiresAt: "-100") on its first
// hit, then a legitimate response on every hit after. If the SDK incorrectly
// cached the poisoned exchange as "fresh" (e.g. by caching before validating,
// or by treating the error path as success), the second GetAccessToken call
// would either wrongly succeed with the poisoned token or short-circuit
// without ever hitting the server again. Both would defeat this assertion.
func TestAuthGetAccessToken_NegativeExpiresAtRejectedAndNotCached(t *testing.T) {
	var calls int64
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-gat-1", map[string]any{
				"accessToken": "token-from-poisoned-response",
				"expiresAt":   "-100",
			})
			return
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-badexp-gat-2", map[string]any{
			"accessToken": "token-from-good-response",
			"expiresAt":   "9999999999",
		})
	}))
	client := newTestClient(t, srv.URL)

	token, err := client.Auth.GetAccessToken(context.Background())
	if err == nil {
		t.Fatalf("first GetAccessToken() error = nil, want an error because expiresAt is negative (-100)")
	}
	if token != "" {
		t.Fatalf("first GetAccessToken() = %q, want the zero value on failure", token)
	}

	token, err = client.Auth.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("second GetAccessToken() error = %v, want nil", err)
	}
	if token != "token-from-good-response" {
		t.Fatalf("second GetAccessToken() = %q, want %q", token, "token-from-good-response")
	}
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Fatalf("server hit %d times, want exactly 2 (the first poisoned response must not have been cached as a fresh token, forcing a real second exchange)", got)
	}
}
