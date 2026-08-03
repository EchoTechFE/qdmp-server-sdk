/**
 * auth 模块畸形"成功"响应测试（对应 codex review round1 发现）：
 * 服务端返回 HTTP 200 + code='0'（业务上判定为成功），但 data 缺失，或
 * data.accessToken 缺失/空字符串时，调用方必须 reject，而不能：
 *   - resolve 成 undefined / 空字符串；
 *   - 把这个坏值写进 tokenStore，污染后续调用的缓存。
 *
 * 覆盖 getAppAccessToken()（CLIENT_CREDENTIALS）与 refreshToken()。
 *
 * 现状（本测试写下时）：
 * - parseEnvelope() 只用 `String(code) === '0'` 判定成功，不检查 data 内容
 *   （`body.data ?? {}` 甚至允许 data 完全缺失）；
 * - AuthModule 的三个方法都直接把 http.request() 的结果透传给调用方，不做
 *   accessToken 存在性/非空校验。
 * 因此下面的测试预期是失败的（红色状态）：当前实现会 resolve 成 undefined
 * 或空字符串，而不是 reject。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

function tokenSuccessData(overrides: Record<string, unknown> = {}) {
  return {
    accessToken: 'app-token-abc',
    expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    ...overrides,
  };
}

void test('auth.getAppAccessToken: HTTP 200 + {code:0} 但完全没有 data 字段时应 reject，且不得把坏值写进 tokenStore（下一次调用仍会真实重新换取）', async () => {
  const {mockAgent, pool} = createMockAgent();
  // 故意不用 businessSuccess() 帮助函数：这里就是要构造"code=0 但没有 data 字段"
  // 这种畸形成功响应。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, {code: 0, message: 'ok', requestId: 'req-malformed-1'});

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());

  // 验证失败之后没有任何坏值被当作"已缓存的新鲜 token"留存下来：
  // 只注册这一个新拦截器，若上一次的失败结果被误当成功缓存，这里就不会真的
  // 再发一次 HTTP 请求，disableNetConnect 会让请求因找不到拦截器而报错。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(
      200,
      businessSuccess(tokenSuccessData({accessToken: 'app-token-good'})),
    );

  const token = await client.auth.getAppAccessToken();
  assert.equal(
    token.accessToken,
    'app-token-good',
    '前一次畸形响应失败之后，下一次调用应当真实重新发起 CLIENT_CREDENTIALS 换取，而不是复用坏缓存',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '第二次真实请求的拦截器应当被消费',
  );
});

void test('auth.getAppAccessToken: data 存在但 data.accessToken 为空字符串时应 reject，不得把空字符串当作合法 token resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: '',
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());
});

void test('auth.getAppAccessToken: data.expiresAt 为空字符串时应 reject（Number("")===0 曾让这个畸形值被当成"已过期但合法"的数字通过校验）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'app-token-abc',
      expiresAt: '',
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());
});

void test('auth.getAppAccessToken: data.expiresAt 为纯空白字符串时应 reject（Number("   ")===0 曾让它被当成合法数字通过校验）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'app-token-abc',
      expiresAt: '   ',
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());
});

void test('auth.getAppAccessToken: data.expiresAt 为科学计数法（如 "1e100"）时应 reject（Go/Java 的严格整数解析都会拒绝这类格式，Node 不应更宽松）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'app-token-abc',
      expiresAt: '1e100',
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.getAppAccessToken());
});

void test('auth.refreshToken: data.accessToken 为空字符串时应 reject，不得把空字符串 resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(200, businessSuccess({accessToken: ''}));

  const client = new QdmpClient({
    appId: 'app-1',
    appSecret: 'secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.refreshToken('some-refresh-token'));
});
