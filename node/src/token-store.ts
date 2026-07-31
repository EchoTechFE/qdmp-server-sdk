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
