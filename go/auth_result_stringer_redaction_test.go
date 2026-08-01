package qdmp_test

// Regression test for a real security finding (Fable5 design-consistency
// review): Code2SessionResult and RefreshTokenResult are plain structs with
// no fmt.Stringer implementation, so fmt.Println(result)/fmt.Printf("%v"/
// "%+v", result) and most structured loggers (which fall back to %v/%+v)
// would print AccessToken/RefreshToken in the clear — an easy way for an
// integrator's incidental debug log line to leak a live user session token.
//
// The fix (auth.go, Code2SessionResult.String / RefreshTokenResult.String)
// must redact AccessToken/RefreshToken in the %v/%+v/Println path while
// leaving json.Marshal and direct field access completely untouched: the
// entire reason Code2Session/RefreshToken exist is so the caller can
// persist the real values into its own session store, so over-redacting the
// serialization path would make the SDK unusable.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

const (
	sentinelAccessToken  = "sentinel-access-token-do-not-leak-xyz123"
	sentinelRefreshToken = "sentinel-refresh-token-do-not-leak-xyz123"
)

func TestCode2SessionResult_StringRedactsTokens(t *testing.T) {
	result := &qdmp.Code2SessionResult{
		AccessToken:  sentinelAccessToken,
		RefreshToken: sentinelRefreshToken,
		ExpiresAt:    "1785508950",
		OpenID:       "open-id-1",
	}

	for _, format := range []string{"%v", "%+v"} {
		out := fmt.Sprintf(format, result)
		if strings.Contains(out, sentinelAccessToken) {
			t.Fatalf("fmt.Sprintf(%q, *Code2SessionResult) = %q, leaked the raw accessToken", format, out)
		}
		if strings.Contains(out, sentinelRefreshToken) {
			t.Fatalf("fmt.Sprintf(%q, *Code2SessionResult) = %q, leaked the raw refreshToken", format, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("fmt.Sprintf(%q, *Code2SessionResult) = %q, want it to contain the [REDACTED] placeholder", format, out)
		}
		// Non-secret fields must still be visible.
		if !strings.Contains(out, "1785508950") || !strings.Contains(out, "open-id-1") {
			t.Fatalf("fmt.Sprintf(%q, *Code2SessionResult) = %q, want ExpiresAt/OpenID still printed", format, out)
		}
	}

	// Same assertions for the value type (not just the pointer), and via
	// fmt.Println's default formatting.
	valueOut := fmt.Sprintf("%v", *result)
	if strings.Contains(valueOut, sentinelAccessToken) || strings.Contains(valueOut, sentinelRefreshToken) {
		t.Fatalf("fmt.Sprintf(%%v, Code2SessionResult value) = %q, leaked a raw token", valueOut)
	}
}

// TestCode2SessionResult_GoStringRedactsTokens is a regression test for a
// follow-up finding from codex's convergence-check review: fmt.Stringer's
// String() is NOT consulted by the %#v verb (Go-syntax representation) —
// only fmt.GoStringer is. Without a GoString() method, fmt.Printf("%#v",
// result) would still print the raw AccessToken/RefreshToken fields even
// though %v/%+v are already redacted above.
func TestCode2SessionResult_GoStringRedactsTokens(t *testing.T) {
	result := &qdmp.Code2SessionResult{
		AccessToken:  sentinelAccessToken,
		RefreshToken: sentinelRefreshToken,
		ExpiresAt:    "1785508950",
		OpenID:       "open-id-1",
	}

	out := fmt.Sprintf("%#v", result)
	if strings.Contains(out, sentinelAccessToken) || strings.Contains(out, sentinelRefreshToken) {
		t.Fatalf("fmt.Sprintf(%%#v, *Code2SessionResult) = %q, leaked a raw token", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("fmt.Sprintf(%%#v, *Code2SessionResult) = %q, want the [REDACTED] placeholder", out)
	}
}

func TestRefreshTokenResult_GoStringRedactsTokens(t *testing.T) {
	result := &qdmp.RefreshTokenResult{
		AccessToken: sentinelAccessToken,
		ExpiresAt:   "1785508950",
	}

	out := fmt.Sprintf("%#v", result)
	if strings.Contains(out, sentinelAccessToken) {
		t.Fatalf("fmt.Sprintf(%%#v, *RefreshTokenResult) = %q, leaked the raw accessToken", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("fmt.Sprintf(%%#v, *RefreshTokenResult) = %q, want the [REDACTED] placeholder", out)
	}
}

// TestCode2SessionResult_SlogRedactsTokens is a regression test for a
// follow-up finding from codex's second convergence-check review:
// fmt.Stringer/fmt.GoStringer are not consulted by log/slog's handlers —
// slog.JSONHandler in particular JSON-marshals values directly, so
// slog.Info("session", "value", result) would still emit the raw
// AccessToken/RefreshToken even with String()/GoString() already redacting
// the plain fmt.Printf paths. Only slog.LogValuer (LogValue()) is consulted
// by slog itself.
func TestCode2SessionResult_SlogRedactsTokens(t *testing.T) {
	result := &qdmp.Code2SessionResult{
		AccessToken:  sentinelAccessToken,
		RefreshToken: sentinelRefreshToken,
		ExpiresAt:    "1785508950",
		OpenID:       "open-id-1",
	}

	handlers := []struct {
		name string
		new  func(*bytes.Buffer) slog.Handler
	}{
		{"JSONHandler", func(buf *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(buf, nil) }},
		{"TextHandler", func(buf *bytes.Buffer) slog.Handler { return slog.NewTextHandler(buf, nil) }},
	}
	for _, h := range handlers {
		var buf bytes.Buffer
		logger := slog.New(h.new(&buf))
		logger.Info("session", "value", result)
		out := buf.String()
		if strings.Contains(out, sentinelAccessToken) || strings.Contains(out, sentinelRefreshToken) {
			t.Fatalf("slog with %s leaked a raw token: %s", h.name, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("slog with %s output should contain the [REDACTED] placeholder, got: %s", h.name, out)
		}
	}
}

func TestRefreshTokenResult_StringRedactsTokens(t *testing.T) {
	result := &qdmp.RefreshTokenResult{
		AccessToken: sentinelAccessToken,
		ExpiresAt:   "1785508950",
	}

	for _, format := range []string{"%v", "%+v"} {
		out := fmt.Sprintf(format, result)
		if strings.Contains(out, sentinelAccessToken) {
			t.Fatalf("fmt.Sprintf(%q, *RefreshTokenResult) = %q, leaked the raw accessToken", format, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("fmt.Sprintf(%q, *RefreshTokenResult) = %q, want it to contain the [REDACTED] placeholder", format, out)
		}
		if !strings.Contains(out, "1785508950") {
			t.Fatalf("fmt.Sprintf(%q, *RefreshTokenResult) = %q, want ExpiresAt still printed", format, out)
		}
	}
}

// TestCode2SessionResult_JSONMarshalNotRedacted is the reverse-direction
// guard: json.Marshal is how the caller persists the real session into its
// own DB/session store, so it must keep returning the real values — the
// String()/fmt.Stringer redaction must not leak into encoding/json, which
// only consults json.Marshaler (not fmt.Stringer).
func TestCode2SessionResult_JSONMarshalNotRedacted(t *testing.T) {
	result := &qdmp.Code2SessionResult{
		AccessToken:  sentinelAccessToken,
		RefreshToken: sentinelRefreshToken,
		ExpiresAt:    "1785508950",
		OpenID:       "open-id-1",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(Code2SessionResult) returned unexpected error: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, sentinelAccessToken) {
		t.Fatalf("json.Marshal(Code2SessionResult) = %q, want it to contain the real accessToken for the caller to persist", out)
	}
	if !strings.Contains(out, sentinelRefreshToken) {
		t.Fatalf("json.Marshal(Code2SessionResult) = %q, want it to contain the real refreshToken for the caller to persist", out)
	}

	// Direct field access must also stay the real plaintext value.
	if result.AccessToken != sentinelAccessToken {
		t.Fatalf("result.AccessToken = %q, want the raw sentinel value %q", result.AccessToken, sentinelAccessToken)
	}
	if result.RefreshToken != sentinelRefreshToken {
		t.Fatalf("result.RefreshToken = %q, want the raw sentinel value %q", result.RefreshToken, sentinelRefreshToken)
	}
}

// TestRefreshTokenResult_JSONMarshalNotRedacted mirrors the above for
// RefreshTokenResult.
func TestRefreshTokenResult_JSONMarshalNotRedacted(t *testing.T) {
	result := &qdmp.RefreshTokenResult{
		AccessToken: sentinelAccessToken,
		ExpiresAt:   "1785508950",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(RefreshTokenResult) returned unexpected error: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, sentinelAccessToken) {
		t.Fatalf("json.Marshal(RefreshTokenResult) = %q, want it to contain the real accessToken for the caller to persist", out)
	}

	if result.AccessToken != sentinelAccessToken {
		t.Fatalf("result.AccessToken = %q, want the raw sentinel value %q", result.AccessToken, sentinelAccessToken)
	}
}
