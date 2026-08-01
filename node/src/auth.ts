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
import {inspect} from 'node:util';

import type {HttpClient} from './http.js';
import type {AppTokenData, RefreshTokenData, SessionData} from './types.js';
import type {StoredToken, TokenStore} from './token-store.js';
import {getRouteMeta} from './generated/route-meta.js';
import {QdmpTransportError, QdmpValidationError} from './errors.js';

/** Refresh this many seconds before the token's real expiresAt. */
const APP_TOKEN_REFRESH_BUFFER_SECONDS = 300;

/**
 * Attaches a non-enumerable [util.inspect.custom] renderer to `value` so
 * that `console.log(value)`/`util.inspect(value)` — an easy accidental
 * debug-print path — never shows the raw accessToken/refreshToken. This is
 * deliberately done via Object.defineProperty on an already-constructed,
 * already-typed value rather than as an object-literal property: adding
 * `[inspect.custom]: ...` directly to a `SessionData`/`RefreshTokenData`
 * literal would trip TypeScript's excess-property check (neither interface
 * declares a symbol-keyed member).
 *
 * This must never affect JSON.stringify(value) or direct property access
 * (value.accessToken): JSON.stringify only consults toJSON(), not
 * util.inspect.custom, and the defined property here is non-enumerable so
 * it does not add a spurious key to the object either. Persisting the real
 * values is the entire point of code2Session()/refreshToken() existing, so
 * that path must stay untouched.
 */
function withRedactedInspect<T extends object>(
  value: T,
  render: () => string,
): T {
  Object.defineProperty(value, inspect.custom, {
    value: render,
    enumerable: false,
  });
  return value;
}

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
 * expiresAt in a one-shot (never-cached) code2Session()/refreshToken()
 * response. assertWellFormedTokenResponse() only checks that expiresAt
 * parses as a non-negative safe integer — a value like "1" (1970-01-01)
 * passes that check but describes a session/token that is already dead, and
 * must not be handed back to the caller as if it were usable (they might
 * persist or start relying on it). Unlike getAccessToken()'s cached path,
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

/** Guards against a malformed code2Session() response: HTTP 200 + business
 * code='0' but with a missing/empty refreshToken or openId. Unlike
 * accessToken/expiresAt (validated by assertWellFormedTokenResponse() and
 * shared with getAccessToken()/refreshToken()), refreshToken/openId are only
 * present in the AUTHORIZATION_CODE grant's response, so this check is
 * intentionally separate and only ever called from code2Session(). Must be
 * called before the result is resolved to the caller — otherwise an empty
 * openId could silently end up persisted as a user identifier. */
function assertWellFormedSessionResponse(
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
          assertWellFormedTokenResponse(token, 'auth.getAccessToken');
          // A token that is syntactically well-formed can still be
          // expired-on-arrival (e.g. a server bug returns an `expiresAt`
          // far in the past). Such a token must never be cached or handed
          // back to the caller as if it were freshly usable.
          if (!this.isFresh(token)) {
            throw new QdmpTransportError(
              'auth.getAccessToken: server response was a business-success envelope but the returned token is already expired (or within the refresh buffer) on arrival',
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
    const session = await this.http.request<SessionData>({
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
    assertWellFormedTokenResponse(session, 'auth.code2Session');
    assertWellFormedSessionResponse(session, 'auth.code2Session');
    assertNotAlreadyExpired(session, 'auth.code2Session');
    return withRedactedInspect(
      session,
      // JSON.stringify (not raw template interpolation) for the non-secret
      // fields: expiresAt is our own validated digit-only string, but openId
      // is server-controlled and only validated as "non-empty" — an
      // embedded CR/LF there would otherwise become a raw line break in
      // this debug rendering, the same log-injection class of bug already
      // fixed for message/code/requestId elsewhere.
      () =>
        `SessionData { accessToken: '[REDACTED]', refreshToken: '[REDACTED]', expiresAt: ${JSON.stringify(session.expiresAt)}, openId: ${JSON.stringify(session.openId)} }`,
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
