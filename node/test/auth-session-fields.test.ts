/**
 * auth.code2Session() 字段完整性校验测试。
 *
 * 需求：code2Session() 除了要求 accessToken/expiresAt 合法（已由
 * assertWellFormedTokenResponse() 覆盖，见 auth-malformed-success.test.ts），
 * 还必须校验业务层返回的 refreshToken/openId 非空 —— 这两个字段是
 * SessionData 的一部分（见 src/types.ts），调用方（宿主后端）需要拿
 * refreshToken 去后续换取新 token、拿 openId 去关联本地用户，一旦
 * 静默 resolve 出 undefined/空字符串，后续业务逻辑会在错误的地方失败
 * （比如把空字符串当 openId 存进数据库）。
 *
 * 现状（本测试写下时）：AuthModule.code2Session()（src/auth.ts 约152-172行）
 * 只调用 assertWellFormedTokenResponse(session, ...) 校验 accessToken/expiresAt，
 * 完全不检查 session.refreshToken / session.openId 是否存在或非空。
 * 因此下面的测试预期是失败的（红色状态）：当前实现会把畸形 session 直接
 * resolve 给调用方，而不是 reject。
 */
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

function sessionData(
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    accessToken: 'session-access-abc',
    refreshToken: 'session-refresh-abc',
    expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    openId: 'open-id-abc',
    ...overrides,
  };
}

function makeClient(
  mockAgent: ReturnType<typeof createMockAgent>['mockAgent'],
) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('auth.code2Session: refreshToken 字段完全缺失时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  // sessionData({refreshToken: undefined}) 展开后 refreshToken 的值是
  // undefined，JSON.stringify（undici MockAgent 序列化 reply body 时会用到）
  // 会直接丢弃 undefined 值的 key，等价于"响应里完全没有这个字段"。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(sessionData({refreshToken: undefined})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.code2Session('some-auth-code'));
});

void test('auth.code2Session: refreshToken 为空字符串时应 reject，不得把空字符串当作合法 refreshToken resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(sessionData({refreshToken: ''})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.code2Session('some-auth-code'));
});

void test('auth.code2Session: openId 字段完全缺失时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(sessionData({openId: undefined})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.code2Session('some-auth-code'));
});

void test('auth.code2Session: openId 为空字符串时应 reject，不得把空字符串当作合法 openId resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(sessionData({openId: ''})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.code2Session('some-auth-code'));
});
