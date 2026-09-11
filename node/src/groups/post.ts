import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  PostDetailParams,
  PostListParams,
  PostData,
  QdmpContext,
} from '../types.js';
import type {GroupDeps} from './base.js';

export class PostGroup {
  constructor(private readonly deps: GroupDeps) {}
  async detail(ctx: QdmpContext, params: PostDetailParams): Promise<PostData> {
    return this.get('postDetail', ctx, {postId: params.postId});
  }
  async list(ctx: QdmpContext, params: PostListParams = {}): Promise<PostData> {
    return this.get('postList', ctx, {
      offset: params.offset,
      limit: params.limit,
    });
  }
  async myList(
    ctx: QdmpContext,
    params: PostListParams = {},
  ): Promise<PostData> {
    return this.get('postMyList', ctx, {
      offset: params.offset,
      limit: params.limit,
    });
  }
  private async get(
    operationId: string,
    ctx: QdmpContext,
    query: Record<string, string | undefined>,
  ): Promise<PostData> {
    const route = getRouteMeta(operationId);
    const accessToken = requireAccessToken(ctx);
    return this.deps.http.request<PostData>({
      method: route.method,
      path: route.path,
      query,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
    });
  }
}
