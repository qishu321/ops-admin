import { ref, reactive, computed } from 'vue'
import { inspectionTemplates, dashboardTemplates } from '../config/templates.js'
import { saveMonitorDashboardPanel, saveMonitorDashboard, deleteMonitorDashboard } from '../../../../api/monitor'
import { ElMessage, ElMessageBox } from 'element-plus'

export function useDashboardEditor({ isListLayout, pageLayout, activeDashboard, panels, defaultDatasourceId, pageTitle, loadBase, dashboards, activeDashboardId, loadDashboard }) {
  const dashboardDialogVisible = ref(false)

  const editingDashboard = ref(false)

  const activeTemplate = ref('blank')

  const dashboardForm = reactive({
    id: undefined,
    name: '',
    layout: 'grid',
    status: 1,
    description: ''
  })

  const availableTemplates = computed(() => isListLayout.value ? inspectionTemplates : dashboardTemplates)

  const currentTemplate = computed(() => availableTemplates.value.find((item) => item.key === activeTemplate.value) || availableTemplates.value[0])

  function resetDashboardForm() {
    Object.assign(dashboardForm, {
      id: undefined,
      name: '',
      layout: pageLayout.value,
      status: 1,
      description: ''
    })
    activeTemplate.value = 'blank'
  }

  function openCreateDashboard() {
    editingDashboard.value = false
    resetDashboardForm()
    dashboardDialogVisible.value = true
  }

  function selectDashboardTemplate(templateKey) {
    activeTemplate.value = templateKey
  }

  function openEditDashboard() {
    if (!activeDashboard.value) return
    editingDashboard.value = true
    Object.assign(dashboardForm, activeDashboard.value)
    activeTemplate.value = 'blank'
    dashboardDialogVisible.value = true
  }

  async function createTemplatePanels(dashboardId) {
    if (editingDashboard.value || !currentTemplate.value.panels.length) return
    if (!defaultDatasourceId.value) return
    await Promise.all(currentTemplate.value.panels.map((panel, index) => saveMonitorDashboardPanel({
      dashboardId,
      datasourceId: defaultDatasourceId.value,
      title: panel.title,
      promql: panel.promql,
      unit: panel.unit,
      chartType: panel.chartType || 'stat',
      span: panel.span || 12,
      inspectionKind: panel.inspectionKind || 'rule',
      reducer: panel.reducer || 'last',
      operator: panel.operator || 'gte',
      warningValue: panel.warningValue ?? 0,
      criticalValue: panel.criticalValue ?? 0,
      noDataState: panel.noDataState || 'warning',
      category: panel.category || '',
      sort: index + 1,
      status: 1,
      description: panel.description || currentTemplate.value.name
    })))
  }

  async function submitDashboard() {
    if (!dashboardForm.name.trim()) {
      ElMessage.warning(`请输入${isListLayout.value ? '巡检方案' : pageTitle.value}名称`)
      return
    }
    if (!editingDashboard.value) dashboardForm.layout = pageLayout.value
    if (!editingDashboard.value && currentTemplate.value.panels.length && !defaultDatasourceId.value) {
      ElMessage.warning('请先创建 Prometheus 数据源，再使用模板创建大屏')
      return
    }
    const savedDashboard = await saveMonitorDashboard(dashboardForm)
    ElMessage.success('保存成功')
    dashboardDialogVisible.value = false
    await loadBase()
    const target = dashboards.value.find((item) => Number(item.id) === Number(savedDashboard?.id)) || savedDashboard || dashboards.value[0]
    if (target) {
      activeDashboardId.value = target.id
      await createTemplatePanels(target.id)
      await loadBase()
      await loadDashboard(target.id)
    }
  }

  async function handleDeleteDashboard() {
    if (!activeDashboard.value) return
    const entityName = isListLayout.value ? '巡检方案' : pageTitle.value
    const childName = isListLayout.value ? '巡检项' : '面板'
    await ElMessageBox.confirm(`确认删除${entityName}「${activeDashboard.value.name}」吗？${childName}也会一起删除。`, '提示', { type: 'warning' })
    await deleteMonitorDashboard(activeDashboard.value.id)
    ElMessage.success('删除成功')
    activeDashboardId.value = undefined
    activeDashboard.value = null
    panels.value = []
    await loadBase()
    await loadDashboard(activeDashboardId.value)
  }

  function handleDashboardManageCommand(command) {
    if (command === 'edit') return openEditDashboard()
    if (command === 'delete') return handleDeleteDashboard()
  }

  return {
    dashboardDialogVisible,
    editingDashboard,
    activeTemplate,
    dashboardForm,
    availableTemplates,
    currentTemplate,
    resetDashboardForm,
    openCreateDashboard,
    selectDashboardTemplate,
    openEditDashboard,
    createTemplatePanels,
    submitDashboard,
    handleDeleteDashboard,
    handleDashboardManageCommand,
  }
}
