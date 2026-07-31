import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  QdmpContext,
  SpuDetailData,
  SpuDetailParams,
  SpuSearchData,
  SpuSearchParams,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class SpuGroup {
  constructor(private readonly deps: GroupDeps) {}

  async detail(
    ctx: QdmpContext,
    params: SpuDetailParams,
  ): Promise<SpuDetailData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('spuDetail');
    return this.deps.http.request<SpuDetailData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {id: params.id},
    });
  }

  async search(
    ctx: QdmpContext,
    params: SpuSearchParams,
  ): Promise<SpuSearchData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('spuSearch');
    return this.deps.http.request<SpuSearchData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {
        keyword: params.keyword,
        ipTag: params.ipTag,
        typeId: params.typeId,
        megaId: params.megaId,
        classification: params.classification,
        filterOptions: params.filterOptions,
        onlyMega: params.onlyMega,
        onlySpu: params.onlySpu,
        offset: params.offset,
        limit: params.limit,
      },
    });
  }
}
