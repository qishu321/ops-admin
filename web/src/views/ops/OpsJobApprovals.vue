<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  approveOpsJobHistory,
  opsJobHistoryDetail,
  queryOpsJobHistoryList,
  rejectOpsJobHistory
} from '../../api/ops'

const loading = ref(false)
const rows = ref([])
const total = ref(0)
const detailVisible = ref(false)
const detailLoading = ref(false)
const approvalNote = ref('')
const detail = reactive({
  history: null,
  steps: []
})

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  status: 'waiting_approval'
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
  Object.assign(query, {
    pageNum: 1,
    pageSize: 10,
    keyword: '',
    status: 'waiting_approval'
  })
  loadData()
}

async function openDetail(row) {
  detailVisible.value = true
  detailLoading.value = true
  approvalNote.value = ''
  try {
    const data = await opsJobHistoryDetail(row.id)
    detail.history = data.history
    detail.steps = data.steps || []
  } finally {
    detailLoading.value = false
  }
}

function currentApprovalStep() {
  return (detail.steps || []).find((item) => item.status === 'waiting_approval') || null
}

async function handleApprove() {
  const step = currentApprovalStep()
  if (!step || !detail.history) return
  await approveOpsJobHistory({
    historyId: detail.history.id,
    stepId: step.stepId,
    note: approvalNote.value
  })
  ElMessage.success('已通过人工确认，作业将继续执行')
  detailVisible.value = false
  await loadData()
}

async function handleReject() {
  const step = currentApprovalStep()
  if (!step || !detail.history) return
  await rejectOpsJobHistory({
    historyId: detail.history.id,
    stepId: step.stepId,
    note: approvalNote.value
  })
  ElMessage.success('已拒绝该作业')
  detailVisible.value = false
  await loadData()
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

onMounted(loadData)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">人工确认</h2>
        <p class="page-desc">集中处理卡在人工确认步骤的作业，确认通过后继续执行，拒绝后终止作业。</p>
      </div>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索作业名称 / 摘要 / 当前步骤" style="width: 320px" @keyup.enter="loadData" />
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="rows" border class="job-approval-table">
      <el-table-column prop="jobName" label="作业名称" min-width="220" />
      <el-table-column label="状态" width="120" align="center">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="summary" label="摘要" min-width="260" show-overflow-tooltip />
      <el-table-column prop="currentStepName" label="待确认步骤" width="220" show-overflow-tooltip />
      <el-table-column prop="startedAt" label="开始时间" width="180" />
      <el-table-column label="操作" width="100" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openDetail(row)">处理</el-button>
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

    <el-drawer v-model="detailVisible" size="70%" title="人工确认处理">
      <div v-loading="detailLoading" class="approval-detail">
        <div v-if="detail.history" class="detail-summary">
          <div class="summary-card"><span>作业名称</span><strong>{{ detail.history.jobName }}</strong></div>
          <div class="summary-card"><span>当前状态</span><strong>{{ detail.history.status }}</strong></div>
          <div class="summary-card"><span>当前步骤</span><strong>{{ detail.history.currentStepName || '-' }}</strong></div>
        </div>

        <el-alert
          type="warning"
          :closable="false"
          title="请在确认内容无误后放行，放行后作业会从当前节点继续向下执行。"
        />

        <el-input
          v-model="approvalNote"
          type="textarea"
          :rows="3"
          placeholder="确认备注，可选"
        />

        <el-table :data="detail.steps" border>
          <el-table-column prop="stepName" label="步骤名称" min-width="180" />
          <el-table-column prop="stepType" label="步骤类型" width="120" />
          <el-table-column label="状态" width="140" align="center">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="summary" label="摘要" min-width="220" show-overflow-tooltip />
          <el-table-column prop="output" label="输出内容" min-width="320" show-overflow-tooltip />
        </el-table>

        <div class="detail-actions">
          <el-button type="danger" plain @click="handleReject">拒绝</el-button>
          <el-button type="primary" @click="handleApprove">确认通过</el-button>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-title { margin: 0 0 8px; font-size: 24px; font-weight: 700; color: #14213d; }
.job-approval-table :deep(th.el-table-fixed-column--right),
.job-approval-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar-left { display: flex; gap: 12px; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; }
.approval-detail { display: flex; flex-direction: column; gap: 16px; }
.detail-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.summary-card { padding: 16px; border: 1px solid #e5ebff; border-radius: 12px; background: #f9fbff; }
.summary-card span { display: block; margin-bottom: 8px; color: #7485a7; font-size: 13px; }
.summary-card strong { color: #1b2a4b; font-size: 16px; }
.detail-actions { display: flex; justify-content: flex-end; gap: 12px; }
</style>
