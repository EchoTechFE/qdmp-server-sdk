package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// QdmpApiError.Error() (errors.go ~line 53) interpolates the server-supplied
// Message and RequestID fields into its output via plain fmt.Sprintf, with
// no escaping/sanitization or length bound. A malicious or buggy upstream
// response could therefore inject raw CR/LF bytes into a message that later
// gets written to a log file (classic log-injection: a fake extra "log
// line"), or send an unbounded message to make every log line built from
// this error unbounded too.
//
// New requirement: QdmpApiError.Error() must neutralize raw \r / \n bytes
// coming from the server-supplied Message/RequestID (so log-injection via
// this error is not possible) and must bound the overall length of the
// string it returns.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestQdmpApiError_MessageCRLFNotEchoedRaw drives a business failure whose
// message contains an embedded CRLF sequence designed to look like a second,
// fake log line if echoed raw into a log file. Error() must not contain a
// literal \r or \n anywhere in its output.
func TestQdmpApiError_MessageCRLFNotEchoedRaw(t *testing.T) {
	const injected = "line1\r\nFAKE injected log line"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(20000), injected, "req-crlf-msg-1", nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken("some-token").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error because code=20000")
	}
	got := err.Error()
	if strings.ContainsRune(got, '\r') || strings.ContainsRune(got, '\n') {
		t.Fatalf("err.Error() = %q, must not contain a raw CR or LF byte from the server-supplied message (log-injection risk)", got)
	}
}

// TestQdmpApiError_RequestIDCRLFNotEchoedRaw is the same log-injection
// scenario, but via the RequestID field instead of Message.
func TestQdmpApiError_RequestIDCRLFNotEchoedRaw(t *testing.T) {
	const injectedRequestID = "req-1\r\nFAKE injected log line"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(20000), "some failure", injectedRequestID, nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken("some-token").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error because code=20000")
	}
	got := err.Error()
	if strings.ContainsRune(got, '\r') || strings.ContainsRune(got, '\n') {
		t.Fatalf("err.Error() = %q, must not contain a raw CR or LF byte from the server-supplied requestId (log-injection risk)", got)
	}
}

// TestQdmpApiError_LongMessageIsBounded drives a business failure whose
// message is ~200000 characters long and asserts the resulting Error()
// string has a bounded length, proving the server-supplied message is
// truncated rather than echoed verbatim and unbounded.
func TestQdmpApiError_LongMessageIsBounded(t *testing.T) {
	longMessage := strings.Repeat("a", 200000)
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, float64(20000), longMessage, "req-long-msg-1", nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.WithAccessToken("some-token").User.Me(context.Background())
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error because code=20000")
	}
	if got := len(err.Error()); got >= 5000 {
		t.Fatalf("len(err.Error()) = %d, want a bounded/truncated length (< 5000); the ~200000-char server-supplied message must not be echoed verbatim", got)
	}
}
