/**
 * QdmpClient 构造器 baseUrl scheme 校验测试。
 *
 * 需求：new QdmpClient({..., baseUrl})，若 baseUrl 的 scheme 是 http（而非
 * https）且 host 不是 loopback/localhost，构造器必须**同步抛异常**——不需要
 * 等到真正发起网络请求才失败（fail fast，和本项目 Go/Java 版 SDK 的对应设计
 * 保持一致：appSecret/appId 等敏感信息绝不应该有机会被明文发往非 HTTPS
 * origin）。但 127.0.0.1 / localhost（不区分大小写）/ 127. 开头的地址即使是
 * http 也必须放行，因为本地开发/测试环境常见 http://localhost:xxxx 这种
 * mock/自建后端。
 *
 * 现状（本测试写下时）：QdmpClient 构造器（src/client.ts 约41-44行）对
 * options.baseUrl 不做任何 scheme 检查，直接透传给 HttpClient。因此第一个
 * 测试（要求非 loopback 的 http baseUrl 必须抛异常）预期是失败的（红色状态）
 * ——当前实现不会抛。其余测试（loopback/https/默认值都不应抛异常）用于防止
 * 未来实现过度收紧规则，现在应当已经通过。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {QdmpClient} from '../src/client.js';

void test('new QdmpClient: baseUrl 为非 loopback 的 http origin 时必须同步抛异常', () => {
  assert.throws(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://evil.example.com',
      }),
    /./,
    'baseUrl 是 http 且 host 既不是 127.0.0.1 也不是 localhost 时，构造器应当同步抛异常，而不是等到发起请求才失败',
  );
});

void test('new QdmpClient: baseUrl 为 http://127.0.0.1:<port> 时不应抛异常（loopback 例外）', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://127.0.0.1:12345',
      }),
  );
});

void test('new QdmpClient: baseUrl 为 http://localhost:<port> 时不应抛异常（不区分大小写）', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://localhost:12345',
      }),
  );
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://LOCALHOST:12345',
      }),
    'localhost 的大小写不应影响 loopback 例外判定',
  );
});

void test('new QdmpClient: baseUrl 为 https origin 时不应抛异常', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'https://example.com',
      }),
  );
});

void test('new QdmpClient: 不传 baseUrl（使用默认值）时不应抛异常', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
      }),
  );
});

/**
 * 补充测试（安全回归）：isLoopbackHost()（src/client.ts 约21-26行）用
 * `host.startsWith('127.')` 判定 loopback 例外，这是对 hostname 字符串做前缀
 * 匹配，而不是校验它确实是一个点分十进制的 IPv4 字面量。因此域名
 * `127.attacker.com`（不是 IP，是一个可以被攻击者解析到任意远程主机的 DNS
 * 名）会被误判为 loopback，从而放行 http://（明文）请求，appSecret/token 会
 * 被发往攻击者控制的远程主机。
 *
 * 现状（本测试写下时）：下面第一个测试断言构造器必须同步抛异常，预期是失败的
 * （红色状态）——当前实现不会抛，因为 '127.attacker.com'.startsWith('127.')
 * 为 true。其余测试断言真正的 127.x.x.x IPv4 字面量仍然不应抛异常（这是
 * loopback 例外本该覆盖的合法场景），这些现在应当已经通过，用于防止未来修复
 * 过度收紧规则。
 */
void test('new QdmpClient: baseUrl 为 http://127.attacker.com（域名，非 IP 字面量）时必须同步抛异常', () => {
  assert.throws(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://127.attacker.com',
      }),
    /./,
    'hostname 是域名 127.attacker.com（可解析到任意攻击者控制的主机），不是真正的 127.0.0.1 loopback IP 字面量，不应被 startsWith("127.") 误判为 loopback 而放行明文 http',
  );
});

void test('new QdmpClient: baseUrl 为 http://127.5.6.7（真正的 127.x.x.x IPv4 loopback 字面量）时不应抛异常', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://127.5.6.7:12345',
      }),
  );
});

void test('new QdmpClient: baseUrl 为 http://127.0.0.1（无端口）时不应抛异常', () => {
  assert.doesNotThrow(
    () =>
      new QdmpClient({
        appId: 'a',
        appSecret: 's',
        baseUrl: 'http://127.0.0.1',
      }),
  );
});
