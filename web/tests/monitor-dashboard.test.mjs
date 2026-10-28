import assert from 'node:assert/strict'
import test from 'node:test'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'
import { computed, reactive, ref, watch } from 'vue'
import { smoothPath, areaPath } from '../src/views/monitor/dashboard/utils/curve.js'
import { formatByUnit, numberValue, metricName, escapeHtml, reducerLabel, formatReportTime } from '../src/views/monitor/dashboard/utils/format.js'

// Match the project's existing VM harness: stub the network boundary, execute real composables.
function load(name, dependencies = {}, expression = name) {
  const source = readFileSync(new URL(`../src/views/monitor/dashboard/composables/${name}.js`, import.meta.url), 'utf8')
    .replace(/^import .*$/gm, '').replaceAll('export ', '')
  return vm.runInNewContext(`${source}; ${expression}`, {
    ref, reactive, computed, watch, onMounted() {}, onBeforeUnmount() {}, AbortController,
    setTimeout, clearTimeout,
    window: { setTimeout, clearTimeout, requestAnimationFrame: callback => setTimeout(callback, 0), cancelAnimationFrame: clearTimeout },
    ElMessage: { success() {}, warning() {}, error() {} }, ...dependencies,
  })
}

function queryFixture(request, count = 9) {
  const panels = ref(Array.from({ length: count }, (_, i) => ({ id: i + 1, status: 1 })))
  const selectedDatasourceId = ref(7)
  const timeRangeSeconds = ref(3600)
  const query = load('usePanelQueries', { queryMonitorDashboardPanel: request })({
    panels, activePanels: panels, selectedDatasourceId,
    panelStateKey: () => 'healthy', isListLayout: ref(false),
    currentTimeBounds: () => ({ startAt: 100, endAt: 3700 }),
    isPodDetailPanel: panel => panel.title === 'Pod 资源明细',
    timeRangeOption: timeRangeSeconds, timeRangeSeconds,
  })
  return { query, panels, selectedDatasourceId, timeRangeSeconds }
}

test('query queue bounds concurrency at four and renders every enabled panel', async () => {
  let active = 0, maximum = 0
  const { query } = queryFixture(async ({ id }) => {
    maximum = Math.max(maximum, ++active)
    await new Promise(resolve => setTimeout(resolve, 1))
    active--
    return { result: [{ value: [100, String(id)] }] }
  })
  await query.refreshAllPanels()
  assert.equal(maximum, 4)
  assert.equal(Object.keys(query.panelResults).length, 9)
  assert.equal(query.isRefreshing.value, false)
  assert.equal(query.syncStatus.value.type, 'success')
})

test('cached queries are separated by source, time range and Pod namespace; force bypasses cache', async () => {
  const requests = []
  const fixture = queryFixture(async payload => { requests.push(payload); return { result: [] } }, 1)
  const { query, selectedDatasourceId, timeRangeSeconds } = fixture
  const panel = { id: 1, title: 'Pod 资源明细' }
  await query.loadPanel(panel, { force: false })
  await query.loadPanel(panel, { force: false })
  assert.equal(requests.length, 1)
  query.podResourceNamespace.value = 'qa'
  await query.loadPanel(panel, { force: false })
  selectedDatasourceId.value = 8
  await query.loadPanel(panel, { force: false })
  timeRangeSeconds.value = 900
  await query.loadPanel(panel, { force: false })
  await query.loadPanel(panel, { force: true })
  assert.equal(requests.length, 5)
  assert.equal(requests[1].namespace, 'qa')
  assert.equal(requests[2].datasourceId, 8)
})

test('late results from an earlier refresh cannot replace the new result', async () => {
  const resolvers = []
  const { query } = queryFixture(() => new Promise(resolve => resolvers.push(resolve)), 1)
  const old = query.refreshAllPanels()
  await new Promise(resolve => setTimeout(resolve, 0))
  const latest = query.refreshAllPanels()
  await new Promise(resolve => setTimeout(resolve, 0))
  resolvers[1]({ result: [{ value: [1, 'new'] }] })
  await latest
  resolvers[0]({ result: [{ value: [1, 'old'] }] })
  await old
  assert.equal(query.panelResults[1].result[0].value[1], 'new')
  assert.equal(query.isRefreshing.value, false)
})

test('query errors leave a retryable result and clear loading', async () => {
  const { query } = queryFixture(async () => { throw Error('unavailable') }, 1)
  await query.refreshAllPanels()
  assert.equal(query.panelResults[1].error, 'unavailable')
  assert.equal(query.syncStatus.value.type, 'error')
  assert.equal(query.isRefreshing.value, false)
})

test('custom inspection range does not trigger live queries', () => {
  let refreshes = 0
  const time = load('useTimeRange')({ isListLayout: ref(true), refreshAllPanels: () => refreshes++ })
  time.timeRangeOption.value = 'custom'
  time.customTimeRange.value = [new Date(100000), new Date(200000)]
  assert.equal(time.currentTimeBounds().startAt, 100)
  assert.equal(time.currentTimeBounds().endAt, 200)
  time.handleTimeRangeChange(86400)
  assert.equal(refreshes, 0)
})

test('grid repairs overlaps, bounds width and persists keyboard resize payloads', async () => {
  const saves = []
  const panels = ref([
    { id: 1, dashboardId: 3, title: 'one', chartType: 'line', gridX: 0, gridY: 0, gridW: 12, gridH: 9, sort: 1 },
    { id: 2, dashboardId: 3, title: 'two', chartType: 'bar', gridX: 0, gridY: 0, gridW: 12, gridH: 9, sort: 2 },
  ])
  const grid = load('useGridLayout', gridDependencies(async data => saves.push(data)))({
    isCustomGridLayout: ref(true), isFullscreen: ref(false), visualPanels: panels, activeDashboard: ref({ id: 3 }),
  })
  grid.layoutEditing.value = true
  grid.initializePanelGrid()
  assert.equal(grid.gridRectsOverlap(grid.panelGridRect(panels.value[0]), grid.panelGridRect(panels.value[1])), false)
  assert.equal(grid.normalizeGridWidth(100), 24)
  assert.equal(grid.normalizeGridWidth(1), 6)
  await grid.resizePanelWithKeyboard({ key: 'ArrowRight', shiftKey: true, preventDefault() {} }, panels.value[0])
  assert.equal(saves[0].gridW, 16)
  assert.equal(saves[0].span, 16)
  assert.equal(saves.length, 2)
  assert.equal(grid.gridRectsOverlap(grid.panelGridRect(panels.value[0]), grid.panelGridRect(panels.value[1])), false)
  assert.equal(grid.layoutSaving.value, false)
})

test('curve interpolates samples and its control points remain inside each segment', () => {
  const points = '0,40 20,10 40,30 60,30 80,5 100,48'
  const path = smoothPath(points)
  assert.ok(path.startsWith('M 0 40'))
  const samples = points.split(' ').map(pair => pair.split(',').map(Number))
  const segments = [...path.matchAll(/C ([^C]+)/g)]
  assert.equal(segments.length, samples.length - 1)
  segments.forEach((segment, i) => {
    const values = segment[1].trim().split(/[ ,]+/).map(Number)
    const low = Math.min(samples[i][1], samples[i + 1][1])
    const high = Math.max(samples[i][1], samples[i + 1][1])
    assert.ok(values[1] >= low && values[1] <= high)
    assert.ok(values[3] >= low && values[3] <= high)
    assert.deepEqual(values.slice(-2), samples[i + 1])
  })
  assert.equal(smoothPath(''), '')
  assert.equal(smoothPath('0,NaN'), '')
  assert.ok(areaPath(points).endsWith('L 100 52 L 0 52 Z'))
})

test('resource detail rows count as loaded data and an empty metric is not a zero', () => {
  const panelResults = reactive({ 1: { result: [], hostResources: [{ instance: 'test' }] }, 2: { result: [] } })
  const display = load('usePanelDisplay', { formatByUnit, numberValue })({
    activePanels: ref([]), panelResults, activeDashboard: ref({}), panelVisualType: () => 'stat', panelTrend: () => [], barRows: () => [],
  })
  assert.equal(display.panelResultCount({ id: 1 }), 1)
  assert.equal(display.panelState({ id: 1, status: 1 }), '实时')
  assert.equal(display.panelDisplayValue({ id: 2 }), '—')
})

test('embedded fullscreen fallback exits with Escape and restores scroll on disposal', async () => {
  const handlers = new Map()
  let dispose, focused = false
  const document = {
    body: { style: { overflow: 'auto' } },
    activeElement: { focus: () => { focused = true } },
    querySelector: () => ({}),
    addEventListener: (name, handler) => handlers.set(name, handler),
    removeEventListener: name => handlers.delete(name),
  }
  const fullscreen = load('useFullscreen', { document, onMounted: fn => fn(), onBeforeUnmount: fn => { dispose = fn } })()
  await fullscreen.toggleFullscreen()
  assert.equal(fullscreen.isFullscreen.value, true)
  assert.equal(document.body.style.overflow, 'hidden')
  handlers.get('keydown')({ key: 'Escape', target: { closest: selector => selector === '[role="dialog"]' ? {} : null } })
  assert.equal(fullscreen.isFullscreen.value, true, 'Escape in a result drawer must not exit fallback presentation')
  handlers.get('keydown')({ key: 'Escape', defaultPrevented: true })
  assert.equal(fullscreen.isFullscreen.value, true, 'an already handled Escape belongs to the focused control')
  handlers.get('keydown')({ key: 'Escape' })
  assert.equal(fullscreen.isFullscreen.value, false)
  assert.equal(document.body.style.overflow, 'auto')
  assert.equal(focused, true)
  await fullscreen.toggleFullscreen()
  dispose()
  assert.equal(document.body.style.overflow, 'auto')
  assert.equal(handlers.size, 0)
})

test('native fullscreen still calls the browser enter and exit APIs', async () => {
  const handlers = new Map()
  let enters = 0, exits = 0
  const document = {
    fullscreenElement: null,
    querySelector: () => ({ requestFullscreen: async () => { enters++; document.fullscreenElement = {}; handlers.get('fullscreenchange')() } }),
    exitFullscreen: async () => { exits++; document.fullscreenElement = null; handlers.get('fullscreenchange')() },
    addEventListener: (name, fn) => handlers.set(name, fn),
    removeEventListener() {},
  }
  const fullscreen = load('useFullscreen', { document, onMounted: fn => fn(), onBeforeUnmount() {} })()
  await fullscreen.toggleFullscreen()
  assert.equal(fullscreen.isFullscreen.value, true)
  await fullscreen.toggleFullscreen()
  assert.equal(fullscreen.isFullscreen.value, false)
  assert.equal(enters, 1)
  assert.equal(exits, 1)
})

test('inspection print export uses persisted results and escapes report text', () => {
  let html = '', printed = 0
  const report = load('useInspectionExport', {
    escapeHtml, reducerLabel, formatReportTime, setTimeout: fn => fn(),
    window: { open: () => ({
      document: { write: value => { html = value }, close() {} },
      focus() {}, print: () => printed++,
    }) },
  })({
    inspectionReport: ref({ id: 5, dashboardName: 'local <test>', totalCount: 1 }),
    inspectionReportResults: ref([{ title: '<script>alert(1)</script>', status: 'healthy', valueText: '42', seriesCount: 1, promql: 'vector(42)' }]),
  })
  report.exportInspectionReportPdf()
  assert.match(html, /巡检报告 #5/)
  assert.match(html, /vector\(42\)/)
  assert.match(html, /&lt;script&gt;/)
  assert.ok(!html.includes('<script>alert'))
  assert.equal(printed, 1)
})

test('cancelled panel queries settle without surfacing an error', async () => {
  let aborted = 0
  const { query } = queryFixture((payload, config) => new Promise((resolve, reject) => {
    if (!config?.signal) return resolve({ result: [] })
    config.signal.addEventListener('abort', () => {
      aborted++
      const error = new Error('canceled')
      error.name = 'CanceledError'
      error.code = 'ERR_CANCELED'
      reject(error)
    })
  }), 1)
  const pending = query.loadPanel({ id: 1 }, { force: true })
  await new Promise(resolve => setTimeout(resolve, 0))
  query.invalidateQueries()
  await pending
  assert.equal(aborted, 1)
  assert.equal(query.panelResults[1], undefined)
  assert.deepEqual(Object.keys(query.panelPending), [])
})

test('layout saves issued while a write is running merge into one follow-up', async () => {
  const writes = []
  let release
  const persistence = load('useLayoutPersistence', {
    saveMonitorDashboardPanel: async panel => {
      writes.push(panel.gridX)
      if (writes.length === 1) await new Promise(resolve => { release = resolve })
    },
    saveMonitorDashboard: async () => {},
  })()
  const dashboard = { id: 3 }
  const before = [{ id: 1, gridX: -1 }]
  const panels = [{ id: 1, gridX: 0 }]
  const first = persistence.persist({ panels, before, dashboard })
  await new Promise(resolve => setTimeout(resolve, 0))
  panels[0].gridX = 5
  const second = persistence.persist({ panels, before, dashboard })
  const third = persistence.persist({ panels, before, dashboard })
  await new Promise(resolve => setTimeout(resolve, 0))
  release()
  assert.deepEqual(await Promise.all([first, second, third]), [true, true, true])
  // Two writes: the running one plus a single merged follow-up, not three.
  assert.deepEqual(writes, [0, 5])
  assert.equal(persistence.layoutSaving.value, false)
  assert.equal(persistence.layoutSaveState.value, 'saved')
})

function gridDependencies(save, dashboardSave = async () => {}) {
  const persistence = load('useLayoutPersistence', { saveMonitorDashboardPanel: save, saveMonitorDashboard: dashboardSave }, '({ useLayoutPersistence, snapshotLayout, restoreLayout })')
  return { ...persistence, useGridGeometry: load('useGridGeometry') }
}
function gridFixture(save = async () => {}, dashboardSave = async () => {}) {
  const panels = ref([
    { id: 1, dashboardId: 3, title: 'a', gridX: 0, gridY: 0, gridW: 12, gridH: 9, span: 12, sort: 1 },
    { id: 2, dashboardId: 3, title: 'b', gridX: 12, gridY: 0, gridW: 12, gridH: 9, span: 12, sort: 2 },
  ])
  const dashboard = ref({ id: 3, layout: 'grid-custom' })
  const grid = load('useGridLayout', gridDependencies(save, dashboardSave))({ isCustomGridLayout: computed(() => dashboard.value.layout === 'grid-custom'), isFullscreen: ref(false), visualPanels: panels, activeDashboard: dashboard })
  grid.layoutEditing.value = true
  const event = (key, extra = {}) => ({ key, preventDefault() {}, ...extra })
  return { grid, panels, dashboard, event }
}
test('failed layout save waits for late writes then compensates every panel', async () => {
  const order = []
  let count = 0
  const fixture = gridFixture(async panel => {
    const attempt = ++count
    if (attempt === 1) { order.push('failed'); throw new Error('offline') }
    if (attempt === 2) await new Promise(resolve => setTimeout(resolve, 8))
    order.push(attempt <= 2 ? 'late' : 'restore')
  })
  const before = JSON.stringify(fixture.panels.value)
  await fixture.grid.resizePanelWithKeyboard(fixture.event('ArrowRight'), fixture.panels.value[0])
  assert.equal(JSON.stringify(fixture.panels.value), before)
  assert.deepEqual(order, ['failed', 'late', 'restore', 'restore'])
  assert.equal(fixture.grid.layoutSaveState.value, 'error')
  assert.equal(fixture.grid.layoutRecovery.value, null)
})
test('unrecoverable save locks edits until explicit recovery succeeds', async () => {
  let offline = true, writes = 0
  const { grid, panels, event } = gridFixture(async () => { writes++; if (offline) throw new Error('offline') })
  await grid.resizePanelWithKeyboard(event('ArrowRight'), panels.value[0])
  assert.ok(grid.layoutRecovery.value)
  const previousWrites = writes
  await grid.movePanelWithKeyboard(event('ArrowDown'), panels.value[0])
  assert.equal(writes, previousWrites)
  offline = false
  await grid.retryLayoutRecovery()
  assert.equal(grid.layoutLocked.value, false)
})
test('saving blocks overlapping gestures and leaving edit does not persist a preview', async () => {
  let finish
  const { grid, panels, event } = gridFixture(() => new Promise(resolve => { finish = resolve }))
  const save = grid.movePanelWithKeyboard(event('ArrowDown'), panels.value[1])
  await grid.resizePanelWithKeyboard(event('ArrowLeft'), panels.value[0])
  assert.equal(panels.value[0].gridW, 12)
  finish()
  await save
  let captured = false
  const handle = { closest: () => ({ getBoundingClientRect: () => ({ left: 0, top: 0, width: 975 }) }), setPointerCapture() { captured = true }, hasPointerCapture: () => captured, releasePointerCapture() { captured = false } }
  grid.startPanelInteraction({ button: 0, pointerId: 5, clientX: 0, clientY: 0, currentTarget: handle, preventDefault() {} }, panels.value[0], 'move')
  grid.updatePanelInteraction({ pointerId: 5, clientX: 80, clientY: 80 })
  await new Promise(resolve => setTimeout(resolve, 2))
  assert.equal(grid.layoutInteraction.value.target.x, 2)
  assert.equal(panels.value[0].gridX, 0)
  grid.exitLayoutEditing()
  assert.equal(grid.layoutInteraction.value, null)
  assert.equal(panels.value[0].gridX, 0)
  assert.equal(captured, false)
})
test('resize preview keeps its top-left anchor and clamps minimum and right edge', async () => {
  const { grid, panels } = gridFixture()
  const handle = { closest: () => ({ getBoundingClientRect: () => ({ left: 0, top: 0, width: 975 }) }) }
  grid.startPanelInteraction({ button: 0, pointerId: 7, clientX: 0, clientY: 0, currentTarget: handle, preventDefault() {} }, panels.value[1], 'resize')
  await grid.finishPanelInteraction({ pointerId: 7, clientX: -9999, clientY: -9999 })
  assert.deepEqual(JSON.parse(JSON.stringify(grid.panelGridRect(panels.value[1]))), { x: 12, y: 0, w: 6, h: 7 })
  assert.equal(grid.normalizeGridWidth(NaN), 12)
  assert.equal(grid.normalizeGridHeight(Infinity), 9)
})
test('overlapping refresh waves and manual queries share one four-request limit', async () => {
  let active = 0, maximum = 0
  const { query, panels } = queryFixture(async () => {
    maximum = Math.max(maximum, ++active)
    await new Promise(resolve => setTimeout(resolve, 2))
    active--
    return { result: [] }
  }, 12)
  const first = query.refreshAllPanels()
  await new Promise(resolve => setTimeout(resolve, 1))
  await Promise.all([first, query.refreshAllPanels(), query.refreshPanel(panels.value[0])])
  assert.equal(maximum, 4)
  assert.equal(Object.keys(query.panelPending).length, 0)
})

function trendFixture(rows, panel = { id: 1, title: '请求量', unit: '%' }, k8s = false) {
  const trends = load('useTrendChart', {
    hostAggregateTrendModes: new Map(), lineColors: ['#111111', '#222222', '#333333'],
    numberValue, metricName, formatByUnit,
  })({
    isHostDashboard: ref(false), panelRows: () => rows, isK8sDashboard: ref(k8s), rangeEndText: ref('当前'),
  })
  return { trends, panel }
}
const sampleRow = (name, values, startAt = 0, step = 60) => ({
  metric: { __name__: name },
  values: values.map((value, index) => [startAt + index * step, String(value)]),
})

test('irregular samples use elapsed time and tooltip does not bridge missing data', () => {
  const { trends, panel } = trendFixture([{ metric: { __name__: 'a' }, values: [[0, '1'], [60, '2'], [660, '3'], [720, '4']] }])
  const series = trends.panelLineSeries(panel)
  assert.ok(Math.abs(Number(series[0].points.split(' ')[1].split(',')[0]) - 100 / 12) < 0.001)
  const container = { getBoundingClientRect: () => ({ left: 0, width: 100 }) }
  trends.handleTrendPointer(panel, { currentTarget: container, clientX: 50 }, series)
  assert.equal(trends.trendTooltips[panel.id].items[0].value, '-')
})

test('failed query never reports a successful refresh time', async () => {
  const { query } = queryFixture(async () => { throw Error('offline') }, 1)
  assert.equal(query.lastRefreshAt.value, null)
  await query.refreshAllPanels()
  assert.equal(query.lastRefreshAt.value, null)
})

test('unmount cancels in-flight queries and prevents queued or later work', async () => {
  let dispose, calls = 0, signal
  const panels = ref([{ id: 1, status: 1 }])
  const query = load('usePanelQueries', {
    onBeforeUnmount: callback => { dispose = callback },
    queryMonitorDashboardPanel: (_, config) => {
      calls++
      signal = config.signal
      return new Promise((resolve, reject) => signal.addEventListener('abort', () => reject(Object.assign(Error('cancelled'), { name: 'AbortError' }))))
    },
  })({ panels, activePanels: panels, selectedDatasourceId: ref(1), panelStateKey: () => 'healthy', isListLayout: ref(false), currentTimeBounds: () => ({ startAt: 0, endAt: 100 }), isPodDetailPanel: () => false, timeRangeOption: ref(100), timeRangeSeconds: ref(100) })
  const pending = query.refreshAllPanels()
  await new Promise(resolve => setTimeout(resolve, 0))
  dispose()
  await pending
  await query.refreshAllPanels()
  assert.equal(signal.aborted, true)
  assert.equal(calls, 1)
  assert.equal(query.isRefreshing.value, false)
})

test('dashboard detail failure is explicit and retry restores the requested dashboard', async () => {
  let fail = true
  const activeDashboard = ref(null), loading = ref(false)
  const loader = load('useDashboardLoader', {
    queryMonitorDashboardList: async () => ({ list: [{ id: 1 }] }),
    queryMonitorDatasourceOptions: async () => [{ id: 1, type: 'prometheus' }],
    monitorDashboardInfo: async id => { if (fail) throw Error('offline'); return { dashboard: { id }, panels: [] } },
  })({ dashboards: ref([]), datasourceOptions: ref([]), selectedDatasourceId: ref(1), visibleDashboards: ref([{ id: 1 }]), activeDashboardId: ref(1), activeDashboard, panels: ref([]), inspectionReport: ref(null), inspectionReportResults: ref([]), loading, inspectionFilter: ref('all'), layoutNeedsMigration: ref(false), preparePanelGrid() {}, exitLayoutEditing() {}, layoutLocked: ref(false), invalidateQueries() {}, panelResults: {}, panelPending: {}, isListLayout: ref(false), refreshAllPanels() {} })
  await loader.loadDashboard(1)
  assert.equal(loader.loadError.value, 'offline')
  assert.equal(loading.value, false)
  fail = false
  await loader.retryLoad()
  assert.equal(loader.loadError.value, '')
  assert.equal(activeDashboard.value.id, 1)
})

test('bounded query cache evicts old ranges instead of retaining every visited window', async () => {
  let calls = 0
  const { query, timeRangeSeconds } = queryFixture(async () => { calls++; return { result: [] } }, 1)
  for (let range = 1; range <= 129; range++) {
    timeRangeSeconds.value = range
    await query.loadPanel({ id: 1 }, { force: false })
  }
  timeRangeSeconds.value = 1
  await query.loadPanel({ id: 1 }, { force: false })
  assert.equal(calls, 130)
})

test('series cap stops reading excess rows and prepared tooltip never rebuilds series', () => {
  const rows = Array.from({ length: 8 }, (_, index) => sampleRow(String(index), [1, 2, 3]))
  rows.push({ get values() { throw Error('unnecessary high-cardinality scan') } })
  const { trends, panel } = trendFixture(rows)
  const series = trends.panelLineSeries(panel)
  assert.equal(series.length, 8)
  rows.splice(0, rows.length, { get values() { throw Error('tooltip rebuilt data') } })
  trends.handleTrendPointer(panel, { currentTarget: { getBoundingClientRect: () => ({ left: 0, width: 100 }) }, clientX: 50 }, series)
  assert.equal(trends.trendTooltips[panel.id].items[0].value, '2%')
})

test('a sampling hole splits the trend line instead of bridging it', () => {
  const { trends, panel } = trendFixture([
    sampleRow('a', [10, 20, 30]),
    sampleRow('b', [10, 20, 30, 40, 50], 0, 60),
  ])
  // Give series b a 10 minute hole after the second sample.
  const rows = [{ metric: { __name__: 'b' }, values: [[0, '10'], [60, '20'], [660, '40'], [720, '50']] }]
  const segmented = trendFixture(rows).trends
  const [first] = segmented.panelLineSeries({ id: 2, title: '请求量', unit: '' })
  assert.equal(first.segmentPoints.length, 2)
  assert.equal(first.segmentPoints[0].split(' ').length, 2)
  assert.equal(first.segmentPoints[1].split(' ').length, 2)
  // Dense series stay whole.
  const [dense] = trends.panelLineSeries(panel)
  assert.equal(dense.segmentPoints.length, 1)
})

test('hiding a legend entry drops the series but keeps every colour stable', () => {
  const { trends, panel } = trendFixture([
    sampleRow('a', [10, 20]), sampleRow('b', [30, 40]), sampleRow('c', [50, 60]),
  ])
  const before = trends.panelLegendSeries(panel)
  assert.deepEqual(Array.from(before, item => item.hidden), [false, false, false])
  assert.equal(trends.panelLineSeries(panel).length, 3)
  trends.toggleTrendSeries(panel, 'a')
  const after = trends.panelLegendSeries(panel)
  assert.equal(after[0].hidden, true)
  assert.deepEqual(Array.from(after, item => item.color), Array.from(before, item => item.color))
  const visible = trends.panelLineSeries(panel)
  assert.deepEqual(Array.from(visible, item => item.name), ['b', 'c'])
  // Colours follow the legend position, not the surviving order.
  assert.equal(visible[0].color, before[1].color)
  trends.toggleTrendSeries(panel, 'a')
  assert.equal(trends.panelLineSeries(panel).length, 3)
})

test('percentage panels keep an absolute 0-100 scale after hiding a series', () => {
  const { trends, panel } = trendFixture([
    sampleRow('high', [90, 95]), sampleRow('low', [10, 12]),
  ])
  const points = trends.panelLineSeries(panel)[0].points.split(' ').map(pair => Number(pair.split(',')[1]))
  // 90% maps near the top of the 0-100 band, 95% slightly above it — never to y=48.
  assert.ok(points.every(y => y < 20))
  trends.toggleTrendSeries(panel, 'high')
  const remaining = trends.panelLineSeries(panel)[0].points.split(' ').map(pair => Number(pair.split(',')[1]))
  assert.ok(remaining.every(y => y > 40), 'a low series must stay near the baseline, not stretch to full height')
})

test('tooltip horizontal position is measured against the plotted SVG, not the card', () => {
  const { trends, panel } = trendFixture([
    { metric: { __name__: 'a' }, values: [[0, '1'], [60, '2'], [120, '3'], [180, '4'], [240, '5']] },
  ])
  const container = {
    getBoundingClientRect: () => ({ left: 100, width: 300 }),
    querySelector: () => ({ getBoundingClientRect: () => ({ left: 114, width: 272 }) }),
  }
  trends.handleTrendPointer(panel, { currentTarget: container, clientX: 114 })
  // 14px is the chart padding, i.e. where the plotted SVG starts inside the card.
  assert.equal(trends.trendTooltips[panel.id].x, '14px')
  assert.equal(trends.trendTooltips[panel.id].items[0].value, '1%')
  trends.handleTrendPointer(panel, { currentTarget: container, clientX: 250 })
  // Halfway across the plot is 14px (left padding) + 136px = 150px inside the card.
  assert.equal(trends.trendTooltips[panel.id].x, '150px')
  assert.equal(trends.trendTooltips[panel.id].items[0].value, '3%')
  trends.hideTrendTooltip(panel)
  assert.equal(trends.trendTooltips[panel.id], undefined)
})

function previewFixture(request) {
  const panelForm = reactive({ title: 'CPU 使用率', datasourceId: 3, promql: '', unit: '%', chartType: 'line' })
  const panelDialogVisible = ref(true)
  const panelResults = reactive({})
  const isListLayout = ref(false)
  const preview = load('usePanelPreview', { queryMonitorPrometheusRange: request })({
    panelForm, panelDialogVisible, panelResults, isListLayout,
    currentTimeBounds: () => ({ startAt: 1000, endAt: 4600 }),
    rangeLabel: () => '最近 1 小时',
  })
  return { preview, panelForm, panelDialogVisible, panelResults, isListLayout }
}

test('panel preview queries the draft panel and feeds the real chart components', async () => {
  const requests = []
  const { preview, panelForm, panelResults } = previewFixture(async (payload, config) => {
    requests.push({ payload, config })
    return { resultType: 'matrix', result: [{ metric: { instance: 'a' }, values: [[1, '40']] }] }
  })
  panelForm.promql = ' up '
  await preview.runPreview()
  assert.equal(requests.length, 1)
  assert.equal(requests[0].payload.datasourceId, 3)
  assert.equal(requests[0].payload.query, 'up')
  assert.equal(requests[0].config.silent, true)
  // 3600s range over a 120-point budget yields a 30s step.
  assert.equal(requests[0].payload.stepSeconds, 30)
  assert.equal(requests[0].payload.startAt, 1000)
  assert.equal(preview.previewState.status, 'ready')
  assert.equal(preview.previewState.series, 1)
  assert.equal(preview.previewState.refreshing, false)
  assert.equal(panelResults[preview.previewPanel.value.id].result.length, 1)
  assert.equal(preview.previewPanel.value.chartType, 'line')
  assert.equal(preview.previewRangeText.value, '最近 1 小时')
})

test('panel preview reports empty, error and idle states without leaking a result', async () => {
  let mode = 'empty'
  const { preview, panelForm, panelResults } = previewFixture(async () => {
    if (mode === 'error') throw new Error('bad_data: parse error')
    return { resultType: 'matrix', result: [] }
  })
  panelForm.promql = 'up'
  await preview.runPreview()
  assert.equal(preview.previewState.status, 'empty')
  assert.equal(preview.previewState.message, '当前时间范围内没有数据')
  assert.equal(Object.keys(panelResults).length, 0)

  mode = 'error'
  await preview.runPreview()
  assert.equal(preview.previewState.status, 'error')
  assert.equal(preview.previewState.error, 'PromQL 语法有误')
  assert.equal(preview.previewState.errorDetail, 'bad_data: parse error')
  assert.equal(Object.keys(panelResults).length, 0)

  panelForm.promql = '   '
  await preview.runPreview()
  assert.equal(preview.previewState.status, 'idle')
  assert.equal(preview.previewState.message, '填写 PromQL 后自动预览')
})

test('closing the panel dialog drops the preview state and caches nothing', async () => {
  const { preview, panelForm, panelDialogVisible, panelResults } = previewFixture(async () => ({
    resultType: 'vector', result: [{ metric: { __name__: 'up' }, value: [1, '1'] }],
  }))
  panelForm.promql = 'up'
  await preview.runPreview()
  assert.equal(Object.keys(panelResults).length, 1)
  panelDialogVisible.value = false
  await new Promise(resolve => setTimeout(resolve, 0))
  assert.equal(preview.previewState.status, 'idle')
  assert.equal(Object.keys(panelResults).length, 0)
  // A closed dialog must not query at all.
  await preview.runPreview()
  assert.equal(preview.previewState.status, 'idle')
})

test('a slower earlier preview cannot overwrite the newest preview', async () => {
  const resolvers = []
  const { preview, panelForm, panelResults } = previewFixture(() => new Promise(resolve => resolvers.push(resolve)))
  panelForm.promql = 'first'
  const stale = preview.runPreview()
  await new Promise(resolve => setTimeout(resolve, 0))
  panelForm.promql = 'second'
  const latest = preview.runPreview()
  await new Promise(resolve => setTimeout(resolve, 0))
  resolvers[1]({ resultType: 'vector', result: [{ metric: { __name__: 'b' }, value: [1, '2'] }] })
  await latest
  resolvers[0]({ resultType: 'vector', result: [{ metric: { __name__: 'a' }, value: [1, '1'] }] })
  await stale
  assert.equal(panelResults[preview.previewPanel.value.id].result[0].metric.__name__, 'b')
  assert.equal(preview.previewState.series, 1)
})

test('typing debounces the preview into a single query', async () => {
  let calls = 0
  const { preview, panelForm } = previewFixture(async () => {
    calls++
    return { resultType: 'vector', result: [{ metric: { __name: 'up' }, value: [1, '1'] }] }
  })
  panelForm.promql = 'u'
  panelForm.promql = 'up'
  panelForm.promql = 'up{job="x"}'
  await new Promise(resolve => setTimeout(resolve, 800))
  assert.equal(calls, 1)
  assert.equal(preview.previewState.status, 'ready')
})

test('inspection layouts never run a chart preview', async () => {
  let calls = 0
  const { preview, panelForm, isListLayout } = previewFixture(async () => { calls++; return { result: [] } })
  isListLayout.value = true
  panelForm.promql = 'up'
  await preview.runPreview()
  await new Promise(resolve => setTimeout(resolve, 800))
  assert.equal(calls, 0)
  assert.equal(preview.previewState.status, 'idle')
})

test('prometheus error envelopes are reduced to an actionable title', () => {
  const { describePreviewError } = load('usePanelPreview', { queryMonitorPrometheusRange: async () => ({}) }, '({ describePreviewError })')
  const parse = describePreviewError(new Error('Prometheus API 返回状态码 400: {"status":"error","errorType":"bad_data","error":"1:37: parse error: unclosed left parenthesis"}'))
  assert.equal(parse.title, 'PromQL 语法有误')
  assert.equal(parse.detail, '1:37: parse error: unclosed left parenthesis')
  assert.equal(describePreviewError(new Error('context deadline exceeded')).title, '查询超时')
  assert.equal(describePreviewError(new Error('dial tcp 10.0.0.1:9090: connect: connection refused')).title, '数据源连接失败')
  assert.equal(describePreviewError(new Error('boom')).title, '预览查询失败')
  assert.equal(describePreviewError(new Error('boom')).detail, 'boom')
})

test('a draft titled like a backend resource table previews its PromQL instead', () => {
  const { preview, panelForm } = previewFixture(async () => ({ result: [] }))
  panelForm.title = 'Pod 资源明细'
  assert.equal(preview.previewPanel.value.title, '未命名面板')
  panelForm.title = '自定义标题'
  assert.equal(preview.previewPanel.value.title, '自定义标题')
  delete panelForm.title
  assert.equal(preview.previewPanel.value.title, '未命名面板')
})

// 编辑布局里可以把面板拉高，卡片高度一旦变化就必须由图表自己撑满，
// 否则卡片下半截会空出来、看着像凭空多了一个框。下面几条锁住
// 「卡片是 flex 纵向容器 → 面板体撑满 → 图表/表格不写死高度」这条链路。
function readDashboard(relative) {
  return readFileSync(new URL(`../src/views/monitor/dashboard/${relative}`, import.meta.url), 'utf8')
}

test('panel cards are flex columns so the body can claim the extra height', () => {
  assert.match(readDashboard('styles/panel-card.css'), /\.chart-panel \{[^}]*display: flex;[^}]*flex-direction: column;/s)
  assert.match(readDashboard('components/PanelContent.vue'), /\.panel-body \{[^}]*display: flex;[^}]*flex: 1 1 auto;[^}]*flex-direction: column;/s)
})

test('every chart kind stretches with the card instead of holding a fixed height', () => {
  const trend = readDashboard('components/charts/TrendChart.vue')
  assert.match(trend, /\.trend-chart \{[^}]*flex: 1 1 auto;/s)
  assert.match(trend, /\.trend-chart \.sparkline \{[^}]*flex: 1 1 auto;[^}]*height: auto;/s)
  assert.match(readDashboard('components/charts/GaugeChart.vue'), /\.gauge-wrap \{[^}]*flex: 1 1 auto;/s)
  assert.match(readDashboard('components/charts/StatChart.vue'), /\.stat-row \{[^}]*flex: 1 1 auto;/s)
  assert.match(readDashboard('components/charts/BarChart.vue'), /\.bar-chart \{[^}]*flex: 1 1 auto;/s)
})

test('table panels follow the card height instead of a pixel height', () => {
  for (const name of ['TableChart', 'PodResourceTable', 'HostResourceTable']) {
    const source = readDashboard(`components/charts/${name}.vue`)
    assert.match(source, /height="100%"/, `${name} 应该让 el-table 跟随容器高度`)
    assert.doesNotMatch(source, /:height="\s*isFullscreen/, `${name} 不应该再把表格高度写成固定像素`)
  }
  assert.match(readDashboard('styles/table-panel.css'), /\.table-panel-body,[\s\S]*?flex: 1 1 auto;[\s\S]*?min-height: 0;/)
})

test('the k8s plot height is a floor rather than a cap', () => {
  assert.match(readDashboard('components/charts/TrendChart.vue'), /\.is-k8s-dashboard \.trend-chart \.sparkline \{\s*min-height: 210px;/)
})

