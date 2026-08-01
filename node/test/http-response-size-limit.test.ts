/**
 * HttpClient.request() 响应体大小上限测试。
 *
 * 需求：一个恶意或异常的服务端/中间代理返回一个体积极大的响应体时，
 * HttpClient.request() 不应无限制地把整个 body 读进内存并 JSON.parse——
 * 这会给调用方进程带来内存耗尽（OOM）风险。响应体大小应当有一个合理的上限
 * （预期实现量级在 10MB 左右），超过上限时应当 reject，而不是成功解析出一个
 * 巨大的对象。
 *
 * 本测试用一个约 11MB 的字符串字段构造响应体，安全超出预期的 10MB 量级，
 * 同时又不至于让测试本身跑得很慢/占用过多内存。
 *
 * 现状（本测试写下时）：HttpClient.request()（src/http.ts 约149-158行）对
 * `response.json()` 的响应体大小没有任何限制，因此这个测试预期是失败的
 * （红色状态）：当前实现会成功 resolve 出这个 11MB 的巨大对象，而不是 reject。
 */
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from './helpers/mock-http.js';

void test('HttpClient.request: 响应体约 11MB 时应 reject，而不是成功解析出巨大的 JSON 对象', async () => {
  const oversizedValue = 'x'.repeat(11 * 1024 * 1024);
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/user/v1/me', method: 'GET'})
    .reply(200, businessSuccess({id: oversizedValue, nickname: 'oversized'}));

  const client = new QdmpClient({
    appId: 'app-id-1',
    appSecret: 'app-secret-1',
    dispatcher: mockAgent,
  });

  await expectFailure(() =>
    client.user.me({accessToken: 'valid-looking-access-token'}),
  );
});
