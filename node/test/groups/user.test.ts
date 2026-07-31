/**
 * user.me 测试：唯一的 user 分组端点，standard 鉴权方案，
 * x-qdmp-token-required: true（必须传用户级/app 级 accessToken）。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';
import type {MockAgent} from 'undici';

import {QdmpClient} from '../../src/client.js';
import {QdmpApiError, QdmpValidationError} from '../../src/errors.js';
import {
  businessFailure,
  businessSuccess,
  createMockAgent,
  expectFailure,
} from '../helpers/mock-http.js';

function makeClient(mockAgent: MockAgent) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('user.me: 正常成功路径，解析业务信封 {code,message,requestId,data} 并透传 standard 请求头', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/user/v1/me',
      method: 'GET',
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        id: 'u-1',
        nickname: 'Alice',
        avatar: 'https://cdn.example.com/a.png',
      }),
    );

  const client = makeClient(mockAgent);
  const me = await client.user.me({accessToken: 'user-token-abc'});

  assert.equal(me.id, 'u-1');
  assert.equal(me.nickname, 'Alice');
  assert.equal(capturedHeaders?.['access-token'], 'user-token-abc');
  assert.equal(
    capturedHeaders?.['x-echo-qdmp-version'],
    '1.0',
    'standard 鉴权方案默认应带上 x-echo-qdmp-version:1.0',
  );
});

void test('user.me: 缺失 accessToken 时本地直接抛出 QdmpValidationError，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  // @ts-expect-error 故意不传 accessToken，验证运行时本地校验（ctx.accessToken 是必填字段）
  const err = await expectFailure(() => client.user.me({}));

  assert.ok(
    err instanceof QdmpValidationError,
    `本地校验失败应抛出 QdmpValidationError，实际抛出: ${String(err)}`,
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('user.me: accessToken 为空字符串时本地直接抛出 QdmpValidationError，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() => client.user.me({accessToken: ''}));

  assert.ok(
    err instanceof QdmpValidationError,
    `空字符串 accessToken 应被本地校验拒绝并抛出 QdmpValidationError，实际抛出: ${String(err)}`,
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    'accessToken 为空字符串时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('user.me: HTTP 401 + code=10005（access-token 失效）判定为失败，且错误不泄露 accessToken 原文', async () => {
  const secretToken = 'LEAK-CHECK-user-token-9f8e7d';
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(401, businessFailure(10005, 'access_token 格式错误或已吊销'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.user.me({accessToken: secretToken}),
  );
  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '10005');
  assert.equal(
    apiErr.httpStatus,
    401,
    '实测确认：access-token 失效是真实 HTTP 401，不是 HTTP 200 携带错误 code',
  );

  const rendered = `${apiErr.message ?? ''} ${String(apiErr)} ${JSON.stringify(apiErr)}`;
  assert.ok(
    !rendered.includes(secretToken),
    `错误信息中不应包含 accessToken 原文，实际: ${rendered}`,
  );
});
