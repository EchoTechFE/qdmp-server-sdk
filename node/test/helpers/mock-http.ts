/**
 * 测试专用 HTTP mock 工具，基于 undici 的 MockAgent。
 *
 * `QdmpClient` 内部发请求用 Node.js 内置全局 `fetch`（不是从 `undici` 包导入的
 * `fetch`），显式传入 `{ dispatcher: mockAgent }` 即可正确命中下面的拦截器
 * ——已在本机 Node.js v22.11.0 + undici(npm) 6.28.0 组合下用最小复现脚本重新实测
 * 确认（全局 fetch 会遵守显式传入的 `dispatcher`，也会遵守 `setGlobalDispatcher()`）。
 * `undici` 仅作为 devDependency 提供 `MockAgent`/`Dispatcher` 类型，不再是运行时依赖。
 */
import {MockAgent} from 'undici';

/** 与 shared/openapi.yaml servers[0].url 保持一致的生产环境 origin。 */
export const QDMP_PROD_ORIGIN = 'https://openapi.qiandao.com';

export interface MockSetup {
  mockAgent: MockAgent;
  pool: ReturnType<MockAgent['get']>;
}

/** 创建一个禁止真实联网、指向千岛生产 origin 的 MockAgent + MockPool。 */
export function createMockAgent(origin = QDMP_PROD_ORIGIN): MockSetup {
  const mockAgent = new MockAgent();
  mockAgent.disableNetConnect();
  const pool = mockAgent.get(origin);
  return {mockAgent, pool};
}

/** 正常业务信封：{code:'0', message, requestId, data}。 */
export function businessSuccess(
  data: Record<string, unknown>,
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    code: '0',
    message: 'ok',
    requestId: `req-mock-${Math.random().toString(36).slice(2)}`,
    data,
    ...overrides,
  };
}

/** 业务失败信封：{code, message, requestId}（无 data，或 data 为空）。 */
export function businessFailure(
  code: string | number,
  message: string,
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    code,
    message,
    requestId: `req-mock-err-${Math.random().toString(36).slice(2)}`,
    ...overrides,
  };
}

/** 网关层错误信封：{code, message, details}（无 data / requestId）。 */
export function gatewayFailure(
  code: number,
  message: string,
  details: unknown = null,
): Record<string, unknown> {
  return {code, message, details};
}

/**
 * 用 `.reply()` 的回调形式来数"真的发出了几次请求"、顺便抓请求头。
 *
 * 不能用 `intercept({headers: h => ...})` 的匹配函数来做这件事：undici 对同一次
 * 请求会多次调用它——挑选候选拦截器时每个未消费的候选各调一次
 * （mock-utils.js 的 getMockDispatch()），回收已消费的拦截器时再调一次
 * （同文件 deleteMockDispatch()）。而 `.reply(opts => ...)` 的回调在
 * mockDispatch() 里严格每次真实 dispatch 只跑一次，是唯一可靠的请求计数点。
 */
export function replyWith(
  statusCode: number,
  data: Record<string, unknown>,
  onRequest?: (headers: Record<string, string>) => void,
): (opts: {headers?: Record<string, string> | Headers}) => {
  statusCode: number;
  data: Record<string, unknown>;
} {
  return opts => {
    onRequest?.(toHeaderRecord(opts.headers));
    return {statusCode, data};
  };
}

function toHeaderRecord(
  headers: Record<string, string> | Headers | undefined,
): Record<string, string> {
  if (!headers) {
    return {};
  }
  if (headers instanceof Headers) {
    return Object.fromEntries(headers.entries());
  }
  return headers;
}

/**
 * 统一捕获"函数应当失败"这件事，同时兼容同步 throw 和异步 reject 两种实现风格
 * ——`assert.rejects()` 只在传入函数返回 rejected promise 时才生效，若实现是同步
 * throw（哪怕包在 async 函数里也可能被某些写法绕过 promise 包装），
 * `assert.rejects()` 会让异常直接冒出而不是被捕获，因此这里手写 try/catch。
 */
export async function expectFailure(fn: () => unknown): Promise<unknown> {
  try {
    await fn();
  } catch (err) {
    return err;
  }
  throw new Error('expected the call to throw or reject, but it succeeded');
}
