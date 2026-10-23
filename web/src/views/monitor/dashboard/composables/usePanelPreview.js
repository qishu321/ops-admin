import { computed, onBeforeUnmount, reactive, watch } from 'vue'
import { queryMonitorPrometheusRange } from '../../../../api/monitor'

// The dialog renders the real dashboard chart components, and those look the
// preview result up by panel id. Keep one reserved id that can never collide
// with a persisted panel row.
export const PREVIEW_PANEL_ID = '__panel_preview__'
const PREVIEW_DEBOUNCE = 650
// Same sample budget the dashboard uses for range queries, so a preview curve
// has the same resolution as the saved panel.
const PREVIEW_MAX_POINTS = 120
const PREVIEW_MIN_STEP = 15
// These two titles are special-cased by the backend and by TableChart into
// dedicated resource tables that need a saved panel row. A draft keeps the
// PromQL textarea in charge, so the preview must not reuse those titles.
const BACKEND_SPECIAL_TITLES = new Set(['Pod 资源明细', '主机信息'])
const PREVIEW_PLACEHOLDER_TITLE = '未命名面板'

function isCanceled(error) {
  return error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError' || error?.name === 'AbortError'
}

/**
 * Prometheus answers a bad query with a JSON envelope the operator should not
 * have to read. Keep the machine detail, but lead with something actionable.
 */
export function describePreviewError(error) {
  const raw = String(error?.message || '').trim() || '预览查询失败'
  let detail = raw
  let errorType = ''
  const body = raw.match(/\{[\s\S]*\}/)
  if (body) {
    try {
      const payload = JSON.parse(body[0])
      if (payload?.error) detail = String(payload.error)
      if (payload?.errorType) errorType = String(payload.errorType)
    } catch {
      // Not JSON: keep the raw message.
    }
  }
  const haystack = `${errorType} ${detail}`.toLowerCase()
  if (haystack.includes('parse error') || haystack.includes('bad_data')) {
    return { title: 'PromQL 语法有误', detail }
  }
  if (haystack.includes('timeout') || haystack.includes('deadline exceeded')) {
    return { title: '查询超时', detail: '缩小时间范围或简化 PromQL 后重试' }
  }
  if (haystack.includes('connection refused') || haystack.includes('no such host') || haystack.includes('dial tcp')) {
    return { title: '数据源连接失败', detail }
  }
  return { title: '预览查询失败', detail }
}

/**
 * Live PromQL preview for the create/edit panel dialog.
 *
 * Uses the range-query endpoint instead of the panel endpoint because an
 * unsaved panel has no row to look up. Results are written into the shared
 * panel result map under PREVIEW_PANEL_ID so PanelContent can render them with
 * zero duplicated chart code.
 */
export function usePanelPreview({ panelForm, panelDialogVisible, panelResults, isListLayout, currentTimeBounds, rangeLabel }) {
  const previewState = reactive({
    status: 'idle',
    message: '填写 PromQL 后自动预览',
    series: 0,
    error: '',
    errorDetail: '',
    refreshing: false
  })

  const previewPanel = computed(() => ({
    id: PREVIEW_PANEL_ID,
    title: BACKEND_SPECIAL_TITLES.has(panelForm.title) ? PREVIEW_PLACEHOLDER_TITLE : panelForm.title || PREVIEW_PLACEHOLDER_TITLE,
    unit: panelForm.unit || '',
    chartType: panelForm.chartType || 'stat',
    promql: panelForm.promql || '',
    status: 1
  }))

  const previewRangeText = computed(() => (typeof rangeLabel === 'function' ? rangeLabel() : ''))
  const previewSeriesText = computed(() => (previewState.series ? `${previewState.series} 条序列` : ''))

  let debounceTimer = null
  let controller = null
  let requestSeq = 0
  let lastSignature = ''

  function clearResult() {
    delete panelResults[PREVIEW_PANEL_ID]
  }

  function abortInFlight() {
    if (!controller) return
    const current = controller
    controller = null
    current.abort()
  }

  function clearTimer() {
    if (!debounceTimer) return
    clearTimeout(debounceTimer)
    debounceTimer = null
  }

  // A dialog that opens with a pre-filled panel fires both the form watcher and
  // the visibility watcher; the signature keeps that from becoming two queries.
  function signature() {
    return `${panelForm.datasourceId}|${String(panelForm.promql || '').trim()}`
  }

  function settle(status, message = '') {
    requestSeq++
    clearTimer()
    abortInFlight()
    clearResult()
    previewState.status = status
    previewState.message = message
    previewState.error = ''
    previewState.errorDetail = ''
    previewState.series = 0
    previewState.refreshing = false
  }

  function resetPreview() {
    settle('idle', '填写 PromQL 后自动预览')
  }

  function guarded() {
    return !isListLayout.value && panelDialogVisible.value
  }

  function pendingMessage() {
    if (!String(panelForm.promql || '').trim()) return '填写 PromQL 后自动预览'
    return '请选择数据源'
  }

  async function runPreview() {
    if (!guarded()) return
    const query = String(panelForm.promql || '').trim()
    if (!query || !panelForm.datasourceId) {
      settle('idle', pendingMessage())
      return
    }
    clearTimer()
    lastSignature = signature()
    requestSeq++
    const seq = requestSeq
    abortInFlight()
    const local = typeof AbortController === 'function' ? new AbortController() : null
    controller = local

    const { startAt, endAt } = currentTimeBounds()
    const rangeSeconds = Math.max(1, endAt - startAt)
    const stepSeconds = Math.max(PREVIEW_MIN_STEP, Math.ceil(rangeSeconds / PREVIEW_MAX_POINTS))

    previewState.refreshing = true
    if (previewState.status !== 'ready') {
      previewState.status = 'loading'
      previewState.message = '正在获取预览数据…'
    }
    try {
      const data = await queryMonitorPrometheusRange(
        { datasourceId: panelForm.datasourceId, query, startAt, endAt, stepSeconds },
        // Silent: a half-typed PromQL must not spam global error toasts while
        // the dialog already shows the failure inline.
        { ...(local ? { signal: local.signal } : {}), silent: true }
      )
      if (seq !== requestSeq) return
      const rows = Array.isArray(data?.result) ? data.result : []
      if (!rows.length) {
        clearResult()
        previewState.status = 'empty'
        previewState.message = '当前时间范围内没有数据'
        previewState.series = 0
        previewState.error = ''
        previewState.errorDetail = ''
        return
      }
      panelResults[PREVIEW_PANEL_ID] = { resultType: data?.resultType, result: rows }
      previewState.status = 'ready'
      previewState.message = ''
      previewState.error = ''
      previewState.errorDetail = ''
      previewState.series = rows.length
    } catch (error) {
      // A superseded or aborted query keeps whatever the newer one produces.
      if (seq !== requestSeq || isCanceled(error)) return
      clearResult()
      const described = describePreviewError(error)
      previewState.status = 'error'
      previewState.message = ''
      previewState.error = described.title
      previewState.errorDetail = described.detail
      previewState.series = 0
    } finally {
      if (seq === requestSeq) {
        previewState.refreshing = false
        if (controller === local) controller = null
      }
    }
  }

  function schedulePreview() {
    if (!guarded()) return
    if (!String(panelForm.promql || '').trim() || !panelForm.datasourceId) {
      settle('idle', pendingMessage())
      return
    }
    clearTimer()
    debounceTimer = window.setTimeout(() => {
      debounceTimer = null
      if (signature() === lastSignature && previewState.status === 'ready') return
      void runPreview()
    }, PREVIEW_DEBOUNCE)
  }

  watch(() => [panelForm.promql, panelForm.datasourceId], schedulePreview)

  watch(panelDialogVisible, (visible) => {
    if (!visible) {
      settle('idle', '填写 PromQL 后自动预览')
      return
    }
    if (String(panelForm.promql || '').trim() && panelForm.datasourceId) void runPreview()
    else resetPreview()
  })

  onBeforeUnmount(() => {
    requestSeq++
    clearTimer()
    abortInFlight()
    clearResult()
  })

  return {
    previewPanel,
    previewState,
    previewRangeText,
    previewSeriesText,
    runPreview,
    schedulePreview,
    resetPreview
  }
}
