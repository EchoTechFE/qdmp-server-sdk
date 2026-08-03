package qdmp_test

// Contract exercised by this file (see doc.go — implemented in a later
// phase, not here):
//
//	client, err := qdmp.NewClient(qdmp.ClientOptions{AppID, AppSecret, BaseURL})
//	app, err   := client.Auth.GetAppAccessToken(ctx)          // app-level, cached + single-flight
//	cred, err  := client.Auth.GetUserAccessToken(ctx, code)   // one-shot, never cached
//	res, err   := client.Auth.RefreshToken(ctx, refreshToken) // one-shot, never cached
//
// qdmp.AppAccessTokenResult{AccessToken, ExpiresAt, RefreshToken, OpenID string}
// qdmp.UserAccessTokenResult{AccessToken, RefreshToken, ExpiresAt, OpenID string}
// qdmp.RefreshTokenResult{AccessToken, ExpiresAt string}
// *qdmp.QdmpApiError{Code, Message, RequestID string; HTTPStatus int}

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// TestAuthGetAppAccessToken_Success verifies the happy path for the
// CLIENT_CREDENTIALS exchange: correct request shape in, correct token out.
func TestAuthGetAppAccessToken_Success(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["grantType"] != "CLIENT_CREDENTIALS" {
			t.Fatalf("grantType = %v, want CLIENT_CREDENTIALS", body["grantType"])
		}
		if body["appId"] != "test-app-id" || body["appSecret"] != "test-app-secret-do-not-leak" {
			t.Fatalf("unexpected appId/appSecret in request body: %+v", body)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{
			"accessToken": "app-level-access-token",
			// expiresAt is deliberately a *string* on the wire — this is a
			// real, confirmed fact (see shared/openapi.yaml), not a guess.
			// Computed relative to time.Now() (rather than a hardcoded
			// absolute timestamp) so this test keeps exercising "a genuinely
			// fresh token" indefinitely, instead of silently starting to
			// fail once real wall-clock time catches up to a fixed constant.
			"expiresAt": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	appToken, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAppAccessToken() error = %v, want nil", err)
	}
	if appToken.AccessToken != "app-level-access-token" {
		t.Fatalf("GetAppAccessToken().AccessToken = %q, want %q", appToken.AccessToken, "app-level-access-token")
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestAuthGetAppAccessToken_CachesUntilExpiry verifies the app-level token is
// cached: a second immediate call must not trigger a second HTTP exchange.
func TestAuthGetAppAccessToken_CachesUntilExpiry(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(0), "ok", "req-1", map[string]any{
			"accessToken": "cached-token",
			"expiresAt":   "9999999999",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)
	ctx := context.Background()

	first, err := client.Auth.GetAppAccessToken(ctx)
	if err != nil {
		t.Fatalf("first GetAppAccessToken() error = %v", err)
	}
	second, err := client.Auth.GetAppAccessToken(ctx)
	if err != nil {
		t.Fatalf("second GetAppAccessToken() error = %v", err)
	}
	if *first != *second {
		t.Fatalf("cached credential changed across calls: %+v != %+v", first, second)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times across two calls, want 1 (token should be cached)", counter.Count())
	}
}

// TestAuthGetAppAccessToken_ConcurrentSingleFlight simulates N goroutines all
// racing to fetch the app-level token from a cold cache. Without a
// single-flight guard this would hit the token endpoint N times and risk
// rate-limiting/thundering-herd against the real qdmp gateway; the SDK must
// collapse them into exactly one outbound exchange.
func TestAuthGetAppAccessToken_ConcurrentSingleFlight(t *testing.T) {
	const goroutines = 50
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-1", map[string]any{
			"accessToken": "single-flight-token",
			"expiresAt":   "9999999999",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make([]*qdmp.AppAccessTokenResult, goroutines)
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = client.Auth.GetAppAccessToken(ctx)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: GetAppAccessToken() error = %v", i, err)
		}
		if results[i].AccessToken != "single-flight-token" {
			t.Fatalf("goroutine %d: token = %q, want %q", i, results[i].AccessToken, "single-flight-token")
		}
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times by %d concurrent callers, want exactly 1 (single-flight failed)", counter.Count(), goroutines)
	}
}

// TestAuthGetUserAccessToken_Success verifies the AUTHORIZATION_CODE exchange and,
// critically, that expiresAt round-trips as a *string* end-to-end — this
// repository has a confirmed-by-capture rule that int64/uint64 wire values
// are strings, and a naive numeric-conversion implementation would corrupt
// this value silently.
func TestAuthGetUserAccessToken_Success(t *testing.T) {
	// Computed relative to time.Now() (rather than a hardcoded absolute
	// timestamp) so this test keeps exercising "a genuinely fresh credential"
	// indefinitely, instead of silently starting to fail once real
	// wall-clock time catches up to a fixed constant (as happened once
	// already with a previous hardcoded value here).
	wantExpiresAt := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["grantType"] != "AUTHORIZATION_CODE" {
			t.Fatalf("grantType = %v, want AUTHORIZATION_CODE", body["grantType"])
		}
		if body["code"] != "auth-code-abc" {
			t.Fatalf("code = %v, want auth-code-abc", body["code"])
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-2", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "user-level-refresh-token",
			"expiresAt":    wantExpiresAt,
			"openId":       "user-openid-1",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "auth-code-abc")
	if err != nil {
		t.Fatalf("GetUserAccessToken() error = %v, want nil", err)
	}
	if cred.AccessToken != "user-level-access-token" {
		t.Fatalf("AccessToken = %q", cred.AccessToken)
	}
	if cred.RefreshToken != "user-level-refresh-token" {
		t.Fatalf("RefreshToken = %q", cred.RefreshToken)
	}
	if cred.ExpiresAt != wantExpiresAt {
		t.Fatalf("ExpiresAt = %q, want the exact wire string %q (must not be converted to a number)", cred.ExpiresAt, wantExpiresAt)
	}
	if cred.OpenID != "user-openid-1" {
		t.Fatalf("OpenID = %q", cred.OpenID)
	}
}

// TestAuthRefreshToken_HTTP200BusinessFailure encodes the single most
// dangerous confirmed fact in this API: HTTP 200 does not mean success.
// A refreshToken that has expired comes back as HTTP 200 with code=10008.
// A naive "status == 200 => success" implementation would silently return
// a bogus/empty token here instead of an error.
func TestAuthRefreshToken_HTTP200BusinessFailure(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(10008), "refreshToken 超过有效期", "req-3", nil)
	}))
	client := newTestClient(t, srv.URL)

	result, err := client.Auth.RefreshToken(context.Background(), "an-expired-refresh-token")
	if err == nil {
		t.Fatalf("RefreshToken() error = nil, want an error because code=10008 (HTTP 200 does not imply success)")
	}
	if result != nil {
		t.Fatalf("RefreshToken() result = %+v, want nil on failure", result)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10008" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "10008")
	}
	if apiErr.HTTPStatus != http.StatusOK {
		t.Fatalf("apiErr.HTTPStatus = %d, want %d (the transport-level status was 200 even though the call failed)", apiErr.HTTPStatus, http.StatusOK)
	}
}

// TestAuthGetAppAccessToken_ErrorDoesNotLeakAppSecret drives a real failure
// through the exchange (10002: 密钥不匹配) and asserts the resulting error's
// message never echoes the appSecret that was sent on the wire, even though
// the mock server itself did receive it (sanity-checked below).
func TestAuthGetAppAccessToken_ErrorDoesNotLeakAppSecret(t *testing.T) {
	const secret = "test-app-secret-do-not-leak"
	var sawSecretOnWire bool
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		sawSecretOnWire = strings.Contains(string(raw), secret)
		businessEnvelope(w, http.StatusOK, float64(10002), "密钥不匹配", "req-4", nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.Auth.GetAppAccessToken(context.Background())
	if err == nil {
		t.Fatalf("GetAppAccessToken() error = nil, want an error because code=10002")
	}
	if !sawSecretOnWire {
		t.Fatalf("test setup broken: mock server never actually received the appSecret")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("err.Error() leaks appSecret: %q", err.Error())
	}
}

// TestAuthGetAppAccessToken_ProactivelyRefreshesWithinBuffer verifies the
// tokenRefreshBufferSeconds (300s) early-refresh boundary in cachedValid:
// a token that has not yet technically expired, but sits within the 300s
// buffer, must still trigger a fresh exchange rather than being served from
// cache — the buffer exists precisely so callers never observe a token that
// expires mid-flight.
func TestAuthGetAppAccessToken_ProactivelyRefreshesWithinBuffer(t *testing.T) {
	now := time.Now().Unix()
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-buffer-1", map[string]any{
			"accessToken": "freshly-refreshed-token",
			"expiresAt":   "9999999999",
		})
	})
	srv := startServer(t, counter)
	// A caller-supplied TokenStore, seeded directly (bypassing any HTTP
	// exchange) with a token that expires in 250s: inside the 300s buffer,
	// so GetAppAccessToken must treat it as not-cached-valid and refresh, even
	// though time.Now().Unix() < that expiresAt.
	store := &customTokenStore{}
	store.Set(qdmp.TokenEntry{AccessToken: "stale-within-buffer-token", ExpiresAt: now + 250})
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:      "test-app-id",
		AppSecret:  "test-app-secret-do-not-leak",
		BaseURL:    srv.URL,
		TokenStore: store,
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() error = %v", err)
	}

	appToken, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAppAccessToken() error = %v, want nil", err)
	}
	if appToken.AccessToken != "freshly-refreshed-token" {
		t.Fatalf("GetAppAccessToken().AccessToken = %q, want the proactively refreshed token %q, not the stale within-buffer one", appToken.AccessToken, "freshly-refreshed-token")
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want exactly 1 (proactive refresh must happen)", counter.Count())
	}
}

// TestAuthGetAppAccessToken_CachedOutsideBuffer is the complementary boundary
// case: a token expiring just outside the 300s buffer must be served from
// cache with zero HTTP calls.
func TestAuthGetAppAccessToken_CachedOutsideBuffer(t *testing.T) {
	now := time.Now().Unix()
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called: token is valid outside the refresh buffer")
	})
	srv := startServer(t, counter)
	store := &customTokenStore{}
	store.Set(qdmp.TokenEntry{AccessToken: "still-fresh-token", ExpiresAt: now + 301})
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:      "test-app-id",
		AppSecret:  "test-app-secret-do-not-leak",
		BaseURL:    srv.URL,
		TokenStore: store,
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() error = %v", err)
	}

	appToken, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAppAccessToken() error = %v, want nil", err)
	}
	if appToken.AccessToken != "still-fresh-token" {
		t.Fatalf("GetAppAccessToken().AccessToken = %q, want the cached token %q", appToken.AccessToken, "still-fresh-token")
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0 (token is outside the refresh buffer)", counter.Count())
	}
}

// TestAuthRefreshToken_Success is RefreshToken's happy path counterpart to
// TestAuthRefreshToken_HTTP200BusinessFailure above (which only covers the
// failure path).
func TestAuthRefreshToken_Success(t *testing.T) {
	// See TestAuthGetUserAccessToken_Success for why this is computed relative to
	// time.Now() instead of a hardcoded absolute timestamp.
	wantExpiresAt := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/refresh" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["refreshToken"] != "a-valid-refresh-token" {
			t.Fatalf("refreshToken = %v, want %q", body["refreshToken"], "a-valid-refresh-token")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-refresh-1", map[string]any{
			"accessToken": "renewed-access-token",
			"expiresAt":   wantExpiresAt,
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	result, err := client.Auth.RefreshToken(context.Background(), "a-valid-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v, want nil", err)
	}
	if result.AccessToken != "renewed-access-token" {
		t.Fatalf("AccessToken = %q, want %q", result.AccessToken, "renewed-access-token")
	}
	if result.ExpiresAt != wantExpiresAt {
		t.Fatalf("ExpiresAt = %q, want the exact wire string %q", result.ExpiresAt, wantExpiresAt)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestAuthGetUserAccessToken_Failure is GetUserAccessToken's failure-path counterpart to
// TestAuthGetUserAccessToken_Success above (which only covers the happy path): a
// consumed/invalid AUTHORIZATION_CODE must surface as a *QdmpApiError, never
// a zero-value result mistaken for success.
func TestAuthGetUserAccessToken_Failure(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(10001), "code 已使用或已过期", "req-user-token-fail", nil)
	}))
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "already-consumed-code")
	if err == nil {
		t.Fatalf("GetUserAccessToken() error = nil, want an error because code=10001")
	}
	if cred != nil {
		t.Fatalf("GetUserAccessToken() result = %+v, want nil on failure", cred)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10001" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "10001")
	}
}

// TestNewClient_CustomQdmpVersion verifies ClientOptions.QdmpVersion
// overrides the default "1.0" x-echo-qdmp-version header sent on "standard"
// authScheme requests (auth.* endpoints are noAuth and never send it; a
// business endpoint is used here instead).
func TestNewClient_CustomQdmpVersion(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-echo-qdmp-version"); got != "2.5" {
			t.Fatalf("x-echo-qdmp-version header = %q, want the configured override %q", got, "2.5")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-version-1", map[string]any{
			"island": map[string]any{"id": "island-1", "name": "n", "image": "", "joined": false},
		})
	})
	srv := startServer(t, counter)
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:       "test-app-id",
		AppSecret:   "test-app-secret-do-not-leak",
		BaseURL:     srv.URL,
		QdmpVersion: "2.5",
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() error = %v", err)
	}

	if _, err := client.WithAccessToken("some-token").Island.Detail(context.Background(), generated.IslandDetailParams{Id: "island-1"}); err != nil {
		t.Fatalf("Island.Detail() error = %v, want nil", err)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// customTokenStore is a minimal TokenStore implementation distinct from the
// SDK's built-in memoryTokenStore, used to verify ClientOptions.TokenStore
// injection is actually honored (as opposed to NewClient silently always
// using its own default store regardless of what was passed in).
type customTokenStore struct {
	mu      sync.Mutex
	entry   qdmp.TokenEntry
	has     bool
	getHits int
	setHits int
}

func (s *customTokenStore) Get() (qdmp.TokenEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getHits++
	return s.entry, s.has
}

func (s *customTokenStore) Set(entry qdmp.TokenEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setHits++
	s.entry = entry
	s.has = true
}

func (s *customTokenStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entry = qdmp.TokenEntry{}
	s.has = false
}

// TestNewClient_CustomTokenStore verifies GetAppAccessToken actually reads from
// and writes to a caller-supplied TokenStore (the multi-instance-deployment
// use case documented on the TokenStore type), not just the built-in
// in-process default.
func TestNewClient_CustomTokenStore(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-store-1", map[string]any{
			"accessToken": "custom-store-token",
			"expiresAt":   "9999999999",
		})
	})
	srv := startServer(t, counter)
	store := &customTokenStore{}
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:      "test-app-id",
		AppSecret:  "test-app-secret-do-not-leak",
		BaseURL:    srv.URL,
		TokenStore: store,
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() error = %v", err)
	}

	appToken, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAppAccessToken() error = %v, want nil", err)
	}
	if appToken.AccessToken != "custom-store-token" {
		t.Fatalf("GetAppAccessToken().AccessToken = %q, want %q", appToken.AccessToken, "custom-store-token")
	}
	if store.setHits != 1 {
		t.Fatalf("custom store Set() called %d times, want 1 (the exchange result must be written through the injected store)", store.setHits)
	}

	// A second call must be served from the injected store's cache, not a
	// fresh exchange.
	if _, err := client.Auth.GetAppAccessToken(context.Background()); err != nil {
		t.Fatalf("second GetAppAccessToken() error = %v", err)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times across two calls, want 1 (custom store must be consulted for caching)", counter.Count())
	}
}

// TestAuthGetAppAccessToken_CacheHitIsAsCompleteAsFreshExchange pins down
// CONTRACT.md §1.1's core requirement: the app-level credential is returned
// whole (accessToken + expiresAt + refreshToken + openId), and the call
// served from the TokenStore cache returns exactly the same object as the
// call that performed the exchange. Caching only accessToken/expiresAt — the
// shape this replaced — would make the second call silently drop
// refreshToken, so a caller's behavior would depend on whether it happened
// to be the one that triggered the exchange.
//
// The mock mirrors the captured CLIENT_CREDENTIALS response: a genuinely
// non-empty refreshToken and an openId that is the empty string (an
// app-level credential belongs to no user), neither of which may be rejected
// as invalid.
func TestAuthGetAppAccessToken_CacheHitIsAsCompleteAsFreshExchange(t *testing.T) {
	wantExpiresAt := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-app-complete-1", map[string]any{
			"accessToken":  "app-complete-access-token",
			"expiresAt":    wantExpiresAt,
			"refreshToken": "app-complete-refresh-token",
			"openId":       "",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	fresh, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("first GetAppAccessToken() error = %v, want nil (an empty openId must not be rejected)", err)
	}
	want := qdmp.AppAccessTokenResult{
		AccessToken:  "app-complete-access-token",
		ExpiresAt:    wantExpiresAt,
		RefreshToken: "app-complete-refresh-token",
		OpenID:       "",
	}
	if *fresh != want {
		t.Fatalf("freshly exchanged credential = %+v, want %+v", *fresh, want)
	}

	cached, err := client.Auth.GetAppAccessToken(context.Background())
	if err != nil {
		t.Fatalf("second GetAppAccessToken() error = %v, want nil", err)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times across two calls, want 1 (the second call must come from the cache)", counter.Count())
	}
	if *cached != *fresh {
		t.Fatalf("cache-hit credential = %+v, want it identical to the freshly exchanged one %+v", *cached, *fresh)
	}
}
