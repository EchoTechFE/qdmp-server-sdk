import type {Dispatcher} from 'undici';

import {AuthModule} from './auth.js';
import {GenaiGroup} from './groups/genai.js';
import {IslandGroup} from './groups/island.js';
import {MarkGroup} from './groups/mark.js';
import {SpuGroup} from './groups/spu.js';
import {TagGroup} from './groups/tag.js';
import {UserGroup} from './groups/user.js';
import {WishSpuGroup} from './groups/wishspu.js';
import {HttpClient} from './http.js';
import {InMemoryTokenStore, type TokenStore} from './token-store.js';

/** The qdmp OpenAPI host (see shared/openapi.yaml `servers[0]`). */
const DEFAULT_BASE_URL = 'https://openapi.qiandao.com';

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

  constructor(options: QdmpClientOptions) {
    const baseUrl = options.baseUrl ?? DEFAULT_BASE_URL;
    const qdmpVersion = options.qdmpVersion ?? '1.0';
    const http = new HttpClient({baseUrl, dispatcher: options.dispatcher});

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
}
