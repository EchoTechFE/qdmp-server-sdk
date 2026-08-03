package qdmp_test

// Contract exercised by this file (see CONTRACT.md §2 — not yet implemented):
//
//	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
//	    AccessToken:  "at_xxx",
//	    RefreshToken: "rt_xxx",
//	    ExpiresAt:    "1785766657", // optional, Unix-seconds string
//	    OnRefresh: func(ctx context.Context, t qdmp.RefreshedUserToken) error { return nil },
//	})
//	me, err := uc.User.Me(ctx)         // business method signatures unchanged
//	snapshot := uc.Credential()         // read-only snapshot
//
// WithUserCredential reuses the existing *UserClient business-group type
// (uc.User, uc.GenAI, ...); it only wires automatic refresh into the
// transport used underneath. WithAccessToken keeps its current
// static-token, no-auto-refresh semantics untouched.
//
// Behavior contract under test (CONTRACT.md §2.3, plus §2.4/§2.5):
//  1. Proactive refresh: expiresAt-now <= 300s triggers a refresh before the
//     business call goes out.
//  2. Refresh action: POST /auth/v1/refresh, body {"refreshToken": "<orig>"},
//     authScheme noAuth (no access-token header); refreshToken never changes.
//  3. onRefresh runs after internal state is updated; if it errors, the
//     whole business call fails with that error but the new token is kept
//     (no rollback).
//  4. 401 + code 10005/10006 -> refresh once, retry the original request
//     exactly once.
//  5. A second failure after the retry is returned as-is, no further retry.
//  6. 5xx / network errors / non-401 business errors / 401 with an
//     unrelated code must never trigger a refresh.
//  7. Refresh failure itself is surfaced as-is, no business retry.
//  8. Concurrent callers collapse into a single in-flight refresh.
//  9. Credential() is a read-only, independent snapshot.
// 10. genai (x-openapi-access-token / x-openapi-app-id headers) gets the
//     same auto-refresh/401-retry behavior.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

const (
	ucOriginalRefreshToken = "uc-original-refresh-token"
)

// futureExpiresAt returns a wire-shaped (string, Unix-seconds) expiresAt
// value d in the future, computed relative to time.Now() so these tests
// never go stale as real wall-clock time advances (matching the existing
// convention in auth_test.go).
func futureExpiresAt(d time.Duration) string {
	return strconv.FormatInt(time.Now().Add(d).Unix(), 10)
}

// snapshotStringField reads a string-typed exported field by name off
// whatever uc.Credential() returns, transparently handling both a value type
// and a pointer type, without this test file needing to know (or guess) the
// snapshot's concrete type name.
func snapshotStringField(t *testing.T, snapshot any, name string) string {
	t.Helper()
	v := reflect.ValueOf(snapshot)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			t.Fatalf("Credential() returned a nil *%s", v.Type().Elem().Name())
		}
		v = v.Elem()
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		t.Fatalf("Credential() snapshot has no field %q (got type %s)", name, v.Type())
	}
	return f.String()
}

// tryTamperStringField attempts to mutate a string field on the snapshot
// in place via reflection. This only succeeds (and thus only matters) if
// Credential() handed back a pointer into the client's real internal state
// instead of an independent copy — which is exactly the bug this exists to
// catch. If Credential() returns a plain value type, CanSet() is false and
// this is a harmless no-op (the immutability property then holds by
// construction, as Go value semantics already guarantee it).
func tryTamperStringField(snapshot any, name, newValue string) {
	v := reflect.ValueOf(snapshot)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	v = v.Elem()
	f := v.FieldByName(name)
	if f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(newValue)
	}
}

// --- 1/2: proactive refresh -------------------------------------------------

// TestUserCredential_ProactiveRefresh_WithinBuffer verifies bullet 1: an
// expiresAt sitting inside the 300s buffer must trigger a refresh BEFORE the
// business call is sent, and the business call must then use the freshly
// refreshed access token (not the stale constructor-supplied one).
func TestUserCredential_ProactiveRefresh_WithinBuffer(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/refresh" {
			t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("access-token"); got != "" {
			t.Fatalf("refresh request must not send an access-token header (authScheme noAuth), got %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode refresh body: %v", err)
		}
		if body["refreshToken"] != ucOriginalRefreshToken {
			t.Fatalf("refresh body refreshToken = %v, want %q", body["refreshToken"], ucOriginalRefreshToken)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-1", map[string]any{
			"accessToken": "freshly-refreshed-token",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("access-token"); got != "freshly-refreshed-token" {
			t.Fatalf("business call access-token header = %q, want the proactively refreshed token %q", got, "freshly-refreshed-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-1", map[string]any{
			"id": "user-uc-1",
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "stale-access-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(250 * time.Second), // inside the 300s buffer
	})

	me, err := uc.User.Me(context.Background())
	if err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}
	if me.ID != "user-uc-1" {
		t.Fatalf("User.Me() result = %+v, want ID = user-uc-1", me)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1", businessCounter.Count())
	}
}

// TestUserCredential_NoProactiveRefresh_OutsideBuffer is the complementary
// boundary case: an expiresAt just outside the 300s buffer must not trigger
// any refresh at all, and the business call must go out with the original
// access token.
func TestUserCredential_NoProactiveRefresh_OutsideBuffer(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: expiresAt is outside the 300s buffer")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("access-token"); got != "still-fresh-access-token" {
			t.Fatalf("business call access-token header = %q, want the original token %q (no refresh should have happened)", got, "still-fresh-access-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-2", map[string]any{
			"id": "user-uc-2",
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "still-fresh-access-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(301 * time.Second), // just outside the 300s buffer
	})

	me, err := uc.User.Me(context.Background())
	if err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}
	if me.ID != "user-uc-2" {
		t.Fatalf("User.Me() result = %+v, want ID = user-uc-2", me)
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0 (expiresAt is outside the refresh buffer)", refreshCounter.Count())
	}
}

// TestUserCredential_ExpiresAtEmpty_SkipsProactiveRefresh verifies that an
// unset ExpiresAt ("unknown expiry") skips proactive refresh entirely: the
// very first business call must go straight to the network with the
// constructor-supplied token, relying only on 401-triggered refresh.
func TestUserCredential_ExpiresAtEmpty_SkipsProactiveRefresh(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: expiresAt is unknown, so only 401 can trigger a refresh")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-3", map[string]any{
			"id": "user-uc-3",
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "unknown-expiry-token",
		RefreshToken: ucOriginalRefreshToken,
		// ExpiresAt intentionally omitted.
	})

	_, err := uc.User.Me(context.Background())
	if err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0 (unknown expiresAt must skip proactive refresh)", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1", businessCounter.Count())
	}
}

// --- 3: onRefresh timing and failure semantics ------------------------------

// TestUserCredential_OnRefresh_CalledWithNewTokenAndCallerContext verifies
// onRefresh actually receives the refreshed accessToken/expiresAt (not the
// stale ones) and is invoked with the same context.Context the caller passed
// into the business method (not a detached context.Background()).
func TestUserCredential_OnRefresh_CalledWithNewTokenAndCallerContext(t *testing.T) {
	wantExpiresAt := futureExpiresAt(time.Hour)
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-2", map[string]any{
			"accessToken": "onrefresh-new-token",
			"expiresAt":   wantExpiresAt,
		})
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-4", map[string]any{"id": "user-uc-4"})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	type ctxKeyType struct{}
	var ctxKey ctxKeyType
	var onRefreshCalls int64
	var sawAccessToken, sawExpiresAt string
	var sawCtxValue any

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "stale-access-token-2",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(200 * time.Second),
		OnRefresh: func(ctx context.Context, tok qdmp.RefreshedUserToken) error {
			atomic.AddInt64(&onRefreshCalls, 1)
			sawAccessToken = tok.AccessToken
			sawExpiresAt = tok.ExpiresAt
			sawCtxValue = ctx.Value(ctxKey)
			return nil
		},
	})

	ctx := context.WithValue(context.Background(), ctxKey, "caller-marker")
	if _, err := uc.User.Me(ctx); err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}
	if atomic.LoadInt64(&onRefreshCalls) != 1 {
		t.Fatalf("onRefresh called %d times, want exactly 1", onRefreshCalls)
	}
	if sawAccessToken != "onrefresh-new-token" {
		t.Fatalf("onRefresh saw AccessToken = %q, want %q", sawAccessToken, "onrefresh-new-token")
	}
	if sawExpiresAt != wantExpiresAt {
		t.Fatalf("onRefresh saw ExpiresAt = %q, want %q", sawExpiresAt, wantExpiresAt)
	}
	if sawCtxValue != "caller-marker" {
		t.Fatalf("onRefresh's ctx.Value(ctxKey) = %v, want %q (onRefresh must run with the caller's context, not a detached one)", sawCtxValue, "caller-marker")
	}
}

// TestUserCredential_OnRefreshError_FailsCallButKeepsNewToken is the most
// important onRefresh test: onRefresh returning an error must fail the
// triggering business call with that exact error (not swallow it, not
// substitute a different one), the business endpoint must never be reached
// in that same call (the whole call fails on account of onRefresh, not a
// business-layer failure), and — critically — the already-refreshed token
// must NOT be rolled back: a subsequent call, once expiresAt is safely in
// the future, must proceed straight to the business endpoint using the new
// token and must NOT trigger a second refresh.
func TestUserCredential_OnRefreshError_FailsCallButKeepsNewToken(t *testing.T) {
	sentinelErr := errors.New("uc: failed to persist refreshed token")
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-3", map[string]any{
			"accessToken": "refreshed-despite-onrefresh-error",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("access-token"); got != "refreshed-despite-onrefresh-error" {
			t.Fatalf("business call access-token header = %q, want the retained refreshed token %q", got, "refreshed-despite-onrefresh-error")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-5", map[string]any{"id": "user-uc-5"})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "stale-access-token-3",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(100 * time.Second), // forces a refresh on the first call
		OnRefresh: func(ctx context.Context, tok qdmp.RefreshedUserToken) error {
			return sentinelErr
		},
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want onRefresh's error to fail the whole call")
	}
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("User.Me() error = %v, want errors.Is(err, sentinelErr) (onRefresh's error must propagate, not be swallowed or replaced)", err)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
	if businessCounter.Count() != 0 {
		t.Fatalf("business endpoint hit %d times, want 0 (the call must fail on onRefresh's error before ever reaching the business endpoint)", businessCounter.Count())
	}

	// Second call: expiresAt is now far in the future (from the successful
	// refresh above), so this must go straight to the business endpoint
	// using the retained new token, without refreshing again.
	me, err := uc.User.Me(context.Background())
	if err != nil {
		t.Fatalf("second User.Me() error = %v, want nil", err)
	}
	if me.ID != "user-uc-5" {
		t.Fatalf("second User.Me() result = %+v, want ID = user-uc-5", me)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times after the second call, want still exactly 1 (no rollback => no second refresh needed)", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1", businessCounter.Count())
	}
}

// --- 4/5: 401-triggered refresh + retry-exactly-once ------------------------

// TestUserCredential_401Code10006_RefreshesAndRetriesExactlyOnce verifies
// bullets 4 and 5 together, using an exact atomic hit-count assertion (not
// "at least once"): the first business call returns 401/10006, a refresh
// happens, and the retried call (using the new token) succeeds.
func TestUserCredential_401Code10006_RefreshesAndRetriesExactlyOnce(t *testing.T) {
	var businessHits int64
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-4", map[string]any{
			"accessToken": "retried-access-token",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&businessHits, 1)
		if n == 1 {
			if got := r.Header.Get("access-token"); got != "expired-access-token" {
				t.Fatalf("first business call access-token header = %q, want %q", got, "expired-access-token")
			}
			businessEnvelope(w, http.StatusUnauthorized, float64(10006), "access_token 超过有效期", "req-uc-me-401", nil)
			return
		}
		if got := r.Header.Get("access-token"); got != "retried-access-token" {
			t.Fatalf("retried business call access-token header = %q, want %q", got, "retried-access-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-6", map[string]any{"id": "user-uc-6"})
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "expired-access-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour), // outside the buffer: no proactive refresh
	})

	me, err := uc.User.Me(context.Background())
	if err != nil {
		t.Fatalf("User.Me() error = %v, want nil (must succeed after the automatic 401 retry)", err)
	}
	if me.ID != "user-uc-6" {
		t.Fatalf("User.Me() result = %+v, want ID = user-uc-6", me)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
	if got := atomic.LoadInt64(&businessHits); got != 2 {
		t.Fatalf("business endpoint hit %d times, want exactly 2 (original + exactly one retry)", got)
	}
}

// TestUserCredential_401StillFailsAfterRetry_NoSecondRefresh verifies bullet
// 5's "give up after one retry" half: if the retried call still fails, the
// error is returned as-is and there must be no second refresh/retry attempt
// (which would show up as unbounded growth in either hit counter).
func TestUserCredential_401StillFailsAfterRetry_NoSecondRefresh(t *testing.T) {
	var businessHits int64
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-5", map[string]any{
			"accessToken": "still-bad-access-token",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&businessHits, 1)
		businessEnvelope(w, http.StatusUnauthorized, float64(10006), "access_token 超过有效期", "req-uc-me-401-again", nil)
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "expired-access-token-2",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	me, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want the retried call's 401/10006 to surface once the retry also fails")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on failure", me)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10006" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "10006")
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1 (no second refresh attempt after the retry also failed)", refreshCounter.Count())
	}
	if got := atomic.LoadInt64(&businessHits); got != 2 {
		t.Fatalf("business endpoint hit %d times, want exactly 2 (original + exactly one retry, then give up)", got)
	}
}

// --- 6/7: non-retryable failures must never refresh -------------------------

// TestUserCredential_401WrongCode_NoRefresh verifies the narrow, easy-to-get-
// wrong boundary in bullet 6: HTTP 401 alone is not sufficient to trigger a
// refresh — the business code must specifically be 10005 or 10006. A 401
// with an unrelated code (10001) must be returned as-is, untouched.
func TestUserCredential_401WrongCode_NoRefresh(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: 401 with code=10001 is not a retryable auth failure")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusUnauthorized, float64(10001), "code 已使用或已过期", "req-uc-me-7", nil)
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "some-access-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want the 401/10001 error surfaced as-is")
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10001" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "10001")
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1 (no retry for a non-retryable code)", businessCounter.Count())
	}
}

// TestUserCredential_NonAuthBusinessError_NoRefresh verifies a plain business
// failure (HTTP 200, unrelated non-zero code) never triggers a refresh.
func TestUserCredential_NonAuthBusinessError_NoRefresh(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: this is an unrelated business error, not an auth failure")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(20000), "some unrelated business failure", "req-uc-me-8", nil)
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "some-access-token-2",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want the code=20000 business error surfaced")
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1", businessCounter.Count())
	}
}

// TestUserCredential_HTTP5xx_NoRefresh verifies a transport-level 5xx (no
// business envelope at all) never triggers a refresh.
func TestUserCredential_HTTP5xx_NoRefresh(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: a 5xx is not an auth failure")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "some-access-token-3",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a non-nil error for HTTP 500")
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0 (a 5xx must never trigger a refresh)", refreshCounter.Count())
	}
	if businessCounter.Count() != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1 (no retry for a 5xx)", businessCounter.Count())
	}
}

// TestUserCredential_NetworkError_NoRefresh verifies a genuine
// transport/network failure (connection closed mid-request, no HTTP response
// at all — not even a status code) never triggers a refresh either. This is
// distinct from the 5xx case above: there is no parsed response to inspect
// at all here, so a naive "refresh on any error" implementation would be
// caught by this test where the 5xx test alone would not.
func TestUserCredential_NetworkError_NoRefresh(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: a network/transport error is not an auth failure")
	})
	var businessHits int64
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&businessHits, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("response writer does not support hijacking; cannot simulate a network error")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("failed to hijack connection: %v", err)
		}
		conn.Close() // close without ever writing a response: a genuine transport error
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "some-access-token-4",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a non-nil error for a closed connection with no response")
	}
	if refreshCounter.Count() != 0 {
		t.Fatalf("refresh endpoint hit %d times, want 0 (a network error must never trigger a refresh)", refreshCounter.Count())
	}
	if got := atomic.LoadInt64(&businessHits); got != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1 (no retry for a network error)", got)
	}
}

// TestUserCredential_RefreshItselfFails_BusinessNeverRetried verifies bullet
// 7: when /auth/v1/refresh itself fails (e.g. code 10008, expired
// refreshToken), that error must be surfaced to the caller as-is, and the
// original business request must never be retried.
func TestUserCredential_RefreshItselfFails_BusinessNeverRetried(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(10008), "refreshToken 超过有效期", "req-uc-refresh-fail", nil)
	})
	var businessHits int64
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&businessHits, 1)
		businessEnvelope(w, http.StatusUnauthorized, float64(10006), "access_token 超过有效期", "req-uc-me-401-refreshfail", nil)
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "expired-access-token-3",
		RefreshToken: "an-expired-refresh-token",
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	_, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want the refresh failure (code=10008) to surface")
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10008" {
		t.Fatalf("apiErr.Code = %q, want %q (the refresh failure's own code, not the original 401/10006)", apiErr.Code, "10008")
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
	if got := atomic.LoadInt64(&businessHits); got != 1 {
		t.Fatalf("business endpoint hit %d times, want exactly 1 (no retry: the refresh itself failed, so the business request must not be retried)", got)
	}
}

// --- 8: concurrent single-flight ---------------------------------------------

// TestUserCredential_ConcurrentCalls_RefreshSingleFlight drives N goroutines
// to call User.Me concurrently on the same *user* credential, all starting
// from a cold, within-buffer expiresAt so every one of them independently
// "wants" a refresh. Without a single-flight guard, this would issue N
// refresh calls; the mock refresh handler sleeps briefly to widen the race
// window a naive implementation would otherwise get lucky and miss. Must
// pass under go test -race.
func TestUserCredential_ConcurrentCalls_RefreshSingleFlight(t *testing.T) {
	const goroutines = 20
	var refreshHits int64
	var businessHits int64

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/v1/refresh", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&refreshHits, 1)
		time.Sleep(50 * time.Millisecond) // widen the race window
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-concurrent", map[string]any{
			"accessToken": "single-flighted-token",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&businessHits, 1)
		if got := r.Header.Get("access-token"); got != "single-flighted-token" {
			t.Errorf("business call access-token header = %q, want %q", got, "single-flighted-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-concurrent", map[string]any{"id": "user-uc-concurrent"})
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "cold-access-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(50 * time.Second), // well within the 300s buffer
	})

	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = uc.User.Me(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: User.Me() error = %v, want nil", i, err)
		}
	}
	if got := atomic.LoadInt64(&refreshHits); got != 1 {
		t.Fatalf("refresh endpoint hit %d times by %d concurrent callers, want exactly 1 (single-flight failed)", got, goroutines)
	}
	if got := atomic.LoadInt64(&businessHits); got != goroutines {
		t.Fatalf("business endpoint hit %d times, want exactly %d (one per caller, after the shared refresh completed)", got, goroutines)
	}
}

// --- 9: read-only snapshot ---------------------------------------------------

// TestUserCredential_Credential_ReflectsStateAndIsIndependentCopy verifies
// bullet 9 end-to-end: Credential() must report the live internal state
// (not a stale constructor-time copy) after a refresh, and mutating the
// returned value must never leak back into the client's internal state.
func TestUserCredential_Credential_ReflectsStateAndIsIndependentCopy(t *testing.T) {
	wantExpiresAt := futureExpiresAt(time.Hour)
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-6", map[string]any{
			"accessToken": "credential-snapshot-token",
			"expiresAt":   wantExpiresAt,
		})
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("access-token"); got != "credential-snapshot-token" {
			t.Fatalf("business call access-token header = %q, want %q (tampering with the snapshot must not affect internal state)", got, "credential-snapshot-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-me-9", map[string]any{"id": "user-uc-9"})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "pre-refresh-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(100 * time.Second), // forces a refresh on the first call
	})

	if _, err := uc.User.Me(context.Background()); err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}

	snap := uc.Credential()
	if got := snapshotStringField(t, snap, "AccessToken"); got != "credential-snapshot-token" {
		t.Fatalf("Credential().AccessToken = %q, want the refreshed token %q (Credential() must reflect live state)", got, "credential-snapshot-token")
	}
	if got := snapshotStringField(t, snap, "ExpiresAt"); got != wantExpiresAt {
		t.Fatalf("Credential().ExpiresAt = %q, want %q", got, wantExpiresAt)
	}

	// Attempt to tamper with the snapshot in place (only has an effect if
	// Credential() handed back a pointer into the real internal state,
	// which would be the bug).
	tryTamperStringField(snap, "AccessToken", "tampered-value-should-not-stick")

	snap2 := uc.Credential()
	if got := snapshotStringField(t, snap2, "AccessToken"); got != "credential-snapshot-token" {
		t.Fatalf("Credential().AccessToken after tampering with a prior snapshot = %q, want it unaffected (%q) — Credential() must return an independent copy", got, "credential-snapshot-token")
	}

	// And a subsequent business call must still use the real, untampered
	// token (expiresAt is far in the future now, so no new refresh fires).
	if _, err := uc.User.Me(context.Background()); err != nil {
		t.Fatalf("second User.Me() error = %v, want nil", err)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
}

// --- local validation (§2.2): must fail before any HTTP request -------------

// TestUserCredential_EmptyAccessToken_LocalErrorNoRequest verifies an empty
// AccessToken is rejected locally, reusing the same sentinel already used by
// WithAccessToken("") elsewhere in this suite (see TestUserMe_MissingAccessToken_NoRequestSent),
// and that this happens before any HTTP request (refresh or business) is
// sent.
func TestUserCredential_EmptyAccessToken_LocalErrorNoRequest(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: local validation must fail first")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("business endpoint should never have been called: local validation must fail first")
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	me, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for an empty AccessToken")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if refreshCounter.Count() != 0 || businessCounter.Count() != 0 {
		t.Fatalf("refresh hits = %d, business hits = %d, want 0/0 (must fail locally before any HTTP request)", refreshCounter.Count(), businessCounter.Count())
	}
}

// TestUserCredential_MissingRefreshToken_LocalErrorNoRequest verifies an
// empty RefreshToken is also rejected locally (it is required per
// CONTRACT.md §2.2: without it, no auto-refresh is possible at all), and
// that this too happens before any HTTP request.
func TestUserCredential_MissingRefreshToken_LocalErrorNoRequest(t *testing.T) {
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint should never have been called: local validation must fail first")
	})
	businessCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("business endpoint should never have been called: local validation must fail first")
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.Handle("/user/v1/me", businessCounter)
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "some-access-token-5",
		RefreshToken: "",
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	me, err := uc.User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a missing RefreshToken")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if refreshCounter.Count() != 0 || businessCounter.Count() != 0 {
		t.Fatalf("refresh hits = %d, business hits = %d, want 0/0 (must fail locally before any HTTP request)", refreshCounter.Count(), businessCounter.Count())
	}
}

// --- 10: genai gets the same treatment ---------------------------------------

// TestUserCredential_GenAI_401Retry verifies bullet 10: the genai group
// (which uses the x-openapi-access-token / x-openapi-app-id header pair
// instead of the standard scheme's access-token header) gets the exact same
// automatic-refresh-then-retry-exactly-once treatment as User.Me.
func TestUserCredential_GenAI_401Retry(t *testing.T) {
	var businessHits int64
	refreshCounter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-refresh-genai", map[string]any{
			"accessToken": "genai-retried-token",
			"expiresAt":   futureExpiresAt(time.Hour),
		})
	})
	mux := http.NewServeMux()
	mux.Handle("/auth/v1/refresh", refreshCounter)
	mux.HandleFunc("/genai/v1/generate", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&businessHits, 1)
		if n == 1 {
			if got := r.Header.Get("x-openapi-access-token"); got != "genai-expired-token" {
				t.Fatalf("first genai call x-openapi-access-token header = %q, want %q", got, "genai-expired-token")
			}
			businessEnvelope(w, http.StatusUnauthorized, float64(10006), "access_token 超过有效期", "req-uc-genai-401", nil)
			return
		}
		if got := r.Header.Get("x-openapi-access-token"); got != "genai-retried-token" {
			t.Fatalf("retried genai call x-openapi-access-token header = %q, want %q", got, "genai-retried-token")
		}
		if got := r.Header.Get("x-openapi-app-id"); got != "test-app-id" {
			t.Fatalf("retried genai call x-openapi-app-id header = %q, want %q", got, "test-app-id")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-uc-genai-ok", map[string]any{"id": "genai-task-uc-1"})
	})
	srv := startServer(t, mux)
	client := newTestClient(t, srv.URL)

	uc := client.WithUserCredential(qdmp.UserCredentialOptions{
		AccessToken:  "genai-expired-token",
		RefreshToken: ucOriginalRefreshToken,
		ExpiresAt:    futureExpiresAt(time.Hour),
	})

	res, err := uc.GenAI.Generate(context.Background(), generated.GenaiGenerateJSONBody{
		Model: "google/gemini-3.1-flash-image-preview",
	})
	if err != nil {
		t.Fatalf("GenAI.Generate() error = %v, want nil (must succeed after the automatic 401 retry)", err)
	}
	if res.ID != "genai-task-uc-1" {
		t.Fatalf("GenAI.Generate() result = %+v, want ID = genai-task-uc-1", res)
	}
	if refreshCounter.Count() != 1 {
		t.Fatalf("refresh endpoint hit %d times, want exactly 1", refreshCounter.Count())
	}
	if got := atomic.LoadInt64(&businessHits); got != 2 {
		t.Fatalf("genai endpoint hit %d times, want exactly 2 (original + exactly one retry)", got)
	}
}
