/**
 * QdmpApiError message/requestId 安全处理测试。
 *
 * 需求：服务端返回的业务失败信封里的 message 字段会被 QdmpApiError 原样拼进
 * Error.message（见 src/errors.ts 约32-35行的 `super(...)` 调用），如果服务端
 * （或中间代理）返回的 message 里嵌入了 \r\n 等控制字符，一旦调用方把
 * error.message 原样写进单行日志，就可能伪造出看似独立的新日志行（日志注入）。
 * 同理，一个异常超长的 message 会被原样塞进 Error.message，缺乏长度上限。
 *
 * 因此新要求：
 *   1. QdmpApiError.message（含 toString()）中不得包含服务端 message 原始的
 *      \r / \n 字符；
 *   2. QdmpApiError.message 的长度必须有上限（不是无限透传服务端的任意长度
 *      字符串）。
 *
 * 现状（本测试写下时）：QdmpApiError 构造函数把 init.message 未经任何转义/
 * 截断地拼进 `super(...)`。因此下面两个测试预期都是失败的（红色状态）。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {QdmpApiError} from '../src/errors.js';
import {
  businessFailure,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

function makeClient(
  mockAgent: ReturnType<typeof createMockAgent>['mockAgent'],
) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('QdmpApiError: message 中嵌入的 \\r\\n 不得原样出现在 error.message / toString() 中（防日志注入）', async () => {
  const injectedMessage = 'line1\r\nFAKE injected log line';
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessFailure('40001', injectedMessage));

  const client = makeClient(mockAgent);
  const err = await expectFailure(() => client.auth.getAccessToken());

  assert.ok(
    err instanceof QdmpApiError,
    `应当 reject QdmpApiError，实际抛出: ${String(err)}`,
  );
  const apiErr = err as QdmpApiError;
  const rendered = `${apiErr.message}\n${apiErr.toString()}`;
  assert.ok(
    !rendered.includes('\r'),
    `QdmpApiError 的 message/toString() 不应包含原始 \\r 字符，实际: ${JSON.stringify(rendered)}`,
  );
  assert.ok(
    !rendered.includes('FAKE injected log line') || !rendered.includes('\n'),
    'QdmpApiError 不应让服务端 message 里的换行符原样保留，从而伪造出独立的新日志行',
  );
});

void test('QdmpApiError: 服务端返回超长 message（约20万字符）时，error.message 的长度必须有界（不能无限透传）', async () => {
  const hugeMessage = 'A'.repeat(200_000);
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessFailure('40002', hugeMessage));

  const client = makeClient(mockAgent);
  const err = await expectFailure(() => client.auth.getAccessToken());

  assert.ok(
    err instanceof QdmpApiError,
    `应当 reject QdmpApiError，实际抛出: ${String(err)}`,
  );
  const apiErr = err as QdmpApiError;
  assert.ok(
    apiErr.message.length < 5000,
    `QdmpApiError.message 长度应当有界（预期远小于原始 20 万字符），实际长度: ${apiErr.message.length}`,
  );
});
