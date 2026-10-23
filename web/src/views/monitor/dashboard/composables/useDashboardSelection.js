import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { retiredK8sPanelTitles, k8sPodPanelDefinitions, k8sTrendPanelTitles, k8sPanelTitleMap } from '../config/templates.js'

export function useDashboardSelection({ panelStateKey }) {
  const loading = ref(false)

  const route = useRoute()

  const dashboards = ref([])

  const datasourceOptions = ref([])

  const selectedDatasourceId = ref()

  const activeDashboardId = ref()

  const activeDashboard = ref(null)

  const panels = ref([])

  const pageMode = computed(() => route.path.includes('/monitor/inspections') ? 'inspection' : 'dashboard')

  const pageLayout = computed(() => pageMode.value === 'inspection' ? 'list' : 'grid')

  const isCustomGridLayout = computed(() => activeDashboard.value?.layout === 'grid-custom')

  const pageTitle = computed(() => pageMode.value === 'inspection' ? '巡检大屏' : '监控大屏')

  const pageDescription = computed(() => (
    pageMode.value === 'inspection'
      ? '按巡检项核查 PromQL 查询状态，异常优先展示，并支持导出巡检报告 PDF。'
      : '以网格大屏方式展示指标卡、趋势、排行和仪表盘，适合投屏观测。'
  ))

  const activePanels = computed(() => panels.value.filter((item) => item.status === 1 && !(!isListLayout.value && isK8sDashboard.value && retiredK8sPanelTitles.has(item.title))))

  const isHostDashboard = computed(() => !isK8sDashboard.value && (
    String(activeDashboard.value?.name || '').includes('主机') ||
    panels.value.some((panel) => String(panel.promql || '').includes('node_cpu_seconds_total'))
  ))

  const headlinePanels = computed(() => activePanels.value
    .filter((item) => ['stat', 'gauge'].includes(item.chartType))
    .slice(0, isK8sDashboard.value ? 9 : 8))

  const headlinePanelIds = computed(() => new Set(headlinePanels.value.map((item) => item.id)))

  const visualPanels = computed(() => {
    const items = panels.value.filter((item) => !headlinePanelIds.value.has(item.id) && !(isK8sDashboard.value && retiredK8sPanelTitles.has(item.title)))
    if (isCustomGridLayout.value) return [...items].sort((left, right) => Number(left.sort || 0) - Number(right.sort || 0))
    if (isK8sDashboard.value) {
      // Keep the fullscreen six-column grid visually coherent: trends share one
      // row, ranking/status cards share the next, and the resource table stays last.
      const k8sOrder = [
        'CPU Request 使用率',
        '内存 Request 使用率',
        'Pod CPU 使用量 Top',
        'Pod 内存使用量 Top',
        'Pod CPU 使用趋势',
        'Pod 内存使用趋势',
        '命名空间 Pod 分布',
        '节点 Pod 分布',
        '工作负载副本可用率',
        'Pod 网络接收速率 Top',
        'Pod 网络发送速率 Top',
        '异常原因 Top',
        'Pod 资源明细'
      ]
      const orderByTitle = new Map(k8sOrder.map((title, index) => [title, index]))
      return [...items].sort((left, right) => {
        const leftOrder = orderByTitle.get(left.title) ?? k8sOrder.length
        const rightOrder = orderByTitle.get(right.title) ?? k8sOrder.length
        return leftOrder - rightOrder || Number(left.sort || 0) - Number(right.sort || 0)
      })
    }
    if (isHostDashboard.value) {
      const hostOrder = [
        'CPU 使用趋势', 'CPU 使用率 Top', '系统负载 Top',
        '内存使用趋势', '内存使用率 Top', '网络发送速率', '网络发送速率 Top',
        // Retain the legacy title here so an existing dashboard still places the
        // card in the middle while its server-side migration turns it into IOPS.
        '磁盘使用率', '磁盘 IOPS（次/秒）', '磁盘使用率 Top', '网络接收速率', '网络接收速率 Top'
      ]
      const orderByTitle = new Map(hostOrder.map((title, index) => [title, index]))
      return [...items].sort((left, right) => {
        const leftOrder = orderByTitle.get(left.title) ?? hostOrder.length
        const rightOrder = orderByTitle.get(right.title) ?? hostOrder.length
        return leftOrder - rightOrder || Number(left.sort || 0) - Number(right.sort || 0)
      })
    }
    const memoryTrendIndex = items.findIndex((item) => item.title === '内存使用趋势')
    const networkReceiveIndex = items.findIndex((item) => item.title === '网络接收速率 Top')
    if (memoryTrendIndex >= 0 && networkReceiveIndex >= 0) {
      ;[items[memoryTrendIndex], items[networkReceiveIndex]] = [items[networkReceiveIndex], items[memoryTrendIndex]]
    }
    return items
  })

  const headlineSupplement = computed(() => headlinePanels.value.length > 0 && headlinePanels.value.length < 8
    ? {
        value: activePanels.value.length,
        healthy: activePanels.value.filter((item) => panelStateKey(item) === 'healthy').length,
      }
    : null)

  const visibleDashboards = computed(() => dashboards.value.filter((item) => (
    pageMode.value === 'inspection'
      ? item.layout === 'list'
      : ['grid', 'grid-custom'].includes(item.layout || 'grid')
  )))

  const isListLayout = computed(() => pageMode.value === 'inspection')

  const isK8sDashboard = computed(() => {
    const name = `${activeDashboard.value?.name || ''} ${activeDashboard.value?.description || ''}`.toLowerCase()
    return name.includes('k8s') || name.includes('kubernetes') || panels.value.some((panel) => String(panel.promql || '').includes('kube_'))
  })

  const missingK8sPodPanels = computed(() => {
    if (isListLayout.value || !isK8sDashboard.value || !activeDashboard.value) return []
    const existingTitles = new Set(panels.value.map((panel) => panel.title))
    return k8sPodPanelDefinitions.filter((panel) => !existingTitles.has(panel.title))
  })

  const defaultDatasourceId = computed(() => selectedDatasourceId.value || datasourceOptions.value[0]?.id)

  const currentDatasourceName = computed(() => datasourceOptions.value.find((item) => item.id === selectedDatasourceId.value)?.name || '未选择数据源')

  function panelVisualType(panel) {
    if (isK8sDashboard.value && k8sTrendPanelTitles.has(panel?.title)) return 'line'
    return panel?.chartType
  }

  function panelDisplayTitle(panel) {
    if (!isK8sDashboard.value) return panel?.title
    return k8sPanelTitleMap[panel?.title] || panel?.title
  }

  function isK8sTopRankingPanel(panel) {
    return isK8sDashboard.value && panelVisualType(panel) === 'bar' && String(panel?.title || '').includes('Top')
  }

  function isK8sDistributionPanel(panel) {
    return isK8sDashboard.value && [
      '命名空间 Pod 分布',
      '节点 Pod 分布',
      '工作负载副本可用率'
    ].includes(panel?.title)
  }

  function isPodDetailPanel(panel) {
    return panel?.title === 'Pod 资源明细'
  }

  function isHostDetailPanel(panel) {
    return !isK8sDashboard.value && panel?.title === '主机信息'
  }

  return {
    loading,
    route,
    dashboards,
    datasourceOptions,
    selectedDatasourceId,
    activeDashboardId,
    activeDashboard,
    panels,
    pageMode,
    pageLayout,
    isCustomGridLayout,
    pageTitle,
    pageDescription,
    activePanels,
    isHostDashboard,
    headlinePanels,
    headlinePanelIds,
    visualPanels,
    headlineSupplement,
    visibleDashboards,
    isListLayout,
    isK8sDashboard,
    missingK8sPodPanels,
    defaultDatasourceId,
    currentDatasourceName,
    panelVisualType,
    panelDisplayTitle,
    isK8sTopRankingPanel,
    isK8sDistributionPanel,
    isPodDetailPanel,
    isHostDetailPanel,
  }
}
