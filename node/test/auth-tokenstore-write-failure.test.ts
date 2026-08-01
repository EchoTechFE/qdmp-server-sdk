/**
 * 回归测试（codex 收敛检查轮发现）：TokenStore.set() 写入失败时，本地
 * cachedToken 镜像不应当被当作"已经成功持久化"来使用。
 *
 * 背景：refreshAppToken()（src/auth.ts）里，`this.cachedToken = token` 曾经
 * 在 `await this.tokenStore.set(token)` 之前就赋值。如果 set() 之后 reject
 * （例如一个 Redis-backed TokenStore 因网络问题写入失败），触发这次刷新的
 * 调用本身会因为 await 到 rejected promise 而失败——但 `this.cachedToken`
 * 已经被设成了这个"实际上没能持久化"的值。此后（哪怕就在同一个 event loop
 * tick 之后）到达的并发/后续调用会看到 `this.cachedToken` 且判定为 fresh，
 * 从而复用这个从未真正写入 store 的值，表现得像是这次写入成功了一样——而
 * 其他共享同一个远程 TokenStore 的实例完全不知道这个新 token。
 *
 * 修复：把 `this.cachedToken = token` 移到 `await tokenStore.set(token)`
 * 成功之后，写入失败时 cachedToken 保持不变。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import type {StoredToken, TokenStore} from '../src/token-store.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

class RejectOnceTokenStore implements TokenStore {
  private token: StoredToken | undefined;
  private rejectNextSet: boolean;
  setCallCount = 0;

  constructor(rejectFirstSet: boolean) {
    this.rejectNextSet = rejectFirstSet;
  }

  get(): StoredToken | undefined {
    return this.token;
  }

  async set(token: StoredToken): Promise<void> {
    this.setCallCount++;
    if (this.rejectNextSet) {
      this.rejectNextSet = false;
      throw new Error(
        'simulated TokenStore.set() failure (e.g. Redis unreachable)',
      );
    }
    this.token = token;
  }

  clear(): void {
    this.token = undefined;
  }
}

void test('auth.getAccessToken: tokenStore.set() 第一次写入失败时，该次调用必须 reject，且不得把这个未持久化的值当作本地新鲜缓存复用', async () => {
  const {mockAgent, pool} = createMockAgent();
  const tokenStore = new RejectOnceTokenStore(/* rejectFirstSet= */ true);

  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'token-from-failed-write',
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
    tokenStore,
  });

  await expectFailure(() => client.auth.getAccessToken());
  assert.equal(
    tokenStore.setCallCount,
    1,
    'tokenStore.set() 应当被调用了一次（即使它失败了）',
  );

  // 紧接着的下一次调用：如果 cachedToken 被错误地提前赋值，这里会直接
  // resolve 成 "token-from-failed-write" 而不发起任何真实请求
  // （disableNetConnect 会让请求因找不到拦截器而报错，从而暴露这个 bug）。
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'token-from-good-write',
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    }),
  );

  const token = await client.auth.getAccessToken();
  assert.equal(
    token,
    'token-from-good-write',
    '第一次写入失败之后，下一次调用应当真实重新发起 CLIENT_CREDENTIALS 换取，而不是复用从未成功持久化的本地缓存',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '第二次真实请求的拦截器应当被消费',
  );
});
