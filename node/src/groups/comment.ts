import {requireAccessToken} from '../context.js';
import {QdmpValidationError} from '../errors.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {
  CommentCreateParams,
  CommentLikeParams,
  CommentRepliesData,
  CommentReplyParams,
  CommentRepliesParams,
  CreateCommentData,
  PostCommentsParams,
  PostCommentsData,
  QdmpContext,
} from '../types.js';
import type {GroupDeps} from './base.js';
export class CommentGroup {
  constructor(private readonly deps: GroupDeps) {}
  async create(
    ctx: QdmpContext,
    params: CommentCreateParams,
  ): Promise<CreateCommentData> {
    return this.call('commentCreate', ctx, {}, params);
  }
  async reply(
    ctx: QdmpContext,
    params: CommentReplyParams,
  ): Promise<CreateCommentData> {
    return this.call(
      'commentReply',
      ctx,
      {commentId: params.commentId},
      {content: params.content},
    );
  }
  async like(ctx: QdmpContext, params: CommentLikeParams): Promise<void> {
    await this.call<unknown>(
      'commentLike',
      ctx,
      {commentId: params.commentId},
      {liked: params.liked},
    );
  }
  async postComments(
    ctx: QdmpContext,
    params: PostCommentsParams,
  ): Promise<PostCommentsData> {
    return this.call('postComments', ctx, {postId: params.postId}, undefined, {
      limit: params.limit,
      offset: params.offset,
    });
  }
  async replies(
    ctx: QdmpContext,
    params: CommentRepliesParams,
  ): Promise<CommentRepliesData> {
    return this.call(
      'commentReplies',
      ctx,
      {commentId: params.commentId},
      undefined,
      {limit: params.limit, cursor: params.cursor},
    );
  }
  private async call<T>(
    operationId: string,
    ctx: QdmpContext,
    pathParams: Record<string, string>,
    body?: unknown,
    query?: Record<string, string | undefined>,
  ): Promise<T> {
    const route = getRouteMeta(operationId);
    const accessToken = requireAccessToken(ctx);
    let path = route.path;
    for (const [key, value] of Object.entries(pathParams)) {
      validatePositiveId(key, value);
      path = path.replace(`{${key}}`, value);
    }
    return this.deps.http.request<T>({
      method: route.method,
      path,
      body,
      query,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
    });
  }
}

function validatePositiveId(name: string, value: string): void {
  if (!/^[1-9][0-9]*$/.test(value)) {
    throw new QdmpValidationError(`${name} must be a positive integer ID`);
  }
}
