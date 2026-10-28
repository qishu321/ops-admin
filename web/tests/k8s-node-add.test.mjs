import assert from 'node:assert/strict'
import test from 'node:test'
import { NodeAddMemory } from './fixtures/k8s-node-add-memory.mjs'
import { parseNodeMetadata, validateNodeDraft } from '../src/utils/k8s-node-add.js'
const hosts = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, hostName: `host-${i + 1}`, privateIp: `192.0.2.${i + 10}`, os: 'Ubuntu 24.04', arch: 'x86_64', cpu: '4', memory: '8G', environment: 'test', groupId: 1, disabledReason: i === 10 ? '缺少凭据' : '', occupiedPlanId: i === 11 ? 1 : 0 }))
const draft = (ids = [2, 3], scenario = 'success') => ({ clusterId: 901, nodes: ids.map(hostId => ({ hostId, nodeName: `worker-${hostId}` })), labels: 'workload=batch', taints: '', scenario })
const start = (service, config, key = 'first') => service.start({ ...config, precheckId: service.precheck(config).id, idempotencyKey: key })
test('主机跨页搜索与占用原因、缺资料的手动接入集群', () => {
  const s = new NodeAddMemory(hosts)
  assert.equal(s.hostsFor({ clusterId: 901, pageNum: 1 }).list.length, 5)
  assert.equal(s.hostsFor({ clusterId: 901, pageNum: 3 }).list.length, 2)
  assert.match(s.hostsFor({ clusterId: 901 }).list[0].disabledReason, /已属于/)
  assert.equal(s.hostsFor({ clusterId: 901, keyword: '192.0.2.20' }).list[0].disabledReason, '缺少凭据')
  assert.equal(s.hostsFor({ clusterId: 901, environment: 'prod' }).total, 0)
  assert.throws(() => s.precheck({ ...draft(), clusterId: 902 }), /缺少安装配置/)
})
test('预检与配置绑定、过期和失败均阻断提交', () => {
  let now = 1000
  const s = new NodeAddMemory(hosts, () => now), d = draft(), check = s.precheck(d)
  assert.throws(() => s.start({ ...d, labels: 'workload=changed', precheckId: check.id, idempotencyKey: 'changed' }), /配置已变更/)
  now += 300001
  assert.throws(() => s.start({ ...d, precheckId: check.id, idempotencyKey: 'expired' }), /已过期/)
  const fail = s.precheck(draft([2], 'blocked'))
  assert.equal(fail.passed, false)
  assert.throws(() => s.start({ ...draft([2], 'blocked'), precheckId: fail.id, idempotencyKey: 'blocked' }), /预检未通过/)
})
test('幂等提交、任务快照、主机互斥和跨集群任务访问', () => {
  const s = new NodeAddMemory(hosts), d = draft(), check = s.precheck(d)
  const body = { ...d, precheckId: check.id, idempotencyKey: 'stable' }, task = s.start(body)
  assert.equal(s.start(body).id, task.id)
  assert.equal(s.tasks.size, 1)
  assert.throws(() => s.start({ ...body, labels: 'test=changed' }), /幂等键不能/)
  assert.throws(() => s.precheck(d), /占用/)
  assert.throws(() => s.task(902, task.id), /不存在/)
  d.nodes[0].nodeName = 'mutated'
  assert.equal(s.task(901, task.id).config.nodes[0].nodeName, 'worker-2')
  assert.equal(s.nodes.get(901).length, 1)
})
test('部分失败保留成功节点，重试仅处理未成功节点', () => {
  const s = new NodeAddMemory(hosts), task = start(s, draft([2, 3], 'partial'))
  for (let i = 0; i < 5; i++) s.advance(task.id)
  assert.equal(s.task(901, task.id).status, 'partial')
  assert.equal(s.nodes.get(901).length, 2)
  assert.equal(s.locks.size, 0)
  const retry = start(s, { ...draft([3]), retryOf: task.id }, 'retry')
  for (let i = 0; i < 5; i++) s.advance(retry.id)
  assert.equal(s.task(901, retry.id).status, 'success')
  assert.equal(s.nodes.get(901).length, 3)
  assert.throws(() => s.precheck(draft([2])), /已属于/)
})
test('取消停止后续步骤，保留任务且不添加节点，释放模拟占用', () => {
  const s = new NodeAddMemory(hosts), task = start(s, draft())
  s.advance(task.id); s.cancel(901, task.id)
  for (let i = 0; i < 6; i++) s.advance(task.id)
  assert.equal(s.task(901, task.id).status, 'cancelled')
  assert.equal(s.nodes.get(901).length, 1)
  assert.equal(s.locks.size, 0)
  assert.equal(s.list(901).length, 1)
})
test('校验节点名、元数据、保留角色标签并阻断重复节点', () => {
  assert.throws(() => validateNodeDraft({ ...draft(), nodes: [{ hostId: 2, nodeName: 'UPPER.invalid' }] }), /名称无效/)
  assert.throws(() => validateNodeDraft({ ...draft(), nodes: [draft().nodes[0], draft().nodes[0]] }), /重复/)
  assert.throws(() => parseNodeMetadata('node-role.kubernetes.io/control-plane='), /节点角色/)
  assert.throws(() => parseNodeMetadata('a=b\na=c'), /重复/)
  assert.throws(() => parseNodeMetadata('dedicated=x:Wrong', true), /污点格式/)
  assert.deepEqual(parseNodeMetadata('dedicated=batch:NoSchedule', true), [{ key: 'dedicated', value: 'batch', effect: 'NoSchedule' }])
})
