/**
 * 回归测试：code2Session()/refreshToken() 返回值的调试打印脱敏。
 *
 * 背景（Fable5 设计一致性审查发现的真实安全问题）：code2Session()/
 * refreshToken() 返回的普通 object literal（类型见 src/types.ts 的
 * SessionData/RefreshTokenData）一旦被 console.log(session)/util.inspect(session)
 * 这类"意外调试打印"路径打印，就会把 accessToken/refreshToken 明文暴露出去。
 *
 * 修复（src/auth.ts 的 withRedactedInspect()）给返回对象挂了一个不可枚举的
 * [util.inspect.custom] 方法，只影响 util.inspect()/console.log() 这条路径。
 *
 * 本测试同时覆盖两个方向：
 * 1. util.inspect(session) 拿到的字符串表示不能包含真实的 accessToken/refreshToken。
 * 2. 反向验证：JSON.stringify(session) 和直接属性访问（session.accessToken /
 *    session.refreshToken）必须仍然是原始明文值——因为 code2Session()/
 *    refreshToken() 存在的整个目的就是让调用方把这些真实值持久化到自己的
 *    数据库/session store，如果连 JSON.stringify 都被脱敏了 SDK 就没法用了。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';
import {inspect} from 'node:util';

import {QdmpClient} from '../src/client.js';
import {businessSuccess, createMockAgent} from './helpers/mock-http.js';

const SENTINEL_ACCESS_TOKEN = 'sentinel-access-token-do-not-leak-xyz123';
const SENTINEL_REFRESH_TOKEN = 'sentinel-refresh-token-do-not-leak-xyz123';

function makeClient(
  mockAgent: ReturnType<typeof createMockAgent>['mockAgent'],
) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

void test('auth.code2Session: util.inspect()/console.log() 路径不得暴露 accessToken/refreshToken 明文', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: SENTINEL_ACCESS_TOKEN,
      refreshToken: SENTINEL_REFRESH_TOKEN,
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
      openId: 'open-id-1',
    }),
  );

  const client = makeClient(mockAgent);
  const session = await client.auth.code2Session('some-auth-code');

  const rendered = inspect(session);
  assert.ok(
    !rendered.includes(SENTINEL_ACCESS_TOKEN),
    `util.inspect(session) leaked the raw accessToken: ${rendered}`,
  );
  assert.ok(
    !rendered.includes(SENTINEL_REFRESH_TOKEN),
    `util.inspect(session) leaked the raw refreshToken: ${rendered}`,
  );
  assert.ok(
    rendered.includes('[REDACTED]'),
    `util.inspect(session) should contain the [REDACTED] placeholder: ${rendered}`,
  );

  // Reverse-direction guard: JSON.stringify (how the caller persists the
  // session) and direct property access must both stay real plaintext.
  const json = JSON.stringify(session);
  assert.ok(
    json.includes(SENTINEL_ACCESS_TOKEN),
    `JSON.stringify(session) must still contain the real accessToken for the caller to persist: ${json}`,
  );
  assert.ok(
    json.includes(SENTINEL_REFRESH_TOKEN),
    `JSON.stringify(session) must still contain the real refreshToken for the caller to persist: ${json}`,
  );
  assert.equal(session.accessToken, SENTINEL_ACCESS_TOKEN);
  assert.equal(session.refreshToken, SENTINEL_REFRESH_TOKEN);
});

void test('auth.refreshToken: util.inspect()/console.log() 路径不得暴露 accessToken 明文', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: SENTINEL_ACCESS_TOKEN,
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    }),
  );

  const client = makeClient(mockAgent);
  const result = await client.auth.refreshToken('some-refresh-token');

  const rendered = inspect(result);
  assert.ok(
    !rendered.includes(SENTINEL_ACCESS_TOKEN),
    `util.inspect(result) leaked the raw accessToken: ${rendered}`,
  );
  assert.ok(
    rendered.includes('[REDACTED]'),
    `util.inspect(result) should contain the [REDACTED] placeholder: ${rendered}`,
  );

  const json = JSON.stringify(result);
  assert.ok(
    json.includes(SENTINEL_ACCESS_TOKEN),
    `JSON.stringify(result) must still contain the real accessToken for the caller to persist: ${json}`,
  );
  assert.equal(result.accessToken, SENTINEL_ACCESS_TOKEN);
});

// 回归测试（codex 收敛检查轮发现）：openId 只校验非空字符串，服务端返回一个
// 嵌入 \r\n 的 openId 时，withRedactedInspect() 的渲染函数如果直接把它拼进
// 模板字符串，util.inspect(session) 的结果就会出现原始换行——伪造出看似
// 独立的新日志行，和 message/code/requestId 已经修过的日志注入是同一类问题。
void test('auth.code2Session: openId 里嵌入的 \\r\\n 不得原样出现在 util.inspect() 的调试渲染中（防日志注入）', async () => {
  const injectedOpenId = 'open-id-1\r\nFAKE injected log line';
  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: SENTINEL_ACCESS_TOKEN,
      refreshToken: SENTINEL_REFRESH_TOKEN,
      expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
      openId: injectedOpenId,
    }),
  );

  const client = makeClient(mockAgent);
  const session = await client.auth.code2Session('some-auth-code');

  const rendered = inspect(session);
  assert.ok(
    !rendered.includes('\r'),
    `util.inspect(session) 不应包含 openId 中原始的 \\r 字符，实际: ${JSON.stringify(rendered)}`,
  );
  // 反向验证：JSON.stringify 仍然携带真实的（哪怕含控制字符的）openId，因为
  // 这是调用方持久化 session 的正常路径，不应该被这个调试渲染修复影响到。
  assert.equal(session.openId, injectedOpenId);
});
