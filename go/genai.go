package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// GenAIGenerateResult is the response of genai.generate.
type GenAIGenerateResult struct {
	ID string `json:"id"`
}

// GenAIDetailResult is the response of genai.detail.
type GenAIDetailResult struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Response map[string]any `json:"response"`
}

// GenAIGroup implements the "genai" operation group. Unlike every other
// group, genai uses its own distinct authScheme ("genai" in
// shared/generated/route-meta.json): it sends x-openapi-access-token /
// x-openapi-app-id instead of access-token / x-echo-qdmp-version, and never
// sends the latter pair (confirmed by the absence of x-echo-qdmp-version in
// this group's codeExample, per shared/openapi.yaml).
type GenAIGroup struct {
	client *Client
}

// Generate submits an async AI generation task.
func (g *GenAIGroup) Generate(ctx context.Context, qdmpCtx Context, body generated.GenaiGenerateJSONBody) (*GenAIGenerateResult, error) {
	if err := requireAccessToken(qdmpCtx, "genai.generate"); err != nil {
		return nil, err
	}

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodPost,
		path:        "/genai/v1/generate",
		jsonBody:    body,
		authScheme:  authSchemeGenai,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result GenAIGenerateResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode genai.generate response: %w", err)
	}
	return &result, nil
}

// Detail polls the status/result of an async AI generation task by ID.
//
// generated.GenaiDetailParams also carries this scheme's own header fields
// (XOpenapiAccessToken, XOpenapiAppId); like SpuGroup.Search, this method
// never reads them back off params — the real values come from the derived
// per-call Context's access token and the client's own configured AppID.
func (g *GenAIGroup) Detail(ctx context.Context, qdmpCtx Context, params generated.GenaiDetailParams) (*GenAIDetailResult, error) {
	if err := requireAccessToken(qdmpCtx, "genai.detail"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("id", params.Id)

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/genai/v1/detail",
		query:       query,
		authScheme:  authSchemeGenai,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result GenAIDetailResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode genai.detail response: %w", err)
	}
	return &result, nil
}
