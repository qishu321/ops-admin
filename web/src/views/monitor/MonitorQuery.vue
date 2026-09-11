<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { queryMonitorDatasourceOptions, queryMonitorPrometheus, queryMonitorPrometheusRange, queryMonitorQueryHistoryList } from '../../api/monitor'

const loading = ref(false)
const datasourceOptions = ref([])
const datasourceId = ref()
const promql = ref('up')
const resultType = ref('')
const rows = ref([])
const resultView = ref('table')
const chartLoading = ref(false)
const chartRange = ref('1h')
const chartResult = ref([])
const chartHover = ref(null)
const seriesSortOrder = ref('desc')
const metricTemplateId = ref('')
const historyVisible = ref(false)
const historyLoading = ref(false)
const historyRows = ref([])
const historyTotal = ref(0)
const historyQuery = ref({ pageNum: 1, pageSize: 10, keyword: '', status: '' })

const metricTemplates = [
  { id: 'host-up', category: '主机资源', name: '主机采集状态', query: 'up{job=~"node.*|node-exporter"}', description: '逐台展示主机采集状态，保留 instance 等标签。' },
  { id: 'host-cpu', category: '主机资源', name: '主机 CPU 使用率', query: '100 - (avg by (instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', description: '按实例展示近 5 分钟 CPU 使用率。' },
  { id: 'host-memory', category: '主机资源', name: '主机内存使用率', query: '(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100', description: '逐台展示主机内存使用率。' },
  { id: 'host-disk', category: '主机资源', name: '主机磁盘使用率', query: '100 - (node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/run.*|/boot.*"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay",mountpoint!~"/run.*|/boot.*"} * 100)', description: '按主机和挂载点展示磁盘使用率。' },
  { id: 'host-cpu-top', category: '主机资源', name: 'CPU 使用率 Top 10', query: 'topk(10, 100 - (avg by (instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100))', description: '按实例查看 CPU 使用率最高的十台主机。' },
  { id: 'host-memory-top', category: '主机资源', name: '内存使用率 Top 10', query: 'topk(10, (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100)', description: '按实例查看内存使用率最高的十台主机。' },
  { id: 'host-load-top', category: '主机资源', name: '系统负载 Top 10', query: 'topk(10, node_load1)', description: '按实例查看 1 分钟系统负载最高的十台主机。' },
  { id: 'host-network-in', category: '主机资源', name: '网络接收速率 Top 10', query: 'topk(10, sum by (instance) (rate(node_network_receive_bytes_total{device!~"lo|veth.*|docker.*|br.*"}[5m])))', description: '按实例查看近 5 分钟网络接收速率。' },
  { id: 'host-processes', category: '主机资源', name: '运行进程数', query: 'node_procs_running', description: '逐台展示主机正在运行的进程数。' },
  { id: 'k8s-pods', category: 'Kubernetes', name: 'Pod 基础信息', query: 'kube_pod_info', description: '逐个展示 Pod，保留 namespace、pod、node 等标签。' },
  { id: 'k8s-running-pods', category: 'Kubernetes', name: '运行中 Pod', query: 'kube_pod_status_phase{phase="Running"}', description: '逐个展示处于 Running 状态的 Pod。' },
  { id: 'k8s-abnormal-pods', category: 'Kubernetes', name: '异常 Pod', query: 'kube_pod_status_phase{phase=~"Failed|Unknown|Pending"}', description: '逐个展示 Failed、Unknown、Pending 状态的 Pod。' },
  { id: 'k8s-ready-nodes', category: 'Kubernetes', name: '节点就绪状态', query: 'kube_node_status_condition{condition="Ready",status="true"}', description: '逐节点展示 Ready 状态。' },
  { id: 'k8s-namespaces', category: 'Kubernetes', name: '命名空间', query: 'kube_namespace_created', description: '展示所有命名空间及创建时间序列。' },
  { id: 'k8s-workloads', category: 'Kubernetes', name: 'Deployment 工作负载', query: 'kube_deployment_created', description: '展示 Deployment 工作负载及其标签。' },
  { id: 'k8s-pod-distribution', category: 'Kubernetes', name: '命名空间 Pod 分布', query: 'sum by (namespace) (kube_pod_info)', description: '按命名空间统计 Pod 分布。' },
  { id: 'k8s-restarts-total', category: 'Kubernetes', name: 'Pod 累计重启次数 Top 10', query: 'topk(10, sum by (namespace, pod) (kube_pod_container_status_restarts_total{pod!=""}))', description: '展示自 Pod 启动以来累计重启次数最多的 Pod，适合快速定位历史重启较多的工作负载。' },
  { id: 'k8s-restarts-hour', category: 'Kubernetes', name: 'Pod 最近 1 小时新增重启 Top 10', query: 'topk(10, sum by (namespace, pod) (increase(kube_pod_container_status_restarts_total{pod!=""}[1h])))', description: '仅统计最近 1 小时新增的重启次数；没有新重启时结果为 0，属于正常现象。' }
]

const selectedMetricTemplate = computed(() => metricTemplates.find((item) => item.id === metricTemplateId.value))
const chartSeries = computed(() => chartResult.value
  .map((row, index) => ({ metric: row.metric || {}, label: chartLabel(row.metric, index), values: (row.values || []).map(([time, value]) => [Number(time), Number(value)]).filter(([time, value]) => Number.isFinite(time) && Number.isFinite(value)) }))
  .filter((item) => item.values.length)
  .slice(0, 16))
const chartBounds = computed(() => {
  const points = chartSeries.value.flatMap((item) => item.values)
  if (!points.length) return { minTime: 0, maxTime: 1, minValue: 0, maxValue: 1 }
  const times = points.map(([time]) => time)
  const values = points.map(([, value]) => value)
  const minValue = Math.min(...values)
  const maxValue = Math.max(...values)
  return { minTime: Math.min(...times), maxTime: Math.max(...times), minValue, maxValue: maxValue === minValue ? minValue + 1 : maxValue }
})
const chartTicks = computed(() => Array.from({ length: 5 }, (_, index) => chartBounds.value.minTime + (chartBounds.value.maxTime - chartBounds.value.minTime) * index / 4))
const sortedChartSeries = computed(() => [...chartSeries.value].sort((left, right) => {
  const leftValue = left.values.at(-1)?.[1] ?? 0
  const rightValue = right.values.at(-1)?.[1] ?? 0
  return seriesSortOrder.value === 'desc' ? rightValue - leftValue : leftValue - rightValue
}))

function metricText(metric) {
  return Object.entries(metric || {})
    .filter(([key]) => key !== '__name__')
    .map(([key, value]) => `${key}="${value}"`)
    .join(', ') || '无标签（汇总结果）'
}

function metricName(metric) {
  return metric?.__name__ || selectedMetricTemplate.value?.name || '汇总结果'
}

function chartLabel(metric, index) {
  const labels = metric || {}
  // Kubernetes 指标通常由同一个 kube-state-metrics 实例采集，单独显示 instance 会让所有曲线同名。
  // 优先使用业务对象标识，再补充采集实例，既能区分 Pod，也能保留排查来源。
  if (labels.pod) {
    const pod = labels.namespace ? `${labels.namespace}/${labels.pod}` : labels.pod
    const container = labels.container ? ` · ${labels.container}` : ''
    const instance = labels.instance ? ` · ${labels.instance}` : ''
    return `${pod}${container}${instance}`
  }
  return labels.instance || labels.namespace || labels.job || metricName(labels) || `序列 ${index + 1}`
}

function formatMetricValue(value) {
  const number = Number(value)
  if (!Number.isFinite(number)) return String(value ?? '-')
  if (Math.abs(number) >= 1000) return number.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
  if (Math.abs(number) >= 1) return number.toLocaleString('zh-CN', { maximumFractionDigits: 3 })
  return number.toPrecision(4)
}

function sortMetricValue(left, right) {
  const leftValue = Number(left.value?.[1])
  const rightValue = Number(right.value?.[1])
  if (!Number.isFinite(leftValue)) return 1
  if (!Number.isFinite(rightValue)) return -1
  return leftValue - rightValue
}

function chartX(time) {
  const bounds = chartBounds.value
  return 38 + (time - bounds.minTime) / (bounds.maxTime - bounds.minTime || 1) * 922
}

function chartY(value) {
  const bounds = chartBounds.value
  return 20 + (1 - (value - bounds.minValue) / (bounds.maxValue - bounds.minValue || 1)) * 198
}

function chartPath(values) {
  return values.map(([time, value], index) => {
    return `${index ? 'L' : 'M'}${chartX(time).toFixed(2)},${chartY(value).toFixed(2)}`
  }).join(' ')
}

function chartTickTime(timestamp) {
  return new Date(timestamp * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false })
}

function chartColor(index) {
  return ['#5b73eb', '#28a881', '#e88d3f', '#d85c73', '#38a5c7', '#8b6ed9', '#b99a2b', '#5d9d59'][index % 8]
}

function updateChartHover(event) {
  const rect = event.currentTarget.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width))
  const bounds = chartBounds.value
  const time = bounds.minTime + ((ratio * 1000 - 38) / 922) * (bounds.maxTime - bounds.minTime)
  const points = chartSeries.value.map((series, index) => {
    const point = series.values.reduce((closest, value) => Math.abs(value[0] - time) < Math.abs(closest[0] - time) ? value : closest, series.values[0])
    return { ...series, color: chartColor(index), time: point[0], value: point[1] }
  }).sort((left, right) => right.value - left.value)
  chartHover.value = { time, position: ratio, points }
}

function toggleSeriesSort() {
  seriesSortOrder.value = seriesSortOrder.value === 'desc' ? 'asc' : 'desc'
}

function formatTimestamp(timestamp) {
  const value = Number(timestamp)
  if (!Number.isFinite(value)) return timestamp || '-'
  return new Date(value * 1000).toLocaleString('zh-CN', { hour12: false })
}

async function loadOptions() {
  const options = await queryMonitorDatasourceOptions()
  datasourceOptions.value = (options || []).filter((item) => ['prometheus', 'victoriametrics'].includes(item.type))
  datasourceId.value = datasourceOptions.value.find((item) => item.isDefault)?.id || datasourceOptions.value[0]?.id
}

function applyMetricTemplate(id) {
  const template = metricTemplates.find((item) => item.id === id)
  if (!template) return
  promql.value = template.query
  rows.value = []
  resultType.value = ''
  resultView.value = 'table'
  chartResult.value = []
}

async function loadChart() {
  if (!datasourceId.value || !promql.value.trim()) return
  chartLoading.value = true
  try {
    const endAt = Math.floor(Date.now() / 1000)
    const duration = { '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800 }[chartRange.value] || 3600
    const data = await queryMonitorPrometheusRange({ datasourceId: datasourceId.value, query: promql.value, startAt: endAt - duration, endAt, stepSeconds: Math.max(15, Math.ceil(duration / 120)) })
    chartResult.value = data.result || []
    chartHover.value = null
  } finally {
    chartLoading.value = false
  }
}

async function changeResultView(view) {
  if (view === 'chart') await loadChart()
}

async function executeQuery() {
  if (!datasourceId.value || !promql.value.trim()) {
    ElMessage.warning('请选择 Prometheus 或 VictoriaMetrics 数据源并输入 PromQL')
    return
  }
  loading.value = true
  try {
    const data = await queryMonitorPrometheus({ datasourceId: datasourceId.value, query: promql.value })
    resultType.value = data.resultType || ''
    rows.value = data.result || []
    chartResult.value = []
    if (resultView.value === 'chart') await loadChart()
    if (historyVisible.value) await loadHistory()
  } finally {
    loading.value = false
  }
}

async function loadHistory() {
  historyLoading.value = true
  try {
    const data = await queryMonitorQueryHistoryList(historyQuery.value)
    historyRows.value = data.list || []
    historyTotal.value = data.total || 0
  } finally {
    historyLoading.value = false
  }
}

async function openHistory() {
  historyVisible.value = true
  await loadHistory()
}

function useHistory(row) {
  const datasource = datasourceOptions.value.find((item) => item.name === row.datasourceName)
  if (!datasource && !datasourceOptions.value.some((item) => item.id === row.datasourceId)) {
    ElMessage.warning('该记录来自非指标数据源，请在日志查询页面中使用')
    return
  }
  datasourceId.value = row.datasourceId || datasource?.id || datasourceId.value
  promql.value = row.query || promql.value
  historyVisible.value = false
}

onMounted(async () => {
  await loadOptions()
  if (!datasourceId.value) ElMessage.warning('请先在数据源管理中配置 Prometheus 或 VictoriaMetrics 数据源')
})
</script>

<template>
  <div class="monitor-query monitor-query-page">
    <div class="query-top">
      <div>
        <p class="query-eyebrow">METRIC EXPLORER</p>
        <h2>即时查询</h2>
        <p>仅支持 Prometheus 与 VictoriaMetrics，输入 PromQL 直接查询指标、调试告警规则和排查现场。</p>
      </div>
      <div class="query-top-actions">
        <el-select v-model="datasourceId" filterable placeholder="选择数据源" style="width: 260px">
          <el-option v-for="item in datasourceOptions" :key="item.id" :label="`${item.name} (${item.type})`" :value="item.id" />
        </el-select>
        <el-button @click="openHistory">查询历史</el-button>
      </div>
    </div>

    <div class="query-editor">
      <div class="template-toolbar">
        <span>内置指标</span>
        <el-select v-model="metricTemplateId" clearable filterable placeholder="选择主机或 Kubernetes 指标" style="width: 310px" @change="applyMetricTemplate">
          <el-option v-for="item in metricTemplates" :key="item.id" :label="`${item.category} / ${item.name}`" :value="item.id" />
        </el-select>
        <span class="template-description">{{ selectedMetricTemplate?.description || '从巡检大屏指标中选择一个查询模板。' }}</span>
      </div>
      <el-input v-model="promql" type="textarea" :rows="5" placeholder="例如：up 或 sum(rate(http_requests_total[5m]))" />
      <div class="query-actions">
        <el-button type="primary" :loading="loading" @click="executeQuery">执行查询</el-button>
        <span>Result Type: {{ resultType || '-' }}</span>
      </div>
    </div>

    <div class="result-toolbar">
      <div><b>查询结果</b><span>{{ rows.length }} 个时间序列</span></div>
      <el-radio-group v-model="resultView" class="result-view-switch" @change="changeResultView">
        <el-radio-button value="table">表格</el-radio-button>
        <el-radio-button value="chart">图表</el-radio-button>
      </el-radio-group>
    </div>

    <section v-if="resultView === 'chart'" class="instant-chart-panel" v-loading="chartLoading || loading">
      <div class="instant-chart-head"><div><h3>指标趋势</h3><p>Prometheus / VictoriaMetrics 时间序列</p></div><div class="chart-actions"><el-select v-model="chartRange" size="small" style="width: 110px" @change="loadChart"><el-option label="最近 1 小时" value="1h" /><el-option label="最近 6 小时" value="6h" /><el-option label="最近 24 小时" value="24h" /><el-option label="最近 7 天" value="7d" /></el-select><el-button size="small" @click="loadChart">刷新图表</el-button></div></div>
      <template v-if="chartSeries.length">
        <div class="line-chart-wrap">
          <svg class="line-chart" viewBox="0 0 1000 260" preserveAspectRatio="none" role="img" aria-label="Metric trend" @mousemove="updateChartHover" @mouseleave="chartHover = null">
            <line v-for="index in 5" :key="`grid-${index}`" x1="38" x2="960" :y1="20 + (index - 1) * 49.5" :y2="20 + (index - 1) * 49.5" class="chart-grid" />
            <text v-for="index in 5" :key="`value-${index}`" x="0" :y="24 + (index - 1) * 49.5" class="chart-value">{{ formatMetricValue(chartBounds.maxValue - (index - 1) * (chartBounds.maxValue - chartBounds.minValue) / 4) }}</text>
            <path v-for="(series, index) in chartSeries" :key="metricText(series.metric)" :d="chartPath(series.values)" class="chart-line" :stroke="chartColor(index)" />
            <template v-if="chartHover"><line :x1="chartX(chartHover.time)" :x2="chartX(chartHover.time)" y1="20" y2="218" class="chart-cursor" /><circle v-for="point in chartHover.points" :key="metricText(point.metric)" :cx="chartX(point.time)" :cy="chartY(point.value)" r="4" :fill="point.color" class="chart-point" /></template>
          </svg>
          <div v-if="chartHover" class="chart-hover-tooltip" :style="{ left: `${Math.min(88, Math.max(8, chartHover.position * 100))}%` }"><b>{{ formatTimestamp(chartHover.time) }}</b><div v-for="point in chartHover.points" :key="metricText(point.metric)"><i :style="{ background: point.color }" /><span>{{ point.label }}</span><strong>{{ formatMetricValue(point.value) }}</strong></div></div>
          <div class="chart-time-axis"><span v-for="tick in chartTicks" :key="tick">{{ chartTickTime(tick) }}</span></div>
        </div>
        <div class="series-list"><div class="series-head"><span>Series</span><button type="button" @click="toggleSeriesSort">Last {{ seriesSortOrder === 'desc' ? '↓' : '↑' }}</button></div><div v-for="series in sortedChartSeries" :key="metricText(series.metric)"><i :style="{ background: chartColor(chartSeries.findIndex((item) => metricText(item.metric) === metricText(series.metric))) }" /><span :title="metricText(series.metric)">{{ metricText(series.metric) }}</span><b>{{ formatMetricValue(series.values.at(-1)?.[1]) }}</b></div></div>
      </template>
      <el-empty v-else :description="chartLoading ? '正在查询时间序列…' : '暂无可绘制的时间序列；请执行查询或检查指标类型'" :image-size="56" />
    </section>

    <el-table v-else v-loading="loading" :data="rows" border>
      <el-table-column label="指标" min-width="220" show-overflow-tooltip>
        <template #default="{ row }"><code class="metric-name">{{ metricName(row.metric) }}</code></template>
      </el-table-column>
      <el-table-column label="标签" min-width="420" show-overflow-tooltip>
        <template #default="{ row }"><span class="metric-labels">{{ metricText(row.metric) }}</span></template>
      </el-table-column>
      <el-table-column label="当前值" width="180" sortable :sort-method="sortMetricValue">
        <template #default="{ row }">{{ row.value?.[1] ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="采样时间" width="190">
        <template #default="{ row }">{{ formatTimestamp(row.value?.[0]) }}</template>
      </el-table-column>
    </el-table>

    <el-drawer v-model="historyVisible" title="查询历史" size="640px">
      <div class="history-toolbar">
        <el-input v-model="historyQuery.keyword" clearable placeholder="搜索查询语句 / 数据源" @keyup.enter="loadHistory" />
        <el-select v-model="historyQuery.status" clearable placeholder="状态" style="width: 120px">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-button type="primary" @click="loadHistory">搜索</el-button>
      </div>
      <el-table v-loading="historyLoading" :data="historyRows" border>
        <el-table-column prop="datasourceName" label="数据源" width="140" />
        <el-table-column prop="queryType" label="类型" width="120" />
        <el-table-column prop="query" label="查询语句" min-width="240" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="90" />
        <el-table-column prop="createTime" label="时间" width="170" />
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="useHistory(row)">使用</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <el-pagination v-model:current-page="historyQuery.pageNum" v-model:page-size="historyQuery.pageSize" :total="historyTotal" layout="total, prev, pager, next" @current-change="loadHistory" @size-change="loadHistory" />
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.monitor-query { display: flex; flex-direction: column; gap: 18px; padding: 24px; background: #fff; border-radius: 18px; box-shadow: 0 12px 30px rgba(36, 54, 90, 0.08); }
.query-top { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.query-top-actions { display: flex; gap: 10px; align-items: center; }
.query-top h2 { margin: 0 0 8px; font-size: 26px; color: #10213f; }
.query-top p { margin: 0; color: #7282a0; }
.query-editor { border: 1px solid #dfe7f2; border-radius: 12px; overflow: hidden; }
.template-toolbar { display: flex; align-items: center; gap: 10px; padding: 10px 12px; background: #f7f9fd; border-bottom: 1px solid #e5ecf6; color: #5b6c88; font-size: 13px; }
.template-toolbar > span:first-child { color: #263d65; font-weight: 600; white-space: nowrap; }
.template-description { flex: 1; color: #8491a9; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.query-editor :deep(.el-textarea__inner) { border: 0; border-radius: 0; font-family: Consolas, Monaco, monospace; font-size: 14px; }
.metric-name { color: #315dcf; font-size: 13px; }
.metric-labels { color: #52637f; font-family: Consolas, Monaco, monospace; font-size: 12px; }
.query-actions { display: flex; justify-content: space-between; align-items: center; padding: 12px; background: #f7f9fd; border-top: 1px solid #e5ecf6; color: #7282a0; }
.result-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 2px 0; }.result-toolbar > div { display: flex; align-items: baseline; gap: 10px; }.result-toolbar b { color: #263d65; }.result-toolbar span { color: #8190a7; font-size: 13px; }.instant-chart-panel { min-height: 310px; padding: 18px 20px; border: 1px solid #dfe7f2; border-radius: 12px; background: #fff; }.instant-chart-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; }.instant-chart-head h3 { margin: 0 0 5px; color: #263d65; font-size: 16px; }.instant-chart-head p { margin: 0; color: #8291a7; font-size: 12px; }.instant-chart-head > span { padding: 5px 9px; border-radius: 5px; color: #4b66b2; background: #edf2ff; font-size: 12px; }.instant-chart { display: flex; align-items: flex-end; gap: 8px; height: 240px; padding: 18px 2px 0; border-bottom: 1px solid #e8edf6; }.metric-bar-item { display: flex; position: relative; flex: 1; flex-direction: column; align-items: center; justify-content: flex-end; min-width: 28px; height: 100%; outline: 0; cursor: help; }.metric-bar-item:focus-visible .metric-bar-track { outline: 2px solid #5a76eb; outline-offset: 2px; }.metric-bar-value { overflow: hidden; width: 100%; margin-bottom: 5px; color: #526786; font-size: 11px; text-align: center; text-overflow: ellipsis; white-space: nowrap; }.metric-bar-track { display: flex; align-items: flex-end; width: 100%; height: 172px; border-radius: 4px 4px 0 0; background: repeating-linear-gradient(to top, #edf1f7 0, #edf1f7 1px, transparent 1px, transparent 43px); }.metric-bar-track span { display: block; width: 100%; min-height: 2px; border-radius: 4px 4px 0 0; background: #5a76eb; transition: background .15s; }.metric-bar-item:hover .metric-bar-track span { background: #3657dc; }.metric-bar-item small { overflow: hidden; width: 100%; margin-top: 6px; color: #8190a7; font-size: 11px; text-align: center; text-overflow: ellipsis; white-space: nowrap; }.metric-tooltip { display: grid; gap: 5px; max-width: 300px; }.metric-tooltip b { font-size: 12px; }.metric-tooltip span { overflow: hidden; color: #d2d9e5; font: 11px/1.4 Consolas, Monaco, monospace; text-overflow: ellipsis; white-space: nowrap; }.metric-tooltip strong { font-size: 14px; }
.history-toolbar { display: flex; gap: 10px; margin-bottom: 14px; }
.pager { display: flex; justify-content: flex-end; margin-top: 14px; }
@media (max-width: 900px) { .instant-chart { overflow-x: auto; }.metric-bar-item { flex: 0 0 44px; } }
.result-view-switch :deep(.el-radio-button__inner){min-width:82px;padding:9px 18px;border-color:#b8c8ef;color:#4663aa;font-size:14px;font-weight:700;box-shadow:none}.result-view-switch :deep(.el-radio-button:first-child .el-radio-button__inner){border-radius:8px 0 0 8px}.result-view-switch :deep(.el-radio-button:last-child .el-radio-button__inner){border-radius:0 8px 8px 0}.result-view-switch :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner){border-color:#5a76eb;background:#5a76eb;color:#fff;box-shadow:-1px 0 0 0 #5a76eb}.instant-chart-panel{min-height:390px}.chart-actions{display:flex;gap:8px;align-items:center}.line-chart-wrap{height:265px;margin-top:18px;padding:10px 12px 0;border:1px solid #e5ebf5;border-radius:8px;background:#fff}.line-chart{display:block;width:100%;height:230px;overflow:visible}.chart-grid{stroke:#e7edf6;stroke-width:1}.chart-value{fill:#8391a7;font-size:11px}.chart-line{fill:none;stroke-width:2.2;vector-effect:non-scaling-stroke}.chart-time-axis{display:flex;justify-content:space-between;padding:3px 3px 0 38px;color:#8391a7;font-size:11px}.series-list{display:flex;flex-direction:column;max-height:145px;margin-top:12px;overflow:auto;border:1px solid #e5ebf5;border-radius:8px}.series-list>div{display:grid;grid-template-columns:10px minmax(0,1fr) auto;align-items:center;gap:8px;padding:7px 10px;border-bottom:1px solid #edf1f7;color:#53657f;font:12px/1.4 Consolas,Monaco,monospace}.series-list>div:last-child{border-bottom:0}.series-list i{width:8px;height:8px;border-radius:50%}.series-list span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.series-list b{color:#2d4e80;font-weight:600}
.line-chart-wrap{position:relative;background:linear-gradient(180deg,#fbfcff,#fff)}.chart-cursor{stroke:#8796b1;stroke-width:1;stroke-dasharray:4 3;vector-effect:non-scaling-stroke}.chart-point{stroke:#fff;stroke-width:2;vector-effect:non-scaling-stroke}.chart-hover-tooltip{position:absolute;z-index:2;top:22px;min-width:210px;max-width:330px;padding:10px 12px;border:1px solid #d8e1f1;border-radius:8px;color:#41536e;background:rgba(255,255,255,.96);box-shadow:0 10px 24px rgba(42,61,98,.18);font-size:12px;pointer-events:none}.chart-hover-tooltip>b{display:block;margin-bottom:7px;color:#263d65}.chart-hover-tooltip>div{display:grid;grid-template-columns:8px minmax(0,1fr) auto;align-items:center;gap:7px;padding:3px 0}.chart-hover-tooltip i{width:7px;height:7px;border-radius:50%}.chart-hover-tooltip span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.chart-hover-tooltip strong{color:#2c4d80}.series-list .series-head{position:sticky;top:0;display:grid;grid-template-columns:minmax(0,1fr) auto;padding:7px 10px;color:#60728e;background:#f6f8fc;font:600 12px/1.4 system-ui,sans-serif}.series-head button{padding:0;border:0;color:#4168bd;background:transparent;font:inherit;cursor:pointer}.series-head button:hover{color:#254fba;text-decoration:underline}
.monitor-query { gap: 16px; padding: 20px; border: 1px solid #dce7f5; border-radius: 14px; background: #f6f9fe; box-shadow: 0 8px 22px rgba(30,79,145,.05); }
.query-top { position: relative; overflow: hidden; padding: 20px 22px; border: 1px solid #d8e6f7; border-radius: 12px; background: linear-gradient(120deg,#fff,#f0f7ff); }
.query-top::before { position:absolute; top:0; left:0; width:100%; height:3px; content:''; background:linear-gradient(90deg,#1e40af,#3b82f6 58%,#d97706 58%,#d97706 66%,transparent 66%); }
.query-eyebrow { margin:0 0 5px !important; color:#4771ba !important; font-size:11px !important; font-weight:800; letter-spacing:.1em; }
.query-top h2 { margin:0 0 6px !important; color:#173b76 !important; font-size:25px !important; letter-spacing:-.02em; }
.query-top p:not(.query-eyebrow) { color:#687e9d !important; line-height:1.55; }
.query-editor { border: 1px solid #223c63 !important; border-radius: 12px !important; background:#122440; box-shadow:0 8px 18px rgba(19,45,82,.12); }
.template-toolbar { min-height:44px; padding:9px 12px; border-bottom-color:#27466f !important; background:#172e4f !important; color:#b9cae3 !important; }
.template-toolbar > span:first-child { color:#e4edff !important; }
.template-description { color:#9bb0cf !important; }
.query-editor :deep(.el-textarea__inner) { min-height:138px; padding:16px; color:#e6efff; background:#10213b !important; box-shadow:none !important; font:14px/1.7 Consolas,Monaco,monospace; }
.query-editor :deep(.el-textarea__inner)::placeholder { color:#8199bd; }
.query-actions { border-top-color:#27466f !important; background:#172e4f !important; color:#b9cae3 !important; }
.query-actions .el-button--primary { --el-button-bg-color:#3b74e8; --el-button-border-color:#3b74e8; }
.result-toolbar { padding: 13px 16px; border:1px solid #dce7f5; border-radius:10px; background:#fff; }
.monitor-query-page .query-editor { border: 1px solid #d9e6f5 !important; background: #fff !important; box-shadow: 0 5px 16px rgba(30,79,145,.045) !important; }
.monitor-query-page .template-toolbar { border-bottom-color: #e1eaf6 !important; background: #f6f9ff !important; color: #627694 !important; }
.monitor-query-page .template-toolbar > span:first-child { color: #234b86 !important; }
.monitor-query-page .template-description { color: #7d8da5 !important; }
.monitor-query-page .query-editor :deep(.el-textarea__inner) { min-height: 138px; color: #203b61; background: #fbfdff !important; box-shadow: inset 3px 0 0 #3b82f6 !important; font: 14px/1.7 Consolas, Monaco, monospace; }
.monitor-query-page .query-editor :deep(.el-textarea__inner)::placeholder { color: #8a9ab2; }
.monitor-query-page .query-actions { border-top-color: #e1eaf6 !important; background: #f6f9ff !important; color: #6e819f !important; }
</style>
