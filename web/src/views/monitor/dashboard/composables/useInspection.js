import { ref, computed, watch } from 'vue'
import { queryLatestMonitorInspectionRun, runMonitorInspection } from '../../../../api/monitor'
import { ElMessage } from 'element-plus'

export function useInspection({ panels, activeDashboardId, selectedDatasourceId, datasourceOptions, isListLayout, panelForm, activePanels, timeRangeOption, customTimeRange, currentTimeBounds, inspectionRangeLabel }) {
  const inspectionRunning = ref(false)
  const selectedInspectionDatasourceIds = ref([])
  const inspectionDatasourceFilter = ref('all')
  let initialized = false, selectionVersion = 0, reportVersion = 0
  watch(selectedInspectionDatasourceIds, () => { selectionVersion++ }, { deep: true, flush: 'sync' })
  watch([datasourceOptions, selectedDatasourceId], () => {
    const allowed = new Set(datasourceOptions.value.map(item => Number(item.id)))
    if (!allowed.size) { selectedInspectionDatasourceIds.value = []; return }
    if (!initialized) {
      const id = allowed.has(Number(selectedDatasourceId.value)) ? Number(selectedDatasourceId.value) : datasourceOptions.value.find(item => item.isDefault)?.id || datasourceOptions.value[0].id
      selectedInspectionDatasourceIds.value = [Number(id)]
      initialized = true
    } else selectedInspectionDatasourceIds.value = selectedInspectionDatasourceIds.value.filter(id => allowed.has(Number(id)))
  }, { immediate: true })

  const lastInspectionAt = ref(null)

  const inspectionReport = ref(null)

  const inspectionReportResults = ref([])

  const inspectionFilter = ref('all')

  const resultKey = (datasourceId, panelId) => `${Number(datasourceId || 0)}:${Number(panelId)}`
  const inspectionResultMap = computed(() => new Map(inspectionReportResults.value.map((item) => [resultKey(item.datasourceId, item.panelId), item])))

  const inspectionResultFor = (panel) => panel?.reportResult || inspectionResultMap.value.get(resultKey(panel?.datasourceId, panel?.id))
  const inspectionReportDatasources = computed(() => {
    const run = inspectionReport.value
    if (!run) return []
    if (run.datasourceIds?.length) return run.datasourceIds.map((id,index) => ({ id:Number(id),name:run.datasourceNames?.[index] || `数据源 #${id}` }))
    return run.datasourceId ? [{id:Number(run.datasourceId),name:run.datasourceName || `数据源 #${run.datasourceId}`}] : []
  })
  const inspectionReportDatasourceLabel = computed(() => inspectionReportDatasources.value.map(item => item.name).join('、') || '—')

  const inspectionRows = computed(() => {
    if (!inspectionReport.value) return panels.value.map(panel => ({ ...panel, rowKey:`rule-${panel.id}` }))
    const currentById = new Map(panels.value.map((panel) => [Number(panel.id), panel]))
    const reported = new Set()
    const rows = inspectionReportResults.value.map((result, index) => {
      const current = currentById.get(Number(result.panelId))
      reported.add(Number(result.panelId))
      return {
        ...(current || {}),
        id: result.panelId,
        rowKey: `result-${result.id || resultKey(result.datasourceId,result.panelId)}-${index}`,
        panelDefinition: current,
        datasourceId: result.datasourceId,
        datasourceName: result.datasourceName,
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
        status: 1,
        panelExists: Boolean(current),
        reportResult: result
      }
    })
    currentById.forEach((panel,id) => { if (!reported.has(id)) rows.push({ ...panel, rowKey:`rule-${id}`, panelExists: true }) })
    return rows
  })

  const inspectionStateKey = (panel) => {
    if (panel.status !== 1) return 'disabled'
    return inspectionResultFor(panel)?.status || 'uninspected'
  }

  const inspectionScopedRows = computed(() => inspectionRows.value.filter(panel => inspectionDatasourceFilter.value === 'all' || Number(panel.reportResult?.datasourceId) === Number(inspectionDatasourceFilter.value)))
  const inspectionSummary = computed(() => {
    const summary = { healthy: 0, warning: 0, danger: 0, disabled: 0, uninspected: 0 }
    inspectionScopedRows.value.forEach((panel) => { summary[inspectionStateKey(panel)] += 1 })
    return summary
  })

  const inspectionPanels = computed(() => {
    const priority = { danger: 0, warning: 1, healthy: 2, uninspected: 3, disabled: 4 }
    return inspectionScopedRows.value
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

  async function loadLatestInspectionReport(dashboardId = activeDashboardId.value, { preserveCurrent = false } = {}) {
    const version = ++reportVersion, selection = selectionVersion
    if (!preserveCurrent) {
      inspectionDatasourceFilter.value = 'all'
      inspectionReport.value = null
      inspectionReportResults.value = []
      lastInspectionAt.value = null
    }
    if (!dashboardId) return
    const data = await queryLatestMonitorInspectionRun(dashboardId)
    if (version !== reportVersion || Number(activeDashboardId.value) !== Number(dashboardId)) return
    applyReport(data)
    if (selection === selectionVersion && data?.run) {
      const ids = inspectionReportDatasources.value.map(item => item.id).filter(id => datasourceOptions.value.some(option => Number(option.id) === id))
      if (ids.length) selectedInspectionDatasourceIds.value = ids
    }
  }

  function applyReport(data) {
    inspectionReport.value = data?.run || null
    inspectionReportResults.value = (data?.results || []).map(result => ({ ...result, datasourceId:result.datasourceId || data?.run?.datasourceId, datasourceName:result.datasourceName || data?.run?.datasourceName || '' }))
    lastInspectionAt.value = data?.run?.completedAt ? new Date(data.run.completedAt) : null
    inspectionDatasourceFilter.value = 'all'
  }

  async function executeInspection() {
    if (inspectionRunning.value) return
    const datasourceIds = [...new Set(selectedInspectionDatasourceIds.value.map(Number))].filter(id => id > 0)
    if (!datasourceIds.length) {
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
    const dashboardId = activeDashboardId.value, version = ++reportVersion
    try {
      const { startAt, endAt } = currentTimeBounds()
      const data = await runMonitorInspection({
        dashboardId,
        datasourceId: datasourceIds[0],
        datasourceIds,
        startAt,
        endAt,
        stepSeconds: Math.max(15, Math.ceil((endAt - startAt) / 240))
      }, { timeout: Math.max(60000, Math.ceil(activePanels.value.length * datasourceIds.length / 4) * 31000 + 15000) })
      if (version !== reportVersion || Number(activeDashboardId.value) !== Number(dashboardId)) return
      if (datasourceIds.length > 1 && !datasourceIds.every(id => data?.run?.datasourceIds?.map(Number).includes(id))) {
        ElMessage.error('服务端未返回完整的多数据源报告，请确认后端已更新后重试')
        return
      }
      applyReport(data)
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
    selectedInspectionDatasourceIds,
    inspectionDatasourceFilter,
    inspectionReportDatasources,
    inspectionReportDatasourceLabel,
    inspectionScopedRows,
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
