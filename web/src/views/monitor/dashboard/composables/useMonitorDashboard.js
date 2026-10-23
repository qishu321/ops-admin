import { onMounted, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { ElMessage } from 'element-plus'
import { provideDashboardDomains } from '../context'
import { useDashboardSelection } from './useDashboardSelection'
import { useTimeRange } from './useTimeRange'
import { usePanelQueries } from './usePanelQueries'
import { usePanelDisplay } from './usePanelDisplay'
import { useTrendChart } from './useTrendChart'
import { useBarChart } from './useBarChart'
import { useGridLayout } from './useGridLayout'
import { useDashboardEditor } from './useDashboardEditor'
import { usePanelEditor } from './usePanelEditor'
import { usePanelPreview } from './usePanelPreview'
import { useInspection } from './useInspection'
import { useInspectionExport } from './useInspectionExport'
import { useK8sPanels } from './useK8sPanels'
import { useDashboardLoader } from './useDashboardLoader'
import { useAutoRefresh } from './useAutoRefresh'
import { useFullscreen } from './useFullscreen'
import { useDashboardView } from './useDashboardView'
import { useDashboardTheme } from './useDashboardTheme'

/** Connect domain actions here; domain modules own their refs and computations. */
export function useMonitorDashboard() {
  const selection = useDashboardSelection({ panelStateKey: panel => display.panelStateKey(panel) })
  const time = useTimeRange({
    isListLayout: selection.isListLayout,
    refreshAllPanels: options => queries.refreshAllPanels(options),
  })
  const fullscreen = useFullscreen()
  const theme = useDashboardTheme()
  const queries = usePanelQueries({
    ...selection, ...time,
    panelStateKey: panel => display.panelStateKey(panel),
  })
  const display = usePanelDisplay({
    ...selection, ...queries,
    panelTrend: panel => trends.panelTrend(panel),
    barRows: panel => bars.barRows(panel),
  })
  const trends = useTrendChart({ ...selection, ...display, ...time, panelResults: queries.panelResults })
  const bars = useBarChart({ ...selection, ...display })
  const grid = useGridLayout({ ...selection, ...fullscreen })
  const allowNavigation = () => {
    if (grid.layoutLocked.value) { ElMessage.warning('请等待布局保存或恢复完成后再切换页面'); return false }
    grid.exitLayoutEditing()
    return true
  }
  onBeforeRouteLeave(allowNavigation)
  onBeforeRouteUpdate(allowNavigation)
  // Load callbacks run after setup, avoiding circular module imports.
  const reload = {
    loadBase: () => loader.loadBase(),
    loadDashboard: id => loader.loadDashboard(id),
  }
  const dashboardEditor = useDashboardEditor({ ...selection, ...reload })
  const panelEditor = usePanelEditor({ ...selection, ...reload })
  const inspection = useInspection({ ...selection, ...time, panelForm: panelEditor.panelForm })
  // Preview reuses the panel result map, so it must be created after queries and
  // after the editor form it mirrors.
  const panelPreview = usePanelPreview({
    ...time,
    panelForm: panelEditor.panelForm,
    panelDialogVisible: panelEditor.panelDialogVisible,
    panelResults: queries.panelResults,
    isListLayout: selection.isListLayout,
    rangeLabel: time.inspectionRangeLabel,
  })
  const report = useInspectionExport(inspection)
  const k8s = useK8sPanels({ ...selection, ...reload })
  const loader = useDashboardLoader({ ...selection, ...queries, ...inspection, ...grid })
  const autoRefresh = useAutoRefresh({ ...selection, ...queries })

  const view = useDashboardView({ ...selection, ...time, ...autoRefresh })

  function resetDashboardFilters() {
    time.timeRangeOption.value = 3600
    time.timeRangeSeconds.value = 3600
    time.customTimeRange.value = []
    autoRefresh.autoRefreshSeconds.value = 30
    autoRefresh.restartAutoRefresh()
    queries.refreshAllPanels()
  }

  provideDashboardDomains({
    selection, time, queries, display, trends, bars, grid,
    dashboardEditor, panelEditor, panelPreview, inspection, report, k8s, loader,
    autoRefresh, fullscreen, theme, view, filters: { resetDashboardFilters },
  })

  onMounted(async () => {
    if (!await loader.loadBase()) return
    view.applySharedView()
    await loader.loadDashboard(selection.activeDashboardId.value)
    view.restorePanelFocus()
    autoRefresh.restartAutoRefresh()
  })

  watch(selection.pageLayout, async () => {
    selection.activeDashboardId.value = undefined
    selection.activeDashboard.value = null
    selection.panels.value = []
    if (!await loader.loadBase()) return
    await loader.loadDashboard(selection.activeDashboardId.value)
  })

  return { ...selection, ...theme, loadError: loader.loadError, retryLoad: loader.retryLoad, isFullscreen: fullscreen.isFullscreen }
}
