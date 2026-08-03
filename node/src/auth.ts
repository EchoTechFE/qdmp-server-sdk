/**
 * Token lifecycle management:
 *
 * - getAppAccessToken(): 应用凭证 (CLIENT_CREDENTIALS), auto-cached via the
 *   pluggable TokenStore, single-flight refresh, proactively renewed 300
 *   seconds before real expiry (not reactive to a 401).
 * - getUserAccessToken(): one-shot AUTHORIZATION_CODE exchange producing a
 *   用户授权凭证. Never cached — the calling backend owns per-end-user
 *   credential persistence.
 * - refreshToken(): one-shot refresh of a caller-supplied refreshToken.
 *   Never cached either.
 */
import type {HttpClient} from './http.js';
import type {
  AppAccessTokenData,
  RefreshTokenData,
  UserAccessTokenData,
} from './types.js';
import type {StoredToken, TokenStore} from './token-store.js';
import {getRouteMeta} from './generated/route-meta.js';
import {QdmpTransportError, QdmpValidationError} from './errors.js';
import {withRedactedInspect} from './redact.js';

/** Refresh this many seconds before the token's real expiresAt. */
const APP_TOKEN_REFRESH_BUFFER_SECONDS = 300;

/** A still-unvalidated {@link AppAccessTokenData} exactly as it arrived off
 * the wire: every field is optional here because a business-success envelope
 * is no guarantee that `data` actually carries them. */
type RawAppAccessTokenData = Partial<AppAccessTokenData>;

/** Guards against a malformed "success" response: HTTP 200 + business
 * code='0' but with a missing/empty accessToken, or an expiresAt that can't
 * be interpreted as a real timestamp. Must be called before the result is
 * written to any cache/tokenStore or resolved to the caller — otherwise a
 * broken value could poison the cache or silently resolve as
 * undefined/empty to callers. */
function assertWellFormedTokenResponse(
  data: {accessToken?: string; expiresAt?: string} | undefined,
  context: string,
): void {
  if (
    !data ||
    typeof data.accessToken !== 'string' ||
    data.accessToken.length === 0
  ) {
    throw new QdmpTransportError(
      `${context}: server response was a business-success envelope but is missing a non-empty accessToken`,
    );
  }
  // Match Go's strconv.ParseInt / Java's Long.parseLong: a strict decimal
  // integer only. Number(expiresAt) alone is far more permissive than
  // either — it silently accepts whitespace-only strings (Number('  ')
  // === 0), scientific notation, hex, and decimals, none of which the
  // other two SDKs would parse as a valid Unix timestamp.
  if (
    typeof data.expiresAt !== 'string' ||
    !/^\d+$/.test(data.expiresAt) ||
    !Number.isSafeInteger(Number(data.expiresAt))
  ) {
    throw new QdmpTransportError(
      `${context}: server response was a business-success envelope but expiresAt could not be parsed as a finite number`,
    );
  }
}

/** Guards against a syntactically well-formed but already-expired-on-arrival
 * expiresAt in a one-shot (never-cached) getUserAccessToken()/refreshToken()
 * response. assertWellFormedTokenResponse() only checks that expiresAt
 * parses as a non-negative safe integer — a value like "1" (1970-01-01)
 * passes that check but describes a credential that is already dead, and
 * must not be handed back to the caller as if it were usable (they might
 * persist or start relying on it). Unlike getAppAccessToken()'s cached path,
 * this only rejects an already-past expiresAt, not one merely within the
 * app-token refresh buffer — these one-shot results are never cached by the
 * SDK, so there is no cache-freshness policy to apply here. */
function assertNotAlreadyExpired(
  data: {expiresAt?: string},
  context: string,
): void {
  const nowSeconds = Math.floor(Date.now() / 1000);
  if (Number(data.expiresAt) <= nowSeconds) {
    throw new QdmpTransportError(
      `${context}: server response was a business-success envelope but expiresAt is already in the past`,
    );
  }
}

/** Guards against a malformed getUserAccessToken() response: HTTP 200 +
 * business code='0' but with a missing/empty refreshToken or openId. Unlike
 * accessToken/expiresAt (validated by assertWellFormedTokenResponse() and
 * shared with getAppAccessToken()/refreshToken()), refreshToken/openId only
 * carry meaning in the AUTHORIZATION_CODE grant's response, so this check is
 * intentionally separate and only ever called from getUserAccessToken() —
 * the CLIENT_CREDENTIALS grant legitimately returns an empty openId. Must be
 * called before the result is resolved to the caller — otherwise an empty
 * openId could silently end up persisted as a user identifier. */
function assertWellFormedUserAccessTokenResponse(
  data: {refreshToken?: string; openId?: string} | undefined,
  context: string,
): void {
  if (
    !data ||
    typeof data.refreshToken !== 'string' ||
    data.refreshToken.length === 0
  ) {
    throw new QdmpTransportError(
      `${context}: server response was a business-success envelope but is missing a non-empty refreshToken`,
    );
  }
  if (typeof data.openId !== 'string' || data.openId.length === 0) {
    throw new QdmpTransportError(
      `${context}: server response was a business-success envelope but is missing a non-empty openId`,
    );
  }
}

/** Renders a cached entry as the complete 应用凭证 callers see. Both the
 * cache-hit and the just-exchanged paths go through here, so neither can
 * return a credential that is less complete than the other. */
function toAppAccessTokenData(token: StoredToken): AppAccessTokenData {
  const credential: AppAccessTokenData = {
    accessToken: token.accessToken,
    expiresAt: token.expiresAt,
    refreshToken: token.refreshToken ?? '',
    openId: token.openId ?? '',
  };
  return withRedactedInspect(
    credential,
    // JSON.stringify (not raw template interpolation) for the non-secret
    // fields: expiresAt is our own validated digit-only string, but openId
    // is server-controlled and not validated for this grant — an embedded
    // CR/LF there would otherwise become a raw line break in this debug
    // rendering, the same log-injection class of bug already fixed for
    // message/code/requestId elsewhere.
    () =>
      `AppAccessTokenData { accessToken: '[REDACTED]', refreshToken: '[REDACTED]', expiresAt: ${JSON.stringify(credential.expiresAt)}, openId: ${JSON.stringify(credential.openId)} }`,
  );
}

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
  private inFlight: Promise<AppAccessTokenData> | undefined;
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

  async getAppAccessToken(): Promise<AppAccessTokenData> {
    if (this.cachedToken && this.isFresh(this.cachedToken)) {
      return toAppAccessTokenData(this.cachedToken);
    }
    const cached = await Promise.resolve(this.tokenStore.get());
    if (cached && this.isFresh(cached)) {
      this.cachedToken = cached;
      return toAppAccessTokenData(cached);
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
   * a caller can reach here (via `getAppAccessToken()`'s `await
   * tokenStore.get()`) *after* another caller's in-flight refresh has
   * already completed and cleared `inFlight` — if that other refresh
   * happened to run entirely while this caller's `tokenStore.get()` was
   * still pending, `this.cachedToken` was already updated in the meantime
   * and must be honored here instead of blindly starting a second,
   * redundant CLIENT_CREDENTIALS exchange. */
  private refreshAppToken(): Promise<AppAccessTokenData> {
    if (this.cachedToken && this.isFresh(this.cachedToken)) {
      return Promise.resolve(toAppAccessTokenData(this.cachedToken));
    }
    if (!this.inFlight) {
      this.inFlight = this.requestClientCredentialsToken()
        .then(async raw => {
          assertWellFormedTokenResponse(raw, 'auth.getAppAccessToken');
          // refreshToken/openId are deliberately not validated here: the
          // CLIENT_CREDENTIALS grant really does return an empty openId,
          // and its refreshToken is not used by the SDK's own renewal
          // strategy (which re-runs the grant instead). Whatever the server
          // sent is passed through as-is.
          const token: StoredToken = {
            accessToken: raw.accessToken as string,
            expiresAt: raw.expiresAt as string,
            refreshToken: raw.refreshToken,
            openId: raw.openId,
          };
          // A token that is syntactically well-formed can still be
          // expired-on-arrival (e.g. a server bug returns an `expiresAt`
          // far in the past). Such a token must never be cached or handed
          // back to the caller as if it were freshly usable.
          if (!this.isFresh(token)) {
            throw new QdmpTransportError(
              'auth.getAppAccessToken: server response was a business-success envelope but the returned token is already expired (or within the refresh buffer) on arrival',
            );
          }
          // Only trust the local mirror once the store write actually
          // succeeds — assigning this.cachedToken before awaiting
          // tokenStore.set() would let a concurrent/later caller (which
          // checks this.cachedToken directly, not this call's own promise)
          // read back a value that was never durably persisted, if the
          // store write goes on to reject. On rejection here, cachedToken
          // is left untouched and this call's own promise rejects too.
          await Promise.resolve(this.tokenStore.set(token));
          this.cachedToken = token;
          return toAppAccessTokenData(token);
        })
        .finally(() => {
          this.inFlight = undefined;
        });
    }
    return this.inFlight;
  }

  private requestClientCredentialsToken(): Promise<RawAppAccessTokenData> {
    const route = getRouteMeta('authToken');
    return this.http.request<RawAppAccessTokenData>({
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

  async getUserAccessToken(code: string): Promise<UserAccessTokenData> {
    if (typeof code !== 'string' || code.length === 0) {
      throw new QdmpValidationError(
        'getUserAccessToken: "code" is required for grantType=AUTHORIZATION_CODE and must be a non-empty string',
      );
    }
    const route = getRouteMeta('authToken');
    const credential = await this.http.request<UserAccessTokenData>({
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
    assertWellFormedTokenResponse(credential, 'auth.getUserAccessToken');
    assertWellFormedUserAccessTokenResponse(
      credential,
      'auth.getUserAccessToken',
    );
    assertNotAlreadyExpired(credential, 'auth.getUserAccessToken');
    return withRedactedInspect(
      credential,
      // JSON.stringify (not raw template interpolation) for the non-secret
      // fields: expiresAt is our own validated digit-only string, but openId
      // is server-controlled and only validated as "non-empty" — an
      // embedded CR/LF there would otherwise become a raw line break in
      // this debug rendering, the same log-injection class of bug already
      // fixed for message/code/requestId elsewhere.
      () =>
        `UserAccessTokenData { accessToken: '[REDACTED]', refreshToken: '[REDACTED]', expiresAt: ${JSON.stringify(credential.expiresAt)}, openId: ${JSON.stringify(credential.openId)} }`,
    );
  }

  async refreshToken(refreshToken: string): Promise<RefreshTokenData> {
    if (typeof refreshToken !== 'string' || refreshToken.length === 0) {
      throw new QdmpValidationError(
        'refreshToken: "refreshToken" is required and must be a non-empty string',
      );
    }
    const route = getRouteMeta('authRefresh');
    const result = await this.http.request<RefreshTokenData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      body: {refreshToken},
    });
    assertWellFormedTokenResponse(result, 'auth.refreshToken');
    assertNotAlreadyExpired(result, 'auth.refreshToken');
    return withRedactedInspect(
      result,
      () =>
        `RefreshTokenData { accessToken: '[REDACTED]', expiresAt: ${JSON.stringify(result.expiresAt)} }`,
    );
  }
}
