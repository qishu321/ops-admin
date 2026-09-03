<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryOpsExecHistory, queryOpsExecHistoryDetail, retryOpsExecTask } from '../../api/ops'

const route = useRoute()
const loading = ref(false)
const detailVisible = ref(false)
const detailLoading = ref(false)
const tableData = ref([])
const total = ref(0)
const detailTask = ref(null)
const detailResults = ref([])

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
    const data = await queryOpsExecHistory(query)
    tableData.value = data.list || []
    total.value = data.total || 0
    await openDetailFromQuery()
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
    const data = await queryOpsExecHistoryDetail(row.id)
    detailTask.value = data.task || null
    detailResults.value = data.results || []
  } finally {
    detailLoading.value = false
  }
}

async function openDetailFromQuery() {
  const taskId = Number(route.query.taskId || 0)
  if (!taskId || detailVisible.value) return
  const hit = tableData.value.find((item) => Number(item.id) === taskId)
  if (hit) {
    await openDetail(hit)
  }
}

function taskTypeLabel(value) {
  const map = {
    command: '命令执行',
    script: '脚本执行',
    file: '文件分发'
  }
  return map[value] || value || '-'
}

function statusTagType(value) {
  if (value === 'success') return 'success'
  if (value === 'partial') return 'warning'
  if (value === 'running') return 'warning'
  return 'danger'
}

function formatExecutionTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

async function retryFailed(row) {
  await ElMessageBox.confirm(`仅重新执行“${row.title}”中的失败或超时主机，是否继续？`, '重试失败目标', { type: 'warning' })
  const data = await retryOpsExecTask(row.id)
  ElMessage.success('重试任务已创建')
  await loadData()
  if (data?.task) await openDetail(data.task)
}

function downloadExecutionResult() {
  if (!detailTask.value) return
  const payload = {
    task: detailTask.value,
    results: detailResults.value
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `ops-execution-${detailTask.value.id || 'result'}.json`
  link.click()
  URL.revokeObjectURL(link.href)
}

onMounted(loadData)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">执行历史</h2>
        <p class="page-desc">记录所有命令执行、脚本执行和文件分发任务的执行结果。</p>
      </div>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索任务名称 / 脚本 / 文件 / 命令" style="width: 320px" @keyup.enter="loadData" />
        <el-select v-model="query.taskType" clearable placeholder="任务类型" style="width: 140px">
          <el-option label="命令执行" value="command" />
          <el-option label="脚本执行" value="script" />
          <el-option label="文件分发" value="file" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="执行状态" style="width: 140px">
          <el-option label="成功" value="success" />
          <el-option label="部分成功" value="partial" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="title" label="任务名称" min-width="220" />
      <el-table-column label="类型" width="110">
        <template #default="{ row }">{{ taskTypeLabel(row.taskType) }}</template>
      </el-table-column>
      <el-table-column prop="scriptName" label="脚本" min-width="160" />
      <el-table-column prop="fileName" label="文件" min-width="160" />
      <el-table-column prop="hostCount" label="目标主机" width="100" />
      <el-table-column prop="successCount" label="成功" width="80" />
      <el-table-column prop="failedCount" label="失败" width="80" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" effect="light">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="summary" label="摘要" min-width="180" />
      <el-table-column prop="operator" label="发起人" width="120"><template #default="{ row }">{{ row.operator || 'system' }}</template></el-table-column>
      <el-table-column label="风险" width="90"><template #default="{ row }"><el-tag :type="row.riskLevel === 'high' ? 'danger' : 'info'" effect="plain">{{ row.riskLevel === 'high' ? '高风险' : '普通' }}</el-tag></template></el-table-column>
      <el-table-column label="执行时间" min-width="180"><template #default="{ row }">{{ formatExecutionTime(row.startedAt || row.createTime) }}</template></el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openDetail(row)">详情</el-button>
          <el-button v-if="row.status !== 'running' && row.failedCount > 0 && row.taskType !== 'file'" link type="warning" @click="retryFailed(row)">重试失败</el-button>
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

    <el-drawer v-model="detailVisible" size="72%" title="执行详情">
      <div v-loading="detailLoading" class="detail-wrap">
        <div class="detail-actions">
          <el-button :disabled="!detailTask" @click="downloadExecutionResult">下载完整结果</el-button>
        </div>
        <div v-if="detailTask" class="detail-summary">
          <div class="summary-item"><span>任务名称</span><strong>{{ detailTask.title }}</strong></div>
          <div class="summary-item"><span>任务类型</span><strong>{{ taskTypeLabel(detailTask.taskType) }}</strong></div>
          <div class="summary-item"><span>执行状态</span><strong>{{ detailTask.status }}</strong></div>
          <div class="summary-item"><span>执行摘要</span><strong>{{ detailTask.summary || '-' }}</strong></div>
          <div class="summary-item"><span>并发数</span><strong>{{ detailTask.concurrency }}</strong></div>
          <div class="summary-item"><span>执行时间</span><strong>{{ formatExecutionTime(detailTask.startedAt || detailTask.createTime) }}</strong></div>
          <div class="summary-item"><span>发起人</span><strong>{{ detailTask.operator || 'system' }}</strong></div>
          <div class="summary-item"><span>脚本版本</span><strong>{{ detailTask.scriptVersion ? `v${detailTask.scriptVersion}` : '-' }}</strong></div>
        </div>

        <el-table :data="detailResults" border>
          <el-table-column prop="hostName" label="主机" min-width="180" />
          <el-table-column prop="groupName" label="主机组" min-width="140" />
          <el-table-column prop="sshIp" label="SSH IP" min-width="140" />
          <el-table-column prop="status" label="状态" width="100" />
          <el-table-column prop="exitCode" label="退出码" width="80" />
          <el-table-column prop="durationMs" label="耗时(ms)" width="110" />
          <el-table-column prop="stdout" label="标准输出" min-width="260" show-overflow-tooltip />
          <el-table-column prop="stderr" label="错误输出" min-width="220" show-overflow-tooltip />
          <el-table-column prop="errorText" label="错误信息" min-width="220" show-overflow-tooltip />
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; }
.page-title { margin: 0 0 8px; font-size: 22px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar-left { display: flex; flex-wrap: wrap; gap: 12px; }
.pager { display: flex; justify-content: flex-end; }
.detail-wrap { display: flex; flex-direction: column; gap: 16px; }
.detail-actions { display: flex; justify-content: flex-end; }
.detail-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.summary-item { padding: 14px 16px; border: 1px solid #e8edf5; border-radius: 8px; background: #f8fbff; }
.summary-item span { display: block; margin-bottom: 6px; color: #7282a0; font-size: 13px; }
.summary-item strong { color: #14213d; }
</style>
