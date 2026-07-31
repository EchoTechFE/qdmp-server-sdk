import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  GenaiDetailData,
  GenaiDetailParams,
  GenaiGenerateData,
  GenaiGenerateParams,
  QdmpContext,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class GenaiGroup {
  constructor(private readonly deps: GroupDeps) {}

  async generate(
    ctx: QdmpContext,
    params: GenaiGenerateParams,
  ): Promise<GenaiGenerateData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('genaiGenerate');
    return this.deps.http.request<GenaiGenerateData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      appId: this.deps.appId,
      body: {
        model: params.model,
        contents: params.contents,
        generationConfig: params.generationConfig,
        safetySettings: params.safetySettings,
        tools: params.tools,
      },
    });
  }

  async detail(
    ctx: QdmpContext,
    params: GenaiDetailParams,
  ): Promise<GenaiDetailData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('genaiDetail');
    return this.deps.http.request<GenaiDetailData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      appId: this.deps.appId,
      query: {id: params.id},
    });
  }
}
