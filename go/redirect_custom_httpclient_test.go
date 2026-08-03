package qdmp_test

// Regression test for a gap found during the Round 1 closing adversarial
// review: NewClient only set CheckRedirect on the *http.Client it built
// itself, so a caller-supplied ClientOptions.HTTPClient (which defaults to
// following redirects, per net/http) silently reintroduced the
// redirect-leak vulnerability the rest of this package guards against. See
// redirect_test.go for the default-client version of this contract.

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestDoRequest_CustomHTTPClient_RedirectStillNotFollowed verifies that
// injecting a plain *http.Client (which follows redirects by default) via
// ClientOptions.HTTPClient does not let a 3xx response be chased to the
// redirect target: NewClient must force CheckRedirect on its own copy of
// the supplied client rather than trusting the caller to have configured it
// safely.
func TestDoRequest_CustomHTTPClient_RedirectStillNotFollowed(t *testing.T) {
	var srv *httpTestServerPlaceholder
	var redirectTargetHits int64

	mux := http.NewServeMux()
	mux.HandleFunc("/user/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL()+"/redirect-target")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/redirect-target", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&redirectTargetHits, 1)
		t.Errorf("redirect target must never be requested even with a caller-supplied HTTPClient, got %s %s", r.Method, r.URL.Path)
	})

	realSrv := startServer(t, mux)
	srv = &httpTestServerPlaceholder{url: realSrv.URL}

	// A stock *http.Client follows redirects by default (nil CheckRedirect).
	customClient := &http.Client{}
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:      "test-app-id",
		AppSecret:  "test-app-secret-do-not-leak",
		BaseURL:    realSrv.URL,
		HTTPClient: customClient,
	})
	if err != nil {
		t.Fatalf("qdmp.NewClient() returned unexpected error: %v", err)
	}

	me, err := client.User.Me(context.Background(), qdmp.Context{AccessToken: "sentinel-secret-token"})
	if err == nil {
		t.Fatalf("User.Me() error = nil, want a non-nil error because the server responded with a 302 redirect")
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil when the request fails", me)
	}
	if atomic.LoadInt64(&redirectTargetHits) != 0 {
		t.Fatalf("redirect target was hit %d times, want 0 (custom HTTPClient must still have redirects disabled)", redirectTargetHits)
	}
	// The caller's original client must not be mutated in place.
	if customClient.CheckRedirect != nil {
		t.Fatalf("the caller-supplied *http.Client was mutated in place; NewClient must operate on a shallow copy")
	}
}
