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

// ErrInvalidAccessToken is a sentinel error returned when a caller-supplied
// access token contains a byte that cannot be safely sent as an HTTP header
// value (any byte <= 0x1F, or 0x7F/DEL — including a bare tab, 0x09, which
// Go's net/http would otherwise transmit as-is instead of rejecting it). The
// SDK fails locally, before any HTTP request is sent, whenever this error is
// returned.
var ErrInvalidAccessToken = errors.New("qdmp: access token contains a character that cannot be safely sent as an HTTP header value")

// ErrRefreshTokenRequired is a sentinel error returned when a client derived
// from WithUserCredential was given an empty RefreshToken. Renewal is the
// whole point of that constructor and is impossible without one, so this
// fails locally on the first business call, before any HTTP request — a
// caller that genuinely has only an access token wants WithAccessToken
// instead.
var ErrRefreshTokenRequired = errors.New("qdmp: refresh token is required")

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

// maxErrorFieldLength bounds how much of a server-supplied Message/
// RequestID sanitizeForErrorMessage will keep, so a misbehaving upstream
// can't make every log line built from a *QdmpApiError unbounded.
const maxErrorFieldLength = 2000

// sanitizeForErrorMessage neutralizes control characters (including raw
// CR/LF, which could otherwise be used to forge a fake extra "log line" when
// this value is later written to a log — classic log injection) and bounds
// the length of server-supplied text. It is applied to QdmpApiError's
// Message/RequestID fields themselves (see doRequest in request.go), rather
// than only inside Error(), so a caller reading these exported fields
// directly also only ever sees the sanitized value.
func sanitizeForErrorMessage(s string) string {
	runes := []rune(s)
	if len(runes) > maxErrorFieldLength {
		runes = runes[:maxErrorFieldLength]
	}
	for i, r := range runes {
		if r < 0x20 || r == 0x7F {
			runes[i] = ' '
		}
	}
	return string(runes)
}
