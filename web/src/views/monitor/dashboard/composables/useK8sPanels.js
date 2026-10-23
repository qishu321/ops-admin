import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { saveMonitorDashboardPanel } from '../../../../api/monitor'

export function useK8sPanels({ activeDashboard, missingK8sPodPanels, selectedDatasourceId, defaultDatasourceId, panels, loadBase, loadDashboard }) {
  const syncingK8sPodPanels = ref(false)

  async function syncK8sPodPanels() {
    if (!activeDashboard.value || !missingK8sPodPanels.value.length) return
    const datasourceId = selectedDatasourceId.value || defaultDatasourceId.value
    if (!datasourceId) {
      ElMessage.warning('请先选择 Prometheus 或 VictoriaMetrics 数据源')
      return
    }
    syncingK8sPodPanels.value = true
    try {
      const missingPanels = [...missingK8sPodPanels.value]
      for (const [index, panel] of missingPanels.entries()) {
        await saveMonitorDashboardPanel({
          dashboardId: activeDashboard.value.id,
          datasourceId,
          title: panel.title,
          promql: panel.promql,
          unit: panel.unit,
          chartType: panel.chartType,
          span: panel.span,
          sort: panels.value.length + index + 1,
          status: 1,
          description: '与容器管理 Pod 监控保持一致'
        })
      }
      ElMessage.success(`已补全 ${missingPanels.length} 个 Pod 监控面板`)
      const dashboardId = activeDashboard.value.id
      await loadBase()
      await loadDashboard(dashboardId)
    } finally {
      syncingK8sPodPanels.value = false
    }
  }

  return {
    syncingK8sPodPanels,
    syncK8sPodPanels,
  }
}
