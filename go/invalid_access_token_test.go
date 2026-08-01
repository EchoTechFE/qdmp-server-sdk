package qdmp_test

// Contract exercised by this file (not yet implemented — see the codex
// review round1 fix task): requireAccessToken (go/client.go) currently only
// rejects an empty access token. The fix task adds rejection of any access
// token containing a control character (byte <= 0x1F, or == 0x7F) — most
// importantly CR/LF, which (if ever allowed through) would enable HTTP
// header/request-splitting via the "access-token" header value. This must
// be caught locally, before any HTTP request is sent, exactly like the
// existing empty-token check (see TestUserMe_MissingAccessToken_NoRequestSent
// in user_test.go).

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestUserMe_ControlCharacterAccessToken_NoRequestSent verifies that an
// access token containing a CR/LF pair is rejected locally: the SDK must
// return a non-nil error without ever reaching the network.
func TestUserMe_ControlCharacterAccessToken_NoRequestSent(t *testing.T) {
	const token = "abc\r\ndef"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for an access token containing control characters, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a CR/LF-containing accessToken")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0 (a control-character accessToken must be rejected locally, before any HTTP request)", counter.Count())
	}
}

// TestUserMe_DELCharacterAccessToken_NoRequestSent covers the other edge of
// the control-character range called out by the fix task: byte 0x7F (DEL)
// must be rejected the same way as the more obviously dangerous 0x00-0x1F
// range.
func TestUserMe_DELCharacterAccessToken_NoRequestSent(t *testing.T) {
	token := "abc" + string(rune(0x7F)) + "def"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for an access token containing a DEL (0x7F) byte, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a DEL-containing accessToken")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0 (a control-character accessToken must be rejected locally, before any HTTP request)", counter.Count())
	}
}

// TestUserMe_ControlCharacterAccessToken_ErrorDoesNotLeakToken asserts the
// local validation error's message never echoes the raw (invalid) token
// content back to the caller, matching the same "never leak the token via
// Error()" rule already enforced for the empty-token and
// rejected-by-server paths (see TestUserMe_ErrorDoesNotLeakAccessToken in
// user_test.go).
func TestUserMe_ControlCharacterAccessToken_ErrorDoesNotLeakToken(t *testing.T) {
	const token = "abc\r\ndef"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for an access token containing control characters, got %s %s", r.Method, r.URL.Path)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a CR/LF-containing accessToken")
	}
	if got := err.Error(); strings.Contains(got, token) {
		t.Fatalf("err.Error() leaks the raw invalid accessToken content: %q", got)
	}
}

// TestUserMe_TabControlCharacterAccessToken_NoRequestSent is the
// discriminating case for this requirement: Go's own net/http header-value
// validation (httpguts.ValidHeaderFieldValue) already refuses to send a
// request whose header value contains CR/LF or a DEL (0x7F) byte — so those
// two cases can appear to "pass" even with no qdmp-level validation at all,
// purely as a side effect of the standard library. A lone horizontal tab
// (0x09) is *not* rejected by net/http (it is treated as legal header
// whitespace/LWS and is transmitted as-is — confirmed empirically: a GET
// request with "access-token: abc\tdef" reaches the server body untouched).
// Since the fix task's contract is "reject any byte <= 0x1F", a tab must
// still be caught by the SDK's own requireAccessToken check; this test only
// passes once that local check actually exists.
func TestUserMe_TabControlCharacterAccessToken_NoRequestSent(t *testing.T) {
	const token = "abc\tdef"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for an access token containing a tab (0x09) byte, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a tab-containing accessToken (net/http alone will NOT reject this and would send it over the wire)")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil on validation failure", me)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0 (a control-character accessToken, including a bare tab, must be rejected locally, before any HTTP request)", counter.Count())
	}
}

// TestUserMe_TabControlCharacterAccessToken_ErrorDoesNotLeakToken is the
// tab-byte counterpart to TestUserMe_ControlCharacterAccessToken_ErrorDoesNotLeakToken
// above, for the same reason TestUserMe_TabControlCharacterAccessToken_NoRequestSent
// exists: it exercises the SDK's own error message, not one manufactured by
// net/http's transport-level rejection.
func TestUserMe_TabControlCharacterAccessToken_ErrorDoesNotLeakToken(t *testing.T) {
	const token = "abc\tdef"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for an access token containing a tab (0x09) byte, got %s %s", r.Method, r.URL.Path)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken(token).User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a local validation error for a tab-containing accessToken")
	}
	if got := err.Error(); strings.Contains(got, token) {
		t.Fatalf("err.Error() leaks the raw invalid accessToken content: %q", got)
	}
}
