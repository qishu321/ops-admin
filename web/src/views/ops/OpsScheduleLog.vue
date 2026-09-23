<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getHTTPProbeLogRetention,
  opsScheduleLogInfo,
  queryOpsScheduleHTTPTaskOptions,
  queryOpsScheduleLogList,
  saveHTTPProbeLogRetention,
} from '../../api/ops'

const router = useRouter()
const activeTab = ref('script')
const loading = ref(false)
const detailLoading = ref(false)
const detailVisible = ref(false)
const rows = ref([])
const total = ref(0)
const detail = ref(null)
const httpTasks = ref([])
const retentionVisible = ref(false)
const retentionLoading = ref(false)
const retentionSaving = ref(false)
const retentionDays = ref(7)
const retentionSetting = ref(null)
const retentionError = ref(false)
let listRequest = 0

const filters = reactive({
  script: { pageNum: 1, pageSize: 10, keyword: '', status: '' },
  http: { pageNum: 1, pageSize: 10, keyword: '', status: '', taskId: undefined }
})
const query = computed(() => filters[activeTab.value])
function requestParams() {
  const current = query.value
  return {
    keyword: current.keyword,
    taskType: activeTab.value,
    taskId: activeTab.value === 'http' ? current.taskId : undefined,
    status: current.status
  }
}

async function loadData() {
  const request = ++listRequest
  const tab = activeTab.value
  const current = query.value
  const params = requestParams()
  loading.value = true
  try {
    const data = await queryOpsScheduleLogList({ ...params, pageNum: current.pageNum, pageSize: current.pageSize })
    if (request !== listRequest || tab !== activeTab.value) return
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    if (request === listRequest) loading.value = false
  }
}

function changeTab() {
  rows.value = []
  total.value = 0
  loadData()
}

function search() {
  query.value.pageNum = 1
  loadData()
}

function resetQuery() {
  Object.assign(query.value, { pageNum: 1, pageSize: 10, keyword: '', status: '' })
  if (activeTab.value === 'http') {
    query.value.taskId = undefined
  }
  loadData()
}

async function openRetentionSettings() {
  retentionVisible.value = true
  retentionLoading.value = true
  retentionSetting.value = null
  retentionError.value = false
  try {
    retentionSetting.value = await getHTTPProbeLogRetention()
    retentionDays.value = retentionSetting.value?.retentionDays || 7
  } catch {
    retentionError.value = true
  } finally {
    retentionLoading.value = false
  }
}

async function saveRetentionSettings() {
  const days = Number(retentionDays.value)
  if (!Number.isInteger(days) || days < 1 || days > 365) {
    ElMessage.warning('保留天数须在 1 到 365 天之间')
    return
  }
  if (retentionSetting.value && days < retentionSetting.value.retentionDays) {
    const confirmed = await ElMessageBox.confirm(
      `将保留期从 ${retentionSetting.value.retentionDays} 天缩短为 ${days} 天后，超期的 HTTP 探针日志会在后台永久删除，无法恢复。确认保存吗？`,
      '确认缩短保留期',
      { type: 'warning', confirmButtonText: '确认并保存' }
    ).then(() => true, () => false)
    if (!confirmed) return
  }
  retentionSaving.value = true
  try {
    retentionSetting.value = await saveHTTPProbeLogRetention({ retentionDays: days })
    retentionVisible.value = false
    ElMessage.success('保留策略已保存，后台开始清理过期探针日志')
  } finally {
    retentionSaving.value = false
  }
}

async function openDetail(row) {
  detail.value = null
  detailVisible.value = true
  detailLoading.value = true
  try {
    detail.value = await opsScheduleLogInfo(row.id)
  } finally {
    detailLoading.value = false
  }
}

function triggerLabel(value) {
  return value === 'manual' ? '手动触发' : '定时调度'
}

function statusTagType(value) {
  if (value === 'success') return 'success'
  if (value === 'running' || value === 'partial') return 'warning'
  return 'danger'
}

function statusLabel(value) {
  return { success: '成功', failed: '失败', running: '执行中', partial: '部分成功' }[value] || value || '-'
}

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

async function copyText(value, message) {
  try {
    await navigator.clipboard.writeText(value || '')
    ElMessage.success(message)
  } catch {
    ElMessage.error('复制失败')
  }
}

function openExecHistory(execTaskId) {
  if (!execTaskId) return
  router.push({ path: '/ops/quick-exec/history', query: { taskId: String(execTaskId) } })
}

async function loadHTTPTasks() {
  httpTasks.value = await queryOpsScheduleHTTPTaskOptions() || []
}

onMounted(() => {
  loadData()
  loadHTTPTasks()
})
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">任务日志</h2>
        <p class="page-desc">按任务类型查看调度结果，定位异常并追溯每次执行。</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="log-tabs" @tab-change="changeTab">
      <el-tab-pane label="脚本任务" name="script" />
      <el-tab-pane label="HTTP 探针" name="http" />
    </el-tabs>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索任务名称 / 摘要" style="width: 260px" @keyup.enter="search" />
        <el-select v-if="activeTab === 'http'" v-model="query.taskId" clearable filterable placeholder="全部探针" style="width: 210px" @change="search">
          <el-option v-for="task in httpTasks" :key="task.id" :label="task.name" :value="task.id" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="全部状态" style="width: 125px" @change="search">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option v-if="activeTab === 'script'" label="部分成功" value="partial" />
          <el-option label="执行中" value="running" />
        </el-select>
        <el-button type="primary" @click="search">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
      <el-button v-if="activeTab === 'http'" @click="openRetentionSettings">日志保留设置</el-button>
    </div>

    <el-table v-loading="loading" :data="rows" border class="schedule-log-table" empty-text="当前条件下没有任务日志">
      <el-table-column prop="taskName" :label="activeTab === 'http' ? '探针名称' : '任务名称'" min-width="190" show-overflow-tooltip />
      <el-table-column v-if="activeTab === 'script'" label="触发方式" width="105"><template #default="{ row }">{{ triggerLabel(row.triggerType) }}</template></el-table-column>
      <el-table-column label="执行状态" width="105" align="center"><template #default="{ row }"><el-tag :type="statusTagType(row.status)" effect="light">{{ statusLabel(row.status) }}</el-tag></template></el-table-column>
      <el-table-column v-if="activeTab === 'http'" label="状态码" width="115" align="center"><template #default="{ row }">{{ row.actualStatus || '-' }} / {{ row.expectedStatus || '-' }}</template></el-table-column>
      <el-table-column v-if="activeTab === 'script'" prop="summary" label="执行摘要" min-width="300" show-overflow-tooltip />
      <el-table-column v-if="activeTab === 'http'" prop="attemptCount" label="尝试次数" width="95" align="center" />
      <el-table-column prop="durationMs" label="耗时(ms)" width="105" align="right" />
      <el-table-column label="开始时间" width="180"><template #default="{ row }">{{ formatDateTime(row.startedAt) }}</template></el-table-column>
      <el-table-column v-if="activeTab === 'script'" label="结束时间" width="180"><template #default="{ row }">{{ formatDateTime(row.finishedAt) }}</template></el-table-column>
      <el-table-column label="操作" width="90" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">详情</el-button></template></el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :total="total" layout="total, sizes, prev, pager, next" @current-change="loadData" @size-change="search" />
    </div>

    <el-drawer v-model="detailVisible" title="任务日志详情" size="min(880px, 92vw)">
      <div v-loading="detailLoading" class="log-detail">
        <template v-if="detail">
          <div class="detail-grid">
            <div><span>任务名称</span><strong>{{ detail.taskName }}</strong></div>
            <div><span>任务类型</span><strong>{{ detail.taskType === 'http' ? 'HTTP 探针' : '脚本任务' }}</strong></div>
            <div><span>执行状态</span><strong>{{ statusLabel(detail.status) }}</strong></div>
            <div><span>触发方式</span><strong>{{ triggerLabel(detail.triggerType) }}</strong></div>
            <div><span>开始时间</span><strong>{{ formatDateTime(detail.startedAt) }}</strong></div>
            <div><span>结束时间</span><strong>{{ formatDateTime(detail.finishedAt) }}</strong></div>
            <div><span>耗时</span><strong>{{ detail.durationMs }} ms</strong></div>
            <div v-if="detail.taskType === 'http'"><span>尝试次数</span><strong>{{ detail.attemptCount || 1 }}</strong></div>
            <div v-if="detail.taskType === 'http'"><span>状态码（实际 / 期望）</span><strong>{{ detail.actualStatus || '-' }} / {{ detail.expectedStatus || '-' }}</strong></div>
          </div>
          <el-alert :title="detail.summary || '无摘要'" type="info" :closable="false" />
          <div class="detail-actions">
            <el-button v-if="detail.execTaskId" type="primary" link @click="openExecHistory(detail.execTaskId)">前往快速执行详情</el-button>
            <el-button link @click="copyText(detail.detail || '', '详细输出已复制')">复制详细输出</el-button>
            <el-button v-if="detail.taskType === 'http'" link @click="copyText(detail.responseBody || '', 'HTTP 响应已复制')">复制 HTTP 响应</el-button>
          </div>
          <div class="detail-section"><h4>详细输出</h4><pre>{{ detail.detail || '-' }}</pre></div>
          <div v-if="detail.taskType === 'http'" class="detail-section"><h4>HTTP 响应</h4><pre>{{ detail.responseBody || '-' }}</pre></div>
        </template>
      </div>
    </el-drawer>

    <el-dialog v-model="retentionVisible" title="HTTP 探针日志保留设置" width="min(520px, 92vw)">
      <div v-loading="retentionLoading" class="retention-dialog">
        <el-alert v-if="retentionError" title="读取保留设置失败，请关闭后重试" type="warning" :closable="false" show-icon />
        <el-form label-width="110px">
          <el-form-item label="保留天数" required>
            <el-input-number v-model="retentionDays" :min="1" :max="365" :step="1" style="width: 180px" />
            <span class="retention-unit">天</span>
          </el-form-item>
        </el-form>
        <p>默认保留最近 7 天。服务启动时及之后每小时清理一次超过保留期的 HTTP 探针日志，每次分批删除。脚本任务日志不受影响。</p>
        <p class="retention-warning">过期日志删除后无法恢复；修改设置会立即触发一次后台清理。</p>
        <p v-if="retentionSetting?.lastCleanupAt" class="retention-last">上次清理：{{ formatDateTime(retentionSetting.lastCleanupAt) }}，删除 {{ retentionSetting.lastDeletedCount || 0 }} 条</p>
      </div>
      <template #footer>
        <el-button @click="retentionVisible = false">取消</el-button>
        <el-button type="primary" :loading="retentionSaving" :disabled="retentionLoading || !retentionSetting" @click="saveRetentionSettings">保存设置</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-title { margin: 0 0 8px; font-size: 22px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.log-tabs { margin-bottom: -14px; }
.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.toolbar-left { display: flex; gap: 12px; flex-wrap: wrap; }
.retention-dialog { min-height: 145px; }
.retention-dialog p { margin: 8px 0; color: #66748f; font-size: 13px; line-height: 1.6; }
.retention-dialog .retention-warning { color: #b76319; }
.retention-dialog .retention-last { color: #8190aa; }
.retention-unit { margin-left: 9px; color: #66748f; }
.schedule-log-table :deep(th.el-table-fixed-column--right), .schedule-log-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
.pager { display: flex; justify-content: flex-end; }
.log-detail { min-height: 280px; display: flex; flex-direction: column; gap: 16px; }
.detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 20px; }
.detail-grid span { display: block; margin-bottom: 4px; color: #7282a0; font-size: 13px; }
.detail-grid strong { color: #14213d; font-weight: 600; }
.detail-actions { display: flex; gap: 16px; flex-wrap: wrap; }
.detail-section h4 { margin: 0 0 8px; color: #14213d; }
.detail-section pre { margin: 0; padding: 14px 16px; border-radius: 10px; background: #111827; color: #e5e7eb; white-space: pre-wrap; word-break: break-word; font-family: 'JetBrains Mono', 'Consolas', monospace; font-size: 13px; line-height: 1.6; }
@media (max-width: 600px) { .detail-grid { grid-template-columns: 1fr; } }
</style>
