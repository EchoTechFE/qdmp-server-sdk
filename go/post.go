package qdmp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// PostGroup implements post detail and list operations. Public documentation does not define post data fields.
type PostGroup struct{ client *Client }

func (g *PostGroup) Detail(ctx context.Context, q Context, postID string) (map[string]any, error) {
	return g.get(ctx, q, "postDetail", "/post/v1/detail", url.Values{"postId": {postID}})
}
func (g *PostGroup) List(ctx context.Context, q Context, params generated.PostListParams) (map[string]any, error) {
	query := url.Values{}
	setIfNotNil(query, "offset", params.Offset)
	setIfNotNil(query, "limit", params.Limit)
	return g.get(ctx, q, "postList", "/post/v1/list", query)
}
func (g *PostGroup) MyList(ctx context.Context, q Context, params generated.PostMyListParams) (map[string]any, error) {
	query := url.Values{}
	setIfNotNil(query, "offset", params.Offset)
	setIfNotNil(query, "limit", params.Limit)
	return g.get(ctx, q, "postMyList", "/post/v1/me/list", query)
}
func (g *PostGroup) get(ctx context.Context, q Context, operationID, path string, query url.Values) (map[string]any, error) {
	if err := requireAccessToken(q, operationID); err != nil {
		return nil, err
	}
	data, err := g.client.doRequest(ctx, requestParams{method: http.MethodGet, path: path, query: query, authScheme: authSchemeStandard, accessToken: q.AccessToken})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(data, &out)
	return out, err
}
