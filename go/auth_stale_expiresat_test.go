package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// exchangeClientCredentials (auth.go ~line 150-184) validates the wire
// expiresAt via parseExpiresAt, which only rejects a non-numeric string or a
// negative value (see auth.go's parseExpiresAt doc comment). It does not
// reject a non-negative expiresAt that is nonetheless already expired
// relative to the current time — e.g. "1", which parses fine as the
// perfectly valid-looking absolute Unix-seconds timestamp 1 (1970-01-01
// 00:00:01 UTC), a moment far in the past for any real "now". Such a value
// sails through parseExpiresAt (it is not negative) and is written straight
// through to the TokenStore via a.store.Set(...) and returned as if it were
// a fresh, valid token to the very first caller that triggered the exchange
// — even though cachedValid's own `entry.ExpiresAt-tokenRefreshBufferSeconds
// <= now` check would immediately (and correctly) treat that same cached
// entry as expired on the *next* call. In other words: a poisoned/stale
// response is accepted and handed to the caller once, on the very call that
// fetched it, before the cache even gets a chance to reject it.
//
// New requirement: exchangeClientCredentials must also reject (return a
// non-nil error, and never call a.store.Set) whenever the parsed expiresAt
// is <= time.Now().Unix() at the moment the exchange completes — a stale
// expiry must never be treated as a usable, freshly-issued token, exactly
// like the existing negative-expiresAt rejection in
// auth_expiresat_validation_test.go.

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// TestAuthGetAccessToken_StaleExpiresAtRejectedAndNotCached mirrors
// TestAuthGetAccessToken_NegativeExpiresAtRejectedAndNotCached in
// auth_expiresat_validation_test.go, but with a non-negative, already-
// expired expiresAt ("1") instead of a negative one ("-100"). The mock
// server returns the poisoned response on its first hit, then a legitimate
// response on every hit after. If the SDK incorrectly accepted/cached the
// poisoned exchange as "fresh" (since expiresAt="1" is not negative and
// today's parseExpiresAt only rejects negative/non-numeric values), the
// first GetAccessToken call would wrongly succeed with the stale token
// and/or the second call would short-circuit without ever hitting the
// server again. Both would defeat the assertions below.
func TestAuthGetAccessToken_StaleExpiresAtRejectedAndNotCached(t *testing.T) {
	var calls int64
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			businessEnvelope(w, http.StatusOK, "0", "ok", "req-staleexp-gat-1", map[string]any{
				"accessToken": "token-from-poisoned-response",
				"expiresAt":   "1",
			})
			return
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-staleexp-gat-2", map[string]any{
			"accessToken": "token-from-good-response",
			"expiresAt":   "9999999999",
		})
	}))
	client := newTestClient(t, srv.URL)

	token, err := client.Auth.GetAccessToken(context.Background())
	if err == nil {
		t.Fatalf("first GetAccessToken() error = nil, want an error because expiresAt=\"1\" is already expired (1970-01-01), even though it is not negative")
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
		t.Fatalf("server hit %d times, want exactly 2 (the first stale-expiresAt response must not have been cached as a fresh token, forcing a real second exchange)", got)
	}
}

// TestAuthGetAccessToken_ExpiresWithinBufferRejectedAndNotCached is a
// regression test for a follow-up finding from codex's convergence-check
// review: the fix above only rejected expiresAt <= now, but cachedValid's
// own freshness predicate is stricter — it requires
// entry.ExpiresAt-tokenRefreshBufferSeconds > now (i.e. more than 300
// seconds of remaining validity), not just "not yet expired". A response
// expiring in, say, 100 seconds (not negative, not even in the past) would
// therefore pass the old check, get cached and handed to the very first
// caller as "fresh" — only for cachedValid to immediately (and correctly)
// call that same entry stale on the *next* call. The write path must use
// the identical predicate as the read path.
func TestAuthGetAccessToken_ExpiresWithinBufferRejectedAndNotCached(t *testing.T) {
	var calls int64
	soonExpiresAt := strconv.FormatInt(time.Now().Add(100*time.Second).Unix(), 10)
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			businessEnvelope(w, http.StatusOK, "0", "ok", "req-buffer-gat-1", map[string]any{
				"accessToken": "token-from-poisoned-response",
				"expiresAt":   soonExpiresAt,
			})
			return
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-buffer-gat-2", map[string]any{
			"accessToken": "token-from-good-response",
			"expiresAt":   "9999999999",
		})
	}))
	client := newTestClient(t, srv.URL)

	token, err := client.Auth.GetAccessToken(context.Background())
	if err == nil {
		t.Fatalf("first GetAccessToken() error = nil, want an error because expiresAt is only 100s away (less than the 300s refresh buffer)")
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
		t.Fatalf("server hit %d times, want exactly 2 (the first within-buffer response must not have been cached as fresh, forcing a real second exchange)", got)
	}
}
