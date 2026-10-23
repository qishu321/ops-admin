import { ref, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { saveMonitorDashboardPanel, deleteMonitorDashboardPanel } from '../../../../api/monitor'

export function usePanelEditor({ activeDashboardId, defaultDatasourceId, panels, isListLayout, loadDashboard }) {
  const panelDialogVisible = ref(false)

  const editingPanel = ref(false)

  const panelForm = reactive({
    id: undefined,
    dashboardId: undefined,
    title: '',
    datasourceId: undefined,
    promql: '',
    unit: '',
    chartType: 'stat',
    span: 12,
    gridX: 0,
    gridY: 0,
    gridW: 0,
    gridH: 0,
    inspectionKind: 'rule',
    reducer: 'max',
    operator: 'gte',
    warningValue: 80,
    criticalValue: 90,
    noDataState: 'warning',
    category: '自定义',
    sort: 0,
    status: 1,
    description: ''
  })

  function resetPanelForm() {
    Object.assign(panelForm, {
      id: undefined,
      dashboardId: activeDashboardId.value,
      title: '',
      datasourceId: defaultDatasourceId.value,
      promql: '',
      unit: '',
      chartType: 'stat',
      span: 12,
      gridX: 0,
      gridY: 0,
      gridW: 0,
      gridH: 0,
      inspectionKind: 'rule',
      reducer: 'max',
      operator: 'gte',
      warningValue: 80,
      criticalValue: 90,
      noDataState: 'warning',
      category: '自定义',
      sort: panels.value.length + 1,
      status: 1,
      description: ''
    })
  }

  function openCreatePanel() {
    if (!activeDashboardId.value) {
      ElMessage.warning(isListLayout.value ? '请先新建或选择巡检方案' : '请先创建或选择监控大屏')
      return
    }
    editingPanel.value = false
    resetPanelForm()
    panelDialogVisible.value = true
  }

  function openEditPanel(row) {
    editingPanel.value = true
    Object.assign(panelForm, row)
    panelDialogVisible.value = true
  }

  async function submitPanel() {
    if (!panelForm.title.trim() || !panelForm.datasourceId || !panelForm.promql.trim()) {
      ElMessage.warning(`请填写${isListLayout.value ? '巡检项' : '面板'}标题、数据源和 PromQL`)
      return
    }
    await saveMonitorDashboardPanel(panelForm)
    ElMessage.success('保存成功')
    panelDialogVisible.value = false
    await loadDashboard(activeDashboardId.value)
  }

  async function handleDeletePanel(row) {
    await ElMessageBox.confirm(`确认删除${isListLayout.value ? '巡检项' : '面板'}「${row.title}」吗？`, '提示', { type: 'warning' })
    await deleteMonitorDashboardPanel(row.id)
    ElMessage.success('删除成功')
    await loadDashboard(activeDashboardId.value)
  }

  async function copyPromql(promql) {
    try {
      await navigator.clipboard.writeText(promql || '')
      ElMessage.success('PromQL 已复制')
    } catch {
      ElMessage.warning('复制失败，请手动复制 PromQL')
    }
  }

  return {
    panelDialogVisible,
    editingPanel,
    panelForm,
    resetPanelForm,
    openCreatePanel,
    openEditPanel,
    submitPanel,
    handleDeletePanel,
    copyPromql,
  }
}
