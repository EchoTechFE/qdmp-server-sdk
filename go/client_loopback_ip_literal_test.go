package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// isLoopbackHost (client.go ~line 166-172) decides whether a plain "http://"
// BaseURL is allowed for a given host by doing a naive string check:
//
//	h == "localhost" || h == "127.0.0.1" || strings.HasPrefix(h, "127.")
//
// strings.HasPrefix(h, "127.") is not an IP-literal check — it is satisfied
// by any hostname that merely starts with the four characters "127.", even
// when the host is not an IP address at all. A domain name such as
// "127.attacker.com" trivially passes this check today, even though it is a
// perfectly ordinary DNS name that could resolve to any attacker-controlled
// IP address anywhere on the internet. Since validateBaseURLScheme only
// treats http:// as safe when isLoopbackHost is true, this lets an attacker-
// chosen BaseURL smuggle a plain-http, non-loopback destination past the
// scheme check, leaking appId/appSecret/access-token headers in plaintext to
// that remote host.
//
// New requirement: isLoopbackHost (via validateBaseURLScheme/NewClient) must
// only recognize genuine loopback IP literals (127.0.0.1 and other bare
// 127.x.x.x dotted-quad addresses with no additional labels) or the exact
// hostname "localhost" (case-insensitive) — never a domain name that merely
// starts with the string "127." or "127.0.0.1" followed by more labels.

import (
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestNewClient_RejectsNonLoopbackDomainStartingWith127 is the core
// regression test: "127.attacker.com" is a domain name, not an IP address —
// it must be rejected for a plain-http BaseURL exactly like any other
// non-loopback host, since DNS could resolve it to a remote attacker-
// controlled server.
func TestNewClient_RejectsNonLoopbackDomainStartingWith127(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://127.attacker.com",
	})
	if err == nil {
		t.Fatalf("NewClient() error = nil, want an error for a plain-http BaseURL whose host is the domain name 127.attacker.com (not a loopback IP literal)")
	}
	if client != nil {
		t.Fatalf("NewClient() client = %+v, want nil when BaseURL validation fails", client)
	}
}

// TestNewClient_RejectsLoopbackIPWithExtraDomainLabels is a second shape of
// the same bug: "127.0.0.1.evil.com" has the exact string "127.0.0.1" as a
// prefix, but it is a five-label domain name, not the loopback IP literal
// 127.0.0.1 — it must resolve via DNS like any other domain and must not be
// treated as loopback.
func TestNewClient_RejectsLoopbackIPWithExtraDomainLabels(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://127.0.0.1.evil.com",
	})
	if err == nil {
		t.Fatalf("NewClient() error = nil, want an error for a plain-http BaseURL whose host is the domain name 127.0.0.1.evil.com (not a loopback IP literal)")
	}
	if client != nil {
		t.Fatalf("NewClient() client = %+v, want nil when BaseURL validation fails", client)
	}
}

// TestNewClient_StillAllowsPlainHTTPForLoopbackIP is a sanity check that the
// fix for the above must not break: a genuine loopback IP literal must
// continue to be accepted with a plain http:// BaseURL, since this
// repository's own test suite depends on it (httptest.NewServer's .URL is
// exactly "http://127.0.0.1:PORT").
func TestNewClient_StillAllowsPlainHTTPForLoopbackIP(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://127.0.0.1:12345",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for a genuine loopback (127.0.0.1) http:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}

// TestNewClient_StillAllowsPlainHTTPForLocalhost is a sanity check that the
// fix for the above must not break the "localhost" hostname exception.
func TestNewClient_StillAllowsPlainHTTPForLocalhost(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://localhost:12345",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for a localhost http:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}
