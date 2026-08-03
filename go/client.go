package qdmp

import (
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
// exchange, using appId/appSecret — no user access token needed) plus every
// business group (User/Island/Spu/...).
//
// Business group methods take the caller's credential as an explicit
// Context argument on every call — the SDK holds no user credential of its
// own and never renews one. This mirrors the Node and Java SDKs, where the
// credential is likewise passed per call (`qdmp.user.me({accessToken})` /
// `qdmp.user().me(ctx)`), so the three ends behave identically.
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

	User    *UserGroup
	Island  *IslandGroup
	Spu     *SpuGroup
	Tag     *TagGroup
	Mark    *MarkGroup
	WishSpu *WishSpuGroup
	GenAI   *GenAIGroup
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
	c.User = &UserGroup{client: c}
	c.Island = &IslandGroup{client: c}
	c.Spu = &SpuGroup{client: c}
	c.Tag = &TagGroup{client: c}
	c.Mark = &MarkGroup{client: c}
	c.WishSpu = &WishSpuGroup{client: c}
	c.GenAI = &GenAIGroup{client: c}
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

// Context carries the caller's credential for one business call. It is the
// Go counterpart of the Node SDK's `{accessToken}` context object and the
// Java SDK's QdmpContext, and is passed explicitly to every business group
// method:
//
//	me, err := client.User.Me(ctx, qdmp.Context{AccessToken: credential.AccessToken})
//
// The SDK stores no user credential and never renews one. When a call fails
// because the token expired or was revoked, the *QdmpApiError is returned
// unchanged; obtaining a new token (see AuthService.RefreshToken) and
// passing it on the next call is the caller's job.
//
// It is a plain struct with no constructor on purpose: like Node, the token
// is validated when a call actually uses it (see requireAccessToken), not at
// construction time, so building a Context can never fail.
type Context struct {
	// AccessToken is the user-level (or, where explicitly opted into,
	// app-level) access token to authenticate this call with.
	AccessToken string
}

// requireAccessToken is the shared fail-fast-locally check every business
// group method performs before touching the network: shared/generated/
// route-meta.json marks every non-auth operation x-qdmp-token-required=true,
// so an empty token here always means "must not call this operation".
func requireAccessToken(qdmpCtx Context, opName string) error {
	if qdmpCtx.AccessToken == "" {
		return fmt.Errorf("qdmp: %s requires an access token: %w", opName, ErrAccessTokenRequired)
	}
	for i := 0; i < len(qdmpCtx.AccessToken); i++ {
		b := qdmpCtx.AccessToken[i]
		if b <= 0x1F || b == 0x7F {
			return fmt.Errorf("qdmp: %s: %w", opName, ErrInvalidAccessToken)
		}
	}
	return nil
}
