package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// UserAccessTokenResult (auth.go ~line 22) declares RefreshToken and OpenID
// fields, but GetUserAccessToken (auth.go ~line 188) only ever checks
// result.AccessToken == "". A response reporting code=0 with a populated
// accessToken/expiresAt but a missing or empty refreshToken/openId must
// still surface as an error: the caller has no usable user-level credential
// without a refreshToken to renew it later, or an openId to know whose
// credential it is.

import (
	"context"
	"net/http"
	"testing"
)

// TestAuthGetUserAccessToken_MissingRefreshTokenRejected drives a response that
// omits the refreshToken field entirely (as opposed to sending it empty).
func TestAuthGetUserAccessToken_MissingRefreshTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-norefresh-1", map[string]any{
			"accessToken": "user-level-access-token",
			"expiresAt":   "1785508950",
			"openId":      "user-openid-1",
			// refreshToken deliberately omitted.
		})
	}))
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("GetUserAccessToken() error = nil, want an error because refreshToken is missing from the response")
	}
	if cred != nil {
		t.Fatalf("GetUserAccessToken() result = %+v, want nil on failure", cred)
	}
}

// TestAuthGetUserAccessToken_EmptyRefreshTokenRejected is the same failure mode
// but with an explicit empty string rather than an omitted field.
func TestAuthGetUserAccessToken_EmptyRefreshTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-norefresh-2", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "",
			"expiresAt":    "1785508950",
			"openId":       "user-openid-1",
		})
	}))
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("GetUserAccessToken() error = nil, want an error because refreshToken is an empty string")
	}
	if cred != nil {
		t.Fatalf("GetUserAccessToken() result = %+v, want nil on failure", cred)
	}
}

// TestAuthGetUserAccessToken_MissingOpenIDRejected drives a response that omits
// the openId field entirely.
func TestAuthGetUserAccessToken_MissingOpenIDRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-noopenid-1", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "user-level-refresh-token",
			"expiresAt":    "1785508950",
			// openId deliberately omitted.
		})
	}))
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("GetUserAccessToken() error = nil, want an error because openId is missing from the response")
	}
	if cred != nil {
		t.Fatalf("GetUserAccessToken() result = %+v, want nil on failure", cred)
	}
}

// TestAuthGetUserAccessToken_EmptyOpenIDRejected is the same failure mode but with
// an explicit empty string rather than an omitted field.
func TestAuthGetUserAccessToken_EmptyOpenIDRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-noopenid-2", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "user-level-refresh-token",
			"expiresAt":    "1785508950",
			"openId":       "",
		})
	}))
	client := newTestClient(t, srv.URL)

	cred, err := client.Auth.GetUserAccessToken(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("GetUserAccessToken() error = nil, want an error because openId is an empty string")
	}
	if cred != nil {
		t.Fatalf("GetUserAccessToken() result = %+v, want nil on failure", cred)
	}
}
