<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveOpsAppPipelineRun,
  copyOpsAppPipeline,
  deleteOpsAppPipeline,
  opsAppPipelineInfo,
  opsAppPipelineRunInfo,
  queryOpsAppPipelineList,
  queryOpsAppPipelineRunList,
  queryOpsAppPipelineTemplates,
  queryOpsApplicationOptions,
  queryOpsImageRegistryList,
  queryNotifyRuleOptions,
  runOpsAppPipeline,
  rollbackOpsAppPipelineRun,
  saveOpsAppPipeline,
  updateOpsAppPipelineStatus
} from '../../api/ops'
import { queryK8sClusterList } from '../../api/k8s'
import { queryAssetHostList } from '../../api/asset'

const loading = ref(false)
const saving = ref(false)
const rows = ref([])
const total = ref(0)
const stats = ref({ total: 0, enabled: 0, failed: 0 })
const appOptions = ref([])
const k8sClusterOptions = ref([])
const notifyRuleOptions = ref([])
const imageRegistryOptions = ref([])
const executorHostOptions = ref([])
const templates = ref([])
const templateVisible = ref(false)
const editorVisible = ref(false)
const runVisible = ref(false)
const runDetailVisible = ref(false)
const runRows = ref([])
const runTotal = ref(0)
const selectedCategory = ref('全部模板')
const selectedTemplate = ref(null)
const activeTab = ref('pipelines')
const activeRunStageId = ref('')
const logBodyRef = ref()
const runRefreshTimer = ref()

const categories = ['全部模板', 'Java', 'Node.js', 'Go', 'Python', 'Vue', '空模板']

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  appId: undefined,
  env: '',
  status: '',
  techStack: ''
})

const runQuery = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  appId: undefined,
  pipelineId: undefined,
  env: '',
  status: ''
})

const form = reactive({
  id: undefined,
  name: '',
  appId: undefined,
  defaultBranch: '',
  env: 'test',
  techStack: 'custom',
  templateId: 0,
  executorHostId: undefined,
  status: 1,
  description: '',
  stages: []
})

const runForm = reactive({
  pipelineId: undefined,
  pipelineName: '',
  branch: '',
  env: 'test',
  imageTag: '',
  paramsText: ''
})

const currentRun = ref({ run: {}, stages: [] })

const currentApp = computed(() => appOptions.value.find((item) => Number(item.id) === Number(form.appId)))
const filteredTemplates = computed(() => {
  if (selectedCategory.value === '全部模板') return templates.value
  if (selectedCategory.value === '空模板') return []
  return templates.value.filter((item) => item.category === selectedCategory.value || item.techStack === selectedCategory.value)
})

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

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    appId: undefined,
    defaultBranch: '',
    env: 'test',
    techStack: 'custom',
    templateId: 0,
    executorHostId: undefined,
    status: 1,
    description: '',
    stages: []
  })
}

function parseStages(definitionJson) {
  if (!definitionJson) return []
  try {
    const data = JSON.parse(definitionJson)
    return Array.isArray(data.stages)
      ? data.stages.map((stage, index) => ({
          id: stage.id || `stage-${index + 1}`,
          name: stage.name || `阶段 ${index + 1}`,
          type: stage.type || 'command',
          timeoutSeconds: stage.timeoutSeconds || 1800,
          failurePolicy: stage.failurePolicy || 'stop',
          config: stage.config && typeof stage.config === 'object' ? stage.config : {},
          env: stage.env && typeof stage.env === 'object' ? stage.env : {}
        }))
      : []
  } catch {
    return []
  }
}

function stringifyDefinition() {
  return JSON.stringify({ stages: form.stages }, null, 2)
}

function defaultStage(type = 'command') {
  const next = form.stages.length + 1
  const names = {
    checkout: '代码拉取',
    command: '构建命令',
    test: '单元测试',
    dockerBuild: 'Docker 镜像构建',
    dockerPush: '上传镜像仓库',
    k8sDeploy: 'K8s 发布',
    manual: '人工确认',
    notify: '消息通知'
  }
  const stage = {
    id: `${type}-${Date.now()}-${next}`,
    name: names[type] || `阶段 ${next}`,
    type,
    timeoutSeconds: 1800,
    failurePolicy: type === 'notify' ? 'ignore' : 'stop',
    config: {},
    env: {}
  }
  normalizeStageConfig(stage)
  return stage
}

function stageHint(type) {
  return {
    checkout: '使用应用的 Git / SVN 仓库和执行分支拉取代码',
    command: '在执行节点的工作目录中执行自定义 Shell 命令',
    test: '执行自动化测试；失败时可按策略停止或继续',
    build: '执行编译、打包等构建命令',
    dockerBuild: '定义镜像目标并在执行节点执行 docker build',
    dockerPush: '推送前面构建出的同一镜像与版本',
    k8sDeploy: '通过执行节点的 kubectl 更新目标工作负载',
    manual: '暂停流水线，等待人工确认后继续',
    notify: '调用消息通知规则创建投递任务'
  }[type] || '配置当前阶段'
}

function stageTone(type) {
  return { checkout: 'source', command: 'command', test: 'test', build: 'build', dockerBuild: 'image', dockerPush: 'push', k8sDeploy: 'deploy', manual: 'manual', notify: 'notify' }[type] || 'command'
}

function normalizeStageConfig(stage) {
  if (!stage.config || typeof stage.config !== 'object') stage.config = {}
  if (['command', 'test', 'build'].includes(stage.type) && stage.config.script === undefined) {
    stage.config.script = ''
  }
  if (stage.type === 'dockerBuild') {
    if (!stage.config.registryId) stage.config.registryId = undefined
    if (!stage.config.dockerfile) stage.config.dockerfile = 'Dockerfile'
    if (!stage.config.context) stage.config.context = '.'
  }
  if (stage.type === 'dockerPush') {
    if (!stage.config.sourceStageId) stage.config.sourceStageId = undefined
    if (!stage.config.loginMode) stage.config.loginMode = 'registry'
  }
  if (stage.type === 'k8sDeploy') {
    if (!stage.config.clusterId) stage.config.clusterId = undefined
    if (!stage.config.workloadType) stage.config.workloadType = 'deployment'
    if (!stage.config.namespace) stage.config.namespace = ''
    if (!stage.config.workload) stage.config.workload = ''
    if (!stage.config.container) stage.config.container = ''
    if (!stage.config.repository) stage.config.repository = ''
    if (!stage.config.healthUrl) stage.config.healthUrl = ''
  }
  if (stage.type === 'notify' && !stage.config.notifyRuleId) {
    stage.config.notifyRuleId = undefined
  }
}

function dockerBuildStages(excludeId) {
  return form.stages.filter((item) => item.type === 'dockerBuild' && item.id !== excludeId)
}

function registryName(id) {
  const registry = imageRegistryOptions.value.find((item) => Number(item.id) === Number(id))
  return registry ? `${registry.name} · ${registry.address}${registry.namespace ? `/${registry.namespace}` : ''}` : '请选择镜像仓库'
}

async function loadApps() {
  appOptions.value = await queryOpsApplicationOptions()
}

async function loadTemplates() {
  templates.value = await queryOpsAppPipelineTemplates()
}

async function loadK8sClusters() {
  k8sClusterOptions.value = await queryK8sClusterList()
}

async function loadNotifyRules() {
  notifyRuleOptions.value = await queryNotifyRuleOptions({ scope: 'pipeline' })
}

async function loadImageRegistries() {
  imageRegistryOptions.value = await queryOpsImageRegistryList({ enabledOnly: 1 })
}

async function loadExecutorHosts() {
  const data = await queryAssetHostList({ pageNum: 1, pageSize: 1000 })
  executorHostOptions.value = data.list || []
}

function executorHostLabel(host) {
  const address = host.sshIp || host.privateIp || host.publicIp || '-'
  return `${host.hostName || `主机 #${host.id}`}（${address}）`
}

function executorHostName(id) {
  const host = executorHostOptions.value.find((item) => Number(item.id) === Number(id))
  return host ? executorHostLabel(host) : (id ? `资产主机 #${id}` : '未配置')
}


async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsAppPipelineList(query)
    rows.value = data.list || []
    total.value = data.total || 0
    stats.value = data.stats || { total: total.value, enabled: 0, failed: 0 }
  } finally {
    loading.value = false
  }
}

async function loadRuns() {
  loading.value = true
  try {
    const data = await queryOpsAppPipelineRunList(runQuery)
    runRows.value = data.list || []
    runTotal.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function openTemplateDialog() {
  selectedCategory.value = '全部模板'
  selectedTemplate.value = null
  templateVisible.value = true
}

function createBlankPipeline() {
  resetForm()
  form.stages = []
  templateVisible.value = false
  editorVisible.value = true
}

function useTemplate(template) {
  selectedTemplate.value = template
}

function confirmTemplate() {
  if (!selectedTemplate.value) {
    ElMessage.warning('请选择流水线模板，或使用空白流水线')
    return
  }
  resetForm()
  form.name = selectedTemplate.value.name.replace('通用模板', '流水线')
  form.techStack = selectedTemplate.value.techStack || 'custom'
  form.templateId = selectedTemplate.value.id
  form.description = selectedTemplate.value.description || ''
  form.stages = parseStages(selectedTemplate.value.definitionJson)
  templateVisible.value = false
  editorVisible.value = true
}

async function openEdit(row) {
  const detail = await opsAppPipelineInfo(row.id)
  const item = detail.pipeline || row
  Object.assign(form, {
    id: item.id,
    name: item.name || '',
    appId: item.appId,
    defaultBranch: item.defaultBranch || '',
    env: item.env || 'test',
    techStack: item.techStack || 'custom',
    templateId: item.templateId || 0,
    executorHostId: item.executorHostId || undefined,
    status: item.status || 1,
    description: item.description || '',
    stages: detail.stages || parseStages(item.definitionJson)
  })
  editorVisible.value = true
}

function fillFromApp() {
  if (!currentApp.value) return
  if (!form.defaultBranch) form.defaultBranch = currentApp.value.branch || 'master'
  if (!form.env) form.env = currentApp.value.env || 'test'
}

function addStage(type) {
  form.stages.push(defaultStage(type))
}

function removeStage(index) {
  form.stages.splice(index, 1)
}

function moveStage(index, direction) {
  const target = index + direction
  if (target < 0 || target >= form.stages.length) return
  const [stage] = form.stages.splice(index, 1)
  form.stages.splice(target, 0, stage)
}

function validateStages(stages = form.stages, executorHostId = form.executorHostId) {
  if (!stages.length) return '请至少配置一个执行阶段'
  const ids = new Set()
  for (let index = 0; index < stages.length; index += 1) {
    const stage = stages[index]
    if (!stage.id || ids.has(stage.id)) return `阶段 ${index + 1} 的标识重复，请删除后重新添加该阶段`
    ids.add(stage.id)
    if (!String(stage.name || '').trim()) return `请填写第 ${index + 1} 个阶段名称`
    if (['command', 'test', 'build'].includes(stage.type) && !String(stage.config?.script || '').trim()) return `请填写「${stage.name}」的执行命令`
    if (stage.type === 'notify' && !stage.config?.notifyRuleId) return `请选择「${stage.name}」使用的通知规则`
  }
  if (!executorHostId) return '请选择流水线执行节点；代码、构建、镜像和发布不会在 Ops Admin 容器内执行'
  return ''
}

async function submitPipeline() {
  if (!form.name || !form.appId) {
    ElMessage.warning('请填写流水线名称并选择所属应用')
    return
  }
  const stageError = validateStages()
  if (stageError) return ElMessage.warning(stageError)
  saving.value = true
  try {
    await saveOpsAppPipeline({
      id: form.id,
      name: form.name,
      appId: form.appId,
      defaultBranch: form.defaultBranch,
      env: form.env,
      techStack: form.techStack,
      templateId: form.templateId,
      executorHostId: form.executorHostId,
      status: form.status,
      description: form.description,
      definitionJson: stringifyDefinition()
    })
    ElMessage.success('保存成功')
    editorVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function openRun(row) {
  const detail = await opsAppPipelineInfo(row.id)
  const stageError = validateStages(detail.stages || [], detail.pipeline?.executorHostId || row.executorHostId)
  if (stageError) return ElMessage.warning(`无法执行：${stageError}`)
  Object.assign(runForm, {
    pipelineId: row.id,
    pipelineName: row.name,
    branch: row.defaultBranch || 'master',
    env: row.env || 'test',
    imageTag: '',
    paramsText: ''
  })
  runVisible.value = true
}

async function submitRun() {
  let params = {}
  if (runForm.paramsText.trim()) {
    try {
      params = JSON.parse(runForm.paramsText)
    } catch {
      ElMessage.warning('自定义参数需要是 JSON 对象')
      return
    }
  }
  const data = await runOpsAppPipeline({
    pipelineId: runForm.pipelineId,
    branch: runForm.branch,
    env: runForm.env,
    imageTag: runForm.imageTag,
    params
  })
  ElMessage.success(`流水线已开始执行：#${data.runId}`)
  runVisible.value = false
  await loadData()
  await openRunDetail(data.runId)
}

async function approveCurrentRun(decision) {
  const action = decision === 'approve' ? '通过' : '拒绝'
  const { value } = await ElMessageBox.prompt(`请输入审批说明，确认${action}当前发布。`, `人工审批${action}`, { inputType: 'textarea', confirmButtonText: action })
  await approveOpsAppPipelineRun({ runId: currentRun.value.run.id, decision, note: value || '' })
  ElMessage.success(`已${action}`)
  await openRunDetail(currentRun.value.run.id)
}

async function rollbackCurrentRun() {
  await ElMessageBox.confirm('将使用上一次成功执行的镜像版本重新执行 K8s 发布阶段，是否继续？', '回滚确认', { type: 'warning' })
  const data = await rollbackOpsAppPipelineRun(currentRun.value.run.id)
  ElMessage.success(`回滚任务已创建：#${data.runId}`)
  await openRunDetail(data.runId)
}

async function openRunDetail(id) {
  const data = await opsAppPipelineRunInfo(id)
  currentRun.value = { run: data.run || {}, stages: data.stages || [] }
  const preferredStage = currentRun.value.stages.find((stage) => ['failed', 'running', 'waiting_approval'].includes(stage.status)) || currentRun.value.stages[0]
  activeRunStageId.value = preferredStage?.id || ''
  runDetailVisible.value = true
  startRunRefresh()
  await nextTick()
  if (logBodyRef.value) logBodyRef.value.scrollTop = logBodyRef.value.scrollHeight
}

function startRunRefresh() {
  stopRunRefresh()
  if (currentRun.value.run?.status !== 'running') return
  runRefreshTimer.value = window.setInterval(async () => {
    if (!currentRun.value.run?.id) return
    const data = await opsAppPipelineRunInfo(currentRun.value.run.id)
    currentRun.value = { run: data.run || {}, stages: data.stages || [] }
    if (currentRun.value.run.status !== 'running') {
      stopRunRefresh()
      await loadData()
      if (activeTab.value === 'runs') await loadRuns()
    }
  }, 2500)
}

function stopRunRefresh() {
  if (runRefreshTimer.value) {
    window.clearInterval(runRefreshTimer.value)
    runRefreshTimer.value = undefined
  }
}

async function toggleStatus(row) {
  const next = Number(row.status) === 1 ? 2 : 1
  await updateOpsAppPipelineStatus({ id: row.id, status: next })
  ElMessage.success(next === 1 ? '已启用' : '已禁用')
  await loadData()
}

async function copyPipeline(row) {
  await copyOpsAppPipeline(row.id)
  ElMessage.success('复制成功')
  await loadData()
}

async function removePipeline(row) {
  await ElMessageBox.confirm(`确认删除流水线「${row.name}」及其执行记录？`, '删除流水线', { type: 'warning' })
  await deleteOpsAppPipeline(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

function statusType(status) {
  if (status === 'success' || Number(status) === 1) return 'success'
  if (status === 'running' || status === 'waiting_approval') return 'warning'
  if (status === 'failed') return 'danger'
  return 'info'
}

function statusText(status) {
  return { success: '成功', running: '执行中', failed: '失败', waiting: '等待中', waiting_approval: '待人工审批', 1: '启用', 2: '禁用' }[status] || status || '-'
}

function durationText(ms) {
  if (!ms) return '-'
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds} 秒`
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
}

function stageTypeText(type) {
  return {
    checkout: '代码拉取',
    command: '命令',
    test: '测试',
    build: '构建',
    dockerBuild: '镜像构建',
    dockerPush: '上传镜像仓库',
    k8sDeploy: 'K8s 发布',
    manual: '人工确认',
    notify: '消息通知'
  }[type] || type
}

const combinedLog = computed(() => {
  return (currentRun.value.stages || [])
    .map((stage) => `===== ${stage.stageName} / ${statusText(stage.status)} =====\n${stage.log || stage.summary || ''}`)
    .join('\n\n')
})

const activeRunStage = computed(() => {
  const stages = currentRun.value.stages || []
  return stages.find((stage) => String(stage.id) === String(activeRunStageId.value)) || stages[0] || {}
})

const activeRunLog = computed(() => activeRunStage.value.log || activeRunStage.value.summary || '该阶段暂未产生执行日志。')

function selectRunStage(stage) {
  activeRunStageId.value = stage.id
  nextTick(() => {
    if (logBodyRef.value) logBodyRef.value.scrollTop = 0
  })
}

function downloadRunLog() {
  const name = `${currentRun.value.run.pipelineName || 'pipeline'}-${currentRun.value.run.id || 'run'}.log`.replace(/[\\/:*?"<>|]/g, '_')
  const blob = new Blob([combinedLog.value], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  URL.revokeObjectURL(url)
}

watch(runDetailVisible, (visible) => {
  if (!visible) stopRunRefresh()
})

onMounted(async () => {
  await Promise.all([loadApps(), loadTemplates(), loadK8sClusters(), loadImageRegistries(), loadNotifyRules(), loadExecutorHosts()])
  await loadData()
})

onBeforeUnmount(stopRunRefresh)
</script>

<template>
  <div class="pipeline-page">
    <div class="hero-panel">
      <div>
        <span class="eyebrow">Cloud Native Delivery</span>
        <h1>CI/CD 流水线</h1>
        <p>从代码拉取、构建、测试、制品、发布到通知，统一编排应用交付流程。</p>
      </div>
      <div class="hero-stats">
        <div><strong>{{ stats.total || 0 }}</strong><span>流水线总数</span></div>
        <div><strong>{{ stats.enabled || 0 }}</strong><span>启用中</span></div>
        <div><strong>{{ stats.failed || 0 }}</strong><span>最近失败</span></div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="pipeline-tabs" @tab-change="activeTab === 'runs' && loadRuns()">
      <el-tab-pane label="流水线列表" name="pipelines">
        <div class="filter-panel">
          <el-form inline>
            <el-form-item label="应用">
              <el-select v-model="query.appId" clearable filterable placeholder="全部应用">
                <el-option v-for="item in appOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="环境">
              <el-select v-model="query.env" clearable placeholder="全部环境">
                <el-option label="dev" value="dev" />
                <el-option label="test" value="test" />
                <el-option label="staging" value="staging" />
                <el-option label="prod" value="prod" />
              </el-select>
            </el-form-item>
            <el-form-item label="技术栈">
              <el-select v-model="query.techStack" clearable placeholder="全部技术栈">
                <el-option label="Go" value="go" />
                <el-option label="Maven Java" value="maven" />
                <el-option label="Vue" value="vue" />
                <el-option label="自定义" value="custom" />
              </el-select>
            </el-form-item>
            <el-form-item label="关键字">
              <el-input v-model="query.keyword" clearable placeholder="搜索流水线 / 应用 / 仓库" @keyup.enter="loadData" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="loadData">搜索</el-button>
              <el-button @click="Object.assign(query, { pageNum: 1, keyword: '', appId: undefined, env: '', status: '', techStack: '' }); loadData()">重置</el-button>
            </el-form-item>
          </el-form>
          <el-button type="primary" @click="openTemplateDialog">新建流水线</el-button>
        </div>

        <el-table v-loading="loading" :data="rows" class="pipeline-table">
          <el-table-column label="流水线" min-width="220">
            <template #default="{ row }">
              <div class="name-cell">
                <strong>{{ row.name }}</strong>
                <span>{{ row.appName || '-' }} / {{ row.defaultBranch || '-' }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="repoUrl" label="仓库地址" min-width="260" show-overflow-tooltip />
          <el-table-column prop="env" label="环境" width="100" />
          <el-table-column prop="techStack" label="技术栈" width="120" />
          <el-table-column label="执行节点" min-width="180" show-overflow-tooltip><template #default="{ row }">{{ executorHostName(row.executorHostId) }}</template></el-table-column>
          <el-table-column prop="stageCount" label="阶段数" width="90" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column label="最近执行" min-width="160">
            <template #default="{ row }">
              <el-tag v-if="row.lastStatus" :type="statusType(row.lastStatus)" size="small">{{ statusText(row.lastStatus) }}</el-tag>
              <span class="muted">{{ row.lastRunAt || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="330" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openRun(row)">立即执行</el-button>
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link type="primary" @click="copyPipeline(row)">复制</el-button>
              <el-button link type="primary" @click="Object.assign(runQuery, { pipelineId: row.id }); activeTab = 'runs'; loadRuns()">历史</el-button>
              <el-button link :type="Number(row.status) === 1 ? 'warning' : 'success'" @click="toggleStatus(row)">
                {{ Number(row.status) === 1 ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="danger" @click="removePipeline(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="pager">
          <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" layout="total, prev, pager, next" :total="total" @current-change="loadData" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="流水线模板" name="templates">
        <div class="template-grid static">
          <div v-for="item in templates" :key="item.id" class="template-card">
            <strong>{{ item.name }}</strong>
            <p>{{ item.description }}</p>
            <span>{{ item.techStack }} / {{ item.stageCount }} 个阶段</span>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="执行记录" name="runs">
        <div class="filter-panel">
          <el-form inline>
            <el-form-item label="应用">
              <el-select v-model="runQuery.appId" clearable filterable placeholder="全部应用">
                <el-option v-for="item in appOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="runQuery.status" clearable placeholder="全部状态">
                <el-option label="成功" value="success" />
                <el-option label="执行中" value="running" />
                <el-option label="失败" value="failed" />
              </el-select>
            </el-form-item>
            <el-form-item label="关键字">
              <el-input v-model="runQuery.keyword" clearable placeholder="流水线 / 应用 / 镜像 Tag" @keyup.enter="loadRuns" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="loadRuns">搜索</el-button>
              <el-button @click="Object.assign(runQuery, { pageNum: 1, keyword: '', appId: undefined, pipelineId: undefined, env: '', status: '' }); loadRuns()">重置</el-button>
            </el-form-item>
          </el-form>
        </div>
        <el-table v-loading="loading" :data="runRows" class="pipeline-table">
          <el-table-column prop="id" label="执行编号" width="100" />
          <el-table-column prop="pipelineName" label="流水线" min-width="180" />
          <el-table-column prop="appName" label="应用" min-width="160" />
          <el-table-column prop="env" label="环境" width="90" />
          <el-table-column prop="branch" label="分支" width="130" />
          <el-table-column prop="imageTag" label="镜像 Tag" min-width="150" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag></template>
          </el-table-column>
          <el-table-column label="耗时" width="120">
            <template #default="{ row }">{{ durationText(row.durationMs) }}</template>
          </el-table-column>
          <el-table-column label="开始时间" min-width="180"><template #default="{ row }">{{ formatDateTime(row.createTime) }}</template></el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }"><el-button link type="primary" @click="openRunDetail(row.id)">详情/日志</el-button></template>
          </el-table-column>
        </el-table>
        <div class="pager">
          <el-pagination
            v-model:current-page="runQuery.pageNum"
            v-model:page-size="runQuery.pageSize"
            layout="total, prev, pager, next"
            :total="runTotal"
            @current-change="loadRuns"
          />
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="templateVisible" title="选择流水线模板" width="980px" class="template-dialog">
      <div class="template-picker">
        <aside>
          <button v-for="item in categories" :key="item" :class="{ active: selectedCategory === item }" @click="selectedCategory = item">
            {{ item }}
          </button>
        </aside>
        <main>
          <div v-if="selectedCategory === '空模板'" class="blank-template" @click="createBlankPipeline">
            <strong>空白流水线</strong>
            <p>从空白画布开始，自定义所有阶段。</p>
          </div>
          <div v-else class="template-grid">
            <div v-for="item in filteredTemplates" :key="item.id" class="template-card" :class="{ selected: selectedTemplate?.id === item.id }" @click="useTemplate(item)">
              <strong>{{ item.name }}</strong>
              <p>{{ item.description }}</p>
              <span>{{ item.techStack }} / {{ item.stageCount }} 个阶段</span>
            </div>
          </div>
        </main>
      </div>
      <template #footer>
        <el-button @click="templateVisible = false">取消</el-button>
        <el-button @click="createBlankPipeline">空白流水线</el-button>
        <el-button type="primary" @click="confirmTemplate">使用选中模板</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editorVisible" :title="form.id ? '编辑流水线' : '新建流水线'" width="1180px" class="pipeline-editor">
      <el-form label-width="100px">
        <div class="form-grid">
          <el-form-item label="流水线名称" required><el-input v-model="form.name" placeholder="请输入流水线名称" /></el-form-item>
          <el-form-item label="所属应用" required>
            <el-select v-model="form.appId" filterable placeholder="请选择应用" @change="fillFromApp">
              <el-option v-for="item in appOptions" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="默认分支"><el-input v-model="form.defaultBranch" placeholder="默认使用应用分支" /></el-form-item>
          <el-form-item label="默认环境">
            <el-select v-model="form.env">
              <el-option label="dev" value="dev" />
              <el-option label="test" value="test" />
              <el-option label="staging" value="staging" />
              <el-option label="prod" value="prod" />
            </el-select>
          </el-form-item>
          <el-form-item label="技术栈">
            <el-select v-model="form.techStack">
              <el-option label="Go" value="go" />
              <el-option label="Maven Java" value="maven" />
              <el-option label="Vue" value="vue" />
              <el-option label="自定义" value="custom" />
            </el-select>
          </el-form-item>
          <el-form-item label="执行节点" required>
            <el-select v-model="form.executorHostId" filterable placeholder="请选择执行代码、构建、镜像与发布的主机">
              <el-option v-for="host in executorHostOptions" :key="host.id" :label="executorHostLabel(host)" :value="host.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">启用</el-radio>
              <el-radio :value="2">禁用</el-radio>
            </el-radio-group>
          </el-form-item>
        </div>
        <el-alert type="warning" :closable="false" show-icon title="流水线阶段会通过 SSH 在所选资产主机上执行；请确保该主机已配置认证凭据，并安装 Git/SVN、Docker、kubectl 等所需工具。" />
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="2" placeholder="说明流水线用途、环境和风险提示" /></el-form-item>
      </el-form>

      <div class="stage-toolbar">
        <strong>阶段编排</strong>
        <div>
          <el-button size="small" @click="addStage('checkout')">代码拉取</el-button>
          <el-button size="small" @click="addStage('command')">命令</el-button>
          <el-button size="small" @click="addStage('test')">测试</el-button>
          <el-button size="small" @click="addStage('build')">构建</el-button>
          <el-button size="small" @click="addStage('dockerBuild')">镜像构建</el-button>
          <el-button size="small" @click="addStage('dockerPush')">上传镜像仓库</el-button>
          <el-button size="small" @click="addStage('k8sDeploy')">K8s 发布</el-button>
          <el-button size="small" @click="addStage('manual')">人工确认</el-button>
          <el-button size="small" @click="addStage('notify')">消息通知</el-button>
        </div>
      </div>
      <div class="stage-editor">
        <el-empty v-if="!form.stages.length" description="当前为空白流水线，请添加阶段" />
        <div v-for="(stage, index) in form.stages" :key="stage.id" class="stage-config-card">
          <div class="stage-card-heading">
            <span class="stage-type-badge" :class="stageTone(stage.type)">{{ String(index + 1).padStart(2, '0') }}</span>
            <div>
              <strong>{{ stageTypeText(stage.type) }}</strong>
              <small>{{ stageHint(stage.type) }}</small>
            </div>
          </div>
          <div class="stage-order-actions">
            <el-tooltip content="上移阶段"><el-button circle size="small" :disabled="index === 0" @click="moveStage(index, -1)">↑</el-button></el-tooltip>
            <el-tooltip content="下移阶段"><el-button circle size="small" :disabled="index === form.stages.length - 1" @click="moveStage(index, 1)">↓</el-button></el-tooltip>
          </div>
          <div class="stage-fields stage-main-fields">
            <label><span>阶段名称</span><el-input v-model="stage.name" placeholder="例如：构建应用镜像" /></label>
            <label><span>阶段类型</span><el-select v-model="stage.type" @change="normalizeStageConfig(stage)">
              <el-option label="代码拉取" value="checkout" />
              <el-option label="命令" value="command" />
              <el-option label="测试" value="test" />
              <el-option label="构建" value="build" />
              <el-option label="镜像构建" value="dockerBuild" />
              <el-option label="上传镜像仓库" value="dockerPush" />
              <el-option label="K8s 发布" value="k8sDeploy" />
              <el-option label="人工确认" value="manual" />
              <el-option label="消息通知" value="notify" />
            </el-select></label>
            <label><span>超时时间</span><div class="stage-timeout"><el-input-number v-model="stage.timeoutSeconds" :min="10" :max="7200" controls-position="right" /><em>秒</em></div></label>
            <label><span>失败策略</span><el-select v-model="stage.failurePolicy">
              <el-option label="失败终止" value="stop" />
              <el-option label="忽略继续" value="ignore" />
            </el-select></label>
          </div>
          <div v-if="['command', 'test', 'build'].includes(stage.type)" class="script-stage-config">
            <div class="script-stage-title"><b>执行脚本</b><span>在执行节点的工作目录内以 Shell 方式执行</span></div>
            <el-input v-model="stage.config.script" type="textarea" :rows="5" placeholder="请输入要执行的 Shell 命令，例如 npm run build / go test ./..." />
          </div>
          <div v-else-if="stage.type === 'dockerBuild'" class="image-stage-config">
            <div class="image-stage-title"><span class="image-stage-badge build">01</span><div><strong>定义镜像产物</strong><small>构建会自动标记为：仓库地址 / 命名空间 / 应用编码 : 分支-时间</small></div></div>
            <div class="image-stage-fields build-fields">
              <label><span>目标镜像仓库</span><el-select v-model="stage.config.registryId" filterable placeholder="选择构建后的镜像仓库"><el-option v-for="registry in imageRegistryOptions" :key="registry.id" :label="`${registry.name}（${registry.address}${registry.namespace ? '/' + registry.namespace : ''}）`" :value="registry.id" /></el-select></label>
              <label><span>Dockerfile</span><el-input v-model="stage.config.dockerfile" placeholder="Dockerfile" /></label>
              <label><span>构建上下文</span><el-input v-model="stage.config.context" placeholder="." /></label>
            </div>
            <div class="image-stage-preview"><span>将构建为</span><code>{{ registryName(stage.config.registryId) }} / {{ currentApp?.code || '<应用编码>' }} : {{ form.defaultBranch || '<分支>' }}-{{ '{时间}' }}</code></div>
          </div>
          <div v-else-if="stage.type === 'dockerPush'" class="image-stage-config">
            <div class="image-stage-title"><span class="image-stage-badge push">02</span><div><strong>推送已构建镜像</strong><small>引用镜像构建阶段，确保镜像地址和版本完全一致。</small></div></div>
            <div class="image-stage-fields push-fields">
              <label><span>镜像来源</span><el-select v-model="stage.config.sourceStageId" filterable placeholder="选择前面的镜像构建阶段"><el-option v-for="buildStage in dockerBuildStages(stage.id)" :key="buildStage.id" :label="`${buildStage.name} · ${registryName(buildStage.config.registryId)}`" :value="buildStage.id" /></el-select></label>
              <label><span>登录方式</span><el-radio-group v-model="stage.config.loginMode" class="login-mode-group"><el-radio-button value="registry">镜像仓库凭据</el-radio-button><el-radio-button value="executor">执行节点已登录</el-radio-button></el-radio-group></label>
            </div>
            <div class="image-stage-tip"><span>✓</span><p><b>推荐使用镜像仓库凭据</b>：从“镜像仓库”页面读取账号和密码；选择“执行节点已登录”时，平台不会执行 docker login。</p></div>
          </div>
          <div v-else-if="stage.type === 'k8sDeploy'" class="delivery-stage-config">
            <div class="delivery-stage-title"><b>发布目标</b><span>使用执行节点上的 kubectl 完成工作负载镜像更新与健康检查</span></div>
            <div class="stage-config-grid">
            <label><span>K8s 集群</span><el-select v-model="stage.config.clusterId" filterable placeholder="选择 K8s 集群">
              <el-option
                v-for="cluster in k8sClusterOptions"
                :key="cluster.id"
                :label="`${cluster.name}（${cluster.statusText || cluster.status || '-'} / ${cluster.version || '-'}）`"
                :value="cluster.id"
              />
            </el-select></label>
            <label><span>工作负载类型</span><el-select v-model="stage.config.workloadType" placeholder="工作负载类型">
              <el-option label="Deployment" value="deployment" />
              <el-option label="StatefulSet" value="statefulset" />
              <el-option label="DaemonSet" value="daemonset" />
            </el-select></label>
            <label><span>命名空间</span><el-input v-model="stage.config.namespace" placeholder="例如 default" /></label>
            <label><span>工作负载名称</span><el-input v-model="stage.config.workload" placeholder="默认使用应用编码" /></label>
            <label><span>容器名称</span><el-input v-model="stage.config.container" placeholder="需要替换镜像的容器" /></label>
            <label><span>镜像仓库地址</span><el-input v-model="stage.config.repository" placeholder="例如 registry.example.com/app/{{appCode}}" /></label>
            <label class="wide"><span>健康检查 URL（可选）</span><el-input v-model="stage.config.healthUrl" placeholder="例如 https://service/health" /></label>
            </div>
          </div>
          <div v-else-if="stage.type === 'checkout'" class="stage-note source-note"><b>代码来源</b><p>自动使用所属应用配置的 Git / SVN 仓库地址，并按本次执行选择的分支或 Tag 拉取代码。</p></div>
          <div v-else-if="stage.type === 'manual'" class="stage-note manual-note"><b>人工确认关卡</b><p>流水线会暂停并等待审批人选择“通过”或“拒绝”；通过后才会继续后续阶段。</p></div>
          <div v-else-if="stage.type === 'notify'" class="stage-notify-config">
            <div class="stage-note notify-note"><b>发送流水线通知</b><p>按所选规则创建投递任务，发送结果可在“消息通知 / 发送日志”中查看。</p></div>
            <label><span>通知规则</span><el-select v-model="stage.config.notifyRuleId" filterable placeholder="选择 CI/CD 流水线通知规则">
              <el-option v-for="rule in notifyRuleOptions" :key="rule.id" :label="`${rule.name}（${rule.channelIds?.length || 0} 个媒介）`" :value="rule.id" />
            </el-select></label>
          </div>
          <div class="stage-card-footer">
            <span>顺序 {{ index + 1 }} / {{ form.stages.length }}</span>
            <el-button link type="danger" @click="removeStage(index)">删除阶段</el-button>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="editorVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitPipeline">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="runVisible" title="立即执行流水线" width="680px">
      <el-form label-width="100px">
        <el-form-item label="流水线"><el-input v-model="runForm.pipelineName" disabled /></el-form-item>
        <el-form-item label="执行环境">
          <el-select v-model="runForm.env">
            <el-option label="dev" value="dev" />
            <el-option label="test" value="test" />
            <el-option label="staging" value="staging" />
            <el-option label="prod" value="prod" />
          </el-select>
        </el-form-item>
        <el-form-item label="分支/Tag"><el-input v-model="runForm.branch" /></el-form-item>
        <el-form-item label="镜像版本"><el-input :model-value="'自动生成：' + (runForm.branch || 'main').replace(/[^a-zA-Z0-9_.-]+/g, '-') + '-YYYYMMDDHHmmss'" disabled /></el-form-item>
        <el-form-item label="自定义参数"><el-input v-model="runForm.paramsText" type="textarea" :rows="4" placeholder='例如：{"version":"1.0.0"}' /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="runVisible = false">取消</el-button>
        <el-button type="primary" @click="submitRun">开始执行</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="runDetailVisible" title="流水线执行详情" width="1080px" class="run-detail-dialog">
      <div class="run-summary">
        <div><span>流水线</span><strong>{{ currentRun.run.pipelineName || '-' }}</strong></div>
        <div><span>状态</span><el-tag :type="statusType(currentRun.run.status)">{{ statusText(currentRun.run.status) }}</el-tag></div>
        <div><span>环境</span><strong>{{ currentRun.run.env || '-' }}</strong></div>
        <div><span>耗时</span><strong>{{ durationText(currentRun.run.durationMs) }}</strong></div>
      </div>
      <div class="run-detail-actions">
        <el-button type="primary" link @click="currentRun.run?.id && openRunDetail(currentRun.run.id)">刷新</el-button>
        <el-button type="primary" link @click="downloadRunLog">下载日志</el-button>
        <el-button v-if="currentRun.run.status === 'waiting_approval'" type="success" @click="approveCurrentRun('approve')">审批通过</el-button>
        <el-button v-if="currentRun.run.status === 'waiting_approval'" type="danger" @click="approveCurrentRun('reject')">审批拒绝</el-button>
        <el-button v-if="currentRun.run.status === 'success'" type="warning" @click="rollbackCurrentRun">回滚上一版本</el-button>
      </div>
      <div class="run-timeline" aria-label="流水线阶段时间线">
        <button
          v-for="(stage, index) in currentRun.stages"
          :key="stage.id"
          class="run-timeline-stage"
          :class="[stage.status, { active: String(stage.id) === String(activeRunStageId) }]"
          @click="selectRunStage(stage)"
        >
          <span class="timeline-order">{{ index + 1 }}</span>
          <span class="timeline-content"><strong>{{ stage.stageName }}</strong><small>{{ stageTypeText(stage.stageType) }} · {{ durationText(stage.durationMs) }}</small></span>
          <el-tag size="small" :type="statusType(stage.status)">{{ statusText(stage.status) }}</el-tag>
        </button>
      </div>
      <div class="stage-log-panel">
        <div class="stage-log-heading">
          <div><span>当前阶段</span><strong>{{ activeRunStage.stageName || '-' }}</strong><small>{{ stageTypeText(activeRunStage.stageType) }} · {{ statusText(activeRunStage.status) }}</small></div>
          <el-button type="primary" link @click="downloadRunLog">下载全部日志</el-button>
        </div>
        <pre ref="logBodyRef" class="run-log">{{ activeRunLog }}</pre>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.pipeline-page { padding: 24px; }
.hero-panel { display: flex; justify-content: space-between; gap: 24px; padding: 28px; margin-bottom: 18px; border: 1px solid #dbe7f7; border-radius: 14px; background: linear-gradient(135deg, #f8fbff 0%, #eef5ff 100%); }
.eyebrow { color: #2f6be6; font-size: 12px; font-weight: 800; letter-spacing: .08em; text-transform: uppercase; }
.hero-panel h1 { margin: 8px 0; color: #071b3d; font-size: 30px; }
.hero-panel p { margin: 0; color: #6b7c9b; }
.hero-stats { display: grid; grid-template-columns: repeat(3, 132px); gap: 12px; }
.hero-stats div { padding: 16px; border: 1px solid #dbe7f7; border-radius: 10px; background: #fff; }
.hero-stats strong { display: block; color: #2f6be6; font-size: 28px; }
.hero-stats span, .muted { color: #7d8daa; font-size: 13px; }
.pipeline-tabs { padding: 18px 22px; border: 1px solid #e3ebf7; border-radius: 14px; background: #fff; }
.filter-panel { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
:deep(.filter-panel .el-select) { width: 220px; }
:deep(.filter-panel .el-input) { width: 280px; }
.pipeline-table { border: 1px solid #edf2f9; border-radius: 10px; overflow: hidden; }
.name-cell { display: flex; flex-direction: column; gap: 4px; }
.name-cell strong { color: #0b5ed7; }
.name-cell span { color: #7d8daa; font-size: 12px; }
.pager { display: flex; justify-content: flex-end; padding-top: 16px; }
.template-picker { display: grid; grid-template-columns: 180px 1fr; min-height: 520px; border-top: 1px solid #e5edf8; border-bottom: 1px solid #e5edf8; }
.template-picker aside { padding: 16px 12px; background: #f5f8fc; border-right: 1px solid #e5edf8; }
.template-picker aside button { display: block; width: 100%; height: 40px; padding: 0 14px; border: 0; border-radius: 6px; background: transparent; color: #49617f; text-align: left; cursor: pointer; }
.template-picker aside button.active { background: #e7f0ff; color: #1677ff; font-weight: 700; }
.template-picker main { padding: 22px; }
.template-grid { display: grid; grid-template-columns: repeat(2, minmax(280px, 1fr)); gap: 16px; }
.template-grid.static { grid-template-columns: repeat(3, minmax(260px, 1fr)); }
.template-card, .blank-template { min-height: 140px; padding: 20px; border: 1px solid #d7e4f5; border-radius: 8px; background: #fff; cursor: pointer; }
.template-card.selected, .template-card:hover, .blank-template:hover { border-color: #2f6be6; box-shadow: 0 8px 24px rgba(47, 107, 230, .12); }
.template-card strong, .blank-template strong { color: #10213d; font-size: 16px; }
.template-card p, .blank-template p { color: #6b7c9b; }
.template-card span { color: #1677ff; font-weight: 700; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(320px, 1fr)); column-gap: 28px; }
.stage-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 18px 0 12px; padding: 16px; border: 1px solid #e5edf9; border-radius: 12px; background: #f8fbff; }
.stage-toolbar strong { color: #1b3760; font-size: 15px; }
.stage-toolbar > div { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 7px; }
.stage-editor { max-height: 520px; overflow: auto; padding: 2px 6px 2px 0; }
.stage-config-card { position: relative; margin-bottom: 14px; padding: 16px 18px; border: 1px solid #dfe9f8; border-radius: 12px; background: linear-gradient(135deg, #fff 0%, #f9fbff 100%); box-shadow: 0 4px 14px rgba(48, 76, 122, .035); }
.stage-card-heading { display: flex; align-items: center; gap: 10px; min-height: 34px; margin-bottom: 14px; padding-right: 88px; }
.stage-card-heading strong { display: block; color: #18355d; font-size: 14px; }
.stage-card-heading small { display: block; margin-top: 3px; color: #8291a8; font-size: 12px; }
.stage-type-badge { display: grid; width: 32px; height: 32px; flex: 0 0 auto; place-items: center; border-radius: 9px; color: #fff; font-size: 12px; font-weight: 800; box-shadow: 0 4px 10px rgba(52, 95, 180, .2); }
.stage-type-badge.source { background: linear-gradient(135deg, #2d83ee, #4f68e9); }.stage-type-badge.command { background: linear-gradient(135deg, #5a6fe8, #7c5ce5); }.stage-type-badge.test { background: linear-gradient(135deg, #21aa91, #3bc28d); }.stage-type-badge.build { background: linear-gradient(135deg, #e09a37, #f3b646); }.stage-type-badge.image { background: linear-gradient(135deg, #3a72ed, #5d5fe9); }.stage-type-badge.push { background: linear-gradient(135deg, #16a582, #2abb91); }.stage-type-badge.deploy { background: linear-gradient(135deg, #2b9ce9, #1bb6be); }.stage-type-badge.manual { background: linear-gradient(135deg, #9664de, #c46bf0); }.stage-type-badge.notify { background: linear-gradient(135deg, #e9796c, #ed9a5a); }
.stage-order-actions { position: absolute; top: 16px; right: 16px; display: flex; gap: 5px; }
.stage-card-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 12px; padding-top: 10px; border-top: 1px dashed #e2eaf5; color: #8a9ab3; font-size: 12px; }
.stage-fields { display: grid; gap: 12px; margin-bottom: 12px; }
.stage-main-fields { grid-template-columns: 1.35fr 1fr 160px 150px; }
.stage-fields label, .stage-config-grid label, .stage-notify-config label { display: flex; min-width: 0; flex-direction: column; gap: 6px; color: #637895; font-size: 12px; font-weight: 600; }
.stage-fields :deep(.el-select), .stage-config-grid :deep(.el-select), .stage-notify-config :deep(.el-select) { width: 100%; }
.stage-timeout { display: flex; align-items: center; gap: 8px; }.stage-timeout :deep(.el-input-number) { width: 118px; }.stage-timeout em { color: #8796ac; font-style: normal; font-weight: 400; }
.script-stage-config, .delivery-stage-config, .stage-notify-config { padding: 13px 15px; border: 1px solid #e1eaf7; border-radius: 10px; background: #fff; }
.script-stage-title, .delivery-stage-title { display: flex; align-items: baseline; gap: 10px; margin-bottom: 10px; }.script-stage-title b, .delivery-stage-title b { color: #25436b; font-size: 13px; }.script-stage-title span, .delivery-stage-title span { color: #8796ad; font-size: 12px; }
.stage-config-grid { display: grid; grid-template-columns: repeat(2, minmax(220px, 1fr)); gap: 12px; }.stage-config-grid .wide { grid-column: 1 / -1; }
.stage-note { padding: 13px 15px; border: 1px solid #dce8fb; border-radius: 10px; background: #f7faff; }.stage-note b { color: #305d99; font-size: 13px; }.stage-note p { margin: 5px 0 0; color: #7185a3; font-size: 12px; line-height: 1.65; }.manual-note { border-color: #ebe0fb; background: #fbf8ff; }.manual-note b { color: #7a58af; }.notify-note { margin-bottom: 12px; border-color: #fee5d7; background: #fffaf6; }.notify-note b { color: #b56b43; }
.image-stage-config { padding: 14px 16px; border: 1px solid #dfe8f7; border-radius: 10px; background: linear-gradient(135deg, #f9fbff 0%, #f5f8ff 100%); }
.image-stage-title { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
.image-stage-title strong { display: block; color: #223b64; font-size: 14px; line-height: 1.4; }
.image-stage-title small { display: block; margin-top: 2px; color: #8191aa; font-size: 12px; }
.image-stage-badge { display: grid; width: 29px; height: 29px; place-items: center; border-radius: 8px; color: #fff; font-size: 12px; font-weight: 800; }
.image-stage-badge.build { background: linear-gradient(135deg, #3a72ed, #5d5fe9); box-shadow: 0 4px 10px rgba(58, 114, 237, .24); }
.image-stage-badge.push { background: linear-gradient(135deg, #16a582, #2abb91); box-shadow: 0 4px 10px rgba(22, 165, 130, .22); }
.image-stage-fields { display: grid; gap: 12px; }
.image-stage-fields.build-fields { grid-template-columns: minmax(260px, 1.65fr) minmax(150px, .7fr) minmax(120px, .55fr); }
.image-stage-fields.push-fields { grid-template-columns: minmax(280px, 1.4fr) minmax(310px, 1fr); }
.image-stage-fields label { display: flex; min-width: 0; flex-direction: column; gap: 6px; }
.image-stage-fields label > span { color: #637895; font-size: 12px; font-weight: 600; }
.image-stage-fields :deep(.el-select) { width: 100%; }
.image-stage-preview { display: flex; align-items: center; gap: 8px; margin-top: 12px; padding: 9px 11px; border-radius: 7px; background: #eef4ff; color: #7185a2; font-size: 12px; }
.image-stage-preview code { overflow: hidden; color: #315fc4; text-overflow: ellipsis; white-space: nowrap; font-family: Consolas, Monaco, monospace; font-size: 12px; }
.login-mode-group { display: flex; width: 100%; }
.login-mode-group :deep(.el-radio-button) { flex: 1; }
.login-mode-group :deep(.el-radio-button__inner) { width: 100%; padding: 9px 8px; }
.image-stage-tip { display: flex; gap: 8px; margin-top: 12px; padding: 9px 11px; border-radius: 7px; background: #f0fbf6; color: #56746c; font-size: 12px; line-height: 1.55; }
.image-stage-tip > span { display: grid; flex: 0 0 auto; width: 17px; height: 17px; place-items: center; border-radius: 50%; background: #20b485; color: #fff; font-size: 11px; font-weight: 800; }
.image-stage-tip p { margin: 0; }
@media (max-width: 900px) { .stage-toolbar { align-items: flex-start; flex-direction: column; }.stage-toolbar > div { justify-content: flex-start; }.stage-main-fields, .image-stage-fields.build-fields, .image-stage-fields.push-fields, .stage-config-grid { grid-template-columns: 1fr; }.stage-config-grid .wide { grid-column: auto; } }
.run-summary { display: grid; grid-template-columns: 2fr 1fr 1fr 1fr; gap: 12px; margin-bottom: 16px; }
.run-summary div { padding: 14px; border: 1px solid #e3ebf7; border-radius: 8px; background: #fbfdff; }
.run-summary span { display: block; margin-bottom: 6px; color: #7d8daa; }
.run-detail-actions { display: flex; justify-content: flex-end; gap: 12px; margin: -4px 0 12px; }
.run-timeline { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 10px; padding: 14px; margin-bottom: 16px; border: 1px solid #e3ebf7; border-radius: 12px; background: #f9fbff; }
.run-timeline-stage { position: relative; display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; gap: 8px; align-items: center; min-height: 70px; padding: 10px; border: 1px solid #e2eaf6; border-radius: 9px; color: #263b5a; background: #fff; text-align: left; cursor: pointer; }
.run-timeline-stage:hover, .run-timeline-stage.active { border-color: #4d7ef3; box-shadow: 0 4px 14px rgba(47, 107, 230, .12); }
.timeline-order { display: grid; width: 26px; height: 26px; place-items: center; border-radius: 50%; color: #5c6f8b; background: #edf2fa; font-weight: 700; }
.run-timeline-stage.success .timeline-order { color: #fff; background: #67c23a; }
.run-timeline-stage.running .timeline-order, .run-timeline-stage.waiting_approval .timeline-order { color: #fff; background: #e6a23c; }
.run-timeline-stage.failed .timeline-order { color: #fff; background: #f56c6c; }
.timeline-content { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.timeline-content strong { overflow: hidden; color: #10213d; text-overflow: ellipsis; white-space: nowrap; }
.timeline-content small { overflow: hidden; color: #8190a8; text-overflow: ellipsis; white-space: nowrap; }
.stage-log-panel { overflow: hidden; border: 1px solid #e3ebf7; border-radius: 12px; }
.stage-log-heading { display: flex; align-items: center; justify-content: space-between; min-height: 70px; padding: 12px 16px; border-bottom: 1px solid #e8eef7; background: #fbfdff; }
.stage-log-heading div { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
.stage-log-heading span, .stage-log-heading small { color: #8392aa; font-size: 12px; }
.stage-log-heading strong { overflow: hidden; color: #203754; text-overflow: ellipsis; white-space: nowrap; }
.run-log { min-height: 480px; max-height: 560px; margin: 0; padding: 16px; overflow: auto; border-radius: 10px; background: #111827; color: #d6e2ff; font-family: Consolas, Monaco, monospace; font-size: 13px; line-height: 1.6; white-space: pre-wrap; }
</style>
