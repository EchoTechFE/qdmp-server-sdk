import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  IslandDetailData,
  IslandDetailParams,
  QdmpContext,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class IslandGroup {
  constructor(private readonly deps: GroupDeps) {}

  async detail(
    ctx: QdmpContext,
    params: IslandDetailParams,
  ): Promise<IslandDetailData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('islandDetail');
    return this.deps.http.request<IslandDetailData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
      query: {id: params.id},
    });
  }
}
