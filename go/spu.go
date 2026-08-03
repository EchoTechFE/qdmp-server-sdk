package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// SpuInfo is the spu/v1/detail data.spu shape.
type SpuInfo struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Image             string           `json:"image"`
	WhiteBgPng        string           `json:"whiteBgPng"`
	TypeID            string           `json:"typeId"`
	TypeName          string           `json:"typeName"`
	WishCount         string           `json:"wishCount"`
	MarkCount         string           `json:"markCount"`
	WishCount3day     string           `json:"wishCount3day"`
	MarkCount3day     string           `json:"markCount3day"`
	EntryProfileItems []map[string]any `json:"entryProfileItems"`
}

// SpuDetailResult is the response of spu.detail.
type SpuDetailResult struct {
	Spu SpuInfo `json:"spu"`
}

// SpuSearchItem is one element of spu.search's data.items[].
type SpuSearchItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	TypeID    string `json:"typeId"`
	TypeName  string `json:"typeName"`
	WishCount string `json:"wishCount"`
	MarkCount string `json:"markCount"`
}

// SpuSearchResult is the response of spu.search.
type SpuSearchResult struct {
	Items      []SpuSearchItem `json:"items"`
	TotalCount string          `json:"totalCount"`
}

// SpuGroup implements the "spu" operation group.
type SpuGroup struct {
	uc *UserClient
}

// Detail fetches a single SPU's details by ID.
func (g *SpuGroup) Detail(ctx context.Context, params generated.SpuDetailParams) (*SpuDetailResult, error) {
	if err := requireAccessToken(g.uc, "spu.detail"); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("id", params.Id)

	data, err := g.uc.doRequest(ctx, requestParams{
		method:     http.MethodGet,
		path:       "/spu/v1/detail",
		query:      query,
		authScheme: authSchemeStandard,
	})
	if err != nil {
		return nil, err
	}

	var result SpuDetailResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode spu.detail response: %w", err)
	}
	return &result, nil
}

// Search searches SPUs by keyword/category/IP-tag filters. Confirmed by
// real testing to also work with an app-level (CLIENT_CREDENTIALS) token.
//
// generated.SpuSearchParams bundles the query fields together with the
// "standard" authScheme's header fields (AccessToken, XEchoQdmpVersion)
// because that is what oapi-codegen produced from the shared openapi.yaml
// spec (header params and query params both land on the operation's Params
// struct). This method deliberately never reads params.AccessToken or
// params.XEchoQdmpVersion — the real access-token comes from the derived
// UserClient, and the real x-echo-qdmp-version comes from the client's own
// configured QdmpVersion — so a caller cannot spoof either header by
// setting those two fields.
func (g *SpuGroup) Search(ctx context.Context, params generated.SpuSearchParams) (*SpuSearchResult, error) {
	if err := requireAccessToken(g.uc, "spu.search"); err != nil {
		return nil, err
	}

	query := url.Values{}
	setIfNotNil(query, "keyword", params.Keyword)
	setIfNotNil(query, "ipTag", params.IpTag)
	setIfNotNil(query, "typeId", params.TypeId)
	setIfNotNil(query, "megaId", params.MegaId)
	setIfNotNil(query, "classification", params.Classification)
	addAllIfNotNil(query, "filterOptions", params.FilterOptions)
	setBoolIfNotNil(query, "onlyMega", params.OnlyMega)
	setBoolIfNotNil(query, "onlySpu", params.OnlySpu)
	setIfNotNil(query, "offset", params.Offset)
	setIfNotNil(query, "limit", params.Limit)

	data, err := g.uc.doRequest(ctx, requestParams{
		method:     http.MethodGet,
		path:       "/spu/v1/search",
		query:      query,
		authScheme: authSchemeStandard,
	})
	if err != nil {
		return nil, err
	}

	var result SpuSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode spu.search response: %w", err)
	}
	return &result, nil
}
