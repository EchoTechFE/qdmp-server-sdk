/**
 * Token lifecycle management:
 *
 * - getAccessToken(): app-level (CLIENT_CREDENTIALS) token, auto-cached via
 *   the pluggable TokenStore, single-flight refresh, proactively renewed
 *   300 seconds before real expiry (not reactive to a 401).
 * - code2Session(): one-shot AUTHORIZATION_CODE exchange. Never cached —
 *   the calling backend owns per-end-user session persistence.
 * - refreshToken(): one-shot refresh of a caller-supplied refreshToken.
 *   Never cached either.
 */
import type {HttpClient} from './http.js';
import type {AppTokenData, RefreshTokenData, SessionData} from './types.js';
import type {StoredToken, TokenStore} from './token-store.js';
import {getRouteMeta} from './generated/route-meta.js';
import {QdmpValidationError} from './errors.js';

/** Refresh this many seconds before the token's real expiresAt. */
const APP_TOKEN_REFRESH_BUFFER_SECONDS = 300;

export interface AuthModuleConfig {
  http: HttpClient;
  appId: string;
  appSecret: string;
  tokenStore: TokenStore;
}

export class AuthModule {
  private readonly http: HttpClient;
  private readonly appId: string;
  private readonly appSecret: string;
  private readonly tokenStore: TokenStore;
  private inFlight: Promise<string> | undefined;
  /** In-process mirror of the last token this instance itself fetched or
   * observed as fresh, set synchronously the moment a refresh HTTP call
   * resolves (see refreshAppToken()) — i.e. before awaiting tokenStore.set()
   * and before `inFlight` is cleared. Checked before tokenStore.get() so
   * that, with an async/remote TokenStore (e.g. Redis), a caller arriving
   * right after another caller's in-flight refresh just finished doesn't
   * have to depend on the store's read-after-write consistency to see the
   * fresh token — it hits this local cache instead and never triggers a
   * redundant CLIENT_CREDENTIALS exchange. */
  private cachedToken: StoredToken | undefined;

  constructor(config: AuthModuleConfig) {
    this.http = config.http;
    this.appId = config.appId;
    this.appSecret = config.appSecret;
    this.tokenStore = config.tokenStore;
  }

  async getAccessToken(): Promise<string> {
    if (this.cachedToken && this.isFresh(this.cachedToken)) {
      return this.cachedToken.accessToken;
    }
    const cached = await Promise.resolve(this.tokenStore.get());
    if (cached && this.isFresh(cached)) {
      this.cachedToken = cached;
      return cached.accessToken;
    }
    return this.refreshAppToken();
  }

  private isFresh(token: StoredToken): boolean {
    const nowSeconds = Math.floor(Date.now() / 1000);
    const expiresAt = Number(token.expiresAt);
    return expiresAt - nowSeconds > APP_TOKEN_REFRESH_BUFFER_SECONDS;
  }

  /** Single-flight: the check-and-set of `this.inFlight` happens
   * synchronously (no `await` before it), so concurrent callers that each
   * reach this point via their own microtask all observe the same in-flight
   * promise instead of triggering redundant HTTP requests.
   *
   * Re-checks `this.cachedToken` freshness right before deciding whether to
   * start a new flight. This closes a race with an async/remote TokenStore:
   * a caller can reach here (via `getAccessToken()`'s `await
   * tokenStore.get()`) *after* another caller's in-flight refresh has
   * already completed and cleared `inFlight` — if that other refresh
   * happened to run entirely while this caller's `tokenStore.get()` was
   * still pending, `this.cachedToken` was already updated in the meantime
   * and must be honored here instead of blindly starting a second,
   * redundant CLIENT_CREDENTIALS exchange. */
  private refreshAppToken(): Promise<string> {
    if (this.cachedToken && this.isFresh(this.cachedToken)) {
      return Promise.resolve(this.cachedToken.accessToken);
    }
    if (!this.inFlight) {
      this.inFlight = this.requestClientCredentialsToken()
        .then(async token => {
          this.cachedToken = token;
          await Promise.resolve(this.tokenStore.set(token));
          return token.accessToken;
        })
        .finally(() => {
          this.inFlight = undefined;
        });
    }
    return this.inFlight;
  }

  private requestClientCredentialsToken(): Promise<AppTokenData> {
    const route = getRouteMeta('authToken');
    return this.http.request<AppTokenData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      body: {
        grantType: 'CLIENT_CREDENTIALS',
        appId: this.appId,
        appSecret: this.appSecret,
      },
    });
  }

  async code2Session(code: string): Promise<SessionData> {
    if (typeof code !== 'string' || code.length === 0) {
      throw new QdmpValidationError(
        'code2Session: "code" is required for grantType=AUTHORIZATION_CODE and must be a non-empty string',
      );
    }
    const route = getRouteMeta('authToken');
    return this.http.request<SessionData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      body: {
        grantType: 'AUTHORIZATION_CODE',
        appId: this.appId,
        appSecret: this.appSecret,
        code,
      },
    });
  }

  async refreshToken(refreshToken: string): Promise<RefreshTokenData> {
    if (typeof refreshToken !== 'string' || refreshToken.length === 0) {
      throw new QdmpValidationError(
        'refreshToken: "refreshToken" is required and must be a non-empty string',
      );
    }
    const route = getRouteMeta('authRefresh');
    return this.http.request<RefreshTokenData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      body: {refreshToken},
    });
  }
}
