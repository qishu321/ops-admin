import assert from 'node:assert/strict'
import test from 'node:test'
import { buildRedisKeyCommands, normalizeRedisSchema, quoteRedisArgument } from '../src/utils/redis-workbench.js'

test('逻辑库输入规范化，不把非法编号回退到 db0', () => {
  assert.equal(normalizeRedisSchema('10'), 'db10')
  assert.equal(normalizeRedisSchema(' DB10 '), 'db10')
  for (const value of ['db', '-1', 'db-1', '2.5', 'db10 GET secret', '2147483648']) assert.throws(() => normalizeRedisSchema(value))
})

test('Key 和值保留空格、引号、反斜杠与多行文本', () => {
  assert.equal(quoteRedisArgument('my key'), '"my key"')
  const commands = buildRedisKeyCommands({ key: 'my key', type: 'string', value: 'a\nb"c\\d' }, true)
  assert.equal(commands[0], 'SET "my key" "a\nb\\"c\\\\d" KEEPTTL')
})

test('复杂类型覆盖前要求完整值和明确 TTL，zset score=0 有效', () => {
  assert.deepEqual(buildRedisKeyCommands({ key: 'list', type: 'list', value: '["a","b"]', ttl: -1 }, true), ['DEL "list"', 'RPUSH "list" "a" "b"', 'PERSIST "list"'])
  assert.throws(() => buildRedisKeyCommands({ key: 'list', type: 'list', value: '["a"]' }, true))
  assert.deepEqual(buildRedisKeyCommands({ key: 'ranking', type: 'zset', value: '[{"member":"user:1","score":0}]', ttl: 60 }), ['ZADD "ranking" "0" "user:1"', 'EXPIRE "ranking" 60'])
  assert.throws(() => buildRedisKeyCommands({ key: 'ranking', type: 'zset', value: '[{"member":"user:1","score":"oops"}]' }))
})
