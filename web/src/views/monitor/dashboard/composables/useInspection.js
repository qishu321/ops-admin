import { ref, computed } from 'vue'
import { queryLatestMonitorInspectionRun, runMonitorInspection } from '../../../../api/monitor'
import { ElMessage } from 'element-plus'

export function useInspection({ panels, activeDashboardId, selectedDatasourceId, isListLayout, panelForm, activePanels, timeRangeOption, customTimeRange, currentTimeBounds, inspectionRangeLabel }) {
  const inspectionRunning = ref(false)

  const lastInspectionAt = ref(null)

  const inspectionReport = ref(null)

  const inspectionReportResults = ref([])

  const inspectionFilter = ref('all')

  const inspectionResultMap = computed(() => new Map(inspectionReportResults.value.map((item) => [Number(item.panelId), item])))

  const inspectionResultFor = (panel) => panel?.reportResult || inspectionResultMap.value.get(Number(panel.id))

  const inspectionRows = computed(() => {
    if (!inspectionReport.value) return panels.value
    const currentById = new Map(panels.value.map((panel) => [Number(panel.id), panel]))
    const rows = inspectionReportResults.value.map((result) => {
      const current = currentById.get(Number(result.panelId))
      currentById.delete(Number(result.panelId))
      return {
        ...(current || {}),
        id: result.panelId,
        title: result.title,
        promql: result.promql,
        unit: result.unit,
        reducer: result.reducer,
        operator: result.operator,
        warningValue: result.warningValue,
        criticalValue: result.criticalValue,
        noDataState: result.noDataState,
        category: result.category,
        inspectionKind: result.inspectionKind,
        status: current?.status ?? 1,
        panelExists: Boolean(current),
        reportResult: result
      }
    })
    currentById.forEach((panel) => rows.push({ ...panel, panelExists: true }))
    return rows
  })

  const inspectionStateKey = (panel) => {
    if (panel.status !== 1) return 'disabled'
    return inspectionResultFor(panel)?.status || 'uninspected'
  }

  const inspectionSummary = computed(() => {
    const summary = { healthy: 0, warning: 0, danger: 0, disabled: 0, uninspected: 0 }
    inspectionRows.value.forEach((panel) => { summary[inspectionStateKey(panel)] += 1 })
    return summary
  })

  const inspectionPanels = computed(() => {
    const priority = { danger: 0, warning: 1, healthy: 2, uninspected: 3, disabled: 4 }
    return inspectionRows.value
      .filter((panel) => inspectionFilter.value === 'all' || inspectionStateKey(panel) === inspectionFilter.value)
      .slice()
      .sort((left, right) => priority[inspectionStateKey(left)] - priority[inspectionStateKey(right)] || Number(left.sort || 0) - Number(right.sort || 0))
  })

  function inspectionState(panel) {
    const state = inspectionStateKey(panel)
    return { healthy: '正常', warning: '待核查', danger: '异常', disabled: '已禁用', uninspected: '未巡检' }[state] || '未巡检'
  }

  function inspectionStateType(panel) {
    return { healthy: 'success', warning: 'warning', danger: 'danger', disabled: 'info', uninspected: 'info' }[inspectionStateKey(panel)] || 'info'
  }

  function inspectionDisplayValue(panel) {
    return inspectionResultFor(panel)?.valueText || '--'
  }

  function inspectionSeriesCount(panel) {
    const result = inspectionResultFor(panel)
    return result ? result.seriesCount : '--'
  }

  function inspectionThresholdText(panel) {
    const result = inspectionResultFor(panel)
    const operator = result?.operator || panel.operator || 'gte'
    const warning = result?.warningValue ?? panel.warningValue
    const critical = result?.criticalValue ?? panel.criticalValue
    const sign = { gt: '>', gte: '≥', lt: '<', lte: '≤' }[operator] || '≥'
    if ((panel.inspectionKind || 'rule') === 'context') return '仅记录'
    return `待核查 ${sign} ${warning}${panel.unit || ''} · 异常 ${sign} ${critical}${panel.unit || ''}`
  }

  async function loadLatestInspectionReport(dashboardId = activeDashboardId.value) {
    inspectionReport.value = null
    inspectionReportResults.value = []
    lastInspectionAt.value = null
    if (!dashboardId) return
    const data = await queryLatestMonitorInspectionRun(dashboardId)
    inspectionReport.value = data?.run || null
    inspectionReportResults.value = data?.results || []
    lastInspectionAt.value = data?.run?.completedAt ? new Date(data.run.completedAt) : null
  }

  async function executeInspection() {
    if (!selectedDatasourceId.value) {
      ElMessage.warning('请先选择巡检数据源')
      return
    }
    if (isListLayout.value && panelForm.inspectionKind !== 'context') {
      const warning = Number(panelForm.warningValue)
      const critical = Number(panelForm.criticalValue)
      if (panelForm.operator === 'gte' && critical < warning) {
        ElMessage.warning('数值越大越异常时，异常阈值不能小于待核查阈值')
        return
      }
      if (panelForm.operator === 'lte' && critical > warning) {
        ElMessage.warning('数值越小越异常时，异常阈值不能大于待核查阈值')
        return
      }
    }
    if (!activePanels.value.length) {
      ElMessage.warning('当前巡检方案没有可执行的巡检项')
      return
    }
    if (timeRangeOption.value === 'custom' && customTimeRange.value?.length !== 2) {
      ElMessage.warning('请选择完整的自定义时间范围')
      return
    }
    inspectionRunning.value = true
    try {
      const { startAt, endAt } = currentTimeBounds()
      const data = await runMonitorInspection({
        dashboardId: activeDashboardId.value,
        datasourceId: selectedDatasourceId.value,
        startAt,
        endAt,
        stepSeconds: Math.max(15, Math.ceil((endAt - startAt) / 240))
      })
      inspectionReport.value = data?.run || null
      inspectionReportResults.value = data?.results || []
      lastInspectionAt.value = data?.run?.completedAt ? new Date(data.run.completedAt) : new Date()
      inspectionFilter.value = 'all'
      const abnormalCount = Number(data?.run?.dangerCount || 0) + Number(data?.run?.warningCount || 0)
      if (abnormalCount) {
        ElMessage.warning(`报告 #${data.run.id} 已生成（${inspectionRangeLabel()}），发现 ${abnormalCount} 个异常或待核查项`)
      } else {
        ElMessage.success(`报告 #${data.run.id} 已生成（${inspectionRangeLabel()}），未发现异常项`)
      }
    } finally {
      inspectionRunning.value = false
    }
  }

  return {
    inspectionRunning,
    lastInspectionAt,
    inspectionReport,
    inspectionReportResults,
    inspectionFilter,
    inspectionResultMap,
    inspectionResultFor,
    inspectionRows,
    inspectionStateKey,
    inspectionSummary,
    inspectionPanels,
    inspectionState,
    inspectionStateType,
    inspectionDisplayValue,
    inspectionSeriesCount,
    inspectionThresholdText,
    loadLatestInspectionReport,
    executeInspection,
  }
}
