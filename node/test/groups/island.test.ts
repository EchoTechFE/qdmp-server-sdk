/**
 * island.detail 测试：唯一的 island 分组端点，standard 鉴权方案，
 * x-qdmp-token-required: true（必须传 accessToken），query 参数为单个 id。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';
import type {MockAgent} from 'undici';

import {QdmpClient} from '../../src/client.js';
import {QdmpApiError} from '../../src/errors.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
  gatewayFailure,
} from '../helpers/mock-http.js';

function makeClient(mockAgent: MockAgent) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('island.detail: 正常成功路径，query.id 被正确透传，standard 请求头齐全', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/island/v1/detail',
      method: 'GET',
      query: {id: 'island-1'},
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        island: {id: 'island-1', name: 'Labubu 岛', joined: true},
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.island.detail(
    {accessToken: 'user-token-1'},
    {id: 'island-1'},
  );

  assert.equal(result.island?.id, 'island-1');
  assert.equal(result.island?.name, 'Labubu 岛');
  assert.equal(capturedHeaders?.['access-token'], 'user-token-1');
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], '1.0');
});

void test('island.detail: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/island/v1/detail', method: 'GET'})
    .reply(200, businessSuccess({island: {id: 'should-not-be-reached'}}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.island.detail({}, {id: 'island-1'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('island.detail: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/island/v1/detail',
      method: 'GET',
      query: {id: 'missing-island'},
    })
    .reply(404, gatewayFailure(5, 'island not found'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.island.detail({accessToken: 'user-token-1'}, {id: 'missing-island'}),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `应解析为 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '5');
  assert.equal(apiErr.httpStatus, 404);
});
