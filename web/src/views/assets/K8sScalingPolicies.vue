<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Clock, DataAnalysis, Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'
import {
	batchDeleteK8sScalingPolicies,
	batchUpdateK8sScalingPolicyStatus,
  createK8sScalingPolicy,
  deleteK8sScalingPolicy,
  queryK8sClusterList,
  queryK8sClusterOverview,
  queryK8sScalingPolicies,
	updateK8sScalingPolicy,
  updateK8sScalingPolicyStatus
} from '../../api/k8s'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const activeTab = ref('hpa')
const clusterOptions = ref([])
const selectedClusterId = ref(0)
const workloads = ref([])
const policies = ref([])
const selectedPolicies = ref([])
const editingPolicy = ref(null)
const hydratingForm = ref(false)

const form = reactive({
  name: '',
  namespace: '',
  targets: [],
  minReplicas: 1,
  maxReplicas: 10,
  cpuEnabled: true,
  cpuUtilization: 80,
  memoryEnabled: false,
  memoryUtilization: 80,
  cronExpr: '0 0 9 * * *',
  scheduledReplicas: 2
})

const cronFields = reactive({
  second: '0',
  minute: '0',
  hour: '9',
  day: '*',
  month: '*',
  week: '*'
})

const cronFieldOptions = [
  { key: 'second', label: '秒', range: '0-59' },
  { key: 'minute', label: '分', range: '0-59' },
  { key: 'hour', label: '时', range: '0-23' },
  { key: 'day', label: '日', range: '1-31' },
  { key: 'month', label: '月', range: '1-12' },
  { key: 'week', label: '周', range: '0-6' }
]

const cronPresets = [
  { label: '每 1 分钟', fields: ['0', '*', '*', '*', '*', '*'], summary: '每分钟执行' },
  { label: '每 5 分钟', fields: ['0', '*/5', '*', '*', '*', '*'], summary: '每 5 分钟执行' },
  { label: '每 10 分钟', fields: ['0', '*/10', '*', '*', '*', '*'], summary: '每 10 分钟执行' },
  { label: '每小时整点', fields: ['0', '0', '*', '*', '*', '*'], summary: '每小时整点执行' },
  { label: '每天 02:00', fields: ['0', '0', '2', '*', '*', '*'], summary: '每天 02:00 执行' },
  { label: '每周一 02:00', fields: ['0', '0', '2', '*', '*', '1'], summary: '每周一 02:00 执行' }
]

const selectedCluster = computed(() => clusterOptions.value.find((item) => item.id === selectedClusterId.value))
const namespaces = computed(() => [...new Set(workloads.value.map((item) => item.namespace).filter(Boolean))].sort())
const workloadOptions = computed(() => workloads.value
  .filter((item) => ['Deployment', 'StatefulSet'].includes(item.type) && (!form.namespace || item.namespace === form.namespace))
  .map((item) => ({
    value: `${item.type.toLowerCase()}/${item.name}`,
    label: `${item.name} · ${item.type}`,
    namespace: item.namespace,
    type: item.type,
    name: item.name
  })))
const visiblePolicies = computed(() => policies.value.filter((item) => Number(item.clusterId) === Number(selectedClusterId.value)))
const currentPolicyTitle = computed(() => activeTab.value === 'hpa' ? 'HPA 水平伸缩' : '定时伸缩')
const inactiveSelectedPolicies = computed(() => selectedPolicies.value.filter((item) => Number(item.status) !== 1))
const dialogTitle = computed(() => `${editingPolicy.value ? '编辑' : '新增'}${activeTab.value === 'hpa' ? ' HPA 水平伸缩' : '定时伸缩'}`)
const dialogSubmitText = computed(() => editingPolicy.value ? '保存修改' : '保存为未启用')
const dialogFooterText = computed(() => editingPolicy.value && Number(editingPolicy.value.status) === 1
  ? '保存后会立即更新已启用的伸缩资源。'
  : '保存后不会立即改变集群。')
const generatedCronExpression = computed(() => cronFieldOptions.map((item) => cronFields[item.key].trim() || '*').join(' '))
const activeCronPreset = computed(() => cronPresets.find((preset) => preset.fields.join(' ') === generatedCronExpression.value)?.label || '')
const cronScheduleText = computed(() => cronPresets.find((preset) => preset.fields.join(' ') === generatedCronExpression.value)?.summary || '按自定义计划执行')

function syncCronFields(expression = form.cronExpr) {
  const fields = String(expression || '').trim().split(/\s+/).filter(Boolean)
  const values = fields.length === 5 ? ['0', ...fields] : fields
  if (values.length !== 6) return
  cronFieldOptions.forEach((item, index) => { cronFields[item.key] = values[index] })
}

function syncCronExpression() {
  form.cronExpr = generatedCronExpression.value
}

function applyCronPreset(preset) {
  cronFieldOptions.forEach((item, index) => { cronFields[item.key] = preset.fields[index] })
  syncCronExpression()
}

function assignForm(values) {
	hydratingForm.value = true
	Object.assign(form, values)
	syncCronFields(values.cronExpr)
	nextTick(() => { hydratingForm.value = false })
}

function resetForm() {
  assignForm({
    name: '',
    namespace: namespaces.value[0] || '',
    targets: [],
    minReplicas: 1,
    maxReplicas: 10,
    cpuEnabled: true,
    cpuUtilization: 80,
    memoryEnabled: false,
    memoryUtilization: 80,
    cronExpr: '0 0 9 * * *',
    scheduledReplicas: 2
  })
}

async function loadPolicies() {
  policies.value = await queryK8sScalingPolicies(activeTab.value)
}

async function loadClusterData() {
  if (!selectedClusterId.value) return
  const data = await queryK8sClusterOverview(selectedClusterId.value, true)
  workloads.value = data.workloads || []
  if (!form.namespace && workloads.value.length) form.namespace = workloads.value[0].namespace
}

async function load() {
  loading.value = true
  try {
    clusterOptions.value = await queryK8sClusterList()
    if (!selectedClusterId.value || !clusterOptions.value.some((item) => item.id === selectedClusterId.value)) {
      selectedClusterId.value = clusterOptions.value[0]?.id || 0
    }
    await Promise.all([loadClusterData(), loadPolicies()])
  } catch (error) {
    ElMessage.error(error.message || '工作负载伸缩策略加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
	editingPolicy.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
	editingPolicy.value = row
	assignForm({
		name: row.name,
		namespace: row.namespace,
		targets: (row.targets || []).map((target) => `${target.workloadType}/${target.workloadName}`),
		minReplicas: Number(row.minReplicas || 1),
		maxReplicas: Number(row.maxReplicas || 10),
		cpuEnabled: Boolean(row.cpuEnabled),
		cpuUtilization: Number(row.cpuUtilization || 80),
		memoryEnabled: Boolean(row.memoryEnabled),
		memoryUtilization: Number(row.memoryUtilization || 80),
		cronExpr: row.cronExpr || '0 0 9 * * *',
		scheduledReplicas: Number(row.scheduledReplicas || 0)
	})
	dialogVisible.value = true
}

function targetPayload() {
  return form.targets.map((value) => {
    const [workloadType, ...nameParts] = value.split('/')
    return { workloadType, workloadName: nameParts.join('/') }
  })
}

async function submit() {
  if (!form.name.trim() || !form.namespace || !form.targets.length) {
    ElMessage.warning('请填写策略名称、命名空间并至少选择一个工作负载')
    return
  }
  if (activeTab.value === 'hpa' && !form.cpuEnabled && !form.memoryEnabled) {
    ElMessage.warning('至少启用 CPU 或内存一个伸缩指标')
    return
  }
  saving.value = true
  try {
		const payload = {
			...(editingPolicy.value ? { id: editingPolicy.value.id } : {}),
      name: form.name.trim(),
      policyType: activeTab.value,
      clusterId: selectedClusterId.value,
      namespace: form.namespace,
      targets: targetPayload(),
      minReplicas: Number(form.minReplicas),
      maxReplicas: Number(form.maxReplicas),
      cpuEnabled: form.cpuEnabled,
      cpuUtilization: Number(form.cpuUtilization),
      memoryEnabled: form.memoryEnabled,
      memoryUtilization: Number(form.memoryUtilization),
      cronExpr: form.cronExpr.trim(),
      scheduledReplicas: Number(form.scheduledReplicas)
    }
		if (editingPolicy.value) {
			await updateK8sScalingPolicy(payload)
			ElMessage.success('策略已更新')
		} else {
			await createK8sScalingPolicy(payload)
			ElMessage.success('策略已保存，当前为未启用状态')
		}
    dialogVisible.value = false
    editingPolicy.value = null
    await loadPolicies()
  } catch (error) {
    ElMessage.error(error.message || '策略保存失败')
  } finally {
    saving.value = false
  }
}

function handleSelectionChange(rows) {
	selectedPolicies.value = rows
}

async function batchEnable() {
	const rows = inactiveSelectedPolicies.value
	if (!rows.length) return
	try {
		await ElMessageBox.confirm(`启用选中的 ${rows.length} 条策略？启用后会立即作用于对应工作负载。`, '批量启用', { type: 'warning' })
		await batchUpdateK8sScalingPolicyStatus({ ids: rows.map((row) => row.id), status: 1 })
		ElMessage.success(`已启用 ${rows.length} 条策略`)
		selectedPolicies.value = []
		await loadPolicies()
	} catch (error) {
		if (error !== 'cancel' && error !== 'close') ElMessage.error(error.message || '批量启用失败')
	}
}

async function batchRemove() {
	const rows = selectedPolicies.value
	if (!rows.length) return
	try {
		await ElMessageBox.confirm(`删除选中的 ${rows.length} 条策略？已启用的策略会先停用并移除对应资源。`, '批量删除', { type: 'warning' })
		await batchDeleteK8sScalingPolicies(rows.map((row) => row.id))
		ElMessage.success(`已删除 ${rows.length} 条策略`)
		selectedPolicies.value = []
		await loadPolicies()
	} catch (error) {
		if (error !== 'cancel' && error !== 'close') ElMessage.error(error.message || '批量删除失败')
	}
}

async function togglePolicy(row, value) {
  const nextStatus = value ? 1 : 2
  try {
    await updateK8sScalingPolicyStatus({ id: row.id, status: nextStatus })
    row.status = nextStatus
    ElMessage.success(value ? '策略已启用' : '策略已停用')
    await loadPolicies()
  } catch (error) {
    ElMessage.error(error.message || '策略状态更新失败')
  }
}

async function removePolicy(row) {
  try {
    await ElMessageBox.confirm(`删除策略“${row.name}”？${row.status === 1 ? '删除会先停用对应的伸缩资源。' : ''}`, '确认删除', { type: 'warning' })
    await deleteK8sScalingPolicy(row.id)
    ElMessage.success('策略已删除')
    await loadPolicies()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error.message || '策略删除失败')
  }
}

function policyStatusText(status) {
  return Number(status) === 1 ? '已启用' : '未启用'
}

function policyStatusType(status) {
  return Number(status) === 1 ? 'success' : 'info'
}

function targetLabel(target) {
  return `${target.workloadName} · ${target.workloadType}`
}

function metricsText(row) {
  const result = []
  if (row.cpuEnabled) result.push(`CPU ${row.cpuUtilization}%`)
  if (row.memoryEnabled) result.push(`内存 ${row.memoryUtilization}%`)
  return result.join(' / ') || '-'
}

watch(() => form.namespace, () => {
	if (!hydratingForm.value) form.targets = []
})
watch(activeTab, () => {
	selectedPolicies.value = []
	void loadPolicies()
})
watch(selectedClusterId, async () => {
  selectedPolicies.value = []
  workloads.value = []
  await Promise.all([loadClusterData(), loadPolicies()])
})

onMounted(load)
</script>

<template>
  <div class="scaling-page" v-loading="loading">
    <header class="scaling-page-head">
      <div class="scaling-title-block">
        <span class="scaling-kicker">WORKLOAD CAPACITY</span>
        <h2>工作负载伸缩策略</h2>
        <p>统一管理自动伸缩与定时副本调整。策略保存后不会立即改变集群，需在列表中手动启用。</p>
      </div>
      <div class="cluster-context">
        <span class="cluster-context-label">当前集群</span>
        <el-select v-model="selectedClusterId" filterable class="cluster-picker" placeholder="选择集群">
          <el-option v-for="item in clusterOptions" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
        <span v-if="selectedCluster" class="cluster-api">{{ selectedCluster.apiServer }}</span>
      </div>
    </header>

    <section class="policy-workspace">
      <div class="capacity-rail" :class="activeTab" aria-hidden="true"><i /><b /></div>
      <el-tabs v-model="activeTab" class="policy-tabs">
        <el-tab-pane name="hpa">
          <template #label><span class="tab-label"><el-icon><DataAnalysis /></el-icon>HPA 水平伸缩</span></template>
        </el-tab-pane>
        <el-tab-pane name="scheduled">
          <template #label><span class="tab-label"><el-icon><Clock /></el-icon>定时伸缩</span></template>
        </el-tab-pane>
      </el-tabs>

      <div class="policy-toolbar">
        <div class="policy-toolbar-copy">
          <h3>{{ currentPolicyTitle }}</h3>
          <p v-if="activeTab === 'hpa'">根据 CPU 或内存目标使用率自动调整副本数。</p>
          <p v-else>按 Cron 时间计划，把选中的工作负载调整为指定副本数。</p>
        </div>
        <div class="toolbar-actions">
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button :disabled="!inactiveSelectedPolicies.length" @click="batchEnable">批量启用<span v-if="inactiveSelectedPolicies.length">（{{ inactiveSelectedPolicies.length }}）</span></el-button>
          <el-button type="danger" plain :disabled="!selectedPolicies.length" @click="batchRemove">批量删除<span v-if="selectedPolicies.length">（{{ selectedPolicies.length }}）</span></el-button>
          <el-button type="primary" :icon="Plus" :disabled="!selectedClusterId" @click="openCreate">新增{{ activeTab === 'hpa' ? ' HPA' : '定时伸缩' }}</el-button>
        </div>
      </div>

      <div class="policy-status-strip">
        <span class="policy-count"><b>{{ visiblePolicies.length }}</b> 条策略</span>
        <span class="policy-status-note">保存后默认为未启用；启用时才会{{ activeTab === 'hpa' ? '创建对应 HPA' : '开始执行计划' }}。</span>
      </div>

      <el-table v-if="visiblePolicies.length" :data="visiblePolicies" class="policy-table" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column label="策略名称" min-width="200"><template #default="{ row }"><div class="policy-name"><span class="policy-mark" :class="row.policyType" /><strong>{{ row.name }}</strong><small>命名空间：{{ row.namespace }}</small></div></template></el-table-column>
        <el-table-column label="目标工作负载" min-width="280"><template #default="{ row }"><div class="target-tags"><el-tag v-for="target in row.targets" :key="`${target.workloadType}/${target.workloadName}`" size="small" effect="plain">{{ targetLabel(target) }}</el-tag></div></template></el-table-column>
        <template v-if="activeTab === 'hpa'">
          <el-table-column label="副本范围" width="128"><template #default="{ row }"><span class="replica-range"><b>{{ row.minReplicas }}</b><i>—</i><b>{{ row.maxReplicas }}</b></span></template></el-table-column>
          <el-table-column label="触发指标" min-width="180"><template #default="{ row }"><span class="metric-value">{{ metricsText(row) }}</span></template></el-table-column>
        </template>
        <template v-else>
          <el-table-column label="执行计划" min-width="210"><template #default="{ row }"><code>{{ row.cronExpr }}</code><small class="schedule-next">下次执行：{{ row.nextRunAt || '启用后计算' }}</small></template></el-table-column>
          <el-table-column label="目标副本" width="112"><template #default="{ row }"><span class="replica-single">{{ row.scheduledReplicas }}</span></template></el-table-column>
        </template>
        <el-table-column label="状态" width="118"><template #default="{ row }"><el-tag :type="policyStatusType(row.status)" effect="plain" class="policy-state">{{ policyStatusText(row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" width="228" fixed="right"><template #default="{ row }"><div class="policy-actions"><el-switch :model-value="row.status === 1" inline-prompt active-text="启用" inactive-text="停用" @change="(value) => togglePolicy(row, value)" /><el-button link type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button><el-button link type="danger" :icon="Delete" @click="removePolicy(row)">删除</el-button></div></template></el-table-column>
      </el-table>
      <el-empty v-else class="policy-empty" description="当前没有伸缩策略">
        <template #description><p>先创建一条{{ activeTab === 'hpa' ? ' HPA 水平伸缩' : '定时伸缩' }}策略，保存后可按需启用。</p></template>
        <el-button type="primary" plain :icon="Plus" :disabled="!selectedClusterId" @click="openCreate">新增策略</el-button>
      </el-empty>
    </section>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" :width="activeTab === 'scheduled' ? '1120px' : '780px'" class="scaling-dialog" destroy-on-close @closed="editingPolicy = null">
      <el-form label-width="142px" class="scaling-form">
        <section class="form-section">
          <div class="form-section-head"><h3>基础配置</h3></div>
          <el-form-item label="策略名称" required><div class="form-control"><el-input v-model="form.name" placeholder="请输入策略名称" /><small class="form-hint">最长 128 个字符，建议使用能识别用途的名称。</small></div></el-form-item>
          <el-form-item label="命名空间" required><el-select v-model="form.namespace" filterable class="form-control"><el-option v-for="item in namespaces" :key="item" :label="item" :value="item" /></el-select></el-form-item>
          <el-form-item label="工作负载" required><div class="form-control form-control--wide"><el-select v-model="form.targets" multiple filterable collapse-tags collapse-tags-tooltip class="full-width" placeholder="请选择工作负载"><el-option v-for="item in workloadOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select><small class="form-hint">支持多选 Deployment 或 StatefulSet；每个目标都会创建独立的伸缩资源。</small></div></el-form-item>
        </section>
        <template v-if="activeTab === 'hpa'">
          <section class="form-section">
            <div class="form-section-head"><h3>扩缩容配置</h3></div>
            <el-form-item label="最小副本数"><div class="form-control form-control--number"><el-input-number v-model="form.minReplicas" :min="1" :max="1000" controls-position="right" /></div></el-form-item>
            <el-form-item label="最大副本数"><div class="form-control form-control--number"><el-input-number v-model="form.maxReplicas" :min="1" :max="1000" controls-position="right" /><small class="form-hint">自动扩缩容的副本数上限，不能小于最小副本数。</small></div></el-form-item>
          </section>
          <section class="form-section">
            <div class="form-section-head"><h3>触发策略</h3></div>
            <el-form-item label="CPU 目标使用率"><div class="metric-line"><el-checkbox v-model="form.cpuEnabled">启用</el-checkbox><el-input-number v-model="form.cpuUtilization" :min="1" :max="100" :disabled="!form.cpuEnabled" controls-position="right" /><span>%</span><small>Pod 平均 CPU 使用率（使用量 / requests）超过该阈值时扩容。</small></div></el-form-item>
            <el-form-item label="内存目标使用率"><div class="metric-line"><el-checkbox v-model="form.memoryEnabled">启用</el-checkbox><el-input-number v-model="form.memoryUtilization" :min="1" :max="100" :disabled="!form.memoryEnabled" controls-position="right" /><span>%</span><small>Pod 平均内存使用率（使用量 / requests）超过该阈值时扩容。</small></div></el-form-item>
            <p class="form-hint form-note">集群需要 metrics-server，且目标容器必须配置对应的 CPU / Memory Requests。</p>
          </section>
        </template>
        <template v-else>
          <section class="form-section">
            <div class="form-section-head"><h3>定时规则</h3></div>
            <el-form-item label="Cron 表达式" required>
              <div class="cron-builder">
                <div class="cron-presets"><strong>常用计划</strong><el-button v-for="preset in cronPresets" :key="preset.label" size="small" plain :class="{ 'is-active': activeCronPreset === preset.label }" @click="applyCronPreset(preset)">{{ preset.label }}</el-button></div>
                <div class="cron-fields"><label v-for="item in cronFieldOptions" :key="item.key"><b>{{ item.label }}</b><el-input v-model="cronFields[item.key]" @input="syncCronExpression" /><small>{{ item.range }}</small></label></div>
                <div class="cron-generated"><span>生成表达式</span><code>{{ generatedCronExpression }}</code><el-tag size="small" effect="plain">{{ cronScheduleText }}</el-tag></div>
                <p>* 表示任意值，<code>*/5</code> 表示每 5 个单位执行一次，多个值可用逗号分隔。</p>
              </div>
            </el-form-item>
            <el-form-item label="目标副本数" required><div class="form-control form-control--number"><el-input-number v-model="form.scheduledReplicas" :min="0" :max="1000" controls-position="right" /></div></el-form-item>
          </section>
        </template>
      </el-form>
      <template #footer><div class="dialog-footer"><span>{{ dialogFooterText }}</span><div><el-button @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="submit">{{ dialogSubmitText }}</el-button></div></div></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.scaling-page { min-height: calc(100vh - 150px); padding: 24px 30px 48px; color: #243754; }
.scaling-page-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; padding: 4px 2px 22px; }
.scaling-title-block { min-width: 0; }
.scaling-kicker { display: block; color: #6781ad; font-size: 11px; font-weight: 760; letter-spacing: .14em; }
.scaling-title-block h2 { margin: 7px 0 6px; color: #172b4d; font-size: 27px; line-height: 1.22; letter-spacing: -.025em; }
.scaling-title-block p { margin: 0; color: #72829a; font-size: 13px; line-height: 1.6; }
.cluster-context { display: grid; grid-template-columns: auto 250px; align-items: center; gap: 8px 10px; min-width: 380px; }
.cluster-context-label { color: #74839a; font-size: 12px; text-align: right; }
.cluster-picker { width: 250px; }
.cluster-api { grid-column: 2; overflow: hidden; color: #8492a6; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.policy-workspace { overflow: hidden; border: 1px solid #dce6f4; border-radius: 12px; background: #fff; box-shadow: 0 6px 18px rgba(32, 62, 104, .045); }
.capacity-rail { display: flex; height: 3px; background: #dce8fb; }
.capacity-rail i { display: block; width: 58%; background: #2f70e8; transition: width .2s ease, background .2s ease; }
.capacity-rail b { display: block; flex: 1; background: #88a8e2; }
.capacity-rail.scheduled i { width: 36%; background: #d58a32; }.capacity-rail.scheduled b { background: #f1c37d; }
.policy-tabs { padding: 0 22px; border-bottom: 1px solid #e7edf6; }
.policy-tabs :deep(.el-tabs__header) { margin: 0; }.policy-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }
.policy-tabs :deep(.el-tabs__item) { height: 52px; color: #718198; font-size: 14px; font-weight: 650; }
.policy-tabs :deep(.el-tabs__item.is-active) { color: #2863cb; }.policy-tabs :deep(.el-tabs__active-bar) { height: 2px; background: #2f70e8; }
.tab-label { display: inline-flex; align-items: center; gap: 7px; }.tab-label .el-icon { font-size: 16px; }
.policy-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 20px 22px 14px; }
.policy-toolbar-copy h3 { margin: 0; color: #1d3659; font-size: 16px; line-height: 1.4; }.policy-toolbar-copy p { margin: 4px 0 0; color: #8593a6; font-size: 12px; }
.toolbar-actions { display: flex; flex: 0 0 auto; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.policy-status-strip { display: flex; align-items: center; gap: 12px; margin: 0 22px 14px; padding: 9px 12px; border-left: 3px solid #79a0e8; background: #f7faff; color: #7b8aa0; font-size: 12px; }
.policy-count { color: #4d6280; white-space: nowrap; }.policy-count b { margin-right: 3px; color: #2863cb; font-size: 15px; }.policy-status-note { color: #8996a8; }
.policy-table { margin: 0 22px 22px; width: calc(100% - 44px); border: 1px solid #e1e8f2; border-radius: 8px; overflow: hidden; }
.policy-table :deep(.el-table__header-wrapper th) { height: 42px; background: #f7f9fc; color: #708097; font-size: 12px; font-weight: 650; }.policy-table :deep(.el-table__cell) { padding: 12px 0; }.policy-table :deep(.el-table__row:hover > td) { background: #fafcff !important; }
.policy-name { position: relative; display: flex; flex-direction: column; gap: 4px; min-height: 32px; padding-left: 12px; }.policy-name strong { color: #315f9f; font-size: 13px; font-weight: 700; }.policy-name small { color: #91a0b2; font-size: 11px; }.policy-mark { position: absolute; top: 2px; left: 0; width: 3px; height: 32px; border-radius: 3px; background: #3672db; }.policy-mark.scheduled { background: #d78b33; }
.target-tags { display: flex; flex-wrap: wrap; gap: 5px; }.target-tags :deep(.el-tag) { border-color: #dbe5f3; background: #f8fbff; color: #526b90; font-size: 11px; }
.replica-range { display: inline-flex; align-items: center; gap: 7px; color: #547096; font-variant-numeric: tabular-nums; }.replica-range b { min-width: 18px; color: #2f5eaa; text-align: center; }.replica-range i { color: #a6b3c3; font-style: normal; }.replica-single { display: inline-flex; min-width: 28px; color: #a26124; font-weight: 700; font-variant-numeric: tabular-nums; }.metric-value { color: #566c8c; font-size: 12px; }
.schedule-next { display: block; margin-top: 5px; color: #99a5b6; font-size: 11px; }.policy-table code, .scaling-form code { color: #596f95; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }.policy-state { min-width: 58px; justify-content: center; }.policy-actions { display: flex; align-items: center; gap: 8px; }.policy-actions :deep(.el-button) { padding: 4px 0; }
.policy-empty { padding: 42px 0 52px; }.policy-empty :deep(.el-empty__description p) { color: #8a98ab; font-size: 13px; }
.scaling-form { padding: 0 2px; }.form-section + .form-section { margin-top: 24px; }.form-section-head { display: flex; align-items: center; gap: 14px; margin: 0 0 19px; }.form-section-head::after { height: 1px; flex: 1; background: #dce4ef; content: ''; }.form-section-head h3 { margin: 0; color: #273f62; font-size: 15px; font-weight: 700; }.full-width { width: 100%; }.scaling-form :deep(.el-form-item) { margin-bottom: 18px; }.scaling-form :deep(.el-form-item__label) { height: 40px; padding-right: 16px; color: #53657e; font-size: 13px; line-height: 40px; }.scaling-form :deep(.el-input-number) { width: 100%; }.form-control { width: 375px; max-width: 100%; }.form-control--wide { width: 520px; }.form-control--number { width: 200px; }.form-hint { display: block; margin-top: 7px; color: #8d9aaa; font-size: 12px; line-height: 1.5; }.metric-line { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; min-height: 40px; }.metric-line :deep(.el-checkbox) { width: 62px; }.metric-line :deep(.el-input-number) { width: 200px; }.metric-line > span { color: #71839d; font-size: 13px; }.metric-line small { flex-basis: 100%; margin-top: 1px; color: #8d9aaa; font-size: 12px; line-height: 1.5; }.form-note { margin: 0 0 0 142px; padding-left: 10px; border-left: 2px solid #b8cdf4; }.dialog-footer { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; }.dialog-footer > span { color: #909cac; font-size: 12px; }.dialog-footer > div { display: flex; gap: 8px; }
.cron-builder { width: 100%; max-width: 900px; box-sizing: border-box; padding: 22px 20px 17px; border: 1px solid #cdddf6; border-radius: 8px; background: #f6f9ff; }.cron-presets { display: flex; flex-wrap: nowrap; align-items: center; gap: 12px; }.cron-presets strong { flex: 0 0 auto; margin-right: 4px; color: #345477; font-size: 14px; }.cron-presets :deep(.el-button) { flex: 0 0 auto; height: 30px; min-width: auto; margin-left: 0; padding: 0 14px; border-color: #d8e1ee; color: #4c617c; background: #fff; }.cron-presets :deep(.el-button.is-active) { border-color: #9cc4ff; color: #2878e8; background: #edf5ff; }.cron-fields { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 14px; margin-top: 27px; }.cron-fields label { display: block; min-width: 0; }.cron-fields b { display: block; margin: 0 0 10px; color: #2e4767; font-size: 14px; }.cron-fields :deep(.el-input__wrapper) { min-height: 40px; padding: 1px 13px; background: #fff; }.cron-fields small { display: block; margin-top: 9px; color: #849abc; font-size: 12px; text-align: center; }.cron-generated { display: flex; align-items: center; gap: 12px; min-height: 48px; margin-top: 27px; padding: 0 14px; border: 1px solid #dbe6f5; border-radius: 7px; background: #fff; }.cron-generated span { color: #6680a4; font-size: 13px; }.cron-generated code { flex: 1; color: #245db7; font-size: 14px; font-weight: 700; letter-spacing: 1px; }.cron-generated :deep(.el-tag) { border-color: #d7e2f3; color: #627693; background: #f9fbfe; }.cron-builder > p { margin: 14px 0 0; color: #7187aa; font-size: 12px; line-height: 1.5; }
@media (max-width: 900px) { .scaling-page { padding: 18px 14px 38px; }.scaling-page-head { align-items: flex-start; flex-direction: column; }.cluster-context { width: 100%; min-width: 0; grid-template-columns: 72px minmax(0, 1fr); }.cluster-picker { width: 100%; }.policy-toolbar { align-items: flex-start; flex-direction: column; }.toolbar-actions { width: 100%; justify-content: flex-start; }.policy-status-strip { align-items: flex-start; flex-direction: column; gap: 3px; }.policy-table { margin-right: 14px; margin-left: 14px; width: calc(100% - 28px); }.policy-tabs { padding: 0 14px; }.policy-toolbar { padding: 18px 14px 12px; }.policy-status-strip { margin: 0 14px 12px; }.form-control, .form-control--wide { width: 100%; }.scaling-form :deep(.el-form-item__label) { height: auto; line-height: 1.5; }.form-note { margin-left: 0; }.dialog-footer { align-items: flex-end; flex-direction: column; }.dialog-footer > span { width: 100%; }.metric-line :deep(.el-input-number) { width: min(200px, calc(100% - 90px)); }.cron-fields { grid-template-columns: repeat(3, minmax(0, 1fr)); }.cron-generated { align-items: flex-start; flex-wrap: wrap; }.cron-generated code { flex-basis: calc(100% - 86px); } }
</style>
