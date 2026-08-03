package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// QdmpApiError's Message and RequestID fields are already sanitized via
// sanitizeForErrorMessage at construction time, in doRequest (request.go
// ~line 160-169), before the error is ever handed back to a caller — this
// defends against log injection (a malicious/buggy upstream embedding a raw
// CR/LF to forge a fake extra "log line") and unbounded length. The Code
// field, constructed on the very same struct literal right next to Message
// and RequestID, is not sanitized the same way — it is assigned the raw
// output of normalizeCode(env.Code) verbatim.
//
// Code is documented (errors.go, QdmpApiError's doc comment) as an
// intentionally open string, not a closed enum/int, because the qdmp API is
// known to use non-numeric-looking business codes in one envelope shape.
// normalizeCode (request.go) explicitly supports the wire code field being
// encoded as either a bare JSON number or a JSON string (see its
// `trimmed[0] == '"'` branch) — so a malicious/buggy upstream can send a
// string-form code containing embedded raw CR/LF bytes (or an unbounded
// length), and that string flows untouched through normalizeCode straight
// into QdmpApiError.Code, and from there into Error()'s output — the exact
// same log-injection risk Message/RequestID are already protected against.
//
// New requirement: Code must be sanitized the same way as Message/RequestID
// at construction time (in doRequest, via sanitizeForErrorMessage or
// equivalent), so both Error()'s formatted output and the exported Code
// field itself are free of raw CR/LF bytes and bounded in length.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestQdmpApiError_CodeCRLFNotEchoedRaw drives a business failure whose
// wire `code` field is a JSON *string* (not a bare number) containing an
// embedded CRLF sequence designed to look like a second, fake log line if
// echoed raw into a log file. Passing a Go string as businessEnvelope's code
// argument makes it JSON-encode as a quoted string, exactly matching the
// string-form business codes normalizeCode's `trimmed[0] == '"'` branch is
// built to support. Both Error()'s formatted output and the exported Code
// field on the underlying *qdmp.QdmpApiError must be free of any raw \r or
// \n byte.
func TestQdmpApiError_CodeCRLFNotEchoedRaw(t *testing.T) {
	const injectedCode = "1\r\nFAKE injected log line"
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusOK, injectedCode, "some failure", "req-crlf-code-1", nil)
	}))
	client := newTestClient(t, srv.URL)

	_, err := client.User.Me(context.Background(), qdmp.Context{AccessToken: "some-token"})
	if err == nil {
		t.Fatalf("User.Me() error = nil, want an error because code=%q is not \"0\"", injectedCode)
	}

	got := err.Error()
	if strings.ContainsRune(got, '\r') || strings.ContainsRune(got, '\n') {
		t.Fatalf("err.Error() = %q, must not contain a raw CR or LF byte from the server-supplied code (log-injection risk)", got)
	}

	var apiErr *qdmp.QdmpApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(err, &apiErr) = false, want true: err should be a *qdmp.QdmpApiError, got %T", err)
	}
	if strings.ContainsRune(apiErr.Code, '\r') || strings.ContainsRune(apiErr.Code, '\n') {
		t.Fatalf("apiErr.Code = %q, must not contain a raw CR or LF byte from the server-supplied code (the exported field itself must be sanitized at construction time, not just cosmetically hidden in Error()'s formatting)", apiErr.Code)
	}
}
