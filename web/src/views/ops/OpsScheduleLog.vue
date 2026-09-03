<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { opsScheduleLogInfo, queryOpsScheduleLogList } from '../../api/ops'

const router = useRouter()
const loading = ref(false)
const detailLoading = ref(false)
const detailVisible = ref(false)
const rows = ref([])
const total = ref(0)
const detail = ref(null)

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  taskType: '',
  status: ''
})

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsScheduleLogList(query)
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, {
    pageNum: 1,
    pageSize: 10,
    keyword: '',
    taskType: '',
    status: ''
  })
  loadData()
}

async function openDetail(row) {
  detailVisible.value = true
  detailLoading.value = true
  try {
    detail.value = await opsScheduleLogInfo(row.id)
  } finally {
    detailLoading.value = false
  }
}

function taskTypeLabel(value) {
  return value === 'http' ? 'HTTP 探针' : '脚本任务'
}

function triggerLabel(value) {
  return value === 'manual' ? '手动触发' : '定时调度'
}

function statusTagType(value) {
  if (value === 'success') return 'success'
  if (value === 'running') return 'warning'
  if (value === 'partial') return 'warning'
  return 'danger'
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

onMounted(loadData)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">任务日志</h2>
        <p class="page-desc">查看定时任务每次调度的执行结果、摘要和详细输出。</p>
      </div>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索任务名称 / 摘要" style="width: 280px" @keyup.enter="loadData" />
        <el-select v-model="query.taskType" clearable placeholder="任务类型" style="width: 140px">
          <el-option label="脚本任务" value="script" />
          <el-option label="HTTP 探针" value="http" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="执行状态" style="width: 120px">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="部分成功" value="partial" />
          <el-option label="执行中" value="running" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="rows" border class="schedule-log-table">
      <el-table-column prop="taskName" label="任务名称" min-width="180" />
      <el-table-column label="任务类型" width="120">
        <template #default="{ row }">{{ taskTypeLabel(row.taskType) }}</template>
      </el-table-column>
      <el-table-column label="触发方式" width="110">
        <template #default="{ row }">{{ triggerLabel(row.triggerType) }}</template>
      </el-table-column>
      <el-table-column label="执行状态" width="110" align="center">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="summary" label="执行摘要" min-width="260" show-overflow-tooltip />
      <el-table-column prop="durationMs" label="耗时(ms)" width="100" />
      <el-table-column label="开始时间" width="180"><template #default="{ row }">{{ formatDateTime(row.startedAt) }}</template></el-table-column>
      <el-table-column label="结束时间" width="180"><template #default="{ row }">{{ formatDateTime(row.finishedAt) }}</template></el-table-column>
      <el-table-column label="操作" width="100" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openDetail(row)">详情</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="query.pageNum"
        v-model:page-size="query.pageSize"
        :total="total"
        layout="total, sizes, prev, pager, next"
        @current-change="loadData"
        @size-change="loadData"
      />
    </div>

    <el-dialog v-model="detailVisible" title="任务日志详情" width="960px">
      <div v-loading="detailLoading" class="log-detail">
        <template v-if="detail">
          <div class="detail-grid">
            <div><span>任务名称</span><strong>{{ detail.taskName }}</strong></div>
            <div><span>任务类型</span><strong>{{ taskTypeLabel(detail.taskType) }}</strong></div>
            <div><span>执行状态</span><strong>{{ detail.status }}</strong></div>
            <div><span>触发方式</span><strong>{{ triggerLabel(detail.triggerType) }}</strong></div>
            <div><span>开始时间</span><strong>{{ formatDateTime(detail.startedAt) }}</strong></div>
            <div><span>结束时间</span><strong>{{ formatDateTime(detail.finishedAt) }}</strong></div>
            <div><span>耗时</span><strong>{{ detail.durationMs }} ms</strong></div>
            <div><span>关联执行任务</span><strong>{{ detail.execTaskId || '-' }}</strong></div>
            <div><span>期望状态码</span><strong>{{ detail.expectedStatus || '-' }}</strong></div>
            <div><span>实际状态码</span><strong>{{ detail.actualStatus || '-' }}</strong></div>
          </div>

          <el-alert :title="detail.summary || '无摘要'" type="info" :closable="false" />

          <div class="detail-actions">
            <el-button v-if="detail.execTaskId" type="primary" link @click="openExecHistory(detail.execTaskId)">前往快速执行详情</el-button>
            <el-button link @click="copyText(detail.detail || '', '详细输出已复制')">复制详细输出</el-button>
            <el-button link @click="copyText(detail.responseBody || '', 'HTTP 响应已复制')">复制 HTTP 响应</el-button>
          </div>

          <div class="detail-section">
            <h4>详细输出</h4>
            <pre>{{ detail.detail || '-' }}</pre>
          </div>
          <div class="detail-section">
            <h4>HTTP 响应</h4>
            <pre>{{ detail.responseBody || '-' }}</pre>
          </div>
        </template>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-title { margin: 0 0 8px; font-size: 22px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar-left { display: flex; gap: 12px; flex-wrap: wrap; }
.schedule-log-table :deep(th.el-table-fixed-column--right),
.schedule-log-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
.pager { display: flex; justify-content: flex-end; }
.log-detail { min-height: 280px; display: flex; flex-direction: column; gap: 16px; }
.detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 20px; }
.detail-grid span { display: block; margin-bottom: 4px; color: #7282a0; font-size: 13px; }
.detail-grid strong { color: #14213d; font-weight: 600; }
.detail-actions { display: flex; gap: 16px; flex-wrap: wrap; }
.detail-section h4 { margin: 0 0 8px; color: #14213d; }
.detail-section pre { margin: 0; padding: 14px 16px; border-radius: 10px; background: #111827; color: #e5e7eb; white-space: pre-wrap; word-break: break-word; font-family: 'JetBrains Mono', 'Consolas', monospace; font-size: 13px; line-height: 1.6; }
</style>
