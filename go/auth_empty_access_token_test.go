package qdmp_test

// Contract exercised by this file (not yet implemented — see the codex
// review round1 fix task): AuthService.exchangeClientCredentials (used by
// GetAccessToken), Code2Session, and RefreshToken currently only validate
// that expiresAt parses as an integer; none of them validate that
// accessToken itself is non-empty. A malformed-but-code=0 response
// (accessToken: "") must be rejected as an error, not treated as a
// successful exchange and returned/cached as if it were a real token.

import (
	"context"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestAuthGetAccessToken_EmptyAccessTokenRejected drives the app-level
// CLIENT_CREDENTIALS exchange (GetAccessToken -> exchangeClientCredentials)
// through a response that reports success (code=0) with a valid expiresAt
// but an empty accessToken. The SDK must return a non-nil error and must
// not write the empty token through to the TokenStore (an empty "cached"
// token would be indistinguishable from "no token" on the next read, or
// worse, would be handed to a caller as if it were usable).
func TestAuthGetAccessToken_EmptyAccessTokenRejected(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-empty-token-1", map[string]any{
			"accessToken": "",
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

	token, err := client.Auth.GetAccessToken(context.Background())
	if err == nil {
		t.Fatalf("GetAccessToken() error = nil, want an error because accessToken is empty despite code=0")
	}
	if token != "" {
		t.Fatalf("GetAccessToken() = %q, want the zero value on failure", token)
	}
	if store.setHits != 0 {
		t.Fatalf("TokenStore.Set() called %d times, want 0 (an empty accessToken must never be cached)", store.setHits)
	}
}

// TestAuthCode2Session_EmptyAccessTokenRejected is Code2Session's
// counterpart: a response reporting code=0 with all other fields populated
// but accessToken empty must still surface as an error, not a
// *Code2SessionResult carrying an unusable empty AccessToken.
func TestAuthCode2Session_EmptyAccessTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-empty-token-2", map[string]any{
			"accessToken":  "",
			"refreshToken": "some-refresh-token",
			"expiresAt":    "1785508950",
			"openId":       "user-openid-1",
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because accessToken is empty despite code=0")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthRefreshToken_EmptyAccessTokenRejected is RefreshToken's
// counterpart to the two tests above.
func TestAuthRefreshToken_EmptyAccessTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-empty-token-3", map[string]any{
			"accessToken": "",
			"expiresAt":   "1785508950",
		})
	}))
	client := newTestClient(t, srv.URL)

	result, err := client.Auth.RefreshToken(context.Background(), "a-valid-refresh-token")
	if err == nil {
		t.Fatalf("RefreshToken() error = nil, want an error because accessToken is empty despite code=0")
	}
	if result != nil {
		t.Fatalf("RefreshToken() result = %+v, want nil on failure", result)
	}
}
