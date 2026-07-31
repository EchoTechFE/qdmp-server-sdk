package qdmp

import (
	"fmt"
	"net/http"
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
// refresh means every concurrent GetAccessToken caller would pile up behind
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
	// timeout, so a hung qdmp endpoint can never block a caller forever.
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
	// never-cached code2Session/refreshToken exchanges.
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
	qdmpVersion := opts.QdmpVersion
	if qdmpVersion == "" {
		qdmpVersion = defaultQdmpVersion
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
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

// UserClient is a qdmp client derived from Client via WithAccessToken,
// carrying a single access token (user-level or, where explicitly opted
// into, app-level) that every business group method on it uses. This is the
// only place business group methods (User, Island, Spu, Tag, Mark, WishSpu,
// GenAI) are reachable, by design: obtaining one requires a token, so it is
// no longer possible to "forget" to pass one at the call site.
type UserClient struct {
	client      *Client
	accessToken string

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
func requireAccessToken(uc *UserClient, opName string) error {
	if uc.accessToken == "" {
		return fmt.Errorf("qdmp: %s requires an access token: %w", opName, ErrAccessTokenRequired)
	}
	return nil
}
