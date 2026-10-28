<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Plus, Box, Share, Connection, Aim, InfoFilled, Monitor } from '@element-plus/icons-vue'
import { deployCapabilities, deployInfo, deploySave, deployPrecheck, deployStart } from '../../../api/k8s-deploy'
import { useEnvironmentOptions } from '../../../composables/useEnvironmentOptions'
import { activeDeployment, deploymentEditable, deploymentSummary, emptyDeployment, nodeFromAsset, runStates } from '../../../utils/k8s-deploy'
import DeployHostPicker from './DeployHostPicker.vue'
import DeploySummary from './DeploySummary.vue'
import DeployMark from './DeployMark.vue'
import DeployTopology from './DeployTopology.vue'
import './deploy.css'

const route = useRoute(), router = useRouter()
const form = reactive(emptyDeployment()), step = ref(0), loading = ref(false), saving = ref(false), pickerOpen = ref(false)
const capabilities = ref({ versions: [], plugins: [] }), live = ref(null), report = ref([]), lastRun = ref(null)
const customDirectory = ref(false), customSource = ref(false), accepted = ref(false)
const now = ref(Date.now())
const { environmentOptions } = useEnvironmentOptions()
const titles = ['集群信息', '高级配置', '新增节点', '信息确认']
const stepNotes = ['版本、运行时与网络', '可用性与集群组件', '关联 CMDB 服务器', '预检并开始部署']
const stepDescriptions = ['定义集群的基础信息，以及节点间的网络通信方式。', '按业务需要配置高可用、访问入口与可选组件。', '选择已有服务器，系统、凭据和网关将自动关联。', '核对部署配置，通过节点预检后即可开始安装。']
const pluginNotes = { calico: '网络策略 · 灵活组网', flannel: '轻量网络 · 简单易用', cilium: 'eBPF 网络 · 待适配' }
const config = computed(() => form.config), counts = computed(() => deploymentSummary(form))
const running = computed(() => activeDeployment(live.value?.status))
const baseSignature = ref('')
function payload() { return { id: form.id, revision: form.revision, name: form.name, env: form.env, description: form.description, config: { ...form.config }, nodes: form.nodes.map(n => ({ ...n })) } }
const dirty = computed(() => JSON.stringify(payload()) !== baseSignature.value)
const canDeploy = computed(() => !dirty.value && live.value?.status === 'ready' && lastRun.value?.kind === 'precheck' && lastRun.value?.status === 'success' && now.value - new Date(lastRun.value.finishedAt).getTime() < 15 * 60000)
const canEdit = computed(() => !live.value || deploymentEditable(live.value.status))
let timer, pollBusy = false

watch(() => config.value.ha, value => { if (!loading.value && !value) { config.value.builtinLB = false; config.value.apiPort = 6443 } })
watch(() => config.value.builtinLB, value => { if (!loading.value) config.value.apiPort = value ? 8443 : 6443 })
watch(() => config.value.autoHostname, value => { if (!loading.value) form.nodes.forEach(n => { n.name = value ? `k8s-${n.ip.replaceAll('.', '-')}` : n.hostName.toLowerCase() }) })
watch(customDirectory, value => { if (!value) config.value.runtimeDirectory = '' })
watch(customSource, value => { if (!value) config.value.customSource = '' })
watch(dirty, value => { if (value) accepted.value = false })
watch(step, async () => { await nextTick(); window.scrollTo({ top: 0, behavior: 'instant' }) })

function validate(index) {
  const c = config.value
  if (index === 0) {
    if (!form.name.trim() || [...form.name.trim()].length > 50) return '请输入不超过 50 个字符的部署名称'
    if (!/^1\.(31|32|33|34)\.\d+$/.test(c.version)) return '请输入 1.31.x–1.34.x 版本，不含 v 前缀'
    if (![c.podCIDR, c.serviceCIDR].every(v => /^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/.test(v))) return '请输入完整 Pod / Service CIDR，例如 172.30.0.0/16'
    if (!c.imageRepository.trim()) return '请填写 Kubernetes 镜像仓库'
    if (customDirectory.value && !c.runtimeDirectory.startsWith('/')) return '请填写运行时数据目录的绝对路径'
    if (customSource.value && !c.customSource.trim()) return '请填写自定义软件源配置，或关闭此选项'
  }
  if (index === 1) {
    if (c.ha && !c.apiServer.trim()) return '高可用部署需要 API Server 地址'
    if (c.builtinLB && c.apiPort === 6443) return '自建负载均衡入口不能占用 API 后端 6443 端口'
  }
  if (index === 2) {
    if (!form.nodes.length) return '请从 CMDB 至少关联一个节点'
    if (counts.value.masters !== (c.ha ? 3 : 1)) return c.ha ? '高可用需要 3 个控制平面节点' : '普通部署需要 1 个控制平面节点'
    if (!counts.value.workers && !c.allowControlPlaneWorkloads) return '没有工作节点，请在高级配置中允许控制平面运行普通业务'
    if (form.nodes.some(n => !n.ip || !n.name || !n.interface)) return '请补齐每个节点的部署 IP、节点名和网口'
  }
  return ''
}
function next() { const error = validate(step.value); if (error) return ElMessage.warning(error); step.value++ }
function validateAll() { for (let i = 0; i < 3; i++) { const error = validate(i); if (error) { step.value = i; ElMessage.warning(error); return false } } if (!form.env) { step.value = 0; ElMessage.warning('请选择所属环境，用于部署后接入集群管理'); return false } return true }
function addNodes(hosts) {
  if (!form.env && hosts[0]?.environment) form.env = hosts[0].environment
  hosts.forEach(host => { if (!form.nodes.some(n => n.hostId === host.id)) form.nodes.push(nodeFromAsset(host, config.value, form.nodes.length)) })
}
async function save(showToast = true) {
  if (!form.name.trim()) { ElMessage.warning('保存草稿前请填写部署名称'); step.value = 0; return null }
  saving.value = true
  try {
    const saved = await deploySave(payload())
    form.id = saved.id; form.revision = saved.revision; form.nodes = saved.nodes || []; live.value = saved
    baseSignature.value = JSON.stringify(payload())
    if (route.path.endsWith('/create')) await router.replace(`/containers/k8s-deployments/${saved.id}/edit`)
    if (showToast) ElMessage.success('部署草稿已保存')
    return saved
  } finally { saving.value = false }
}
async function refreshReport() {
  if (!form.id || pollBusy) return
  pollBusy = true
  try {
    const planId = form.id
    const data = await deployInfo(planId)
    if (planId !== form.id) return
    live.value = data.plan; report.value = data.steps || []; lastRun.value = data.runs?.[0] || null
    if (!dirty.value && data.plan.status === 'ready') { form.nodes = data.plan.nodes || []; baseSignature.value = JSON.stringify(payload()) }
  } finally { pollBusy = false }
}
async function precheck() {
  if (!validateAll()) return
  saving.value = true
  try {
    if (dirty.value || !form.id) { const saved = await save(false); if (!saved) return }
    saving.value = true
    lastRun.value = await deployPrecheck(form.id, form.revision); live.value = { ...live.value, status: 'prechecking' }; report.value = []; await refreshReport()
  } finally { saving.value = false }
}
async function start() {
  if (!canDeploy.value || !accepted.value) return
  saving.value = true
  try { await deployStart(form.id, form.revision); await router.push(`/containers/k8s-deployments/${form.id}`) }
  finally { saving.value = false }
}
async function loadPlan() {
  loading.value = true
  try {
    Object.assign(form, emptyDeployment()); live.value = null; report.value = []; lastRun.value = null; step.value = 0; accepted.value = false
    customDirectory.value = false; customSource.value = false
    capabilities.value = await deployCapabilities()
    if (route.params.id) {
      const data = await deployInfo(Number(route.params.id))
      Object.assign(form, { ...emptyDeployment(), ...data.plan }); live.value = data.plan; report.value = data.steps || []; lastRun.value = data.runs?.[0] || null
      customDirectory.value = Boolean(form.config.runtimeDirectory); customSource.value = Boolean(form.config.customSource)
      if (data.plan.status === 'ready' || activeDeployment(data.plan.status)) step.value = 3
    }
    // Watchers must settle before establishing the saved snapshot.
    await Promise.resolve(); baseSignature.value = JSON.stringify(payload())
  } finally { loading.value = false }
}
watch(() => route.params.id, id => { if (Number(id || 0) !== form.id) loadPlan() })
onMounted(async () => { await loadPlan(); timer = setInterval(() => { now.value = Date.now(); if (running.value) refreshReport().catch(() => {}) }, 2500) })
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div v-loading="loading" class="k8s-deploy deploy-wizard">
    <el-button class="deploy-back" link :icon="ArrowLeft" @click="router.push('/containers/k8s-deployments')">返回部署列表</el-button>
    <header class="deploy-heading"><div class="deploy-heading-main"><div class="deploy-heading-symbol"><DeployMark /></div><div><div class="deploy-eyebrow">KUBERNETES / DEPLOYMENT</div><h1>{{ form.id ? '配置集群部署' : '创建 Kubernetes 集群' }}</h1><p>从服务器到集群，让每一步部署清晰可见。</p></div></div><span class="deploy-footer-state"><i></i>{{ form.id ? `计划 #${form.id}` : '新建部署计划' }}</span></header>
    <div class="deploy-workspace">
      <aside class="deploy-rail">
        <nav class="deploy-stepbar" aria-label="集群部署步骤"><span class="deploy-rail-label">部署流程</span><button v-for="(title, index) in titles" :key="title" type="button" class="deploy-step-button" :class="{ current: step === index, done: step > index }" :disabled="index > step" :aria-current="step === index ? 'step' : undefined" @click="step = index"><span class="deploy-step-number">{{ step > index ? '✓' : `0${index + 1}` }}</span><span><strong>{{ title }}</strong><small>{{ stepNotes[index] }}</small></span></button></nav>
        <DeployTopology :plan="form" />
        <div class="deploy-rail-note"><el-icon><InfoFilled /></el-icon>主机系统、访问凭据和网关自动从 CMDB 获取，无需重复配置。</div>
      </aside>
      <section class="deploy-card deploy-workspace-main">
      <div class="deploy-form-heading"><div><h2>{{ titles[step] }}</h2><p>{{ stepDescriptions[step] }}</p></div><span class="deploy-form-index">0{{ step + 1 }}</span></div>
      <el-alert v-if="!canEdit && !running" title="当前计划已执行或需要核对现场，请到部署详情查看状态。" type="warning" show-icon :closable="false" style="margin-bottom:20px" />
      <el-form v-if="step < 3" class="deploy-form" label-position="top" :disabled="running || !canEdit">
        <template v-if="step === 0">
          <h2 class="deploy-section-title">基础配置</h2>
          <div class="deploy-grid">
            <el-form-item label="部署名称" required><el-input v-model="form.name" maxlength="50" show-word-limit placeholder="请输入部署集群名称" /></el-form-item>
            <el-form-item label="Kubernetes 版本" required><div class="field"><el-select v-model="config.version" filterable allow-create default-first-option placeholder="选择或输入版本"><el-option v-for="version in capabilities.versions" :key="version" :label="version" :value="version" /></el-select><div class="deploy-help">支持 {{ capabilities.versionRange }}，手动输入格式如 1.34.9，不含 v。</div></div></el-form-item>
            <el-form-item label="容器运行时"><div class="deploy-runtime"><div class="deploy-runtime-brand"><el-icon><Box /></el-icon>containerd<span class="deploy-help" style="margin:0 0 0 auto">CRI</span></div><div class="deploy-inline" style="margin-top:8px"><el-checkbox v-model="customDirectory">自定义数据目录</el-checkbox></div><el-input v-if="customDirectory" v-model="config.runtimeDirectory" placeholder="例如 /data/containerd" /><div class="deploy-help">未勾选时使用系统默认目录。</div></div></el-form-item>
            <el-form-item label="所属环境"><div class="field"><el-select v-model="form.env" clearable placeholder="关联节点后自动带入，也可手动选择"><el-option v-for="env in environmentOptions" :key="env.code" :label="env.name" :value="env.code" /></el-select><div class="deploy-help">默认从首台 CMDB 主机带入；部署前必须明确所属环境。</div></div></el-form-item>
            <el-form-item label="描述" class="wide"><el-input v-model="form.description" type="textarea" :rows="2" maxlength="255" show-word-limit placeholder="可选，描述此集群的用途" /></el-form-item>
          </div>
          <h2 class="deploy-section-title">网络配置</h2>
          <el-form-item label="容器网络插件" required><div class="deploy-plugin-grid" role="radiogroup" aria-label="容器网络插件"><label v-for="plugin in capabilities.plugins" :key="plugin.value" class="deploy-plugin" :class="{ active: config.networkPlugin === plugin.value, disabled: plugin.disabled }"><input v-model="config.networkPlugin" type="radio" name="networkPlugin" :value="plugin.value" :disabled="plugin.disabled || running || !canEdit" :aria-label="plugin.label" class="deploy-plugin-native" /><span class="deploy-plugin-icon"><el-icon><component :is="plugin.value === 'calico' ? Share : plugin.value === 'flannel' ? Connection : Aim" /></el-icon></span><strong>{{ plugin.value === 'cilium' ? 'Cilium' : plugin.label }}</strong><small>{{ pluginNotes[plugin.value] }}</small></label></div></el-form-item>
          <div class="deploy-grid deploy-subpanel">
            <el-form-item label="Pod CIDR" required><div class="field"><el-input v-model="config.podCIDR" class="deploy-mono" placeholder="172.30.0.0/16" /><div class="deploy-help">Pod 地址网段，部署后本功能不支持修改。</div></div></el-form-item>
            <el-form-item label="Service CIDR" required><div class="field"><el-input v-model="config.serviceCIDR" class="deploy-mono" placeholder="10.254.0.0/16" /><div class="deploy-help">不能与 Pod、主机或 VPC 网段重叠。</div></div></el-form-item>
            <el-form-item label="默认节点网口" required><div class="field"><el-input v-model="config.interface" placeholder="eth0" /><div class="deploy-help">关联节点后可逐台调整，部署前将验证实际网口。</div></div></el-form-item>
          </div>
          <h2 class="deploy-section-title">节点初始化</h2>
          <div class="deploy-grid">
            <el-form-item label="节点命名模式"><div class="field"><el-radio-group v-model="config.autoHostname"><el-radio :value="false">保留主机名</el-radio><el-radio :value="true">自动命名</el-radio></el-radio-group><div class="deploy-help">自动命名将修改系统主机名，可在节点表预览并调整。</div></div></el-form-item>
            <el-form-item label="关闭 SELinux"><div class="field"><el-switch v-model="config.disableSELinux" aria-label="关闭 SELinux" /><div class="deploy-help">仅在节点安装了 SELinux 时执行。</div></div></el-form-item>
            <el-form-item label="关闭 Firewalld"><div class="field"><el-switch v-model="config.disableFirewalld" aria-label="关闭 Firewalld" /><div class="deploy-help">默认保留防火墙。CentOS 正在运行 Firewalld 时，需核对网络要求后选择关闭；此设置在正式安装时执行。</div></div></el-form-item>
            <el-form-item label="Kubernetes 镜像仓库" required class="wide"><div class="field"><el-input v-model="config.imageRepository" placeholder="registry.cn-hangzhou.aliyuncs.com/google_containers" /><div class="deploy-help">基础组件镜像仓库；网络插件和 Metrics Server 使用各自官方镜像。</div></div></el-form-item>
            <el-form-item label="自定义软件源" class="wide"><div class="field"><el-switch v-model="customSource" aria-label="自定义软件源" /><div class="deploy-help">默认按系统匹配阿里云源。启用后使用自定义 APT deb 行或 Yum repo；APT 原配置会备份，Yum 仓库 ID 需以 ops-admin-centos7- 开头并启用签名校验。</div><el-input v-if="customSource" v-model="config.customSource" type="textarea" :rows="4" class="deploy-mono" placeholder="APT 示例：deb https://mirrors.aliyun.com/ubuntu noble main restricted universe multiverse；CentOS 请填写 Yum repo 配置" /></div></el-form-item>
          </div>
          <el-collapse><el-collapse-item title="自定义 kubelet 配置（可选）" name="kubelet"><p class="deploy-help">仅支持 maxPods、systemReserved、kubeReserved，使用 YAML 格式。</p><el-input v-model="config.kubeletConfig" type="textarea" :rows="5" class="deploy-mono" placeholder="maxPods: 110&#10;systemReserved:&#10;  cpu: '200m'&#10;  memory: '256Mi'" /></el-collapse-item></el-collapse>
        </template>
        <template v-else-if="step === 1">
          <h2 class="deploy-section-title">可用性与组件</h2>
          <div class="deploy-grid deploy-advanced-grid">
            <el-form-item label="高可用"><div class="field"><el-switch v-model="config.ha" aria-label="高可用" /><div class="deploy-help">启用后需要关联 3 个控制平面节点，采用堆叠 etcd。</div></div></el-form-item>
            <el-form-item label="自建负载均衡"><div class="field"><el-switch v-model="config.builtinLB" aria-label="自建负载均衡" :disabled="!config.ha" /><div class="deploy-help">在控制平面节点安装 HAProxy + Keepalived；关闭时使用已有负载均衡。</div></div></el-form-item>
            <el-form-item :label="config.builtinLB ? 'API Server VIP' : 'API Server 地址'" :required="config.ha"><div class="field"><el-input v-model="config.apiServer" :placeholder="config.builtinLB ? '填写未被使用的 VIP' : 'IP 或域名，不含协议与端口'" /><div class="deploy-help">{{ config.ha ? '需要稳定的 API 入口，已有 LB 应配置到所有控制平面的 6443 端口。' : '可留空，自动使用首个控制平面节点 IP。' }}</div></div></el-form-item>
            <el-form-item label="API 入口端口" required><div class="field"><el-input-number v-model="config.apiPort" :min="1" :max="65535" /><div class="deploy-help">默认 6443；自建 LB 推荐 8443，后端 API 仍使用 6443。</div></div></el-form-item>
            <el-form-item v-if="config.builtinLB" label="虚拟路由 ID" required><div class="field"><el-input-number v-model="config.routerId" :min="1" :max="255" /><div class="deploy-help">同网络中应保持唯一；需要允许节点间 VRRP 单播通信。</div></div></el-form-item>
            <el-form-item label="Kube-proxy 模式"><el-radio-group v-model="config.proxyMode"><el-radio value="iptables">iptables</el-radio><el-radio value="ipvs">IPVS</el-radio></el-radio-group></el-form-item>
            <el-form-item label="自定义证书有效期" class="wide"><div class="field"><el-switch v-model="config.customCertificates" aria-label="自定义证书有效期" /><div v-if="config.customCertificates" class="deploy-inline" style="margin-top:16px"><span>集群证书</span><el-input-number v-model="config.certificateYears" aria-label="集群证书有效期（年）" :min="1" :max="10" /><span>年</span><span style="margin-left:14px">CA</span><el-input-number v-model="config.caYears" aria-label="CA 有效期（年）" :min="config.certificateYears" :max="30" /><span>年</span></div><div class="deploy-help">通过 kubeadm 配置设置；关闭时使用 kubeadm 默认有效期。</div></div></el-form-item>
            <el-form-item label="Metrics Server"><div class="field"><el-switch v-model="config.metricsServer" aria-label="Metrics Server" /><div class="deploy-help">提供 kubectl top 与 HPA 指标。需要 kubelet 服务证书可被集群 CA 验证；默认不安装，可在集群就绪后配置。</div></div></el-form-item>
            <el-form-item label="控制平面运行普通业务"><div class="field"><el-switch v-model="config.allowControlPlaneWorkloads" aria-label="控制平面运行普通业务" /><div class="deploy-help">没有工作节点的试验集群需要开启；生产环境建议使用独立工作节点。</div></div></el-form-item>
          </div>
        </template>
        <template v-else>
          <div class="deploy-toolbar"><h2>节点清单</h2><div class="deploy-inline"><span class="deploy-node-count">{{ counts.masters }} 控制平面 / {{ counts.workers }} 工作节点</span><el-button type="primary" :icon="Plus" @click="pickerOpen = true">从 CMDB 关联节点</el-button></div></div>
          <div class="deploy-node-source"><el-icon><Monitor /></el-icon><div><strong>主机信息自动同步</strong><p>{{ capabilities.systems || 'Ubuntu / Debian / CentOS 7.9' }} · 部署前将重新核验系统与连通性。</p></div></div>
          <el-table :data="form.nodes" row-key="hostId" border>
            <el-table-column label="主机 / 系统" min-width="185"><template #default="{ row }"><strong>{{ row.hostName }}</strong><div class="deploy-help">{{ row.os || '待预检识别' }}</div><div class="deploy-help">{{ row.cpu || '—' }} 核 / {{ row.memory || '—' }}</div></template></el-table-column>
            <el-table-column label="角色" width="140"><template #default="{ row }"><el-select v-model="row.role" :aria-label="`${row.hostName} 角色`"><el-option label="控制平面" value="master" /><el-option label="工作节点" value="worker" /></el-select></template></el-table-column>
            <el-table-column label="部署 IP" min-width="160"><template #default="{ row }"><el-input v-model="row.ip" :aria-label="`${row.hostName} 部署 IP`" /></template></el-table-column>
            <el-table-column label="节点名称" min-width="170"><template #default="{ row }"><el-input v-model="row.name" :readonly="!config.autoHostname" :aria-label="`${row.hostName} 节点名称`" /><div v-if="!config.autoHostname" class="deploy-help">预检时读取实际主机名</div></template></el-table-column>
            <el-table-column label="网口" width="115"><template #default="{ row }"><el-input v-model="row.interface" :aria-label="`${row.hostName} 网口`" /></template></el-table-column>
            <el-table-column label="操作" width="75"><template #default="{ $index }"><el-button link type="danger" @click="form.nodes.splice($index, 1)">移除</el-button></template></el-table-column>
            <template #empty><div class="deploy-empty"><div class="deploy-empty-icon"><DeployMark /></div><h3>为集群选择第一台服务器</h3><p>从 CMDB 关联已有主机，再为它分配控制平面或工作节点角色。</p><el-button type="primary" :icon="Plus" @click="pickerOpen = true">关联 CMDB 节点</el-button></div></template>
          </el-table>
          <p class="deploy-help">{{ config.ha ? '需要 3 个控制平面节点。' : '需要 1 个控制平面节点。' }}部署 IP 用于节点间通信，SSH 连接仍使用资产管理中的地址与网关配置。</p>
        </template>
      </el-form>
      <div v-else class="deploy-form">
        <DeploySummary :plan="form" :editable="!running && canEdit" @edit="value => step = value" />
        <el-alert title="部署将安装软件包、配置 containerd 和 Kubernetes，并根据选项修改软件源、主机名、swap、内核参数、SELinux 及 Firewalld。请使用准备好的目标服务器。" type="warning" show-icon :closable="false" style="margin-bottom:20px" />
        <div class="deploy-toolbar"><h2>部署预检</h2><el-button :loading="running && lastRun?.kind === 'precheck'" :disabled="saving || running || !canEdit" @click="precheck">{{ lastRun ? '重新预检' : '执行预检' }}</el-button></div>
        <p v-if="!lastRun" class="deploy-help">预检先使用阿里云软件源自动准备 Python、网络工具、证书与时间同步组件，再检查环境；不安装 Kubernetes、不升级内核、不重启。CentOS 7.9 需运行 5.10/5.15 或 6.x 内核。通过后 15 分钟内可部署，修改配置后需重新预检。</p>
        <el-alert v-else-if="dirty" title="配置已修改，请重新预检。" type="info" :closable="false" />
        <el-alert v-else-if="lastRun.error" :title="lastRun.error" type="error" show-icon :closable="false" />
        <el-alert v-else-if="canDeploy" title="预检通过，可以开始部署。未知 VPC 网段与外部网络策略仍需确认。" type="success" show-icon :closable="false" />
        <div v-for="(record, index) in report" :key="record.id" class="deploy-step-record" :class="record.status"><span class="deploy-step-dot">{{ record.status === 'success' ? '✓' : index + 1 }}</span><div class="deploy-step-copy"><strong>{{ record.title }}</strong><p>{{ record.hostName }} · {{ record.summary || runStates[record.status] }}</p></div><el-tag size="small" :type="record.status === 'failed' ? 'danger' : record.status === 'success' ? 'success' : 'primary'">{{ runStates[record.status] }}</el-tag></div>
        <div style="margin-top:24px"><el-checkbox v-model="accepted" :disabled="!canDeploy || saving">已核对配置和目标节点，确认执行安装</el-checkbox></div>
      </div>
      </section>
    </div>
    <footer class="deploy-footer"><div class="deploy-actions"><el-button v-if="step > 0" @click="step--">上一步</el-button><el-button v-else @click="router.push('/containers/k8s-deployments')">取消</el-button><el-button :loading="saving" :disabled="running || !canEdit" @click="save()">保存草稿</el-button></div><span class="deploy-footer-state"><i></i>{{ !form.id ? '尚未保存的部署计划' : dirty ? '有尚未保存的配置' : '配置已保存' }}</span><div class="deploy-actions"><el-button v-if="form.id" @click="router.push(`/containers/k8s-deployments/${form.id}`)">查看详情</el-button><el-button v-if="step < 3" type="primary" @click="next">下一步：{{ titles[step + 1] }}</el-button><el-button v-else type="primary" :loading="saving" :disabled="!canDeploy || !accepted || running" @click="start">确认并部署</el-button></div></footer>
    <DeployHostPicker v-model="pickerOpen" :existing="form.nodes" :plan-id="form.id" @confirm="addNodes" />
  </div>
</template>
