package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// SpuSummary is the small SPU summary shape ({id, name, image}) embedded in
// mark/wishspu list-style responses' data.items[].spu field.
type SpuSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// Rating is the { value: 1-5 } rating object attached to a mark.
type Rating struct {
	Value int `json:"value"`
}

// MarkAddResult is the response of mark.add.
type MarkAddResult struct {
	ID string `json:"id"`
}

// MarkItem is one element of mark.list / mark.search's data.items[].
type MarkItem struct {
	ID        string      `json:"id"`
	Spu       *SpuSummary `json:"spu"`
	MarkAt    string      `json:"markAt"`
	Count     string      `json:"count"`
	Rating    *Rating     `json:"rating"`
	CreatedAt string      `json:"createdAt"`
	TypeID    string      `json:"typeId"`
}

// MarkListResult is the response of mark.list.
type MarkListResult struct {
	Items      []MarkItem `json:"items"`
	TotalCount string     `json:"totalCount"`
}

// MarkSearchResult is the response of mark.search.
type MarkSearchResult struct {
	Items      []MarkItem `json:"items"`
	TotalCount string     `json:"totalCount"`
}

// MarkHistoryItem is one element of mark.detail's data.marks[] history list.
type MarkHistoryItem struct {
	ID        string `json:"id"`
	MarkAt    string `json:"markAt"`
	CreatedAt string `json:"createdAt"`
}

// MarkDetailResult is the response of mark.detail.
type MarkDetailResult struct {
	ID        string            `json:"id"`
	Spu       *SpuSummary       `json:"spu"`
	MarkAt    string            `json:"markAt"`
	Count     string            `json:"count"`
	Rating    *Rating           `json:"rating"`
	CreatedAt string            `json:"createdAt"`
	TypeID    string            `json:"typeId"`
	TypeName  string            `json:"typeName"`
	Marks     []MarkHistoryItem `json:"marks"`
	HasMore   bool              `json:"hasMore"`
}

// MarkGroup implements the "marks" operation group.
type MarkGroup struct {
	uc *UserClient
}

// Add adds a mark (optionally with a rating) for a given SPU.
func (g *MarkGroup) Add(ctx context.Context, body generated.MarkAddJSONBody) (*MarkAddResult, error) {
	if err := requireAccessToken(g.uc, "mark.add"); err != nil {
		return nil, err
	}

	data, err := g.uc.client.doRequest(ctx, requestParams{
		method:      http.MethodPost,
		path:        "/mark/v1/add",
		jsonBody:    body,
		authScheme:  authSchemeStandard,
		accessToken: g.uc.accessToken,
	})
	if err != nil {
		return nil, err
	}

	var result MarkAddResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode mark.add response: %w", err)
	}
	return &result, nil
}

// List fetches the current user's marks, paginated.
func (g *MarkGroup) List(ctx context.Context, params generated.MarkListParams) (*MarkListResult, error) {
	if err := requireAccessToken(g.uc, "mark.list"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("limit", params.Limit)
	query.Set("offset", params.Offset)

	data, err := g.uc.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/mark/v1/me/list",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: g.uc.accessToken,
	})
	if err != nil {
		return nil, err
	}

	var result MarkListResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode mark.list response: %w", err)
	}
	return &result, nil
}

// Search searches the current user's marks by category.
func (g *MarkGroup) Search(ctx context.Context, params generated.MarkSearchParams) (*MarkSearchResult, error) {
	if err := requireAccessToken(g.uc, "mark.search"); err != nil {
		return nil, err
	}

	query := url.Values{}
	setIfNotNil(query, "typeId", params.TypeId)
	query.Set("limit", params.Limit)
	query.Set("offset", params.Offset)

	data, err := g.uc.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/mark/v1/me/search",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: g.uc.accessToken,
	})
	if err != nil {
		return nil, err
	}

	var result MarkSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode mark.search response: %w", err)
	}
	return &result, nil
}

// Detail fetches a single mark's details, including its history.
func (g *MarkGroup) Detail(ctx context.Context, params generated.MarkDetailParams) (*MarkDetailResult, error) {
	if err := requireAccessToken(g.uc, "mark.detail"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("id", params.Id)
	query.Set("limit", params.Limit)
	query.Set("offset", params.Offset)

	data, err := g.uc.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/mark/v1/me/detail",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: g.uc.accessToken,
	})
	if err != nil {
		return nil, err
	}

	var result MarkDetailResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode mark.detail response: %w", err)
	}
	return &result, nil
}
