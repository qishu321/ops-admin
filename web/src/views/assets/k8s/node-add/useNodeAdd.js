import { computed, onBeforeUnmount, reactive, watch } from 'vue'
import { nodeAddContext, nodeAddPrecheck, nodeAddStart, nodeAddTasks, nodeAddTask, nodeAddCancel } from '../../../../api/k8s-node-add'
import { validateNodeDraft } from '../../../../utils/k8s-node-add'

export function useNodeAdd(clusterId, completed) {
  const state = reactive({ visible: false, step: 0, busy: false, checking: false, error: '', context: null, check: null, task: null, history: [], view: 'wizard',
    draft: { clusterId, nodes: [], labels: '', taints: '', disableSELinux: true, disableFirewalld: false }, idempotencyKey: '' })
  let timer, checkTimer, checkGeneration = 0, generation = 0, disposed = false
  const notified = new Set()
  const running = computed(() => state.task?.status === 'running')
  const checkCurrent = () => state.check?.passed && Date.parse(state.check.expiresAt) > Date.now()
  const validCheck = computed(checkCurrent)
  watch(() => JSON.stringify(state.draft), () => { ++checkGeneration; clearTimeout(checkTimer); state.checking = false; state.check = null; state.idempotencyKey = ''; if (state.step === 3) state.step = 2 }, { flush: 'sync' })
  async function perform(fn) {
    if (state.busy) return
    state.busy = true; state.error = ''
    try { return await fn() } catch (error) { if (!disposed) state.error = error.response?.status === 404 ? '添加节点接口未启动，请更新并重启后端服务' : error.response?.data?.message || error.message || '请求失败，请重试' }
    finally { if (!disposed) state.busy = false }
  }
  async function open(view = 'wizard') {
    state.visible = true
    await perform(async () => {
      const [context, history] = await Promise.all([nodeAddContext(clusterId), nodeAddTasks(clusterId)])
      if (disposed) return
      if (typeof context?.canAdd !== 'boolean') throw new Error('节点添加接口版本不匹配，请更新并重启后端')
      state.context = context; state.history = history
      if (view === 'wizard' && state.view !== 'wizard' && state.task && !history.some(t => t.id === state.task.id && t.status === 'running')) newDraft()
      state.view = view
      const active = history.find(t => t.status === 'running')
      if (active) showTask(active)
      else if (state.check?.status === 'running') pollCheck()
    })
  }
  function pick(hosts) {
    const selected = new Map(state.draft.nodes.map(n => [n.hostId, n]))
    hosts.forEach(h => { if (!selected.has(h.id)) selected.set(h.id, { ...h, hostId: h.id, nodeName: h.hostName.toLowerCase().replace(/[^a-z0-9.-]/g, '-') }) })
    state.draft.nodes = [...selected.values()]
  }
  async function check() {
    return perform(async () => {
      validateNodeDraft(state.draft)
      const snapshot = JSON.stringify(state.draft)
      const result = await nodeAddPrecheck(JSON.parse(snapshot))
      if (!disposed && snapshot === JSON.stringify(state.draft)) { state.check = result; if (result.status === 'running') pollCheck() }
    })
  }
  function pollCheck() {
    clearTimeout(checkTimer)
    const current = ++checkGeneration
    state.checking = true
    const tick = async () => {
      try {
        const result = await nodeAddTask(clusterId, state.check.id)
        if (disposed || current !== checkGeneration || !state.visible) return
        state.check = result; state.error = ''
        if (result.status !== 'running') { state.checking = false; return }
      } catch (e) { if (disposed || current !== checkGeneration) return; state.error = '读取预检进度失败，正在重试；关闭后可重新查看' }
      if (state.visible && !disposed && current === checkGeneration) checkTimer = setTimeout(tick, 1000)
    }
    void tick()
  }
  async function next() {
    state.error = ''
    if (!state.context?.canAdd) { state.error = state.context?.blockedReason || '无法添加节点'; return }
    try { validateNodeDraft(state.draft) } catch (e) { state.error = e.message; return }
    if (state.step === 2 && !checkCurrent()) { state.error = '请先完成预检；更改配置或检查过期后需要重新检查'; return }
    state.step++
  }
  async function start() {
    await perform(async () => {
      if (!checkCurrent()) { state.step = 2; throw new Error('预检已失效，请重新检查') }
      // getRandomValues also works on a LAN HTTP origin, where randomUUID is unavailable.
      state.idempotencyKey ||= [...crypto.getRandomValues(new Uint8Array(16))].map(n => n.toString(16).padStart(2, '0')).join('')
      const result = await nodeAddStart({ ...JSON.parse(JSON.stringify(state.draft)), precheckId: state.check.id, idempotencyKey: state.idempotencyKey })
      if (disposed) return
      state.task = result; state.view = 'progress'; poll()
    })
  }
  function poll() {
    clearTimeout(timer)
    const current = ++generation
    const tick = async () => {
      try {
        const result = await nodeAddTask(clusterId, state.task.id)
        if (disposed || current !== generation || !state.visible) return
        state.task = result; state.error = ''
        if (result.status !== 'running') {
          if (!notified.has(result.id)) { notified.add(result.id); completed() }
          const [history, context] = await Promise.all([nodeAddTasks(clusterId), nodeAddContext(clusterId)])
          if (disposed || current !== generation) return
          state.history = history; state.context = context
          return
        }
      } catch (e) { if (disposed || current !== generation) return; state.error = '读取进度失败，正在重试；任务仍保留，可关闭后重新查看' }
      if (state.visible && !disposed && current === generation) timer = setTimeout(tick, 1000)
    }
    void tick()
  }
  function showTask(task) {
    if (task.kind === 'precheck') {
      state.draft = { ...task.config, nodes: task.nodes.map(node => ({ ...node.host, hostId: node.hostId, nodeName: node.nodeName })) }
      state.check = task; state.step = 2; state.view = 'wizard'
      if (task.status === 'running') pollCheck()
      return
    }
    state.task = task; state.view = 'progress'; poll()
  }
  async function cancel() {
    await perform(async () => { state.task = await nodeAddCancel(clusterId, state.task.id); poll() })
  }
  function retryFailed() {
    const failed = state.task.nodes.filter(n => n.retryable !== false && n.status !== 'success')
    state.draft = { ...state.task.config, nodes: failed.map(n => ({ ...n.host, hostId: n.hostId, nodeName: n.nodeName })), retryOf: state.task.id }
    state.check = null; state.step = 1; state.view = 'wizard'; state.error = ''
  }
  function newDraft() { state.draft = { clusterId, nodes: [], labels: '', taints: '', disableSELinux: true, disableFirewalld: false }; state.check = null; state.step = 0; state.task = null; state.view = 'wizard'; state.error = '' }
  watch(() => state.visible, open => { if (!open) { ++generation; ++checkGeneration; clearTimeout(timer); clearTimeout(checkTimer); state.checking = false } })
  onBeforeUnmount(() => { disposed = true; ++generation; ++checkGeneration; clearTimeout(timer); clearTimeout(checkTimer) })
  return { state, running, validCheck, open, pick, check, next, start, showTask, cancel, retryFailed, newDraft }
}
