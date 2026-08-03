/**
 * auth 模块"已过期即到达（expired-on-arrival）"token 测试。
 *
 * 需求：服务端返回 HTTP 200 + 业务 code='0'（判定为成功）且 accessToken/
 * expiresAt 语法上都合法，但 expiresAt 是一个早已过期的时间戳（例如服务端
 * bug 返回 "1"，即 1970 年 Unix epoch 之后 1 秒）时，refreshAppToken() 必须
 * reject，而不能把这个"语法合法但语义上已过期"的 token 当作新鲜 token
 * resolve 给调用方、写进 this.cachedToken / tokenStore。
 *
 * 现状（本测试写下时）：src/auth.ts 的 assertWellFormedTokenResponse()（约
 * 27-54行）只检查 accessToken 非空、expiresAt 是否是语法上合法的非负整数
 * 字符串，完全不检查它是否已经过期。isFresh(token)（约127-131行）本可以做
 * 这个"是否新鲜"的判断（expiresAt - now > 300秒缓冲），但 refreshAppToken()
 * （约147-164行）在拿到刚请求回来的 token 后，只调用了
 * assertWellFormedTokenResponse()，从未对这个新 token 调用 isFresh() 校验，
 * 就直接写入 this.cachedToken / tokenStore 并 resolve 给调用方。因此下面的
 * 测试预期是失败的（红色状态）：当前实现会 resolve 成 'app-token-abc'，而不是
 * reject。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

void test('auth.getAppAccessToken: HTTP 200 + {code:0} 但 expiresAt 是早已过期的时间戳（"1"，1970年）时应 reject，且不得把这个已过期 token 当作新鲜缓存写入（下一次调用仍会真实重新换取）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(
      200,
      businessSuccess({accessToken: 'app-token-abc', expiresAt: '1'}),
    );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());

  // 验证这个"已过期即到达"的坏值没有被当作新鲜 token 缓存下来：只注册这一个
  // 新拦截器，若上一次的失败结果被误当成功缓存，这里就不会真的再发一次 HTTP
  // 请求，disableNetConnect 会让请求因找不到拦截器而报错。
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'app-token-good',
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    }),
  );

  const token = await client.auth.getAppAccessToken();
  assert.equal(
    token.accessToken,
    'app-token-good',
    '前一次已过期 token 被拒绝之后，下一次调用应当真实重新发起 CLIENT_CREDENTIALS 换取，而不是复用坏缓存',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '第二次真实请求的拦截器应当被消费',
  );
});

// 回归测试（codex 收敛检查轮发现）：getUserAccessToken/refreshToken 这两个"一次性、
// 不缓存"的路径同样只校验 expiresAt 是否语法合法（非负整数字符串），没有校验
// 它是否早已过期——一次性会话/token 虽然不走本 SDK 的缓存，但如果早已过期就
// 返回给调用方，调用方可能会把这个"看起来成功"的死会话持久化下来。
void test('auth.getUserAccessToken: expiresAt 是早已过期的时间戳（"1"）时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'user-access-token',
      refreshToken: 'user-refresh-token',
      expiresAt: '1',
      openId: 'open-id-1',
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getUserAccessToken('auth-code-abc'));
});

void test('auth.refreshToken: expiresAt 是早已过期的时间戳（"1"）时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(200, businessSuccess({accessToken: 'new-token', expiresAt: '1'}));

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.refreshToken('some-refresh-token'));
});
