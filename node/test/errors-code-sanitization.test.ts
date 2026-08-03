/**
 * QdmpApiError code 字段安全处理测试。
 *
 * 需求：`QdmpErrorCode` 是有意开放的 `string | number` union（见
 * src/errors.ts 文件头注释），服务端可以返回任意字符串作为 code（例如网关层
 * 小整数、未文档化的 code），因此 code 和 message 一样，都是"服务端可控、会
 * 被原样拼进 Error.message"的字段。message 已经过 sanitizeForErrorMessage()
 * 处理（截断首个 C0/DEL 控制字符，并限制最大长度 500），但 code 没有——
 * QdmpApiError 构造函数（src/errors.ts 约66-69行）里 `init.code` 被直接
 * 拼进 `super(...)` 模板字符串，未经任何处理。因此服务端可以在一个
 * STRING 类型的 code 字段里嵌入 \r\n（例如 "1\r\nFAKE injected"），这段内容
 * 会原样出现在 QdmpApiError.message / toString() 里，一旦调用方把
 * error.message 写进单行日志，就会伪造出看似独立的新日志行（日志注入）。
 *
 * 现状（本测试写下时）：QdmpApiError 构造函数只对 init.message 调用
 * sanitizeForErrorMessage()，对 init.code 完全不做任何处理。因此下面的测试
 * 预期是失败的（红色状态）：error.message / toString() 中会原样包含 code
 * 里注入的 \r 字符。
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

void test('QdmpApiError: code 字段中嵌入的 \\r\\n 不得原样出现在 error.message / toString() 中（防日志注入）', async () => {
  const injectedCode = '1\r\nFAKE injected';
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessFailure(injectedCode, 'ok'));

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() => client.auth.getAppAccessToken());

  assert.ok(
    err instanceof QdmpApiError,
    `应当 reject QdmpApiError，实际抛出: ${String(err)}`,
  );
  const apiErr = err as QdmpApiError;
  const rendered = `${apiErr.message}\n${apiErr.toString()}`;
  assert.ok(
    !rendered.includes('\r'),
    `QdmpApiError 的 message/toString() 不应包含 code 字段中原始的 \\r 字符，实际: ${JSON.stringify(rendered)}`,
  );
  assert.ok(
    !rendered.includes('FAKE injected') || !rendered.includes('\n'),
    'QdmpApiError 不应让服务端 code 字段里的换行符原样保留，从而伪造出独立的新日志行',
  );

  // 回归测试（codex 收敛检查轮发现）：构造函数只在拼 message 时对 code
  // 做了清洗，`this.code`/`toJSON().code` 这两个调用方可能直接读取/序列化的
  // 字段之前存的是未清洗的 init.code 原始值，同样会造成日志注入。
  const rawFieldsRendered = `${String(apiErr.code)}\n${JSON.stringify(apiErr.toJSON().code)}`;
  assert.ok(
    !rawFieldsRendered.includes('\r'),
    `QdmpApiError.code / toJSON().code 不应包含原始的 \\r 字符，实际: ${JSON.stringify(rawFieldsRendered)}`,
  );
});

void test('QdmpApiError: requestId 字段中嵌入的 \\r\\n 不得原样出现在 .requestId / toJSON() 中（防日志注入，调用方可能直接读字段而不经过 message）', async () => {
  const injectedRequestId = 'req-1\r\nFAKE injected';
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessFailure('10003', 'bad code', {
      requestId: injectedRequestId,
    }),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() => client.auth.getAppAccessToken());

  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.ok(
    apiErr.requestId === undefined || !apiErr.requestId.includes('\r'),
    `QdmpApiError.requestId 不应包含原始的 \\r 字符，实际: ${JSON.stringify(apiErr.requestId)}`,
  );
  const jsonRequestId = apiErr.toJSON().requestId;
  assert.ok(
    typeof jsonRequestId !== 'string' || !jsonRequestId.includes('\r'),
    'toJSON() 里的 requestId 同样不应包含原始的 \\r 字符',
  );
});

void test('HttpClient.request: 响应体的 code 字段是数组/对象（既不是 string 也不是 number）时应 reject，而不是把整个数组/对象无界地拼进 error message', async () => {
  const {mockAgent, pool} = createMockAgent();
  // `body.code as string | number` 曾经只是 TS 的类型断言，不是运行时校验：
  // 服务端返回一个数组形态的 code，会绕过 QdmpApiError 构造函数里"只在
  // typeof === 'string' 时才清洗"的判断，把 Array.prototype.toString() 的
  // 输出原样、无长度上限地拼进 message/.code/.toJSON()。
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(200, {
    code: Array.from({length: 100}, (_, i) => `malformed-code-part-${i}`),
    message: 'ok',
  });

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() => client.auth.getAppAccessToken());
  assert.ok(
    err instanceof QdmpApiError,
    `应当 reject QdmpApiError，实际抛出: ${String(err)}`,
  );
  const apiErr = err as QdmpApiError;
  assert.ok(
    apiErr.message.length < 1000,
    `code 字段是数组时，QdmpApiError.message 长度应当有界，实际长度: ${apiErr.message.length}`,
  );
  assert.ok(
    typeof apiErr.code === 'string' || typeof apiErr.code === 'number',
    `QdmpApiError.code 应当是 string 或 number（对畸形 code 归一化为一个安全的哨兵值），实际: ${JSON.stringify(apiErr.code)}`,
  );
});
