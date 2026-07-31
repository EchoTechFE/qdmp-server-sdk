package qdmp_test

// Contract exercised by this file:
//
//	asUser := client.WithAccessToken(token)   // derived client, no functional options
//	res, err := asUser.User.Me(ctx)
//
// qdmp.UserMeResult{ID, Nickname, Avatar string; IdentityTags, InterestTags []map[string]any}
// user.me has x-qdmp-token-required=true (shared/generated/route-meta.json) — an
// empty/missing accessToken must fail locally, before any HTTP request.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestUserMe_Success verifies the happy path against the normal business
// envelope {code, message, requestId, data} and that both the access-token
// and the (default) x-echo-qdmp-version headers required by the "standard"
// authScheme are actually sent.
func TestUserMe_Success(t *testing.T) {
	const token = "user-access-token-xyz"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/v1/me" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("access-token"); got != token {
			t.Fatalf("access-token header = %q, want %q", got, token)
		}
		if got := r.Header.Get("x-echo-qdmp-version"); got != "1.0" {
			t.Fatalf("x-echo-qdmp-version header = %q, want default %q", got, "1.0")
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-user-1", map[string]any{
			"id":           "user-123",
			"nickname":     "小千",
			"avatar":       "https://example.com/avatar.png",
			"identityTags": []map[string]any{{"name": "collector"}},
			"interestTags": []map[string]any{{"name": "figures"}},
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken(token).User.Me(context.Background())
	if err != nil {
		t.Fatalf("User.Me() error = %v, want nil", err)
	}
	if me.ID != "user-123" || me.Nickname != "小千" || me.Avatar != "https://example.com/avatar.png" {
		t.Fatalf("unexpected result: %+v", me)
	}
	if len(me.IdentityTags) != 1 || len(me.InterestTags) != 1 {
		t.Fatalf("expected 1 identity tag and 1 interest tag, got %+v", me)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestUserMe_MissingAccessToken_NoRequestSent is the core "fail fast,
// locally" requirement: user.me is tokenRequired, so calling it with an
// empty token must never reach the network.
func TestUserMe_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken("").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for empty accessToken")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0 (missing token must short-circuit locally)", counter.Count())
	}
}

// TestUserMe_HTTP401_Code10005 encodes a second confirmed-by-capture fact:
// an invalid/revoked access token comes back as a genuine HTTP 401 with
// business code 10005 (undocumented upstream, discovered via real testing).
func TestUserMe_HTTP401_Code10005(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusUnauthorized, float64(10005), "access_token 格式错误或已吊销", "req-user-401", nil)
	}))
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken("a-revoked-token").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error for HTTP 401 / code=10005")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on failure", me)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10005" {
		t.Fatalf("apiErr.Code = %q, want %q", apiErr.Code, "10005")
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("apiErr.HTTPStatus = %d, want %d", apiErr.HTTPStatus, http.StatusUnauthorized)
	}
}

// TestUserMe_ErrorDoesNotLeakAccessToken asserts the SDK's own error value
// never echoes the caller's accessToken back, even in a failure path where
// the token was clearly sent to (and rejected by) the server.
func TestUserMe_ErrorDoesNotLeakAccessToken(t *testing.T) {
	const token = "super-secret-user-token-should-not-leak"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("access-token") != token {
			t.Fatalf("server never received the expected access-token header")
		}
		businessEnvelope(w, http.StatusUnauthorized, float64(10005), "access_token 格式错误或已吊销", "req-user-402", nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("expected an error")
	}
	if got := err.Error(); strings.Contains(got, token) {
		t.Fatalf("err.Error() leaks the accessToken: %q", got)
	}
}
