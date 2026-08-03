package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// TagInfo is the tag/v1/detail data.tag shape, also reused by
// tag/v1/search's data.items[].
type TagInfo struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Image    string      `json:"image"`
	TypeID   string      `json:"typeId"`
	TypeName string      `json:"typeName"`
	Island   *IslandInfo `json:"island"`
}

// TagDetailResult is the response of tag.detail.
type TagDetailResult struct {
	Tag TagInfo `json:"tag"`
}

// TagSearchResult is the response of tag.search.
type TagSearchResult struct {
	Items      []TagInfo `json:"items"`
	TotalCount string    `json:"totalCount"`
}

// TagGroup implements the "tag" operation group.
type TagGroup struct {
	client *Client
}

// Detail fetches a single Tag's details by ID.
func (g *TagGroup) Detail(ctx context.Context, qdmpCtx Context, params generated.TagDetailParams) (*TagDetailResult, error) {
	if err := requireAccessToken(qdmpCtx, "tag.detail"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("id", params.Id)

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/tag/v1/detail",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result TagDetailResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode tag.detail response: %w", err)
	}
	return &result, nil
}

// Search searches Tags by keyword/category filters.
func (g *TagGroup) Search(ctx context.Context, qdmpCtx Context, params generated.TagSearchParams) (*TagSearchResult, error) {
	if err := requireAccessToken(qdmpCtx, "tag.search"); err != nil {
		return nil, err
	}

	query := url.Values{}
	setIfNotNil(query, "keyword", params.Keyword)
	setIfNotNil(query, "typeId", params.TypeId)
	setIfNotNil(query, "classification", params.Classification)
	addAllIfNotNil(query, "filterOptions", params.FilterOptions)
	setIfNotNil(query, "offset", params.Offset)
	setIfNotNil(query, "limit", params.Limit)

	data, err := g.client.doRequest(ctx, requestParams{
		method:      http.MethodGet,
		path:        "/tag/v1/search",
		query:       query,
		authScheme:  authSchemeStandard,
		accessToken: qdmpCtx.AccessToken,
	})
	if err != nil {
		return nil, err
	}

	var result TagSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode tag.search response: %w", err)
	}
	return &result, nil
}
