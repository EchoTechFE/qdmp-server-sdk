/**
 * mark 分组测试：list/search/detail/add 四个端点，均为 standard 鉴权方案，
 * x-qdmp-token-required: true（必须传 accessToken）。
 *
 * 覆盖点：各端点的 query/body 映射（list 的 limit/offset、search 的
 * typeId/limit/offset、detail 的 id/limit/offset、add 的 spuId/rating）、
 * 缺失 accessToken 时的本地校验、以及网关层错误信封 {code,message,details}
 * （无 data/requestId）——按 shared/openapi.yaml 对 marks 分组的实测描述建模：
 * 用缺失用户身份的 token 调用 mark 系列端点会在业务层被拒绝，网关返回类 gRPC
 * 小整数 code（如 2 "openid is required"），与 auth/user 用到的业务信封结构
 * 不同。
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

void test('mark.list: 正常成功路径，query.limit/offset 被正确透传，解析业务信封 data', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/list',
      method: 'GET',
      query: {limit: '20', offset: '0'},
    })
    .reply(
      200,
      businessSuccess({
        items: [{id: 'mark-1', markAt: '2026-01-01T00:00:00Z'}],
        totalCount: '1',
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.mark.list(
    {accessToken: 'user-token-1'},
    {limit: '20', offset: '0'},
  );

  assert.equal(result.items?.length, 1);
  assert.equal(result.items?.[0]?.id, 'mark-1');
  assert.equal(result.totalCount, '1');
});

void test('mark.list: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/mark/v1/me/list', method: 'GET'})
    .reply(200, businessSuccess({items: [{id: 'should-not-be-reached'}]}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.mark.list({}, {limit: '20', offset: '0'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('mark.list: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/list',
      method: 'GET',
      query: {limit: '20', offset: '0'},
    })
    .reply(500, gatewayFailure(2, 'rpc error: ... openid is required'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.mark.list(
      {accessToken: 'app-level-token'},
      {limit: '20', offset: '0'},
    ),
  );
  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '2');
  assert.equal(apiErr.httpStatus, 500);
});

void test('mark.search: 正常成功路径，query.typeId/limit/offset 被正确透传', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/search',
      method: 'GET',
      query: {typeId: 'type-1', limit: '10', offset: '5'},
    })
    .reply(
      200,
      businessSuccess({
        items: [{id: 'mark-2', typeId: 'type-1'}],
        totalCount: '1',
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.mark.search(
    {accessToken: 'user-token-1'},
    {typeId: 'type-1', limit: '10', offset: '5'},
  );

  assert.equal(result.items?.length, 1);
  assert.equal(result.items?.[0]?.id, 'mark-2');
});

void test('mark.search: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/mark/v1/me/search', method: 'GET'})
    .reply(200, businessSuccess({items: [{id: 'should-not-be-reached'}]}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.mark.search({}, {limit: '10', offset: '0'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('mark.search: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/search',
      method: 'GET',
      query: {limit: '10', offset: '0'},
    })
    .reply(500, gatewayFailure(2, 'rpc error: ... openid is required'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.mark.search(
      {accessToken: 'app-level-token'},
      {limit: '10', offset: '0'},
    ),
  );
  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '2');
  assert.equal(apiErr.httpStatus, 500);
});

void test('mark.detail: 正常成功路径，query.id/limit/offset 被正确透传', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/detail',
      method: 'GET',
      query: {id: 'mark-1', limit: '5', offset: '0'},
    })
    .reply(
      200,
      businessSuccess({
        id: 'mark-1',
        spu: {id: 'spu-1', name: 'Labubu 盲盒'},
        marks: [{id: 'history-1', markAt: '2026-01-01T00:00:00Z'}],
        hasMore: false,
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.mark.detail(
    {accessToken: 'user-token-1'},
    {id: 'mark-1', limit: '5', offset: '0'},
  );

  assert.equal(result.id, 'mark-1');
  assert.equal(result.spu?.id, 'spu-1');
  assert.equal(result.marks?.length, 1);
  assert.equal(result.hasMore, false);
});

void test('mark.detail: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/mark/v1/me/detail', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.mark.detail({}, {id: 'mark-1', limit: '5', offset: '0'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('mark.detail: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/me/detail',
      method: 'GET',
      query: {id: 'missing-mark', limit: '5', offset: '0'},
    })
    .reply(404, gatewayFailure(5, 'mark not found'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.mark.detail(
      {accessToken: 'user-token-1'},
      {id: 'missing-mark', limit: '5', offset: '0'},
    ),
  );
  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '5');
  assert.equal(apiErr.httpStatus, 404);
});

void test('mark.add: 正常成功路径，请求体透传 spuId/rating，解析业务信封 data', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/mark/v1/add',
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
    .reply(200, businessSuccess({id: 'mark-1'}));

  const client = makeClient(mockAgent);
  const result = await client.mark.add(
    {accessToken: 'user-token-1'},
    {spuId: '12345', rating: {value: 5}},
  );

  assert.equal(result.id, 'mark-1');
  assert.deepEqual(capturedBody, {spuId: '12345', rating: {value: 5}});
  assert.equal(capturedHeaders?.['access-token'], 'user-token-1');
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], '1.0');
});

void test('mark.add: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/mark/v1/add', method: 'POST'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.mark.add({}, {spuId: '12345'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('mark.add: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  // HTTP 500 + 类 gRPC 小整数 code（本项目实测在 marks 分组见过 code=2 "openid is required"）。
  pool
    .intercept({path: '/mark/v1/add', method: 'POST'})
    .reply(500, gatewayFailure(2, 'rpc error: ... openid is required'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.mark.add({accessToken: 'app-level-token'}, {spuId: '12345'}),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `网关层信封（无 data/requestId）也应该被解析成 QdmpApiError，而不是解析失败抛出无关的 parse error，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '2');
  assert.equal(apiErr.httpStatus, 500);
});
