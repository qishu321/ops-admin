<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { queryAssetDatabaseMetrics } from '../../api/asset'

const props = defineProps({ databaseId: { type: Number, required: true }, enabled: Boolean })
const loading = ref(false)
const data = ref({ metrics: {} })
const activeRange = ref('1h')
const customRange = ref([])
let timer
let loadVersion = 0

const ranges = [['1h', '1小时'], ['3h', '3小时'], ['6h', '6小时'], ['12h', '12小时'], ['1d', '1天'], ['3d', '3天'], ['7d', '7天'], ['14d', '14天']]
const cards = [
  ['qps', '每秒查询数', '#4268f5', value => `${format(value, 2)}`],
  ['connections', '连接数', '#16b887', value => `${format(value, 0)}`],
  ['threads', '运行线程', '#8a61e8', value => `${format(value, 0)}`],
  ['bytesIn', '入流量', '#12a9b8', value => rate(value)],
  ['bytesOut', '出流量', '#f29b3a', value => rate(value)],
  ['slowQueries', '慢查询 / 秒', '#f15d68', value => `${format(value, 3)}`],
  ['uptime', '运行时长', '#4f80dc', value => duration(value)],
  ['cacheHitRate', '缓存命中率', '#1cab72', value => `${format(value, 2)}%`]
]
const chartRows = [
  { key: 'qps', title: '每秒查询数', color: '#416cf4' },
  { key: 'connections', title: '连接数', color: '#17b989' },
  { key: 'traffic', title: '每秒网络流量', color: '#16a9b8', extra: { key: 'bytesOut', color: '#8a61e8', label: '出' } },
  { key: 'slowQueries', title: '慢查询趋势', color: '#f15d68' }
]

function format(value, digits = 1) { return Number(value || 0).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: 0 }) }
function rate(value) {
  value = Number(value || 0)
  if (value < 1024) return `${format(value, 0)} B/s`
  if (value < 1024 ** 2) return `${format(value / 1024, 1)} KB/s`
  return `${format(value / 1024 ** 2, 1)} MB/s`
}
function duration(value) {
  value = Math.max(0, Number(value || 0)); const days = Math.floor(value / 86400); const hours = Math.floor((value % 86400) / 3600); const mins = Math.floor((value % 3600) / 60)
  return days ? `${days}天 ${hours}小时` : hours ? `${hours}小时 ${mins}分` : `${mins}分`
}
function latest(key) { return data.value.metrics?.[key]?.latest }
function points(key) { return data.value.metrics?.[key]?.points || [] }
function path(key, width = 640, height = 178) {
  const values = points(key); if (!values.length) return ''
  const max = Math.max(...values.map(item => Number(item.value)), 1); const min = Math.min(...values.map(item => Number(item.value)), 0)
  const spread = max - min || 1
  return values.map((item, index) => `${index ? 'L' : 'M'} ${28 + index * ((width - 48) / Math.max(values.length - 1, 1))} ${height - 18 - ((Number(item.value) - min) / spread) * (height - 46)}`).join(' ')
}
function labels(key) {
  const values = points(key); if (!values.length) return []
  const indexes = [...new Set([0, Math.floor((values.length - 1) / 2), values.length - 1])]
  return indexes.map(index => ({ x: 28 + index * (592 / Math.max(values.length - 1, 1)), label: new Date(values[index].timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }))
}
function valueFor(key) { return cards.find(item => item[0] === key)?.[3](latest(key)) || format(latest(key)) }
async function load() {
  const version = ++loadVersion
  if (!props.enabled || !props.databaseId) {
    data.value = { metrics: {} }
    return
  }
  loading.value = true
  try {
    const params = { id: props.databaseId, range: activeRange.value }
    if (activeRange.value === 'custom' && customRange.value?.length === 2) { params.start = Number(customRange.value[0]); params.end = Number(customRange.value[1]) }
    const response = await queryAssetDatabaseMetrics(params)
    if (version !== loadVersion) return
    data.value = response
    if (response.collectError) ElMessage.warning(`指标采集失败：${response.collectError}`)
  } finally { if (version === loadVersion) loading.value = false }
}
function chooseRange(key) { activeRange.value = key; load() }
function applyCustom() { if (customRange.value?.length === 2) { activeRange.value = 'custom'; load() } }
watch(() => [props.databaseId, props.enabled], load, { immediate: true })
onMounted(() => { timer = window.setInterval(load, 30000) })
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <section class="database-metrics" v-loading="loading">
    <header class="metrics-header"><div><h3>运行指标</h3><p>开启后直接通过数据库原生查询采集；每 30 秒刷新一次，指标仅保留在本平台。</p></div><div class="range-tools"><el-button-group><el-button v-for="item in ranges" :key="item[0]" :type="activeRange === item[0] ? 'primary' : 'default'" @click="chooseRange(item[0])">{{ item[1] }}</el-button></el-button-group><el-date-picker v-model="customRange" type="datetimerange" value-format="x" start-placeholder="开始" end-placeholder="结束" style="width: 300px" @change="applyCustom" /></div></header>
    <el-alert v-if="!enabled" title="监控仪表盘未启用" description="请在数据库编辑页启用监控仪表盘；启用后将用该数据库的已配置连接实时读取运行指标。" type="info" :closable="false" show-icon />
    <el-alert v-else-if="data.collectError && data.status !== 'available'" title="暂时无法采集数据库指标" :description="data.collectError" type="warning" :closable="false" show-icon />
    <template v-else-if="enabled"><div class="metric-summary"><article v-for="item in cards" :key="item[0]" class="summary-card"><span>{{ item[1] }}</span><strong :style="{ color: item[2] }">{{ item[3](latest(item[0])) }}</strong></article></div><div class="chart-grid"><article v-for="chart in chartRows" :key="chart.key" class="chart-card"><header><h4>{{ chart.title }}</h4><span v-if="chart.extra"><i :style="{ background: chart.color }"></i>入 <i :style="{ background: chart.extra.color }"></i>{{ chart.extra.label }}</span><small v-else>{{ valueFor(chart.key) }}</small></header><svg viewBox="0 0 640 178" preserveAspectRatio="none"><line v-for="y in [34, 78, 122, 166]" :key="y" x1="28" :y1="y" x2="620" :y2="y"/><path :d="path(chart.key)" :stroke="chart.color"/><path v-if="chart.extra" :d="path(chart.extra.key)" :stroke="chart.extra.color"/></svg><footer><span v-for="tick in labels(chart.key)" :key="tick.x" :style="{ left: `${tick.x / 6.4}%` }">{{ tick.label }}</span></footer></article></div></template>
  </section>
</template>

<style scoped>
.database-metrics{padding:20px;border:1px solid #e3eaf5;border-radius:10px;background:#fff}.metrics-header{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin-bottom:18px}.metrics-header h3{margin:0;color:#13294b;font-size:17px}.metrics-header p{margin:6px 0 0;color:#7b8ca8;font-size:13px}.range-tools{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.metric-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:16px}.summary-card{min-height:86px;padding:16px;border:1px solid #e3eaf5;border-radius:10px;background:linear-gradient(145deg,#fff,#f8fbff)}.summary-card span{display:block;color:#667895;font-size:13px}.summary-card strong{display:block;margin-top:8px;font-size:25px;line-height:1.1}.chart-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.chart-card{padding:16px;border:1px solid #e3eaf5;border-radius:10px;background:#fff}.chart-card header{display:flex;justify-content:space-between;align-items:center}.chart-card h4{margin:0;color:#213856;font-size:15px}.chart-card small,.chart-card header span{color:#7b8ca8;font-size:12px}.chart-card i{display:inline-block;width:8px;height:8px;border-radius:50%;margin:0 4px 1px 8px}.chart-card svg{display:block;width:100%;height:190px;margin-top:8px}.chart-card line{stroke:#e8eef7;stroke-width:1}.chart-card path{fill:none;stroke-width:3;stroke-linecap:round;stroke-linejoin:round}.chart-card footer{position:relative;height:17px;color:#8997aa;font-size:11px}.chart-card footer span{position:absolute;transform:translateX(-50%);white-space:nowrap}@media(max-width:1120px){.metrics-header{flex-direction:column}.metric-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.chart-grid{grid-template-columns:1fr}}@media(max-width:640px){.metric-summary{grid-template-columns:1fr}}
</style>
