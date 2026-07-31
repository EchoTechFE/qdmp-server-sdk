import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  MarkAddData,
  MarkAddParams,
  MarkDetailData,
  MarkDetailParams,
  MarkListData,
  MarkListParams,
  MarkSearchData,
  MarkSearchParams,
  QdmpContext,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class MarkGroup {
  constructor(private readonly deps: GroupDeps) {}

  async add(ctx: QdmpContext, params: MarkAddParams): Promise<MarkAddData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('markAdd');
    return this.deps.http.request<MarkAddData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      body: {spuId: params.spuId, rating: params.rating},
    });
  }

  async list(ctx: QdmpContext, params: MarkListParams): Promise<MarkListData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('markList');
    return this.deps.http.request<MarkListData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {limit: params.limit, offset: params.offset},
    });
  }

  async search(
    ctx: QdmpContext,
    params: MarkSearchParams,
  ): Promise<MarkSearchData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('markSearch');
    return this.deps.http.request<MarkSearchData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {
        typeId: params.typeId,
        limit: params.limit,
        offset: params.offset,
      },
    });
  }

  async detail(
    ctx: QdmpContext,
    params: MarkDetailParams,
  ): Promise<MarkDetailData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('markDetail');
    return this.deps.http.request<MarkDetailData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {id: params.id, limit: params.limit, offset: params.offset},
    });
  }
}
