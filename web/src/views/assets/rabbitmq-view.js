// Pure presentation helpers for the RabbitMQ workbench. Keeping them free of Vue
// makes the formatting rules testable without mounting the page.
export const rabbitAllVhosts = '*'
export const rabbitExchangeTypes = ['direct', 'fanout', 'topic', 'headers']
export const rabbitQueueTypeOptions = [['', '服务端默认'], ['classic', 'Classic'], ['quorum', 'Quorum'], ['stream', 'Stream']]

export function rabbitVhostLabel(vhost) {
  return !vhost || vhost === rabbitAllVhosts ? '全部虚拟主机' : vhost
}

export function rabbitVhostRows(vhosts, keyword = '') {
  const search = String(keyword || '').trim().toLowerCase()
  return (Array.isArray(vhosts) ? vhosts : [])
    .filter(vhost => vhost && typeof vhost.name === 'string')
    .filter(vhost => !search || vhost.name.toLowerCase().includes(search))
    .map(vhost => ({
      name: vhost.name,
      messages: Number(vhost.messages || 0),
      ready: Number(vhost.messages_ready || 0),
      unacked: Number(vhost.messages_unacknowledged || 0),
      description: vhost.description || ''
    }))
}

export function rabbitQueueRows(queues, keyword = '') {
  const search = String(keyword || '').trim().toLowerCase()
  return (Array.isArray(queues) ? queues : [])
    .filter(queue => queue && typeof queue.name === 'string')
    .filter(queue => !search || queue.name.toLowerCase().includes(search))
}

export function rabbitQueueState(queue) {
  const state = String(queue?.state || '').toLowerCase()
  if (state === 'running') return { label: '运行中', type: 'success' }
  if (state === 'idle') return { label: '空闲', type: 'info' }
  if (state === 'flow') return { label: '限流', type: 'warning' }
  if (state === 'down' || state === 'stopped') return { label: '已停止', type: 'danger' }
  return { label: state || '—', type: 'info' }
}

export function rabbitConnectionState(state) {
  const value = String(state || '').toLowerCase()
  if (value === 'running') return { label: '运行中', type: 'success' }
  if (value === 'idle') return { label: '空闲', type: 'info' }
  if (value === 'flow') return { label: '限流', type: 'warning' }
  if (value === 'closing' || value === 'closed') return { label: value === 'closing' ? '关闭中' : '已关闭', type: 'danger' }
  return { label: value || '—', type: 'info' }
}

export function rabbitCount(value) {
  const num = Number(value)
  if (!Number.isFinite(num) || num < 0) return '—'
  return num.toLocaleString('en-US')
}

export function rabbitBytes(value) {
  const num = Number(value)
  if (!Number.isFinite(num) || num < 0) return '—'
  if (num < 1024) return `${num} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = num / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ }
  return `${size >= 10 ? size.toFixed(1) : Number(size.toFixed(2))} ${units[index]}`
}

export function rabbitRate(value) {
  const num = Number(value)
  if (!Number.isFinite(num) || num <= 0) return '—'
  if (num >= 1000) return `${(num / 1000).toFixed(num >= 10000 ? 0 : 1)}k/s`
  return `${num < 10 ? num.toFixed(1) : Math.round(num)}/s`
}

export function rabbitQueueMessages(queue) {
  const ready = Number(queue?.messages_ready || 0)
  const unacked = Number(queue?.messages_unacknowledged || 0)
  const total = Number(queue?.messages)
  return {
    messages: Number.isFinite(total) ? total : ready + unacked,
    ready,
    unacked
  }
}

export function rabbitArguments(args) {
  if (!args || typeof args !== 'object' || Array.isArray(args)) return []
  return Object.keys(args).sort().map(key => ({ key, value: typeof args[key] === 'string' ? args[key] : JSON.stringify(args[key]) }))
}

export function rabbitBindings(bindings) {
  return (Array.isArray(bindings) ? bindings : []).map(binding => ({
    source: binding.source === '' ? '(默认交换机)' : binding.source || '—',
    destination: binding.destination || binding.queue || '—',
    routingKey: binding.routing_key || '',
    kind: binding.destination_type || 'queue'
  }))
}

// Queue declarations are sent as RabbitMQ JSON; the UI keeps the extra arguments
// in a textarea and translates the queue type into x-queue-type.
export function rabbitQueueArguments(queueType, argumentsText) {
  const raw = String(argumentsText || '').trim()
  let args = {}
  if (raw) {
    try {
      args = JSON.parse(raw)
    } catch {
      return { ok: false, error: '扩展参数必须是合法的 JSON 对象' }
    }
    if (!args || typeof args !== 'object' || Array.isArray(args)) {
      return { ok: false, error: '扩展参数必须是 JSON 对象，例如 {"x-max-length":1000}' }
    }
  }
  if (queueType) args['x-queue-type'] = queueType
  return { ok: true, value: JSON.stringify(args) }
}

export function rabbitQueuePayload(form) {
  const name = String(form.name || '').trim()
  if (!name) return { ok: false, error: '请填写队列名称' }
  const args = rabbitQueueArguments(form.queueType, form.arguments)
  if (!args.ok) return args
  return {
    ok: true,
    params: {
      name,
      vhost: form.vhost || '/',
      durable: form.durable ? 'true' : 'false',
      auto_delete: form.autoDelete ? 'true' : 'false',
      arguments: args.value
    }
  }
}

export function rabbitExchangePayload(form) {
  const name = String(form.name || '').trim()
  if (!name) return { ok: false, error: '请填写交换机名称' }
  const args = rabbitQueueArguments('', form.arguments)
  if (!args.ok) return args
  return {
    ok: true,
    params: {
      name,
      vhost: form.vhost || '/',
      type: form.type || 'direct',
      durable: form.durable ? 'true' : 'false',
      auto_delete: form.autoDelete ? 'true' : 'false',
      arguments: args.value
    }
  }
}
