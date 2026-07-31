import {requireAccessToken} from '../context.js';
import {getRouteMeta} from '../generated/route-meta.js';
import type {QdmpContext, UserMeData} from '../types.js';
import type {GroupDeps} from './base.js';

export class UserGroup {
  constructor(private readonly deps: GroupDeps) {}

  async me(ctx: QdmpContext): Promise<UserMeData> {
    const accessToken = requireAccessToken(ctx);
    const route = getRouteMeta('userMe');
    return this.deps.http.request<UserMeData>({
      method: route.method,
      path: route.path,
      authScheme: route.authScheme,
      accessToken,
      qdmpVersion: this.deps.qdmpVersion,
    });
  }
}
