import { ElMessage } from 'element-plus'
import { queryMonitorDashboardList, queryMonitorDatasourceOptions, monitorDashboardInfo } from '../../../../api/monitor'

export function useDashboardLoader({ dashboards, datasourceOptions, selectedDatasourceId, visibleDashboards, activeDashboardId, activeDashboard, panels, inspectionReport, inspectionReportResults, loading, inspectionFilter, layoutNeedsMigration, initializePanelGrid, preparePanelGrid, exitLayoutEditing, layoutLocked, invalidateQueries, panelResults, panelPending, isListLayout, loadLatestInspectionReport, refreshAllPanels }) {
  let loadVersion = 0
  async function loadBase() {
    const [dashboardData, datasources] = await Promise.all([
      queryMonitorDashboardList({ pageNum: 1, pageSize: 100 }),
      queryMonitorDatasourceOptions()
    ])
    dashboards.value = dashboardData.list || []
    datasourceOptions.value = (datasources || []).filter((item) => ['prometheus', 'victoriametrics'].includes(item.type))
    if (!datasourceOptions.value.some((item) => item.id === selectedDatasourceId.value)) {
      selectedDatasourceId.value = datasourceOptions.value.find((item) => item.isDefault)?.id || datasourceOptions.value[0]?.id
    }
    if (!visibleDashboards.value.some((item) => item.id === activeDashboardId.value)) {
      activeDashboardId.value = visibleDashboards.value[0]?.id
    }
  }

  async function loadDashboard(id = activeDashboardId.value) {
    if (layoutLocked.value) { ElMessage.warning('请等待布局保存或恢复完成'); return }
    exitLayoutEditing()
    invalidateQueries()
    const version = ++loadVersion
    if (!id) {
      activeDashboard.value = null
      panels.value = []
      inspectionReport.value = null
      inspectionReportResults.value = []
      return
    }
    loading.value = true
    try {
      const data = await monitorDashboardInfo(id)
      if (version !== loadVersion) return
      activeDashboard.value = data.dashboard
      panels.value = data.panels || []
      activeDashboardId.value = id
      inspectionFilter.value = 'all'
      if (data.dashboard?.layout === 'grid-custom') preparePanelGrid()
      else layoutNeedsMigration.value = false
      Object.keys(panelResults).forEach((key) => delete panelResults[key])
      Object.keys(panelPending).forEach((key) => delete panelPending[key])
      if (isListLayout.value) {
        await loadLatestInspectionReport(id)
      } else {
        // Monitoring dashboards remain live; inspection schemes only read their
        // last persisted report and never query metrics on page entry.
        void refreshAllPanels({ progressive: true, force: false })
      }
    } finally {
      if (version === loadVersion) loading.value = false
    }
  }

  return {
    loadBase,
    loadDashboard,
  }
}
