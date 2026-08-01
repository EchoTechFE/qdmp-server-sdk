package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// String implements fmt.Stringer so that accidental debug-printing (e.g.
// fmt.Println(result), fmt.Printf("%v"/"%+v", result), or most structured
// loggers that fall back to %v/%+v) never leaks the raw AccessToken/
// RefreshToken values. This is a value-receiver method, so it is also picked
// up for *Code2SessionResult (a pointer's method set includes all
// value-receiver methods of the pointed-to type).
//
// This must never affect json.Marshal(result): encoding/json only consults
// the json.Marshaler interface (MarshalJSON), not fmt.Stringer, so callers
// that persist the real values via JSON — the entire point of
// Code2Session/RefreshToken existing — are unaffected. Direct field access
// (result.AccessToken) is likewise untouched.
func (r Code2SessionResult) String() string {
	return fmt.Sprintf(
		"Code2SessionResult{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OpenID:%q}",
		"[REDACTED]", "[REDACTED]", r.ExpiresAt, r.OpenID,
	)
}

// String implements fmt.Stringer — see Code2SessionResult.String for why.
func (r RefreshTokenResult) String() string {
	return fmt.Sprintf(
		"RefreshTokenResult{AccessToken:%q, ExpiresAt:%q}",
		"[REDACTED]", r.ExpiresAt,
	)
}

// GoString implements fmt.GoStringer. The %#v verb does NOT consult
// fmt.Stringer — it falls back to Go-syntax struct reflection, which would
// print every exported field (including the raw AccessToken/RefreshToken)
// even with String() defined above. GoStringer is the only way to also
// redact under %#v.
func (r Code2SessionResult) GoString() string {
	return fmt.Sprintf(
		"qdmp.Code2SessionResult{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OpenID:%q}",
		"[REDACTED]", "[REDACTED]", r.ExpiresAt, r.OpenID,
	)
}

// GoString implements fmt.GoStringer — see Code2SessionResult.GoString for why.
func (r RefreshTokenResult) GoString() string {
	return fmt.Sprintf(
		"qdmp.RefreshTokenResult{AccessToken:%q, ExpiresAt:%q}",
		"[REDACTED]", r.ExpiresAt,
	)
}

// LogValue implements slog.LogValuer. Structured loggers built on log/slog
// (e.g. slog.JSONHandler, slog.TextHandler) do NOT consult fmt.Stringer —
// slog.JSONHandler JSON-marshals values directly, which would still emit
// the raw AccessToken/RefreshToken fields even with String()/GoString()
// already redacting the fmt-based paths above. LogValuer is slog's own
// redaction hook; it has no effect on json.Marshal (persistence stays
// exactly as before).
func (r Code2SessionResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("RefreshToken", "[REDACTED]"),
		slog.String("ExpiresAt", r.ExpiresAt),
		slog.String("OpenID", r.OpenID),
	)
}

// LogValue implements slog.LogValuer — see Code2SessionResult.LogValue for why.
func (r RefreshTokenResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("ExpiresAt", r.ExpiresAt),
	)
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

	expiresAt, err := parseExpiresAt(result.ExpiresAt)
	if err != nil {
		return "", fmt.Errorf("qdmp: auth.token response has an invalid expiresAt: %w", err)
	}
	// A non-negative expiresAt can still be already expired, or expiring
	// within tokenRefreshBufferSeconds (e.g. "1", 1970-01-01, or "a few
	// seconds from now") — parseExpiresAt only rejects negative/non-numeric
	// values. Accepting such a value here would cache it and hand it back to
	// this call's own caller as if it were a usably-fresh token, even though
	// cachedValid uses the same buffer and would immediately (and correctly)
	// treat that same cached entry as not-fresh on the very next call. Apply
	// the identical predicate here so "fresh enough to hand out" means the
	// same thing on both the write and read paths.
	if expiresAt-tokenRefreshBufferSeconds <= time.Now().Unix() {
		return "", fmt.Errorf("qdmp: auth.token response's expiresAt (%d) leaves less than the required %ds before expiry", expiresAt, tokenRefreshBufferSeconds)
	}

	a.store.Set(TokenEntry{AccessToken: result.AccessToken, ExpiresAt: expiresAt})
	return result.AccessToken, nil
}

// parseExpiresAt parses a wire expiresAt string (see Code2SessionResult's
// doc comment for why it is a string on the wire) as a non-negative absolute
// Unix-seconds timestamp. Anything that fails to parse as a base-10 int64,
// or that parses to a negative value, is rejected outright: a
// negative/corrupted expiresAt must never be treated as a usable expiry
// (whether cached or returned directly to a one-shot caller), rather than
// silently limping along.
func parseExpiresAt(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("non-numeric expiresAt %q: %w", s, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("negative expiresAt %q", s)
	}
	return v, nil
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
	if result.RefreshToken == "" {
		return nil, fmt.Errorf("qdmp: auth.code2Session response has an empty refreshToken")
	}
	if result.OpenID == "" {
		return nil, fmt.Errorf("qdmp: auth.code2Session response has an empty openId")
	}
	expiresAt, err := parseExpiresAt(result.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("qdmp: auth.code2Session response has an invalid expiresAt: %w", err)
	}
	// A non-negative expiresAt can still already be in the past (e.g. "1",
	// 1970-01-01) — parseExpiresAt only rejects negative/non-numeric values.
	// Handing back a session whose access token is already dead on arrival
	// would let a caller persist (or start relying on) a permanently-broken
	// session, same reasoning as the app-level cached path.
	if expiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("qdmp: auth.code2Session response has an already-expired expiresAt (%d)", expiresAt)
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
	expiresAt, err := parseExpiresAt(result.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("qdmp: auth.refreshToken response has an invalid expiresAt: %w", err)
	}
	// See the identical check in Code2Session: a non-negative but
	// already-expired expiresAt must not be handed back as a usable result.
	if expiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("qdmp: auth.refreshToken response has an already-expired expiresAt (%d)", expiresAt)
	}
	return &result, nil
}
