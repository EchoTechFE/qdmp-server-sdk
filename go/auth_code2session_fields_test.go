package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// Code2SessionResult (auth.go ~line 22) declares RefreshToken and OpenID
// fields, but Code2Session (auth.go ~line 188) only ever checks
// result.AccessToken == "". A response reporting code=0 with a populated
// accessToken/expiresAt but a missing or empty refreshToken/openId must
// still surface as an error: the caller has no usable user-level session
// without a refreshToken to renew it later, or an openId to know whose
// session it is.

import (
	"context"
	"net/http"
	"testing"
)

// TestAuthCode2Session_MissingRefreshTokenRejected drives a response that
// omits the refreshToken field entirely (as opposed to sending it empty).
func TestAuthCode2Session_MissingRefreshTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-norefresh-1", map[string]any{
			"accessToken": "user-level-access-token",
			"expiresAt":   "1785508950",
			"openId":      "user-openid-1",
			// refreshToken deliberately omitted.
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because refreshToken is missing from the response")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthCode2Session_EmptyRefreshTokenRejected is the same failure mode
// but with an explicit empty string rather than an omitted field.
func TestAuthCode2Session_EmptyRefreshTokenRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-norefresh-2", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "",
			"expiresAt":    "1785508950",
			"openId":       "user-openid-1",
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because refreshToken is an empty string")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthCode2Session_MissingOpenIDRejected drives a response that omits
// the openId field entirely.
func TestAuthCode2Session_MissingOpenIDRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-noopenid-1", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "user-level-refresh-token",
			"expiresAt":    "1785508950",
			// openId deliberately omitted.
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because openId is missing from the response")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}

// TestAuthCode2Session_EmptyOpenIDRejected is the same failure mode but with
// an explicit empty string rather than an omitted field.
func TestAuthCode2Session_EmptyOpenIDRejected(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-noopenid-2", map[string]any{
			"accessToken":  "user-level-access-token",
			"refreshToken": "user-level-refresh-token",
			"expiresAt":    "1785508950",
			"openId":       "",
		})
	}))
	client := newTestClient(t, srv.URL)

	sess, err := client.Auth.Code2Session(context.Background(), "some-code")
	if err == nil {
		t.Fatalf("Code2Session() error = nil, want an error because openId is an empty string")
	}
	if sess != nil {
		t.Fatalf("Code2Session() result = %+v, want nil on failure", sess)
	}
}
