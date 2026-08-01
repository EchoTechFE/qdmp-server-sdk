/**
 * 传输层安全测试：禁止跟随任意/跨域重定向。
 *
 * 需求（对应 codex review round1 发现）：HttpClient.request() 收到的响应
 * HTTP status 落在 [300, 400) 区间（任何 3xx 重定向）时，必须**不跟随重定向**，
 * 而是 reject 一个 QdmpTransportError 实例，且这个 error 的 message/toString()
 * 不能包含 accessToken 原文。
 *
 * 现状（本测试写下时）：QdmpTransportError 尚不存在于 src/errors.ts，且
 * HttpClient.request() 没有对 3xx 做任何特殊处理（Node 全局 fetch 默认
 * `redirect: 'follow'`），因此这些测试预期是失败的（红色状态）。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {QdmpTransportError} from '../src/errors.js';
import {createMockAgent, expectFailure} from './helpers/mock-http.js';

void test('HttpClient.request: 收到 302 + Location 指向另一 origin 时必须 reject QdmpTransportError，不能跟随重定向', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/user/v1/me', method: 'GET'}).reply(302, '', {
    headers: {location: 'https://evil-redirect-target.example.com/steal'},
  });

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() =>
    client.user.me({accessToken: 'sentinel-secret-token'}),
  );

  assert.ok(
    err instanceof QdmpTransportError,
    `收到 3xx 重定向响应时应当 reject QdmpTransportError，实际抛出: ${String(err)}`,
  );
});

void test('HttpClient.request: 3xx 重定向触发的 QdmpTransportError 不得在 message/toString() 中泄露 accessToken 原文', async () => {
  const secretToken = 'sentinel-secret-token';
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/user/v1/me', method: 'GET'}).reply(301, '', {
    headers: {location: 'https://another-origin.example.com/anywhere'},
  });

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() =>
    client.user.me({accessToken: secretToken}),
  );

  assert.ok(
    err instanceof QdmpTransportError,
    `应当 reject QdmpTransportError，实际抛出: ${String(err)}`,
  );
  const transportErr = err as QdmpTransportError;
  const rendered = `${transportErr.message ?? ''} ${String(transportErr)}`;
  assert.ok(
    !rendered.includes(secretToken),
    `QdmpTransportError 的 message/toString() 不应包含 accessToken 原文，实际: ${rendered}`,
  );
});

void test('HttpClient.request: 即使 Location host 本身反射了 accessToken，QdmpTransportError 也不能包含任何 Location 派生内容', async () => {
  const secretToken = 'sentinel-secret-token';
  const {mockAgent, pool} = createMockAgent();
  // 服务端（或攻击者控制的网关）把 access-token 反射进 Location 的 host 部分——
  // 错误对象绝不能从响应里派生任何内容，只应报告 HTTP status。
  pool.intercept({path: '/user/v1/me', method: 'GET'}).reply(302, '', {
    headers: {location: `https://${secretToken}.evil.example.com/steal`},
  });

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() =>
    client.user.me({accessToken: secretToken}),
  );

  assert.ok(err instanceof QdmpTransportError);
  const transportErr = err as QdmpTransportError;
  const rendered = `${transportErr.message ?? ''} ${String(transportErr)}`;
  assert.ok(
    !rendered.includes(secretToken),
    `QdmpTransportError 不应包含 Location 派生的任何内容（含反射进 host 的 token），实际: ${rendered}`,
  );
  assert.ok(
    !rendered.toLowerCase().includes('location'),
    `QdmpTransportError 不应包含任何 Location 相关内容，实际: ${rendered}`,
  );
});
