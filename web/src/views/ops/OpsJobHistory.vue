<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { opsJobHistoryDetail, queryOpsJobHistoryList } from '../../api/ops'

const router = useRouter()
const loading = ref(false)
const rows = ref([])
const total = ref(0)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detail = reactive({
  history: null,
  steps: []
})

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  status: ''
})

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsJobHistoryList(query)
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, { pageNum: 1, pageSize: 10, keyword: '', status: '' })
  loadData()
}

async function openDetail(row) {
  detailVisible.value = true
  detailLoading.value = true
  try {
    const data = await opsJobHistoryDetail(row.id)
    detail.history = data.history
    detail.steps = data.steps || []
  } finally {
    detailLoading.value = false
  }
}

function openExecHistory(step) {
  if (!step.execTaskId) return
  router.push({ path: '/ops/quick-exec/history', query: { taskId: String(step.execTaskId) } })
}

function statusTagType(status) {
  switch (status) {
    case 'success':
      return 'success'
    case 'running':
      return 'warning'
    case 'waiting_approval':
      return 'warning'
    case 'rejected':
      return 'danger'
    default:
      return 'info'
  }
}

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

onMounted(loadData)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">作业历史</h2>
        <p class="page-desc">查看作业每次执行的整体状态、步骤结果和执行输出。人工确认请前往独立的待办页面处理。</p>
      </div>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索作业名称 / 摘要 / 当前步骤" style="width: 320px" @keyup.enter="loadData" />
        <el-select v-model="query.status" clearable placeholder="状态" style="width: 140px">
          <el-option label="运行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="待确认" value="waiting_approval" />
          <el-option label="已拒绝" value="rejected" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="rows" border class="job-history-table">
      <el-table-column prop="jobName" label="作业名称" min-width="220" />
      <el-table-column prop="triggerType" label="触发方式" width="100" />
      <el-table-column label="状态" width="120" align="center">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="summary" label="摘要" min-width="260" show-overflow-tooltip />
      <el-table-column prop="currentStepName" label="当前步骤" width="180" show-overflow-tooltip />
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

    <el-drawer v-model="detailVisible" size="70%" title="作业执行详情">
      <div v-loading="detailLoading" class="history-detail">
        <div v-if="detail.history" class="history-summary">
          <div class="summary-item"><span>作业</span><strong>{{ detail.history.jobName }}</strong></div>
          <div class="summary-item"><span>状态</span><strong>{{ detail.history.status }}</strong></div>
          <div class="summary-item"><span>摘要</span><strong>{{ detail.history.summary || '-' }}</strong></div>
        </div>

        <el-table :data="detail.steps" border class="job-history-detail-table">
          <el-table-column prop="stepName" label="步骤名称" min-width="180" />
          <el-table-column prop="stepType" label="步骤类型" width="120" />
          <el-table-column label="状态" width="140" align="center">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="summary" label="摘要" min-width="220" show-overflow-tooltip />
          <el-table-column prop="durationMs" label="耗时(ms)" width="120" />
          <el-table-column prop="output" label="输出" min-width="280" show-overflow-tooltip />
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button v-if="row.execTaskId" link type="primary" @click="openExecHistory(row)">执行明细</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-title { margin: 0 0 8px; font-size: 24px; font-weight: 700; color: #14213d; }
.job-history-table :deep(th.el-table-fixed-column--right),
.job-history-table :deep(td.el-table-fixed-column--right),
.job-history-detail-table :deep(th.el-table-fixed-column--right),
.job-history-detail-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar-left { display: flex; gap: 12px; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; }
.history-detail { display: flex; flex-direction: column; gap: 16px; }
.history-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.summary-item { padding: 16px; border: 1px solid #e5ebff; border-radius: 12px; background: #f9fbff; }
.summary-item span { display: block; margin-bottom: 8px; color: #7485a7; font-size: 13px; }
.summary-item strong { color: #1b2a4b; font-size: 16px; }
</style>
