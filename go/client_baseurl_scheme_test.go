package qdmp_test

// Contract exercised by this file (not yet implemented):
//
// NewClient (client.go ~line 80) currently does not inspect
// ClientOptions.BaseURL's scheme at all — a plain "http://" BaseURL pointed
// at a real host is accepted exactly like "https://". Since the access-token
// and appId/appSecret are sent as plain headers/body fields (see request.go
// doRequest), a plain-http BaseURL to a non-loopback host would leak them to
// anyone able to observe the network path.
//
// New requirement: NewClient must reject (return a non-nil error, no panic)
// any BaseURL whose scheme is "http" (not "https") UNLESS the host is a
// loopback/local address — 127.0.0.1, any 127.x.x.x address, or
// "localhost" (case-insensitive) — which must continue to work unchanged,
// since every existing test in this package points BaseURL at an
// httptest.Server whose .URL is exactly "http://127.0.0.1:PORT".

import (
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
)

// TestNewClient_RejectsPlainHTTPForNonLoopbackHost is the core regression
// test: a plain-http BaseURL pointed at a real (non-loopback) host must
// never be accepted, since it would send appSecret/access-token headers in
// the clear over the network.
func TestNewClient_RejectsPlainHTTPForNonLoopbackHost(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://evil.example.com",
	})
	if err == nil {
		t.Fatalf("NewClient() error = nil, want an error for a plain-http, non-loopback BaseURL")
	}
	if client != nil {
		t.Fatalf("NewClient() client = %+v, want nil when BaseURL validation fails", client)
	}
}

// TestNewClient_AllowsPlainHTTPForLoopbackIP verifies the 127.0.0.1 loopback
// exception this repository's own test suite depends on
// (httptest.NewServer's .URL is exactly this shape).
func TestNewClient_AllowsPlainHTTPForLoopbackIP(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://127.0.0.1:12345",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for a loopback (127.0.0.1) http:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}

// TestNewClient_AllowsPlainHTTPForOther127Address verifies the exception
// extends to the whole 127.0.0.0/8 loopback block, not just the single
// address 127.0.0.1.
func TestNewClient_AllowsPlainHTTPForOther127Address(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://127.5.6.7:8080",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for a 127.x.x.x http:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}

// TestNewClient_AllowsPlainHTTPForLocalhost verifies the "localhost" hostname
// exception.
func TestNewClient_AllowsPlainHTTPForLocalhost(t *testing.T) {
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

// TestNewClient_AllowsPlainHTTPForLocalhostCaseInsensitive verifies the
// "localhost" exception is matched case-insensitively.
func TestNewClient_AllowsPlainHTTPForLocalhostCaseInsensitive(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "http://LOCALHOST:12345",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for a case-insensitive LOCALHOST http:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}

// TestNewClient_AllowsHTTPS verifies a normal https:// BaseURL to a
// non-loopback host still works unchanged.
func TestNewClient_AllowsHTTPS(t *testing.T) {
	client, err := qdmp.NewClient(qdmp.ClientOptions{
		AppID:     "a",
		AppSecret: "s",
		BaseURL:   "https://example.com",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil for an https:// BaseURL", err)
	}
	if client == nil {
		t.Fatalf("NewClient() client = nil, want a non-nil client")
	}
}
