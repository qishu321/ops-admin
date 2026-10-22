<script setup>
import { computed, nextTick, onActivated, onDeactivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Refresh, Search, Upload } from '@element-plus/icons-vue'
import { queryAssetServiceList, queryAssetServiceRuntimeTopology, queryAssetServiceWorkloadRuntime, queryAssetServiceDiagnosisEnvironment, queryAssetServiceDiagnosisProcesses, queryAssetServiceDiagnosisRun, uploadAssetServiceArthas } from '../../api/asset'
import { queryK8sPodContainers } from '../../api/k8s'

const loading = ref(false)
const actionLoading = ref(false)
const diagnosticLoading = ref(false)
const services = ref([])
const topology = ref({})
const runtime = ref({})
const containers = ref([])
const processes = ref([])
const environment = ref(null)
const diagnostic = ref(null)
const flameSrcdoc = ref('')
const flameViewport = ref(null)
const flameResults = ref({ cpu: null, alloc: null })
let flameRenderVersion = 0
let sessionVersion = 0, reloadOnReturn = false, pageActive = true
const target = ref({ serviceId: '', workloadKey: '', podName: '', container: '', pid: '' })
const activeTab = ref('dashboard')
const classPattern = ref('')
const sampleSeconds = ref(30)
const flameEvent = ref('cpu')

const workloads = computed(() => topology.value.workloads || [])
const selectedWorkload = computed(() => workloads.value.find(item => `${item.type}:${item.name}` === target.value.workloadKey))
const canInspect = computed(() => target.value.serviceId && target.value.workloadKey && target.value.podName && target.value.container)
const params = computed(() => ({ serviceId: Number(target.value.serviceId), workloadType: selectedWorkload.value?.type, workloadName: selectedWorkload.value?.name, podName: target.value.podName, container: target.value.container, pid: target.value.pid }))
const resultRows = computed(() => diagnostic.value?.rows || [])
const dashboard = computed(() => diagnostic.value?.dashboard || null)
const currentFlameResult = computed(() => flameResults.value[flameEvent.value] || null)
const flamePreviewHeight = computed(() => {
  const html = diagnostic.value?.flameHtml || ''
  const match = html.match(/#canvas\s*\{[^}]*height:\s*(\d+)px/i)
  const canvasHeight = Number(match?.[1] || 0)
  return `${Math.max(canvasHeight + 110, 520)}px`
})
const flameViewportHeight = computed(() => {
  const previewHeight = Number.parseInt(flamePreviewHeight.value, 10) || 520
  return `${Math.min(previewHeight, 680)}px`
})
const activeOperation = computed(() => ({ dashboard: 'JVM 仪表板', thread: '线程分析', jvm: 'JVM 信息', memory: '内存信息', env: '环境变量', sysprop: '系统属性', class: '代码分析', flame: '火焰图' }[activeTab.value] || '诊断结果'))

function readableName(name) {
  const labels = { heap: '堆内存', nonheap: '非堆内存', Memory: '内存', Runtime: '运行时', Thread: '线程', java: 'Java', os: '操作系统', gc: 'GC' }
  return labels[name] || name
}
function prepareFlameHtml(html) {
  return String(html || '').replace(/\blet inverted = true;/, 'let inverted = false;')
}
async function loadServices() { const version = sessionVersion; const data = await queryAssetServiceList({ pageNum: 1, pageSize: 100 }); if (version === sessionVersion) services.value = data.list || [] }
async function changeService() { reset('service'); const version = sessionVersion; const data = target.value.serviceId ? await queryAssetServiceRuntimeTopology(target.value.serviceId) : {}; if (version === sessionVersion) topology.value = data }
async function changeWorkload() { reset('workload'); const version = sessionVersion; if (selectedWorkload.value) { const data = await queryAssetServiceWorkloadRuntime(params.value); if (version === sessionVersion) runtime.value = data } }
async function changePod() { reset('pod'); if (!target.value.podName) return; const service = services.value.find(item => Number(item.id) === Number(target.value.serviceId)); const version = sessionVersion; const data = await queryK8sPodContainers(service.k8sClusterId, service.namespace, target.value.podName); if (version === sessionVersion) containers.value = data }
function clearFlameResults() {
  flameResults.value = { cpu: null, alloc: null }
  clearFlamePreview()
  if (activeTab.value === 'flame') diagnostic.value = null
}
function clearFlamePreview() {
  flameRenderVersion += 1
  flameSrcdoc.value = ''
}
function positionFlameAtRoot() {
  const showTop = () => {
    if (flameViewport.value) flameViewport.value.scrollTop = 0
  }
  requestAnimationFrame(showTop)
  setTimeout(showTop, 80)
}
function changeContainer() {
  target.value.pid = ''
  processes.value = []
  environment.value = null
  diagnostic.value = null
  clearFlameResults()
}
function changeProcess() {
  diagnostic.value = null
  clearFlameResults()
}
function reset(level) { if (level === 'service') target.value.workloadKey = ''; if (level === 'service' || level === 'workload') { target.value.podName = ''; runtime.value = {} }; if (level !== 'pod') containers.value = []; target.value.container = ''; target.value.pid = ''; processes.value = []; environment.value = null; diagnostic.value = null; clearFlameResults() }
async function refreshProcesses() { if (!canInspect.value) return ElMessage.warning('请先选择服务、工作负载、Pod 和容器'); clearFlameResults(); const version = sessionVersion; actionLoading.value = true; try { const data = await queryAssetServiceDiagnosisProcesses(params.value); if (version !== sessionVersion) return; processes.value = data.processes || []; target.value.pid = ''; const env = await queryAssetServiceDiagnosisEnvironment(params.value); if (version !== sessionVersion) return; environment.value = env; ElMessage.success(`已加载 ${processes.value.length} 个 Java 进程，并完成环境检查`) } finally { if (version === sessionVersion) actionLoading.value = false } }
async function checkEnvironment() { if (!canInspect.value) return ElMessage.warning('请先选择诊断目标'); const version = sessionVersion; actionLoading.value = true; try { const data = await queryAssetServiceDiagnosisEnvironment(params.value); if (version === sessionVersion) environment.value = data } finally { if (version === sessionVersion) actionLoading.value = false } }
async function uploadArthas(file) { if (!canInspect.value) { ElMessage.warning('请先选择诊断目标'); return false }; const version = sessionVersion; actionLoading.value = true; try { const form = new FormData(); form.append('file', file.raw); Object.entries(params.value).forEach(([key, value]) => form.append(key, value || '')); const data = await uploadAssetServiceArthas(form); if (version !== sessionVersion) return false; environment.value = data; ElMessage.success('Arthas 已上传') } finally { if (version === sessionVersion) actionLoading.value = false }; return false }
async function runDiagnostic(operation, extra = {}) {
  if (!target.value.pid) return ElMessage.warning('请先刷新并选择 Java 进程 PID')
  if (!environment.value?.ready) return ElMessage.warning('请先检查 Arthas 环境')
  const version = sessionVersion
  diagnosticLoading.value = true
  diagnostic.value = null
  clearFlamePreview()
  try {
    const result = await queryAssetServiceDiagnosisRun({ ...params.value, operation, ...extra })
    if (version !== sessionVersion) return
    if (operation === 'flame' && result.data?.flameHtml) {
      const event = result.data.event === 'alloc' ? 'alloc' : 'cpu'
      const cached = { ...result.data, flameHtml: prepareFlameHtml(result.data.flameHtml), seconds: Number(extra.seconds), generatedAt: new Date().toLocaleString() }
      flameResults.value = { ...flameResults.value, [event]: cached }
      await showFlameResult(cached)
    } else {
      diagnostic.value = result.data
    }
  } finally {
    if (version === sessionVersion) diagnosticLoading.value = false
  }
}
async function showFlameResult(result) {
  const renderVersion = ++flameRenderVersion
  diagnostic.value = result || null
  flameSrcdoc.value = ''
  if (!result?.flameHtml) return
  // Mount and lay out the empty iframe first. async-profiler measures the
  // canvas width while its inline script starts; injecting srcdoc in the
  // same render pass can make that measurement zero and leave a blank graph.
  await nextTick()
  await new Promise(resolve => requestAnimationFrame(resolve))
  if (renderVersion === flameRenderVersion) {
    flameSrcdoc.value = result.flameHtml
    await nextTick()
    if (renderVersion === flameRenderVersion) positionFlameAtRoot()
  }
}
function changeFlameEvent() { return showFlameResult(currentFlameResult.value) }
function runFlameDiagnostic() { return runDiagnostic('flame', { event: flameEvent.value, seconds: Number(sampleSeconds.value) }) }
function exportFlamegraph() {
  const result = currentFlameResult.value
  if (!result?.flameHtml) return ElMessage.warning('请先生成当前类型的火焰图')
  const blob = new Blob([result.flameHtml], { type: 'text/html;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  const stamp = new Date().toISOString().replace(/[:.]/g, '-')
  link.href = url
  link.download = `arthas-flame-${flameEvent.value}-${result.seconds || sampleSeconds.value}-${stamp}.html`
  link.click()
  URL.revokeObjectURL(url)
}
function changeTab(tab) {
  activeTab.value = tab
  if (tab === 'flame') return showFlameResult(currentFlameResult.value)
  diagnostic.value = null
  clearFlamePreview()
}
function clearSession() {
  sessionVersion++
  reset('service')
  target.value.serviceId = ''
  topology.value = {}; services.value = []
  activeTab.value = 'dashboard'; classPattern.value = ''
  actionLoading.value = false; diagnosticLoading.value = false
  reloadOnReturn = true
}
function handleTabClosed(event) {
  if (event.detail?.path === '/containers/services/health-diagnosis') clearSession()
}
function handlePermissionsChanged() { clearSession(); if (pageActive) { reloadOnReturn = false; loadServices() } }
onMounted(() => {
  window.addEventListener('ops-admin:tab-closed', handleTabClosed)
  window.addEventListener('ops-admin-permissions-changed', handlePermissionsChanged)
  loadServices()
})
onActivated(() => {
  pageActive = true
  if (reloadOnReturn) { reloadOnReturn = false; loadServices() }
})
onDeactivated(() => { pageActive = false })
onBeforeUnmount(() => {
  sessionVersion++
  clearFlameResults()
  window.removeEventListener('ops-admin:tab-closed', handleTabClosed)
  window.removeEventListener('ops-admin-permissions-changed', handlePermissionsChanged)
})
</script>

<template>
  <div class="diagnosis-page" v-loading="loading">
    <section class="hero"><div class="hero-icon">⌘</div><div><h1>Arthas Java 诊断</h1><p>通过当前容器内的 Arthas CLI 获取 Java 进程诊断数据。</p></div></section>
    <section class="target-card">
      <div class="target-fields">
        <label>服务<el-select v-model="target.serviceId" filterable placeholder="选择服务" @change="changeService"><el-option v-for="item in services" :key="item.id" :label="item.name" :value="item.id"/></el-select></label>
        <label>工作负载<el-select v-model="target.workloadKey" :disabled="!target.serviceId" placeholder="选择工作负载" @change="changeWorkload"><el-option v-for="item in workloads" :key="`${item.type}:${item.name}`" :label="`${item.type} · ${item.name}`" :value="`${item.type}:${item.name}`"/></el-select></label>
        <label>Pod<el-select v-model="target.podName" :disabled="!target.workloadKey" placeholder="选择 Pod" @change="changePod"><el-option v-for="item in runtime.pods || []" :key="item.name" :label="item.name" :value="item.name"/></el-select></label>
        <label>容器<el-select v-model="target.container" :disabled="!target.podName" placeholder="选择容器" @change="changeContainer"><el-option v-for="item in containers" :key="item" :label="item" :value="item"/></el-select></label>
        <label>进程<el-select v-model="target.pid" :disabled="!processes.length" placeholder="刷新进程后选择" @change="changeProcess"><el-option v-for="item in processes" :key="item.pid" :label="`PID ${item.pid} · ${item.name}`" :value="item.pid"/></el-select></label>
      </div>
      <div class="target-actions"><el-button type="success" :loading="actionLoading" :icon="Check" @click="checkEnvironment">检查环境</el-button><el-button type="primary" :loading="actionLoading" :icon="Refresh" @click="refreshProcesses">刷新进程</el-button><el-upload :show-file-list="false" accept=".jar" :auto-upload="false" :on-change="uploadArthas"><el-button type="warning" :loading="actionLoading" :icon="Upload">上传 Arthas</el-button></el-upload></div>
    </section>
    <el-alert v-if="environment" :type="environment.ready ? 'success' : 'warning'" :closable="false" show-icon class="environment"><template #title>Arthas 环境：{{ environment.ready ? '已就绪，可以开始诊断' : '未就绪，请完成环境准备' }}</template><p>{{ environment.message }}</p></el-alert>
    <section class="diagnostic-card">
      <el-tabs v-model="activeTab" @tab-change="changeTab">
        <el-tab-pane label="JVM仪表板" name="dashboard"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('dashboard')">刷新数据</el-button></div></el-tab-pane>
        <el-tab-pane label="线程分析" name="thread"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('thread')">刷新线程</el-button></div></el-tab-pane>
        <el-tab-pane label="JVM信息" name="jvm"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('jvm')">刷新数据</el-button></div></el-tab-pane>
        <el-tab-pane label="内存信息" name="memory"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('memory')">刷新数据</el-button></div></el-tab-pane>
        <el-tab-pane label="环境变量" name="env"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('env')">刷新数据</el-button></div></el-tab-pane>
        <el-tab-pane label="系统属性" name="sysprop"><div class="tab-tools"><el-button type="primary" :icon="Refresh" :loading="diagnosticLoading" @click="runDiagnostic('sysprop')">刷新数据</el-button></div></el-tab-pane>
        <el-tab-pane label="代码分析" name="class"><div class="tab-tools"><el-input v-model="classPattern" placeholder="输入类名模式（支持通配符 *）" @keyup.enter="runDiagnostic('class', { pattern: classPattern })"/><el-button type="primary" :icon="Search" :loading="diagnosticLoading" @click="runDiagnostic('class', { pattern: classPattern })">搜索类</el-button></div></el-tab-pane>
        <el-tab-pane label="火焰图" name="flame"><div class="tab-tools"><b>采样类型：</b><el-radio-group v-model="flameEvent" :disabled="diagnosticLoading" @change="changeFlameEvent"><el-radio-button value="cpu">CPU</el-radio-button><el-radio-button value="alloc">内存分配</el-radio-button></el-radio-group><b>采样时长：</b><el-radio-group v-model="sampleSeconds" :disabled="diagnosticLoading"><el-radio-button :value="10">10秒</el-radio-button><el-radio-button :value="30">30秒</el-radio-button><el-radio-button :value="60">60秒</el-radio-button><el-radio-button :value="120">120秒</el-radio-button></el-radio-group><el-button type="primary" :loading="diagnosticLoading" @click="runFlameDiagnostic">{{ currentFlameResult ? '重新生成' : '生成火焰图' }}</el-button><el-button v-if="currentFlameResult" :icon="Download" @click="exportFlamegraph">导出火焰图</el-button></div></el-tab-pane>
      </el-tabs>
      <div v-if="!diagnostic" class="empty">点击上方按钮获取{{ activeOperation }}数据</div>
      <div v-else class="result-panel">
        <template v-if="activeTab === 'dashboard' && dashboard">
          <el-row :gutter="16" class="metric-grid"><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>热点线程数</span><strong>{{ dashboard.hotThreads }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>堆已用</span><strong>{{ dashboard.heapUsed }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>堆总量</span><strong>{{ dashboard.heapTotal }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>非堆已用</span><strong>{{ dashboard.nonHeapUsed }}</strong></div></el-col></el-row>
          <el-descriptions title="内存摘要" :column="4" border class="details"><el-descriptions-item label="堆已用">{{ dashboard.heapUsed }}</el-descriptions-item><el-descriptions-item label="堆总量">{{ dashboard.heapTotal }}</el-descriptions-item><el-descriptions-item label="非堆已用">{{ dashboard.nonHeapUsed }}</el-descriptions-item><el-descriptions-item label="非堆总量">{{ dashboard.nonHeapTotal }}</el-descriptions-item></el-descriptions>
        </template>
        <el-table v-else-if="activeTab === 'thread' && diagnostic.threads" :data="diagnostic.threads" border stripe class="thread-table"><el-table-column prop="id" label="线程ID" width="110"/><el-table-column prop="name" label="线程名称" min-width="260" show-overflow-tooltip/><el-table-column prop="state" label="状态" width="150"/><el-table-column prop="cpu" label="CPU %" width="120"/><el-table-column prop="time" label="时间" width="140"/></el-table>
        <el-row v-else-if="activeTab === 'jvm' && diagnostic.jvm" :gutter="16" class="metric-grid"><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>虚拟机名称</span><strong>{{ diagnostic.jvm.vmName }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>JVM 版本</span><strong>{{ diagnostic.jvm.vmVersion }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>JVM 启动时间</span><strong>{{ diagnostic.jvm.startTime }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>已加载类数</span><strong>{{ diagnostic.jvm.loadedClasses }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>当前线程数</span><strong>{{ diagnostic.jvm.threads }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="8"><div class="metric-card"><span>可用处理器</span><strong>{{ diagnostic.jvm.processors }}</strong></div></el-col></el-row>
        <template v-else-if="activeTab === 'memory' && diagnostic.memory"><el-row :gutter="16" class="metric-grid"><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>堆已用</span><strong>{{ diagnostic.memory.heap.used }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>堆使用率</span><strong>{{ diagnostic.memory.heap.usage }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>非堆已用</span><strong>{{ diagnostic.memory.nonHeap.used }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>非堆使用率</span><strong>{{ diagnostic.memory.nonHeap.usage }}</strong></div></el-col></el-row><el-table :data="diagnostic.memory.rows" border stripe class="memory-table"><el-table-column prop="name" label="内存池" min-width="230"/><el-table-column prop="used" label="已用" width="130"/><el-table-column prop="total" label="总量" width="130"/><el-table-column prop="max" label="最大值" width="130"/><el-table-column prop="usage" label="使用率" width="130"/></el-table></template>
        <el-table v-else-if="activeTab === 'env' && diagnostic.envs" :data="diagnostic.envs" border stripe class="environment-table"><el-table-column prop="key" label="环境变量" min-width="300"/><el-table-column prop="value" label="变量值" min-width="560" show-overflow-tooltip/></el-table>
        <el-row v-else-if="activeTab === 'sysprop' && diagnostic.propertySummary" :gutter="16" class="metric-grid"><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>应用名称</span><strong>{{ diagnostic.propertySummary.application }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>启动命令</span><strong>{{ diagnostic.propertySummary.command }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>进程 PID</span><strong>{{ diagnostic.propertySummary.pid }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>工作目录</span><strong>{{ diagnostic.propertySummary.workDir }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>Java Home</span><strong>{{ diagnostic.propertySummary.javaHome }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>临时目录</span><strong>{{ diagnostic.propertySummary.tempDir }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>时区</span><strong>{{ diagnostic.propertySummary.timezone }}</strong></div></el-col><el-col :xs="24" :sm="12" :lg="6"><div class="metric-card"><span>文件编码</span><strong>{{ diagnostic.propertySummary.encoding }}</strong></div></el-col></el-row>
        <el-descriptions v-else :title="activeOperation" :column="1" border class="details"><el-descriptions-item v-for="(item, index) in resultRows" :key="`${item.name}-${index}`" :label="readableName(item.name)">{{ item.value }}</el-descriptions-item></el-descriptions>
        <template v-if="activeTab === 'flame' && diagnostic.flameHtml">
          <el-alert type="success" :closable="false" show-icon :title="`${diagnostic.event === 'alloc' ? '内存分配' : 'CPU'}火焰图已生成 · ${diagnostic.seconds}秒 · ${diagnostic.generatedAt}`" />
          <div ref="flameViewport" class="flame-viewport" :style="{ height: flameViewportHeight }">
            <iframe class="flame-preview" :srcdoc="flameSrcdoc || '<!doctype html><html><body></body></html>'" :style="{ height: flamePreviewHeight }" title="Arthas 火焰图预览" @load="positionFlameAtRoot" />
          </div>
        </template>
        <el-collapse class="raw-output"><el-collapse-item title="查看 Arthas 原始命令输出（诊断参考）"><pre>{{ diagnostic.raw }}</pre></el-collapse-item></el-collapse>
      </div>
    </section>
  </div>
</template>

<style scoped>
.diagnosis-page{min-height:100%;padding:26px 30px;background:#f5f8fd}.hero,.target-card,.diagnostic-card{border:1px solid #e0e8f5;border-radius:18px;background:#fff;box-shadow:0 8px 24px rgba(55,83,135,.06)}.hero{display:flex;align-items:center;gap:16px;padding:20px 24px}.hero-icon{display:grid;place-items:center;width:48px;height:48px;border-radius:14px;color:#fff;background:linear-gradient(135deg,#8067e8,#4f82e8);font-size:27px;font-weight:800}.hero h1{margin:0;color:#536bd4;font-size:23px}.hero p{margin:6px 0 0;color:#8391a8}.target-card{display:flex;align-items:end;justify-content:space-between;gap:18px;padding:16px 20px;margin-top:14px}.target-fields,.target-actions,.tab-tools{display:flex;gap:12px;align-items:end;flex-wrap:wrap}.target-fields label{display:grid;gap:6px;color:#60708a;font-size:12px}.target-fields .el-select{width:172px}.target-actions{flex-wrap:nowrap}.environment{margin-top:14px}.environment p{margin:6px 0 0}.diagnostic-card{min-height:430px;margin-top:14px;padding:0 22px}.tab-tools{align-items:center;padding:12px 0 20px;border-bottom:1px solid #edf1f7}.tab-tools .el-input{width:400px}.empty{display:grid;place-items:center;min-height:260px;border-radius:10px;color:#8b99af;background:#f8faff}.result-panel{padding:20px 0}.metric-grid{margin-bottom:20px}.metric-card{display:grid;min-height:112px;place-items:center;border:1px solid #e2e9f7;border-radius:12px;color:#7c8ba2;background:#fbfcff}.metric-card strong{max-width:95%;overflow:hidden;color:#5f72d9;font-size:18px;text-overflow:ellipsis;white-space:nowrap}.details{margin-top:12px}.thread-table,.memory-table,.environment-table{width:100%;margin-top:12px}.flame-viewport{width:100%;margin-top:16px;overflow:auto;border:1px solid #dfe7f5;border-radius:12px;background:#fff}.flame-preview{display:block;width:100%;min-height:520px;margin:0;border:0;background:#fff}.raw-output{margin-top:16px}.raw-output pre{max-height:360px;overflow:auto;margin:0;padding:14px;border-radius:8px;color:#dbe8ff;background:#101c33;white-space:pre-wrap;font:12px/1.7 Consolas,monospace}@media(max-width:1100px){.target-card{align-items:flex-start;flex-direction:column}.target-actions{flex-wrap:wrap}}
</style>
