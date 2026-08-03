/**
 * auth 模块测试：app 级 token 缓存 + 单飞刷新 + 300 秒缓冲提前刷新 + getUserAccessToken +
 * refreshToken 一次性换取。均依据 shared/openapi.yaml 的 authToken/authRefresh
 * operation 的 token 生命周期模型。
 *
 * 覆盖点：
 * - 正常成功路径（CLIENT_CREDENTIALS）
 * - app 级 token 并发单飞（只触发一次真实 HTTP 换取）
 * - 300 秒缓冲提前刷新（不是等到真正过期才刷新）
 * - HTTP 200 + code !== '0' 判定为失败（不能只信 HTTP 状态码）
 * - getUserAccessToken（AUTHORIZATION_CODE）不进入 app 级缓存
 * - refreshToken 一次性刷新，HTTP 200 + code:10008（refreshToken 超期）判定为失败
 * - 错误信息不泄露 appSecret
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {QdmpApiError} from '../src/errors.js';
import type {StoredToken, TokenStore} from '../src/token-store.js';
import {
  businessFailure,
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

/**
 * Simulates an eventually-consistent remote TokenStore (e.g. Redis with
 * replication lag): `set()` resolves immediately but the value it wrote only
 * becomes visible to `get()` after `visibilityDelayMs`. This reproduces the
 * exact race the "回读 TokenStore" fix targets — a caller landing right
 * after another caller's in-flight refresh finished must not still see a
 * stale/missing value and trigger a redundant CLIENT_CREDENTIALS exchange.
 */
class EventuallyConsistentTokenStore implements TokenStore {
  getCalls = 0;
  private trueValue: StoredToken | undefined;
  private visibleValue: StoredToken | undefined;

  constructor(private readonly visibilityDelayMs: number) {}

  get(): Promise<StoredToken | undefined> {
    this.getCalls++;
    return Promise.resolve(this.visibleValue);
  }

  set(token: StoredToken): Promise<void> {
    this.trueValue = token;
    setTimeout(() => {
      this.visibleValue = this.trueValue;
    }, this.visibilityDelayMs);
    return Promise.resolve();
  }

  clear(): void {
    this.trueValue = undefined;
    this.visibleValue = undefined;
  }
}

/**
 * Reproduces the concurrent single-flight bypass (see code review finding):
 * `get()` returns a snapshot of the store's value *taken at call time*, but
 * only resolves once `releaseNextGet()` is invoked — modeling a slow/lagging
 * remote read whose result reflects the store's state from when the read was
 * dispatched, not from when it happens to settle. This lets a test
 * deterministically arrange "caller B's tokenStore.get() resolves stale,
 * after caller A's in-flight refresh has already completed and cleared
 * `inFlight`" without relying on real timers.
 */
class ManualGetTokenStore implements TokenStore {
  private value: StoredToken | undefined;
  private pendingResolvers: Array<() => void> = [];

  get(): Promise<StoredToken | undefined> {
    const snapshot = this.value;
    return new Promise(resolve => {
      this.pendingResolvers.push(() => resolve(snapshot));
    });
  }

  /** Settles the oldest still-pending get() call with the value snapshot
   * taken at the time *that* get() call was made. */
  releaseNextGet(): void {
    const resolver = this.pendingResolvers.shift();
    if (!resolver) {
      throw new Error('releaseNextGet(): no pending get() call to release');
    }
    resolver();
  }

  set(token: StoredToken): Promise<void> {
    this.value = token;
    return Promise.resolve();
  }

  clear(): void {
    this.value = undefined;
  }
}

function tokenSuccessData(overrides: Record<string, unknown> = {}) {
  return {
    accessToken: 'app-token-abc',
    expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
    ...overrides,
  };
}

void test('auth.getAppAccessToken: 正常路径用 CLIENT_CREDENTIALS 换取 app 级 token', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  pool
    .intercept({
      path: '/auth/v1/token',
      method: 'POST',
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
    })
    .reply(200, businessSuccess(tokenSuccessData()));

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const token = await client.auth.getAppAccessToken();

  assert.equal(token.accessToken, 'app-token-abc');
  assert.deepEqual(capturedBody, {
    grantType: 'CLIENT_CREDENTIALS',
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
  });
});

void test('auth.getAppAccessToken: 并发调用只触发一次真实换取请求（单飞去重）', async () => {
  const {mockAgent, pool} = createMockAgent();
  // 只注册一次拦截器：若实现没有做单飞去重，第二/三次并发请求会找不到匹配的
  // 拦截器而报错（因为 disableNetConnect），从而暴露问题。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(tokenSuccessData()));

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const [t1, t2, t3] = await Promise.all([
    client.auth.getAppAccessToken(),
    client.auth.getAppAccessToken(),
    client.auth.getAppAccessToken(),
  ]);

  assert.equal(t1.accessToken, 'app-token-abc');
  assert.equal(t2.accessToken, 'app-token-abc');
  assert.equal(t3.accessToken, 'app-token-abc');
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '唯一一个拦截器应当被消费且仅消费一次，证明三次并发调用真的只发起了一次 HTTP 请求',
  );
});

void test('auth.getAppAccessToken: 缓存命中时不重复发起 HTTP 请求，且缓存命中路径返回的应用凭证与新换取路径同样完整', async () => {
  const {mockAgent, pool} = createMockAgent();
  // 按实测事实构造：CLIENT_CREDENTIALS 响应同样带 refreshToken（真实非空值）
  // 和 openId（空字符串）。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(
      200,
      businessSuccess(
        tokenSuccessData({refreshToken: 'app-refresh-abc', openId: ''}),
      ),
    );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const first = await client.auth.getAppAccessToken();
  // 没有再注册第二个拦截器：如果实现没有缓存、又发起了第二次 HTTP 请求，
  // disableNetConnect 会让该请求报错，下面这行就会抛出。
  const second = await client.auth.getAppAccessToken();

  assert.equal(first.accessToken, 'app-token-abc');
  assert.equal(second.accessToken, 'app-token-abc');
  assert.equal(first.refreshToken, 'app-refresh-abc');
  assert.equal(
    first.openId,
    '',
    '应用凭证的 openId 实测就是空字符串，不应被当作缺失字段报错',
  );
  assert.deepEqual(
    {...second},
    {...first},
    '缓存命中那一次返回的凭证必须与刚换取那一次逐字段相同，不能少 refreshToken/openId',
  );
});

void test('auth.getAppAccessToken: 距真正过期还有 300 秒缓冲区间内即主动刷新（不是等到真正过期）', async t => {
  t.mock.timers.enable({apis: ['Date']});
  const nowSeconds = Math.floor(Date.now() / 1000);

  const {mockAgent, pool} = createMockAgent();
  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess(
      tokenSuccessData({
        accessToken: 'token-1',
        expiresAt: String(nowSeconds + 400),
      }),
    ),
  );

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const first = await client.auth.getAppAccessToken();
  assert.equal(first.accessToken, 'token-1');

  // 时间前进 150 秒：距真正过期（400 秒后）还剩 250 秒，落在 300 秒缓冲区间内，
  // 理应主动换新，而不是复用这个"技术上还没过期但快过期"的旧 token。
  t.mock.timers.tick(150 * 1000);

  pool.intercept({path: '/auth/v1/token', method: 'POST'}).reply(
    200,
    businessSuccess(
      tokenSuccessData({
        accessToken: 'token-2',
        expiresAt: String(nowSeconds + 150 + 7200),
      }),
    ),
  );

  const second = await client.auth.getAppAccessToken();

  assert.equal(
    second.accessToken,
    'token-2',
    '进入 300 秒缓冲期后应当换新 token，而不是继续返回 token-1',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '第二次刷新的拦截器应当被真实消费',
  );
});

void test('auth.getAppAccessToken: 异步 TokenStore 存在"回读延迟"时，紧随其后的调用命中进程内缓存而不重复换取', async () => {
  const {mockAgent, pool} = createMockAgent();
  // 只注册一个拦截器：若第二次调用没有命中本地缓存、退化为重新调用
  // tokenStore.get()（此刻仍返回旧值/undefined）再触发 refreshAppToken()，
  // disableNetConnect 会让第二次真实 HTTP 换取因找不到拦截器而报错。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(tokenSuccessData({accessToken: 'token-1'})));

  const tokenStore = new EventuallyConsistentTokenStore(50);
  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
    tokenStore,
  });

  const first = await client.auth.getAppAccessToken();
  assert.equal(first.accessToken, 'token-1');
  assert.equal(
    tokenStore.getCalls,
    1,
    '首次调用应当命中一次 tokenStore.get()（缓存未命中，触发真实换取）',
  );

  // 紧接着再次调用：此时 tokenStore 的 visibleValue 仍是回读延迟窗口内的旧值
  // （setTimeout(50ms) 还没触发），只有本地 cachedToken 能证明拿到了新 token。
  const second = await client.auth.getAppAccessToken();
  assert.equal(second.accessToken, 'token-1');
  assert.equal(
    tokenStore.getCalls,
    1,
    '第二次调用应命中进程内 cachedToken 快速路径，不应再调用 tokenStore.get()，也不应触发第二次 HTTP 换取',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '唯一的拦截器应当只被消费一次',
  );
});

void test('auth.getAppAccessToken: 并发调用者的 tokenStore.get() 在另一调用者的单飞刷新已完成之后才 resolve（且值仍是刷新前的旧快照）时，不应触发第二次冗余换取', async () => {
  const {mockAgent, pool} = createMockAgent();
  // 只注册一个拦截器：若实现在这种交错时序下仍触发第二次 CLIENT_CREDENTIALS
  // 换取，disableNetConnect 会让第二次请求找不到拦截器而报错，从而暴露问题。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessSuccess(tokenSuccessData({accessToken: 'token-1'})));

  const tokenStore = new ManualGetTokenStore();
  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
    tokenStore,
  });

  // 两个调用"同时"发起：两者都在各自看到 this.cachedToken 为空后，
  // 各自调用了一次 tokenStore.get()（此刻都拿到了「无 token」的快照，都还未
  // resolve）。
  const callA = client.auth.getAppAccessToken();
  const callB = client.auth.getAppAccessToken();

  // 放行 A 的 get()：A 发现无缓存，进入单飞刷新，真实发起（唯一注册的）HTTP
  // 换取请求，成功后设置 this.cachedToken 并清空 inFlight。
  tokenStore.releaseNextGet();
  const resultA = await callA;
  assert.equal(resultA.accessToken, 'token-1');

  // 放行 B 的 get()：resolve 的是 B 发起 get() 那一刻捕获的快照（当时仍是
  // 「无 token」），复现"回读结果滞后于另一并发调用者已完成的刷新"这一时序。
  // 此时 inFlight 已被 A 清空，如果 refreshAppToken() 不重新检查
  // this.cachedToken 的新鲜度，就会误判为需要发起第二次换取。
  tokenStore.releaseNextGet();
  const resultB = await callB;

  assert.equal(
    resultB.accessToken,
    'token-1',
    'B 应当复用 A 已经写入的进程内缓存 token，而不是触发第二次冗余换取',
  );
  assert.equal(
    mockAgent.pendingInterceptors().length,
    0,
    '唯一的拦截器应当只被消费一次，证明只发生了一次真实 HTTP 换取',
  );
});

void test('auth.getAppAccessToken: HTTP 200 但业务 code !== "0" 判定为失败，不能只信 HTTP 状态码', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessFailure(10001, 'appId 无效'));

  const client = new QdmpClient({
    appId: 'bad-app-id',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() => client.auth.getAppAccessToken());
  assert.ok(
    err instanceof QdmpApiError,
    `应当抛出 QdmpApiError，实际抛出: ${err}`,
  );
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '10001');
  assert.equal(apiErr.httpStatus, 200);
});

void test('auth.getAppAccessToken: 换取失败时错误信息不泄露 appSecret 原文', async () => {
  const secretMarker = 'DO-NOT-LEAK-app-secret-zzz999';
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(200, businessFailure(10002, '密钥不匹配'));

  const client = new QdmpClient({
    appId: 'app-x',
    appSecret: secretMarker,
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() => client.auth.getAppAccessToken());
  const rendered = `${(err as Error).message ?? ''} ${String(err)} ${safeStringify(err)}`;
  assert.ok(
    !rendered.includes(secretMarker),
    `错误信息中不应包含 appSecret 原文，实际: ${rendered}`,
  );
});

void test('auth.getUserAccessToken: AUTHORIZATION_CODE 正常换取用户级 token，且不进入 app 级缓存', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  pool
    .intercept({
      path: '/auth/v1/token',
      method: 'POST',
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        accessToken: 'user-token-xyz',
        refreshToken: 'refresh-xyz',
        expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
        openId: 'openid-abc',
      }),
    );

  const client = new QdmpClient({
    appId: 'app-1',
    appSecret: 'secret-1',
    dispatcher: mockAgent,
  });

  const userCredential = await client.auth.getUserAccessToken('login-code-123');

  assert.equal(userCredential.accessToken, 'user-token-xyz');
  assert.equal(userCredential.refreshToken, 'refresh-xyz');
  assert.equal(userCredential.openId, 'openid-abc');
  assert.equal(typeof userCredential.expiresAt, 'string');
  assert.deepEqual(capturedBody, {
    grantType: 'AUTHORIZATION_CODE',
    appId: 'app-1',
    appSecret: 'secret-1',
    code: 'login-code-123',
  });

  // 换取 app 级 token 时必须仍然真实发起一次独立的 CLIENT_CREDENTIALS 请求，
  // 证明 getUserAccessToken 拿到的用户级 token 没有被误当成 app 级 token 缓存复用。
  pool
    .intercept({path: '/auth/v1/token', method: 'POST'})
    .reply(
      200,
      businessSuccess(tokenSuccessData({accessToken: 'app-token-fresh'})),
    );
  const appToken = await client.auth.getAppAccessToken();
  assert.equal(appToken.accessToken, 'app-token-fresh');
});

void test('auth.refreshToken: 正常路径换取新 accessToken（一次性，不缓存）', async () => {
  const {mockAgent, pool} = createMockAgent();
  let capturedBody: unknown;
  pool
    .intercept({
      path: '/auth/v1/refresh',
      method: 'POST',
      body: raw => {
        capturedBody = JSON.parse(raw as string);
        return true;
      },
    })
    .reply(
      200,
      businessSuccess({
        accessToken: 'refreshed-token',
        expiresAt: String(Math.floor(Date.now() / 1000) + 7200),
      }),
    );

  const client = new QdmpClient({
    appId: 'app-1',
    appSecret: 'secret-1',
    dispatcher: mockAgent,
  });

  const result = await client.auth.refreshToken('some-refresh-token');

  assert.equal(result.accessToken, 'refreshed-token');
  assert.deepEqual(capturedBody, {refreshToken: 'some-refresh-token'});
});

void test('auth.refreshToken: 空字符串 refreshToken 本地直接报错，不发起任何 HTTP 请求（与 getUserAccessToken 对 code 的校验保持一致）', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(200, businessSuccess({accessToken: 'should-not-be-reached'}));

  const client = new QdmpClient({
    appId: 'app-1',
    appSecret: 'secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() => client.auth.refreshToken(''));

  assert.equal(
    mockAgent.pendingInterceptors().length,
    1,
    '空字符串 refreshToken 时不应真的发出 HTTP 请求，拦截器必须仍处于未消费状态',
  );
});

void test('auth.refreshToken: HTTP 200 + code=10008（refreshToken 超期）必须判定为失败', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/auth/v1/refresh', method: 'POST'})
    .reply(200, businessFailure(10008, 'refreshToken 超过有效期'));

  const client = new QdmpClient({
    appId: 'app-1',
    appSecret: 'secret-1',
    dispatcher: mockAgent,
  });

  const err = await expectFailure(() =>
    client.auth.refreshToken('expired-refresh-token'),
  );
  assert.ok(err instanceof QdmpApiError);
  const apiErr = err as QdmpApiError;
  assert.equal(String(apiErr.code), '10008');
  assert.equal(
    apiErr.httpStatus,
    200,
    'HTTP 层面是 200，必须靠 code 字段判断失败，不能靠状态码',
  );
});

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value);
  } catch {
    return '[unstringifiable]';
  }
}
