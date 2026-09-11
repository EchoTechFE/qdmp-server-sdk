import assert from 'node:assert/strict';
import {test} from 'node:test';
import type {MockAgent} from 'undici';

import {QdmpClient} from '../../src/client.js';
import {QdmpValidationError} from '../../src/errors.js';
import {
  businessSuccess,
  createMockAgent,
  expectFailure,
} from '../helpers/mock-http.js';

function makeClient(mockAgent: MockAgent): QdmpClient {
  return new QdmpClient({
    appId: 'app-id',
    appSecret: 'app-secret',
    dispatcher: mockAgent,
  });
}

void test('mark.batchAdd、post.detail、comment.create 映射请求并解析响应', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/mark/v1/batch/add',
      method: 'POST',
      body: JSON.stringify({spuIds: ['1']}),
    })
    .reply(200, businessSuccess({result: {1: 'm1'}}));
  pool
    .intercept({path: '/post/v1/detail', method: 'GET', query: {postId: '1'}})
    .reply(200, businessSuccess({id: '1'}));
  pool
    .intercept({
      path: '/comment',
      method: 'POST',
      body: JSON.stringify({postId: '1', content: '评论'}),
    })
    .reply(200, businessSuccess({id: 'c1'}));
  const client = makeClient(mockAgent);
  const ctx = {accessToken: 'user-token'};
  assert.equal(
    (await client.mark.batchAdd(ctx, {spuIds: ['1']})).result?.['1'],
    'm1',
  );
  assert.equal((await client.post.detail(ctx, {postId: '1'})).id, '1');
  assert.equal(
    (await client.comment.create(ctx, {postId: '1', content: '评论'})).id,
    'c1',
  );
});

void test('comment 动态路径接口拒绝非法 ID，且不发请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/comment/1/reply',
      method: 'POST',
      body: JSON.stringify({content: '回复'}),
    })
    .reply(200, businessSuccess({id: 'c2'}));
  pool
    .intercept({
      path: '/post/1/comments',
      method: 'GET',
      query: {limit: '10', offset: '0'},
    })
    .reply(200, businessSuccess({items: [], count: 0}));
  const client = makeClient(mockAgent);
  const ctx = {accessToken: 'user-token'};
  await assert.rejects(
    () => client.comment.reply(ctx, {commentId: '..', content: '回复'}),
    QdmpValidationError,
  );
  await assert.rejects(
    () =>
      client.comment.postComments(ctx, {
        postId: '..',
        limit: '10',
        offset: '0',
      }),
    QdmpValidationError,
  );
  await assert.rejects(
    () => client.comment.like(ctx, {commentId: '..', liked: true}),
    QdmpValidationError,
  );
  await assert.rejects(
    () => client.comment.replies(ctx, {commentId: '..'}),
    QdmpValidationError,
  );
  assert.equal(mockAgent.pendingInterceptors().length, 2);
});

void test('post 列表与 comment.like/replies 映射 method、path、query、body 和响应', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({
      path: '/post/v1/list',
      method: 'GET',
      query: {offset: '0', limit: '20'},
    })
    .reply(200, businessSuccess({items: [{id: 'p1'}]}));
  pool
    .intercept({
      path: '/post/v1/me/list',
      method: 'GET',
    })
    .reply(200, businessSuccess({items: []}));
  pool
    .intercept({
      path: '/comment/1/like',
      method: 'POST',
      body: JSON.stringify({liked: true}),
    })
    .reply(200, {code: '0', message: 'ok'});
  pool
    .intercept({
      path: '/comment/1/replies',
      method: 'GET',
      query: {limit: '10', cursor: 'next'},
    })
    .reply(200, businessSuccess({items: [{id: 'r1'}], cursor: 'next'}));
  const client = makeClient(mockAgent);
  const ctx = {accessToken: 'user-token'};
  assert.deepEqual(await client.post.list(ctx, {offset: '0', limit: '20'}), {
    items: [{id: 'p1'}],
  });
  assert.deepEqual(await client.post.myList(ctx), {items: []});
  assert.equal(
    await client.comment.like(ctx, {commentId: '1', liked: true}),
    undefined,
  );
  assert.equal(
    (
      await client.comment.replies(ctx, {
        commentId: '1',
        limit: '10',
        cursor: 'next',
      })
    ).items?.[0]?.id,
    'r1',
  );
});

void test('新增分组缺 token 时不发请求', async () => {
  const {mockAgent, pool} = createMockAgent();
  pool
    .intercept({path: '/post/v1/detail', method: 'GET'})
    .reply(200, businessSuccess({}));
  await expectFailure(() =>
    // @ts-expect-error 故意验证运行时缺 token 检查
    makeClient(mockAgent).post.detail({}, {postId: '1'}),
  );
  await expectFailure(() =>
    // @ts-expect-error 故意验证运行时缺 token 检查
    makeClient(mockAgent).comment.create({}, {postId: '1', content: '评论'}),
  );
  assert.equal(mockAgent.pendingInterceptors().length, 1);
});
