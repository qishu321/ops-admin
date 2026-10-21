<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { ArrowLeft, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { getUser, getPermissions } from '../../utils/auth'
import { listMiddleware, operateMiddleware } from '../../api/middleware'
import {
  nacosConfigKey,
  nacosConfigPayload,
  nacosConfigRows,
  nacosConfigTotal,
  nacosDefaultGroup,
  nacosFormatLabel,
  nacosFormats,
  nacosInstancePayload,
  nacosInstanceRows,
  nacosNamespaceOptions,
  nacosNamespacePayload,
  nacosPretty,
  nacosServiceIdentity,
  nacosServiceRows,
  nacosServiceTotal,
  nacosTime
} from './nacos-view'

const props = defineProps({ sourceId: Number })
const router = useRouter(), route = useRoute()
const tabs = [['configs', '配置管理'], ['services', '服务发现'], ['namespaces', '命名空间']]

const source = ref(null)
const error = ref('')
const tab = ref('configs')
const namespacesRaw = ref([])
const namespaceId = ref('public')
const revision = ref(0)

const configQuery = reactive({ dataId: '', group: nacosDefaultGroup })
const configs = ref([])
const configTotal = ref(0)
const configPage = ref(1)
const configPageSize = ref(10)
const configSelection = ref([])
const loading = ref(false), busy = ref(false)
let generation = 0, detailGeneration = 0

const serviceQuery = reactive({ serviceName: '', group: nacosDefaultGroup })
const services = ref([])
const serviceTotal = ref(0)
const servicePage = ref(1)
const servicePageSize = ref(10)

const configDrawer = ref(false)
const configRow = ref(null)
const configContent = ref('')
const configDetailLoading = ref(false)
const formatted = ref(false)

const editorDialog = ref(false)
const editorMode = ref('create')
const editorLoading = ref(false)
const editorForm = reactive({ dataId: '', groupName: nacosDefaultGroup, namespaceId: '', type: 'text', appName: '', desc: '', content: '' })

const namespaceDialog = ref(false)
const namespaceForm = reactive({ namespaceId: '', namespaceName: '', namespaceDesc: '' })

const serviceDrawer = ref(false)
const service = ref(null)
const instances = ref([])
const instancesLoading = ref(false)

const can = action => { revision.value; return getUser().accountType === 'administrator' || getPermissions().includes(`assets:middleware:${action}`) }
const canRead = computed(() => can('query'))
const valid = computed(() => Number.isSafeInteger(props.sourceId) && props.sourceId > 0)
const namespaceOptions = computed(() => nacosNamespaceOptions(namespacesRaw.value))
// 2.x 用空 tenant 表示 public，3.x 直接用 public，请求前统一转换。
const effectiveNamespace = computed(() => {
  if (namespaceId.value === 'public' && source.value?.version !== '3') return ''
  return namespaceId.value
})
const namespaceLabel = computed(() => {
  const option = namespaceOptions.value.find(item => item.id === namespaceId.value)
  return option ? option.name : (namespaceId.value || 'public')
})
const serviceNamespaceLabel = computed(() => {
  const id = service.value?.namespaceId || 'public'
  const option = namespaceOptions.value.find(item => item.id === id)
  return option ? option.name : id
})
const shownContent = computed(() => formatted.value ? nacosPretty(configContent.value, 'json').value : configContent.value)
const tableLoading = computed(() => loading.value)

function api(operation, params = {}, action = 'query') {
  return operateMiddleware(props.sourceId, operation, params, action, action !== 'query', 20000, true)
}

onMounted(async () => {
  window.addEventListener('ops-admin-permissions-changed', permissionsChanged)
  if (!valid.value) return
  const initialTab = String(route.query.tab || '')
  if (tabs.some(([key]) => key === initialTab)) tab.value = initialTab
  if (route.query.namespaceId !== undefined) namespaceId.value = String(route.query.namespaceId || 'public')
  if (route.query.dataId) configQuery.dataId = String(route.query.dataId)
  if (route.query.group) configQuery.group = String(route.query.group)
  await loadSource()
  if (!source.value || !canRead.value) return
  await loadNamespaces()
  await reload()
})
onUnmounted(() => {
  generation++; detailGeneration++
  window.removeEventListener('ops-admin-permissions-changed', permissionsChanged)
})

async function permissionsChanged() {
  revision.value++
  error.value = ''
  if (!canRead.value) {
    configs.value = []; services.value = []
    return
  }
  await loadNamespaces()
  await reload()
}

async function loadSource() {
  try {
    const rows = await listMiddleware('nacos')
    source.value = (rows || []).find(item => item.id === props.sourceId) || null
    if (!source.value) error.value = '未找到该 Nacos 数据源，可能已被删除或当前账号无权访问。'
  } catch { source.value = null }
}

async function loadNamespaces() {
  try {
    const data = await api('namespaces')
    namespacesRaw.value = Array.isArray(data) ? data : (Array.isArray(data?.pageItems) ? data.pageItems : [])
  } catch {
    namespacesRaw.value = []
  }
}

async function reload() {
  if (!canRead.value) return
  const g = ++generation
  loading.value = true
  error.value = ''
  try {
    if (tab.value === 'configs') await loadConfigs(g)
    else if (tab.value === 'services') await loadServices(g)
    else await loadNamespaces()
  } catch (e) {
    if (g === generation) error.value = e.message || '查询失败，请检查连接与权限'
  } finally {
    if (g === generation) loading.value = false
  }
}

async function loadConfigs(g = ++generation) {
  const data = await api('configs', {
    namespaceId: effectiveNamespace.value,
    groupName: configQuery.group || nacosDefaultGroup,
    dataId: configQuery.dataId,
    pageNo: String(configPage.value),
    pageSize: String(configPageSize.value)
  })
  if (g !== generation) return
  configs.value = nacosConfigRows(data).map(row => ({
    ...row,
    group: row.group || row.groupName || configQuery.group,
    gmtModified: row.gmtModified ?? row.modifyTime ?? row.createTime
  }))
  configTotal.value = nacosConfigTotal(data, configs.value.length)
  configSelection.value = []
}

async function loadServices(g = ++generation) {
  const data = await api('services', {
    namespaceId: effectiveNamespace.value,
    groupName: serviceQuery.group || nacosDefaultGroup,
    serviceName: serviceQuery.serviceName,
    pageNo: String(servicePage.value),
    pageSize: String(servicePageSize.value)
  })
  if (g !== generation) return
  services.value = nacosServiceRows(data).map(row => ({
    ...row,
    name: row.name || row.serviceName || '',
    groupName: row.groupName || serviceQuery.group
  }))
  serviceTotal.value = nacosServiceTotal(data, services.value.length)
}

function searchConfigs() {
  configPage.value = 1
  reload()
}

function searchServices() {
  servicePage.value = 1
  reload()
}

function onNamespaceChange() {
  configPage.value = 1
  servicePage.value = 1
  configs.value = []; services.value = []; configSelection.value = []
  if (tab.value === 'namespaces') return
  reload()
}

function changeTab() {
  error.value = ''
  reload()
}

function onConfigSize() { configPage.value = 1; reload() }
function onConfigPage() { reload() }
function onServiceSize() { servicePage.value = 1; reload() }
function onServicePage() { reload() }

function onConfigSelection(rows) { configSelection.value = rows || [] }

async function openConfig(row) {
  const g = ++detailGeneration
  configRow.value = row
  configContent.value = ''
  formatted.value = false
  configDrawer.value = true
  configDetailLoading.value = true
  try {
    const text = await api('config.get', { namespaceId: effectiveNamespace.value, groupName: row.group || configQuery.group, dataId: row.dataId })
    if (g !== detailGeneration) return
    configContent.value = typeof text === 'string' ? text : (text?.content ?? JSON.stringify(text ?? ''))
  } catch (e) {
    if (g === detailGeneration) configContent.value = ''
    ElMessage.error(e.message || '读取配置内容失败')
  } finally {
    if (g === detailGeneration) configDetailLoading.value = false
  }
}

async function copyContent() {
  if (!configContent.value) return
  try {
    await navigator.clipboard.writeText(configContent.value)
    ElMessage.success('配置内容已复制')
  } catch {
    ElMessage.warning('当前浏览器不允许访问剪贴板，请手动选择内容复制')
  }
}

function toggleFormat() {
  if (formatted.value) { formatted.value = false; return }
  if (!nacosPretty(configContent.value, 'json').ok) { ElMessage.warning('当前内容不是合法 JSON，无法格式化'); return }
  formatted.value = true
}

function openCreate() {
  Object.assign(editorForm, {
    dataId: '',
    groupName: configQuery.group || nacosDefaultGroup,
    namespaceId: effectiveNamespace.value,
    type: 'text',
    appName: '',
    desc: '',
    content: ''
  })
  editorMode.value = 'create'
  editorDialog.value = true
}

async function openEdit(row) {
  Object.assign(editorForm, {
    dataId: row.dataId,
    groupName: row.group || configQuery.group,
    namespaceId: effectiveNamespace.value,
    type: String(row.type || 'text').toLowerCase(),
    appName: row.appName || '',
    desc: row.desc || '',
    content: ''
  })
  editorMode.value = 'edit'
  editorDialog.value = true
  // 打开抽屉时已经取过内容，避免重复请求。
  if (configRow.value?.dataId === row.dataId && configContent.value) {
    editorForm.content = configContent.value
    return
  }
  editorLoading.value = true
  try {
    const text = await api('config.get', { namespaceId: effectiveNamespace.value, groupName: editorForm.groupName, dataId: row.dataId })
    editorForm.content = typeof text === 'string' ? text : (text?.content ?? '')
  } catch (e) {
    ElMessage.error(e.message || '读取配置内容失败')
  } finally {
    editorLoading.value = false
  }
}

function formatEditorContent() {
  const pretty = nacosPretty(editorForm.content, 'json')
  if (!pretty.ok) { ElMessage.warning('当前内容不是合法 JSON，无法格式化'); return }
  editorForm.content = pretty.value
}

async function saveConfig() {
  const payload = nacosConfigPayload(editorForm)
  if (!payload.ok) { ElMessage.warning(payload.error); return }
  busy.value = true
  try {
    await api('config.put', payload.params, 'write')
    ElMessage.success(editorMode.value === 'create' ? '配置已发布' : '配置已更新')
    editorDialog.value = false
    await reload()
    if (configDrawer.value && configRow.value?.dataId === payload.params.dataId) {
      configRow.value = { ...configRow.value, type: payload.params.type, group: payload.params.groupName }
      configContent.value = payload.params.content
      formatted.value = false
    }
  } catch (e) {
    ElMessage.error(e.message || '发布配置失败')
  } finally {
    busy.value = false
  }
}

async function deleteConfig(row) {
  try {
    await ElMessageBox.confirm(`将删除配置 ${row.dataId}（Group ${row.group}），删除后客户端将读取不到该配置。`, '删除配置', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  busy.value = true
  try {
    await api('config.delete', { namespaceId: effectiveNamespace.value, groupName: row.group || configQuery.group, dataId: row.dataId }, 'delete')
    ElMessage.success('配置已删除')
    if (configDrawer.value && configRow.value?.dataId === row.dataId) configDrawer.value = false
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '删除配置失败')
  } finally {
    busy.value = false
  }
}

async function removeSelectedConfigs() {
  const rows = configSelection.value.slice()
  if (!rows.length) return
  const names = rows.map(row => row.dataId)
  try {
    await ElMessageBox.confirm(`将删除 ${rows.length} 个配置：${names.slice(0, 4).join('、')}${names.length > 4 ? ' 等' : ''}。删除后客户端将读取不到这些配置。`, '批量删除配置', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  busy.value = true
  const failed = []
  for (const row of rows) {
    try {
      await api('config.delete', { namespaceId: effectiveNamespace.value, groupName: row.group || configQuery.group, dataId: row.dataId }, 'delete')
    } catch (e) {
      failed.push(`${row.dataId}：${e.message}`)
    }
  }
  busy.value = false
  if (failed.length) ElMessage.warning(`已删除 ${rows.length - failed.length} 个，失败 ${failed.length} 个。首个失败：${failed[0]}`)
  else ElMessage.success(`已删除 ${rows.length} 个配置`)
  await reload()
}

async function openService(row) {
  service.value = { ...row, namespaceId: effectiveNamespace.value, groupName: row.groupName || serviceQuery.group }
  instances.value = []
  serviceDrawer.value = true
  await loadInstances()
}

async function loadInstances() {
  if (!service.value) return
  instancesLoading.value = true
  try {
    const identity = nacosServiceIdentity(service.value, effectiveNamespace.value)
    const data = await api('instances', identity)
    instances.value = nacosInstanceRows(data)
  } catch (e) {
    instances.value = []
    ElMessage.error(e.message || '读取服务实例失败')
  } finally {
    instancesLoading.value = false
  }
}

async function toggleInstance(instance, enabled) {
  const action = enabled ? '上线' : '下线'
  try {
    await ElMessageBox.confirm(`将实例 ${instance.ip}:${instance.port} ${action}，${enabled ? '恢复' : '暂停'}接收流量。`, `${action}实例`, { type: 'warning', confirmButtonText: `确认${action}`, cancelButtonText: '取消' })
  } catch { return }
  busy.value = true
  try {
    await api('instance.put', nacosInstancePayload(service.value, instance, enabled), 'write')
    ElMessage.success(`实例已${action}`)
    await loadInstances()
  } catch (e) {
    ElMessage.error(e.message || `${action}实例失败`)
  } finally {
    busy.value = false
  }
}

async function removeInstance(instance) {
  try {
    await ElMessageBox.confirm(`将从服务中移除实例 ${instance.ip}:${instance.port}，操作不可恢复。`, '移除实例', { type: 'warning', confirmButtonText: '确认移除', cancelButtonText: '取消' })
  } catch { return }
  busy.value = true
  try {
    const payload = nacosInstancePayload(service.value, instance, instance.enabled !== false)
    delete payload.enabled
    await api('instance.delete', payload, 'delete')
    ElMessage.success('实例已移除')
    await loadInstances()
  } catch (e) {
    ElMessage.error(e.message || '移除实例失败')
  } finally {
    busy.value = false
  }
}

function openNamespaceDialog() {
  Object.assign(namespaceForm, { namespaceId: '', namespaceName: '', namespaceDesc: '' })
  namespaceDialog.value = true
}

async function saveNamespace() {
  const payload = nacosNamespacePayload(namespaceForm)
  if (!payload.ok) { ElMessage.warning(payload.error); return }
  busy.value = true
  try {
    await api('namespace.put', payload.params, 'write')
    namespaceDialog.value = false
    ElMessage.success('命名空间已创建')
    await loadNamespaces()
    if (tab.value === 'namespaces') await reload()
  } catch (e) {
    ElMessage.error(e.message || '创建命名空间失败')
  } finally {
    busy.value = false
  }
}

async function deleteNamespace(row) {
  if (!row.id) return
  try {
    await ElMessageBox.confirm(`将删除命名空间 ${row.name}（${row.id}），其下的 ${row.configCount} 个配置会一并删除。`, '删除命名空间', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  busy.value = true
  try {
    await api('namespace.delete', { namespaceId: row.id }, 'delete')
    ElMessage.success('命名空间已删除')
    if (namespaceId.value === row.id) {
      namespaceId.value = 'public'
      configs.value = []
    }
    await loadNamespaces()
    if (tab.value === 'namespaces') await reload()
  } catch (e) {
    ElMessage.error(e.message || '删除命名空间失败')
  } finally {
    busy.value = false
  }
}

function back() { router.push('/assets/middleware/nacos') }
</script>

<template>
  <el-config-provider :locale="zhCn">
  <section class="nacos-page">
    <header class="nacos-head">
      <el-button link type="primary" :icon="ArrowLeft" @click="back">数据源</el-button>
      <div class="nacos-title">
        <h1>{{ source?.name || 'Nacos' }}</h1>
        <p>{{ source?.description || '统一管理配置、服务发现与命名空间，支持 Nacos 2.x / 3.x。' }}<span v-if="source?.version"> · {{ source.version }}.x</span></p>
      </div>
      <el-button :icon="Refresh" :loading="loading" :disabled="!canRead" @click="reload">刷新</el-button>
    </header>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>
    <el-alert v-if="valid && !canRead" title="当前账号只有查看数据源的权限，浏览配置与服务需要额外授权。" type="info" :closable="false"/>

    <div v-if="valid" class="nacos-main">
      <div class="nacos-toolbar">
        <template v-if="tab === 'configs'">
          <el-button type="primary" :icon="Plus" :disabled="!can('write')" @click="openCreate">新建配置</el-button>
          <el-button :disabled="!can('delete') || !configSelection.length || busy" @click="removeSelectedConfigs">批量删除<span v-if="configSelection.length">（{{ configSelection.length }}）</span></el-button>
        </template>
        <template v-else-if="tab === 'services'">
          <span class="toolbar-hint">实例总数与健康数来自 Nacos 服务目录</span>
        </template>
        <template v-else>
          <el-button type="primary" :icon="Plus" :disabled="!can('write')" @click="openNamespaceDialog">新建命名空间</el-button>
        </template>

        <div class="toolbar-right">
          <el-select v-if="tab !== 'namespaces'" v-model="namespaceId" class="namespace-select" placeholder="命名空间" filterable :disabled="!canRead" @change="onNamespaceChange">
            <el-option v-for="item in namespaceOptions" :key="item.id" :label="item.name" :value="item.id">
              <span class="namespace-option">{{ item.name }}</span>
              <span class="namespace-id">{{ item.id }}</span>
            </el-option>
          </el-select>
          <template v-if="tab === 'configs'">
            <el-input v-model="configQuery.dataId" :prefix-icon="Search" placeholder="Data ID" clearable aria-label="按 Data ID 查询" class="filter-input" @keyup.enter="searchConfigs"/>
            <el-input v-model="configQuery.group" placeholder="Group" clearable aria-label="按 Group 查询" class="filter-input group-input" @keyup.enter="searchConfigs"/>
            <el-button type="primary" :disabled="!canRead" @click="searchConfigs">查询</el-button>
          </template>
          <template v-else-if="tab === 'services'">
            <el-input v-model="serviceQuery.serviceName" :prefix-icon="Search" placeholder="服务名称" clearable aria-label="按服务名称查询" class="filter-input" @keyup.enter="searchServices"/>
            <el-input v-model="serviceQuery.group" placeholder="Group" clearable aria-label="按分组查询" class="filter-input group-input" @keyup.enter="searchServices"/>
            <el-button type="primary" :disabled="!canRead" @click="searchServices">查询</el-button>
          </template>
          <el-button v-if="tab === 'namespaces'" :icon="Refresh" :loading="loading" circle aria-label="刷新命名空间" @click="reload"/>
        </div>
      </div>

      <el-tabs v-model="tab" class="nacos-tabs" @tab-change="changeTab">
        <el-tab-pane v-for="[key, label] in tabs" :key="key" :label="label" :name="key"/>
      </el-tabs>

      <div v-loading="tableLoading" class="nacos-results">
        <template v-if="tab === 'configs'">
          <el-table :data="configs" :row-key="nacosConfigKey" empty-text="暂无配置，可调整命名空间或查询条件" @selection-change="onConfigSelection">
            <el-table-column type="selection" width="46"/>
            <el-table-column label="Data ID" min-width="260">
              <template #default="{ row }"><el-button link type="primary" class="resource-name" @click="openConfig(row)">{{ row.dataId }}</el-button></template>
            </el-table-column>
            <el-table-column label="Group" min-width="160"><template #default="{ row }">{{ row.group || row.groupName || '—' }}</template></el-table-column>
            <el-table-column label="格式" width="120"><template #default="{ row }"><el-tag size="small" effect="plain">{{ nacosFormatLabel(row.type) }}</el-tag></template></el-table-column>
            <el-table-column label="应用" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ row.appName || '—' }}</template></el-table-column>
            <el-table-column label="更新时间" width="190"><template #default="{ row }">{{ nacosTime(row.gmtModified) }}</template></el-table-column>
            <el-table-column label="操作" width="190" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openConfig(row)">查看</el-button>
                <el-button link type="primary" :disabled="!can('write')" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" :disabled="!can('delete')" @click="deleteConfig(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div class="table-foot">
            <span class="result-note">共 {{ configTotal }} 条</span>
            <el-pagination v-model:current-page="configPage" v-model:page-size="configPageSize" background :page-sizes="[10, 20, 50]" :total="configTotal" layout="sizes, prev, pager, next" @current-change="onConfigPage" @size-change="onConfigSize"/>
          </div>
        </template>

        <template v-else-if="tab === 'services'">
          <el-table :data="services" row-key="name" empty-text="暂无服务，请检查命名空间与分组">
            <el-table-column label="服务名称" min-width="260"><template #default="{ row }"><el-button link type="primary" class="resource-name" @click="openService(row)">{{ row.name }}</el-button></template></el-table-column>
            <el-table-column label="分组" min-width="160"><template #default="{ row }">{{ row.groupName || '—' }}</template></el-table-column>
            <el-table-column label="实例数" width="120" align="right"><template #default="{ row }">{{ row.ipCount ?? row.instanceCount ?? '—' }}</template></el-table-column>
            <el-table-column label="健康实例" width="120" align="right"><template #default="{ row }"><span :class="{ 'health-warn': Number(row.healthyInstanceCount ?? 0) === 0 }">{{ row.healthyInstanceCount ?? '—' }}</span></template></el-table-column>
            <el-table-column label="集群数" width="110" align="right"><template #default="{ row }">{{ row.clusterCount ?? '—' }}</template></el-table-column>
            <el-table-column label="操作" width="130" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openService(row)">查看实例</el-button></template></el-table-column>
          </el-table>
          <div class="table-foot">
            <span class="result-note">共 {{ serviceTotal }} 条</span>
            <el-pagination v-model:current-page="servicePage" v-model:page-size="servicePageSize" background :page-sizes="[10, 20, 50]" :total="serviceTotal" layout="sizes, prev, pager, next" @current-change="onServicePage" @size-change="onServiceSize"/>
          </div>
        </template>

        <template v-else>
          <el-table :data="namespaceOptions" :row-key="row => row.id" empty-text="暂无命名空间">
            <el-table-column label="命名空间 ID" min-width="220"><template #default="{ row }"><span class="mono">{{ row.id }}</span><el-tag v-if="row.id === 'public'" size="small" type="info" effect="plain" class="inline-tag">默认</el-tag></template></el-table-column>
            <el-table-column prop="name" label="名称" min-width="180"/>
            <el-table-column label="描述" min-width="240" show-overflow-tooltip><template #default="{ row }">{{ row.desc || '—' }}</template></el-table-column>
            <el-table-column label="配置数量" width="120" align="right"><template #default="{ row }">{{ row.configCount }}</template></el-table-column>
            <el-table-column label="操作" width="120" fixed="right"><template #default="{ row }"><el-button link type="danger" :disabled="!can('delete') || row.id === 'public'" @click="deleteNamespace(row)">删除</el-button></template></el-table-column>
          </el-table>
          <p class="result-note">命名空间用于隔离配置与服务；public 为默认命名空间，不可删除。</p>
        </template>
      </div>
    </div>

    <div v-else class="nacos-placeholder">请先选择 Nacos 数据源</div>

    <el-drawer v-model="configDrawer" :title="configRow?.dataId || '配置详情'" size="min(860px, 96vw)" class="nacos-drawer">
      <div v-loading="configDetailLoading" class="drawer-body">
        <div class="drawer-tags">
          <el-tag size="small" type="info" effect="plain">{{ configRow?.group || '—' }}</el-tag>
          <el-tag size="small" effect="plain">{{ nacosFormatLabel(configRow?.type) }}</el-tag>
          <el-tag v-if="configRow?.appName" size="small" type="success" effect="plain">{{ configRow.appName }}</el-tag>
          <span class="drawer-meta">命名空间 {{ namespaceLabel }} · 更新于 {{ nacosTime(configRow?.gmtModified) }}</span>
        </div>
        <div class="content-block">
          <pre>{{ shownContent || '（空配置）' }}</pre>
          <div class="content-foot">
            <span>{{ formatted ? '已格式化为缩进 JSON' : '' }}</span>
            <div class="content-actions">
              <el-button size="small" :disabled="!configContent" @click="copyContent">复制</el-button>
              <el-button size="small" :disabled="!configContent" @click="toggleFormat">{{ formatted ? '还原' : '格式化' }}</el-button>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="drawer-foot">
          <el-button :disabled="configDetailLoading" @click="configDrawer = false">关闭</el-button>
          <div class="drawer-actions">
            <el-button type="primary" :disabled="!can('write') || !configRow" @click="openEdit(configRow)">编辑配置</el-button>
            <el-button type="danger" plain :disabled="!can('delete') || !configRow" @click="deleteConfig(configRow)">删除配置</el-button>
          </div>
        </div>
      </template>
    </el-drawer>

    <el-dialog v-model="editorDialog" :title="editorMode === 'create' ? '新建配置' : `编辑配置 · ${editorForm.dataId}`" width="min(880px, 94vw)" top="5vh" :close-on-click-modal="false" class="nacos-dialog">
      <el-form v-loading="editorLoading" label-position="top" :disabled="busy">
        <div class="form-columns">
          <el-form-item label="Data ID" required><el-input v-model="editorForm.dataId" :disabled="editorMode === 'edit'" placeholder="例如：order-service.yaml" maxlength="255"/></el-form-item>
          <el-form-item label="Group" required><el-input v-model="editorForm.groupName" placeholder="DEFAULT_GROUP" maxlength="128"/></el-form-item>
        </div>
        <div class="form-columns">
          <el-form-item label="格式"><el-select v-model="editorForm.type"><el-option v-for="[value, label] in nacosFormats" :key="value" :label="label" :value="value"/></el-select></el-form-item>
          <el-form-item label="应用名"><el-input v-model="editorForm.appName" placeholder="可选，便于在控制台筛选" maxlength="128"/></el-form-item>
        </div>
        <el-form-item label="描述"><el-input v-model="editorForm.desc" placeholder="可选，说明本次配置用途" maxlength="255"/></el-form-item>
        <el-form-item label="配置内容">
          <el-input v-model="editorForm.content" type="textarea" :rows="14" spellcheck="false" class="mono-input" placeholder="配置内容，支持 TEXT / JSON / XML / YAML / HTML / PROPERTIES"/>
        </el-form-item>
      </el-form>
      <div class="editor-foot">
        <span class="dialog-note">发布后客户端会收到变更通知；格式选择 JSON 时可直接格式化内容。</span>
        <el-button size="small" :disabled="busy || editorLoading" @click="formatEditorContent">格式化 JSON</el-button>
      </div>
      <template #footer>
        <el-button :disabled="busy" @click="editorDialog = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="saveConfig">{{ editorMode === 'create' ? '发布配置' : '保存配置' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="namespaceDialog" title="新建命名空间" width="min(560px, 94vw)" :close-on-click-modal="false" class="nacos-dialog">
      <el-form label-position="top" :disabled="busy">
        <el-form-item label="命名空间 ID"><el-input v-model="namespaceForm.namespaceId" placeholder="可选，留空由服务端生成；仅支持字母、数字、- _ ." maxlength="128"/></el-form-item>
        <el-form-item label="名称" required><el-input v-model="namespaceForm.namespaceName" placeholder="例如：预发布环境" maxlength="128"/></el-form-item>
        <el-form-item label="描述"><el-input v-model="namespaceForm.namespaceDesc" type="textarea" :rows="3" placeholder="可选" maxlength="255"/></el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="busy" @click="namespaceDialog = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="saveNamespace">创建</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="serviceDrawer" :title="service?.name || '服务实例'" size="min(880px, 96vw)" class="nacos-drawer">
      <div class="drawer-body">
        <div class="drawer-tags">
          <el-tag size="small" type="info" effect="plain">{{ service?.groupName || nacosDefaultGroup }}</el-tag>
          <el-tag size="small" effect="plain">命名空间 {{ serviceNamespaceLabel }}</el-tag>
          <span class="drawer-meta">共 {{ instances.length }} 个实例 · 健康 {{ instances.filter(item => item.healthy).length }} 个</span>
          <el-button size="small" :icon="Refresh" :loading="instancesLoading" @click="loadInstances">刷新实例</el-button>
        </div>
        <el-table v-loading="instancesLoading" :data="instances" empty-text="该服务暂无实例">
          <el-table-column label="IP" min-width="170"><template #default="{ row }"><span class="mono">{{ row.ip }}</span></template></el-table-column>
          <el-table-column prop="port" label="端口" width="90" align="right"/>
          <el-table-column label="集群" width="120"><template #default="{ row }">{{ row.clusterName || 'DEFAULT' }}</template></el-table-column>
          <el-table-column label="健康" width="100"><template #default="{ row }"><el-tag size="small" :type="row.healthy ? 'success' : 'danger'" effect="light">{{ row.healthy ? '健康' : '异常' }}</el-tag></template></el-table-column>
          <el-table-column label="启用" width="90"><template #default="{ row }"><el-switch :model-value="row.enabled !== false" :disabled="!can('write') || busy" size="small" @change="value => toggleInstance(row, value)"/></template></el-table-column>
          <el-table-column label="权重" width="90" align="right"><template #default="{ row }">{{ row.weight ?? 1 }}</template></el-table-column>
          <el-table-column label="临时实例" width="110"><template #default="{ row }">{{ row.ephemeral === false ? '否' : '是' }}</template></el-table-column>
          <el-table-column label="操作" width="110" fixed="right"><template #default="{ row }"><el-button link type="danger" :disabled="!can('delete') || busy" @click="removeInstance(row)">移除</el-button></template></el-table-column>
        </el-table>
        <p class="result-note">实例上下线通过更新 enabled 与权重生效；临时实例由客户端心跳维持，可在客户端下线后自动摘除。</p>
      </div>
    </el-drawer>
  </section>
  </el-config-provider>
</template>

<style scoped>
.nacos-page{display:flex;flex-direction:column;gap:14px;height:calc(100vh - 190px);min-height:560px;color:var(--el-text-color-primary,#23364f)}
.nacos-head{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:16px 20px;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px}
.nacos-title{margin-right:auto;min-width:0}
.nacos-title h1{margin:0;font-size:22px}
.nacos-title p{margin:6px 0 0;font-size:14px;color:var(--el-text-color-secondary,#8795aa);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.nacos-main{display:flex;flex-direction:column;flex:1;min-height:0;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px;padding:16px 20px 18px;overflow:hidden}
.nacos-toolbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;min-height:40px}
.toolbar-label{font-size:15px;font-weight:600;color:#41546d}
.toolbar-hint{font-size:13.5px;color:#8b98ab}
.toolbar-right{display:flex;align-items:center;gap:8px;margin-left:auto;flex-wrap:wrap}
.toolbar-right :deep(.el-input__inner),.toolbar-right :deep(.el-select__placeholder){font-size:15px}
.namespace-select{width:190px}
.filter-input{width:210px}
.group-input{width:170px}
.nacos-tabs :deep(.el-tabs__header){margin:8px 0 4px}
.nacos-tabs :deep(.el-tabs__item){font-size:15px;height:44px}
.nacos-results{flex:1;min-height:0;overflow:auto;padding-top:2px}
.nacos-results :deep(.el-table th){font-size:13.5px;color:#7b889d;font-weight:600}
.nacos-results :deep(.el-table td){font-size:14.5px}
.resource-name{font-size:15px;font-weight:550}
.health-warn{color:#d06a3f;font-weight:600}
.table-foot{display:flex;align-items:center;gap:16px;flex-wrap:wrap;margin-top:14px}
.table-foot .result-note{margin:0;margin-right:auto}
.result-note{font-size:13.5px;color:#8795aa;margin:12px 0 0;line-height:1.7}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13.5px}
.inline-tag{margin-left:8px}
.namespace-option{margin-right:12px}
.namespace-id{font-size:12px;color:#9aa7ba;font-family:ui-monospace,monospace}
.nacos-placeholder{display:grid;place-items:center;flex:1;border:1px dashed var(--el-border-color,#dfe6f1);border-radius:12px;background:var(--el-bg-color,#fff);font-size:15px;color:#8795aa;min-height:320px}
.drawer-body{min-height:200px}
.drawer-tags{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:16px}
.drawer-meta{font-size:13px;color:#8795aa;margin-left:4px}
.content-block{border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:9px;overflow:hidden}
.content-block pre{margin:0;padding:16px 18px;max-height:calc(100vh - 380px);min-height:180px;overflow:auto;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14.5px;line-height:1.75;color:#2d3f5c;white-space:pre-wrap;overflow-wrap:anywhere;background:var(--el-fill-color-blank,#fff)}
.content-foot{display:flex;align-items:center;justify-content:space-between;gap:12px;min-height:46px;padding:0 10px 0 16px;border-top:1px solid var(--el-border-color-lighter,#eef2f8);font-size:13.5px;color:#8795aa}
.content-actions{display:flex;gap:8px}
.drawer-foot{display:flex;align-items:center;gap:10px}
.drawer-actions{display:flex;gap:8px;margin-left:auto}
.form-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 16px}
.mono-input :deep(textarea){font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14px;line-height:1.7}
.editor-foot{display:flex;align-items:center;gap:12px;justify-content:space-between;flex-wrap:wrap}
.dialog-note{font-size:13px;color:#8795aa;line-height:1.7;margin:0}
@media (max-width:1080px){
  .nacos-page{height:auto}
  .toolbar-right{margin-left:0;width:100%}
  .namespace-select,.filter-input,.group-input{width:100%;max-width:none}
  .form-columns{grid-template-columns:1fr}
}
</style>
