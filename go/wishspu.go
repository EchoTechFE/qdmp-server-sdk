package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// WishAddResult is the response of wishspu.add.
type WishAddResult struct {
	SuccessCount string `json:"successCount"`
}

// WishCancelResult is the response of wishspu.cancel.
type WishCancelResult struct {
	SuccessCount string `json:"successCount"`
}

// WishItem is one element of wishspu.list's data.items[].
type WishItem struct {
	ID        string      `json:"id"`
	Spu       *SpuSummary `json:"spu"`
	TypeID    string      `json:"typeId"`
	MarkAt    string      `json:"markAt"`
	CreatedAt string      `json:"createdAt"`
}

// WishListResult is the response of wishspu.list.
type WishListResult struct {
	Items      []WishItem `json:"items"`
	TotalCount string     `json:"totalCount"`
}

// WishSpuGroup implements the "wishspu" operation group.
type WishSpuGroup struct {
	client *Client
}

// Add batch-adds "wish" entries.
func (g *WishSpuGroup) Add(ctx context.Context, qdmpCtx Context, body generated.WishAddJSONBody) (*WishAddResult, error) {
	if err := requireAccessToken(qdmpCtx, "wishspu.add"); err != nil {
		return nil, err
	}

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodPost,
		path:        "/wishspu/v1/add",
		jsonBody:    body,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result WishAddResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode wishspu.add response: %w", err)
	}
	return &result, nil
}

// Cancel batch-cancels "wish" entries.
func (g *WishSpuGroup) Cancel(ctx context.Context, qdmpCtx Context, body generated.WishCancelJSONBody) (*WishCancelResult, error) {
	if err := requireAccessToken(qdmpCtx, "wishspu.cancel"); err != nil {
		return nil, err
	}

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodPost,
		path:        "/wishspu/v1/cancel",
		jsonBody:    body,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result WishCancelResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode wishspu.cancel response: %w", err)
	}
	return &result, nil
}

// List fetches the current user's "wish" list, paginated.
func (g *WishSpuGroup) List(ctx context.Context, qdmpCtx Context, params generated.WishListParams) (*WishListResult, error) {
	if err := requireAccessToken(qdmpCtx, "wishspu.list"); err != nil {
		return nil, err
	}

	query := url.Values{}
	setIfNotNil(query, "typeId", params.TypeId)
	query.Set("offset", params.Offset)
	query.Set("limit", params.Limit)

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/wishspu/v1/list",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result WishListResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode wishspu.list response: %w", err)
	}
	return &result, nil
}
