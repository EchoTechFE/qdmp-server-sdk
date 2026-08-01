package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/EchoTechFE/qdmp-server-sdk/go/generated"
)

// tokenRefreshBufferSeconds is how long before actual expiry the app-level
// token is proactively refreshed.
const tokenRefreshBufferSeconds = 300

// Code2SessionResult is the one-shot AUTHORIZATION_CODE exchange result.
// The SDK never caches this — the caller's own session/DB persists it,
// scoped to whichever end-user the code belonged to.
type Code2SessionResult struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	// ExpiresAt is the absolute Unix-seconds expiry timestamp, confirmed by
	// capture to be a *string* on the wire (e.g. "1785508950") even though
	// it is logically an int64. It is intentionally kept as a Go string
	// end-to-end so this round-trips byte-for-byte with no risk of a
	// numeric-conversion implementation silently corrupting it.
	ExpiresAt string `json:"expiresAt"`
	OpenID    string `json:"openId"`
}

// RefreshTokenResult is the one-shot refreshToken exchange result. Also
// never cached by the SDK.
type RefreshTokenResult struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// authInflight represents one in-flight app-level token exchange that
// concurrent GetAccessToken callers can wait on instead of each starting
// their own HTTP call (single-flight).
type authInflight struct {
	done  chan struct{}
	token string
	err   error
}

// AuthService implements the qdmp OAuth2-flavored auth endpoints.
//
// GetAccessToken manages the app-level (CLIENT_CREDENTIALS) token: cached
// in TokenStore, refreshed automatically ahead of expiry, with concurrent
// callers collapsed into a single outbound HTTP exchange (single-flight) to
// avoid a thundering herd against the real qdmp gateway.
//
// Code2Session and RefreshToken are one-shot exchanges the SDK never
// caches — user-level tokens are the calling application's responsibility
// to persist, since a single server process serves many end users and the
// SDK has no way to know which user a given call belongs to.
type AuthService struct {
	client *Client
	store  TokenStore

	mu       sync.Mutex
	inflight *authInflight
}

// GetAccessToken returns a valid app-level access token, using the cached
// value if it is not within tokenRefreshBufferSeconds of expiry, otherwise
// exchanging for a new one (collapsing concurrent callers via single-flight).
func (a *AuthService) GetAccessToken(ctx context.Context) (string, error) {
	if token, ok := a.cachedValid(); ok {
		return token, nil
	}
	return a.refreshShared(ctx)
}

func (a *AuthService) cachedValid() (string, bool) {
	entry, ok := a.store.Get()
	if !ok {
		return "", false
	}
	now := time.Now().Unix()
	if entry.ExpiresAt-tokenRefreshBufferSeconds <= now {
		return "", false
	}
	return entry.AccessToken, true
}

// refreshShared performs the single-flight-guarded app-level token
// exchange: the first caller in starts the real HTTP request, every other
// concurrent caller waits on the same result instead of issuing its own.
//
// Every waiter (leader included) selects on its own ctx.Done() rather than
// blocking unconditionally on the shared result: a caller whose context is
// cancelled or times out must return promptly regardless of how long the
// underlying exchange takes or whether some other caller's context is still
// healthy. To make that safe, the actual HTTP exchange itself runs on a
// context detached from any single caller (context.Background(), bounded by
// the http.Client's own timeout — see NewClient) so that one caller giving
// up does not fail the shared result for every other concurrent waiter, and
// so the leader's own cancellation can't get attributed to healthy waiters.
func (a *AuthService) refreshShared(ctx context.Context) (string, error) {
	a.mu.Lock()
	if a.inflight != nil {
		existing := a.inflight
		a.mu.Unlock()
		select {
		case <-existing.done:
			return existing.token, existing.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	// Re-check the cache under the lock: another goroutine may have
	// completed a refresh (and cleared a.inflight) between our caller's
	// lock-free cachedValid() check and this point, in which case starting
	// a brand new exchange here would be a redundant, avoidable request.
	if token, ok := a.cachedValid(); ok {
		a.mu.Unlock()
		return token, nil
	}

	inflight := &authInflight{done: make(chan struct{})}
	a.inflight = inflight
	a.mu.Unlock()

	go func() {
		token, err := a.exchangeClientCredentials(context.Background())

		a.mu.Lock()
		a.inflight = nil
		a.mu.Unlock()

		inflight.token = token
		inflight.err = err
		close(inflight.done)
	}()

	select {
	case <-inflight.done:
		return inflight.token, inflight.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *AuthService) exchangeClientCredentials(ctx context.Context) (string, error) {
	data, err := a.client.doRequest(ctx, requestParams{
		method:     http.MethodPost,
		path:       "/auth/v1/token",
		authScheme: authSchemeNoAuth,
		jsonBody: generated.AuthTokenJSONBody{
			GrantType: generated.CLIENTCREDENTIALS,
			AppId:     a.client.appID,
			AppSecret: a.client.appSecret,
		},
	})
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   string `json:"expiresAt"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("qdmp: failed to decode auth.token response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("qdmp: auth.token response has an empty accessToken")
	}

	expiresAt, err := strconv.ParseInt(result.ExpiresAt, 10, 64)
	if err != nil {
		return "", fmt.Errorf("qdmp: auth.token response has a non-numeric expiresAt: %w", err)
	}

	a.store.Set(TokenEntry{AccessToken: result.AccessToken, ExpiresAt: expiresAt})
	return result.AccessToken, nil
}

// Code2Session performs a one-shot AUTHORIZATION_CODE exchange. Never
// cached — see the AuthService doc comment.
func (a *AuthService) Code2Session(ctx context.Context, code string) (*Code2SessionResult, error) {
	data, err := a.client.doRequest(ctx, requestParams{
		method:     http.MethodPost,
		path:       "/auth/v1/token",
		authScheme: authSchemeNoAuth,
		jsonBody: generated.AuthTokenJSONBody{
			GrantType: generated.AUTHORIZATIONCODE,
			AppId:     a.client.appID,
			AppSecret: a.client.appSecret,
			Code:      &code,
		},
	})
	if err != nil {
		return nil, err
	}

	var result Code2SessionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode auth.code2Session response: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("qdmp: auth.code2Session response has an empty accessToken")
	}
	return &result, nil
}

// RefreshToken performs a one-shot refreshToken exchange. Never cached —
// see the AuthService doc comment. HTTP 200 does not imply success here:
// an expired refreshToken comes back as HTTP 200 + code=10008, which
// doRequest already turns into a *QdmpApiError.
func (a *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenResult, error) {
	data, err := a.client.doRequest(ctx, requestParams{
		method:     http.MethodPost,
		path:       "/auth/v1/refresh",
		authScheme: authSchemeNoAuth,
		jsonBody:   generated.AuthRefreshJSONBody{RefreshToken: refreshToken},
	})
	if err != nil {
		return nil, err
	}

	var result RefreshTokenResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode auth.refreshToken response: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("qdmp: auth.refreshToken response has an empty accessToken")
	}
	return &result, nil
}
