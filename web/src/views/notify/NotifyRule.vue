<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteNotifyRule,
  notifyRuleInfo,
  queryNotifyChannelOptions,
  queryNotifyRuleList,
  queryNotifyTemplateOptions,
  saveNotifyRule,
  testNotifyRule
} from '../../api/ops'

const loading = ref(false)
const saving = ref(false)
const optionsLoading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const activeStep = ref(0)
const rows = ref([])
const total = ref(0)
const templateOptions = ref([])
const channelOptions = ref([])

const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', scope: '', status: '' })
const form = reactive({
  id: undefined,
  name: '',
  scope: 'monitor',
  events: ['firing', 'recovered'],
  templateId: undefined,
  channelIds: [],
  status: 1,
  description: ''
})

const scopeOptions = [
  { label: '作业编排', value: 'job' },
  { label: 'CI/CD 流水线', value: 'pipeline' },
  { label: '定时任务', value: 'schedule' },
  { label: '监控告警', value: 'monitor' }
]
const filterScopeOptions = scopeOptions

const eventOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '等待人工确认', value: 'waiting_approval' },
  { label: '已拒绝', value: 'rejected' },
  { label: '告警触发', value: 'firing' },
  { label: '告警恢复', value: 'recovered' },
  { label: '全部事件', value: 'all' }
]

const channelTypeLabels = {
  dingtalk: '钉钉机器人',
  wecom: '企业微信机器人',
  feishu: '飞书机器人',
  webhook: '自定义 Webhook'
}

const selectedTemplate = computed(() => templateOptions.value.find((item) => Number(item.id) === Number(form.templateId)))
const compatibleTemplateOptions = computed(() => templateOptions.value.filter((item) => {
  const templateScope = item.scope || 'all'
  if (form.scope === 'all') return true
  if (templateScope === 'all') return true
  return templateScope === form.scope
}))
const selectedChannels = computed(() => channelOptions.value.filter((item) => form.channelIds.map(Number).includes(Number(item.id))))
const wizardSteps = [
  { title: '触发场景', description: '选择何时通知' },
  { title: '发送方式', description: '选择模板与媒介' },
  { title: '确认启用', description: '检查路由并保存' }
]
const routeWarnings = computed(() => {
  const warnings = []
  if (!form.scope || form.scope === 'all') warnings.push('请指定具体的业务场景')
  if (!form.events.length) warnings.push('至少选择一个触发事件')
  if (!selectedTemplate.value) warnings.push('请选择消息模板')
  if (selectedTemplate.value && !compatibleTemplateOptions.value.some((item) => Number(item.id) === Number(selectedTemplate.value.id))) {
    warnings.push('消息模板与适用范围不一致')
  }
  if (!selectedChannels.value.length) warnings.push('请选择通知媒介')
  const templateType = selectedTemplate.value?.channelType
  if (templateType && selectedChannels.value.some((item) => item.channelType !== templateType)) {
    warnings.push('模板与通知媒介类型不一致')
  }
  return warnings
})

function scopeLabel(value) {
  if (!value) return '未选择范围'
  if (value === 'all') return '需指定范围'
  return scopeOptions.find((item) => item.value === value)?.label || value
}

function templateScopeLabel(value) {
  return value === 'all' ? '通用模板' : scopeLabel(value)
}

function eventLabel(value) {
  return eventOptions.find((item) => item.value === value)?.label || value
}

function typeLabel(value) {
  return channelTypeLabels[value] || value || '-'
}

function defaultEventsForScope(scope) {
  if (scope === 'monitor') return ['firing', 'recovered']
  if (scope === 'job') return ['failed', 'waiting_approval', 'rejected']
  if (scope === 'pipeline') return ['success', 'failed', 'waiting_approval', 'rejected']
  return ['success', 'failed', 'recovered']
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    scope: 'monitor',
    events: ['firing', 'recovered'],
    templateId: undefined,
    channelIds: [],
    status: 1,
    description: ''
  })
}

async function loadBaseOptions() {
  optionsLoading.value = true
  try {
    const [templates, channels] = await Promise.all([
      queryNotifyTemplateOptions(),
      queryNotifyChannelOptions()
    ])
    templateOptions.value = Array.isArray(templates) ? templates : []
    channelOptions.value = Array.isArray(channels) ? channels : []
  } catch (error) {
    templateOptions.value = []
    channelOptions.value = []
    ElMessage.error(error?.message || '加载消息模板和通知媒介失败')
  } finally {
    optionsLoading.value = false
  }
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryNotifyRuleList(query)
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

async function openCreate() {
  isEdit.value = false
  activeStep.value = 0
  resetForm()
  dialogVisible.value = true
  await loadBaseOptions()
}

async function openEdit(row) {
  isEdit.value = true
  activeStep.value = 0
  dialogVisible.value = true
  const [data] = await Promise.all([
    notifyRuleInfo(row.id),
    loadBaseOptions()
  ])
  Object.assign(form, {
    id: data.id,
    name: data.name || '',
    scope: data.scope === 'all' ? undefined : (data.scope || undefined),
    events: data.events || [],
    templateId: data.templateId,
    channelIds: data.channelIds || [],
    status: data.status || 1,
    description: data.description || ''
  })
}

function handleScopeChange(value) {
  form.events = defaultEventsForScope(value)
  if (!compatibleTemplateOptions.value.some((item) => Number(item.id) === Number(form.templateId))) {
    form.templateId = undefined
    form.channelIds = []
  }
}

function handleEventsChange(values) {
  const last = values[values.length - 1]
  form.events = last === 'all' ? ['all'] : values.filter((item) => item !== 'all')
}

function handleTemplateChange(value) {
  const template = templateOptions.value.find((item) => Number(item.id) === Number(value))
  const templateScope = template?.scope || 'all'
  if (!template || form.scope !== 'all' || templateScope === 'all') return
  form.scope = templateScope
  form.events = defaultEventsForScope(templateScope)
  ElMessage.info(`已根据模板切换到「${scopeLabel(templateScope)}」场景`)
}

function channelDisabled(item) {
  return Boolean(selectedTemplate.value && item.channelType !== selectedTemplate.value.channelType)
}

function nextStep() {
  if (activeStep.value === 0) {
    if (!form.name.trim()) return ElMessage.warning('请先填写规则名称')
    if (!form.events.length) return ElMessage.warning('请至少选择一个触发事件')
  }
  if (activeStep.value === 1 && routeWarnings.value.length) {
    return ElMessage.warning(routeWarnings.value[0])
  }
  activeStep.value = Math.min(activeStep.value + 1, wizardSteps.length - 1)
}

function previousStep() {
  activeStep.value = Math.max(activeStep.value - 1, 0)
}

async function submit() {
  if (!form.name.trim() || routeWarnings.value.length) {
    ElMessage.warning(routeWarnings.value[0] || '请填写规则名称')
    return
  }
  saving.value = true
  try {
    await saveNotifyRule(form)
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function handleTest(row) {
  await ElMessageBox.confirm(
    `测试会按照规则「${row.name}」向已配置媒介发送一条真实消息，是否继续？`,
    '测试通知规则',
    { type: 'warning', confirmButtonText: '确认发送', cancelButtonText: '取消' }
  )
  const result = await testNotifyRule(row.id)
  ElMessage.success(`测试消息已进入发送队列，共 ${result?.queued || 0} 条`)
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除通知规则「${row.name}」吗？`, '删除规则', { type: 'warning' })
  await deleteNotifyRule(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

function templateName(id) {
  return templateOptions.value.find((item) => Number(item.id) === Number(id))?.name || id || '-'
}

function channelNames(ids) {
  return (ids || [])
    .map((id) => channelOptions.value.find((item) => Number(item.id) === Number(id))?.name || id)
    .join('、') || '-'
}

watch(() => form.templateId, () => {
  if (!selectedTemplate.value) return
  form.channelIds = form.channelIds.filter((id) => {
    const channel = channelOptions.value.find((item) => Number(item.id) === Number(id))
    return channel?.channelType === selectedTemplate.value.channelType
  })
})

onMounted(async () => {
  await loadBaseOptions()
  await loadData()
})
</script>

<template>
  <div class="page-card notify-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">通知规则</h2>
        <p class="page-desc">将业务事件、消息模板和通知媒介组合为可复用的发送路由。</p>
      </div>
      <el-button type="primary" @click="openCreate">新增规则</el-button>
    </div>

    <div class="toolbar">
      <el-input v-model="query.keyword" clearable placeholder="搜索规则名称 / 描述" style="width: 260px" @keyup.enter="loadData" />
      <el-select v-model="query.scope" clearable placeholder="适用范围" style="width: 160px">
        <el-option v-for="item in filterScopeOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-select v-model="query.status" clearable placeholder="状态" style="width: 120px">
        <el-option label="启用" value="1" />
        <el-option label="禁用" value="2" />
      </el-select>
      <el-button type="primary" @click="loadData">搜索</el-button>
    </div>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="name" label="规则名称" min-width="180" />
      <el-table-column label="适用范围" width="130">
        <template #default="{ row }">
          <el-tag :type="row.scope === 'all' ? 'warning' : 'info'" effect="plain">{{ scopeLabel(row.scope) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="触发事件" min-width="230">
        <template #default="{ row }">
          <el-tag v-for="item in row.events || []" :key="item" class="tag" effect="plain">{{ eventLabel(item) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="消息模板" min-width="170">
        <template #default="{ row }">{{ templateName(row.templateId) }}</template>
      </el-table-column>
      <el-table-column label="通知媒介" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">{{ channelNames(row.channelIds) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="90" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="190" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="handleTest(row)">测试</el-button>
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :total="total" layout="total, sizes, prev, pager, next" @current-change="loadData" @size-change="loadData" />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑通知规则' : '新建通知规则'" width="880px" destroy-on-close>
      <div class="notify-wizard-steps">
        <button v-for="(step, index) in wizardSteps" :key="step.title" type="button" class="notify-wizard-step" :class="{ active: activeStep === index, done: activeStep > index }" @click="index < activeStep && (activeStep = index)">
          <span>{{ index + 1 }}</span><strong>{{ step.title }}</strong><small>{{ step.description }}</small>
        </button>
      </div>
      <el-form class="notify-wizard-form" label-position="top">
        <template v-if="activeStep === 0">
          <div class="wizard-heading"><h3>什么情况需要通知？</h3><p>先定义场景和触发事件，系统会自动推荐适用模板。</p></div>
          <el-form-item label="规则名称" required><el-input v-model="form.name" placeholder="例如：生产环境告警通知" /></el-form-item>
          <el-form-item label="适用范围">
            <el-select v-model="form.scope" style="width: 100%" @change="handleScopeChange">
              <el-option v-for="item in scopeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="触发事件" required>
            <el-checkbox-group v-model="form.events" class="event-choice-grid" @change="handleEventsChange">
              <el-checkbox v-for="item in eventOptions" :key="item.value" :value="item.value" border>{{ item.label }}</el-checkbox>
            </el-checkbox-group>
          </el-form-item>
        </template>
        <template v-else-if="activeStep === 1">
          <div class="wizard-heading"><h3>发给谁，如何呈现？</h3><p>模板和媒介类型会自动校验；可同时选择多个同类型媒介。</p></div>
          <el-form-item label="消息模板" required>
          <el-select
            v-model="form.templateId"
            filterable
            :loading="optionsLoading"
            no-data-text="暂无已启用的消息模板，请先在消息模板中创建或启用模板"
            placeholder="选择与媒介类型一致的模板"
            style="width: 100%"
            @change="handleTemplateChange"
          >
            <el-option v-for="item in compatibleTemplateOptions" :key="item.id" :label="`${item.name} · ${templateScopeLabel(item.scope || 'all')} · ${typeLabel(item.channelType)}`" :value="item.id" />
          </el-select>
          </el-form-item>
          <el-form-item label="通知媒介" required>
          <el-select v-model="form.channelIds" multiple filterable :loading="optionsLoading" placeholder="可选择多个同类型媒介" style="width: 100%">
            <el-option v-for="item in channelOptions" :key="item.id" :label="`${item.name} · ${typeLabel(item.channelType)}`" :value="item.id" :disabled="channelDisabled(item)" />
          </el-select>
          </el-form-item>
          <el-alert v-if="routeWarnings.length" :title="routeWarnings.join('；')" type="warning" :closable="false" show-icon />
        </template>
        <template v-else>
          <div class="wizard-heading"><h3>确认通知路由</h3><p>保存后即可从规则列表发送测试消息，确认实际触达效果。</p></div>
          <div class="route-preview">
            <div class="route-chain">
              <span class="route-node">{{ form.events.length ? form.events.map(eventLabel).join('、') : '未选事件' }}</span>
              <span class="route-arrow">→</span>
              <span class="route-node">{{ selectedTemplate?.name || '未选模板' }}</span>
              <span class="route-arrow">→</span>
              <span class="route-node">{{ selectedChannels.length ? selectedChannels.map((item) => item.name).join('、') : '未选媒介' }}</span>
            </div>
            <div v-if="selectedTemplate" class="route-meta">适用场景：{{ templateScopeLabel(selectedTemplate.scope || 'all') }} · 消息格式：{{ typeLabel(selectedTemplate.channelType) }}</div>
            <el-alert v-if="routeWarnings.length" :title="routeWarnings.join('；')" type="warning" :closable="false" show-icon />
            <el-alert v-else title="路由配置完整，保存后可在列表中发送测试消息。" type="success" :closable="false" show-icon />
          </div>
          <el-form-item label="保存后状态" class="wizard-status-field">
            <el-radio-group v-model="form.status"><el-radio :value="1">立即启用</el-radio><el-radio :value="2">暂存为禁用</el-radio></el-radio-group>
          </el-form-item>
          <el-form-item label="说明（可选）"><el-input v-model="form.description" type="textarea" :rows="3" placeholder="说明通知对象、使用场景或值班要求" /></el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button v-if="activeStep" @click="previousStep">上一步</el-button>
        <el-button v-if="activeStep < wizardSteps.length - 1" type="primary" @click="nextStep">下一步</el-button>
        <el-button v-else type="primary" :loading="saving" @click="submit">保存规则</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.notify-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-title { margin: 0 0 8px; font-size: 24px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar { display: flex; gap: 12px; flex-wrap: wrap; }
.tag { margin-right: 6px; margin-bottom: 4px; }
.pager { display: flex; justify-content: flex-end; }
.notify-wizard-steps { display:grid; grid-template-columns:repeat(3, 1fr); gap:8px; margin:0 0 22px; padding:8px; border:1px solid #e1eaf8; border-radius:12px; background:#f7faff; }
.notify-wizard-step { display:grid; grid-template-columns:28px 1fr; column-gap:8px; align-items:center; min-height:54px; padding:8px 10px; color:#8a9ab3; text-align:left; border:0; border-radius:9px; background:transparent; cursor:default; }
.notify-wizard-step span { display:grid; width:26px; height:26px; place-items:center; grid-row:span 2; border:1px solid #ced9ea; border-radius:50%; color:#7d8da6; background:#fff; font-size:12px; font-weight:700; }
.notify-wizard-step strong { color:inherit; font-size:13px; }
.notify-wizard-step small { margin-top:2px; color:inherit; font-size:12px; }
.notify-wizard-step.done { cursor:pointer; color:#4f73c6; }
.notify-wizard-step.active { color:#1f5fce; background:#fff; box-shadow:0 4px 14px rgba(44, 84, 160, .08); }
.notify-wizard-step.active span { border-color:#4f73ff; color:#fff; background:#4f73ff; }
.notify-wizard-form { min-height:330px; }
.wizard-heading { margin:0 0 18px; }
.wizard-heading h3 { margin:0 0 5px; color:#1f2a44; font-size:18px; }
.wizard-heading p { margin:0; color:#7485a7; font-size:13px; line-height:1.6; }
.event-choice-grid { display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:10px; width:100%; }
.event-choice-grid :deep(.el-checkbox) { display:flex; width:100%; height:38px; margin-right:0; }
.wizard-status-field { margin-top:18px; }
.route-preview { width: 100%; padding: 14px; border: 1px solid #dce6f5; border-radius: 6px; background: #f7faff; }
.route-chain { display: flex; align-items: center; gap: 10px; min-width: 0; margin-bottom: 10px; }
.route-node { min-width: 0; padding: 7px 10px; overflow: hidden; color: #264d85; text-overflow: ellipsis; white-space: nowrap; border: 1px solid #c9dbf5; border-radius: 5px; background: #fff; }
.route-arrow { flex: 0 0 auto; color: #6b8fc7; font-size: 18px; }
.route-meta { margin-bottom: 10px; color: #7282a0; font-size: 13px; }
@media (max-width: 720px) { .notify-wizard-steps, .event-choice-grid { grid-template-columns:1fr; } .notify-wizard-form { min-height:0; } }
</style>
