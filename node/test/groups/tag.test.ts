/**
 * tag 分组测试：detail + search 两个端点，standard 鉴权方案。search 覆盖多个
 * 可选 query 参数（含 filterOptions 字符串数组、uint64 线格式为 string 的
 * offset/limit）。
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

function makeClient(mockAgent: MockAgent) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('tag.detail: 正常成功路径，query.id 被正确透传', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/tag/v1/detail',
      method: 'GET',
      query: {id: 'tag-1'},
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(200, businessSuccess({tag: {id: 'tag-1', name: 'Molly'}}));

  const client = makeClient(mockAgent);
  const result = await client.tag.detail(
    {accessToken: 'user-token-1'},
    {id: 'tag-1'},
  );

  assert.equal(result.tag?.id, 'tag-1');
  assert.equal(result.tag?.name, 'Molly');
  assert.equal(capturedHeaders?.['access-token'], 'user-token-1');
  assert.equal(capturedHeaders?.['x-echo-qdmp-version'], '1.0');
});

void test('tag.detail: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/tag/v1/detail', method: 'GET'})
    .reply(200, businessSuccess({tag: {id: 'should-not-be-reached'}}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.tag.detail({}, {id: 'tag-1'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('tag.search: 正常成功路径，query 参数（含数组 filterOptions）被正确序列化', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/tag/v1/search',
      method: 'GET',
      query: {
        keyword: 'molly',
        typeId: 'type-1',
        classification: 'ip',
        offset: '0',
        limit: '20',
        filterOptions: ['101', '202'],
      },
    })
    .reply(200, businessSuccess({items: [{id: 'tag-1', name: 'Molly'}]}));

  const client = makeClient(mockAgent);
  const result = await client.tag.search(
    {accessToken: 'user-token-1'},
    {
      keyword: 'molly',
      typeId: 'type-1',
      classification: 'ip',
      offset: '0',
      limit: '20',
      filterOptions: ['101', '202'],
    },
  );

  assert.equal(result.items?.length, 1);
  assert.equal(result.items?.[0]?.id, 'tag-1');
});

void test('tag.search: 缺失 accessToken 时本地直接报错，不发起任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/tag/v1/search', method: 'GET'})
    .reply(200, businessSuccess({items: [{id: 'should-not-be-reached'}]}));

  const client = makeClient(mockAgent);

  await expectFailure(() =>
    // @ts-expect-error 故意不传 accessToken，验证运行时本地校验
    client.tag.search({}, {keyword: 'molly'}),
  );

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '缺少 accessToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('tag.search: HTTP 200 但业务 code !== "0" 判定为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  // NOTE: undici's MockAgent path matching is an exact string match, so a
  // query-less declared path can never match a request that does carry a
  // query string (here `?keyword=` from the empty-but-defined keyword) — see
  // test/groups/spu.test.ts for the full explanation. This is a one-line
  // addition, not a behavior change to the assertions below.
  pool
    .intercept({path: '/tag/v1/search', method: 'GET', query: {keyword: ''}})
    .reply(200, businessFailure(20000, 'keyword 参数非法'));

  const client = makeClient(mockAgent);

  const err = await expectFailure(() =>
    client.tag.search({accessToken: 'user-token-1'}, {keyword: ''}),
  );
  assert.ok(
    err instanceof QdmpApiError,
    `应解析为 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '20000');
  assert.equal(apiErr.httpStatus, 200);
});
