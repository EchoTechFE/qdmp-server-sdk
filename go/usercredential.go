package qdmp

// Automatic renewal of a user-level credential ("用户授权凭证").
//
// A user access token lives 7200 seconds; the refresh token that renews it
// lives 30 days. Rather than making every caller notice expiry themselves,
// WithUserCredential derives a *UserClient that renews on two triggers:
//
//   - before a business call, when the known expiry is within
//     tokenRefreshBufferSeconds (the same 300s buffer the app-level
//     credential uses);
//   - after a business call comes back HTTP 401 with business code 10005
//     ("access_token 格式错误或已吊销") or 10006 ("access_token 超过有效期"),
//     in which case the original request is retried exactly once.
//
// Nothing else renews. A 5xx, a transport error, an unrelated business
// error, or even a 401 carrying some other code are all handed back to the
// caller untouched — renewing on those would burn refresh attempts on
// failures a new token cannot fix.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// The two business codes that mean "this access token is no longer usable,
// get a new one" (see shared/generated/error-codes.json). Both are reported
// with HTTP 401.
const (
	codeAccessTokenRevoked = "10005"
	codeAccessTokenExpired = "10006"
)

// UserCredentialOptions configures Client.WithUserCredential.
type UserCredentialOptions struct {
	// AccessToken is the current user access token. Required.
	AccessToken string
	// RefreshToken is the token used to renew AccessToken. Required: without
	// it renewal is impossible, and a caller holding only an access token
	// wants WithAccessToken instead.
	RefreshToken string
	// ExpiresAt is AccessToken's absolute expiry as Unix seconds in a string,
	// exactly as the server sent it. Optional: leaving it empty means "expiry
	// unknown", which skips proactive renewal entirely and leaves a 401 as
	// the only renewal trigger.
	ExpiresAt string
	// OnRefresh, when set, is called after a successful renewal has already
	// been applied to the client's own state, so the caller can persist the
	// new token. It receives the context of the business call that triggered
	// the renewal. Returning an error fails that business call with that
	// error; the renewed token is kept regardless (it has already been issued
	// — discarding it would only force another renewal).
	OnRefresh func(ctx context.Context, token RefreshedUserToken) error
}

// RefreshedUserToken is what a renewal produced. It carries no refreshToken
// because POST /auth/v1/refresh does not issue one: the original refresh
// token stays valid and unchanged.
type RefreshedUserToken struct {
	AccessToken string
	ExpiresAt   string
}

// UserCredential is the read-only snapshot returned by UserClient.Credential.
type UserCredential struct {
	AccessToken  string
	RefreshToken string
	// ExpiresAt is the wire-shaped absolute Unix-seconds string, or empty
	// when the expiry is unknown.
	ExpiresAt string
}

// String implements fmt.Stringer so an incidental debug print of a
// credential never puts a live token in a log line. Same rationale (and same
// [REDACTED] placeholder) as UserAccessTokenResult.String.
func (o UserCredentialOptions) String() string {
	return fmt.Sprintf(
		"UserCredentialOptions{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OnRefresh:%s}",
		"[REDACTED]", "[REDACTED]", o.ExpiresAt, callbackPresence(o.OnRefresh),
	)
}

// GoString implements fmt.GoStringer — %#v does not consult fmt.Stringer.
func (o UserCredentialOptions) GoString() string {
	return fmt.Sprintf(
		"qdmp.UserCredentialOptions{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OnRefresh:%s}",
		"[REDACTED]", "[REDACTED]", o.ExpiresAt, callbackPresence(o.OnRefresh),
	)
}

// LogValue implements slog.LogValuer — log/slog consults neither Stringer
// nor GoStringer.
func (o UserCredentialOptions) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("RefreshToken", "[REDACTED]"),
		slog.String("ExpiresAt", o.ExpiresAt),
		slog.String("OnRefresh", callbackPresence(o.OnRefresh)),
	)
}

// String implements fmt.Stringer — see UserCredentialOptions.String for why.
func (t RefreshedUserToken) String() string {
	return fmt.Sprintf("RefreshedUserToken{AccessToken:%q, ExpiresAt:%q}", "[REDACTED]", t.ExpiresAt)
}

// GoString implements fmt.GoStringer — see UserCredentialOptions.GoString for why.
func (t RefreshedUserToken) GoString() string {
	return fmt.Sprintf("qdmp.RefreshedUserToken{AccessToken:%q, ExpiresAt:%q}", "[REDACTED]", t.ExpiresAt)
}

// LogValue implements slog.LogValuer — see UserCredentialOptions.LogValue for why.
func (t RefreshedUserToken) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("ExpiresAt", t.ExpiresAt),
	)
}

// String implements fmt.Stringer — see UserCredentialOptions.String for why.
func (c UserCredential) String() string {
	return fmt.Sprintf(
		"UserCredential{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q}",
		"[REDACTED]", "[REDACTED]", c.ExpiresAt,
	)
}

// GoString implements fmt.GoStringer — see UserCredentialOptions.GoString for why.
func (c UserCredential) GoString() string {
	return fmt.Sprintf(
		"qdmp.UserCredential{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q}",
		"[REDACTED]", "[REDACTED]", c.ExpiresAt,
	)
}

// LogValue implements slog.LogValuer — see UserCredentialOptions.LogValue for why.
func (c UserCredential) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("RefreshToken", "[REDACTED]"),
		slog.String("ExpiresAt", c.ExpiresAt),
	)
}

// callbackPresence describes a callback without printing its code pointer,
// which %v would otherwise render as a bare memory address.
func callbackPresence(fn func(ctx context.Context, token RefreshedUserToken) error) string {
	if fn == nil {
		return "<nil>"
	}
	return "<set>"
}

// WithUserCredential derives a *UserClient that renews its own user access
// token: proactively when the known expiry is within
// tokenRefreshBufferSeconds, and reactively on an HTTP 401 carrying business
// code 10005/10006 (after which the original request is retried once).
//
// The business group methods are exactly the same as on a WithAccessToken
// client — renewal happens underneath them, in the transport. Options are
// validated on the first business call rather than here, so a missing
// AccessToken/RefreshToken surfaces from the call site as
// ErrAccessTokenRequired/ErrRefreshTokenRequired instead of from a
// constructor that cannot return an error.
func (c *Client) WithUserCredential(opts UserCredentialOptions) *UserClient {
	cred := &userCredential{
		client:       c,
		onRefresh:    opts.OnRefresh,
		accessToken:  opts.AccessToken,
		refreshToken: opts.RefreshToken,
	}
	cred.setExpiresAt(opts.ExpiresAt)

	uc := c.WithAccessToken(opts.AccessToken)
	uc.cred = cred
	return uc
}

// userRefreshInflight is one renewal that concurrent business calls can wait
// on instead of each issuing their own (single-flight).
type userRefreshInflight struct {
	done  chan struct{}
	token string
	gen   uint64
	err   error
}

// userCredential is the mutable state behind a WithUserCredential client.
//
// generation is what makes "renew once" safe under concurrency: a business
// call remembers the generation of the token it sent, and a renewal request
// carrying a stale generation is answered with the token someone else has
// already fetched instead of issuing a second one. Without it, two calls
// that both got a 401 with the same expired token would renew twice — the
// second one throwing away a perfectly good token the first had just
// obtained.
type userCredential struct {
	client    *Client
	onRefresh func(ctx context.Context, token RefreshedUserToken) error

	mu            sync.Mutex
	accessToken   string
	refreshToken  string
	expiresAt     string
	expiresAtUnix int64
	hasExpiry     bool
	generation    uint64
	inflight      *userRefreshInflight
}

// setExpiresAt records a wire expiresAt string. An unparseable or negative
// value is treated exactly like an absent one — expiry unknown — so a
// malformed timestamp degrades to "renew only on 401" rather than either
// renewing on every single call or trusting a nonsense expiry.
func (c *userCredential) setExpiresAt(raw string) {
	c.expiresAt = raw
	if raw == "" {
		c.expiresAtUnix, c.hasExpiry = 0, false
		return
	}
	parsed, err := parseExpiresAt(raw)
	if err != nil {
		c.expiresAtUnix, c.hasExpiry = 0, false
		return
	}
	c.expiresAtUnix, c.hasExpiry = parsed, true
}

// snapshot returns an independent copy of the current credential state.
func (c *userCredential) snapshot() UserCredential {
	c.mu.Lock()
	defer c.mu.Unlock()
	return UserCredential{
		AccessToken:  c.accessToken,
		RefreshToken: c.refreshToken,
		ExpiresAt:    c.expiresAt,
	}
}

// current reports the token to send next, the generation it belongs to, and
// whether it is close enough to expiry to renew first. All three are read
// under one lock so the generation always describes the token returned
// alongside it.
func (c *userCredential) current() (token string, gen uint64, stale bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	token, gen = c.accessToken, c.generation
	stale = c.hasExpiry && c.expiresAtUnix-tokenRefreshBufferSeconds <= time.Now().Unix()
	return token, gen, stale
}

// do runs one business request with renewal wrapped around it: renew first
// if the token is about to expire, then send; and if the response says the
// token was rejected, renew and retry exactly once.
func (c *userCredential) do(ctx context.Context, p requestParams) (json.RawMessage, error) {
	token, gen, stale := c.current()
	if stale {
		var err error
		token, gen, err = c.refresh(ctx, gen)
		if err != nil {
			return nil, err
		}
	}

	p.accessToken = token
	data, err := c.client.doRequest(ctx, p)
	if err == nil || !isExpiredAccessTokenError(err) {
		return data, err
	}

	// The token was rejected. Renew once and replay the request; whatever the
	// replay returns is final — no second renewal, no second retry.
	retryToken, _, refreshErr := c.refresh(ctx, gen)
	if refreshErr != nil {
		// The renewal itself failed (e.g. code 10008, the refresh token is
		// gone too). Surface that, not the original 401: the caller has to
		// re-run the authorization-code flow, and retrying the business
		// request would be pointless.
		return nil, refreshErr
	}
	p.accessToken = retryToken
	return c.client.doRequest(ctx, p)
}

// isExpiredAccessTokenError reports whether an error is specifically "the
// access token was rejected" — HTTP 401 *and* business code 10005/10006.
// Both halves matter: HTTP status is not authoritative on this API, and a
// 401 carrying any other code means something a new token would not fix.
func isExpiredAccessTokenError(err error) bool {
	var apiErr *QdmpApiError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		return false
	}
	return apiErr.Code == codeAccessTokenRevoked || apiErr.Code == codeAccessTokenExpired
}

// refresh renews the access token, collapsing concurrent callers into a
// single exchange, and returns the new token with its generation.
//
// seenGen is the generation of the token that prompted this call. If the
// stored generation has already moved past it, someone else's renewal has
// landed in the meantime and its token is returned as-is — this is what
// keeps a burst of concurrent 401s (or a burst of cold-start proactive
// renewals) down to one exchange.
func (c *userCredential) refresh(ctx context.Context, seenGen uint64) (string, uint64, error) {
	c.mu.Lock()
	if c.generation != seenGen {
		token, gen := c.accessToken, c.generation
		c.mu.Unlock()
		return token, gen, nil
	}
	if existing := c.inflight; existing != nil {
		c.mu.Unlock()
		select {
		case <-existing.done:
			return existing.token, existing.gen, existing.err
		case <-ctx.Done():
			return "", 0, ctx.Err()
		}
	}
	inflight := &userRefreshInflight{done: make(chan struct{})}
	c.inflight = inflight
	refreshToken := c.refreshToken
	c.mu.Unlock()

	token, gen, err := c.exchange(ctx, refreshToken)

	c.mu.Lock()
	c.inflight = nil
	c.mu.Unlock()

	inflight.token, inflight.gen, inflight.err = token, gen, err
	close(inflight.done)
	return token, gen, err
}

// exchange performs the actual renewal: POST /auth/v1/refresh with the
// original refresh token and no access-token header, then applies the result
// to this credential and notifies OnRefresh.
//
// Ordering matters here. Internal state is updated *before* OnRefresh runs,
// and is deliberately not rolled back when OnRefresh fails: the server has
// already issued the new token and invalidated nothing, so discarding it
// because the caller's own persistence failed would just force another
// renewal on the next call. The OnRefresh error still fails this business
// call, so the caller does learn that persistence failed.
func (c *userCredential) exchange(ctx context.Context, refreshToken string) (string, uint64, error) {
	result, err := c.client.Auth.RefreshToken(ctx, refreshToken)
	if err != nil {
		return "", 0, err
	}

	c.mu.Lock()
	c.accessToken = result.AccessToken
	c.setExpiresAt(result.ExpiresAt)
	// The refresh token is intentionally left alone: /auth/v1/refresh returns
	// no new one, and the old one stays valid.
	c.generation++
	gen := c.generation
	c.mu.Unlock()

	if c.onRefresh != nil {
		if err := c.onRefresh(ctx, RefreshedUserToken{
			AccessToken: result.AccessToken,
			ExpiresAt:   result.ExpiresAt,
		}); err != nil {
			return "", 0, err
		}
	}
	return result.AccessToken, gen, nil
}
