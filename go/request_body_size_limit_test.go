package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// doRequest (request.go ~line 130) reads the entire response body via
// io.ReadAll(resp.Body) with no size limit whatsoever. A misbehaving or
// compromised qdmp gateway (or a man-in-the-middle) returning an
// arbitrarily large response body would make the SDK buffer it entirely in
// memory before even attempting to JSON-decode it — an easy memory-
// exhaustion vector for anything driving qdmp SDK calls against an
// untrusted or unreliable endpoint.
//
// New requirement: doRequest must enforce an upper bound on how much of the
// response body it will read, surfacing a non-nil error for a response that
// exceeds it rather than buffering the whole thing and only failing later
// (or, as this test demonstrates, not failing at all) at JSON-decode time.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestDoRequest_OversizedResponseBodyRejected drives a real business call
// (User.Me) against a mock server that returns an ~11MB response that is
// still perfectly valid JSON matching the business envelope + UserMeResult
// shape (a huge but otherwise legitimate avatar URL string) — chosen
// deliberately so that, absent a size cap, doRequest would decode it
// successfully and return err == nil, proving there is currently no limit
// on how much of the response body gets buffered/parsed. With a size cap in
// place this must instead produce a non-nil error.
func TestDoRequest_OversizedResponseBodyRejected(t *testing.T) {
	const oversized = 11 * 1024 * 1024 // 11MB
	hugeAvatar := strings.Repeat("a", oversized)

	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-huge-1", map[string]any{
			"id":       "user-huge",
			"nickname": "n",
			"avatar":   hugeAvatar,
		})
	}))
	client := newTestClient(t, srv.URL)

	me, err := client.WithAccessToken("some-token").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error because the response body (%d bytes) exceeds the SDK's size limit; it decoded successfully as a valid oversized envelope, proving no size cap is currently enforced", oversized)
	}
	if me != nil {
		t.Fatalf("User.Me() result = %+v, want nil when the response is rejected as oversized", me)
	}
}
