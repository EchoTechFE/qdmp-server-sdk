/**
 * wishspu 分组测试：add/cancel/list 三个端点，standard 鉴权方案。add/cancel
 * 用请求体透传 ids 数组 + type，list 用 query 分页参数。
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

void test('wishspu.add: 正常成功路径，请求体透传 ids/type，standard 请求头齐全', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/wishspu/v1/add',
      method: 'POST',
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
    })
    .reply(200, businessSuccess({successCount: '2'}));

  const client = makeClient(mockAgent);
  const result = await client.wishspu.add(
    {accessToken: 'user-token-1'},
    {ids: ['spu-1', 'spu-2'], type: 'WISH_SPU_TYPE_SPU'},
  );

  assert.equal(result.successCount, '2');
  assert.deepEqual(capturedBody, {
    ids: ['spu-1', 'spu-2'],
    type: 'WISH_SPU_TYPE_SPU',
  });
  assert.equal(capturedHeaders?.['access-token'], 'user-token-1');
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], '1.0');
});

void test('wishspu.add: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/wishspu/v1/add', method: 'POST'})
    .reply(200, businessSuccess({successCount: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    client.wishspu.add(
      // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
      {},
      {ids: ['spu-1'], type: 'WISH_SPU_TYPE_SPU'},
    ),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('wishspu.cancel: 正常成功路径，请求体透传 ids/type', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  pool
    .intercept({
      path: '/wishspu/v1/cancel',
      method: 'POST',
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
    })
    .reply(200, businessSuccess({successCount: '1'}));

  const client = makeClient(mockAgent);
  const result = await client.wishspu.cancel(
    {accessToken: 'user-token-1'},
    {ids: ['spu-1'], type: 'WISH_SPU_TYPE_SPU'},
  );

  assert.equal(result.successCount, '1');
  assert.deepEqual(capturedBody, {ids: ['spu-1'], type: 'WISH_SPU_TYPE_SPU'});
});

void test('wishspu.list: 正常成功路径，query 分页参数被正确序列化', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/wishspu/v1/list',
      method: 'GET',
      query: {typeId: 'type-1', offset: '0', limit: '20'},
    })
    .reply(
      200,
      businessSuccess({
        items: [{id: 'wish-1', spu: {id: 'spu-1', name: 'Labubu'}}],
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.wishspu.list(
    {accessToken: 'user-token-1'},
    {typeId: 'type-1', offset: '0', limit: '20'},
  );

  assert.equal(result.items?.length, 1);
  assert.equal(result.items?.[0]?.id, 'wish-1');
});

void test('wishspu.list: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/wishspu/v1/list', method: 'GET'})
    .reply(200, businessSuccess({items: [{id: 'should-not-be-reached'}]}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.wishspu.list({}, {offset: '0', limit: '20'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('wishspu.add: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/wishspu/v1/add', method: 'POST'})
    .reply(500, gatewayFailure(2, 'rpc error: ... openid is required'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.wishspu.add(
      {accessToken: 'app-level-token'},
      {ids: ['spu-1'], type: 'WISH_SPU_TYPE_SPU'},
    ),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `网关层信封（无 data/requestId）也应该被解析成 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '2');
  assert.equal(apiErr.httpStatus, 500);
});
