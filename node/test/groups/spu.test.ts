/**
 * spu.detail + spu.search 测试：standard 鉴权方案。
 *
 * spu.detail 覆盖点：单个 query.id 透传、成功路径解析 data.spu、缺失
 * accessToken 本地校验、网关层错误信封。
 *
 * spu.search 覆盖点：多个可选 query 参数（含 uint64 线格式为 string 的
 * offset/limit，以及 filterOptions 字符串数组）正确序列化、成功路径解析
 * data.items[]、以及第二种网关层错误信封 {code,message,details}
 * （shared/openapi.yaml 明确记录 tag/v1/search 参数校验失败会走这种信封，
 * spu/v1/search 与 tag/v1/search 同属 standard 方案下的查询类端点，此处按
 * 同一份 GatewayErrorEnvelope schema 建模验证兼容性）。
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

void test('spu.detail: 正常成功路径，query.id 被正确透传，standard 请求头齐全', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/spu/v1/detail',
      method: 'GET',
      query: {id: 'spu-1'},
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        spu: {id: 'spu-1', name: 'Labubu 盲盒'},
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.spu.detail(
    {accessToken: 'user-token-1'},
    {id: 'spu-1'},
  );

  assert.equal(result.spu?.id, 'spu-1');
  assert.equal(result.spu?.name, 'Labubu 盲盒');
  assert.equal(capturedHeaders?.['access-token'], 'user-token-1');
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], '1.0');
});

void test('spu.detail: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/spu/v1/detail', method: 'GET'})
    .reply(200, businessSuccess({spu: {id: 'should-not-be-reached'}}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.spu.detail({}, {id: 'spu-1'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('spu.detail: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/spu/v1/detail',
      method: 'GET',
      query: {id: 'missing-spu'},
    })
    .reply(404, gatewayFailure(5, 'spu not found'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.spu.detail({accessToken: 'user-token-1'}, {id: 'missing-spu'}),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `应解析为 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '5');
  assert.equal(apiErr.httpStatus, 404);
});

void test('spu.search: 正常成功路径，query 参数（含数组 filterOptions）被正确序列化', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/spu/v1/search',
      method: 'GET',
      query: {
        keyword: 'labubu',
        offset: '0',
        limit: '20',
        onlyMega: 'true',
        filterOptions: ['101', '202'],
      },
    })
    .reply(
      200,
      businessSuccess({
        items: [{id: 'spu-1', name: 'Labubu 盲盒'}],
      }),
    );

  const client = makeClient(mockAgent);
  const result = await client.spu.search(
    {accessToken: 'user-token-1'},
    {
      keyword: 'labubu',
      offset: '0',
      limit: '20',
      onlyMega: true,
      filterOptions: ['101', '202'],
    },
  );

  assert.equal(result.items?.length, 1);
  assert.equal(result.items?.[0]?.id, 'spu-1');
});

void test('spu.search: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/spu/v1/search', method: 'GET'})
    .reply(200, businessSuccess({items: [{id: 'should-not-be-reached'}]}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.spu.search({}, {keyword: 'labubu'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('spu.search: 网关层错误信封 {code,message,details}（无 data/requestId）也必须被正确解析为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    // NOTE(implementer): added `query: {keyword: '???'}` — the call below
    // passes {keyword: '???'}, so the real outgoing request necessarily
    // carries a `?keyword=...` query string. undici's MockAgent path
    // matching is an exact string match (see mock-utils.js matchKey /
    // safeUrl): a query-less declared path can never match a request path
    // that does carry a query string, for any implementation. Verified with
    // a minimal repro against undici 6.28.0 (this project's actual declared
    // and installed dependency, see package.json / mock-http.ts) directly
    // (bypassing this SDK) before concluding this was a test bug rather than
    // an implementation bug. This is a one-line addition, not a behavior
    // change to the test's assertions.
    .intercept({path: '/spu/v1/search', method: 'GET', query: {keyword: '???'}})
    .reply(500, gatewayFailure(13, 'query parse error'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.spu.search({accessToken: 'user-token-1'}, {keyword: '???'}),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `应解析为 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '13');
  assert.equal(apiErr.httpStatus, 500);
});
