<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Refresh } from '@element-plus/icons-vue'
import { deployInfo, deployLogs, deployPrecheck, deployStart, deployStop, deployRegister } from '../../../api/k8s-deploy'
import { activeDeployment, deploymentEditable, formatDeployTime, runStates, statusText } from '../../../utils/k8s-deploy'
import DeploySummary from './DeploySummary.vue'
import DeployMark from './DeployMark.vue'
import './deploy.css'

const route = useRoute(), router = useRouter(), plan = ref(null), runs = ref([]), steps = ref([]), logs = ref([])
const loading = ref(false), busy = ref(false), selectedRun = ref(null), selectedHost = ref(''), tab = ref('progress')
const now = ref(Date.now())
let timer, refreshing = false, logVersion = 0
const selected = computed(() => runs.value.find(run => run.id === selectedRun.value))
const running = computed(() => activeDeployment(plan.value?.status))
const precheckValid = computed(() => plan.value?.status === 'ready' && runs.value.some(run => run.kind === 'precheck' && run.status === 'success' && run.revision === plan.value.revision && now.value - new Date(run.finishedAt).getTime() < 15 * 60000))
const visibleLogs = computed(() => logs.value.filter(log => !selectedHost.value || log.hostId === selectedHost.value).map(log => log.content).join(''))
const visibleLogLines = computed(() => visibleLogs.value ? visibleLogs.value.replace(/\r/g, '').trimEnd().split('\n') : [])
const completedSteps = computed(() => steps.value.filter(step => step.status === 'success').length)

async function loadLogs(reset = false) {
  if (!selectedRun.value) { logs.value = []; steps.value = []; return }
  const version = ++logVersion, id = selectedRun.value
  if (reset) { logs.value = []; steps.value = [] }
  const after = logs.value.at(-1)?.id || 0
  const data = await deployLogs(Number(route.params.id), id, after)
  if (version !== logVersion || id !== selectedRun.value) return
  const known = new Set(logs.value.map(log => log.id))
  logs.value = [...logs.value, ...(data.logs || []).filter(log => !known.has(log.id))].slice(-2000)
  steps.value = data.steps || []
  const i = runs.value.findIndex(run => run.id === id); if (i >= 0) runs.value[i] = data.run
}
async function refresh(quiet = false) {
  if (refreshing) return
  refreshing = true; if (!quiet) loading.value = true
  try {
    const planId = Number(route.params.id)
    const data = await deployInfo(planId)
    if (planId !== Number(route.params.id)) return
    const previousLatest = runs.value[0]?.id
    plan.value = data.plan; runs.value = data.runs || []
    if (!selectedRun.value || (selectedRun.value === previousLatest && previousLatest !== runs.value[0]?.id)) { selectedRun.value = runs.value[0]?.id || null; logs.value = [] }
    if (selectedRun.value) await loadLogs()
  } finally { loading.value = false; refreshing = false }
}
async function action(operation) {
  busy.value = true
  try { await operation(); await refresh(true) } finally { busy.value = false }
}
async function start() {
  try { await ElMessageBox.confirm(`将在「${plan.value.name}」的 ${plan.value.nodes.length} 台主机上执行 Kubernetes 安装，配置将按确认页应用。`, '确认部署', { type: 'warning', confirmButtonText: '开始安装', cancelButtonText: '取消' }) } catch { return }
  await action(() => deployStart(plan.value.id, plan.value.revision))
}
async function stop() {
  try { await ElMessageBox.confirm('任务将在当前远程步骤结束后停止，已完成的系统变更会保留。', '停止部署任务', { confirmButtonText: '请求停止', cancelButtonText: '取消' }) } catch { return }
  await action(() => deployStop(plan.value.id))
}
async function register() { await action(async () => { await deployRegister(plan.value.id); ElMessage.success('集群已接入') }) }
watch(() => route.params.id, async () => { selectedRun.value = null; logs.value = []; steps.value = []; runs.value = []; plan.value = null; logVersion++; refreshing = false; await refresh() })
onMounted(async () => { await refresh(); timer = setInterval(() => { now.value = Date.now(); if (running.value) refresh(true).catch(() => {}) }, 2500) })
onBeforeUnmount(() => { clearInterval(timer); logVersion++ })
</script>

<template>
  <div v-loading="loading" class="k8s-deploy">
    <el-button class="deploy-back" link :icon="ArrowLeft" @click="router.push('/containers/k8s-deployments')">返回部署列表</el-button>
    <template v-if="plan">
      <header class="deploy-heading"><div class="deploy-heading-main"><div class="deploy-heading-symbol"><DeployMark /></div><div><div class="deploy-eyebrow">DEPLOYMENT / #{{ plan.id }}</div><h1>{{ plan.name }}</h1><p>{{ plan.description || 'Kubernetes 集群部署计划' }}</p><div class="deploy-status-line"><span class="deploy-status" :class="plan.status">{{ statusText(plan.status) }}</span><span class="deploy-help" style="margin:0">由 {{ plan.createdBy }} 创建 · {{ formatDeployTime(plan.createTime) }}</span></div></div></div><div class="deploy-actions"><el-button :icon="Refresh" @click="refresh()">刷新</el-button><el-button v-if="deploymentEditable(plan.status)" @click="router.push(`/containers/k8s-deployments/${plan.id}/edit`)">配置</el-button><el-button v-if="running" type="warning" :loading="busy" :disabled="plan.status === 'stopping'" @click="stop">请求停止</el-button><el-button v-if="plan.status === 'ready'" type="primary" :loading="busy" :disabled="!precheckValid" @click="start">开始部署</el-button><el-button v-if="plan.clusterId" type="primary" @click="router.push(`/containers/k8s/clusters/${plan.clusterId}/detail`)">进入集群</el-button></div></header>
      <dl class="deploy-detail-meta"><div><dt>KUBERNETES</dt><dd class="deploy-mono">v{{ plan.config.version }}</dd></div><div><dt>节点构成</dt><dd>{{ plan.nodes.filter(n => n.role === 'master').length }} 控制平面<span style="color:#cad3e1;margin:0 8px">/</span>{{ plan.nodes.filter(n => n.role === 'worker').length }} 工作节点</dd></div><div><dt>网络插件 / 运行时</dt><dd>{{ plan.config.networkPlugin }} <span style="color:#cad3e1;margin:0 5px">/</span> containerd</dd></div><div><dt>部署架构</dt><dd>{{ plan.config.ha ? '高可用集群' : '单控制平面' }}<span class="deploy-help" style="margin-left:8px">{{ plan.env || '未分配环境' }}</span></dd></div></dl>
      <el-alert v-if="plan.status === 'needs_review'" title="安装已产生系统变更或执行器中断，请核对节点现场。系统保留占用记录，不会自动重装。" type="warning" show-icon :closable="false" />
      <el-alert v-if="plan.status === 'ready' && !precheckValid" title="预检有效期已过，请重新预检后部署。" type="info" show-icon :closable="false" />
      <el-alert v-if="plan.registrationStatus === 'failed'" title="集群安装已完成，接入平台失败，可单独重试接入。" type="warning" show-icon :closable="false"><template #default><p>{{ plan.registrationError }}</p><el-button :loading="busy" @click="register">重新接入集群管理</el-button></template></el-alert>
      <section class="deploy-card deploy-detail-card">
        <el-tabs v-model="tab">
          <el-tab-pane label="执行进度与日志" name="progress">
            <div class="deploy-toolbar" style="margin:23px 0"><div><h2>部署执行记录</h2><p class="deploy-help">从节点预检到集群就绪，查看每个阶段的执行结果。</p></div><el-select v-model="selectedRun" placeholder="暂无运行记录" style="width:330px;max-width:100%" @change="loadLogs(true)"><el-option v-for="run in runs" :key="run.id" :value="run.id" :label="`#${run.id} ${run.kind === 'precheck' ? '预检' : '部署'} · ${runStates[run.status] || run.status} · ${formatDeployTime(run.startedAt)}`" /></el-select></div>
            <el-empty v-if="!runs.length" description="尚未执行预检"><el-button type="primary" @click="router.push(`/containers/k8s-deployments/${plan.id}/edit`)">完善配置并预检</el-button></el-empty>
            <template v-else>
              <el-alert v-if="selected?.error" :title="selected.error" type="error" show-icon :closable="false" style="margin-bottom:16px" />
              <div class="deploy-detail-grid">
                <div class="deploy-stage-panel"><div class="deploy-stage-header"><strong>阶段记录</strong><span>{{ completedSteps }} / {{ steps.length }}</span></div><div v-if="selected?.status === 'success'" class="deploy-progress-track"><span style="width:100%"></span></div><div v-for="(record, index) in steps" :key="record.id" class="deploy-step-record" :class="record.status"><span class="deploy-step-dot">{{ record.status === 'success' ? '✓' : index + 1 }}</span><div class="deploy-step-copy"><strong>{{ record.title }}</strong><p>{{ record.hostName }} · {{ runStates[record.status] }}</p><p v-if="['failed', 'warning'].includes(record.status)">{{ record.summary }}</p></div></div><p v-if="!steps.length" class="deploy-help">执行器正在准备任务…</p></div>
                <div><div class="deploy-toolbar"><h2>执行日志</h2><div class="deploy-inline"><el-select v-model="selectedHost" clearable placeholder="全部节点" style="width:160px"><el-option v-for="node in selected?.nodes || plan.nodes" :key="node.hostId" :label="node.hostName" :value="node.hostId" /></el-select><el-button :icon="Refresh" @click="loadLogs()">加载更多</el-button></div></div><div class="deploy-terminal"><div class="deploy-terminal-bar"><span class="deploy-inline"><span class="deploy-terminal-dots" aria-hidden="true"><i></i><i></i><i></i></span>部署输出 <span style="color:#657c9e;margin-left:8px">#{{ selectedRun }}</span></span><span class="deploy-terminal-state">{{ selected?.status === 'running' ? '实时同步' : '历史输出' }}</span></div><div class="deploy-log" role="log" aria-label="部署执行日志"><div v-for="(line, index) in visibleLogLines" :key="index" class="deploy-log-line"><span aria-hidden="true">{{ String(index + 1).padStart(2, '0') }}</span><span>{{ line || ' ' }}</span></div><span v-if="!visibleLogLines.length" style="color:#647d9f">等待日志输出…</span></div><div class="deploy-terminal-footer"><span>{{ visibleLogLines.length }} 行输出</span><span>凭据与密钥已隐藏</span></div></div><p class="deploy-help">按节点筛选日志，或切换运行记录查看历史执行过程。</p></div>
              </div>
              <div v-if="['precheck_failed', 'stopped', 'failed'].includes(plan.status)" class="deploy-actions" style="margin-top:20px"><el-button :loading="busy" @click="action(() => deployPrecheck(plan.id, plan.revision))">重新预检</el-button></div>
            </template>
          </el-tab-pane>
          <el-tab-pane label="计划配置" name="config"><div style="padding-top:20px"><DeploySummary :plan="plan" /></div></el-tab-pane>
          <el-tab-pane label="本次运行快照" name="snapshot"><div v-if="selected" style="padding-top:20px"><p class="deploy-help">运行 #{{ selected.id }} · 配置版本 {{ selected.revision }} · 操作人 {{ selected.operator }}</p><DeploySummary :plan="{ ...plan, config: selected.config, nodes: selected.nodes }" /></div><el-empty v-else description="暂无运行记录" /></el-tab-pane>
        </el-tabs>
      </section>
    </template>
  </div>
</template>

