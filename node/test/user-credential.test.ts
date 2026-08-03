/**
 * withUserCredential()：用户授权凭证自动续期测试。
 *
 * 需求见 shared/contract（本次重构新增能力）：`qdmp.withUserCredential(options)`
 * 返回一个绑定了某个用户授权凭证（accessToken+refreshToken[+expiresAt]）的
 * client，业务调用前会在需要时自动续期（提前 300 秒续期 / 401+10005/10006
 * 触发的续期重试），业务方法本身不再需要传 ctx。
 *
 *
 * mock 响应严格按已实测的服务端事实构造：响应字段驼峰、`expiresAt` 是字符串、
 * `code` 是数字 0（本项目 helpers/mock-http.ts 用字符串 `'0'` 表达
 * `String(code) === '0'` 这一唯一成功判据，与既有测试套件保持一致）、
 * `POST /auth/v1/refresh` 成功响应 data 只有 accessToken+expiresAt（不返回新
 * refreshToken）、无效 access-token 是 HTTP 401 + code 10005/10006、无效
 * refreshToken 是 HTTP 200 + code 10008。
 *
 * 覆盖点（对应 CONTRACT.md §2.3 行为契约 1-10 条 + §2.4 + §2.5）：
 * - 本地校验：缺 refreshToken / 空 accessToken / 空 refreshToken 不发任何 HTTP 请求
 * - §2.3.1 提前续期（300 秒缓冲）+ expiresAt 未传时跳过提前续期
 * - §2.3.2 续期请求形状：body={refreshToken}，noAuth（不带 access-token 头）
 * - §2.3.2 refreshToken 续期后保持原值不变
 * - §2.3.3 onRefresh 只收到 {accessToken,expiresAt}，必须被 await，抛错时整个
 *   业务调用失败但已更新的新 token 不回滚
 * - §2.3.4/5 401+10005/10006 触发恰好一次续期 + 恰好一次重试；重试后仍失败则
 *   直接抛错，不再续期
 * - §2.3.6 HTTP 5xx / 网络错误 / 非 401 业务错误 / 401 但 code 不在
 *   {10005,10006} 一律不触发续期
 * - §2.3.7 续期本身失败时原样抛给调用方，不重试业务请求
 * - §2.3.8 并发业务调用时续期只发生一次（单飞）
 * - §2.3.9 credential 是只读快照，外部改它不影响内部状态
 * - §2.3.10 credential 在 util.inspect()/console.log() 路径脱敏
 * - §2.4 业务方法不带 ctx（uc.user.me()、uc.mark.add({spuId})）
 * - §2.5 genai 分组同样自动续期
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';
import {inspect} from 'node:util';
import type {MockAgent} from 'undici';

import {QdmpClient} from '../src/client.js';
import {QdmpApiError, QdmpValidationError} from '../src/errors.js';
import {
  businessFailure,
  businessSuccess,
  createMockAgent,
  expectFailure,
  gatewayFailure,
  replyWith,
} from './helpers/mock-http.js';

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000);
}

function makeRootClient(mockAgent: MockAgent) {
  return new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });
}

// ---------------------------------------------------------------------------
// 本地校验：不发任何 HTTP 请求
// ---------------------------------------------------------------------------

void test('withUserCredential: accessToken 为空字符串时本地报错，不发任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'should-not-be-used',
      expiresAt: String(nowSeconds() + 7200),
    }),
  );

  const qdmp = makeRootClient(mockAgent);

  const err = await expectFailure(async () => {
    const uc = qdmp.withUserCredential({
      accessToken: '',
      refreshToken: 'rt-1',
    });
    await uc.user.me();
  });

  assert.ok(
    err instanceof QdmpValidationError,
    `accessToken 为空字符串时应本地抛出 QdmpValidationError，实际抛出: ${String(err)}`,
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    2,
    '本地校验失败时不应发出任何 HTTP 请求，业务接口和 refresh 接口的拦截器都必须仍未被消费',
  );
});

void test('withUserCredential: 缺少 refreshToken 时本地报错，不发任何 HTTP 请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const qdmp = makeRootClient(mockAgent);

  const err = await expectFailure(async () => {
    // @ts-expect-error 故意不传 refreshToken，验证运行时本地校验
    const uc = qdmp.withUserCredential({accessToken: 'at-1'});
    await uc.user.me();
  });

  assert.ok(
    err instanceof QdmpValidationError,
    `缺少 refreshToken 时应本地抛出 QdmpValidationError，实际抛出: ${String(err)}`,
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '本地校验失败时不应发出任何 HTTP 请求',
  );
});

void test('withUserCredential: refreshToken 为空字符串时本地报错（没有它就无法续期，等同于静态 token）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'should-not-be-reached'}));

  const qdmp = makeRootClient(mockAgent);

  const err = await expectFailure(async () => {
    const uc = qdmp.withUserCredential({
      accessToken: 'at-1',
      refreshToken: '',
    });
    await uc.user.me();
  });

  assert.ok(
    err instanceof QdmpValidationError,
    `refreshToken 为空字符串时应本地抛出 QdmpValidationError，实际抛出: ${String(err)}`,
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '本地校验失败时不应发出任何 HTTP 请求',
  );
});

// ---------------------------------------------------------------------------
// §2.3.1 / §2.3.2：提前续期 + 续期请求本身的形状
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.1/§2.3.2: expiresAt 距过期 <=300 秒时业务调用前先续期，续期请求 body={refreshToken} 且不带 access-token 头', async t => {
  t.mock.timers.enable({apis: ['Date']});
  const nowS = nowSeconds();
  const {mockAgent, pool} = createMockAgent();

  let refreshBody: unknown;
  let refreshHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/auth/v1/refresh',
      method: 'POST',
      body: raw => {
        refreshBody = JSON.parse(raw as string);
        return true;
      },
      headers: h => {
        refreshHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        accessToken: 'at-refreshed-1',
        expiresAt: String(nowS + 7200),
      }),
    );

  let businessHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/user/v1/me',
      method: 'GET',
      headers: h => {
        businessHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(200, businessSuccess({id: 'u-1', nickname: 'Alice'}));

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-stale-000',
    refreshToken: 'rt-stable-000',
    expiresAt: String(nowS + 200), // 距过期只剩 200 秒，落在 300 秒缓冲区内
  });

  const me = await uc.user.me();

  assert.equal(me.id, 'u-1');
  assert.deepEqual(
    refreshBody,
    {refreshToken: 'rt-stable-000'},
    '续期请求 body 必须只有原始 refreshToken 这一个字段',
  );
  assert.equal(
    refreshHeaders?.['access-token'],
    undefined,
    'noAuth 方案的续期请求不应携带 access-token 头',
  );
  assert.equal(
    businessHeaders?.['access-token'],
    'at-refreshed-1',
    '业务请求必须使用续期后的新 accessToken，而不是原来即将过期的旧值',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '续期请求和业务请求的拦截器都应当各被消费恰好一次',
  );
});

void test('withUserCredential §2.3.1: expiresAt 未传时跳过提前续期，首次调用直接发业务请求（refresh 端点 0 次请求）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'u-1'}));

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-000',
    refreshToken: 'rt-000',
    // 不传 expiresAt
  });

  const me = await uc.user.me();

  assert.equal(me.id, 'u-1');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '唯一注册的业务拦截器应当被消费；若实现误触发了续期，会因 refresh 端点没有注册拦截器而在 disableNetConnect 下报错，导致本次调用失败',
  );
});

// ---------------------------------------------------------------------------
// §2.3.2 / §2.3.3：refreshToken 保持不变 + onRefresh 参数/时机
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.2/§2.3.3: 续期后 refreshToken 保持原值不变，onRefresh 只收到 {accessToken,expiresAt}', async t => {
  t.mock.timers.enable({apis: ['Date']});
  const nowS = nowSeconds();
  const {mockAgent, pool} = createMockAgent();

  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'at-refreshed-2',
      expiresAt: String(nowS + 7200),
    }),
  );
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: 'u-1'}));

  const onRefreshCalls: unknown[] = [];
  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-stale',
    refreshToken: 'rt-must-not-change',
    expiresAt: String(nowS + 100),
    onRefresh: async token => {
      onRefreshCalls.push(token);
    },
  });

  await uc.user.me();

  assert.equal(onRefreshCalls.length, 1, 'onRefresh 应当恰好被调用一次');
  assert.deepEqual(
    onRefreshCalls[0],
    {accessToken: 'at-refreshed-2', expiresAt: String(nowS + 7200)},
    'onRefresh 收到的参数必须只有 accessToken/expiresAt，不应包含 refreshToken（服务端 refresh 响应本来就不返回新 refreshToken）',
  );
  assert.equal(
    uc.credential.refreshToken,
    'rt-must-not-change',
    '续期后 refreshToken 必须保持原值不变',
  );
  assert.equal(uc.credential.accessToken, 'at-refreshed-2');
  assert.equal(uc.credential.expiresAt, String(nowS + 7200));
});

void test('withUserCredential §2.3.3: onRefresh 必须被 await，业务请求要等 onRefresh resolve 之后才发出', async t => {
  t.mock.timers.enable({apis: ['Date']});
  const nowS = nowSeconds();
  const {mockAgent, pool} = createMockAgent();

  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'at-refreshed-3',
      expiresAt: String(nowS + 7200),
    }),
  );

  const order: string[] = [];
  let releaseOnRefresh: () => void = () => {};
  const gate = new Promise<void>(resolve => {
    releaseOnRefresh = resolve;
  });

  pool.intercept({path: '/user/v1/me', method: 'GET'}).reply(
    replyWith(200, businessSuccess({id: 'u-1'}), () => {
      order.push('business-request-sent');
    }),
  );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-stale',
    refreshToken: 'rt-1',
    expiresAt: String(nowS + 100),
    onRefresh: async () => {
      order.push('onRefresh-start');
      await gate;
      order.push('onRefresh-end');
    },
  });

  const callPromise = uc.user.me();

  await new Promise(resolve => setTimeout(resolve, 20));
  assert.deepEqual(
    order,
    ['onRefresh-start'],
    '业务请求不应在 onRefresh 完成之前就被发出',
  );

  releaseOnRefresh();
  await callPromise;

  assert.deepEqual(
    order,
    ['onRefresh-start', 'onRefresh-end', 'business-request-sent'],
    'onRefresh 必须被 await，业务请求必须严格排在 onRefresh resolve 之后',
  );
});

void test('withUserCredential §2.3.3: onRefresh 抛错时整个业务调用失败，但内部已更新的新 token 不回滚', async t => {
  t.mock.timers.enable({apis: ['Date']});
  const nowS = nowSeconds();
  const {mockAgent, pool} = createMockAgent();

  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'at-after-onrefresh-throw',
      expiresAt: String(nowS + 7200),
    }),
  );
  // 故意不注册 /user/v1/me 拦截器：onRefresh 失败后业务请求本不应该被发出；
  // 若实现仍然发出业务请求，disableNetConnect 会让它报出与预期不同的错误，
  // 从而暴露"onRefresh 失败后仍继续发业务请求"这个 bug。

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-stale',
    refreshToken: 'rt-1',
    expiresAt: String(nowS + 100),
    onRefresh: () => {
      throw new Error('db write failed');
    },
  });

  const err = await expectFailure(() => uc.user.me());

  assert.ok(
    `${(err as Error)?.message ?? err}`.includes('db write failed'),
    `业务调用应当以 onRefresh 抛出的错误失败，不能被吞掉，实际抛出: ${String(err)}`,
  );
  assert.equal(
    uc.credential.accessToken,
    'at-after-onrefresh-throw',
    '即使 onRefresh 失败，已经换到手的新 accessToken 也不应回滚',
  );
  assert.equal(uc.credential.expiresAt, String(nowS + 7200));
});

// ---------------------------------------------------------------------------
// §2.3.4 / §2.3.5：401 续期重试 + 只重试一次
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.4/§2.3.5: 401+10006 触发恰好一次续期 + 恰好一次重试，用新 token 重试成功', async () => {
  const {mockAgent, pool} = createMockAgent();

  let businessCallCount = 0;
  const capturedTokens: Array<string | undefined> = [];
  const countBusinessCall = (h: Record<string, string>) => {
    businessCallCount++;
    capturedTokens.push(h['access-token']);
  };
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(
      replyWith(
        401,
        businessFailure(10006, 'access_token 超过有效期'),
        countBusinessCall,
      ),
    );

  let refreshCallCount = 0;
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    replyWith(
      200,
      businessSuccess({
        accessToken: 'at-after-401-retry',
        expiresAt: String(nowSeconds() + 7200),
      }),
      () => {
        refreshCallCount++;
      },
    ),
  );

  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(replyWith(200, businessSuccess({id: 'u-1'}), countBusinessCall));

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-invalid',
    refreshToken: 'rt-1',
    // 不传 expiresAt：只靠 401 触发续期
  });

  const me = await uc.user.me();

  assert.equal(me.id, 'u-1');
  assert.equal(refreshCallCount, 1, '401+10006 应当恰好触发一次续期');
  assert.equal(
    businessCallCount,
    2,
    '业务请求应当恰好发生两次：首次失败 + 重试一次',
  );
  assert.equal(capturedTokens[0], 'at-invalid');
  assert.equal(
    capturedTokens[1],
    'at-after-401-retry',
    '重试请求必须使用续期后的新 accessToken',
  );
});

void test('withUserCredential §2.3.5: 重试后仍然失败（同样是 401+10006）时直接抛出该错误，不再触发第二次续期', async () => {
  const {mockAgent, pool} = createMockAgent();

  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(401, businessFailure(10006, 'access_token 超过有效期'));

  let refreshCallCount = 0;
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    replyWith(
      200,
      businessSuccess({
        accessToken: 'at-still-bad',
        expiresAt: String(nowSeconds() + 7200),
      }),
      () => {
        refreshCallCount++;
      },
    ),
  );

  // 重试请求同样失败：若实现在重试失败后又发起了第二次续期，会因为 refresh
  // 端点只注册了一个拦截器（已被上面的续期消费）而在 disableNetConnect 下
  // 报出与预期（10006）不同的错误，从而暴露"重试失败后仍继续续期"的 bug。
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(401, businessFailure(10006, 'access_token 超过有效期（仍然无效）'));

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-invalid',
    refreshToken: 'rt-1',
  });

  const err = await expectFailure(() => uc.user.me());

  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '10006');
  assert.equal(apiErr.httpStatus, 401);
  assert.equal(refreshCallCount, 1, '重试后仍失败时不应再触发第二次续期');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '三个拦截器（首次业务失败、续期、重试业务仍失败）应当各被消费恰好一次，不多不少',
  );
});

// ---------------------------------------------------------------------------
// §2.3.6：不该重试的一律不重试（也不触发续期）
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.6: HTTP 5xx 不触发续期，原样抛出错误', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(500, gatewayFailure(2, 'internal server error'));
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'should-not-be-used',
      expiresAt: String(nowSeconds() + 7200),
    }),
  );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-1',
    refreshToken: 'rt-1',
  });

  const err = await expectFailure(() => uc.user.me());

  assert.ok(err instanceof QdmpApiError);
  assert.equal((err as QdmpApiError).httpStatus, 500);
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    'HTTP 5xx 不应触发续期，refresh 端点的拦截器必须仍未被消费',
  );
});

void test('withUserCredential §2.3.6: 网络/传输错误不触发续期，原样抛出错误', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .replyWithError(new Error('simulated network failure'));
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'should-not-be-used',
      expiresAt: String(nowSeconds() + 7200),
    }),
  );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-1',
    refreshToken: 'rt-1',
  });

  await expectFailure(() => uc.user.me());

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '网络/传输错误不应触发续期，refresh 端点的拦截器必须仍未被消费',
  );
});

void test('withUserCredential §2.3.6: 非 401 的业务错误码不触发续期', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessFailure(10001, 'appId 无效'));
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'should-not-be-used',
      expiresAt: String(nowSeconds() + 7200),
    }),
  );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-1',
    refreshToken: 'rt-1',
  });

  const err = await expectFailure(() => uc.user.me());
  assert.ok(err instanceof QdmpApiError);
  assert.equal(String((err as QdmpApiError).code), '10001');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '非 401 的业务错误不应触发续期',
  );
});

void test('withUserCredential §2.3.6: HTTP 401 但业务 code 不是 10005/10006 时不触发续期', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(401, businessFailure(40001, '未知的 401 语义'));
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    200,
    businessSuccess({
      accessToken: 'should-not-be-used',
      expiresAt: String(nowSeconds() + 7200),
    }),
  );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-1',
    refreshToken: 'rt-1',
  });

  const err = await expectFailure(() => uc.user.me());
  assert.ok(err instanceof QdmpApiError);
  assert.equal(String((err as QdmpApiError).code), '40001');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    'HTTP 401 但 code 不是 10005/10006 时不应触发续期',
  );
});

// ---------------------------------------------------------------------------
// §2.3.7：续期本身失败
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.7: 续期本身失败（refresh 返回 10008）时把该错误原样抛给调用方，且不重试业务请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(401, businessFailure(10006, 'access_token 超过有效期'));
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(200, businessFailure(10008, 'refreshToken 超过有效期'));
  // 故意不注册第二个 /user/v1/me 拦截器：续期失败后不应该发起业务重试；
  // 若实现仍然重试，disableNetConnect 会让它报出与预期（10008）不同的错误。

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-invalid',
    refreshToken: 'rt-expired',
  });

  const err = await expectFailure(() => uc.user.me());

  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '10008');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '首次业务请求和续期请求的拦截器都应各被消费恰好一次，不应发生业务重试',
  );
});

// ---------------------------------------------------------------------------
// §2.3.8：并发单飞
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.8: 并发多个业务调用时续期只发生一次（单飞）', async () => {
  const nowS = nowSeconds();
  const {mockAgent, pool} = createMockAgent();

  let refreshCallCount = 0;
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(
      replyWith(
        200,
        businessSuccess({
          accessToken: 'at-single-flight',
          expiresAt: String(nowS + 7200),
        }),
        () => {
          refreshCallCount++;
        },
      ),
    )
    .delay(20);

  const capturedTokens: Array<string | undefined> = [];
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(
      replyWith(200, businessSuccess({id: 'u-1'}), h => {
        capturedTokens.push(h['access-token']);
      }),
    )
    .persist();

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-stale',
    refreshToken: 'rt-1',
    expiresAt: String(nowS + 100), // 落在续期缓冲区内，三个并发调用都会看到同样的"需要续期"状态
  });

  const [r1, r2, r3] = await Promise.all([
    uc.user.me(),
    uc.user.me(),
    uc.user.me(),
  ]);

  assert.equal(r1.id, 'u-1');
  assert.equal(r2.id, 'u-1');
  assert.equal(r3.id, 'u-1');
  assert.equal(
    refreshCallCount,
    1,
    '三个并发业务调用应当只触发一次真实的续期请求，其余调用应等待同一次续期结果',
  );
  assert.equal(capturedTokens.length, 3);
  for (const token of capturedTokens) {
    assert.equal(
      token,
      'at-single-flight',
      '所有并发调用最终都应使用同一次续期换来的新 accessToken',
    );
  }
});

// ---------------------------------------------------------------------------
// §2.3.9：credential 快照
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.9: credential 返回快照，外部修改它不影响内部状态', async () => {
  const {mockAgent, pool} = createMockAgent();

  let capturedToken: string | undefined;
  pool
    .intercept({
      path: '/user/v1/me',
      method: 'GET',
      headers: h => {
        capturedToken = (h as Record<string, string>)['access-token'];
        return true;
      },
    })
    .reply(200, businessSuccess({id: 'u-1'}))
    .persist();

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-real-000',
    refreshToken: 'rt-real-000',
  });

  await uc.user.me();
  assert.equal(capturedToken, 'at-real-000');

  const snapshot = uc.credential as {
    accessToken: string;
    refreshToken: string;
  };
  snapshot.accessToken = 'tampered-access-token';
  snapshot.refreshToken = 'tampered-refresh-token';

  assert.equal(
    uc.credential.accessToken,
    'at-real-000',
    'credential 必须是快照/不可变副本，外部改它不应污染内部状态',
  );

  await uc.user.me();
  assert.equal(
    capturedToken,
    'at-real-000',
    '篡改快照后再次发起业务调用，仍应使用未被污染的真实内部 accessToken',
  );
});

// ---------------------------------------------------------------------------
// §2.3.10：调试打印路径脱敏
// ---------------------------------------------------------------------------

void test('withUserCredential §2.3.10: uc.credential 的 util.inspect()/console.log() 路径必须脱敏，不暴露 accessToken/refreshToken 明文', () => {
  const SENTINEL_ACCESS = 'sentinel-uc-access-do-not-leak-abc123';
  const SENTINEL_REFRESH = 'sentinel-uc-refresh-do-not-leak-abc123';
  const {mockAgent} = createMockAgent();
  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: SENTINEL_ACCESS,
    refreshToken: SENTINEL_REFRESH,
  });

  const rendered = inspect(uc.credential);
  assert.ok(
    !rendered.includes(SENTINEL_ACCESS),
    `util.inspect(uc.credential) 泄露了 accessToken 明文: ${rendered}`,
  );
  assert.ok(
    !rendered.includes(SENTINEL_REFRESH),
    `util.inspect(uc.credential) 泄露了 refreshToken 明文: ${rendered}`,
  );

  // 反向验证：直接属性访问必须仍是明文，否则调用方无法在进程退出前落库。
  assert.equal(uc.credential.accessToken, SENTINEL_ACCESS);
  assert.equal(uc.credential.refreshToken, SENTINEL_REFRESH);
});

// ---------------------------------------------------------------------------
// §2.4：业务方法不带 ctx
// ---------------------------------------------------------------------------

void test('withUserCredential §2.4: uc.mark.add 业务方法不需要 ctx 参数，只传业务 body', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  let capturedHeaders: Record<string, string> | undefined;
  pool
    .intercept({
      path: '/mark/v1/add',
      method: 'POST',
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
      headers: h => {
        capturedHeaders = h as Record<string, string>;
        return true;
      },
    })
    .reply(200, businessSuccess({id: 'mark-1'}));

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-mark-1',
    refreshToken: 'rt-mark-1',
  });

  const result = await uc.mark.add({spuId: '12345'});

  assert.equal(result.id, 'mark-1');
  assert.deepEqual(capturedBody, {spuId: '12345'});
  assert.equal(
    capturedHeaders?.['access-token'],
    'at-mark-1',
    'uc.mark.add 只传业务 body，accessToken 应当由 withUserCredential 内部自动附加',
  );
});

// ---------------------------------------------------------------------------
// §2.5：genai 分组同样自动续期
// ---------------------------------------------------------------------------

void test('withUserCredential §2.5: genai 分组同样自动续期，401+10006 触发续期后用新 token 重试', async () => {
  const {mockAgent, pool} = createMockAgent();

  let firstAttemptCount = 0;
  pool
    .intercept({path: '/genai/v1/detail', method: 'GET', query: {id: 'task-1'}})
    .reply(
      replyWith(401, businessFailure(10006, 'access_token 超过有效期'), () => {
        firstAttemptCount++;
      }),
    );

  let refreshCallCount = 0;
  pool.intercept({path: '/auth/v1/refresh', method: 'POST'}).reply(
    replyWith(
      200,
      businessSuccess({
        accessToken: 'at-genai-refreshed',
        expiresAt: String(nowSeconds() + 7200),
      }),
      () => {
        refreshCallCount++;
      },
    ),
  );

  let retryHeaders: Record<string, string> | undefined;
  pool
    .intercept({path: '/genai/v1/detail', method: 'GET', query: {id: 'task-1'}})
    .reply(
      replyWith(
        200,
        businessSuccess({id: 'task-1', status: 'COMPLETED'}),
        h => {
          retryHeaders = h;
        },
      ),
    );

  const qdmp = makeRootClient(mockAgent);
  const uc = qdmp.withUserCredential({
    accessToken: 'at-genai-invalid',
    refreshToken: 'rt-genai-1',
  });

  const result = await uc.genai.detail({id: 'task-1'});

  assert.equal(result.id, 'task-1');
  assert.equal(firstAttemptCount, 1);
  assert.equal(
    refreshCallCount,
    1,
    'genai 分组的 401+10006 也应当恰好触发一次续期',
  );
  assert.equal(
    retryHeaders?.['x-openapi-access-token'],
    'at-genai-refreshed',
    'genai 重试请求必须携带续期后的新 accessToken（走 x-openapi-access-token 头）',
  );
  assert.equal(
    retryHeaders?.['access-token'],
    undefined,
    'genai 方案不应携带 standard 方案专用的 access-token 头',
  );
});
