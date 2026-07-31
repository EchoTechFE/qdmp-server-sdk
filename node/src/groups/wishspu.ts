import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  QdmpContext,
  WishAddData,
  WishAddParams,
  WishCancelData,
  WishCancelParams,
  WishListData,
  WishListParams,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class WishSpuGroup {
  constructor(private readonly deps: GroupDeps) {}

  async add(ctx: QdmpContext, params: WishAddParams): Promise<WishAddData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('wishAdd');
    return this.deps.http.request<WishAddData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      body: {ids: params.ids, type: params.type},
    });
  }

  async cancel(
    ctx: QdmpContext,
    params: WishCancelParams,
  ): Promise<WishCancelData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('wishCancel');
    return this.deps.http.request<WishCancelData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      body: {ids: params.ids, type: params.type},
    });
  }

  async list(ctx: QdmpContext, params: WishListParams): Promise<WishListData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('wishList');
    return this.deps.http.request<WishListData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {
        typeId: params.typeId,
        offset: params.offset,
        limit: params.limit,
      },
    });
  }
}
