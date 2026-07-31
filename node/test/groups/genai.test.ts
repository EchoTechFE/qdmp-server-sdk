/**
 * genai.generate + genai.detail 测试：genai 鉴权方案（x-qdmp-auth-scheme:
 * "genai"），独立请求头——只用 x-openapi-access-token + x-openapi-app-id，不带
 * standard 方案的 access-token / x-echo-qdmp-version。x-openapi-app-id 取自
 * QdmpClient 构造时的 appId（客户端自身配置），不是调用方传入的参数。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';
import type {MockAgent} from 'undici';

import {QdmpClient} from '../../src/client.js';
import {QdmpApiError} from '../../src/errors.js';
import {
  businessFailure,
  businessSuccess,
  createMockAgent,
  expectFailure,
} from '../helpers/mock-http.js';

function makeClient(mockAgent: MockAgent, appId = 'app-id-1') {
  return new QdmpClient({
    appId,
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('genai.detail: 正常成功路径，query.id 被正确透传，使用独立请求头 x-openapi-access-token + x-openapi-app-id', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/genai/v1/detail',
      method: 'GET',
      query: {id: 'task-1'},
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(200, businessSuccess({id: 'task-1', status: 'COMPLETED'}));

  const client = makeClient(mockAgent, 'my-genai-app-id');
  const result = await client.genai.detail(
    {accessToken: 'genai-token-1'},
    {id: 'task-1'},
  );

  assert.equal(result.id, 'task-1');
  assert.equal(result.status, 'COMPLETED');
  assert.equal(capturedHeaders?.['x-openapi-access-token'], 'genai-token-1');
  assert.equal(capturedHeaders?.['x-openapi-app-id'], 'my-genai-app-id');
  assert.equal(capturedHeaders?.['access-token'], undefined);
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], undefined);
});

void test('genai.detail: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/genai/v1/detail', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.genai.detail({}, {id: 'task-1'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('genai.detail: HTTP 200 但业务 code !== "0" 判定为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/genai/v1/detail', method: 'GET', query: {id: 'task-1'}})
    .reply(200, businessFailure(10005, 'access_token 格式错误或已吊销'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.genai.detail({accessToken: 'bad-genai-token'}, {id: 'task-1'}),
  );
  assert.ok(err instanceof QdmpApiError);
  assert.equal(String((err as QdmpApiError).code), '10005');
});

void test('genai.generate: 正常成功路径，使用独立请求头 x-openapi-access-token + x-openapi-app-id，不带 standard 方案的请求头', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/genai/v1/generate',
      method: 'POST',
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(200, businessSuccess({id: 'task-1'}));

  const client = makeClient(mockAgent, 'my-genai-app-id');
  const result = await client.genai.generate(
    {accessToken: 'genai-token-1'},
    {
      model: 'google/gemini-3.1-flash-image-preview',
      contents: [{role: 'user', parts: [{text: 'hello'}]}],
    },
  );

  assert.equal(result.id, 'task-1');
  assert.equal(capturedHeaders?.['x-openapi-access-token'], 'genai-token-1');
  assert.equal(
    capturedHeaders?.['x-openapi-app-id'],
    'my-genai-app-id',
    'x-openapi-app-id 应取自 QdmpClient 构造时配置的 appId',
  );
  assert.equal(
    capturedHeaders?.['access-token'],
    undefined,
    'genai 方案不应携带 standard 方案专用的 access-token 头',
  );
  assert.equal(
    capturedHeaders?.['x-echo-qdmp-version'],
    undefined,
    'genai 方案不应携带 standard 方案专用的 x-echo-qdmp-version 头',
  );
});

void test('genai.generate: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/genai/v1/generate', method: 'POST'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    client.genai.generate(
      // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
      {},
      {model: 'm', contents: []},
    ),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('genai.generate: HTTP 200 但业务 code !== "0" 判定为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/genai/v1/generate', method: 'POST'})
    .reply(200, businessFailure(10005, 'access_token 格式错误或已吊销'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.genai.generate(
      {accessToken: 'bad-genai-token'},
      {model: 'm', contents: [{role: 'user', parts: [{text: 'hi'}]}]},
    ),
  );
  assert.ok(err instanceof QdmpApiError);
  assert.equal(String((err as QdmpApiError).code), '10005');
});
