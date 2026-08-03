package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// IslandInfo is the island/v1/detail data.island shape, also reused as
// tag/v1/detail's data.tag.island (a Tag's optionally-associated island).
type IslandInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	Joined bool   `json:"joined"`
}

// IslandDetailResult is the response of island.detail.
type IslandDetailResult struct {
	Island IslandInfo `json:"island"`
}

// IslandGroup implements the "island" operation group.
type IslandGroup struct {
	client *Client
}

// Detail fetches an island's basic info by ID. Confirmed by real testing to
// also work with an app-level (CLIENT_CREDENTIALS) token; the SDK still
// requires the caller to explicitly pass some token (there is no silent
// app-token fallback).
func (g *IslandGroup) Detail(ctx context.Context, qdmpCtx Context, params generated.IslandDetailParams) (*IslandDetailResult, error) {
	if err := requireAccessToken(qdmpCtx, "island.detail"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("id", params.Id)

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/island/v1/detail",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result IslandDetailResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode island.detail response: %w", err)
	}
	return &result, nil
}
