/**
 * auth.getUserAccessToken() 字段完整性校验测试。
 *
 * 需求：getUserAccessToken() 除了要求 accessToken/expiresAt 合法（已由
 * assertWellFormedTokenResponse() 覆盖，见 auth-malformed-success.test.ts），
 * 还必须校验业务层返回的 refreshToken/openId 非空 —— 这两个字段是
 * UserAccessTokenData 的一部分（见 src/types.ts），调用方（宿主后端）需要拿
 * refreshToken 去后续换取新 token、拿 openId 去关联本地用户，一旦
 * 静默 resolve 出 undefined/空字符串，后续业务逻辑会在错误的地方失败
 * （比如把空字符串当 openId 存进数据库）。
 *
 * 现状（本测试写下时）：AuthModule.getUserAccessToken()（src/auth.ts 约152-172行）
 * 只调用 assertWellFormedTokenResponse(userCredential, ...) 校验 accessToken/expiresAt，
 * 完全不检查 userCredential.refreshToken / userCredential.openId 是否存在或非空。
 * 因此下面的测试预期是失败的（红色状态）：当前实现会把畸形 userCredential 直接
 * resolve 给调用方，而不是 reject。
 */
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

function userCredentialData(
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    accessToken: 'userCredential-access-abc',
    refreshToken: 'userCredential-refresh-abc',
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

void test('auth.getUserAccessToken: refreshToken 字段完全缺失时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  // userCredentialData({refreshToken: undefined}) 展开后 refreshToken 的值是
  // undefined，JSON.stringify（undici MockAgent 序列化 reply body 时会用到）
  // 会直接丢弃 undefined 值的 key，等价于"响应里完全没有这个字段"。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(userCredentialData({refreshToken: undefined})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.getUserAccessToken('some-auth-code'));
});

void test('auth.getUserAccessToken: refreshToken 为空字符串时应 reject，不得把空字符串当作合法 refreshToken resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(userCredentialData({refreshToken: ''})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.getUserAccessToken('some-auth-code'));
});

void test('auth.getUserAccessToken: openId 字段完全缺失时应 reject', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(userCredentialData({openId: undefined})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.getUserAccessToken('some-auth-code'));
});

void test('auth.getUserAccessToken: openId 为空字符串时应 reject，不得把空字符串当作合法 openId resolve 给调用方', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(userCredentialData({openId: ''})));

  const client = makeClient(mockAgent);
  await expectFailure(() => client.auth.getUserAccessToken('some-auth-code'));
});
