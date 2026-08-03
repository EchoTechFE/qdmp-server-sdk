package qdmp_test

// Contract exercised by this file (not yet implemented — see the codex
// review round1 fix task): the default *http.Client built by NewClient must
// never follow an HTTP redirect (3xx + Location) returned by the qdmp
// OpenAPI. doRequest must surface this as a non-nil error instead of
// silently chasing the Location header, and that error must never echo the
// caller's access-token value.
//
// Regression motivation: the stock http.Client (used whenever
// ClientOptions.HTTPClient is nil) follows redirects by default. A
// compromised/misconfigured gateway (or an attacker able to influence the
// Location header on a response) could redirect the client to an arbitrary
// host, which would replay the access-token header (and any other request
// state) against that host if redirects were followed uncritically.

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestDoRequest_RedirectNotFollowed verifies that a 3xx response from the
// qdmp OpenAPI results in a non-nil error and that the SDK never actually
// issues a request to the redirect target (proving the redirect was not
// followed, not just that the eventual error was later swallowed).
func TestDoRequest_RedirectNotFollowed(t *testing.T) {
	var srv *httpTestServerPlaceholder
	var redirectTargetHits int64

	mux := http.NewServeMux()
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL()+"/redirect-target")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/redirect-target", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&redirectTargetHits, 1)
		t.Errorf("redirect target must never be requested: the SDK must not follow 3xx redirects, got %s %s", r.Method, r.URL.Path)
	})

	realSrv := startServer(t, mux)
	srv = &httpTestServerPlaceholder{url: realSrv.URL}

	client := newTestClient(t, realSrv.URL)

	me, err := client.User.Me(context.Background(), qdmp.Context{AccessToken: "sentinel-secret-token"})
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a non-nil error because the server responded with a 302 redirect")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil when the request fails", me)
	}
	if atomic.LoadInt64(&redirectTargetHits) != 0 {
		t.Fatalf("redirect target was hit %d times, want 0 (the SDK must not follow redirects)", redirectTargetHits)
	}
}

// TestDoRequest_RedirectErrorDoesNotLeakAccessToken drives the same 3xx
// redirect scenario and additionally asserts the resulting error message
// never echoes the caller's access-token value — a redirect error is still
// an SDK-internal error value and must uphold the same "never leak secrets
// via Error()" rule as every other error path in this package (see
// TestUserMe_ErrorDoesNotLeakAccessToken in user_test.go).
func TestDoRequest_RedirectErrorDoesNotLeakAccessToken(t *testing.T) {
	const token = "sentinel-secret-token"
	var srv *httpTestServerPlaceholder

	mux := http.NewServeMux()
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("access-token"); got != token {
			t.Fatalf("server never received the expected access-token header, got %q", got)
		}
		w.Header().Set("Location", srv.URL()+"/redirect-target")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/redirect-target", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect target must never be requested, got %s %s", r.Method, r.URL.Path)
	})

	realSrv := startServer(t, mux)
	srv = &httpTestServerPlaceholder{url: realSrv.URL}

	client := newTestClient(t, realSrv.URL)

	_, err := client.User.Me(context.Background(), qdmp.Context{AccessToken: token})
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a non-nil error because the server responded with a 302 redirect")
	}
	if got := err.Error(); strings.Contains(got, token) {
		t.Fatalf("err.Error() leaks the accessToken: %q", got)
	}
}

// httpTestServerPlaceholder lets the "/user/v1/me" handler above compute a
// self-referencing "Location" header (pointing back at the same
// httptest.Server) without a chicken-and-egg problem: the handler closure
// only reads srv.URL() at request time, by which point the httptest.Server
// has already been constructed and its real URL assigned.
type httpTestServerPlaceholder struct {
	url string
}

func (p *httpTestServerPlaceholder) URL() string {
	return p.url
}
