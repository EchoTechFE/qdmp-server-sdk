package qdmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultBaseURL is the production qdmp OpenAPI host (see
// shared/openapi.yaml servers[0]). Callers can override it via
// ClientOptions.BaseURL — tests always do, pointing at an httptest.Server.
const defaultBaseURL = "https://openapi.qiandao.com"

// defaultQdmpVersion is the default x-echo-qdmp-version value sent on every
// "standard" authScheme request, overridable via ClientOptions.QdmpVersion.
const defaultQdmpVersion = "1.0"

// defaultHTTPTimeout bounds every outbound request made by the default
// http.Client (used whenever ClientOptions.HTTPClient is nil). Without this,
// a hung token endpoint would block indefinitely: the default *http.Client
// has no timeout at all, which combined with AuthService's single-flight
// refresh means every concurrent GetAppAccessToken caller would pile up behind
// one stuck exchange with no way out other than each caller's own context
// deadline. Callers needing a different bound can still override it via
// ClientOptions.HTTPClient.
const defaultHTTPTimeout = 30 * time.Second

// ClientOptions configures NewClient.
type ClientOptions struct {
	// AppID is the qdmp application ID. Required.
	AppID string
	// AppSecret is the qdmp application secret, used only for the
	// CLIENT_CREDENTIALS/AUTHORIZATION_CODE token exchange. Required. Never
	// logged or attached to any error value produced by this SDK.
	AppSecret string
	// BaseURL overrides the qdmp OpenAPI host. Defaults to the production
	// host. Tests point this at an httptest.Server.
	BaseURL string
	// QdmpVersion overrides the x-echo-qdmp-version header value sent on
	// "standard" authScheme requests. Defaults to "1.0".
	QdmpVersion string
	// HTTPClient overrides the *http.Client used for outbound requests.
	// Defaults to a new client with a defaultHTTPTimeout (30s) request
	// timeout, so a hung qdmp endpoint can never block a caller forever. Its
	// CheckRedirect is always overridden by NewClient (on a shallow copy,
	// never mutating the caller's original client) to refuse redirects --
	// see the comment in NewClient for why this can't be left configurable.
	HTTPClient *http.Client
	// TokenStore overrides the app-level access token cache. Defaults to an
	// in-process memory store. See TokenStore for why a pluggable store
	// exists (multi-instance deployments).
	TokenStore TokenStore
}

// Client is the root qdmp SDK client. It exposes Auth (app-level token
// exchange, using appId/appSecret — no user access token needed) and
// WithAccessToken (which derives a client bound to a specific user or
// app-level access token for calling business endpoints).
//
// The root Client intentionally does NOT expose business group methods
// (User/Island/Spu/...) directly: every business operation requires an
// access token (x-qdmp-token-required=true for all of them per
// shared/generated/route-meta.json), and Go has no way to make a plain
// method parameter as unskippable as a derived client — so token-required
// business calls only exist on the object returned by WithAccessToken.
type Client struct {
	appID       string
	appSecret   string
	baseURL     string
	qdmpVersion string
	httpClient  *http.Client

	// Auth manages the app-level (CLIENT_CREDENTIALS) token lifecycle
	// (cached + single-flight refresh) plus the one-shot,
	// never-cached getUserAccessToken/refreshToken exchanges.
	Auth *AuthService
}

// NewClient constructs a qdmp SDK client from the given options.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.AppID == "" {
		return nil, fmt.Errorf("qdmp: ClientOptions.AppID is required")
	}
	if opts.AppSecret == "" {
		return nil, fmt.Errorf("qdmp: ClientOptions.AppSecret is required")
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if err := validateBaseURLScheme(baseURL); err != nil {
		return nil, err
	}
	qdmpVersion := opts.QdmpVersion
	if qdmpVersion == "" {
		qdmpVersion = defaultQdmpVersion
	}
	// Never follow a redirect: a compromised/misconfigured gateway (or an
	// attacker able to influence a response's Location header) must not be
	// able to make this client replay the access-token header (and any other
	// request state) against an arbitrary host. http.ErrUseLastResponse makes
	// Client.Do return the 3xx response itself instead of chasing Location;
	// doRequest (see request.go) then rejects any 3xx status explicitly.
	//
	// This is enforced even on a caller-supplied ClientOptions.HTTPClient: we
	// shallow-copy it and override CheckRedirect on the copy (never mutating
	// the caller's original *http.Client), so injecting a custom transport or
	// proxy can never silently reintroduce the redirect-leak vulnerability by
	// omission.
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout, CheckRedirect: checkRedirect}
	} else {
		copied := *httpClient
		copied.CheckRedirect = checkRedirect
		httpClient = &copied
	}
	store := opts.TokenStore
	if store == nil {
		store = NewMemoryTokenStore()
	}

	c := &Client{
		appID:       opts.AppID,
		appSecret:   opts.AppSecret,
		baseURL:     baseURL,
		qdmpVersion: qdmpVersion,
		httpClient:  httpClient,
	}
	c.Auth = &AuthService{client: c, store: store}
	return c, nil
}

// validateBaseURLScheme rejects a plain-http BaseURL pointed at a
// non-loopback host. appId/appSecret and access-token values are sent as
// plain headers/body fields (see request.go doRequest), so a plain-http
// BaseURL to a real host would expose them in the clear to anyone able to
// observe the network path. http:// remains allowed for loopback/local
// addresses (127.0.0.0/8, or "localhost", case-insensitive) since this
// repository's own test suite points BaseURL at an httptest.Server, whose
// .URL is exactly "http://127.0.0.1:PORT".
func validateBaseURLScheme(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("qdmp: ClientOptions.BaseURL is not a valid URL: %w", err)
	}
	if u.Scheme != "http" {
		return nil
	}
	if isLoopbackHost(u.Host) {
		return nil
	}
	return fmt.Errorf("qdmp: ClientOptions.BaseURL %q uses a plain http:// scheme for a non-loopback host; use https:// to avoid sending appSecret/access-token in the clear", baseURL)
}

// isLoopbackHost reports whether host (as found in a url.URL.Host, which may
// carry a ":port" suffix) refers to a loopback/local address: the exact
// hostname "localhost" (case-insensitive), or a genuine loopback IP literal
// (127.0.0.1, any other bare 127.x.x.x dotted-quad, or the IPv6 "::1").
//
// This deliberately does NOT do a string-prefix check like
// strings.HasPrefix(h, "127."): that would also match ordinary domain names
// that merely start with the string "127." (e.g. "127.attacker.com" or
// "127.0.0.1.evil.com"), which are not IP addresses at all and could resolve
// via DNS to any attacker-controlled host. net.ParseIP only parses h as an
// IP-address literal — it never performs a DNS lookup — so this stays a pure
// string-literal check with no network access.
func isLoopbackHost(host string) bool {
	h := host
	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		h = hostOnly
	}
	h = strings.ToLower(h)
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// UserClient is a qdmp client derived from Client via WithAccessToken,
// carrying a single access token (user-level or, where explicitly opted
// into, app-level) that every business group method on it uses. This is the
// only place business group methods (User, Island, Spu, Tag, Mark, WishSpu,
// GenAI) are reachable, by design: obtaining one requires a token, so it is
// no longer possible to "forget" to pass one at the call site.
// A UserClient derived from WithUserCredential additionally carries a
// userCredential, which renews the access token on its own (see
// usercredential.go); one derived from WithAccessToken has a nil cred and
// keeps its static, never-renewed token.
type UserClient struct {
	client      *Client
	accessToken string
	cred        *userCredential

	User    *UserGroup
	Island  *IslandGroup
	Spu     *SpuGroup
	Tag     *TagGroup
	Mark    *MarkGroup
	WishSpu *WishSpuGroup
	GenAI   *GenAIGroup
}

// WithAccessToken derives a UserClient bound to the given access token. An
// empty token is accepted here (construction never fails) so that the
// "missing token" failure surfaces uniformly as ErrAccessTokenRequired from
// the business method call itself, not from this constructor.
func (c *Client) WithAccessToken(token string) *UserClient {
	uc := &UserClient{client: c, accessToken: token}
	uc.User = &UserGroup{uc: uc}
	uc.Island = &IslandGroup{uc: uc}
	uc.Spu = &SpuGroup{uc: uc}
	uc.Tag = &TagGroup{uc: uc}
	uc.Mark = &MarkGroup{uc: uc}
	uc.WishSpu = &WishSpuGroup{uc: uc}
	uc.GenAI = &GenAIGroup{uc: uc}
	return uc
}

// requireAccessToken is the shared fail-fast-locally check every business
// group method performs before touching the network: shared/generated/
// route-meta.json marks every non-auth operation x-qdmp-token-required=true,
// so an empty token here always means "must not call this operation".
//
// For a WithUserCredential client this checks the credential's *current*
// access token (which a renewal may have replaced since construction), and
// additionally requires the refresh token that makes renewal possible at all.
func requireAccessToken(uc *UserClient, opName string) error {
	current := uc.Credential()
	if current.AccessToken == "" {
		return fmt.Errorf("qdmp: %s requires an access token: %w", opName, ErrAccessTokenRequired)
	}
	for i := 0; i < len(current.AccessToken); i++ {
		b := current.AccessToken[i]
		if b <= 0x1F || b == 0x7F {
			return fmt.Errorf("qdmp: %s: %w", opName, ErrInvalidAccessToken)
		}
	}
	if uc.cred != nil && current.RefreshToken == "" {
		return fmt.Errorf("qdmp: %s requires a refresh token: %w", opName, ErrRefreshTokenRequired)
	}
	return nil
}

// doRequest is the single transport chokepoint every business group method
// goes through. Routing all seven groups here (instead of letting each call
// Client.doRequest directly) is what makes token renewal and the 401 retry
// a property of the client rather than something each operation has to
// remember to implement.
func (uc *UserClient) doRequest(ctx context.Context, p requestParams) (json.RawMessage, error) {
	if uc.cred == nil {
		p.accessToken = uc.accessToken
		return uc.client.doRequest(ctx, p)
	}
	return uc.cred.do(ctx, p)
}

// Credential returns a snapshot of the credential this client currently
// holds, so a caller can persist a renewed access token before its process
// exits. The returned value is an independent copy: it never changes as the
// client renews, and changing it never affects the client.
//
// For a WithAccessToken client (no renewal, no refresh token) only
// AccessToken is populated.
func (uc *UserClient) Credential() UserCredential {
	if uc.cred == nil {
		return UserCredential{AccessToken: uc.accessToken}
	}
	return uc.cred.snapshot()
}
