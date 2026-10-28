// Acceptance-only state machine. No database, SSH, Kubernetes client, or upstream HTTP.
import { createHash } from 'node:crypto'
import { validateNodeDraft, parseNodeMetadata, nodeAddStages } from '../../src/utils/k8s-node-add.js'
const copy = value => structuredClone(value)
function reject(message, status = 400) { throw Object.assign(new Error(message), { status }) }
export class NodeAddMemory {
  constructor(hosts, now = () => Date.now()) {
    this.hosts = copy(hosts); this.now = now; this.sequence = 1000
    this.tasks = new Map(); this.checks = new Map(); this.pendingChecks = new Map(); this.locks = new Map(); this.keys = new Map()
    this.clusters = [
      { id: 901, name: '研发测试集群 · 模拟', source: 'deployment', canAdd: true },
      { id: 902, name: '手动接入集群 · 模拟', source: 'imported', canAdd: false, blockedReason: '仅登记了 kubeconfig，缺少安装配置和扩容凭据；本阶段不支持此类集群自动添加。' },
      { id: 903, name: '空节点列表 · 模拟', source: 'deployment', canAdd: true }
    ].map(c => ({ ...c, version: 'v1.34.9', apiServer: `https://192.0.2.${c.id - 900}:6443`, status: 1, nodeCount: c.id === 903 ? 0 : 1 }))
    this.nodes = new Map(this.clusters.map(c => [c.id, c.id === 903 ? [] : [this.node(hosts[0], hosts[0].hostName, 'control-plane')]]))
  }
  node(host, name, role = 'worker', config = {}) {
    return { hostId: host.id, name, role, roles: role, status: 'Ready', internalIP: host.privateIp, os: host.os, architecture: host.arch,
      version: 'v1.34.9', cpu: '4000m', memory: '8192 Mi', pods: '0/110', containerRuntime: 'containerd', kernel: '6.8.0 (模拟)',
      allocatableCPU: '3800m', capacityCPU: '4000m', allocatableMemory: '7680 Mi', capacityMemory: '8192 Mi',
      labels: Object.fromEntries(parseNodeMetadata(config.labels).map(l => [l.key, l.value])), taints: parseNodeMetadata(config.taints, true) }
  }
  cluster(id) { const c = this.clusters.find(c => c.id === Number(id)); if (!c) reject('模拟集群不存在', 404); return c }
  context(id) {
    const c = this.cluster(id)
    return copy({ ...c, nodeCount: this.nodes.get(c.id).length, mode: 'simulation', runtime: c.canAdd ? 'containerd' : '待补充', network: c.canAdd ? 'Calico' : '待补充', podCIDR: c.canAdd ? '172.30.0.0/16' : '待补充', serviceCIDR: c.canAdd ? '10.254.0.0/16' : '待补充', environments: [{ code: 'test', name: '测试环境' }], groups: [{ id: 1, name: '模拟服务器组' }] })
  }
  hostReason(host) {
    if (!host) return '资产不存在'
    if (this.clusters.some(c => this.nodes.get(c.id).some(n => n.hostId === host.id || n.internalIP === host.privateIp))) return '已属于模拟集群'
    if (host.disabledReason) return host.disabledReason
    if (host.occupiedPlanId || this.locks.has(host.id)) return '被其他任务占用'
    return ''
  }
  hostsFor(params) {
    this.cluster(params.clusterId)
    const keyword = String(params.keyword || '').toLowerCase()
    const list = this.hosts.filter(h => `${h.hostName} ${h.privateIp}`.toLowerCase().includes(keyword) && (!params.environment || h.environment === params.environment) && (!params.groupId || String(h.groupId) === String(params.groupId)))
      .map(h => ({ ...h, disabledReason: this.hostReason(h) }))
    const page = Math.max(1, Number(params.pageNum) || 1), size = Math.min(50, Math.max(1, Number(params.pageSize) || 5))
    return copy({ list: list.slice((page - 1) * size, page * size), total: list.length })
  }
  normalize(body) {
    const config = { clusterId: Number(body.clusterId), nodes: (body.nodes || []).map(n => ({ hostId: Number(n.hostId), nodeName: String(n.nodeName || '') })),
      labels: String(body.labels || '').trim(), taints: String(body.taints || '').trim(), disableSELinux: Boolean(body.disableSELinux), disableFirewalld: Boolean(body.disableFirewalld), scenario: body.scenario || 'success', retryOf: Number(body.retryOf) || 0 }
    validateNodeDraft(config)
    if (!['success', 'partial', 'blocked'].includes(config.scenario)) reject('未知模拟场景')
    const c = this.cluster(config.clusterId)
    if (!c.canAdd) reject(c.blockedReason)
    return config
  }
  fingerprint(config) { return createHash('sha256').update(JSON.stringify(config)).digest('hex') }
  validateHosts(config) {
    const existing = this.nodes.get(config.clusterId)
    for (const n of config.nodes) {
      const reason = this.hostReason(this.hosts.find(h => h.id === n.hostId))
      if (reason) reject(`${n.nodeName}：${reason}`)
      if (existing.some(e => e.name === n.nodeName)) reject(`节点名已存在：${n.nodeName}`)
      if ([...this.tasks.values()].some(t => t.clusterId === config.clusterId && t.status === 'running' && t.nodes.some(e => e.nodeName === n.nodeName))) reject(`节点名被运行中任务占用：${n.nodeName}`)
    }
  }
  precheck(body) {
    const config = this.normalize(body); this.validateHosts(config)
    const nodes = config.nodes.map((n, i) => {
      const host = this.hosts.find(h => h.id === n.hostId), fail = config.scenario === 'blocked' && i === config.nodes.length - 1
      return { ...n, host: copy(host), passed: !fail, checks: [
        { title: '连通性与权限', status: 'success', message: '模拟 SSH 与提权检查通过' },
        { title: '系统与架构', status: 'success', message: `${host.os} / ${host.arch}（模拟识别）` },
        { title: '资源与安装状态', status: 'success', message: `${host.cpu} 核 / ${host.memory}；无已有安装（模拟）` },
        { title: '名称与 IP 冲突', status: 'success', message: '模拟资产中未发现冲突' },
        { title: '集群可达性', status: fail ? 'failed' : 'success', message: fail ? '模拟 API Server 不可达，请切换模拟场景后重检' : '模拟连通性正常' },
        ...(host.connectionMode === 'gateway' ? [{ title: '访问路径', status: 'warning', message: '使用模拟 SSH 网关；真实环境需另行验证网络路径' }] : [])
      ] }
    })
    const result = { id: ++this.sequence, clusterId: config.clusterId, config: copy(config), kind: 'precheck', status: nodes.every(n => n.passed) ? 'success' : 'failed', fingerprint: this.fingerprint(config), passed: nodes.every(n => n.passed), nodes, expiresAt: new Date(this.now() + 300000).toISOString() }
    this.checks.set(result.id, copy(result)); return copy(result)
  }
  start(body) {
    const config = this.normalize(body), fingerprint = this.fingerprint(config)
    const key = `${config.clusterId}:${body.idempotencyKey}`
    if (!body.idempotencyKey || String(body.idempotencyKey).length > 128) reject('缺少有效幂等键')
    if (this.keys.has(key)) {
      const prior = this.tasks.get(this.keys.get(key))
      if (prior.fingerprint !== fingerprint) reject('幂等键不能用于不同配置', 409)
      return copy(prior)
    }
    const check = this.checks.get(Number(body.precheckId))
    if (!check?.passed || check.fingerprint !== fingerprint || Date.parse(check.expiresAt) <= this.now()) reject('预检未通过、已过期或配置已变更，请重新检查')
    this.validateHosts(config)
    if (config.retryOf) {
      const parent = this.tasks.get(config.retryOf)
      if (!parent || parent.clusterId !== config.clusterId || parent.status === 'running' || config.nodes.some(n => !parent.nodes.some(p => p.hostId === n.hostId && p.status !== 'success'))) reject('重试仅允许原任务未成功的节点')
    }
    const task = { id: ++this.sequence, clusterId: config.clusterId, clusterName: this.cluster(config.clusterId).name, config: copy(config), fingerprint, status: 'running', stage: 0,
      createdAt: new Date(this.now()).toISOString(), nodes: config.nodes.map(n => ({ ...n, host: copy(this.hosts.find(h => h.id === n.hostId)), status: 'running', stage: 0, message: '等待模拟执行' })), logs: ['[模拟] 任务已创建，配置快照已冻结；不执行远程命令。'] }
    for (const n of task.nodes) this.locks.set(n.hostId, task.id)
    this.tasks.set(task.id, task); this.keys.set(key, task.id); return copy(task)
  }
  task(clusterId, id) { this.cluster(clusterId); const task = this.tasks.get(Number(id)); if (!task || task.clusterId !== Number(clusterId)) reject('模拟任务不存在', 404); return copy(task) }
  list(clusterId) { this.cluster(clusterId); return copy([...this.tasks.values()].filter(t => t.clusterId === Number(clusterId)).reverse()) }
  advance(id) {
    const task = this.tasks.get(id); if (!task || task.status !== 'running') return
    task.stage++
    for (const [index, n] of task.nodes.entries()) {
      if (n.status !== 'running') continue
      n.stage = task.stage
      if (task.config.scenario === 'partial' && index === task.nodes.length - 1 && task.stage === 3) {
        n.status = 'failed'; n.message = '模拟 join 超时；可重试此节点'; task.logs.push(`[模拟] ${n.nodeName}：加入集群超时。`); continue
      }
      n.message = `${nodeAddStages[task.stage]}（模拟）`
      task.logs.push(`[模拟] ${n.nodeName} · ${nodeAddStages[task.stage]}：通过`)
      if (task.stage === nodeAddStages.length - 1) {
        n.status = 'success'; n.message = '模拟节点 Ready'
        this.nodes.get(task.clusterId).push(this.node(n.host, n.nodeName, 'worker', task.config))
      }
    }
    if (!task.nodes.some(n => n.status === 'running')) {
      task.status = task.nodes.every(n => n.status === 'success') ? 'success' : task.nodes.some(n => n.status === 'success') ? 'partial' : 'failed'
      this.finish(task)
    }
  }
  finish(task) { task.finishedAt = new Date(this.now()).toISOString(); for (const n of task.nodes) if (this.locks.get(n.hostId) === task.id) this.locks.delete(n.hostId); this.cluster(task.clusterId).nodeCount = this.nodes.get(task.clusterId).length }
  cancel(clusterId, id) {
    this.task(clusterId, id); const task = this.tasks.get(Number(id))
    if (task.status === 'running') {
      task.nodes.filter(n => n.status === 'running').forEach(n => { n.status = 'cancelled'; n.message = '后续模拟已停止' })
      task.status = 'cancelled'; task.logs.push('[模拟] 停止后续步骤，保留已完成结果。'); this.finish(task)
    }
    return copy(task)
  }
  handle(req, url, body, respond, res) {
    const path = url.pathname, params = Object.fromEntries(url.searchParams), method = req.method
    if (!path.startsWith('/api/v1/k8s/node-add/') && !['/api/v1/k8s/cluster/list', '/api/v1/k8s/cluster/detail', '/api/v1/k8s/node/detail', '/api/v1/k8s/node/pods', '/api/v1/k8s/node/labels'].includes(path)) return false
    try {
      if (req.headers.authorization !== 'Bearer isolated-test-token') reject('需要隔离验收管理员身份', 403)
      const action = path.replace('/api/v1/k8s/node-add/', '')
      let data
      if (method === 'GET' && path.endsWith('/cluster/list')) data = copy(this.clusters)
      else if (method === 'GET' && path.endsWith('/cluster/detail')) { const cluster = this.cluster(params.clusterId); data = { cluster: copy(cluster), nodes: copy(this.nodes.get(cluster.id)), overview: {}, namespaces: [], pods: [], workloads: [], network: {}, configStorage: {} } }
      else if (path.includes('/node/')) {
        const cluster = this.cluster(params.clusterId || body.clusterId), name = params.nodeName || body.nodeName
        const node = this.nodes.get(cluster.id).find(n => n.name === name); if (!node) reject('模拟节点不存在', 404)
        if (method === 'GET' && path.endsWith('/detail')) data = copy(node)
        else if (method === 'GET' && path.endsWith('/pods')) data = []
        else if (method === 'PUT' && path.endsWith('/labels')) { node.labels = copy(body.labels || {}); data = true }
        else reject('模拟接口未实现', 404)
      }
      else if (method === 'GET' && action === 'context') data = this.context(params.clusterId)
      else if (method === 'GET' && action === 'hosts') data = this.hostsFor(params)
      else if (method === 'POST' && action === 'prechecks') {
        const result = this.precheck(body)
        this.pendingChecks.set(result.id, this.now() + 1500)
        data = { ...result, status: 'running', passed: false }
      }
      else if (method === 'POST' && action === 'tasks') data = this.start(body)
      else if (method === 'GET' && action === 'tasks') {
        const checks = [...this.checks.values()].filter(check => check.clusterId === Number(params.clusterId) && this.pendingChecks.has(check.id)).map(check => this.now() < this.pendingChecks.get(check.id) ? { ...copy(check), status: 'running', passed: false } : copy(check))
        data = [...this.list(params.clusterId), ...checks].sort((a, b) => b.id - a.id)
      }
      else if (method === 'GET' && /^tasks\/\d+$/.test(action)) {
        const id = Number(action.split('/')[1]), check = this.checks.get(id)
        if (check && this.pendingChecks.has(id)) {
          if (check.clusterId !== Number(params.clusterId)) reject('预检不存在', 404)
          data = this.now() < this.pendingChecks.get(id) ? { ...copy(check), status: 'running', passed: false } : copy(check)
        } else data = this.task(params.clusterId, id)
      }
      else if (method === 'POST' && /^tasks\/\d+\/cancel$/.test(action)) data = this.cancel(body.clusterId, action.split('/')[1])
      else reject('模拟接口未实现', 404)
      respond(res, data)
    } catch (e) { respond(res, null, e.status || 400, e.message) }
    return true
  }
}
