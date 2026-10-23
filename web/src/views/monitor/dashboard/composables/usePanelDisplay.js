import { computed } from 'vue'
import { formatByUnit, numberValue } from '../utils/format.js'
import { Cpu, Coin, DataLine, Connection, TrendCharts, Warning, Monitor } from '@element-plus/icons-vue'

export function usePanelDisplay({ activePanels, panelResults, activeDashboard, panelVisualType, panelTrend, barRows }) {
  const dashboardHealth = computed(() => {
    const errors = activePanels.value.filter((panel) => panelResults[panel.id]?.error).length
    if (!activeDashboard.value) return { text: '未选择', type: 'info' }
    if (errors > 0) return { text: `${errors} 个异常`, type: 'danger' }
    return { text: '正常', type: 'success' }
  })

  function panelRows(panel) {
    return panelResults[panel.id]?.result || []
  }

  function podDetailRows(panel) {
    return panelResults[panel.id]?.podResources || []
  }

  function podResourceNamespaces(panel) {
    return panelResults[panel.id]?.namespaces || []
  }

  function hostDetailRows(panel) {
    return panelResults[panel.id]?.hostResources || []
  }

  function panelValue(panel) {
    const value = panelSummaryValue(panel)
    if (!Number.isFinite(value)) return '-'
    const formatted = formatByUnit(value, panel.unit)
    return panel.unit && formatted.endsWith(panel.unit) ? formatted.slice(0, -panel.unit.length).trim() : formatted
  }

  function panelSummaryValue(panel) {
    if (!panelRows(panel).length) return Number.NaN
    if (panelVisualType(panel) === 'line') {
      const values = panelTrend(panel)
      if (values.length) return values[values.length - 1]
    }
    return numberValue(panelRows(panel)[0])
  }

  function panelChartLabel(chartType) {
    return ({ stat: '指标', gauge: '仪表盘', bar: '排行', line: '趋势', table: '明细' })[chartType] || chartType
  }

  function panelTone(panel) {
    const title = String(panel?.title || '').toLowerCase()
    if (title.includes('离线') || title.includes('异常')) return 'danger'
    if (title.includes('在线') || title.includes('正常')) return 'success'
    if (title.includes('磁盘') || title.includes('负载')) return 'warning'
    if (title.includes('内存')) return 'purple'
    if (title.includes('网络') || title.includes('连接')) return 'cyan'
    return 'primary'
  }

  function panelIcon(panel) {
    const title = String(panel?.title || '').toLowerCase()
    if (title.includes('cpu')) return Cpu
    if (title.includes('内存')) return Coin
    if (title.includes('磁盘')) return DataLine
    if (title.includes('网络') || title.includes('连接')) return Connection
    if (title.includes('负载')) return TrendCharts
    if (title.includes('离线') || title.includes('异常')) return Warning
    return Monitor
  }

  function panelVisibleSeriesCount(panel) {
    return panelVisualType(panel) === 'bar' ? barRows(panel).length : panelResultCount(panel)
  }

  function gaugePercent(panel) {
  	const value = numberValue(panelRows(panel)[0])
    if (!Number.isFinite(value)) return 0
    return Math.max(0, Math.min(100, value))
  }

  function panelState(panel) {
    if (panel.status !== 1) return '已禁用'
    if (panelResults[panel.id]?.error) return '查询失败'
    if (!panelResultCount(panel)) return '暂无数据'
    return '实时'
  }

  function panelStateKey(panel) {
    if (panel.status !== 1) return 'disabled'
    if (panelResults[panel.id]?.error) return 'danger'
    if (!panelResultCount(panel)) return 'warning'
    return 'healthy'
  }

  function panelStateType(panel) {
    if (panel.status !== 1) return 'info'
    if (panelResults[panel.id]?.error) return 'danger'
    if (!panelResultCount(panel)) return 'warning'
    return 'success'
  }

  function panelResultCount(panel) {
    const result = panelResults[panel.id]
    return result?.podResources?.length || result?.hostResources?.length || panelRows(panel).length
  }

  function panelDisplayValue(panel) {
    const value = panelSummaryValue(panel)
    if (!Number.isFinite(value)) return '—'
    return formatByUnit(value, panel.unit)
  }

  return {
    dashboardHealth,
    panelRows,
    podDetailRows,
    podResourceNamespaces,
    hostDetailRows,
    panelValue,
    panelSummaryValue,
    panelChartLabel,
    panelTone,
    panelIcon,
    panelVisibleSeriesCount,
    gaugePercent,
    panelState,
    panelStateKey,
    panelStateType,
    panelResultCount,
    panelDisplayValue,
  }
}
