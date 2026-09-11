package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
	"net/http"
	"net/url"
	"regexp"
)

type CommentGroup struct{ client *Client }

var positiveIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// CommentCreateResult is the identifier returned after creating a comment or reply.
type CommentCreateResult struct {
	ID string `json:"id"`
}

// PostCommentsResult is a page of top-level comments and their first replies.
type PostCommentsResult struct {
	Items []generated.CommentThread `json:"items"`
	Count int32                     `json:"count"`
}

// CommentRepliesResult is a cursor page of replies to a top-level comment.
type CommentRepliesResult struct {
	Items   []generated.CommentItem `json:"items"`
	HasMore bool                    `json:"hasMore"`
	Cursor  string                  `json:"cursor"`
	Count   int32                   `json:"count"`
}

func (g *CommentGroup) Create(ctx context.Context, q Context, b generated.CommentCreateJSONRequestBody) (*CommentCreateResult, error) {
	return g.postID(ctx, q, "commentCreate", "/comment", b)
}
func (g *CommentGroup) Reply(ctx context.Context, q Context, id string, b generated.CommentReplyJSONRequestBody) (*CommentCreateResult, error) {
	if err := requireAccessToken(q, "commentReply"); err != nil {
		return nil, err
	}
	if err := validatePositiveID("commentId", id); err != nil {
		return nil, err
	}
	return g.postID(ctx, q, "commentReply", "/comment/"+id+"/reply", b)
}
func (g *CommentGroup) Like(ctx context.Context, q Context, id string, b generated.CommentLikeJSONRequestBody) error {
	if err := requireAccessToken(q, "commentLike"); err != nil {
		return err
	}
	if err := validatePositiveID("commentId", id); err != nil {
		return err
	}
	_, err := g.client.doRequest(ctx, requestParams{method: http.MethodPost, path: "/comment/" + url.PathEscape(id) + "/like", jsonBody: b, authScheme: authSchemeStandard, accessToken: q.AccessToken})
	return err
}

// PostComments returns top-level comments and their first replies for a post.
func (g *CommentGroup) PostComments(ctx context.Context, q Context, postID string, params generated.PostCommentsParams) (*PostCommentsResult, error) {
	if err := requireAccessToken(q, "postComments"); err != nil {
		return nil, err
	}
	if err := validatePositiveID("postId", postID); err != nil {
		return nil, err
	}
	query := url.Values{}
	if params.Limit != nil {
		query.Set("limit", *params.Limit)
	}
	if params.Offset != nil {
		query.Set("offset", *params.Offset)
	}
	data, err := g.client.doRequest(ctx, requestParams{method: http.MethodGet, path: "/post/" + postID + "/comments", query: query, authScheme: authSchemeStandard, accessToken: q.AccessToken})
	if err != nil {
		return nil, err
	}
	var result PostCommentsResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: decode post comments response: %w", err)
	}
	return &result, nil
}

// Replies returns a page of replies for a top-level comment.
func (g *CommentGroup) Replies(ctx context.Context, q Context, commentID string, params generated.CommentRepliesParams) (*CommentRepliesResult, error) {
	if err := requireAccessToken(q, "commentReplies"); err != nil {
		return nil, err
	}
	if err := validatePositiveID("commentId", commentID); err != nil {
		return nil, err
	}
	query := url.Values{}
	if params.Limit != nil {
		query.Set("limit", *params.Limit)
	}
	if params.Cursor != nil {
		query.Set("cursor", *params.Cursor)
	}
	data, err := g.client.doRequest(ctx, requestParams{method: http.MethodGet, path: "/comment/" + commentID + "/replies", query: query, authScheme: authSchemeStandard, accessToken: q.AccessToken})
	if err != nil {
		return nil, err
	}
	var result CommentRepliesResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: decode comment replies response: %w", err)
	}
	return &result, nil
}
func (g *CommentGroup) postID(ctx context.Context, q Context, operationID, path string, b any) (*CommentCreateResult, error) {
	if err := requireAccessToken(q, operationID); err != nil {
		return nil, err
	}
	data, err := g.client.doRequest(ctx, requestParams{method: http.MethodPost, path: path, jsonBody: b, authScheme: authSchemeStandard, accessToken: q.AccessToken})
	if err != nil {
		return nil, err
	}
	var v CommentCreateResult
	if err = json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("qdmp: decode comment response: %w", err)
	}
	return &v, nil
}

func validatePositiveID(name, value string) error {
	if !positiveIDPattern.MatchString(value) {
		return fmt.Errorf("qdmp: %s must be a positive integer ID", name)
	}
	return nil
}
