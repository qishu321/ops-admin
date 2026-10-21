import test from 'node:test'
import assert from 'node:assert/strict'
import {
  nacosConfigKey,
  nacosConfigPayload,
  nacosConfigRows,
  nacosConfigTotal,
  nacosFormatLabel,
  nacosInstancePayload,
  nacosInstanceRows,
  nacosNamespaceOptions,
  nacosNamespacePayload,
  nacosPretty,
  nacosServiceIdentity,
  nacosServiceRows,
  nacosServiceTotal,
  nacosTime
} from '../src/views/assets/nacos-view.js'

test('配置列表兼容 2.x / 3.x 返回结构并保留总数', () => {
  assert.deepEqual(nacosConfigRows({ pageItems: [{ dataId: 'a.yaml' }] }), [{ dataId: 'a.yaml' }])
  assert.deepEqual(nacosConfigRows({}), [])
  assert.equal(nacosConfigTotal({ pageItems: [{}], totalCount: 42 }, 1), 42)
  assert.equal(nacosConfigTotal({}, 3), 3)
  assert.equal(nacosConfigKey({ group: 'OPS', dataId: 'app.yaml' }), 'OPS::app.yaml')
})

test('服务列表兼容 catalog、doms 与 3.x 分页结构', () => {
  assert.deepEqual(nacosServiceRows({ serviceList: [{ name: 'order', ipCount: 2 }] }), [{ name: 'order', ipCount: 2 }])
  assert.deepEqual(nacosServiceRows({ doms: ['order'] }), [{ name: 'order' }])
  assert.deepEqual(nacosServiceRows({ pageItems: [{ name: 'user' }] }), [{ name: 'user' }])
  assert.equal(nacosServiceTotal({ count: 8 }, 1), 8)
  assert.equal(nacosServiceTotal({ totalCount: 5 }, 1), 5)
  assert.equal(nacosServiceTotal({ serviceList: [{}] }, 1), 1)
})

test('实例、命名空间与格式标签完成归一化', () => {
  assert.deepEqual(nacosInstanceRows({ hosts: [{ ip: '10.0.0.1', port: 8080 }] }), [{ ip: '10.0.0.1', port: 8080 }])
  assert.deepEqual(nacosInstanceRows({ pageItems: [{ ip: '10.0.0.2' }] }), [{ ip: '10.0.0.2' }])
  assert.deepEqual(nacosInstanceRows({}), [])
  const options = nacosNamespaceOptions([{ namespace: 'dev', namespaceShowName: '开发', namespaceDesc: 'dev ns', configCount: 3 }])
  assert.deepEqual(options[0], { id: 'public', name: 'public', desc: '默认命名空间', configCount: 0, quota: 0, type: 0 })
  assert.equal(options[1].id, 'dev')
  assert.equal(options[1].name, '开发')
  assert.equal(options[1].configCount, 3)
  const withPublic = nacosNamespaceOptions([{ namespace: '', namespaceShowName: 'public' }, { namespace: 'dev', namespaceShowName: '开发' }])
  assert.equal(withPublic.length, 2)
  assert.equal(nacosFormatLabel('json'), 'JSON')
  assert.equal(nacosFormatLabel(''), 'TEXT')
  assert.equal(nacosFormatLabel('properties'), 'PROPERTIES')
})

test('配置时间与 JSON 格式化遵循控制台展示规则', () => {
  assert.equal(nacosTime(0), '—')
  assert.equal(nacosTime('bad'), '—')
  const stamp = new Date(2026, 9, 21, 18, 5, 6).getTime()
  assert.equal(nacosTime(stamp), '2026-10-21 18:05:06')
  assert.deepEqual(nacosPretty('{"a":1}', 'json'), { ok: true, value: '{\n  "a": 1\n}' })
  assert.equal(nacosPretty('{bad', 'json').ok, false)
  assert.equal(nacosPretty('a: 1', 'yaml').ok, false)
})

test('配置发布参数会补齐默认分组并校验 Data ID', () => {
  const missing = nacosConfigPayload({ dataId: '  ' })
  assert.equal(missing.ok, false)
  const payload = nacosConfigPayload({ dataId: ' app.yaml ', groupName: '', namespaceId: 'dev', type: 'JSON', content: '{"a":1}', appName: ' order ', desc: '' })
  assert.equal(payload.ok, true)
  assert.equal(payload.params.dataId, 'app.yaml')
  assert.equal(payload.params.groupName, 'DEFAULT_GROUP')
  assert.equal(payload.params.namespaceId, 'dev')
  assert.equal(payload.params.type, 'json')
  assert.equal(payload.params.appName, 'order')
  assert.equal(payload.params.content, '{"a":1}')
  assert.equal(nacosConfigPayload({ dataId: 'x'.repeat(256) }).ok, false)
})

test('实例上下线与移除参数携带服务身份', () => {
  const service = { name: 'order', groupName: 'OPS', namespaceId: 'dev' }
  const instance = { ip: '10.0.0.1', port: 8080, weight: 2, ephemeral: false }
  const payload = nacosInstancePayload(service, instance, false)
  assert.deepEqual(payload, {
    serviceName: 'order',
    groupName: 'OPS',
    namespaceId: 'dev',
    ip: '10.0.0.1',
    port: '8080',
    enabled: 'false',
    weight: '2',
    ephemeral: 'false',
    clusterName: ''
  })
  assert.deepEqual(nacosServiceIdentity({ name: 'user' }, 'dev'), { serviceName: 'user', groupName: 'DEFAULT_GROUP', namespaceId: 'dev' })
})

test('命名空间创建参数校验 ID 字符并保留描述', () => {
  assert.equal(nacosNamespacePayload({ namespaceName: '' }).ok, false)
  assert.equal(nacosNamespacePayload({ namespaceName: '开发', namespaceId: 'bad id' }).ok, false)
  const payload = nacosNamespacePayload({ namespaceName: ' 开发 ', namespaceId: 'dev-1', namespaceDesc: ' 测试 ' })
  assert.equal(payload.ok, true)
  assert.deepEqual(payload.params, { namespaceId: 'dev-1', namespaceName: '开发', namespaceDesc: '测试' })
})
