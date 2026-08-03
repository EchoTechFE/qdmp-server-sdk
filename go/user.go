package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserMeResult is the response of user.me (data.identityTags/interestTags
// have no documented internal shape, per shared/openapi.yaml — modeled as
// open maps, unverified).
type UserMeResult struct {
	ID           string           `json:"id"`
	Nickname     string           `json:"nickname"`
	Avatar       string           `json:"avatar"`
	IdentityTags []map[string]any `json:"identityTags"`
	InterestTags []map[string]any `json:"interestTags"`
}

// UserGroup implements the "user" operation group.
type UserGroup struct {
	client *Client
}

// Me fetches the current user identified by the per-call Context's access
// token (user.me has x-qdmp-token-required=true — no query parameters, the
// server identifies the caller purely from the access-token header).
func (g *UserGroup) Me(ctx context.Context, qdmpCtx Context) (*UserMeResult, error) {
	if err := requireAccessToken(qdmpCtx, "user.me"); err != nil {
		return nil, err
	}

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/user/v1/me",
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result UserMeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode user.me response: %w", err)
	}
	return &result, nil
}
