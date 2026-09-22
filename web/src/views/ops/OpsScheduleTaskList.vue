<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryAssetHostGroupList, queryAssetHostList } from '../../api/asset'
import {
  addOpsScheduleTask,
  batchDeleteOpsScheduleTask,
  deleteOpsScheduleTask,
  opsScheduleTaskInfo,
  previewOpsScheduleTaskNotification,
  queryNotifyRuleOptions,
  queryOpsScheduleTaskList,
  queryOpsScheduleTemplateList,
  opsScheduleTemplateInfo,
  queryOpsScriptOptions,
  runOpsScheduleTask,
  updateOpsScheduleTask,
  updateOpsScheduleTaskStatus
} from '../../api/ops'
import OpsTargetSelector from './components/OpsTargetSelector.vue'
import OpsCronEditor from './components/OpsCronEditor.vue'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const rows = ref([])
const total = ref(0)
const selectedRows = ref([])
const scriptOptions = ref([])
const hostOptions = ref([])
const groupOptions = ref([])
const templateOptions = ref([])
const notifyRuleOptions = ref([])
const previewVisible = ref(false)
const previewLoading = ref(false)
const previewStatus = ref('failed')
const previewData = ref(null)

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  taskType: '',
  status: ''
})

const form = reactive({
  id: undefined,
  name: '',
  taskType: 'script',
  templateId: undefined,
  scriptId: undefined,
  variables: {},
  hostIds: [],
  groupId: undefined,
  concurrency: 5,
  httpMethod: 'GET',
  url: '',
  headersJson: '{\n  "User-Agent": "OpsAdmin-Scheduler"\n}',
  body: '',
  expectedStatus: 200,
  timeoutSeconds: 10,
  retryEnabled: false,
  maxRetries: 2,
  retryIntervalSeconds: 5,
  retryBackoff: 'exponential',
  allowUnsafeRetry: false,
  cronExpr: '0 */5 * * * *',
  description: '',
  status: 1,
  notifyEnabled: false,
  notifyRuleId: undefined,
  notifyOnFailureOnly: false
})

const selectedScriptTimeout = computed(() => {
  const current = scriptOptions.value.find((item) => Number(item.id) === Number(form.scriptId))
  return current?.timeoutSeconds || 300
})

const selectedScriptVariables = computed(() => {
  const current = scriptOptions.value.find((item) => Number(item.id) === Number(form.scriptId))
  return current?.variables || []
})

const unsafeRetryMethod = computed(() => ['POST', 'PATCH', 'DELETE'].includes(form.httpMethod))

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function syncScriptVariables(values = {}) {
  const next = {}
  selectedScriptVariables.value.forEach((variable) => {
    next[variable.name] = values[variable.name] ?? (variable.secret ? '' : (variable.defaultValue || ''))
  })
  form.variables = next
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    taskType: 'script',
    templateId: undefined,
    scriptId: undefined,
    variables: {},
    hostIds: [],
    groupId: undefined,
    concurrency: 5,
    httpMethod: 'GET',
    url: '',
    headersJson: '{\n  "User-Agent": "OpsAdmin-Scheduler"\n}',
    body: '',
    expectedStatus: 200,
    timeoutSeconds: 10,
    retryEnabled: false,
    maxRetries: 2,
    retryIntervalSeconds: 5,
    retryBackoff: 'exponential',
    allowUnsafeRetry: false,
    cronExpr: '0 */5 * * * *',
    description: '',
    status: 1,
    notifyEnabled: false,
    notifyRuleId: undefined,
    notifyOnFailureOnly: false
  })
}

watch(
  () => form.taskType,
  (value) => {
    if (value === 'script') {
      form.httpMethod = 'GET'
      form.url = ''
      form.body = ''
      form.expectedStatus = 200
      form.timeoutSeconds = 10
    } else {
      form.scriptId = undefined
      form.hostIds = []
      form.groupId = undefined
      form.concurrency = 1
    }
  }
)

watch(() => form.scriptId, () => {
  if (form.taskType === 'script') syncScriptVariables(form.variables)
})

watch(() => form.httpMethod, () => {
  if (!unsafeRetryMethod.value) form.allowUnsafeRetry = false
})

watch(() => form.notifyOnFailureOnly, (failureOnly) => {
  if (failureOnly) previewStatus.value = 'failed'
})

async function loadBaseOptions() {
  const [scripts, hosts, groups, templates, notifyRules] = await Promise.all([
    queryOpsScriptOptions(),
    queryAssetHostList({ pageNum: 1, pageSize: 1000 }),
    queryAssetHostGroupList(),
    queryOpsScheduleTemplateList({ pageNum: 1, pageSize: 1000, status: '1' }),
    queryNotifyRuleOptions({ scope: 'schedule' })
  ])
  scriptOptions.value = scripts || []
  hostOptions.value = hosts.list || []
  groupOptions.value = groups.tree || []
  templateOptions.value = templates.list || []
  notifyRuleOptions.value = notifyRules || []
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsScheduleTaskList(query)
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

function openCreate() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

function openCreateFromTemplate() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row) {
  isEdit.value = true
  const data = await opsScheduleTaskInfo(row.id)
  Object.assign(form, {
    id: data.id,
    name: data.name || '',
    taskType: data.taskType || 'script',
    templateId: data.templateId || undefined,
    scriptId: data.scriptId || undefined,
    variables: data.variables || {},
    hostIds: data.hostIds || [],
    groupId: data.groupIds?.[0] || undefined,
    concurrency: data.concurrency || 5,
    httpMethod: data.httpMethod || 'GET',
    url: data.url || '',
    headersJson: data.headersJson || '{}',
    body: data.body || '',
    expectedStatus: data.expectedStatus || 200,
    timeoutSeconds: data.timeoutSeconds || 10,
    retryEnabled: !!data.retryEnabled,
    maxRetries: data.maxRetries || 2,
    retryIntervalSeconds: data.retryIntervalSeconds || 5,
    retryBackoff: data.retryBackoff || 'exponential',
    allowUnsafeRetry: !!data.allowUnsafeRetry,
    cronExpr: data.cronExpr || '0 */5 * * * *',
    description: data.description || '',
    status: data.status || 1,
    notifyEnabled: !!data.notifyEnabled,
    notifyRuleId: data.notifyRuleId || undefined,
    notifyOnFailureOnly: !!data.notifyOnFailureOnly
  })
  syncScriptVariables(data.variables || {})
  dialogVisible.value = true
}

async function handleCopy(row) {
  const data = await opsScheduleTaskInfo(row.id)
  isEdit.value = false
  Object.assign(form, {
    id: undefined,
    name: `${data.name || row.name}-copy`,
    taskType: data.taskType || 'script',
    templateId: data.templateId || undefined,
    scriptId: data.scriptId || undefined,
    variables: data.variables || {},
    hostIds: data.hostIds || [],
    groupId: data.groupIds?.[0] || undefined,
    concurrency: data.concurrency || 5,
    httpMethod: data.httpMethod || 'GET',
    url: data.url || '',
    headersJson: data.headersJson || '{}',
    body: data.body || '',
    expectedStatus: data.expectedStatus || 200,
    timeoutSeconds: data.timeoutSeconds || 10,
    retryEnabled: !!data.retryEnabled,
    maxRetries: data.maxRetries || 2,
    retryIntervalSeconds: data.retryIntervalSeconds || 5,
    retryBackoff: data.retryBackoff || 'exponential',
    allowUnsafeRetry: !!data.allowUnsafeRetry,
    cronExpr: data.cronExpr || '0 */5 * * * *',
    description: data.description || '',
    status: data.status || 1,
    notifyEnabled: !!data.notifyEnabled,
    notifyRuleId: data.notifyRuleId || undefined,
    notifyOnFailureOnly: !!data.notifyOnFailureOnly
  })
  syncScriptVariables(data.variables || {})
  dialogVisible.value = true
}

async function applyTemplate(templateId) {
  const selected = templateOptions.value.find((item) => Number(item.id) === Number(templateId))
  if (!selected) return
  const template = await opsScheduleTemplateInfo(templateId)
  form.taskType = template.taskType || 'script'
  form.scriptId = template.scriptId || undefined
  syncScriptVariables(template.variables || {})
  form.httpMethod = template.httpMethod || 'GET'
  form.url = template.url || ''
  form.headersJson = template.headersJson || '{}'
  form.body = template.body || ''
  form.expectedStatus = template.expectedStatus || 200
  form.timeoutSeconds = template.timeoutSeconds || 10
  if (template.cronExpr) {
    form.cronExpr = template.cronExpr
  }
  if (template.description && !form.description) {
    form.description = template.description
  }
}

function buildPayload() {
  return {
    id: form.id,
    name: form.name,
    taskType: form.taskType,
    templateId: form.templateId,
    scriptId: form.scriptId,
    variables: form.variables,
    hostIds: form.hostIds,
    groupIds: form.groupId ? [form.groupId] : [],
    concurrency: form.concurrency,
    httpMethod: form.httpMethod,
    url: form.url,
    headersJson: form.headersJson,
    body: form.body,
    expectedStatus: form.expectedStatus,
    timeoutSeconds: form.timeoutSeconds,
    retryEnabled: form.taskType === 'http' && form.retryEnabled,
    maxRetries: form.maxRetries,
    retryIntervalSeconds: form.retryIntervalSeconds,
    retryBackoff: form.retryBackoff,
    allowUnsafeRetry: form.taskType === 'http' && form.retryEnabled && form.allowUnsafeRetry,
    cronExpr: form.cronExpr,
    description: form.description,
    status: form.status,
    notifyEnabled: form.notifyEnabled,
    notifyRuleId: form.notifyEnabled ? form.notifyRuleId : undefined,
    notifyOnFailureOnly: form.notifyEnabled && form.notifyOnFailureOnly
  }
}

async function loadNotifyPreview() {
  if (form.notifyOnFailureOnly) previewStatus.value = 'failed'
  previewLoading.value = true
  previewData.value = null
  try {
    previewData.value = await previewOpsScheduleTaskNotification({
      task: buildPayload(),
      previewStatus: previewStatus.value
    })
  } finally {
    previewLoading.value = false
  }
}

async function openNotifyPreview() {
  if (!form.notifyEnabled) {
    ElMessage.warning('请先开启消息通知')
    return
  }
  if (!form.notifyRuleId) {
    ElMessage.warning('请选择通知规则')
    return
  }
  previewStatus.value = form.notifyOnFailureOnly ? 'failed' : previewStatus.value
  previewVisible.value = true
  await loadNotifyPreview()
}

async function submit() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入任务名称')
    return
  }
  if (form.notifyEnabled && !form.notifyRuleId) {
    ElMessage.warning('请选择通知规则')
    return
  }
  if (form.taskType === 'http' && form.retryEnabled && unsafeRetryMethod.value && !form.allowUnsafeRetry) {
    ElMessage.warning('请确认允许该请求方法重复提交后再保存')
    return
  }
  saving.value = true
  try {
    if (isEdit.value) {
      await updateOpsScheduleTask(buildPayload())
      ElMessage.success('任务已更新')
    } else {
      await addOpsScheduleTask(buildPayload())
      ElMessage.success('任务已创建')
    }
    dialogVisible.value = false
    await loadData()
    await loadBaseOptions()
  } finally {
    saving.value = false
  }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除任务“${row.name}”吗？`, '提示', { type: 'warning' })
  await deleteOpsScheduleTask(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

async function handleRun(row) {
  await ElMessageBox.confirm(`确认立即执行任务“${row.name}”吗？`, '提示', { type: 'warning' })
  await runOpsScheduleTask(row.id)
  ElMessage.success('任务已触发，请前往任务日志查看执行结果')
  await loadData()
}

async function batchDelete() {
  if (!selectedRows.value.length) {
    ElMessage.warning('请先选择任务')
    return
  }
  await ElMessageBox.confirm(`确认删除选中的 ${selectedRows.value.length} 个任务吗？`, '提示', { type: 'warning' })
  await batchDeleteOpsScheduleTask(selectedRows.value.map((item) => item.id))
  ElMessage.success('批量删除成功')
  await loadData()
}

async function batchUpdateStatus(status) {
  if (!selectedRows.value.length) {
    ElMessage.warning('请先选择任务')
    return
  }
  await updateOpsScheduleTaskStatus({
    ids: selectedRows.value.map((item) => item.id),
    status
  })
  ElMessage.success(status === 1 ? '批量启用成功' : '批量禁用成功')
  await loadData()
}

async function toggleRowStatus(row) {
  await updateOpsScheduleTaskStatus({
    ids: [row.id],
    status: row.status === 1 ? 2 : 1
  })
  ElMessage.success(row.status === 1 ? '任务已禁用' : '任务已启用')
  await loadData()
}

function handleSelectionChange(value) {
  selectedRows.value = value
}

function statusLabel(value) {
  return value === 1 ? '启用' : '禁用'
}

function taskTypeLabel(value) {
  return value === 'http' ? 'HTTP 探针' : '脚本任务'
}

onMounted(async () => {
  await loadBaseOptions()
  await loadData()
})
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">任务列表</h2>
        <p class="page-desc">统一维护脚本任务和 HTTP 探针任务，支持批量启用、禁用、删除和立即执行。</p>
      </div>
      <div class="header-actions">
        <el-button @click="openCreateFromTemplate">基于模板新建</el-button>
        <el-button type="primary" @click="openCreate">新建任务</el-button>
      </div>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索任务名称 / 描述 / 地址" style="width: 280px" @keyup.enter="loadData" />
        <el-select v-model="query.taskType" clearable placeholder="任务类型" style="width: 140px">
          <el-option label="脚本任务" value="script" />
          <el-option label="HTTP 探针" value="http" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="状态" style="width: 120px">
          <el-option label="启用" value="1" />
          <el-option label="禁用" value="2" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
      <div class="toolbar-right">
        <el-button @click="batchUpdateStatus(1)">批量启用</el-button>
        <el-button @click="batchUpdateStatus(2)">批量禁用</el-button>
        <el-button type="danger" plain @click="batchDelete">批量删除</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="rows" border class="schedule-task-table" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="38" />
      <el-table-column prop="name" label="任务名称" min-width="150" />
      <el-table-column label="任务类型" width="120">
        <template #default="{ row }">{{ taskTypeLabel(row.taskType) }}</template>
      </el-table-column>
      <el-table-column prop="cronExpr" label="Cron 表达式" min-width="150" />
      <el-table-column prop="scriptName" label="脚本 / 地址" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ row.taskType === 'http' ? row.url : row.scriptName }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'info'" effect="light">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="lastStatus" label="最近结果" width="100" />
      <el-table-column prop="lastSummary" label="最近摘要" min-width="200" show-overflow-tooltip />
      <el-table-column label="最近执行" width="170"><template #default="{ row }">{{ formatDateTime(row.lastRunAt) }}</template></el-table-column>
      <el-table-column label="下次执行" width="170"><template #default="{ row }">{{ formatDateTime(row.nextRunAt) }}</template></el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button link type="success" @click="handleRun(row)">立即执行</el-button>
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link @click="handleCopy(row)">复制</el-button>
          <el-button link :class="row.status === 1 ? 'schedule-action-disable' : 'schedule-action-enable'" @click="toggleRowStatus(row)">
            {{ row.status === 1 ? '禁用' : '启用' }}
          </el-button>
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

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑任务' : '新建任务'" width="min(1080px, 92vw)">
      <el-form label-width="110px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="任务名称" required>
              <el-input v-model="form.name" placeholder="例如：生产环境 Nginx 巡检" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="任务模板">
              <el-select v-model="form.templateId" clearable filterable placeholder="可选，选择后自动带入模板内容" style="width: 100%" @change="applyTemplate">
                <el-option v-for="item in templateOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="任务类型">
              <el-radio-group v-model="form.taskType">
                <el-radio value="script">脚本任务</el-radio>
                <el-radio value="http">HTTP 探针</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="状态">
              <el-radio-group v-model="form.status">
                <el-radio :value="1">启用</el-radio>
                <el-radio :value="2">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="Cron 表达式" required>
              <OpsCronEditor v-model="form.cronExpr" />
            </el-form-item>
          </el-col>

          <el-col :span="8">
            <el-form-item label="消息通知">
              <el-switch v-model="form.notifyEnabled" />
            </el-form-item>
          </el-col>
          <el-col v-if="form.notifyEnabled" :span="16">
            <el-form-item label="通知规则" required>
              <el-select v-model="form.notifyRuleId" filterable placeholder="选择通知规则" style="width: 100%">
                <el-option v-for="item in notifyRuleOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col v-if="form.notifyEnabled" :span="24">
            <el-form-item label="通知策略">
              <el-switch v-model="form.notifyOnFailureOnly" active-text="仅失败时通知" inactive-text="每次执行后通知" />
              <span class="form-tip">开启后，只有执行失败或 HTTP 状态码不符合预期时才发送通知。</span>
            </el-form-item>
          </el-col>

          <template v-if="form.taskType === 'script'">
            <el-col :span="12">
              <el-form-item label="脚本" required>
                <el-select v-model="form.scriptId" filterable placeholder="选择脚本库中的脚本" style="width: 100%">
                  <el-option v-for="item in scriptOptions" :key="item.id" :label="item.name" :value="item.id" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="并发数">
                <el-input-number v-model="form.concurrency" :min="1" :max="10" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="超时秒数">
                <el-input :model-value="selectedScriptTimeout" disabled>
                  <template #append>s</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="24">
              <div class="variable-panel">
                <div class="variable-panel__header">
                  <div>
                    <div class="variable-panel__title">运行变量</div>
                    <div class="variable-panel__hint">变量将作为 <code>VARIABLE_变量名</code> 注入脚本；密钥不会回显。</div>
                  </div>
                </div>
                <div v-if="!selectedScriptVariables.length" class="variable-panel__empty">此脚本未声明变量，无需额外配置。</div>
                <div v-else class="variable-grid">
                  <div v-for="variable in selectedScriptVariables" :key="variable.name" class="variable-field">
                    <div class="variable-field__label"><code>VARIABLE_{{ variable.name }}</code><el-tag v-if="variable.required" size="small" type="danger" effect="plain">必填</el-tag></div>
                    <el-input v-model="form.variables[variable.name]" :type="variable.secret ? 'password' : 'text'" :show-password="variable.secret" :placeholder="variable.secret ? '已配置时留空可保留原值' : (variable.defaultValue || '请输入变量值')" />
                    <div v-if="variable.description" class="variable-field__desc">{{ variable.description }}</div>
                  </div>
                </div>
              </div>
            </el-col>
            <el-col :span="24">
              <OpsTargetSelector
                :host-options="hostOptions"
                :group-options="groupOptions"
                :host-ids="form.hostIds"
                :group-id="form.groupId"
                @update:host-ids="form.hostIds = $event"
                @update:group-id="form.groupId = $event"
              />
            </el-col>
          </template>

          <template v-else>
            <el-col :span="6">
              <el-form-item label="请求方法">
                <el-select v-model="form.httpMethod" style="width: 100%">
                  <el-option label="GET" value="GET" />
                  <el-option label="POST" value="POST" />
                  <el-option label="PUT" value="PUT" />
                  <el-option label="PATCH" value="PATCH" />
                  <el-option label="DELETE" value="DELETE" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="探针地址" required>
                <el-input v-model="form.url" placeholder="例如：https://example.com/healthz" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="超时秒数">
                <el-input-number v-model="form.timeoutSeconds" :min="10" :max="3600" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="期望状态码">
                <el-input-number v-model="form.expectedStatus" :min="100" :max="599" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="18">
              <el-form-item label="失败重试">
                <el-switch v-model="form.retryEnabled" active-text="启用" inactive-text="关闭" />
                <span class="form-tip">请求异常或状态码不等于期望值时重试，包含 400、401、403、404 等所有 4xx。</span>
              </el-form-item>
            </el-col>
            <template v-if="form.retryEnabled">
              <el-col :span="6">
                <el-form-item label="重试次数">
                  <el-input-number v-model="form.maxRetries" :min="1" :max="5" style="width: 100%" />
                </el-form-item>
              </el-col>
              <el-col :span="6">
                <el-form-item label="重试间隔">
                  <el-input-number v-model="form.retryIntervalSeconds" :min="1" :max="300" style="width: 100%">
                    <template #suffix>秒</template>
                  </el-input-number>
                </el-form-item>
              </el-col>
              <el-col :span="6">
                <el-form-item label="退避策略">
                  <el-select v-model="form.retryBackoff" style="width: 100%">
                    <el-option label="指数退避" value="exponential" />
                    <el-option label="固定间隔" value="fixed" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :span="6" class="retry-summary">
                最多请求 {{ Number(form.maxRetries || 0) + 1 }} 次
              </el-col>
              <el-col v-if="unsafeRetryMethod" :span="24">
                <el-alert type="warning" :closable="false" show-icon>
                  <template #title>该方法可能产生重复写入，请确认接口具备幂等性。</template>
                  <el-checkbox v-model="form.allowUnsafeRetry">允许重复提交 {{ form.httpMethod }} 请求</el-checkbox>
                </el-alert>
              </el-col>
            </template>
            <el-col :span="18">
              <el-form-item label="请求头 JSON">
                <el-input v-model="form.headersJson" type="textarea" :rows="4" />
              </el-form-item>
            </el-col>
            <el-col :span="24">
              <el-form-item label="请求体">
                <el-input v-model="form.body" type="textarea" :rows="5" />
              </el-form-item>
            </el-col>
          </template>

          <el-col :span="24">
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="3" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button :disabled="!form.notifyEnabled || !form.notifyRuleId" @click="openNotifyPreview">发送预览</el-button>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="previewVisible" title="通知发送预览" width="min(760px, 90vw)" append-to-body>
      <div class="preview-toolbar">
        <span>预览场景</span>
        <el-radio-group v-model="previewStatus" :disabled="form.notifyOnFailureOnly" @change="loadNotifyPreview">
          <el-radio-button value="failed">失败通知</el-radio-button>
          <el-radio-button v-if="!form.notifyOnFailureOnly" value="success">成功通知</el-radio-button>
        </el-radio-group>
      </div>
      <el-alert
        v-if="form.notifyOnFailureOnly"
        title="当前为“仅失败时通知”，发送预览只展示失败通知。"
        type="warning"
        :closable="false"
        show-icon
      />
      <div v-loading="previewLoading" class="notify-preview">
        <template v-if="previewData">
          <div class="preview-meta">
            <div><span>通知规则</span><strong>{{ previewData.ruleName }}</strong></div>
            <div><span>消息模板</span><strong>{{ previewData.templateName }}</strong></div>
            <div><span>预览状态</span><el-tag :type="previewData.previewStatus === 'failed' ? 'danger' : 'success'">{{ previewData.statusLabel }}</el-tag></div>
            <div><span>目标媒介</span><div class="preview-channels"><el-tag v-for="item in previewData.channels" :key="item.id" effect="plain">{{ item.name }} · {{ item.channelType }}</el-tag></div></div>
          </div>
          <div class="preview-message">
            <h4>{{ previewData.title }}</h4>
            <pre>{{ previewData.content }}</pre>
          </div>
          <p class="preview-note">预览使用示例执行结果渲染，不会实际发送，也不会产生发送记录。</p>
        </template>
      </div>
      <template #footer>
        <el-button :loading="previewLoading" @click="loadNotifyPreview">刷新预览</el-button>
        <el-button type="primary" @click="previewVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.header-actions { display: flex; gap: 12px; }
.page-title { margin: 0 0 8px; font-size: 22px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar { display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap; }
.toolbar-left, .toolbar-right { display: flex; gap: 12px; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; }
.form-tip { margin-left: 12px; color: #8694ad; font-size: 13px; }
.retry-summary { display: flex; align-items: center; min-height: 40px; color: #66748f; font-size: 13px; }
.preview-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 14px; color: #52617a; }
.notify-preview { min-height: 220px; margin-top: 14px; }
.preview-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 20px; margin-bottom: 18px; }
.preview-meta span { display: block; margin-bottom: 5px; color: #7b89a2; font-size: 12px; }
.preview-meta strong { color: #172744; }
.preview-channels { display: flex; gap: 6px; flex-wrap: wrap; }
.preview-message { overflow: hidden; border: 1px solid #dce5f2; border-radius: 10px; }
.preview-message h4 { margin: 0; padding: 13px 16px; background: #f5f8fd; color: #172744; }
.preview-message pre { margin: 0; padding: 16px; min-height: 120px; white-space: pre-wrap; word-break: break-word; color: #34435d; font: 13px/1.7 'JetBrains Mono', 'Consolas', monospace; }
.preview-note { margin: 10px 0 0; color: #8694ad; font-size: 12px; }
.variable-panel { padding: 16px; border: 1px solid #d9e6ff; border-radius: 10px; background: linear-gradient(135deg, #f8fbff, #fff); }
.variable-panel__header { display: flex; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.variable-panel__title { color: #172744; font-size: 15px; font-weight: 700; }
.variable-panel__hint, .variable-field__desc { margin-top: 4px; color: #7282a0; font-size: 12px; }
.variable-panel__hint code, .variable-field__label code { color: #3869d9; }
.variable-panel__empty { padding: 12px; color: #8190aa; border: 1px dashed #cbdcff; border-radius: 7px; }
.variable-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.variable-field { min-width: 0; }
.variable-field__label { display: flex; align-items: center; gap: 8px; margin-bottom: 7px; font-size: 13px; font-weight: 600; }
.schedule-action-disable { color: #c87506 !important; font-weight: 600; }
.schedule-action-disable:hover { color: #9a5a00 !important; }
.schedule-action-enable { color: #49a828 !important; font-weight: 600; }
.schedule-task-table :deep(th.el-table-fixed-column--right),
.schedule-task-table :deep(td.el-table-fixed-column--right) { border-left: 1px solid #dfe6f1 !important; }
@media (max-width: 720px) { .variable-grid { grid-template-columns: 1fr; } }
</style>
