/**
 * Pluggable persistence for the app-level (CLIENT_CREDENTIALS) access token.
 *
 * Defaults to an in-process in-memory implementation. Multi-instance
 * deployments should supply their own (e.g. Redis-backed) implementation so
 * every instance shares the same cached token instead of each independently
 * refreshing and potentially tripping rate limits.
 *
 * Only the app-level token goes through TokenStore. User-level tokens
 * (AUTHORIZATION_CODE) are never cached by the SDK — see auth.ts.
 */

export interface StoredToken {
  accessToken: string;
  /** Absolute Unix seconds timestamp, wire format is a string (see
   * shared/openapi.yaml expiresAt field notes). */
  expiresAt: string;
  /** The rest of the credential the CLIENT_CREDENTIALS grant returns.
   * Cached alongside accessToken/expiresAt so a getAppAccessToken() call
   * served from the store returns exactly as complete a credential as one
   * that just performed the exchange. Optional because a store populated by
   * an older SDK version (or a hand-written store) may not carry them —
   * they are then reported as empty strings rather than treated as an
   * error. */
  refreshToken?: string;
  openId?: string;
}

export interface TokenStore {
  get(): StoredToken | undefined | Promise<StoredToken | undefined>;
  set(token: StoredToken): void | Promise<void>;
  clear(): void | Promise<void>;
}

export class InMemoryTokenStore implements TokenStore {
  private token: StoredToken | undefined;

  get(): StoredToken | undefined {
    return this.token;
  }

  set(token: StoredToken): void {
    this.token = token;
  }

  clear(): void {
    this.token = undefined;
  }
}
