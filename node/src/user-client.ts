/**
 * The client returned by `qdmp.withUserCredential(...)`: the same seventeen
 * business operations as the root QdmpClient, except every one of them is
 * already bound to one 用户授权凭证 and therefore takes only its own
 * parameters — no QdmpContext, no access-token threading by the caller.
 *
 * The groups themselves are reused untouched. They receive a QdmpContext
 * whose accessToken is a live view of the credential (so the existing local
 * header-safety validation still runs on every call) and a transport bound
 * to that same credential (so the token actually sent is the one the
 * credential resolves at request time, after any renewal).
 */
import type {AuthModule} from './auth.js';
import type {GroupDeps} from './groups/base.js';
import {GenaiGroup} from './groups/genai.js';
import {IslandGroup} from './groups/island.js';
import {MarkGroup} from './groups/mark.js';
import {SpuGroup} from './groups/spu.js';
import {TagGroup} from './groups/tag.js';
import {UserGroup} from './groups/user.js';
import {WishSpuGroup} from './groups/wishspu.js';
import type {HttpClient} from './http.js';
import type {QdmpContext} from './types.js';
import {
  UserCredential,
  type UserCredentialOptions,
  type UserCredentialSnapshot,
} from './user-credential.js';

/** A business group with the leading QdmpContext parameter stripped from
 * every method — the shape those methods take on a QdmpUserClient. */
export type ContextBound<T> = {
  [K in keyof T]: T[K] extends (ctx: QdmpContext, ...args: infer A) => infer R
    ? (...args: A) => R
    : never;
};

/** Wraps each of `group`'s methods so callers no longer pass `ctx`. Driven
 * off the prototype rather than a hand-written list, so a group gaining a
 * method never silently misses it here. */
function bindContext<T extends object>(
  group: T,
  ctx: QdmpContext,
): ContextBound<T> {
  const bound: Record<string, unknown> = {};
  const prototype = Object.getPrototypeOf(group) as object;
  for (const key of Object.getOwnPropertyNames(prototype)) {
    if (key === 'constructor') {
      continue;
    }
    const method = (group as Record<string, unknown>)[key];
    if (typeof method !== 'function') {
      continue;
    }
    const callable = method as (...args: unknown[]) => unknown;
    bound[key] = (...args: unknown[]): unknown =>
      callable.apply(group, [ctx, ...args]);
  }
  return bound as ContextBound<T>;
}

export interface QdmpUserClientDeps {
  /** The root client's unbound transport; a credential-bound sibling is
   * derived from it here. */
  http: HttpClient;
  appId: string;
  qdmpVersion: string;
  /** Used only to perform POST /auth/v1/refresh when the credential renews. */
  auth: AuthModule;
  options: UserCredentialOptions;
}

export class QdmpUserClient {
  readonly user: ContextBound<UserGroup>;
  readonly island: ContextBound<IslandGroup>;
  readonly spu: ContextBound<SpuGroup>;
  readonly tag: ContextBound<TagGroup>;
  readonly mark: ContextBound<MarkGroup>;
  readonly wishspu: ContextBound<WishSpuGroup>;
  readonly genai: ContextBound<GenaiGroup>;

  private readonly state: UserCredential;

  constructor(deps: QdmpUserClientDeps) {
    // Constructing the credential first means an invalid accessToken /
    // refreshToken is reported before any group even exists, and long before
    // any request could go out.
    const state = new UserCredential(deps.auth, deps.options);
    this.state = state;

    const groupDeps: GroupDeps = {
      http: deps.http.withAccessTokenSource(state),
      appId: deps.appId,
      qdmpVersion: deps.qdmpVersion,
    };
    // A live view rather than a snapshot: the group reads it at call time,
    // so requireAccessToken() validates whatever token is current, including
    // one obtained by a renewal.
    const ctx: QdmpContext = {
      get accessToken(): string {
        return state.snapshot().accessToken;
      },
    };

    this.user = bindContext(new UserGroup(groupDeps), ctx);
    this.island = bindContext(new IslandGroup(groupDeps), ctx);
    this.spu = bindContext(new SpuGroup(groupDeps), ctx);
    this.tag = bindContext(new TagGroup(groupDeps), ctx);
    this.mark = bindContext(new MarkGroup(groupDeps), ctx);
    this.wishspu = bindContext(new WishSpuGroup(groupDeps), ctx);
    this.genai = bindContext(new GenaiGroup(groupDeps), ctx);
  }

  /** The credential as it stands right now — read it before the process
   * exits to persist the latest token. Each read returns a fresh copy. */
  get credential(): UserCredentialSnapshot {
    return this.state.snapshot();
  }
}
