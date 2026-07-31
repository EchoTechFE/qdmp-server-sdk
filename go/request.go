package qdmp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// authScheme selects which pair of auth headers a request uses, driven by
// the x-qdmp-auth-scheme extension field (see shared/generated/route-meta.json).
// Only "standard" sends the x-echo-qdmp-version header; "genai" uses its own
// distinct header pair and never sends "standard"'s headers; "noAuth" sends
// neither (used only by the auth.* endpoints, which authenticate via
// appId/appSecret in the request body instead).
type authScheme string

const (
	authSchemeNoAuth   authScheme = "noAuth"
	authSchemeStandard authScheme = "standard"
	authSchemeGenai    authScheme = "genai"
)

// requestParams describes one outbound call to request(). It intentionally
// has no notion of "token missing" handling: callers (the business group
// methods) are responsible for the fail-fast-locally check before ever
// constructing one of these, per the design's "token required is enforced at
// the language layer" rule.
type requestParams struct {
	method      string
	path        string
	query       url.Values
	jsonBody    any
	authScheme  authScheme
	accessToken string
}

// rawEnvelope captures the fields common to both confirmed response envelope
// shapes: {code, message, requestId, data} (normal business envelope) and
// {code, message, details} (gateway-layer error envelope, no data/requestId).
// Code is decoded as json.RawMessage because it has been observed on the
// wire as both a JSON number and a JSON string, and callers must compare
// against "0" regardless of which shape a given response used.
type rawEnvelope struct {
	Code      json.RawMessage `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"requestId"`
	Data      json.RawMessage `json:"data"`
}

// normalizeCode extracts the canonical string form of the envelope's code
// field without ever round-tripping through a float64, so integer codes
// keep their exact textual representation (e.g. "10008", not "10008.0" or
// something mangled by %g formatting of large numbers).
func normalizeCode(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("qdmp: response envelope is missing a code field")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", fmt.Errorf("qdmp: failed to parse string code field: %w", err)
		}
		return s, nil
	}
	return string(trimmed), nil
}

// doRequest performs one HTTP call against the qdmp OpenAPI and returns the
// raw `data` payload on success (code == "0"). On business failure it
// returns a *QdmpApiError; HTTP status is never treated as authoritative —
// only the normalized code field is, per the confirmed HTTP-200-can-still-
// fail / HTTP-401-is-also-possible facts in this repository's design.
func (c *Client) doRequest(ctx context.Context, p requestParams) (json.RawMessage, error) {
	reqURL := strings.TrimRight(c.baseURL, "/") + p.path
	if len(p.query) > 0 {
		reqURL += "?" + p.query.Encode()
	}

	var bodyReader io.Reader
	if p.jsonBody != nil {
		b, err := json.Marshal(p.jsonBody)
		if err != nil {
			return nil, fmt.Errorf("qdmp: failed to encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, p.method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("qdmp: failed to build request: %w", err)
	}
	if p.jsonBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	switch p.authScheme {
	case authSchemeStandard:
		req.Header.Set("access-token", p.accessToken)
		req.Header.Set("x-echo-qdmp-version", c.qdmpVersion)
	case authSchemeGenai:
		req.Header.Set("x-openapi-access-token", p.accessToken)
		req.Header.Set("x-openapi-app-id", c.appID)
	case authSchemeNoAuth:
		// No auth headers: auth.* endpoints authenticate via appId/appSecret
		// carried in the JSON request body instead.
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdmp: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qdmp: failed to read response body (http status %d): %w", resp.StatusCode, err)
	}

	var env rawEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode response envelope (http status %d): %w", resp.StatusCode, err)
	}

	code, err := normalizeCode(env.Code)
	if err != nil {
		return nil, err
	}

	if code != "0" {
		return nil, &QdmpApiError{
			Code:       code,
			Message:    env.Message,
			RequestID:  env.RequestID,
			HTTPStatus: resp.StatusCode,
		}
	}

	return env.Data, nil
}

// setIfNotNil sets a query parameter from an optional string pointer,
// leaving the parameter unset entirely when the pointer is nil (as opposed
// to setting it to an empty string), matching the openapi.yaml optional
// query parameter semantics.
func setIfNotNil(v url.Values, key string, val *string) {
	if val != nil {
		v.Set(key, *val)
	}
}

func setBoolIfNotNil(v url.Values, key string, val *bool) {
	if val != nil {
		if *val {
			v.Set(key, "true")
		} else {
			v.Set(key, "false")
		}
	}
}

func addAllIfNotNil(v url.Values, key string, vals *[]string) {
	if vals == nil {
		return
	}
	for _, item := range *vals {
		v.Add(key, item)
	}
}
