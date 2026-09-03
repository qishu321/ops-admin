<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteOpsJob, queryOpsJobList, runOpsJob, updateOpsJobStatus } from '../../api/ops'

const router = useRouter()
const loading = ref(false)
const rows = ref([])
const total = ref(0)

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  status: ''
})

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsJobList(query)
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

function openCreate() {
  router.push('/ops/jobs/designer')
}

function openEdit(row) {
  router.push({ path: '/ops/jobs/designer', query: { id: String(row.id) } })
}

async function handleRun(row) {
  await ElMessageBox.confirm(`确认立即执行作业“${row.name}”吗？`, '提示', { type: 'warning' })
  await runOpsJob(row.id)
  ElMessage.success('作业已触发，请前往作业历史查看执行过程')
  loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除作业“${row.name}”吗？`, '提示', { type: 'warning' })
  await deleteOpsJob(row.id)
  ElMessage.success('删除成功')
  loadData()
}

async function toggleStatus(row) {
  const enabled = Number(row.status) === 1
  await ElMessageBox.confirm(`确认${enabled ? '禁用' : '启用'}作业“${row.name}”吗？`, '提示', { type: 'warning' })
  await updateOpsJobStatus({ id: row.id, status: enabled ? 2 : 1 })
  ElMessage.success(enabled ? '作业已禁用' : '作业已启用')
  await loadData()
}

function statusLabel(status) {
  return Number(status) === 1 ? '启用' : '禁用'
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
        <h2 class="page-title">作业列表</h2>
        <p class="page-desc">查看所有作业定义，支持直接运行、继续编排与删除。</p>
      </div>
      <el-button type="primary" @click="openCreate">新建作业</el-button>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索作业名称 / 描述" style="width: 280px" @keyup.enter="loadData" />
        <el-select v-model="query.status" clearable placeholder="状态" style="width: 120px">
          <el-option label="启用" value="1" />
          <el-option label="禁用" value="2" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="rows" border class="job-list-table">
      <el-table-column prop="name" label="作业名称" min-width="220" />
      <el-table-column prop="description" label="描述" min-width="260" show-overflow-tooltip />
      <el-table-column prop="templateId" label="来源模板" width="100" align="center">
        <template #default="{ row }">{{ row.templateId || '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'info'" effect="light">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="180"><template #default="{ row }">{{ formatDateTime(row.updateTime) }}</template></el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button link type="success" @click="handleRun(row)">运行</el-button>
          <el-button link type="primary" @click="openEdit(row)">编排</el-button>
          <el-button link :class="Number(row.status) === 1 ? 'job-action-disable' : 'job-action-enable'" @click="toggleStatus(row)">{{ Number(row.status) === 1 ? '禁用' : '启用' }}</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
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
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-title { margin: 0 0 8px; font-size: 24px; font-weight: 700; color: #14213d; }
.job-list-table :deep(th.el-table-fixed-column--right),
.job-list-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar-left { display: flex; gap: 12px; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; }
.job-action-disable { color: #c87506 !important; font-weight: 600; }
.job-action-disable:hover { color: #9a5a00 !important; }
.job-action-enable { color: #49a828 !important; font-weight: 600; }
</style>
