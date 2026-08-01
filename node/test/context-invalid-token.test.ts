/**
 * requireAccessToken 本地校验测试：拒绝包含控制字符的 accessToken。
 *
 * 需求（对应 codex review round1 发现）：requireAccessToken（node/src/context.ts）
 * 目前只做 truthy 检查。新增行为契约：当传入的 accessToken 字符串包含控制字符
 * （charCode <= 0x1F 或 === 0x7F，比如 \r、\n）时，必须**同步抛出**
 * QdmpValidationError，且在发起任何网络请求之前就抛出（不需要 MockAgent 介入）。
 *
 * 现状（本测试写下时）：requireAccessToken 只做 `!accessToken` 的 truthy 检查，
 * 不检测控制字符，因此第 1、2 个测试预期是失败的（红色状态）。第 3 个测试
 * （合法 token 不应报错）用于防止未来实现过度收紧规则，现在应当已经通过。
 */
import assert from 'node:assert/strict';
import {test} from 'node:test';

import {requireAccessToken} from '../src/context.js';
import {QdmpValidationError} from '../src/errors.js';

void test('requireAccessToken: accessToken 包含 \\r\\n 控制字符时应同步抛出 QdmpValidationError', () => {
  assert.throws(
    () => requireAccessToken({accessToken: 'sentinel-secret\r\nleak'}),
    (err: unknown) => {
      assert.ok(
        err instanceof QdmpValidationError,
        `应当抛出 QdmpValidationError，实际抛出: ${String(err)}`,
      );
      return true;
    },
  );
});

void test('requireAccessToken: 抛出的 QdmpValidationError.message 不得包含非法 token 的原始内容', () => {
  const secretMarker = 'sentinel-secret\r\nleak';
  let caught: unknown;
  try {
    requireAccessToken({accessToken: secretMarker});
  } catch (err) {
    caught = err;
  }
  assert.ok(
    caught instanceof QdmpValidationError,
    `应当抛出 QdmpValidationError，实际抛出: ${String(caught)}`,
  );
  const message = (caught as QdmpValidationError).message ?? '';
  assert.ok(
    !message.includes(secretMarker),
    `error.message 不应包含非法 token 原文，实际: ${message}`,
  );
});

void test('requireAccessToken: 完全合法（纯 ASCII 可打印字符）的 token 不应抛错', () => {
  const validToken = 'valid-ascii-token-ABC123._~-';
  assert.equal(requireAccessToken({accessToken: validToken}), validToken);
});
