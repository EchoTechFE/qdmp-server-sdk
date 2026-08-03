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

// UserAccessTokenResult is the one-shot AUTHORIZATION_CODE exchange result.
// The SDK never caches this — the caller's own session/DB persists it,
// scoped to whichever end-user the code belonged to.
type UserAccessTokenResult struct {
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

// AppAccessTokenResult is the CLIENT_CREDENTIALS ("应用凭证") exchange result.
//
// Real traffic capture confirms this grant type returns the very same data
// shape as the user-level one — {accessToken, expiresAt, refreshToken,
// openId} — with a genuinely non-empty refreshToken and an empty openId
// (an app-level token belongs to no user). All four fields are handed back
// verbatim so the caller can see the real expiry and decide for itself
// whether to use the refreshToken; the SDK's own renewal strategy for this
// credential is unchanged (re-exchange via CLIENT_CREDENTIALS ahead of
// expiry, never via refreshToken).
//
// Validation is deliberately strict only on accessToken/expiresAt: openId
// is empty by design here, and refreshToken is passed through as received
// without being required.
type AppAccessTokenResult struct {
	AccessToken string `json:"accessToken"`
	// ExpiresAt is the absolute Unix-seconds expiry timestamp as a string,
	// matching the wire shape (see UserAccessTokenResult.ExpiresAt).
	ExpiresAt    string `json:"expiresAt"`
	RefreshToken string `json:"refreshToken"`
	OpenID       string `json:"openId"`
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
// up for *UserAccessTokenResult (a pointer's method set includes all
// value-receiver methods of the pointed-to type).
//
// This must never affect json.Marshal(result): encoding/json only consults
// the json.Marshaler interface (MarshalJSON), not fmt.Stringer, so callers
// that persist the real values via JSON — the entire point of
// GetUserAccessToken/RefreshToken existing — are unaffected. Direct field access
// (result.AccessToken) is likewise untouched.
func (r UserAccessTokenResult) String() string {
	return fmt.Sprintf(
		"UserAccessTokenResult{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OpenID:%q}",
		"[REDACTED]", "[REDACTED]", r.ExpiresAt, r.OpenID,
	)
}

// String implements fmt.Stringer — see UserAccessTokenResult.String for why.
func (r AppAccessTokenResult) String() string {
	return fmt.Sprintf(
		"AppAccessTokenResult{AccessToken:%q, ExpiresAt:%q, RefreshToken:%q, OpenID:%q}",
		"[REDACTED]", r.ExpiresAt, "[REDACTED]", r.OpenID,
	)
}

// GoString implements fmt.GoStringer — see UserAccessTokenResult.GoString for why.
func (r AppAccessTokenResult) GoString() string {
	return fmt.Sprintf(
		"qdmp.AppAccessTokenResult{AccessToken:%q, ExpiresAt:%q, RefreshToken:%q, OpenID:%q}",
		"[REDACTED]", r.ExpiresAt, "[REDACTED]", r.OpenID,
	)
}

// LogValue implements slog.LogValuer — see UserAccessTokenResult.LogValue for why.
func (r AppAccessTokenResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("ExpiresAt", r.ExpiresAt),
		slog.String("RefreshToken", "[REDACTED]"),
		slog.String("OpenID", r.OpenID),
	)
}

// String implements fmt.Stringer — see UserAccessTokenResult.String for why.
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
func (r UserAccessTokenResult) GoString() string {
	return fmt.Sprintf(
		"qdmp.UserAccessTokenResult{AccessToken:%q, RefreshToken:%q, ExpiresAt:%q, OpenID:%q}",
		"[REDACTED]", "[REDACTED]", r.ExpiresAt, r.OpenID,
	)
}

// GoString implements fmt.GoStringer — see UserAccessTokenResult.GoString for why.
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
func (r UserAccessTokenResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("RefreshToken", "[REDACTED]"),
		slog.String("ExpiresAt", r.ExpiresAt),
		slog.String("OpenID", r.OpenID),
	)
}

// LogValue implements slog.LogValuer — see UserAccessTokenResult.LogValue for why.
func (r RefreshTokenResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AccessToken", "[REDACTED]"),
		slog.String("ExpiresAt", r.ExpiresAt),
	)
}

// authInflight represents one in-flight app-level token exchange that
// concurrent GetAppAccessToken callers can wait on instead of each starting
// their own HTTP call (single-flight). The result is held by value so every
// waiter gets its own independent *AppAccessTokenResult copy.
type authInflight struct {
	done   chan struct{}
	result AppAccessTokenResult
	err    error
}

// value converts one in-flight outcome into this caller's own result.
func (f *authInflight) value() (*AppAccessTokenResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := f.result
	return &result, nil
}

// appResultFromEntry builds the caller-facing app credential from a cached
// TokenEntry. Both the cache-hit path and the fresh-exchange path go through
// this one function, so a caller can never tell the two apart by looking at
// which fields are populated.
func appResultFromEntry(entry TokenEntry) AppAccessTokenResult {
	return AppAccessTokenResult{
		AccessToken:  entry.AccessToken,
		ExpiresAt:    strconv.FormatInt(entry.ExpiresAt, 10),
		RefreshToken: entry.RefreshToken,
		OpenID:       entry.OpenID,
	}
}

// AuthService implements the qdmp OAuth2-flavored auth endpoints.
//
// GetAppAccessToken manages the app-level (CLIENT_CREDENTIALS) token: cached
// in TokenStore, refreshed automatically ahead of expiry, with concurrent
// callers collapsed into a single outbound HTTP exchange (single-flight) to
// avoid a thundering herd against the real qdmp gateway.
//
// GetUserAccessToken and RefreshToken are one-shot exchanges the SDK never
// caches — user-level tokens are the calling application's responsibility
// to persist, since a single server process serves many end users and the
// SDK has no way to know which user a given call belongs to.
type AuthService struct {
	client *Client
	store  TokenStore

	mu       sync.Mutex
	inflight *authInflight
}

// GetAppAccessToken returns a valid app-level ("应用凭证") credential, using
// the cached value if it is not within tokenRefreshBufferSeconds of expiry,
// otherwise exchanging for a new one (collapsing concurrent callers via
// single-flight).
func (a *AuthService) GetAppAccessToken(ctx context.Context) (*AppAccessTokenResult, error) {
	if result, ok := a.cachedValid(); ok {
		return result, nil
	}
	return a.refreshShared(ctx)
}

func (a *AuthService) cachedValid() (*AppAccessTokenResult, bool) {
	entry, ok := a.store.Get()
	if !ok {
		return nil, false
	}
	now := time.Now().Unix()
	if entry.ExpiresAt-tokenRefreshBufferSeconds <= now {
		return nil, false
	}
	result := appResultFromEntry(entry)
	return &result, true
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
func (a *AuthService) refreshShared(ctx context.Context) (*AppAccessTokenResult, error) {
	a.mu.Lock()
	if a.inflight != nil {
		existing := a.inflight
		a.mu.Unlock()
		select {
		case <-existing.done:
			return existing.value()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Re-check the cache under the lock: another goroutine may have
	// completed a refresh (and cleared a.inflight) between our caller's
	// lock-free cachedValid() check and this point, in which case starting
	// a brand new exchange here would be a redundant, avoidable request.
	if result, ok := a.cachedValid(); ok {
		a.mu.Unlock()
		return result, nil
	}

	inflight := &authInflight{done: make(chan struct{})}
	a.inflight = inflight
	a.mu.Unlock()

	go func() {
		result, err := a.exchangeClientCredentials(context.Background())

		a.mu.Lock()
		a.inflight = nil
		a.mu.Unlock()

		if result != nil {
			inflight.result = *result
		}
		inflight.err = err
		close(inflight.done)
	}()

	select {
	case <-inflight.done:
		return inflight.value()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *AuthService) exchangeClientCredentials(ctx context.Context) (*AppAccessTokenResult, error) {
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
		return nil, err
	}

	// Only accessToken/expiresAt are validated: an app-level credential's
	// openId is an empty string by design (it belongs to no user), and its
	// refreshToken — non-empty in practice — is passed through as received
	// rather than being required, since the SDK never renews this credential
	// with it (see AppAccessTokenResult).
	var result AppAccessTokenResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode auth.token response: %w", err)
	}

	if result.AccessToken == "" {
		return nil, fmt.Errorf("qdmp: auth.token response has an empty accessToken")
	}

	expiresAt, err := parseExpiresAt(result.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("qdmp: auth.token response has an invalid expiresAt: %w", err)
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
		return nil, fmt.Errorf("qdmp: auth.token response's expiresAt (%d) leaves less than the required %ds before expiry", expiresAt, tokenRefreshBufferSeconds)
	}

	entry := TokenEntry{
		AccessToken:  result.AccessToken,
		ExpiresAt:    expiresAt,
		RefreshToken: result.RefreshToken,
		OpenID:       result.OpenID,
	}
	a.store.Set(entry)
	// Build the returned value from the entry that was just cached, so this
	// path and the cache-hit path are the same code producing the same
	// fields.
	cached := appResultFromEntry(entry)
	return &cached, nil
}

// parseExpiresAt parses a wire expiresAt string (see UserAccessTokenResult's
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

// GetUserAccessToken performs a one-shot AUTHORIZATION_CODE exchange. Never
// cached — see the AuthService doc comment.
func (a *AuthService) GetUserAccessToken(ctx context.Context, code string) (*UserAccessTokenResult, error) {
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

	var result UserAccessTokenResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("qdmp: failed to decode auth.getUserAccessToken response: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("qdmp: auth.getUserAccessToken response has an empty accessToken")
	}
	if result.RefreshToken == "" {
		return nil, fmt.Errorf("qdmp: auth.getUserAccessToken response has an empty refreshToken")
	}
	if result.OpenID == "" {
		return nil, fmt.Errorf("qdmp: auth.getUserAccessToken response has an empty openId")
	}
	expiresAt, err := parseExpiresAt(result.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("qdmp: auth.getUserAccessToken response has an invalid expiresAt: %w", err)
	}
	// A non-negative expiresAt can still already be in the past (e.g. "1",
	// 1970-01-01) — parseExpiresAt only rejects negative/non-numeric values.
	// Handing back a session whose access token is already dead on arrival
	// would let a caller persist (or start relying on) a permanently-broken
	// session, same reasoning as the app-level cached path.
	if expiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("qdmp: auth.getUserAccessToken response has an already-expired expiresAt (%d)", expiresAt)
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
	// See the identical check in GetUserAccessToken: a non-negative but
	// already-expired expiresAt must not be handed back as a usable result.
	if expiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("qdmp: auth.refreshToken response has an already-expired expiresAt (%d)", expiresAt)
	}
	return &result, nil
}
