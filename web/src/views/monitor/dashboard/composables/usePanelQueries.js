import { reactive, ref, computed, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { queryMonitorDashboardPanel } from '../../../../api/monitor'

export function usePanelQueries({ selectedDatasourceId, activePanels, panelStateKey, isListLayout, currentTimeBounds, isPodDetailPanel, timeRangeOption, timeRangeSeconds, panels }) {
  const panelResults = reactive({})

  const panelPending = reactive({})
  const isRefreshing = computed(() => Object.values(panelPending).some(Boolean))
  const syncStatus = computed(() => {
    if (isRefreshing.value) return { text: '正在查询', type: 'loading' }
    if (!activePanels.value.length) return { text: '等待配置', type: 'idle' }
    const errors = activePanels.value.filter(panel => panelResults[panel.id]?.error).length
    if (errors) return { text: `${errors} 个查询失败`, type: 'error' }
    const empty = activePanels.value.filter(panel => {
      const result = panelResults[panel.id]
      return !(result?.result?.length || result?.podResources?.length || result?.hostResources?.length)
    }).length
    return empty ? { text: `${empty} 个面板无数据`, type: 'idle' } : { text: '数据已同步', type: 'success' }
  })

  const lastRefreshAt = ref(null)
  let disposed = false
  let remainingTimer = null

  let panelRefreshVersion = 0
  let activeRequests = 0
  const waiting = []
  const requestVersions = new Map()
  const requestControllers = new Map()

  function abortError(error) {
    return error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError' || error?.name === 'AbortError'
  }
  function cancelPanelRequest(panelId) {
    const controller = requestControllers.get(panelId)
    if (!controller) return
    requestControllers.delete(panelId)
    controller.abort()
  }
  function cancelAllPanelRequests() {
    Array.from(requestControllers.keys()).forEach(cancelPanelRequest)
  }
  function invalidateQueries() {
    panelRefreshVersion++
    if (remainingTimer !== null) window.clearTimeout(remainingTimer)
    remainingTimer = null
    // Abort in-flight requests so a slow older window stops occupying one of the
    // four concurrency slots while the new window is already being fetched.
    cancelAllPanelRequests()
    Object.keys(panelPending).forEach(key => delete panelPending[key])
  }
  // Slots are handed over by release(). A waiter that no longer matches the
  // refresh version passes the slot on instead of leaking it.
  async function acquire(version) {
    if (version !== panelRefreshVersion) return false
    if (activeRequests < 4) {
      activeRequests++
      return true
    }
    await new Promise(resolve => waiting.push(resolve))
    if (version !== panelRefreshVersion) {
      release()
      return false
    }
    return true
  }
  function release() {
    const next = waiting.shift()
    if (next) next()
    else activeRequests--
  }

  const panelResultCache = new Map()

  const PANEL_QUERY_CONCURRENCY = 4

  const PANEL_CACHE_TTL = 15 * 1000
  const CACHE_LIMIT = 128
  onBeforeUnmount(() => {
    disposed = true
    invalidateQueries()
    panelResultCache.clear()
    requestVersions.clear()
  })

  const podResourceNamespace = ref('')

  const lastRefreshText = computed(() => lastRefreshAt.value ? lastRefreshAt.value.toLocaleTimeString('zh-CN', { hour12: false }) : '尚未成功查询')

  function handlePodNamespaceChange(panel) {
    panelResultCache.delete(panelCacheKey(panel))
    refreshPanel(panel)
  }

  async function refreshPanel(row) {
    if (!selectedDatasourceId.value) {
      ElMessage.warning('请先在数据源管理中配置 Prometheus 或 VictoriaMetrics 数据源')
      return
    }
    await loadPanel(row, { force: true })
  }

  async function refreshProblemPanels() {
    const items = activePanels.value.filter((panel) => ['danger', 'warning'].includes(panelStateKey(panel)))
    if (!items.length) return ElMessage.success(isListLayout.value ? '当前没有需要复核的异常巡检项' : '当前没有需要复核的异常面板')
    invalidateQueries()
    const version = panelRefreshVersion
    await runPanelQueue(items, version, true)
  }

  function panelQueryPayload(panel) {
  	const { startAt, endAt } = currentTimeBounds()
  	const rangeSeconds = Math.max(1, endAt - startAt)
  	return {
  		id: panel.id,
  		datasourceId: selectedDatasourceId.value,
  		namespace: isPodDetailPanel(panel) ? podResourceNamespace.value : '',
  		startAt,
  		endAt,
  		stepSeconds: Math.max(15, Math.ceil(rangeSeconds / 120))
  	}
  }

  function panelCacheKey(panel) {
    const { startAt, endAt } = currentTimeBounds()
    const rangeKey = timeRangeOption.value === 'custom' ? `${startAt}-${endAt}` : timeRangeSeconds.value
    return `${selectedDatasourceId.value}:${rangeKey}:${panel.id}:${isPodDetailPanel(panel) ? podResourceNamespace.value : ''}`
  }

  async function loadPanel(panel, { force = true, version = panelRefreshVersion } = {}) {
    if (disposed) return
    for (const [cacheKey, entry] of panelResultCache) {
      if (entry.expiresAt <= Date.now()) panelResultCache.delete(cacheKey)
    }
    const key = panelCacheKey(panel)
    const cached = panelResultCache.get(key)
    if (!force && cached && cached.expiresAt > Date.now()) {
      panelResults[panel.id] = cached.data
      return
    }

    const request = (requestVersions.get(panel.id) || 0) + 1
    requestVersions.set(panel.id, request)
    const current = () => version === panelRefreshVersion && request === requestVersions.get(panel.id)
    const settle = () => { if (current()) delete panelPending[panel.id] }
    panelPending[panel.id] = true

    if (!await acquire(version)) {
      settle()
      return
    }
    // Only one request per panel may be in flight; a newer query for the same
    // panel makes the previous response useless, so free the connection.
    cancelPanelRequest(panel.id)
    const controller = typeof AbortController === 'function' ? new AbortController() : null
    if (controller) requestControllers.set(panel.id, controller)
    try {
      if (!current()) return
      const data = await queryMonitorDashboardPanel(panelQueryPayload(panel), controller ? { signal: controller.signal } : undefined)
      if (!current()) return
      panelResults[panel.id] = data
      lastRefreshAt.value = new Date()
      panelResultCache.delete(key)
      panelResultCache.set(key, { data, expiresAt: Date.now() + PANEL_CACHE_TTL })
      while (panelResultCache.size > CACHE_LIMIT) panelResultCache.delete(panelResultCache.keys().next().value)
    } catch (error) {
      // A cancelled request is not a failure: never surface it as a panel error.
      if (abortError(error) || !current()) return
      panelResults[panel.id] = { error: error.message || 'query failed' }
    } finally {
      if (controller && requestControllers.get(panel.id) === controller) requestControllers.delete(panel.id)
      release()
      settle()
    }
  }

  async function runPanelQueue(items, version, force) {
    let cursor = 0
    const worker = async () => {
      while (cursor < items.length && version === panelRefreshVersion) {
        const panel = items[cursor]
        cursor += 1
        await loadPanel(panel, { force, version })
      }
    }
    await Promise.all(Array.from({ length: Math.min(PANEL_QUERY_CONCURRENCY, items.length) }, worker))
  }

  async function refreshAllPanels({ progressive = false, force = true } = {}) {
    if (disposed) return
    if (!selectedDatasourceId.value) {
      const message = '请先在数据源管理中配置 Prometheus 或 VictoriaMetrics 数据源'
      for (const panel of panels.value.filter((item) => item.status === 1)) panelResults[panel.id] = { error: message }
      return
    }
    const enabledPanels = panels.value.filter((item) => item.status === 1)
    invalidateQueries()
    const version = panelRefreshVersion
    const firstScreenPanels = enabledPanels.slice(0, 6)
    const remainingPanels = enabledPanels.slice(6)
    const loadRemaining = async () => {
      await runPanelQueue(remainingPanels, version, force)
    }

    if (progressive) {
      await runPanelQueue(firstScreenPanels, version, force)
      if (version === panelRefreshVersion && remainingPanels.length) {
        remainingTimer = window.setTimeout(() => { remainingTimer = null; void loadRemaining() }, 0)
      }
      return
    }

    await runPanelQueue(firstScreenPanels, version, force)
    if (version === panelRefreshVersion) await loadRemaining()
  }

  async function handleDatasourceChange() {
    if (!isListLayout.value && activePanels.value.length) {
      await refreshAllPanels()
    }
  }

  return {
    invalidateQueries,
    panelResults,
    panelPending,
    isRefreshing,
    syncStatus,
    lastRefreshAt,
    podResourceNamespace,
    lastRefreshText,
    handlePodNamespaceChange,
    refreshPanel,
    refreshProblemPanels,
    panelQueryPayload,
    panelCacheKey,
    loadPanel,
    runPanelQueue,
    refreshAllPanels,
    handleDatasourceChange,
  }
}
