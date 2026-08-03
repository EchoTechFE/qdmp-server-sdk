/**
 * The 用户授权凭证 (AUTHORIZATION_CODE grant) held by a QdmpUserClient, plus
 * its renewal policy.
 *
 * Two things renew it, and nothing else does:
 * - proactively, when `expiresAt` is known and within
 *   USER_TOKEN_RENEWAL_BUFFER_SECONDS of now (checked before every business
 *   request, in currentAccessToken());
 * - reactively, when a business request came back HTTP 401 with business
 *   code 10005/10006 (decided by HttpClient, which then calls
 *   renewAccessToken()).
 *
 * Renewals are single-flight, so N concurrent business calls that all see a
 * stale token trigger exactly one POST /auth/v1/refresh and all end up using
 * the one access-token it returned.
 */
import type {AuthModule} from './auth.js';
import {QdmpValidationError} from './errors.js';
import type {AccessTokenSource} from './http.js';
import {withRedactedInspect} from './redact.js';

/** Renew this many seconds before the credential's real expiresAt — same
 * buffer the 应用凭证 uses. */
const USER_TOKEN_RENEWAL_BUFFER_SECONDS = 300;

/** What a renewal produced. POST /auth/v1/refresh returns only these two
 * fields — the refreshToken is not reissued — so this is exactly what the
 * caller's onRefresh hook receives. */
export interface RefreshedUserToken {
  accessToken: string;
  expiresAt: string;
}

export interface UserCredentialOptions {
  /** Non-empty; rejected locally if it carries a character that cannot be
   * sent as an HTTP header value. */
  accessToken: string;
  /** Non-empty. Without it the credential could not be renewed at all, which
   * is the entire point of withUserCredential(). */
  refreshToken: string;
  /** Absolute Unix seconds, as a string. Omit it when unknown: the
   * credential then skips proactive renewal and relies solely on the
   * 401 + 10005/10006 signal. */
  expiresAt?: string;
  /** Called after the SDK's own state has been updated with a renewed
   * token, so the caller can persist it. Awaited — the business request that
   * triggered the renewal is not sent until it settles — and a rejection
   * fails that business call. */
  onRefresh?: (token: RefreshedUserToken) => void | Promise<void>;
}

/** A read-only copy of the credential's current state. Mutating it has no
 * effect on the client it came from. */
export interface UserCredentialSnapshot {
  accessToken: string;
  refreshToken: string;
  expiresAt?: string;
}

/** Rejects a token that cannot be put in an HTTP header verbatim. Never
 * echoes the offending value into the error message. */
function assertHeaderSafe(value: string, field: string): void {
  for (let i = 0; i < value.length; i++) {
    const charCode = value.charCodeAt(i);
    if (charCode <= 0x1f || charCode === 0x7f) {
      throw new QdmpValidationError(
        `withUserCredential: "${field}" contains a character that cannot be safely sent as an HTTP header value`,
      );
    }
  }
}

/** Parses the wire format (absolute Unix seconds as a decimal string).
 * Anything else — including undefined — means "expiry unknown", which
 * disables proactive renewal rather than failing the call. */
function parseExpiresAtSeconds(
  expiresAt: string | undefined,
): number | undefined {
  if (typeof expiresAt !== 'string' || !/^\d+$/.test(expiresAt)) {
    return undefined;
  }
  const seconds = Number(expiresAt);
  return Number.isSafeInteger(seconds) ? seconds : undefined;
}

export class UserCredential implements AccessTokenSource {
  private readonly auth: AuthModule;
  private readonly refreshToken: string;
  private readonly onRefresh:
    ((token: RefreshedUserToken) => void | Promise<void>) | undefined;
  private accessToken: string;
  private expiresAt: string | undefined;
  private inFlight: Promise<string> | undefined;

  constructor(auth: AuthModule, options: UserCredentialOptions) {
    if (
      typeof options?.accessToken !== 'string' ||
      options.accessToken.length === 0
    ) {
      throw new QdmpValidationError(
        'withUserCredential: "accessToken" is required and must be a non-empty string',
      );
    }
    assertHeaderSafe(options.accessToken, 'accessToken');
    if (
      typeof options.refreshToken !== 'string' ||
      options.refreshToken.length === 0
    ) {
      throw new QdmpValidationError(
        'withUserCredential: "refreshToken" is required and must be a non-empty string — without it the credential cannot be renewed',
      );
    }
    this.auth = auth;
    this.accessToken = options.accessToken;
    this.refreshToken = options.refreshToken;
    this.expiresAt = options.expiresAt;
    this.onRefresh = options.onRefresh;
  }

  /** A fresh object every time, so a caller that mutates what it got back
   * cannot corrupt the credential this client keeps using. */
  snapshot(): UserCredentialSnapshot {
    const snapshot: UserCredentialSnapshot = {
      accessToken: this.accessToken,
      refreshToken: this.refreshToken,
      expiresAt: this.expiresAt,
    };
    return withRedactedInspect(
      snapshot,
      () =>
        `UserCredentialSnapshot { accessToken: '[REDACTED]', refreshToken: '[REDACTED]', expiresAt: ${JSON.stringify(snapshot.expiresAt)} }`,
    );
  }

  /** Deliberately NOT `async`: the check-and-set of `this.inFlight` inside
   * renew() must run in the caller's own synchronous turn, so that business
   * calls started in the same tick all join one renewal instead of each
   * starting their own. */
  currentAccessToken(): Promise<string> {
    if (this.needsProactiveRenewal()) {
      return this.renew();
    }
    return Promise.resolve(this.accessToken);
  }

  renewAccessToken(staleAccessToken: string): Promise<string> {
    // A concurrent call may have already renewed while this request was in
    // flight — its result is by definition newer than the token that just
    // got rejected, so use it rather than burning a second renewal.
    if (this.accessToken !== staleAccessToken) {
      return Promise.resolve(this.accessToken);
    }
    return this.renew();
  }

  private needsProactiveRenewal(): boolean {
    const expiresAtSeconds = parseExpiresAtSeconds(this.expiresAt);
    if (expiresAtSeconds === undefined) {
      return false;
    }
    const nowSeconds = Math.floor(Date.now() / 1000);
    return expiresAtSeconds - nowSeconds <= USER_TOKEN_RENEWAL_BUFFER_SECONDS;
  }

  private renew(): Promise<string> {
    if (!this.inFlight) {
      this.inFlight = this.performRenewal().finally(() => {
        this.inFlight = undefined;
      });
    }
    return this.inFlight;
  }

  private async performRenewal(): Promise<string> {
    const refreshed = await this.auth.refreshToken(this.refreshToken);
    // The server does not reissue a refreshToken here, so the original one
    // stays in use for every later renewal.
    this.accessToken = refreshed.accessToken;
    this.expiresAt = refreshed.expiresAt;
    if (this.onRefresh) {
      // Awaited on purpose, and after the state above was already updated:
      // if persisting fails, the business call fails with that error, but a
      // token the SDK has already obtained is not thrown away — the caller
      // can still read it off `userClient.credential`.
      await this.onRefresh({
        accessToken: refreshed.accessToken,
        expiresAt: refreshed.expiresAt,
      });
    }
    return this.accessToken;
  }
}
