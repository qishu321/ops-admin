import test from 'node:test'
import assert from 'node:assert/strict'
import { zkFormatBytes, zkHex, zkJoinPath, zkNameValid, zkParentPath, zkPretty, zkSegments, zkStatRows, zkTime, zkTreeNodes } from '../src/views/assets/zk-node.js'

test('ZooKeeper 路径拼接与父级计算保持根节点规范', () => {
  assert.equal(zkJoinPath('/', 'app'), '/app')
  assert.equal(zkJoinPath('/app', 'config'), '/app/config')
  assert.equal(zkParentPath('/'), '/')
  assert.equal(zkParentPath('/app'), '/')
  assert.equal(zkParentPath('/app/config'), '/app')
})

test('面包屑按层级生成可点击路径', () => {
  assert.deepEqual(zkSegments('/'), [{ name: '/', path: '/' }])
  assert.deepEqual(zkSegments('/Mgri4Szfc/1040/Cluster'), [
    { name: '/', path: '/' },
    { name: 'Mgri4Szfc', path: '/Mgri4Szfc' },
    { name: '1040', path: '/Mgri4Szfc/1040' },
    { name: 'Cluster', path: '/Mgri4Szfc/1040/Cluster' }
  ])
})

test('节点名称拒绝空值与层级符号', () => {
  assert.equal(zkNameValid('Cluster'), true)
  assert.equal(zkNameValid(''), false)
  assert.equal(zkNameValid('a/b'), false)
  assert.equal(zkNameValid('.'), false)
  assert.equal(zkNameValid('..'), false)
})

test('大小、zxid 与时间按节点信息展示格式输出', () => {
  assert.equal(zkFormatBytes(672), '672 B')
  assert.equal(zkFormatBytes(2048), '2.0 KB')
  assert.equal(zkFormatBytes(-1), '—')
  assert.equal(zkHex(0), '0x0')
  assert.equal(zkHex(10321689704), '0x267387c68')
  assert.equal(zkTime(0), '—')
  assert.match(zkTime(1759216026000), /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
})

test('只有合法 JSON 才允许格式化，非法内容保持原文', () => {
  assert.deepEqual(zkPretty('{"a":1}'), { ok: true, value: '{\n  "a": 1\n}' })
  assert.deepEqual(zkPretty('not json'), { ok: false, value: 'not json' })
  assert.deepEqual(zkPretty(''), { ok: false, value: '' })
})

test('节点信息按 zkCli stat 字段顺序输出', () => {
  const rows = zkStatRows({ Czxid: 5, Mzxid: 7, Ctime: 0, Mtime: 0, Version: 3, Cversion: 1, Aversion: 0, EphemeralOwner: 0, DataLength: 672, NumChildren: 0, Pzxid: 6 })
  assert.deepEqual(rows.map(r => r.key), ['ephemeralOwner', 'mtime', 'ctime', 'mZxid', 'pZxid', 'cZxid', 'dataLength', 'numChildren', 'dataVersion', 'aclVersion', 'cVersion'])
  assert.equal(rows.find(r => r.key === 'dataLength').value, '672')
  assert.equal(rows.find(r => r.key === 'dataVersion').value, '3')
  assert.equal(rows.find(r => r.key === 'mZxid').value, '0x7')
})

test('树节点使用子节点摘要，缺少摘要时退化为名称列表', () => {
  const nodes = zkTreeNodes({ path: '/', items: [{ name: 'app', path: '/app', hasChildren: true, numChildren: 2 }, { name: 'leaf', path: '/leaf', hasChildren: false }] })
  assert.deepEqual(nodes.map(n => [n.name, n.path, n.isLeaf]), [['app', '/app', false], ['leaf', '/leaf', true]])
  assert.deepEqual(zkTreeNodes({ path: '/app', children: ['a'] }).map(n => n.path), ['/app/a'])
  assert.deepEqual(zkTreeNodes(undefined), [])
})
