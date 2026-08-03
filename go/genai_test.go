package qdmp_test

// Contract exercised by this file:
//
//	qdmpCtx := qdmp.Context{AccessToken: token}   // 凭证每次调用显式传
//	res, err := asUser.GenAI.Generate(ctx, generated.GenaiGenerateJSONBody{Model, Contents})
//
// qdmp.GenAIGenerateResult{ID string}
//
// genai is a distinct authScheme (shared/generated/route-meta.json:
// genaiGenerate/genaiDetail -> "genai") that uses its own header pair,
// x-openapi-access-token / x-openapi-app-id, and must NOT send the
// "standard" scheme's access-token / x-echo-qdmp-version headers (per
// shared/openapi.yaml operation description: "x-echo-qdmp-version 头只对
// standard 认证方案的接口生效"). The
// x-openapi-app-id value is the client's own configured AppID, not
// something the caller passes per call.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	qdmp "github.com/EchoTechFE/qdmp-server-sdk/go"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

func TestGenAIGenerate_Success_UsesGenaiHeaderPair(t *testing.T) {
	const token = "genai-access-token"
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/genai/v1/generate" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-openapi-access-token"); got != token {
			t.Fatalf("x-openapi-access-token header = %q, want %q", got, token)
		}
		if got := r.Header.Get("x-openapi-app-id"); got != "test-app-id" {
			t.Fatalf("x-openapi-app-id header = %q, want the client's configured AppID %q", got, "test-app-id")
		}
		if got := r.Header.Get("access-token"); got != "" {
			t.Fatalf("access-token header = %q, want empty — genai scheme must not send the standard-scheme header", got)
		}
		if got := r.Header.Get("x-echo-qdmp-version"); got != "" {
			t.Fatalf("x-echo-qdmp-version header = %q, want empty — genai scheme must not send the standard-scheme version header", got)
		}
		businessEnvelope(w, http.StatusOK, "0", "ok", "req-genai-1", map[string]any{
			"id": "genai-task-42",
		})
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	role := "user"
	text := "画一只猫"
	res, err := client.GenAI.Generate(context.Background(), qdmp.Context{AccessToken: token}, generated.GenaiGenerateJSONBody{
		Model: "google/gemini-3.1-flash-image-preview",
		Contents: []struct {
			Parts *[]generated.GenaiGenerateJSONBody_Contents_Parts_Item `json:"parts,omitempty"`
			Role  *string                                                `json:"role,omitempty"`
		}{
			{
				Role: &role,
				Parts: &[]generated.GenaiGenerateJSONBody_Contents_Parts_Item{
					{Text: &text},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GenAI.Generate() error = %v, want nil", err)
	}
	if res.ID != "genai-task-42" {
		t.Fatalf("GenAI.Generate() result = %+v, want ID = genai-task-42", res)
	}
	if counter.Count() != 1 {
		t.Fatalf("server hit %d times, want 1", counter.Count())
	}
}

// TestGenAIGenerate_MissingAccessToken_NoRequestSent: genai.generate is
// tokenRequired=true too, despite using its own header pair.
func TestGenAIGenerate_MissingAccessToken_NoRequestSent(t *testing.T) {
	counter := newRequestCounter(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should never have been called for a missing access token, got %s %s", r.Method, r.URL.Path)
	})
	srv := startServer(t, counter)
	client := newTestClient(t, srv.URL)

	res, err := client.GenAI.Generate(context.Background(), qdmp.Context{AccessToken: ""}, generated.GenaiGenerateJSONBody{
		Model: "google/gemini-3.1-flash-image-preview",
	})
	if err == nil {
		t.Fatalf("GenAI.Generate() error = nil, want a local validation error for empty accessToken")
	}
	if res != nil {
		t.Fatalf("GenAI.Generate() result = %+v, want nil on validation failure", res)
	}
	if !errors.Is(err, qdmp.ErrAccessTokenRequired) {
		t.Fatalf("err = %v, want errors.Is(err, qdmp.ErrAccessTokenRequired)", err)
	}
	if counter.Count() != 0 {
		t.Fatalf("server hit %d times, want 0", counter.Count())
	}
}

// TestGenAIGenerate_HTTP401_Code10005 verifies the same "HTTP status is not
// authoritative, check code" rule holds for the genai authScheme too.
func TestGenAIGenerate_HTTP401_Code10005(t *testing.T) {
	srv := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessEnvelope(w, http.StatusUnauthorized, float64(10005), "access_token 格式错误或已吊销", "req-genai-401", nil)
	}))
	client := newTestClient(t, srv.URL)

	res, err := client.GenAI.Generate(context.Background(), qdmp.Context{AccessToken: "a-revoked-genai-token"}, generated.GenaiGenerateJSONBody{
		Model: "google/gemini-3.1-flash-image-preview",
	})
	if err == nil {
		t.Fatalf("GenAI.Generate() error = nil, want an error for HTTP 401 / code=10005")
	}
	if res != nil {
		t.Fatalf("GenAI.Generate() result = %+v, want nil on failure", res)
	}
	apiErr, ok := err.(*qdmp.QdmpApiError)
	if !ok {
		t.Fatalf("err type = %T, want *qdmp.QdmpApiError", err)
	}
	if apiErr.Code != "10005" || apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("apiErr = %+v, want Code=10005 HTTPStatus=401", apiErr)
	}
}
