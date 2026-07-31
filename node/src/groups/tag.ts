import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  QdmpContext,
  TagDetailData,
  TagDetailParams,
  TagSearchData,
  TagSearchParams,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class TagGroup {
  constructor(private readonly deps: GroupDeps) {}

  async detail(
    ctx: QdmpContext,
    params: TagDetailParams,
  ): Promise<TagDetailData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('tagDetail');
    return this.deps.http.request<TagDetailData>({
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
    params: TagSearchParams,
  ): Promise<TagSearchData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('tagSearch');
    return this.deps.http.request<TagSearchData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {
        keyword: params.keyword,
        typeId: params.typeId,
        classification: params.classification,
        filterOptions: params.filterOptions,
        offset: params.offset,
        limit: params.limit,
      },
    });
  }
}
