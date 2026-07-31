package qdmp

import (
	"errors"
	"fmt"
)

// ErrAccessTokenRequired is a sentinel error returned when a business method
// that requires a user/app-level access token (x-qdmp-token-required=true in
// shared/openapi.yaml) is called without one. Callers should use errors.Is
// to detect this case. The SDK fails locally, before any HTTP request is
// sent, whenever this error is returned.
var ErrAccessTokenRequired = errors.New("qdmp: access token is required")

// QdmpApiError represents a business-level failure reported by the qdmp
// OpenAPI, as opposed to a transport-level failure (network error, etc).
//
// This repository has confirmed by real traffic capture that HTTP status is
// not authoritative: a refreshToken that has expired is HTTP 200 with
// code=10008, while an invalid access-token is a genuine HTTP 401 with
// code=10005. Callers must inspect Code, not just check for a non-nil error.
//
// Code is intentionally a string (not a closed enum/int): the qdmp API is
// known to use non-numeric-looking business codes in one envelope shape and
// small gRPC-style integer codes in the other, and undocumented codes have
// already been observed in practice (10005, 20000, 13, 2, ...).
type QdmpApiError struct {
	// Code is the normalized business error code (e.g. "10005", "13").
	Code string
	// Message is the human-readable message reported by the server.
	Message string
	// RequestID is the request tracing ID, when present (only the "normal"
	// business envelope {code,message,requestId,data} carries one; the
	// gateway-style envelope {code,message,details} does not).
	RequestID string
	// HTTPStatus is the transport-level HTTP status code of the response
	// that carried this business error.
	HTTPStatus int
}

// Error implements the error interface. It intentionally only ever
// interpolates fields already stored on QdmpApiError (Code/Message/
// RequestID/HTTPStatus) — accessToken and appSecret are never stored on this
// type, so they can never leak through this method.
func (e *QdmpApiError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("qdmp: request failed (code=%s, httpStatus=%d, requestId=%s): %s", e.Code, e.HTTPStatus, e.RequestID, e.Message)
	}
	return fmt.Sprintf("qdmp: request failed (code=%s, httpStatus=%d): %s", e.Code, e.HTTPStatus, e.Message)
}
