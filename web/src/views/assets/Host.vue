<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import { useEnvironmentOptions } from '../../composables/useEnvironmentOptions'
import {
  addAssetHost,
  assetHostInfo,
  batchDeleteAssetHosts,
  batchReplaceAssetHostCredential,
  batchSyncAssetHosts,
  deleteAssetHost,
  downloadAssetHostTemplate,
  importAssetHosts,
  queryAssetCloudAccountOptions,
  queryAssetCredentialOptions,
  queryAssetGatewayOptions,
  queryAssetHostGroupList,
  queryAssetHostList,
  removeAssetHostsFromGroup,
  syncAssetHost,
  syncAssetHostsFromCloud,
  updateAssetHost
} from '../../api/asset'

const route = useRoute()
const router = useRouter()
const { environmentOptions, environmentLoading, environmentName } = useEnvironmentOptions()
const loading = ref(false)
const syncingId = ref()
const dialogVisible = ref(false)
const importDialogVisible = ref(false)
const cloudSyncDialogVisible = ref(false)
const batchCredentialDialogVisible = ref(false)
const importSubmitting = ref(false)
const cloudSyncSubmitting = ref(false)
const batchCredentialSubmitting = ref(false)
const batchSyncSubmitting = ref(false)
const isEdit = ref(false)
const isCopy = ref(false)
const tableData = ref([])
const selectedRows = ref([])
const groupOptions = ref([])
const credentialOptions = ref([])
const gatewayOptions = ref([])
const cloudAccountOptions = ref([])
const total = ref(0)
const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', ipKeyword: '', status: '', groupId: undefined, environment: '' })
const form = reactive({
  id: undefined,
  hostName: '',
  groupId: undefined,
  groupIds: [],
  sshUser: '',
  sshIp: '',
  sshPort: 22,
  credentialId: undefined,
  connectionMode: 'direct',
  gatewayId: undefined,
  environment: '',
  status: 1,
  description: ''
})
const importForm = reactive({
  groupId: undefined,
  file: null
})
const cloudSyncForm = reactive({
  groupId: undefined,
  provider: 'tencent',
  useExistingAccount: true,
  cloudAccountId: undefined,
	credentialId: undefined,
	connectionMode: 'direct',
	gatewayId: undefined,
	environment: '',
  accessKey: '',
  secretKey: '',
  saveAccount: false,
  accountName: ''
})
const batchCredentialForm = reactive({
  credentialId: undefined
})

const filteredCloudAccounts = computed(() =>
  cloudAccountOptions.value.filter((item) => (item.provider || '').toLowerCase() === cloudSyncForm.provider)
)
const isGroupView = computed(() => Number(query.groupId || 0) > 0)

function resetForm() {
  Object.assign(form, {
    id: undefined,
    hostName: '',
    groupId: undefined,
    groupIds: [],
    sshUser: '',
    sshIp: '',
    sshPort: 22,
    credentialId: undefined,
    connectionMode: 'direct',
    gatewayId: undefined,
    environment: '',
    status: 1,
    description: ''
  })
}

function resetImportForm() {
  Object.assign(importForm, {
    groupId: undefined,
    file: null
  })
}

function resetCloudSyncForm() {
  Object.assign(cloudSyncForm, {
    groupId: undefined,
    provider: 'tencent',
    useExistingAccount: true,
    cloudAccountId: undefined,
	credentialId: undefined,
	connectionMode: 'direct',
	gatewayId: undefined,
	environment: '',
    accessKey: '',
    secretKey: '',
    saveAccount: false,
    accountName: ''
  })
}

function resetBatchCredentialForm() {
  batchCredentialForm.credentialId = undefined
}

async function loadOptions() {
  const [groups, credentials, cloudAccounts, gateways] = await Promise.all([
    queryAssetHostGroupList(),
    queryAssetCredentialOptions(),
    queryAssetCloudAccountOptions(),
    queryAssetGatewayOptions()
  ])
  groupOptions.value = groups.list || []
  credentialOptions.value = credentials || []
  cloudAccountOptions.value = cloudAccounts || []
  gatewayOptions.value = gateways || []
}

async function loadData() {
  loading.value = true
  try {
    const keyword = [query.keyword, query.ipKeyword].filter(Boolean).join(' ')
    const data = await queryAssetHostList({
      pageNum: query.pageNum,
      pageSize: query.pageSize,
      keyword,
      status: query.status,
      groupId: query.groupId,
      environment: query.environment
    })
    tableData.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function resetQuery() {
  query.keyword = ''
  query.ipKeyword = ''
  query.status = ''
  query.groupId = undefined
  query.environment = ''
  query.pageNum = 1
  loadData()
}

function applyRouteGroupFilter() {
  const groupId = Number(route.query.groupId || 0)
  query.groupId = groupId > 0 ? groupId : undefined
  query.pageNum = 1
}

function openCreate() {
  isEdit.value = false
  isCopy.value = false
  resetForm()
  dialogVisible.value = true
}

function openImportDialog() {
  resetImportForm()
  importDialogVisible.value = true
}

function openCloudSyncDialog() {
  resetCloudSyncForm()
  cloudSyncDialogVisible.value = true
}

function openBatchCredentialDialog() {
  if (!selectedRows.value.length) {
    ElMessage.warning('请先选择主机')
    return
  }
  resetBatchCredentialForm()
  batchCredentialDialogVisible.value = true
}

async function openEdit(row) {
  isEdit.value = true
  isCopy.value = false
  const data = await assetHostInfo(row.id)
  resetForm()
  Object.assign(form, {
    id: data.id,
    hostName: data.hostName,
    groupId: data.groupId,
    groupIds: (data.hostGroups || []).map((item) => item.id).length ? (data.hostGroups || []).map((item) => item.id) : (data.groupId ? [data.groupId] : []),
    sshUser: data.sshUser,
    sshIp: data.sshIp,
    sshPort: data.sshPort || 22,
    credentialId: data.credentialId,
    connectionMode: data.connectionMode || 'direct',
    gatewayId: data.gatewayId || undefined,
    environment: data.environment || '',
    status: data.status || 1,
    description: data.description
  })
  dialogVisible.value = true
}

async function openCopy(row) {
  isEdit.value = false
  isCopy.value = true
  const data = await assetHostInfo(row.id)
  resetForm()
  Object.assign(form, {
    id: undefined,
    hostName: `${data.hostName || row.hostName || ''}-副本`,
    groupId: data.groupId,
    groupIds: (data.hostGroups || []).map((item) => item.id).length ? (data.hostGroups || []).map((item) => item.id) : (data.groupId ? [data.groupId] : []),
    sshUser: data.sshUser,
    sshIp: data.sshIp,
    sshPort: data.sshPort || 22,
    credentialId: data.credentialId,
    connectionMode: data.connectionMode || 'direct',
    gatewayId: data.gatewayId || undefined,
    environment: data.environment || '',
    status: data.status || 1,
    description: data.description
  })
  dialogVisible.value = true
}

async function submit() {
  if (!form.hostName || !form.groupIds.length || !form.environment || !form.sshUser || !form.sshIp || !form.credentialId) {
    ElMessage.warning('请填写主机名称、所属主机组、SSH 连接和认证凭据')
    return
  }
  if (form.connectionMode === 'gateway' && !form.gatewayId) {
    ElMessage.warning('请选择访问网关')
    return
  }
  form.groupId = form.groupIds[0]

  if (isEdit.value) {
    await updateAssetHost(form)
    ElMessage.success('主机已更新')
  } else {
    await addAssetHost(form)
    ElMessage.success(isCopy.value ? '主机已复制，请按需同步采集公网地址和配置信息' : '主机已创建，可以点击同步采集公网地址和配置信息')
  }
  isCopy.value = false
  dialogVisible.value = false
  await loadData()
}

async function handleSync(row) {
  syncingId.value = row.id
  try {
    await syncAssetHost(row.id)
    ElMessage.success('同步完成')
    await loadData()
  } finally {
    syncingId.value = undefined
  }
}

async function handleDelete(row) {
  if (isGroupView.value) {
    await handleRemoveFromGroup(row)
    return
  }
  await ElMessageBox.confirm(`确认删除主机 ${row.hostName} 吗？`, '提示', { type: 'warning' })
  await deleteAssetHost(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

async function handleRemoveFromGroup(row) {
  await ElMessageBox.confirm(`确认将主机 ${row.hostName} 移出当前主机组吗？主机资产不会被删除。`, '提示', { type: 'warning' })
  await removeAssetHostsFromGroup({ groupId: query.groupId, hostId: row.id })
  ElMessage.success('已移出当前主机组')
  await loadData()
}

function handleSelectionChange(rows) {
  selectedRows.value = rows
}

function selectedIds() {
  return selectedRows.value.map((item) => item.id)
}

async function handleBatchSync() {
	if (batchSyncSubmitting.value) return
  const ids = selectedIds()
  if (!ids.length) {
    ElMessage.warning('请先选择主机')
    return
  }
	batchSyncSubmitting.value = true
	try {
		const data = await batchSyncAssetHosts(ids)
		ElMessage.success(`批量同步完成：成功 ${data.success} 台，失败 ${data.fail} 台`)
		await loadData()
	} finally {
		batchSyncSubmitting.value = false
	}
}

async function handleBatchDelete() {
  const ids = selectedIds()
  if (!ids.length) {
    ElMessage.warning('请先选择主机')
    return
  }
  if (isGroupView.value) {
    await ElMessageBox.confirm(`确认将已选中的 ${ids.length} 台主机移出当前主机组吗？主机资产不会被删除。`, '提示', { type: 'warning' })
    await removeAssetHostsFromGroup({ groupId: query.groupId, hostIds: ids })
    ElMessage.success('已批量移出当前主机组')
    selectedRows.value = []
    await loadData()
    return
  }
  await ElMessageBox.confirm(`确认批量删除已选中的 ${ids.length} 台主机吗？`, '提示', { type: 'warning' })
  await batchDeleteAssetHosts(ids)
  ElMessage.success('批量删除成功')
  selectedRows.value = []
  await loadData()
}

async function submitBatchCredential() {
  const ids = selectedIds()
  if (!ids.length) {
    ElMessage.warning('请先选择主机')
    return
  }
  if (!batchCredentialForm.credentialId) {
    ElMessage.warning('请选择认证凭据')
    return
  }
  batchCredentialSubmitting.value = true
  try {
    await batchReplaceAssetHostCredential({
      ids,
      credentialId: batchCredentialForm.credentialId
    })
    ElMessage.success('批量替换认证凭据成功')
    batchCredentialDialogVisible.value = false
    await loadData()
  } finally {
    batchCredentialSubmitting.value = false
  }
}

async function handleTemplateDownload() {
  const response = await downloadAssetHostTemplate()
  const blob = new Blob([response.data], { type: response.headers['content-type'] })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'asset-host-template.xlsx'
  link.click()
  window.URL.revokeObjectURL(url)
}

function handleFileChange(uploadFile) {
  importForm.file = uploadFile.raw || null
}

function clearImportFile() {
  importForm.file = null
}

async function submitImport() {
  if (!importForm.groupId || !importForm.file) {
    ElMessage.warning('请选择分组并上传 Excel 文件')
    return
  }
  importSubmitting.value = true
  try {
    const formData = new FormData()
    formData.append('groupId', importForm.groupId)
    formData.append('file', importForm.file)
    const data = await importAssetHosts(formData)
    const failedPreview = (data.failedHosts || []).slice(0, 3).join('；')
    ElMessage.success(`导入完成：成功 ${data.success} 台，失败 ${data.fail} 台${failedPreview ? `（${failedPreview}）` : ''}`)
    importDialogVisible.value = false
    await loadData()
  } finally {
    importSubmitting.value = false
  }
}

async function submitCloudSync() {
  if (!cloudSyncForm.groupId || !cloudSyncForm.provider) {
    ElMessage.warning('请选择分组和云厂商')
    return
  }
  if (cloudSyncForm.useExistingAccount && !cloudSyncForm.cloudAccountId) {
    ElMessage.warning('请选择已有云账号')
    return
  }
	if (!cloudSyncForm.credentialId) {
		ElMessage.warning('请选择认证凭据')
		return
	}
  if (!cloudSyncForm.environment) {
		ElMessage.warning('请选择所属环境')
		return
	}
	if (cloudSyncForm.connectionMode === 'gateway' && !cloudSyncForm.gatewayId) {
		ElMessage.warning('请选择访问网关')
		return
	}
  if (!cloudSyncForm.useExistingAccount && (!cloudSyncForm.accessKey || !cloudSyncForm.secretKey)) {
    ElMessage.warning('请填写 AccessKey 和 SecretKey')
    return
  }
  cloudSyncSubmitting.value = true
  try {
    const data = await syncAssetHostsFromCloud(cloudSyncForm)
    const addedNames = (data.addedHosts || []).slice(0, 3).join('、')
    const updatedNames = (data.updatedHosts || []).slice(0, 3).join('、')
    const details = [
      addedNames ? `新增：${addedNames}` : '',
      updatedNames ? `更新：${updatedNames}` : '',
      Object.keys(data.regionCounts || {}).length ? `地域：${Object.entries(data.regionCounts).map(([region, count]) => `${region} ${count} 台`).join('，')}` : ''
    ].filter(Boolean).join('；')
    ElMessage.success(`云端发现 ${data.total || 0} 台：新增 ${data.added} 台，更新 ${data.updated} 台，跳过 ${data.skipped} 台${details ? `（${details}）` : ''}`)
    cloudSyncDialogVisible.value = false
    await loadData()
  } finally {
    cloudSyncSubmitting.value = false
  }
}

function groupName(row) {
  const groups = row.hostGroups || []
  if (groups.length) {
    return groups.map((item) => item.name).join(' / ')
  }
  return row.group?.name || '-'
}

function statusText(value, onlineText, offlineText, unknownText = '未检测') {
  if (value === 1) return onlineText
  if (value === 2) return offlineText
  return unknownText
}

function statusType(value) {
  if (value === 1) return 'success'
  if (value === 2) return 'danger'
  return 'info'
}

function configText(row) {
  const parts = [row.cpu, row.memory, row.disk].filter(Boolean)
  return parts.length ? parts.join(' / ') : '待同步'
}

function goCredential() {
  router.push('/assets/server/credentials')
}

function goTerminal() {
  router.push('/assets/terminal')
}

function goDetail(row) {
  router.push(`/assets/server/hosts/${row.id}/detail`)
}

function handleCreateCommand(command) {
  if (command === 'create') openCreate()
  if (command === 'excel') openImportDialog()
  if (command === 'cloud') openCloudSyncDialog()
}

function handleMoreCommand(command) {
  if (command === 'batch-sync') handleBatchSync()
  if (command === 'batch-delete') handleBatchDelete()
  if (command === 'batch-credential') openBatchCredentialDialog()
}

onMounted(async () => {
  applyRouteGroupFilter()
  await loadOptions()
  await loadData()
})

watch(
  () => route.query.groupId,
  async () => {
    applyRouteGroupFilter()
    await loadData()
  }
)
</script>

<template>
  <div class="asset-host-page">
    <header class="host-page-header">
      <div>
        <p class="host-page-kicker">ASSET INVENTORY</p>
        <h2>主机管理</h2>
        <p>统一查看主机连通性、认证状态与容量指标，优先处置离线和认证异常资源。</p>
      </div>
      <div class="host-page-summary">
        <span>资源工作台</span>
        <small>支持批量同步与凭据替换</small>
      </div>
    </header>
    <section class="query-panel">
      <el-form inline>
        <el-form-item label="主机名称">
          <el-input v-model="query.keyword" clearable placeholder="请输入主机名称" style="width: 160px" @keyup.enter="loadData" />
        </el-form-item>
        <el-form-item label="IP地址">
          <el-input v-model="query.ipKeyword" clearable placeholder="请输入IP地址" style="width: 160px" @keyup.enter="loadData" />
        </el-form-item>
        <el-form-item label="主机状态">
          <el-select v-model="query.status" clearable placeholder="请选择状态" style="width: 140px">
            <el-option label="在线" value="1" />
            <el-option label="离线" value="2" />
          </el-select>
        </el-form-item>
        <el-form-item label="主机组">
          <el-select v-model="query.groupId" clearable filterable placeholder="请选择主机组" style="width: 180px">
            <el-option v-for="item in groupOptions" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="环境">
          <el-select v-model="query.environment" clearable placeholder="全部环境" style="width: 150px">
            <el-option v-for="item in environmentOptions" :key="item.code" :label="item.name" :value="item.code" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="query-actions">
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button color="#f0a43a" @click="resetQuery">重置</el-button>
        <el-dropdown split-button type="success" @click="openCreate" @command="handleCreateCommand">
          新增
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="create">导入主机</el-dropdown-item>
              <el-dropdown-item command="excel">Excel导入</el-dropdown-item>
              <el-dropdown-item command="cloud">云主机同步</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button color="#6f58c9" @click="goTerminal">终端</el-button>
        <el-dropdown
          placement="bottom-end"
          popper-class="host-more-dropdown"
          :disabled="!selectedRows.length"
          @command="handleMoreCommand"
        >
          <el-button class="more-action-trigger" :disabled="!selectedRows.length">
            更多操作
            <el-icon class="el-icon--right"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
			  <el-dropdown-item command="batch-sync" :disabled="batchSyncSubmitting">
				<el-icon v-if="batchSyncSubmitting" class="is-loading"><Loading /></el-icon>
				{{ batchSyncSubmitting ? '同步中' : '批量同步' }}
			  </el-dropdown-item>
              <el-dropdown-item command="batch-delete">{{ isGroupView ? '批量移出当前组' : '批量删除' }}</el-dropdown-item>
              <el-dropdown-item command="batch-credential">批量替换认证凭据</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </section>

    <el-table v-loading="loading" :data="tableData" class="host-table" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="52" />
      <el-table-column label="主机名称" min-width="170">
        <template #default="{ row }">
          <div class="host-name">
            <span class="linux-icon">L</span>
            <el-button link type="primary" @click="goDetail(row)">{{ row.hostName }}</el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="IP地址" min-width="170">
        <template #default="{ row }">
          <div class="ip-list">
            <span v-if="row.publicIp" class="ip public">公 {{ row.publicIp }}</span>
            <span v-if="row.privateIp || row.sshIp" class="ip private">内 {{ row.privateIp || row.sshIp }}</span>
            <span v-if="!row.publicIp && !row.privateIp && !row.sshIp">-</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="CPU使用" width="100">
        <template #default="{ row }"><span :class="{ 'metric-unavailable': !row.cpuUsage }" :title="row.metricsStatus === 'not_configured' ? '未配置 Prometheus/VictoriaMetrics 数据源' : '来自本地监控数据源'">{{ row.cpuUsage || '-' }}</span></template>
      </el-table-column>
      <el-table-column label="内存使用" width="100">
        <template #default="{ row }"><span :class="{ 'metric-unavailable': !row.memoryUsage }" :title="row.metricsStatus === 'not_configured' ? '未配置 Prometheus/VictoriaMetrics 数据源' : '来自本地监控数据源'">{{ row.memoryUsage || '-' }}</span></template>
      </el-table-column>
      <el-table-column label="磁盘使用" width="100">
        <template #default="{ row }"><span :class="{ 'metric-unavailable': !row.diskUsage }" :title="row.metricsStatus === 'not_configured' ? '未配置 Prometheus/VictoriaMetrics 数据源' : '来自本地监控数据源'">{{ row.diskUsage || '-' }}</span></template>
      </el-table-column>
      <el-table-column label="配置信息" min-width="170">
        <template #default="{ row }">
          <div class="config-info">
            <span>{{ configText(row) }}</span>
            <small v-if="row.os">{{ row.os }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="存活状态" width="110">
        <template #default="{ row }">
          <el-tag :type="statusType(row.aliveStatus)" effect="light">
            {{ statusText(row.aliveStatus, '在线', '离线') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="认证状态" width="120">
        <template #default="{ row }">
          <el-tag :type="statusType(row.authStatus)" effect="light">
            {{ statusText(row.authStatus, '认证成功', '认证失败') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="访问方式" min-width="140">
        <template #default="{ row }">
          <span v-if="row.connectionMode === 'gateway'">网关：{{ row.gateway?.name || '-' }}</span>
          <span v-else>直连</span>
        </template>
      </el-table-column>
      <el-table-column label="主机类型" width="100">
        <template #default="{ row }">{{ row.provider || '自建' }}</template>
      </el-table-column>
      <el-table-column label="环境" width="110">
        <template #default="{ row }"><el-tag effect="plain">{{ environmentName(row.environment) }}</el-tag></template>
      </el-table-column>
      <el-table-column label="所属分组" min-width="120">
        <template #default="{ row }">{{ groupName(row) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" @click="openCopy(row)">复制</el-button>
          <el-button link type="success" :loading="syncingId === row.id" @click="handleSync(row)">同步</el-button>
          <el-button link type="danger" @click="handleDelete(row)">{{ isGroupView ? '移出组' : '删除' }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="query.pageNum"
        v-model:page-size="query.pageSize"
        layout="total, sizes, prev, pager, next, jumper"
        :total="total"
        @current-change="loadData"
        @size-change="loadData"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑主机' : (isCopy ? '复制主机' : '新增主机')" width="640px">
      <el-form label-width="96px">
        <el-row :gutter="18">
          <el-col :span="12">
            <el-form-item label="主机名称" required>
              <el-input v-model="form.hostName" placeholder="请输入主机名称" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="所属主机组" required>
              <el-select v-model="form.groupIds" multiple collapse-tags collapse-tags-tooltip filterable placeholder="请选择主机组" style="width: 100%">
                <el-option v-for="item in groupOptions" :key="item.id" :value="item.id" :label="item.name" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="SSH连接" required>
              <div class="ssh-line">
                <el-input v-model="form.sshUser" placeholder="用户名" />
                <span>@</span>
                <el-input v-model="form.sshIp" placeholder="主机地址" />
                <span>-p</span>
                <el-input-number v-model="form.sshPort" :min="1" :max="65535" controls-position="right" />
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="认证凭据" required>
              <div class="credential-line">
                <el-select v-model="form.credentialId" clearable filterable placeholder="请选择认证凭据">
                  <el-option v-for="item in credentialOptions" :key="item.id" :value="item.id" :label="item.name" />
                </el-select>
                <el-button color="#f59e0b" @click="goCredential">+ 创建凭据</el-button>
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="所属环境" required>
              <el-select v-model="form.environment" :loading="environmentLoading" placeholder="请选择环境" style="width: 100%">
                <el-option v-for="item in environmentOptions" :key="item.code" :label="`${item.name} / ${item.code}`" :value="item.code" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="连接方式">
              <el-radio-group v-model="form.connectionMode">
                <el-radio-button label="direct">直连</el-radio-button>
                <el-radio-button label="gateway">通过网关</el-radio-button>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col v-if="form.connectionMode === 'gateway'" :span="12">
            <el-form-item label="访问网关" required>
              <el-select v-model="form.gatewayId" filterable placeholder="请选择网关" style="width: 100%">
                <el-option v-for="item in gatewayOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="备注">
              <el-input v-model="form.description" type="textarea" :rows="3" placeholder="请输入备注信息" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialogVisible" title="Excel导入主机" width="520px">
      <el-form label-width="92px">
        <el-form-item label="模板下载">
          <el-button type="primary" @click="handleTemplateDownload">下载模板</el-button>
        </el-form-item>
        <el-form-item label="选择分组">
          <el-select v-model="importForm.groupId" filterable placeholder="请选择分组" style="width: 100%">
            <el-option v-for="item in groupOptions" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="上传Excel">
          <el-upload
            :auto-upload="false"
            :show-file-list="true"
            :limit="1"
            accept=".xlsx,.xls"
            :on-change="handleFileChange"
            :on-remove="clearImportFile"
          >
            <el-button type="primary">选择文件</el-button>
          </el-upload>
        </el-form-item>
        <div class="dialog-tip">目标分组由本窗口统一指定。模板必填列：主机名称、SSH地址、SSH用户、认证凭据、连接方式、所属环境；网关模式还需填写访问网关名称。SSH 地址建议填写内网 IP。</div>
      </el-form>
      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="importSubmitting" @click="submitImport">导入主机</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="cloudSyncDialogVisible" title="云主机同步" width="620px">
      <el-form label-width="108px">
        <el-form-item label="目标分组">
          <el-select v-model="cloudSyncForm.groupId" filterable placeholder="请选择同步分组" style="width: 100%">
            <el-option v-for="item in groupOptions" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="云厂商">
          <el-select v-model="cloudSyncForm.provider" placeholder="请选择云厂商" style="width: 100%">
            <el-option label="腾讯云" value="tencent" />
            <el-option label="阿里云" value="aliyun" />
          </el-select>
        </el-form-item>
        <el-form-item label="云账号">
          <el-select v-model="cloudSyncForm.cloudAccountId" filterable clearable placeholder="请选择云账号" style="width: 100%">
            <el-option
              v-for="item in filteredCloudAccounts"
              :key="item.id"
              :value="item.id"
              :label="`${item.name}（${(item.regions?.length ? item.regions : (item.region ? item.region.split(/[,，;；\s]+/).filter(Boolean) : [])).join('、')}）`"
            />
          </el-select>
        </el-form-item>
		<el-form-item label="认证凭据" required>
		  <div class="credential-line">
			<el-select v-model="cloudSyncForm.credentialId" clearable filterable placeholder="请选择认证凭据">
			  <el-option v-for="item in credentialOptions" :key="item.id" :value="item.id" :label="item.name" />
			</el-select>
			<el-button color="#f59e0b" @click="goCredential">+ 创建凭据</el-button>
		  </div>
		</el-form-item>
        <el-form-item label="连接方式">
		  <el-radio-group v-model="cloudSyncForm.connectionMode">
			<el-radio value="direct">直连</el-radio>
			<el-radio value="gateway">通过网关</el-radio>
		  </el-radio-group>
		</el-form-item>
		<el-form-item v-if="cloudSyncForm.connectionMode === 'gateway'" label="访问网关">
		  <el-select v-model="cloudSyncForm.gatewayId" filterable placeholder="请选择网关" style="width: 100%">
			<el-option v-for="item in gatewayOptions" :key="item.id" :value="item.id" :label="item.name" />
		  </el-select>
		</el-form-item>
		<el-form-item label="所属环境">
		  <el-select v-model="cloudSyncForm.environment" filterable placeholder="请选择所属环境" :loading="environmentLoading" style="width: 100%">
			<el-option v-for="item in environmentOptions" :key="item.code" :label="`${item.name} / ${item.code}`" :value="item.code" />
		  </el-select>
		</el-form-item>
        <div class="dialog-tip">同步将严格使用云账号管理中配置的地域；请先在云账号管理维护至少一个地域。阿里云和腾讯云均支持多地域云主机同步。</div>
      </el-form>
      <template #footer>
        <el-button @click="cloudSyncDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="cloudSyncSubmitting" @click="submitCloudSync">开始同步</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchCredentialDialogVisible" title="批量替换认证凭据" width="480px">
      <el-form label-width="96px">
        <el-form-item label="已选主机">
          <span>{{ selectedRows.length }} 台</span>
        </el-form-item>
        <el-form-item label="认证凭据">
          <el-select v-model="batchCredentialForm.credentialId" filterable placeholder="请选择认证凭据" style="width: 100%">
            <el-option v-for="item in credentialOptions" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchCredentialDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="batchCredentialSubmitting" @click="submitBatchCredential">确定替换</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.asset-host-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.query-panel {
  padding: 16px;
  border: 1px solid #e3e8f0;
  border-radius: 10px;
  background: #f9fafc;
}

.query-actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
  align-items: center;
}

.host-table {
  overflow: hidden;
  border-radius: 10px;
  border: 1px solid #e3e8f0;
  box-shadow: 0 2px 5px rgba(20, 34, 58, 0.035);
}

.host-name {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #2787ff;
  font-weight: 700;
}

.linux-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #111827;
  color: #facc15;
  font-size: 12px;
  font-weight: 800;
}

.ip-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #2787ff;
  font-size: 13px;
}

.ip {
  line-height: 1.2;
}

.config-info {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.config-info small {
  color: #8190ad;
}

.host-page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  padding: 20px 22px;
  border: 1px solid #dfe7f3;
  border-radius: 12px;
  background: linear-gradient(110deg, #ffffff 0%, #f3f7fd 100%);
}

.host-page-kicker {
  margin: 0 0 6px;
  color: #356ae6;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: .12em;
}

.host-page-header h2 {
  margin: 0;
  color: #18243a;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: -.02em;
}

.host-page-header > div > p:last-child {
  margin: 7px 0 0;
  color: #66758d;
  font-size: 13px;
}

.host-page-summary {
  display: grid;
  gap: 4px;
  min-width: 180px;
  padding: 10px 12px;
  border-left: 3px solid #356ae6;
  color: #304b78;
  background: rgba(53, 106, 230, .06);
}

.host-page-summary span { font-size: 13px; font-weight: 650; }
.host-page-summary small { color: #71809a; font-size: 12px; }

.pager {
  display: flex;
  justify-content: flex-end;
}

.ssh-line,
.credential-line {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.ssh-line .el-input:first-child {
  width: 130px;
}

.ssh-line .el-input:nth-child(3) {
  flex: 1;
}

.ssh-line .el-input-number {
  width: 96px;
}

.credential-line .el-select {
  flex: 1;
}

.dialog-tip {
  color: #7c87a6;
  font-size: 13px;
  line-height: 1.7;
}

.more-action-trigger {
  min-width: 110px;
  height: 34px;
  border: 1px solid #dce3ed;
  border-radius: 8px;
  background: #fff;
  color: #52647e;
  box-shadow: none;
}

.more-action-trigger:hover,
.more-action-trigger:focus {
  color: #4e67a2;
  border-color: #9fbdf7;
  background: #edf4ff;
}

.more-action-trigger.is-disabled {
  color: #aeb8cf;
  border-color: #dce5f6;
  background: #f7f9fd;
}

:deep(.host-more-dropdown.el-popper) {
  border: 1px solid #dbe6ff;
  border-radius: 10px;
  box-shadow: 0 16px 32px rgba(42, 68, 132, 0.14);
  overflow: hidden;
}

:deep(.host-more-dropdown .el-dropdown-menu) {
  padding: 6px 0;
}

:deep(.host-more-dropdown .el-dropdown-menu__item) {
  min-width: 152px;
  padding: 10px 14px;
  color: #53688f;
  font-size: 13px;
}

:deep(.host-more-dropdown .el-dropdown-menu__item:hover) {
  background: #eef4ff;
  color: #4669c9;
}

@media (max-width: 900px) {
  .host-page-header { align-items: flex-start; flex-direction: column; }
  .host-page-summary { width: 100%; }
}

.metric-unavailable {
  color: #a6b1c2;
}
</style>
