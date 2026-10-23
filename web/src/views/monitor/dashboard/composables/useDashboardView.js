import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'

export function useDashboardView({ activeDashboard, activeDashboardId, visibleDashboards, panels, selectedDatasourceId, datasourceOptions, timeRangeOption, timeRangeSeconds, autoRefreshSeconds }) {
  const route = useRoute(), router = useRouter()
  const focusedPanel = ref(null), shareUrl = ref(''), shareDialogVisible = ref(false)
  function openPanelFocus(panel) { focusedPanel.value = panel }
  function closePanelFocus() { focusedPanel.value = null }
  watch(() => activeDashboard.value?.id, closePanelFocus)
  function explorePanel(panel) {
    closePanelFocus()
    router.push({ path: '/monitor/query', query: { query: panel.promql, datasourceId: selectedDatasourceId.value || panel.datasourceId, range: timeRangeSeconds.value } })
  }
  async function copyViewLink() {
    const query = { dashboard: activeDashboard.value.id, source: selectedDatasourceId.value, range: timeRangeSeconds.value, refresh: autoRefreshSeconds.value }
    if (focusedPanel.value) query.panel = focusedPanel.value.id
    shareUrl.value = new URL(router.resolve({ path: '/monitor/dashboards', query }).href, window.location.origin).href
    try { await navigator.clipboard.writeText(shareUrl.value); ElMessage.success('当前视图链接已复制') }
    catch { shareDialogVisible.value = true }
  }
  function applySharedView() {
    if (route.path !== '/monitor/dashboards') return
    const query = route.query
    if (visibleDashboards.value.some(item => item.id === Number(query.dashboard))) activeDashboardId.value = Number(query.dashboard)
    if (datasourceOptions.value.some(item => item.id === Number(query.source))) selectedDatasourceId.value = Number(query.source)
    if ([900, 3600, 21600, 86400].includes(Number(query.range))) timeRangeOption.value = timeRangeSeconds.value = Number(query.range)
    if ([0, 10, 30, 60].includes(Number(query.refresh))) autoRefreshSeconds.value = Number(query.refresh)
  }
  function restorePanelFocus() {
    if (route.path === '/monitor/dashboards') focusedPanel.value = panels.value.find(panel => panel.id === Number(route.query.panel)) || null
  }
  return { focusedPanel, shareUrl, shareDialogVisible, openPanelFocus, closePanelFocus, explorePanel, copyViewLink, applySharedView, restorePanelFocus }
}
