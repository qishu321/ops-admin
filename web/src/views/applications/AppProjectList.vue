<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteOpsApplication, queryOpsApplicationList, saveOpsApplication } from '../../api/ops'
import { queryAssetCredentialOptions } from '../../api/asset'
import { useEnvironmentOptions } from '../../composables/useEnvironmentOptions'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const rows = ref([])
const total = ref(0)
const { environmentOptions, environmentLoading, environmentName } = useEnvironmentOptions()
const credentialOptions = ref([])

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  serviceType: '',
  env: ''
})

const form = reactive({
  id: undefined,
  name: '',
  code: '',
  serviceType: '后端服务',
  repoType: 'git',
  repoUrl: '',
  repoCredentialId: undefined,
  branch: 'master',
  env: 'test',
  status: 1,
  description: ''
})

function isSVNRepository() { return form.repoType === 'svn' }

function handleRepoTypeChange(repoType) {
  if (repoType === 'svn' && (!form.branch || form.branch === 'master' || form.branch === 'main')) form.branch = 'HEAD'
  if (repoType === 'git' && (!form.branch || form.branch === 'HEAD')) form.branch = 'master'
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    code: '',
    serviceType: '后端服务',
    repoType: 'git',
    repoUrl: '',
    repoCredentialId: undefined,
    branch: 'master',
    env: 'test',
    status: 1,
    description: ''
  })
}

async function loadCredentialOptions() {
  const credentials = await queryAssetCredentialOptions()
  credentialOptions.value = credentials || []
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsApplicationList(query)
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row) {
  Object.assign(form, {
    id: row.id,
    name: row.name || '',
    code: row.code || '',
    serviceType: row.serviceType || row.env || '后端服务',
    repoType: row.repoType || 'git',
    repoUrl: row.repoUrl || '',
    repoCredentialId: row.repoCredentialId || undefined,
    branch: row.branch || (row.repoType === 'svn' ? 'HEAD' : 'master'),
    env: row.env || 'test',
    status: row.status || 1,
    description: row.description || ''
  })
  dialogVisible.value = true
}

async function submit() {
  if (!form.name || !form.code || !form.repoUrl) {
    ElMessage.warning('请填写项目名称、项目编码和仓库地址')
    return
  }
  if (isSVNRepository() && form.branch && !/^(HEAD|\d+)$/i.test(form.branch.trim())) {
    ElMessage.warning('SVN 版本号仅支持 HEAD 或数字修订号')
    return
  }
  saving.value = true
  try {
    await saveOpsApplication({ ...form })
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function remove(row) {
  await ElMessageBox.confirm(`确认删除项目「${row.name}」？`, '删除项目', { type: 'warning' })
  await deleteOpsApplication(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

function statusType(status) {
  return Number(status) === 1 ? 'success' : 'info'
}

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

onMounted(async () => { await Promise.all([loadCredentialOptions(), loadData()]) })
</script>

<template>
  <div class="app-page">
    <div class="app-header">
      <div>
        <h1>应用管理</h1>
        <p>统一维护应用代码仓库，并按环境绑定主机、K8s、数据库、网关和监控资源。</p>
      </div>
      <el-button type="primary" @click="openCreate">+ 新建应用</el-button>
    </div>

    <div class="filter-panel">
      <el-form inline>
        <el-form-item label="应用名称">
          <el-input v-model="query.keyword" clearable placeholder="请输入应用名称 / 仓库地址" @keyup.enter="loadData" />
        </el-form-item>
        <el-form-item label="服务类别">
          <el-select v-model="query.serviceType" clearable placeholder="请选择服务类别">
            <el-option label="前端服务" value="前端服务" />
            <el-option label="后端服务" value="后端服务" />
            <el-option label="中间件" value="中间件" />
          </el-select>
        </el-form-item>
        <el-form-item label="环境">
          <el-select v-model="query.env" clearable placeholder="全部环境" @change="loadData">
            <el-option v-for="item in environmentOptions" :key="item.code" :label="item.name" :value="item.code" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadData">搜索</el-button>
          <el-button @click="Object.assign(query, { keyword: '', serviceType: '', env: '', pageNum: 1 }); loadData()">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <el-card shadow="never" class="table-card">
      <el-table v-loading="loading" :data="rows" row-key="id">
        <el-table-column prop="name" label="应用名称" min-width="170">
          <template #default="{ row }">
            <div class="name-cell">
              <strong>{{ row.name }}</strong>
              <span>{{ row.code }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="业务功能" min-width="170" show-overflow-tooltip />
        <el-table-column prop="serviceType" label="服务类别" width="120">
          <template #default="{ row }">{{ row.serviceType || row.env || '-' }}</template>
        </el-table-column>
        <el-table-column label="默认环境" width="120">
          <template #default="{ row }"><el-tag effect="plain">{{ environmentName(row.env) }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="repoUrl" label="仓库地址" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.repoType || 'git' }}</el-tag>
            <span class="repo-url">{{ row.repoUrl }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ Number(row.status) === 1 ? '正常' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建者" width="100">管理员</el-table-column>
        <el-table-column label="创建时间" min-width="170"><template #default="{ row }">{{ formatDateTime(row.createTime) }}</template></el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">查看</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager">
        <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" layout="total, prev, pager, next" :total="total" @current-change="loadData" />
      </div>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑应用' : '新建应用'" width="min(1280px, 94vw)" top="4vh" class="app-project-dialog">
      <el-form :model="form" label-width="96px">
        <el-row :gutter="14">
          <el-col :span="12">
            <el-form-item label="应用名称" required><el-input v-model="form.name" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="应用编码" required><el-input v-model="form.code" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="服务类别"><el-input v-model="form.serviceType" placeholder="例如：前端服务 / 后端服务" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="仓库类型">
              <el-radio-group v-model="form.repoType" @change="handleRepoTypeChange">
                <el-radio-button label="git">Git</el-radio-button>
                <el-radio-button label="svn">SVN</el-radio-button>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="仓库地址" required><el-input v-model="form.repoUrl" :placeholder="isSVNRepository() ? 'https://svn.example.com/svn/team/app' : 'https://git.example.com/team/app.git'" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="仓库凭据">
              <el-select v-model="form.repoCredentialId" clearable filterable style="width: 100%" :placeholder="isSVNRepository() ? '公开 SVN 可不选择；支持 HTTP(S) 认证' : '公开仓库可不选择'">
                <el-option v-for="item in credentialOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="isSVNRepository() ? 'SVN 版本' : '默认分支'">
              <el-input v-model="form.branch" :placeholder="isSVNRepository() ? 'HEAD（最新版本）或数字修订号' : '例如：main / master / release/1.0'" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="默认环境" required>
              <el-select v-model="form.env" :loading="environmentLoading" style="width: 100%" placeholder="请选择环境">
                <el-option v-for="item in environmentOptions" :key="item.code" :label="`${item.name} / ${item.code}`" :value="item.code" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="业务功能"><el-input v-model="form.description" type="textarea" :rows="3" placeholder="例如：提供用户登录、订单处理或游戏网关等业务能力" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="状态">
              <el-radio-group v-model="form.status">
                <el-radio :value="1">启用</el-radio>
                <el-radio :value="2">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.app-page { padding: 24px; }
.app-header, .filter-panel, .table-card { background: #fff; border: 1px solid #e5edf8; border-radius: 12px; }
.app-header { display: flex; justify-content: space-between; align-items: center; padding: 24px; margin-bottom: 16px; }
.app-header h1 { margin: 0; font-size: 28px; color: #071b3d; }
.app-header p { margin: 8px 0 0; color: #6b7c9b; }
.filter-panel { padding: 18px 24px 0; margin-bottom: 16px; }
:deep(.filter-panel .el-select) { width: 220px; }
:deep(.filter-panel .el-input) { width: 280px; }
.table-card { border-radius: 12px; }
.name-cell { display: flex; flex-direction: column; gap: 4px; }
.name-cell strong { color: #1677ff; }
.name-cell span, .repo-url { color: #697b99; }
.repo-url { margin-left: 8px; }
.pager { display: flex; justify-content: flex-end; padding-top: 16px; }
:deep(.app-project-dialog) { overflow: hidden; border: 1px solid #d8e5f6; border-radius: 16px; box-shadow: 0 20px 48px rgba(20, 55, 105, .18); }
:deep(.app-project-dialog .el-dialog__header) { position: relative; padding: 20px 24px 16px !important; border-bottom: 1px solid #e2eaf6; background: linear-gradient(118deg, #fff, #f3f8ff); }
:deep(.app-project-dialog .el-dialog__header::before) { position: absolute; top: 0; left: 0; width: 100%; height: 3px; content: ''; background: linear-gradient(90deg, #2563eb, #4b86f2 58%, #ea580c 58%, #ea580c 66%, transparent 66%); }
:deep(.app-project-dialog .el-dialog__title) { color: #183962; font-size: 18px; font-weight: 700; }
:deep(.app-project-dialog .el-dialog__body) { max-height: calc(92vh - 150px); overflow: auto; padding: 20px 24px !important; background: #fbfdff; }
:deep(.app-project-dialog .el-form > .el-row) { padding: 18px 18px 4px; border: 1px solid #dfe9f7; border-radius: 12px; background: #fff; }
:deep(.app-project-dialog .el-form-item__label) { color: #526985; font-weight: 600; }
:deep(.app-project-dialog .el-radio-button__inner) { min-width: 54px; border-radius: 7px !important; }
:deep(.app-project-dialog .el-dialog__footer) { padding: 14px 24px 18px !important; border-top: 1px solid #e2eaf6; background: #fff; }
:deep(.app-project-dialog .el-dialog__footer .el-button--primary) { min-width: 88px; }
</style>
