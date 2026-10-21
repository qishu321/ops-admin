import test from 'node:test'
import assert from 'node:assert/strict'
import {
  rabbitArguments,
  rabbitBindings,
  rabbitBytes,
  rabbitConnectionState,
  rabbitCount,
  rabbitExchangePayload,
  rabbitQueueArguments,
  rabbitQueueMessages,
  rabbitQueuePayload,
  rabbitQueueRows,
  rabbitQueueState,
  rabbitRate,
  rabbitVhostLabel,
  rabbitVhostRows
} from '../src/views/assets/rabbitmq-view.js'

test('虚拟主机与队列列表支持名称过滤并保留消息统计', () => {
  const vhosts = [
    { name: '/', messages: 12, messages_ready: 10, messages_unacknowledged: 2 },
    { name: 'orders', messages: 4, messages_ready: 4 },
    { name: 'billing', messages: 0 }
  ]
  assert.equal(rabbitVhostLabel('*'), '全部虚拟主机')
  assert.equal(rabbitVhostLabel('orders'), 'orders')
  assert.deepEqual(rabbitVhostRows(vhosts).map(row => row.name), ['/', 'orders', 'billing'])
  assert.deepEqual(rabbitVhostRows(vhosts, 'ord').map(row => row.name), ['orders'])
  assert.equal(rabbitVhostRows(vhosts)[0].unacked, 2)
  const queues = [{ name: 'order.created' }, { name: 'order.paid' }, { name: 'user.created' }, {}]
  assert.deepEqual(rabbitQueueRows(queues, 'order.').map(row => row.name), ['order.created', 'order.paid'])
  assert.equal(rabbitQueueRows(queues).length, 3)
})

test('队列状态、数量、容量与速率使用统一展示格式', () => {
  assert.deepEqual(rabbitQueueState({ state: 'running' }), { label: '运行中', type: 'success' })
  assert.deepEqual(rabbitQueueState({ state: 'idle' }), { label: '空闲', type: 'info' })
  assert.deepEqual(rabbitQueueState({ state: 'flow' }), { label: '限流', type: 'warning' })
  assert.deepEqual(rabbitQueueState({}), { label: '—', type: 'info' })
  assert.deepEqual(rabbitConnectionState('running'), { label: '运行中', type: 'success' })
  assert.deepEqual(rabbitConnectionState('closing'), { label: '关闭中', type: 'danger' })
  assert.deepEqual(rabbitConnectionState(''), { label: '—', type: 'info' })
  assert.equal(rabbitCount(1234567), '1,234,567')
  assert.equal(rabbitCount(undefined), '—')
  assert.equal(rabbitBytes(672), '672 B')
  assert.equal(rabbitBytes(1536), '1.5 KB')
  assert.equal(rabbitBytes(2048), '2 KB')
  assert.equal(rabbitBytes(1048576), '1 MB')
  assert.equal(rabbitBytes(-1), '—')
  assert.equal(rabbitRate(0), '—')
  assert.equal(rabbitRate(0.5), '0.5/s')
  assert.equal(rabbitRate(12.34), '12/s')
  assert.equal(rabbitRate(1500), '1.5k/s')
  assert.equal(rabbitRate(15000), '15k/s')
  assert.deepEqual(rabbitQueueMessages({ messages_ready: 3, messages_unacknowledged: 2 }), { messages: 5, ready: 3, unacked: 2 })
  assert.deepEqual(rabbitQueueMessages({ messages: 9, messages_ready: 3, messages_unacknowledged: 2 }), { messages: 9, ready: 3, unacked: 2 })
})

test('交换机/队列扩展参数按 JSON 解析并合并队列类型', () => {
  const args = rabbitQueueArguments('quorum', '{"x-max-length":1000}')
  assert.equal(args.ok, true)
  assert.deepEqual(JSON.parse(args.value), { 'x-max-length': 1000, 'x-queue-type': 'quorum' })
  assert.deepEqual(JSON.parse(rabbitQueueArguments('', '').value), {})
  assert.equal(rabbitQueueArguments('', '{bad').ok, false)
  assert.equal(rabbitQueueArguments('', '[]').ok, false)
  assert.deepEqual(rabbitArguments({ b: 2, a: 'x' }), [{ key: 'a', value: 'x' }, { key: 'b', value: '2' }])
  assert.deepEqual(rabbitArguments(null), [])
})

test('队列与交换机声明会校验必填项并保留默认值', () => {
  const missing = rabbitQueuePayload({ name: '  ' })
  assert.equal(missing.ok, false)
  const queue = rabbitQueuePayload({ name: ' order.created ', durable: false, autoDelete: true, queueType: '', arguments: '{"x-max-priority":5}' })
  assert.equal(queue.ok, true)
  assert.equal(queue.params.name, 'order.created')
  assert.equal(queue.params.vhost, '/')
  assert.equal(queue.params.durable, 'false')
  assert.equal(queue.params.auto_delete, 'true')
  assert.deepEqual(JSON.parse(queue.params.arguments), { 'x-max-priority': 5 })
  const exchange = rabbitExchangePayload({ name: 'orders', vhost: 'biz', durable: true })
  assert.equal(exchange.params.type, 'direct')
  assert.equal(exchange.params.vhost, 'biz')
  assert.equal(exchange.params.durable, 'true')
  assert.equal(rabbitExchangePayload({ name: 'x', arguments: '{bad' }).ok, false)
})

test('绑定关系标注默认交换机与目标类型', () => {
  const rows = rabbitBindings([
    { source: '', destination: 'order.created', routing_key: 'order.created', destination_type: 'queue' },
    { source: 'orders', destination: 'paid', routing_key: 'paid' }
  ])
  assert.deepEqual(rows[0], { source: '(默认交换机)', destination: 'order.created', routingKey: 'order.created', kind: 'queue' })
  assert.equal(rows[1].source, 'orders')
  assert.equal(rows[1].kind, 'queue')
  assert.deepEqual(rabbitBindings(null), [])
})
