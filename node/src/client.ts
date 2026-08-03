import type {Dispatcher} from 'undici';

import {AuthModule} from './auth.js';
import {QdmpValidationError} from './errors.js';
import {GenaiGroup} from './groups/genai.js';
import {IslandGroup} from './groups/island.js';
import {MarkGroup} from './groups/mark.js';
import {SpuGroup} from './groups/spu.js';
import {TagGroup} from './groups/tag.js';
import {UserGroup} from './groups/user.js';
import {WishSpuGroup} from './groups/wishspu.js';
import {HttpClient} from './http.js';
import {InMemoryTokenStore, type TokenStore} from './token-store.js';
import {QdmpUserClient} from './user-client.js';
import type {UserCredentialOptions} from './user-credential.js';

/** The qdmp OpenAPI host (see shared/openapi.yaml `servers[0]`). */
const DEFAULT_BASE_URL = 'https://openapi.qiandao.com';

/** Matches a dotted-decimal IPv4 literal: exactly four groups of 1-3 digits
 * separated by dots. This only tests the string SHAPE — range-checking each
 * octet (0-255) and confirming the first octet is `127` is done separately
 * in `isLoopbackHost()`. Intentionally does not accept anything else (no
 * hex/octal forms, no hostnames) — a hostname like `127.attacker.com` must
 * NOT match this, since its second "octet" is not a pure decimal string. */
const IPV4_LITERAL_PATTERN = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

/** True if `hostname` is a syntactically valid dotted-decimal IPv4 literal
 * (all four octets in 0-255) whose first octet is exactly 127 — i.e. a
 * genuine loopback IP address, not merely a string that starts with the
 * text "127.". Deliberately does NOT perform any DNS resolution: this must
 * stay a pure string/literal check, since resolving the hostname here would
 * make the safety decision depend on network state instead of the literal
 * the caller wrote. */
function isLoopbackIpv4Literal(hostname: string): boolean {
  const match = IPV4_LITERAL_PATTERN.exec(hostname);
  if (!match) {
    return false;
  }
  const octets = match.slice(1, 5).map(Number);
  if (!octets.every(octet => octet >= 0 && octet <= 255)) {
    return false;
  }
  return octets[0] === 127;
}

/** Loopback/localhost hosts (case-insensitive) that are allowed to use
 * plain http:// — e.g. a local dev/mock backend. Anything else on http://
 * would send appId/appSecret/accessToken in the clear.
 *
 * Must reject anything that is merely a string starting with "127." but is
 * not itself a genuine IPv4 literal — e.g. the hostname `127.attacker.com`
 * is a DNS name (not an IP) that can resolve to any attacker-controlled
 * remote host, and must NOT be treated as loopback. */
function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase();
  return host === 'localhost' || isLoopbackIpv4Literal(host);
}

/** Fail fast at construction time — never wait for the first real network
 * request — if baseUrl is http:// against a non-loopback host. */
function assertSafeBaseUrl(baseUrl: string): void {
  let parsed: URL;
  try {
    parsed = new URL(baseUrl);
  } catch {
    throw new QdmpValidationError(
      `QdmpClient: options.baseUrl "${baseUrl}" is not a valid URL`,
    );
  }
  if (parsed.protocol === 'http:' && !isLoopbackHost(parsed.hostname)) {
    throw new QdmpValidationError(
      `QdmpClient: options.baseUrl "${baseUrl}" uses plain http:// against a non-loopback host; use https:// (loopback/localhost is exempt for local dev/test)`,
    );
  }
}

export interface QdmpClientOptions {
  appId: string;
  appSecret: string;
  /** Overrides the default qdmp OpenAPI host. */
  baseUrl?: string;
  /** Value sent as x-echo-qdmp-version for `standard`-scheme calls. */
  qdmpVersion?: string;
  /** Only needed for tests (undici MockAgent) or custom connection pooling. */
  dispatcher?: Dispatcher;
  /** Defaults to an in-process InMemoryTokenStore; swap in a shared store
   * (e.g. Redis-backed) for multi-instance deployments. */
  tokenStore?: TokenStore;
}

export class QdmpClient {
  readonly auth: AuthModule;
  readonly user: UserGroup;
  readonly island: IslandGroup;
  readonly spu: SpuGroup;
  readonly tag: TagGroup;
  readonly mark: MarkGroup;
  readonly wishspu: WishSpuGroup;
  readonly genai: GenaiGroup;

  private readonly http: HttpClient;
  private readonly appId: string;
  private readonly qdmpVersion: string;

  constructor(options: QdmpClientOptions) {
    const baseUrl = options.baseUrl ?? DEFAULT_BASE_URL;
    assertSafeBaseUrl(baseUrl);
    const qdmpVersion = options.qdmpVersion ?? '1.0';
    const http = new HttpClient({baseUrl, dispatcher: options.dispatcher});
    this.http = http;
    this.appId = options.appId;
    this.qdmpVersion = qdmpVersion;

    this.auth = new AuthModule({
      http,
      appId: options.appId,
      appSecret: options.appSecret,
      tokenStore: options.tokenStore ?? new InMemoryTokenStore(),
    });

    const deps = {http, appId: options.appId, qdmpVersion};
    this.user = new UserGroup(deps);
    this.island = new IslandGroup(deps);
    this.spu = new SpuGroup(deps);
    this.tag = new TagGroup(deps);
    this.mark = new MarkGroup(deps);
    this.wishspu = new WishSpuGroup(deps);
    this.genai = new GenaiGroup(deps);
  }

  /** Binds one 用户授权凭证 to a client whose business methods take no
   * QdmpContext and renew the credential on their own — proactively within
   * 300 seconds of expiry, and reactively on an HTTP 401 carrying business
   * code 10005/10006 (renewed once, request replayed once).
   *
   * The groups on `this` are unaffected: they keep requiring an explicit
   * ctx and never renew anything. */
  withUserCredential(options: UserCredentialOptions): QdmpUserClient {
    return new QdmpUserClient({
      http: this.http,
      appId: this.appId,
      qdmpVersion: this.qdmpVersion,
      auth: this.auth,
      options,
    });
  }
}
