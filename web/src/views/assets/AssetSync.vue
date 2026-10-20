<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Connection, Refresh, Plus, ArrowRight, FolderOpened, Monitor, Key, Check, Warning, Search, Edit, Delete } from '@element-plus/icons-vue'
import { queryAssetSyncData, queryAssetSyncHostOptions, saveAssetSyncSource, deleteAssetSyncSource, testAssetSyncSource, previewAssetSync, applyAssetSync, retryAssetSyncCollection, queryAssetGatewayOptions, queryAssetSyncRuns, assetSyncRunInfo, queryAssetHostGroupList } from '../../api/asset'
import { pendingAssetSyncItems, assetSyncProvider, assetSyncAccountRegions } from '../../utils/asset-sync-preview'
import { useEnvironmentOptions } from '../../composables/useEnvironmentOptions'

const router = useRouter(), route = useRoute()
const { environmentOptions, environmentLoading, environmentName } = useEnvironmentOptions()
const cloudAccounts = ref([]), gateways = ref([])
const providerOptions = [{ value: 'jumpserver', label: 'JumpServer', badge: 'JS', hint: '堡垒机资产与节点' }, { value: 'aliyun', label: '阿里云', badge: 'ALI', hint: 'ECS 实例与地域' }, { value: 'tencent', label: '腾讯云', badge: 'TC', hint: 'CVM 实例与地域' }]
const providerInfo = source => providerOptions.find(p => p.value === assetSyncProvider(source?.provider)) || providerOptions[0]
const isCloud = computed(() => assetSyncProvider(selected.value?.provider) !== 'jumpserver')
const formIsCloud = computed(() => assetSyncProvider(form.provider) !== 'jumpserver')
const formAccounts = computed(() => cloudAccounts.value.filter(a => assetSyncProvider(a.provider) === assetSyncProvider(form.provider)))
const formAccount = computed(() => cloudAccounts.value.find(a => a.id === form.cloudAccountId))
const formRegions = computed(() => assetSyncAccountRegions(formAccount.value))
const accountName = id => cloudAccounts.value.find(a => a.id === id)?.name || `云账号 #${id}`
const collectionLabels = { pending: '等待采集', success: '采集成功', failed: '采集失败', skipped: '未采集' }
const canRetryCollection = computed(() => currentRun.value?.inventoryStatus === 'success' && !active.value && items.value.some(i => i.hostId && ['failed', 'skipped', 'pending'].includes(i.collectStatus)))
function changeProvider(value) { form.provider = value; form.cloudAccountId = undefined; form.regions = []; form.nodeIds = []; form.rules = []; form.autoCollect = value !== 'jumpserver' }
function changeAccount() { form.regions = [...formRegions.value]; form.nodeIds = []; form.rules = [] }
async function retryCollection() {
  busy.value = true; const epoch = generation, id = currentRun.value.id
  try { await retryAssetSyncCollection(id); if (epoch === generation) await loadRun(id, epoch) } finally { if (epoch === generation) busy.value = false }
}
const sources = ref([]), credentials = ref([]), groups = ref([]), groupTree = ref([])
const selectedId = ref(0), loading = ref(false), busy = ref(false), testing = ref(false), saving = ref(false)
const selected = computed(() => sources.value.find(s => s.id === selectedId.value))
const nodes = ref([]), items = ref([]), runs = ref([]), currentRun = ref(null), tab = ref('assets')
const showHistoryDetails = ref(false)
const pendingItems = computed(() => pendingAssetSyncItems(items.value, currentRun.value?.status))
const displayedItems = computed(() => showHistoryDetails.value ? items.value : pendingItems.value)
const actionLabels = computed(() => !showHistoryDetails.value && currentRun.value?.inventoryStatus === 'success'
  ? { collect_failed: '采集失败', collect_pending: '等待采集', collect_skipped: '未采集', conflict: '冲突', skip: '跳过' }
  : Object.fromEntries(Object.entries(labels).filter(([key]) => showHistoryDetails.value || key !== 'unchanged')))
const keyword = ref(''), action = ref(''), nodeFilter = ref(''), nodeKeyword = ref(''), page = ref(1), scopeTree = ref(null)
const links = reactive({}), hostOptions = ref([]), hostLoading = ref(false)
const hostCache = reactive({})
const conflictDialog = ref(false), conflictItem = ref(null), conflictTarget = ref(undefined)
const conflictOptions = computed(() => {
  const chosen = hostCache[conflictTarget.value]
  return chosen && !hostOptions.value.some(h => h.id === chosen.id) ? [chosen, ...hostOptions.value] : hostOptions.value
})
function openConflict(row) { conflictItem.value = row; conflictTarget.value = links[row.id] || undefined; conflictDialog.value = true; searchHosts(row.address) }
function confirmConflict() { if (conflictTarget.value) links[conflictItem.value.id] = conflictTarget.value; else delete links[conflictItem.value.id]; conflictDialog.value = false }
const effectiveCredentialId = row => hostCache[links[row.id]]?.credentialId || row.credentialId
const pendingCredentialCount = computed(() => pendingItems.value.filter(i => !['skip', 'missing'].includes(i.action) && !effectiveCredentialId(i)).length)
const scopeAll = ref(true), selectedNodes = ref([]), ruleDraft = ref([]), rootGroupId = ref(0), defaultCredentialId = ref(0)
const dialog = ref(false), form = reactive({})
let timer, generation = 0, hostGeneration = 0
const apiCredentials = computed(() => credentials.value.filter(c => c.authType === 'jumpserver_key'))
const sshCredentials = computed(() => credentials.value.filter(c => ['key', 'password'].includes(c.authType)))
const active = computed(() => Boolean(selected.value?.activeRunId) || ['fetching', 'running', 'collecting'].includes(currentRun.value?.status))
const stalePreview = computed(() => currentRun.value?.status === 'ready' && currentRun.value.sourceVersion !== selected.value?.version)
const labels = { add: '新增', update: '更新', unchanged: '无变化', conflict: '冲突', skip: '跳过', missing: '本次未发现' }
const statuses = { fetching: '生成预览中', ready: '待确认', running: '同步中', collecting: '采集中', success: '同步完成', partial: '部分完成', failed: '失败' }
const statusType = status => ({ ready: 'primary', success: 'success', partial: 'warning', failed: 'danger' }[status] || 'info')
const credentialName = id => credentials.value.find(c => c.id === id)?.name || (id ? `凭据 #${id}` : '待配置凭据')
function localGroupPath(id) {
  const parts = [], seen = new Set()
  while (id && !seen.has(id)) { seen.add(id); const group = groups.value.find(g => g.id === id); if (!group) break; parts.unshift(group.name); id = group.parentId }
  return parts
}
function targetGroupPath(path, row) {
  const node = nodes.value.find(n => n.full_value === path || (n.name === path && row.nodes?.includes(n.id)))
  if (!node) return path
  const parts = [], seen = new Set()
  let id = node.id
  while (id && !seen.has(id)) {
    seen.add(id); const rule = selected.value.rules?.find(r => r.nodeId === id), current = nodeMap.value.get(id)
    if (rule?.groupId) return [...localGroupPath(rule.groupId), ...parts].join(' / ')
    if (!current) break
    parts.unshift(current.name); id = current.parentId
  }
  return [...localGroupPath(selected.value.rootGroupId), `${providerInfo(selected.value).label} · ${selected.value.name}`, ...parts].join(' / ')
}
const date = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const nodeMap = computed(() => new Map(nodes.value.map(n => [n.id, n])))
const treeData = computed(() => {
  const map = new Map(nodes.value.map(n => [n.id, { ...n, children: [] }]))
  const roots = []
  map.forEach(n => { const parent = map.get(n.parentId); if (parent) parent.children.push(n); else roots.push(n) })
  return roots
})
const filteredItems = computed(() => displayedItems.value.filter(item => {
  const q = keyword.value.trim().toLowerCase()
  if (q && !`${item.name} ${item.address}`.toLowerCase().includes(q)) return false
  if (action.value && (action.value.startsWith('collect_') ? item.collectStatus !== action.value.slice(8) : item.action !== action.value)) return false
  if (!nodeFilter.value) return true
  return item.nodes?.some(ref => {
    let id = typeof ref === 'string' ? ref : ref.id || ref.pk
    const visited = new Set()
    while (id && !visited.has(id)) { if (id === nodeFilter.value) return true; visited.add(id); id = nodeMap.value.get(id)?.parentId }
    return false
  })
}))
const visibleItems = computed(() => filteredItems.value.slice((page.value - 1) * 15, page.value * 15))
const ruleNodes = computed(() => nodes.value.filter(n => !nodeKeyword.value || `${n.name} ${n.full_value}`.includes(nodeKeyword.value)))
const rulePage = ref(1)
const visibleRuleNodes = computed(() => ruleNodes.value.slice((rulePage.value - 1) * 10, rulePage.value * 10))
const draftConfig = () => ({ rootGroupId: rootGroupId.value || 0, defaultCredentialId: defaultCredentialId.value || 0, nodeIds: scopeAll.value ? [] : [...selectedNodes.value].sort(), rules: ruleDraft.value.filter(r => r.groupId || r.credentialId).map(r => ({ nodeId: r.nodeId, groupId: r.groupId || 0, credentialId: r.credentialId || 0 })).sort((a, b) => a.nodeId.localeCompare(b.nodeId)) })
const dirty = computed(() => selected.value && JSON.stringify(draftConfig()) !== JSON.stringify({ rootGroupId: selected.value.rootGroupId || 0, defaultCredentialId: selected.value.defaultCredentialId || 0, nodeIds: [...(selected.value.nodeIds || [])].sort(), rules: [...(selected.value.rules || [])].filter(r => r.groupId || r.credentialId).sort((a, b) => a.nodeId.localeCompare(b.nodeId)) }))
const counters = computed(() => {
  const collecting = !showHistoryDetails.value && currentRun.value?.inventoryStatus === 'success'
  return (collecting ? ['collect_failed', 'collect_pending', 'collect_skipped'] : showHistoryDetails.value ? ['add', 'update', 'unchanged', 'conflict'] : ['add', 'update', 'conflict']).map(key => ({ key, label: actionLabels.value[key] || labels[key], count: displayedItems.value.filter(i => key.startsWith('collect_') ? i.collectStatus === key.slice(8) : i.action === key).length }))
})
const unresolved = computed(() => items.value.filter(i => i.action === 'conflict' && !links[i.id]).length)

function resetDraft() {
  const s = selected.value
  scopeAll.value = !s?.nodeIds?.length; selectedNodes.value = [...(s?.nodeIds || [])]
  ruleDraft.value = JSON.parse(JSON.stringify(s?.rules || [])); rootGroupId.value = s?.rootGroupId || 0; defaultCredentialId.value = s?.defaultCredentialId || 0
  nextTick(() => scopeTree.value?.setCheckedKeys(selectedNodes.value))
}
function ruleFor(id) { let rule = ruleDraft.value.find(r => r.nodeId === id); if (!rule) { rule = { nodeId: id, groupId: 0, credentialId: 0 }; ruleDraft.value.push(rule) }; return rule }
function setRule(id, key, value) { ruleFor(id)[key] = value || 0 }
async function loadBase() {
  const [data, groupData, gatewayData] = await Promise.all([queryAssetSyncData(), queryAssetHostGroupList({}), queryAssetGatewayOptions()])
  cloudAccounts.value = data.cloudAccounts || []; gateways.value = gatewayData || []
  sources.value = data.sources || []; credentials.value = data.credentials || []; groups.value = groupData.list || []; groupTree.value = groupData.tree || []
}
function clearPoll() { if (timer) clearTimeout(timer); timer = undefined }
function schedulePoll(id, epoch, delay = 1500) { clearPoll(); timer = setTimeout(() => loadRun(id, epoch).catch(() => { if (epoch === generation) schedulePoll(id, epoch, 5000) }), delay) }
async function loadRun(id, epoch = generation) {
  const data = await assetSyncRunInfo(id)
  if (epoch !== generation) return
  currentRun.value = data.run; nodes.value = data.nodes || []; items.value = data.items || []
  if (['fetching', 'running', 'collecting'].includes(data.run.status)) { schedulePoll(id, epoch); return }
  clearPoll()
  const history = await queryAssetSyncRuns(selectedId.value)
  if (epoch !== generation) return
  runs.value = history || []
  const dataBase = await queryAssetSyncData()
  if (epoch !== generation) return
  sources.value = dataBase.sources || []; credentials.value = dataBase.credentials || []
}
async function selectSource(id) {
  if (selectedId.value === id) return
  if (dirty.value) { try { await ElMessageBox.confirm('切换同步源将放弃未保存的分组配置。', '放弃更改？', { confirmButtonText: '切换同步源', cancelButtonText: '继续编辑' }) } catch { return } }
  clearPoll(); const epoch = ++generation
  selectedId.value = id; nodes.value = []; items.value = []; currentRun.value = null; runs.value = []; nodeFilter.value = ''; tab.value = 'assets'
  showHistoryDetails.value = false
  Object.keys(links).forEach(k => delete links[k]); resetDraft(); busy.value = true
  try {
    const history = await queryAssetSyncRuns(id)
    if (epoch !== generation) return
    runs.value = history || []
    if (selected.value?.activeRunId) await loadRun(selected.value.activeRunId, epoch)
    else if (runs.value[0]) await loadRun(runs.value[0].id, epoch)
  } finally { if (epoch === generation) busy.value = false }
}
function openSource(source, provider = 'jumpserver', accountId) {
  Object.keys(form).forEach(k => delete form[k])
  Object.assign(form, source ? JSON.parse(JSON.stringify(source)) : { provider, cloudAccountId: accountId, regions: assetSyncAccountRegions(cloudAccounts.value.find(a => a.id === accountId)), environment: '', connectionMode: 'direct', gatewayId: 0, autoCollect: provider !== 'jumpserver', name: '', url: '', orgId: '00000000-0000-0000-0000-000000000002', apiCredentialId: undefined, defaultCredentialId: 0, rootGroupId: 0, nodeIds: [], rules: [] })
  form.provider = assetSyncProvider(form.provider)
  if (!source && accountId) form.name = `${accountName(accountId)}资产`
  dialog.value = true
}
async function saveSource() {
  if (!form.name?.trim()) { ElMessage.warning('请填写同步源名称'); return }
  if (!formIsCloud.value && (!form.url?.trim() || !form.apiCredentialId || !form.orgId?.trim())) { ElMessage.warning('请填写地址、组织 ID 并选择 API 凭据'); return }
  if (formIsCloud.value && (!form.cloudAccountId || !form.regions?.length || !form.environment)) { ElMessage.warning('请选择云账号、同步地域和所属环境'); return }
  if (formIsCloud.value && form.connectionMode === 'gateway' && !form.gatewayId) { ElMessage.warning('请选择访问网关'); return }
  if (form.autoCollect && !form.defaultCredentialId) { ElMessage.warning('自动采集需要选择默认主机登录凭据'); return }
  saving.value = true
  try {
    const result = await saveAssetSyncSource({ ...form, apiCredentialId: formIsCloud.value ? 0 : form.apiCredentialId, gatewayId: form.connectionMode === 'gateway' ? form.gatewayId : 0 }); dialog.value = false
    await loadBase(); selectedId.value = 0; await selectSource(result.id); ElMessage.success('同步源已保存，可测试连接并选择同步范围')
  } finally { saving.value = false }
}
async function testConnection() {
  const id = selectedId.value, epoch = generation; testing.value = true
  try { const data = await testAssetSyncSource(id); if (epoch !== generation) return; nodes.value = data || []; await nextTick(); scopeTree.value?.setCheckedKeys(selectedNodes.value); ElMessage.success(`连接成功，已读取 ${nodes.value.length} 个节点`) } finally { testing.value = false }
}
async function saveRules() {
  if (!scopeAll.value && !selectedNodes.value.length) { ElMessage.warning('请至少选择一个同步节点，或切换为全部节点'); return }
  saving.value = true
  try {
    await saveAssetSyncSource({ ...selected.value, ...draftConfig() }); await loadBase(); resetDraft(); currentRun.value = null; items.value = []; ElMessage.success('分组与凭据规则已保存，请重新生成预览')
  } finally { saving.value = false }
}
async function preview() {
  showHistoryDetails.value = false
  busy.value = true; const epoch = generation
  try { const run = await previewAssetSync(selectedId.value); if (epoch !== generation) return; Object.keys(links).forEach(k => delete links[k]); page.value = 1; keyword.value = ''; action.value = ''; nodeFilter.value = ''; tab.value = 'assets'; items.value = []; currentRun.value = run; await loadRun(run.id, epoch) } finally { if (epoch === generation) busy.value = false }
}
async function apply() {
  if (stalePreview.value) { ElMessage.warning('同步配置已变化，请重新生成预览'); return }
  const source = selectedId.value, runId = currentRun.value.id, epoch = generation
  try { await ElMessageBox.confirm(`本次将处理 ${pendingItems.value.filter(i => !['conflict', 'skip'].includes(i.action) || links[i.id]).length} 条资产变更${unresolved.value ? `，跳过 ${unresolved.value} 台未处理的冲突资产` : ''}。已有登录凭据和手工分组会保留。${selected.value.autoCollect ? '入库后将自动检测 SSH 并采集主机信息。' : ''}`, '确认同步资产', { confirmButtonText: '开始同步', cancelButtonText: '继续检查', type: 'info' }) } catch { return }
  if (source !== selectedId.value || runId !== currentRun.value?.id) return
  busy.value = true
  try { await applyAssetSync({ runId, links: { ...links } }); if (epoch === generation) await loadRun(runId, epoch) } finally { if (epoch === generation) busy.value = false }
}
async function removeSource() {
  const source = selected.value
  try { await ElMessageBox.confirm(`删除“${source.name}”的同步配置和记录？已导入的主机、主机组及其归属会保留。`, '删除同步源', { type: 'warning' }) } catch { return }
  await deleteAssetSyncSource(source.id); clearPoll(); generation++; selectedId.value = 0; currentRun.value = null; nodes.value = []; items.value = []; await loadBase(); if (sources.value[0]) await selectSource(sources.value[0].id)
}
async function searchHosts(keyword) {
  const epoch = ++hostGeneration; hostLoading.value = true
  try { const data = await queryAssetSyncHostOptions(keyword); if (epoch === hostGeneration) { hostOptions.value = data || []; hostOptions.value.forEach(h => { hostCache[h.id] = h }) } } finally { if (epoch === hostGeneration) hostLoading.value = false }
}
async function viewHistory(runId) {
  await loadRun(runId)
  showHistoryDetails.value = true
  keyword.value = ''; action.value = ''; nodeFilter.value = ''; page.value = 1; tab.value = 'assets'
}
function leaveHistoryDetails() {
  showHistoryDetails.value = false
  keyword.value = ''; action.value = ''; nodeFilter.value = ''; page.value = 1
}
watch([keyword, action, nodeFilter], () => { page.value = 1 })
watch(nodeKeyword, () => { rulePage.value = 1 })
watch(tab, async value => { if (value === 'rules') { await nextTick(); scopeTree.value?.setCheckedKeys(selectedNodes.value) } })
onMounted(async () => { loading.value = true; try { await loadBase(); const accountId = Number(route.query.cloudAccountId || 0)
    const existing = sources.value.find(s => accountId && s.cloudAccountId === accountId)
    if (existing || sources.value[0]) await selectSource((existing || sources.value[0]).id)
    if (accountId && !existing) { const account = cloudAccounts.value.find(a => a.id === accountId); if (account) openSource(null, assetSyncProvider(account.provider), accountId) }
    else if (route.query.type === 'cloud' && !existing) openSource(null, 'tencent')
  } finally { loading.value = false } })
onBeforeUnmount(() => { generation++; clearPoll() })
</script>

<template>
  <div v-loading="loading" class="asset-sync-page">
    <header class="sync-heading"><div><div class="sync-eyebrow">资产管理 / 外部资产接入</div><h1>资产同步<span>统一接入</span></h1><p>连接云平台与堡垒机，一次配置，完成资产入库、分组与信息采集。</p></div><el-button type="primary" :icon="Plus" @click="openSource()">新增同步源</el-button></header>
    <div class="sync-workspace">
      <aside class="sync-sources sync-panel"><div class="panel-heading"><h2>同步源</h2><span>{{ sources.length }}</span></div>
        <button v-for="source in sources" :key="source.id" type="button" class="source-card" :class="{ selected: selectedId === source.id }" @click="selectSource(source.id)"><span class="source-icon" :class="assetSyncProvider(source.provider)">{{ providerInfo(source).badge }}</span><span class="source-content"><strong>{{ source.name }}</strong><small>{{ assetSyncProvider(source.provider) === 'jumpserver' ? source.url : accountName(source.cloudAccountId) }}</small><span class="source-state"><i :class="{ working: source.activeRunId }" />{{ source.activeRunId ? '任务执行中' : providerInfo(source).label }}</span></span><el-icon><ArrowRight /></el-icon></button>
        <el-empty v-if="!sources.length" description="还没有同步源" :image-size="70"><el-button type="primary" plain @click="openSource()">添加第一个同步源</el-button></el-empty>
        <div class="source-help"><el-icon><Key /></el-icon><div><strong>凭据统一管理</strong><p>云 API 使用已维护的云账号；SSH 登录使用凭据管理中的凭据。</p><el-button link type="primary" @click="router.push('/assets/server/credentials')">前往凭据管理 <el-icon><ArrowRight /></el-icon></el-button></div></div>
      </aside>
      <main v-if="selected" class="sync-main" v-loading="busy">
        <section class="sync-overview sync-panel"><div class="overview-top"><div><div class="overview-title"><span class="provider-badge" :class="assetSyncProvider(selected.provider)">{{ providerInfo(selected).badge }}</span><div><h2>{{ selected.name }}</h2><p v-if="isCloud" class="source-subtitle">{{ accountName(selected.cloudAccountId) }} · {{ selected.regions?.length || 0 }} 个地域</p><a v-else :href="selected.url" target="_blank" rel="noopener noreferrer">{{ selected.url }} <el-icon><ArrowRight /></el-icon></a></div></div></div><div class="overview-actions"><el-button :icon="Connection" :loading="testing" :disabled="active" @click="testConnection">测试连接</el-button><el-button :icon="Edit" :disabled="active || dirty" @click="openSource(selected)">编辑</el-button><el-button :icon="Delete" :disabled="active" aria-label="删除同步源" @click="removeSource" /></div></div>
          <div class="sync-route"><div><span class="route-icon"><Connection /></span><section><small>资产来源</small><strong>{{ isCloud ? `${providerInfo(selected).label}实例` : 'JumpServer 节点' }}</strong></section></div><span class="route-connector"><span />自动映射<el-icon><ArrowRight /></el-icon></span><div><span class="route-icon teal"><FolderOpened /></span><section><small>本地归属</small><strong>主机与主机组</strong></section></div><span class="route-rule"><el-icon><Check /></el-icon> 保留手工分组与已有凭据</span></div>
          <div v-if="!isCloud" class="overview-meta"><span>组织 <code>{{ selected.orgId }}</code></span><span>API 凭据 <b>{{ credentialName(selected.apiCredentialId) }}</b></span></div>
          <div v-else class="overview-meta"><span>地域 <b>{{ selected.regions?.join(' / ') }}</b></span><span>环境 <b>{{ environmentName(selected.environment) }}</b></span><span>连接 <b>{{ selected.connectionMode === 'gateway' ? (gateways.find(g => g.id === selected.gatewayId)?.name || '网关') : '直连' }}</b></span></div>
          <div class="source-collection-meta"><span><Key /> {{ credentialName(selected.defaultCredentialId) }}</span><span :class="{ enabled: selected.autoCollect }"><Check /> {{ selected.autoCollect ? '同步后自动检测并采集' : '仅同步资产信息' }}</span></div>
        </section>
        <section class="sync-panel sync-content"><div class="content-heading"><el-tabs v-model="tab" class="sync-tabs"><el-tab-pane label="资产预览" name="assets" /><el-tab-pane label="分组与凭据" name="rules" /><el-tab-pane label="同步记录" name="history" /></el-tabs><el-button v-if="tab !== 'history'" type="primary" :icon="Refresh" :loading="active" :disabled="active || dirty" @click="preview">生成同步预览</el-button></div>
          <div v-if="dirty" class="sync-notice warning"><el-icon><Warning /></el-icon>分组配置有未保存的更改，请保存后重新生成预览。</div>
          <template v-if="tab === 'assets'"><div v-if="showHistoryDetails" class="sync-notice history-notice">正在查看完整同步记录，已同步资产仅在这里展示。<el-button link type="primary" @click="leaveHistoryDetails">返回待同步视图</el-button></div><div v-if="stalePreview" class="sync-notice warning">同步配置已变化，请重新生成预览后再确认。</div>
            <template v-if="currentRun">
              <div class="run-banner" :class="currentRun.status"><el-tag :type="statusType(currentRun.status)">{{ statuses[currentRun.status] }}</el-tag><span>{{ currentRun.message || '正在读取远端分组与主机，请稍候…' }}</span><small>#{{ currentRun.id }} · {{ date(currentRun.createTime) }}</small></div>
              <div v-if="currentRun.inventoryStatus === 'success'" class="collection-summary">
                <div class="inventory-stage"><span class="stage-check"><Check /></span><div><strong>资产入库完成</strong><small>新增 {{ currentRun.added }} · 更新 {{ currentRun.updated }} · 冲突 {{ currentRun.conflicts }}</small></div></div>
                <el-icon class="stage-arrow"><ArrowRight /></el-icon>
                <div class="collection-stage"><span class="stage-check" :class="{ warning: currentRun.collectStatus === 'partial', running: currentRun.status === 'collecting' }"><Monitor /></span><div><strong>{{ currentRun.status === 'collecting' ? '正在检测并采集' : currentRun.collectStatus === 'disabled' ? '未启用自动采集' : '主机信息采集' }}</strong><small v-if="currentRun.collectTotal">成功 <b>{{ currentRun.collectSuccess }}</b> / {{ currentRun.collectTotal }} 台 · 失败 {{ currentRun.collectFailed }} · 跳过 {{ currentRun.collectSkipped }}</small><small v-else>可在主机管理中按需采集</small></div></div>
                <el-button v-if="canRetryCollection" type="primary" plain :icon="Refresh" @click="retryCollection">重试未成功采集</el-button>
              </div>
              <div class="sync-stats" :class="{ pending: !showHistoryDetails }"><button v-for="counter in counters" :key="counter.key" :class="[counter.key, { chosen: action === counter.key }]" type="button" @click="action = action === counter.key ? '' : counter.key"><span>{{ counter.label }}</span><strong>{{ counter.count }}</strong><i /></button></div>
            </template>
            <div v-if="displayedItems.length" class="preview-layout"><aside class="preview-tree"><button type="button" class="all-nodes" :class="{ chosen: !nodeFilter }" @click="nodeFilter = ''"><el-icon><FolderOpened /></el-icon> 全部节点 <span>{{ displayedItems.length }}</span></button><el-tree :data="treeData" node-key="id" :props="{ label: 'name' }" default-expand-all highlight-current :expand-on-click-node="false" @node-click="nodeFilter = $event.id" /></aside><div class="preview-table"><div class="preview-toolbar"><el-input v-model="keyword" placeholder="搜索主机名称 / 地址" :prefix-icon="Search" clearable /><el-select v-model="action" placeholder="全部变更" clearable><el-option v-for="(label, key) in actionLabels" :key="key" :label="label" :value="key" /></el-select></div>
                <el-table :data="visibleItems" row-key="id"><el-table-column label="主机" min-width="160"><template #default="{ row }"><div class="host-cell"><el-icon><Monitor /></el-icon><div><strong>{{ row.name }}</strong><code>{{ row.address }}:{{ row.port || '—' }}</code><el-tooltip v-if="row.collectStatus" :content="row.collectMessage" placement="top"><span class="collect-result" :class="row.collectStatus">{{ collectionLabels[row.collectStatus] || row.collectStatus }}</span></el-tooltip></div></div></template></el-table-column><el-table-column label="目标分组" min-width="180"><template #default="{ row }"><div v-for="(path, i) in row.groupPaths" :key="i" class="group-path">{{ targetGroupPath(path, row) }}</div><span v-if="!row.groupPaths?.length" class="muted">同步源根分组</span></template></el-table-column><el-table-column label="登录凭据" min-width="130"><template #default="{ row }"><span :class="{ 'pending-text': !effectiveCredentialId(row) }">{{ credentialName(effectiveCredentialId(row)) }}</span></template></el-table-column><el-table-column label="变更" width="80"><template #default="{ row }"><el-tag :title="row.reason" :type="row.action === 'conflict' ? 'warning' : row.action === 'add' ? 'success' : row.action === 'update' ? 'primary' : 'info'" effect="light">{{ currentRun?.inventoryStatus === 'success' && ['add', 'update'].includes(row.action) ? '已' : '' }}{{ labels[row.action] }}</el-tag></template></el-table-column><el-table-column v-if="currentRun?.inventoryStatus !== 'success'" label="处理" width="100" fixed="right"><template #default="{ row }"><el-button v-if="row.action === 'conflict' && currentRun?.status === 'ready' && !showHistoryDetails" link type="primary" @click="openConflict(row)">{{ links[row.id] ? '已选关联' : '处理冲突' }}</el-button><el-tooltip v-else :content="row.reason" placement="top"><span class="row-reason">{{ row.action === 'conflict' ? '已跳过' : row.action === 'missing' ? '本地保留' : row.action === 'skip' ? '查看原因' : '自动处理' }}</span></el-tooltip></template></el-table-column></el-table>
                <div class="table-pager"><span>共 {{ filteredItems.length }} 台 · {{ currentRun?.inventoryStatus === 'success' ? '资产已入库，采集失败可直接重试' : '预览不会修改本地资产' }}</span><el-pagination v-model:current-page="page" :page-size="15" :total="filteredItems.length" layout="prev, pager, next" /></div>
              </div></div>
            <el-empty v-else :description="active ? '正在读取资产，请稍候' : currentRun?.status === 'failed' ? '预览失败，请查看上方原因后重试' : currentRun && items.length ? '资产已同步，暂无待处理变更' : currentRun ? '当前同步范围内没有主机' : '连接同步源后，生成预览以查看资产变更'" :image-size="100"><el-button v-if="!active" plain type="primary" :disabled="dirty" @click="preview">生成同步预览</el-button></el-empty>
            <div v-if="currentRun?.status === 'ready' && !showHistoryDetails && pendingItems.length" class="confirm-bar"><div><strong>{{ displayedItems.length }} 条资产记录</strong><span>{{ pendingCredentialCount }} 台待配置凭据 · {{ unresolved }} 台未处理冲突</span></div><el-button type="primary" :icon="Check" :disabled="active || dirty || stalePreview || !pendingItems.some(i => !['skip', 'conflict'].includes(i.action) || links[i.id])" @click="apply">确认同步</el-button></div>
          </template>
          <div v-else-if="tab === 'rules'" class="rules-content"><div class="rules-intro"><h3>节点决定分组，凭据按需继承</h3><p>自动保留节点层级；也可以把指定节点映射到已有主机组。新主机优先使用最深节点的登录凭据。</p></div><div class="rule-defaults"><el-form-item label="目标根主机组"><el-tree-select :model-value="rootGroupId || undefined" @update:model-value="rootGroupId = $event || 0" :data="groupTree" node-key="id" :props="{ label: 'name' }" check-strictly clearable placeholder="在顶层创建独立同步分组" :disabled="active" /></el-form-item><el-form-item label="默认登录凭据"><el-select :model-value="defaultCredentialId || undefined" @update:model-value="defaultCredentialId = $event || 0" filterable clearable placeholder="暂不绑定，导入后再配置" :disabled="active"><el-option v-for="c in sshCredentials" :key="c.id" :label="c.name" :value="c.id" /></el-select></el-form-item></div>
            <div class="sync-notice"><el-icon><Key /></el-icon>已有主机的登录凭据始终保留；未绑定凭据的新主机也可导入，稍后在主机管理中配置。</div>
            <div class="rules-layout"><aside class="scope-panel"><h4>同步范围</h4><el-radio-group v-model="scopeAll" :disabled="active"><el-radio :value="true">全部节点</el-radio><el-radio :value="false">指定节点及子节点</el-radio></el-radio-group><el-tree v-if="!scopeAll && nodes.length" ref="scopeTree" :data="treeData" node-key="id" :props="{ label: 'name', disabled: () => active }" show-checkbox default-expand-all :default-checked-keys="selectedNodes" @check="(_, data) => selectedNodes = data.checkedKeys" /><p v-if="scopeAll" class="muted">新增的远端节点会在下次预览时自动纳入。</p><el-button v-if="!nodes.length" :icon="Connection" :loading="testing" :disabled="active" @click="testConnection">读取节点树</el-button></aside><div class="rule-mapping"><div class="mapping-heading"><h4>节点映射</h4><el-input v-model="nodeKeyword" :prefix-icon="Search" placeholder="搜索节点" clearable /></div><el-table :data="visibleRuleNodes"><el-table-column :label="isCloud ? '云地域' : 'JumpServer 节点'" min-width="190"><template #default="{ row }"><strong>{{ row.name }}</strong><small class="rule-path">{{ row.full_value || row.key }}</small></template></el-table-column><el-table-column label="本地主机组" min-width="210"><template #default="{ row }"><el-tree-select :model-value="ruleDraft.find(r => r.nodeId === row.id)?.groupId || undefined" :data="groupTree" node-key="id" :props="{ label: 'name' }" check-strictly clearable filterable placeholder="自动创建对应主机组" :disabled="active" @update:model-value="setRule(row.id, 'groupId', $event)" /></template></el-table-column><el-table-column label="登录凭据" min-width="200"><template #default="{ row }"><el-select :model-value="ruleDraft.find(r => r.nodeId === row.id)?.credentialId || undefined" clearable filterable placeholder="继承父节点 / 默认凭据" :disabled="active" @update:model-value="setRule(row.id, 'credentialId', $event)"><el-option v-for="c in sshCredentials" :key="c.id" :label="c.name" :value="c.id" /></el-select></template></el-table-column></el-table><div class="table-pager"><span>同一主机属于多个节点时，会关联多个组。</span><el-pagination v-model:current-page="rulePage" :page-size="10" :total="ruleNodes.length" layout="prev, pager, next" /></div></div></div>
            <footer class="rules-footer"><span>默认保留源端已删除的本地资产，避免同步导致误删。</span><el-button :disabled="!dirty || active" @click="resetDraft">放弃更改</el-button><el-button type="primary" :disabled="!dirty || active" :loading="saving" @click="saveRules">保存规则</el-button></footer>
          </div>
          <div v-else class="history-content"><div class="history-heading"><h3>最近同步记录</h3><span>显示最近 50 次预览及执行结果</span></div><el-table :data="runs"><el-table-column label="任务" width="95"><template #default="{ row }">#{{ row.id }}</template></el-table-column><el-table-column label="状态" width="120"><template #default="{ row }"><el-tag :type="statusType(row.status)">{{ statuses[row.status] }}</el-tag></template></el-table-column><el-table-column label="开始时间" min-width="180"><template #default="{ row }">{{ date(row.createTime) }}</template></el-table-column><el-table-column label="结果" min-width="240"><template #default="{ row }">新增 {{ row.added }} · 更新 {{ row.updated }} · 冲突 {{ row.conflicts }}<small v-if="row.collectTotal" class="rule-path">采集成功 {{ row.collectSuccess }} / {{ row.collectTotal }} · 失败 {{ row.collectFailed }} · 跳过 {{ row.collectSkipped }}</small></template></el-table-column><el-table-column prop="message" label="说明" min-width="250" /><el-table-column width="90"><template #default="{ row }"><el-button link type="primary" :disabled="active" @click="viewHistory(row.id)">查看</el-button></template></el-table-column></el-table></div>
        </section>
      </main>
      <section v-else class="sync-welcome sync-panel"><span class="welcome-icon"><Connection /></span><h2>把分散的资产接入运维工作台</h2><p>云主机与堡垒机统一同步，自动分组、检测与采集。</p><div class="welcome-steps"><span><Connection /> 选择同步源</span><ArrowRight /><span><FolderOpened /> 配置分组</span><ArrowRight /><span><Check /> 预览并同步</span></div><el-button type="primary" :icon="Plus" @click="openSource()">新增同步源</el-button><small>云平台使用已配置的云账号，主机登录凭据统一从凭据管理选择。</small></section>
    </div>
    <el-dialog v-model="conflictDialog" title="处理资产冲突" width="min(560px, calc(100vw - 28px))">
      <template v-if="conflictItem">
        <div class="conflict-summary"><strong>{{ conflictItem.name }}</strong><code>{{ conflictItem.address }}</code><p>{{ conflictItem.reason }}</p></div>
        <el-form label-position="top"><el-form-item label="关联已有主机"><el-select v-model="conflictTarget" remote filterable clearable :remote-method="searchHosts" :loading="hostLoading" placeholder="搜索主机名称或地址，不选则跳过"><el-option v-for="host in conflictOptions" :key="host.id" :value="host.id" :label="`${host.hostName} · ${host.sshIp || host.privateIp}`" /></el-select></el-form-item></el-form>
        <div class="sync-notice"><el-icon><Key /></el-icon>关联后保留已有主机的登录凭据，确认整批同步时才会写入资产。</div>
      </template>
      <template #footer><el-button @click="conflictDialog = false">取消</el-button><el-button type="primary" @click="confirmConflict">确认关联</el-button></template>
    </el-dialog>
    <el-dialog v-model="dialog" :title="form.id ? '编辑同步源' : '新增同步源'" width="min(760px, calc(100vw - 28px))" class="sync-source-dialog" :close-on-click-modal="false">
      <div class="source-provider-picker" role="group" aria-label="同步源类型"><button v-for="provider in providerOptions" :key="provider.value" type="button" :class="{ chosen: form.provider === provider.value }" :disabled="!!form.id" :aria-pressed="form.provider === provider.value" @click="changeProvider(provider.value)"><span class="provider-badge" :class="provider.value">{{ provider.badge }}</span><strong>{{ provider.label }}</strong><small>{{ provider.hint }}</small></button></div>
      <el-form label-position="top">
        <el-form-item label="同步源名称" required><el-input v-model="form.name" maxlength="100" placeholder="为这组同步配置取个名称" /></el-form-item>
        <template v-if="!formIsCloud">
          <el-form-item label="JumpServer 地址" required><el-input v-model="form.url" placeholder="https://jumpserver.example.com" /></el-form-item>
          <div class="source-form-grid"><el-form-item label="组织 ID" required><el-input v-model="form.orgId" placeholder="JumpServer 组织 UUID" /></el-form-item><el-form-item label="API 访问凭据" required><el-select v-model="form.apiCredentialId" filterable placeholder="选择 JumpServer API Key"><el-option v-for="c in apiCredentials" :key="c.id" :label="c.name" :value="c.id" /></el-select></el-form-item></div>
          <small class="field-hint">组织默认使用 DEFAULT；API Key 在凭据管理中维护。</small>
        </template>
        <template v-else>
          <el-form-item label="云账号" required><el-select v-model="form.cloudAccountId" :disabled="!!form.id" filterable placeholder="选择已维护的云账号" @change="changeAccount"><el-option v-for="account in formAccounts" :key="account.id" :label="account.name" :value="account.id" /></el-select><div class="field-hint">API 密钥与可用地域由云账号管理维护。<el-button link type="primary" @click="router.push('/assets/server/cloud-accounts'); dialog = false">管理云账号</el-button></div></el-form-item>
          <el-form-item label="同步地域" required><el-select v-model="form.regions" multiple filterable :disabled="!form.cloudAccountId" placeholder="选择账号已配置的地域"><el-option v-for="region in formRegions" :key="region" :label="region" :value="region" /></el-select><small class="field-hint">按地域自动建组，也可以在「分组与凭据」中映射到已有主机组。</small></el-form-item>
        </template>
        <div class="source-section-title"><Key /> 主机接入配置 <small>已有主机保留原来的登录凭据与连接配置</small></div>
        <div class="source-form-grid"><el-form-item label="默认主机登录凭据" :required="form.autoCollect"><el-select :model-value="form.defaultCredentialId || undefined" @update:model-value="form.defaultCredentialId = $event || 0" clearable filterable placeholder="选择 SSH 密码或私钥凭据"><el-option v-for="c in sshCredentials" :key="c.id" :label="`${c.name}${c.username ? ' · ' + c.username : ''}`" :value="c.id" /></el-select><div class="field-hint"><el-button link type="primary" @click="router.push('/assets/server/credentials'); dialog = false">前往凭据管理</el-button></div></el-form-item><el-form-item label="目标根主机组"><el-tree-select :model-value="form.rootGroupId || undefined" @update:model-value="form.rootGroupId = $event || 0" :data="groupTree" node-key="id" :props="{ label: 'name' }" check-strictly clearable filterable placeholder="自动创建独立同步分组" /></el-form-item></div>
        <template v-if="formIsCloud"><div class="source-form-grid"><el-form-item label="所属环境" required><el-select v-model="form.environment" filterable :loading="environmentLoading" placeholder="选择所属环境"><el-option v-for="env in environmentOptions" :key="env.code" :label="env.name" :value="env.code" /></el-select></el-form-item><el-form-item label="连接方式"><el-radio-group v-model="form.connectionMode"><el-radio value="direct">直连</el-radio><el-radio value="gateway">通过网关</el-radio></el-radio-group></el-form-item></div><el-form-item v-if="form.connectionMode === 'gateway'" label="访问网关" required><el-select :model-value="form.gatewayId || undefined" @update:model-value="form.gatewayId = $event || 0" filterable placeholder="选择访问网关"><el-option v-for="gateway in gateways" :key="gateway.id" :label="gateway.name" :value="gateway.id" /></el-select></el-form-item></template>
        <div class="auto-collect-setting"><span class="route-icon teal"><Monitor /></span><div><strong>同步后自动检测并采集</strong><p>为新增和更新的主机检测 SSH、采集配置与容量信息；失败时保留已入库资产，可直接重试。</p></div><el-switch v-model="form.autoCollect" aria-label="同步后自动检测并采集" /></div>
      </el-form><template #footer><el-button @click="dialog = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveSource">保存同步源</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped src="./asset-sync.css"></style>
