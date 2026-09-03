<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { opsAppReleaseInfo, queryOpsApplicationOptions, queryOpsAppReleaseList, retryOpsAppRelease } from '../../api/ops'

const route = useRoute()
const loading = ref(false)
const rows = ref([])
const total = ref(0)
const appOptions = ref([])
const detailVisible = ref(false)
const detailLoading = ref(false)
const currentDetail = ref({})
const logKeyword = ref('')
const dateRange = ref([])
const logBodyRef = ref()
let detailPollTimer = null
let refreshingDetail = false

const query = reactive({ pageNum: 1, pageSize: 10, appId: undefined, env: '', keyword: '', status: '' })

async function loadApps() { appOptions.value = await queryOpsApplicationOptions() }
async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsAppReleaseList({ ...query, startTime: dateRange.value?.[0] || '', endTime: dateRange.value?.[1] || '' })
    rows.value = data.list || []
    total.value = data.total || 0
  } finally { loading.value = false }
}
function search() { query.pageNum = 1; loadData() }
function reset() { Object.assign(query, { pageNum: 1, appId: undefined, env: '', status: '', keyword: '' }); dateRange.value = []; loadData() }
function toggleStatus(status) { query.status = query.status === status ? '' : status; search() }
function statusType(status) { return ({ success: 'success', running: 'warning', failed: 'danger', waiting: 'info' })[status] || 'info' }
function statusText(status) { return ({ success: '成功', running: '执行中', failed: '失败', waiting: '等待' })[status] || status || '-' }
function stageText(stage) { return ({ checkout: '拉取代码', build: '应用构建', post_build: '构建后操作', prepare: '准备执行', done: '已完成' })[stage] || stage || '-' }
function sourceText(row) { return row.repoType === 'svn' ? 'SVN 更新' : 'Git 拉取' }
function versionText(row) { return row.repoType === 'svn' ? (row.branch || 'HEAD') : (row.branch || '-') }
function durationText(ms) { if (!ms) return '-'; const seconds = Math.max(0, Math.round(ms / 1000)); return seconds < 60 ? `${seconds}秒` : `${Math.floor(seconds / 60)}分${seconds % 60}秒` }
function formatDateTime(value) {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const match = raw.match(/^(\d{4}-\d{2}-\d{2})[T\s](\d{2}:\d{2}:\d{2})/)
  if (match) return `${match[1]} ${match[2]}`
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}
function releaseTitle(row) { return `${row.buildTaskName || row.appName || '构建任务'} #${row.id || ''}` }
function stages(row) {
  const stateFor = (name) => {
    if (row.status === 'failed' && row.stage === name) return 'failed'
    if (row.status === 'running' && row.stage === name) return 'running'
    if ((name === 'build' && row.stage === 'checkout') || (name === 'post_build' && ['checkout', 'build'].includes(row.stage))) return 'waiting'
    return row.status === 'waiting' ? 'waiting' : 'success'
  }
  const list = [{ key: 'checkout', name: sourceText(row) }, { key: 'build', name: '应用构建' }]
  if (row.deployLog || row.stage === 'post_build') list.push({ key: 'post_build', name: '构建后操作' })
  return list.map((item) => ({ ...item, status: stateFor(item.key) }))
}
const detailLogs = computed(() => {
  const content = [currentDetail.value.buildLog || '', currentDetail.value.deployLog || ''].filter(Boolean).join('\n')
  const keyword = logKeyword.value.trim().toLowerCase()
  return keyword ? content.split('\n').filter((line) => line.toLowerCase().includes(keyword)).join('\n') : content
})
const logFileName = computed(() => `${releaseTitle(currentDetail.value)}.log`.replace(/[\\/:*?"<>|]/g, '_'))

function stopDetailPolling() {
  if (detailPollTimer) window.clearInterval(detailPollTimer)
  detailPollTimer = null
}
async function scrollLogToLatest() {
  await nextTick()
  if (!logKeyword.value && logBodyRef.value) logBodyRef.value.scrollTop = logBodyRef.value.scrollHeight
}
async function refreshDetail() {
  if (!currentDetail.value?.id || refreshingDetail) return
  refreshingDetail = true
  try {
    currentDetail.value = await opsAppReleaseInfo(currentDetail.value.id)
    await scrollLogToLatest()
    if (currentDetail.value.status !== 'running') {
      stopDetailPolling()
      await loadData()
    }
  } finally { refreshingDetail = false }
}
function startDetailPolling() {
  stopDetailPolling()
  if (currentDetail.value.status === 'running') detailPollTimer = window.setInterval(refreshDetail, 2000)
}
async function openDetail(row) {
  detailVisible.value = true
  detailLoading.value = true
  logKeyword.value = ''
  try {
    currentDetail.value = await opsAppReleaseInfo(row.id)
    await scrollLogToLatest()
    startDetailPolling()
  } finally { detailLoading.value = false }
}
async function retry(row) {
  await ElMessageBox.confirm(`将按「${releaseTitle(row)}」的任务、分支和原始参数重新执行构建。`, '确认重试构建', { type: 'warning', confirmButtonText: '立即重试' })
  await retryOpsAppRelease(row.id)
  ElMessage.success('已创建新的构建任务')
  await loadData()
}
async function copyLog() { await navigator.clipboard.writeText(detailLogs.value || ''); ElMessage.success('日志已复制') }
function downloadLog() { const blob = new Blob([detailLogs.value || ''], { type: 'text/plain;charset=utf-8' }); const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = logFileName.value; link.click(); URL.revokeObjectURL(url) }
watch(detailVisible, (visible) => { if (!visible) stopDetailPolling() })
onBeforeUnmount(stopDetailPolling)
onMounted(async () => { if (route.query.appId) query.appId = Number(route.query.appId); if (route.query.env) query.env = String(route.query.env); if (route.query.keyword) query.keyword = String(route.query.keyword); await Promise.all([loadApps(), loadData()]) })
</script>

<template>
  <div class="history-page">
    <div class="app-header"><div><h1>构建历史</h1><p>集中查看构建状态、失败原因与完整日志。</p></div><el-button @click="loadData">刷新</el-button></div>
    <section class="filter-panel">
      <div class="filter-main">
        <el-select v-model="query.appId" clearable filterable placeholder="全部应用"><el-option v-for="item in appOptions" :key="item.id" :label="item.name" :value="item.id" /></el-select>
        <el-select v-model="query.env" clearable placeholder="全部环境"><el-option label="dev" value="dev" /><el-option label="test" value="test" /><el-option label="prod" value="prod" /></el-select>
        <div class="build-history-date-range">
          <el-date-picker v-model="dateRange" type="datetimerange" value-format="YYYY-MM-DD HH:mm:ss" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" />
        </div>
        <el-input v-model="query.keyword" clearable placeholder="搜索任务、版本、摘要" @keyup.enter="search" />
        <el-button type="primary" @click="search">搜索</el-button><el-button @click="reset">重置</el-button>
      </div>
      <div class="quick-filters"><span>快速筛选</span><el-button :type="query.status === 'running' ? 'warning' : 'default'" plain @click="toggleStatus('running')">执行中</el-button><el-button :type="query.status === 'failed' ? 'danger' : 'default'" plain @click="toggleStatus('failed')">仅看失败</el-button><el-button :type="query.status === 'success' ? 'success' : 'default'" plain @click="toggleStatus('success')">仅看成功</el-button></div>
    </section>
    <section v-loading="loading" class="history-list">
      <el-empty v-if="!rows.length" description="暂无符合条件的构建记录" />
      <el-table v-else :data="rows" class="history-table">
        <el-table-column label="状态" width="104"><template #default="{ row }"><el-tag :type="statusType(row.status)" effect="light">{{ statusText(row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="任务 / 应用" min-width="220"><template #default="{ row }"><div class="task-cell"><strong>{{ releaseTitle(row) }}</strong><span>{{ row.appName || '-' }} · {{ row.env || '-' }}</span></div></template></el-table-column>
        <el-table-column label="代码版本" min-width="170"><template #default="{ row }"><div class="code-cell"><span>{{ row.repoType === 'svn' ? 'SVN' : 'Git' }} · {{ versionText(row) }}</span><code>{{ row.commitId || '尚未获取提交版本' }}</code></div></template></el-table-column>
        <el-table-column label="当前阶段" min-width="220"><template #default="{ row }"><div class="stage-cell"><strong>{{ stageText(row.stage) }}</strong><span>{{ row.summary || '-' }}</span></div></template></el-table-column>
        <el-table-column label="开始时间" width="190"><template #default="{ row }">{{ formatDateTime(row.createTime) }}</template></el-table-column><el-table-column label="耗时" width="100"><template #default="{ row }">{{ durationText(row.durationMs) }}</template></el-table-column>
        <el-table-column label="操作" width="156" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">查看详情</el-button><el-button v-if="row.status === 'failed' && row.buildTaskId" link type="danger" @click="retry(row)">重试</el-button></template></el-table-column>
      </el-table>
      <div class="pager"><el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :page-sizes="[10, 20, 50]" layout="total, sizes, prev, pager, next" :total="total" @current-change="loadData" @size-change="search" /></div>
    </section>
    <el-drawer v-model="detailVisible" :title="releaseTitle(currentDetail)" size="min(760px, 100vw)" class="build-detail-drawer">
      <div v-loading="detailLoading" class="detail-content"><template v-if="currentDetail.id">
        <div class="detail-top"><el-tag :type="statusType(currentDetail.status)">{{ statusText(currentDetail.status) }}</el-tag><span>{{ currentDetail.summary || '-' }}</span><strong>耗时 {{ durationText(currentDetail.durationMs) }}</strong></div>
        <div class="detail-meta"><span>应用<b>{{ currentDetail.appName || '-' }}</b></span><span>环境<b>{{ currentDetail.env || '-' }}</b></span><span>代码版本<b>{{ currentDetail.repoType === 'svn' ? 'SVN ' : 'Git ' }}{{ versionText(currentDetail) }}</b></span><span>提交版本<b>{{ currentDetail.commitId || '-' }}</b></span><span>执行路径<b>{{ currentDetail.workspace || '-' }}</b></span><span>开始时间<b>{{ formatDateTime(currentDetail.createTime) }}</b></span></div>
        <section class="detail-section"><div class="section-head"><strong>构建阶段</strong><span>当前：{{ stageText(currentDetail.stage) }}</span></div><div class="stage-track"><div v-for="stage in stages(currentDetail)" :key="stage.key" class="stage-node" :class="stage.status"><i></i><strong>{{ stage.name }}</strong><span>{{ statusText(stage.status) }}</span></div></div></section>
        <section class="detail-section"><div class="section-head"><strong>完整日志</strong><div><el-input v-model="logKeyword" clearable size="small" placeholder="搜索日志" /><el-button link type="primary" @click="copyLog">复制</el-button><el-button link type="primary" @click="downloadLog">下载</el-button></div></div><pre ref="logBodyRef" class="log-body">{{ detailLogs || '暂无日志输出' }}</pre></section>
        <div class="detail-actions"><el-button v-if="currentDetail.status === 'failed' && currentDetail.buildTaskId" type="danger" plain @click="retry(currentDetail)">按原配置重试</el-button></div>
      </template></div>
    </el-drawer>
  </div>
</template>

<style scoped>
.history-page { padding: 24px; }.app-header, .filter-panel, .history-list { border: 1px solid #e1eaf7; border-radius: 12px; background: #fff; }.app-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; padding: 24px; background: linear-gradient(120deg, #fff, #f4f8ff); }.app-header h1 { margin: 0; color: #10213d; font-size: 28px; }.app-header p { margin: 8px 0 0; color: #70819e; }.filter-panel { margin-bottom: 16px; padding: 16px 20px; }.filter-main, .quick-filters { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }.filter-main :deep(.el-select) { width: 180px; }.filter-main :deep(.el-input) { width: 240px; }.build-history-date-range { flex: 0 0 480px; width: 480px; }.build-history-date-range :deep(.el-date-editor) { width: 100% !important; }.quick-filters { margin-top: 12px; color: #74839c; font-size: 13px; }.quick-filters .el-button { margin-left: 0; }.history-list { padding: 12px 16px 18px; }.history-table { width: 100%; }.task-cell, .code-cell, .stage-cell { display: grid; gap: 4px; }.task-cell strong, .stage-cell strong { color: #172a47; }.task-cell span, .stage-cell span, .code-cell code { overflow: hidden; color: #71819c; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }.code-cell code { color: #496484; }.pager { display: flex; justify-content: flex-end; padding-top: 16px; }.detail-content { min-height: 100%; }.detail-top { display: flex; align-items: center; gap: 10px; padding-bottom: 18px; border-bottom: 1px solid #e8eef7; color: #647691; }.detail-top strong { margin-left: auto; color: #1e3352; }.detail-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; padding: 18px 0; }.detail-meta span { display: grid; gap: 4px; color: #74839c; font-size: 12px; }.detail-meta b { overflow-wrap: anywhere; color: #263b5b; font-size: 13px; font-weight: 500; }.detail-section { margin-top: 18px; }.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: #6e809b; }.section-head strong { color: #1a2f4f; }.section-head > div { display: flex; align-items: center; gap: 6px; }.section-head :deep(.el-input) { width: 160px; }.stage-track { display: flex; gap: 8px; margin-top: 12px; }.stage-node { flex: 1; min-width: 0; padding: 12px; border: 1px solid #e2eaf5; border-radius: 8px; background: #fafcff; }.stage-node i { display: inline-block; width: 8px; height: 8px; margin-right: 6px; border-radius: 50%; background: #b5c2d5; }.stage-node strong, .stage-node span { display: block; }.stage-node span { margin-top: 5px; color: #8290a6; font-size: 12px; }.stage-node.success i { background: #42b883; }.stage-node.running i { background: #e6a23c; }.stage-node.failed { border-color: #fecaca; background: #fff8f8; }.stage-node.failed i { background: #f56c6c; }.log-body { min-height: 320px; max-height: calc(100vh - 540px); margin: 12px 0 0; padding: 16px; overflow: auto; border-radius: 8px; background: #101b30; color: #d9e6f5; font-family: Consolas, Monaco, monospace; font-size: 12px; line-height: 1.65; white-space: pre-wrap; }.detail-actions { margin-top: 20px; }@media (max-width: 900px) { .history-page { padding: 14px; }.filter-main :deep(.el-select), .filter-main :deep(.el-input), .filter-main :deep(.el-date-editor) { width: 100%; }.detail-meta { grid-template-columns: 1fr; }.stage-track { flex-direction: column; } }
</style>
