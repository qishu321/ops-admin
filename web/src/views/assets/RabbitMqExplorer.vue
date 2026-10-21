<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { getUser, getPermissions } from '../../utils/auth'
import { listMiddleware, operateMiddleware } from '../../api/middleware'
import {
  rabbitAllVhosts,
  rabbitArguments,
  rabbitBindings,
  rabbitBytes,
  rabbitConnectionState,
  rabbitCount,
  rabbitExchangePayload,
  rabbitExchangeTypes,
  rabbitQueueMessages,
  rabbitQueuePayload,
  rabbitQueueRows,
  rabbitQueueState,
  rabbitQueueTypeOptions,
  rabbitRate,
  rabbitVhostRows
} from './rabbitmq-view'

const props = defineProps({ sourceId: Number })
const router = useRouter(), route = useRoute()
const tabs = [['overview', '实例概览'], ['queues', '队列'], ['exchanges', '交换机'], ['connections', '连接'], ['consumers', '消费者']]

const source = ref(null)
const error = ref('')
const tab = ref('overview')
const vhosts = ref([])
const vhost = ref(rabbitAllVhosts)
const vhostKeyword = ref('')
const queueKeyword = ref('')
const overview = ref(null)
const queues = ref([])
const exchanges = ref([])
const connections = ref([])
const consumers = ref([])
const selection = ref([])
const loading = ref(false), refreshing = ref(false), actionBusy = ref(false)
const revision = ref(0)
let generation = 0, drawerGeneration = 0

const queueDialog = ref(false)
const exchangeDialog = ref(false)
const queueForm = reactive({ name: '', vhost: '/', durable: true, autoDelete: false, queueType: '', arguments: '' })
const exchangeForm = reactive({ name: '', vhost: '/', type: 'direct', durable: true, autoDelete: false, arguments: '' })

const drawer = ref(false)
const drawerKind = ref('queue')
const queue = ref(null)
const exchange = ref(null)
const bindings = ref([])
const detailLoading = ref(false), detailError = ref('')

const can = action => { revision.value; return getUser().accountType === 'administrator' || getPermissions().includes(`assets:middleware:${action}`) }
const canRead = computed(() => can('query'))
const valid = computed(() => Number.isSafeInteger(props.sourceId) && props.sourceId > 0)
const tabLabel = computed(() => (tabs.find(([key]) => key === tab.value) || ['', ''])[1])
const filteredVhosts = computed(() => rabbitVhostRows(vhosts.value, vhostKeyword.value))
const filteredQueues = computed(() => rabbitQueueRows(queues.value, queueKeyword.value))
const vhostTotal = computed(() => vhosts.value.reduce((sum, item) => sum + Number(item?.messages || 0), 0))

const overviewCards = computed(() => {
  const o = overview.value || {}
  const totals = o.object_totals || {}
  const q = o.queue_totals || {}
  return [
    { label: 'RabbitMQ 版本', value: o.rabbitmq_version || '—', hint: o.management_version ? `管理插件 ${o.management_version}` : '' },
    { label: '集群名称', value: o.cluster_name || '—', hint: o.node ? `节点 ${o.node}` : '' },
    { label: '队列 / 交换机', value: `${rabbitCount(totals.queues)} / ${rabbitCount(totals.exchanges)}`, hint: `连接 ${rabbitCount(totals.connections)} · 消费者 ${rabbitCount(totals.consumers)}` },
    { label: '待消费消息', value: rabbitCount(q.messages_ready), hint: `未确认 ${rabbitCount(q.messages_unacknowledged)} · 总消息 ${rabbitCount(q.messages)}` },
    { label: '投递速率', value: rabbitRate(o.message_stats?.deliver_get_details?.rate), hint: `发布 ${rabbitRate(o.message_stats?.publish_details?.rate)}` },
    { label: '磁盘 / 内存', value: rabbitBytes(o.disk_free), hint: o.mem_used ? `内存占用 ${rabbitBytes(o.mem_used)}` : '' }
  ]
})

const queueMeta = computed(() => {
  const item = queue.value
  if (!item) return []
  const counts = rabbitQueueMessages(item)
  return [
    ['Virtual host', item.vhost || '—'],
    ['队列类型', item.type || 'classic'],
    ['持久化', item.durable ? '是' : '否'],
    ['自动删除', item.auto_delete ? '是' : '否'],
    ['状态', rabbitQueueState(item).label],
    ['消费者', rabbitCount(item.consumers)],
    ['消息总数', rabbitCount(counts.messages)],
    ['待消费', rabbitCount(counts.ready)],
    ['未确认', rabbitCount(counts.unacked)],
    ['内存占用', rabbitBytes(item.memory)],
    ['节点', item.node || '—'],
    ['策略', item.policy || '—'],
    ['投递速率', rabbitRate(item.message_stats?.deliver_get_details?.rate)],
    ['确认速率', rabbitRate(item.message_stats?.ack_details?.rate)]
  ]
})
const queueArgs = computed(() => rabbitArguments(queue.value?.arguments))
const queueConsumers = computed(() => Array.isArray(queue.value?.consumer_details) ? queue.value.consumer_details : [])
const exchangeArgs = computed(() => rabbitArguments(exchange.value?.arguments))

const queueVhost = row => row?.vhost || (vhost.value === rabbitAllVhosts ? '/' : vhost.value)

onMounted(async () => {
  window.addEventListener('ops-admin-permissions-changed', permissionsChanged)
  if (!valid.value) { error.value = '数据源参数不正确，请从中间件列表进入。'; return }
  const initialTab = String(route.query.tab || '')
  if (tabs.some(([key]) => key === initialTab)) tab.value = initialTab
  if (route.query.vhost) vhost.value = String(route.query.vhost)
  await loadSource()
  await loadVhosts()
  await reload()
})
onUnmounted(() => {
  generation++; drawerGeneration++
  window.removeEventListener('ops-admin-permissions-changed', permissionsChanged)
})

async function permissionsChanged() {
  revision.value++
  error.value = ''
  if (!canRead.value) { queues.value = []; exchanges.value = []; connections.value = []; consumers.value = []; overview.value = null; return }
  await loadVhosts()
  await reload()
}

async function loadSource() {
  try {
    const rows = await listMiddleware('rabbitmq')
    source.value = (rows || []).find(item => item.id === props.sourceId) || null
  } catch { source.value = null }
}

async function loadVhosts() {
  try {
    const data = await api('vhosts')
    vhosts.value = Array.isArray(data) ? data : []
  } catch { vhosts.value = [] }
}

function api(operation, params = {}, action = 'query') {
  return operateMiddleware(props.sourceId, operation, params, action, action !== 'query', 20000, true)
}

async function reload() {
  if (!canRead.value) return
  const g = ++generation
  loading.value = true
  error.value = ''
  try {
    if (tab.value === 'overview') {
      const data = await api('overview')
      if (g === generation) overview.value = data && typeof data === 'object' ? data : null
    } else if (tab.value === 'queues') {
      const data = await api('queues', { vhost: vhost.value })
      if (g === generation) { queues.value = Array.isArray(data) ? data : []; selection.value = [] }
    } else if (tab.value === 'exchanges') {
      const data = await api('exchanges', { vhost: vhost.value })
      if (g === generation) exchanges.value = Array.isArray(data) ? data : []
    } else if (tab.value === 'connections') {
      const data = await api('connections')
      if (g === generation) connections.value = Array.isArray(data) ? data : []
    } else if (tab.value === 'consumers') {
      const data = await api('consumers', { vhost: vhost.value })
      if (g === generation) consumers.value = Array.isArray(data) ? data : []
    }
  } catch (e) {
    if (g === generation) {
      error.value = e.message || '查询失败，请检查连接与权限'
      if (tab.value === 'queues') queues.value = []
      if (tab.value === 'exchanges') exchanges.value = []
      if (tab.value === 'connections') connections.value = []
      if (tab.value === 'consumers') consumers.value = []
    }
  } finally {
    if (g === generation) loading.value = false
  }
}

async function refresh() {
  refreshing.value = true
  try {
    await loadVhosts()
    await reload()
  } finally {
    refreshing.value = false
  }
}

function changeTab() {
  error.value = ''
  queueKeyword.value = ''
  selection.value = []
  reload()
}

function onSelectionChange(rows) {
  selection.value = rows || []
}

function selectVhost(name) {
  if (vhost.value === name) return
  vhost.value = name
  queues.value = []
  exchanges.value = []
  consumers.value = []
  selection.value = []
  if (tab.value !== 'overview') reload()
}

function openVhost(row) {
  selectVhost(row.name)
  tab.value = 'queues'
  reload()
}

function openQueueDialog() {
  Object.assign(queueForm, {
    name: '',
    vhost: vhost.value === rabbitAllVhosts ? (vhosts.value[0]?.name || '/') : vhost.value,
    durable: true,
    autoDelete: false,
    queueType: '',
    arguments: ''
  })
  queueDialog.value = true
}

async function saveQueue() {
  const payload = rabbitQueuePayload(queueForm)
  if (!payload.ok) { ElMessage.warning(payload.error); return }
  actionBusy.value = true
  try {
    await api('queue.put', payload.params, 'write')
    queueDialog.value = false
    ElMessage.success('队列已创建')
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '创建队列失败')
  } finally {
    actionBusy.value = false
  }
}

function openExchangeDialog() {
  Object.assign(exchangeForm, {
    name: '',
    vhost: vhost.value === rabbitAllVhosts ? (vhosts.value[0]?.name || '/') : vhost.value,
    type: 'direct',
    durable: true,
    autoDelete: false,
    arguments: ''
  })
  exchangeDialog.value = true
}

async function saveExchange() {
  const payload = rabbitExchangePayload(exchangeForm)
  if (!payload.ok) { ElMessage.warning(payload.error); return }
  actionBusy.value = true
  try {
    await api('exchange.put', payload.params, 'write')
    exchangeDialog.value = false
    ElMessage.success('交换机已创建')
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '创建交换机失败')
  } finally {
    actionBusy.value = false
  }
}

async function openQueue(row) {
  const g = ++drawerGeneration
  drawerKind.value = 'queue'
  queue.value = { ...row }
  exchange.value = null
  bindings.value = []
  detailError.value = ''
  drawer.value = true
  detailLoading.value = true
  const host = queueVhost(row)
  try {
    const [detail, bindingList] = await Promise.all([
      api('queue.get', { vhost: host, name: row.name }),
      api('queue.bindings', { vhost: host, name: row.name })
    ])
    if (g !== drawerGeneration) return
    if (detail && typeof detail === 'object') queue.value = { ...row, ...detail, vhost: queue.value.vhost || host }
    bindings.value = rabbitBindings(bindingList)
  } catch (e) {
    if (g === drawerGeneration) detailError.value = e.message || '读取队列详情失败'
  } finally {
    if (g === drawerGeneration) detailLoading.value = false
  }
}

async function openExchange(row) {
  const g = ++drawerGeneration
  drawerKind.value = 'exchange'
  exchange.value = { ...row }
  queue.value = null
  bindings.value = []
  detailError.value = ''
  drawer.value = true
  // 默认交换机没有名称，也没有可查询的详情/绑定接口。
  if (!row.name) { detailLoading.value = false; return }
  detailLoading.value = true
  const host = queueVhost(row)
  try {
    const [detail, bindingList] = await Promise.all([
      api('exchange.get', { vhost: host, name: row.name }),
      api('exchange.bindings', { vhost: host, name: row.name })
    ])
    if (g !== drawerGeneration) return
    if (detail && typeof detail === 'object') exchange.value = { ...row, ...detail, vhost: exchange.value.vhost || host }
    bindings.value = rabbitBindings(bindingList)
  } catch (e) {
    if (g === drawerGeneration) detailError.value = e.message || '读取交换机详情失败'
  } finally {
    if (g === drawerGeneration) detailLoading.value = false
  }
}

async function purgeQueue(row) {
  const counts = rabbitQueueMessages(row)
  try {
    await ElMessageBox.confirm(`将清空队列 ${row.name} 中的 ${rabbitCount(counts.messages)} 条消息，操作不可恢复。`, '清空队列', { type: 'warning', confirmButtonText: '确认清空', cancelButtonText: '取消' })
  } catch { return }
  actionBusy.value = true
  try {
    await api('queue.purge', { vhost: queueVhost(row), name: row.name }, 'delete')
    ElMessage.success('队列已清空')
    await reload()
    if (drawer.value && queue.value?.name === row.name) await openQueue({ ...queue.value, vhost: queueVhost(row) })
  } catch (e) {
    ElMessage.error(e.message || '清空队列失败')
  } finally {
    actionBusy.value = false
  }
}

async function deleteQueue(row) {
  try {
    await ElMessageBox.confirm(`将删除队列 ${row.name}，队列中的消息会一并删除，操作不可恢复。`, '删除队列', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  actionBusy.value = true
  try {
    await api('queue.delete', { vhost: queueVhost(row), name: row.name }, 'delete')
    ElMessage.success('队列已删除')
    if (drawer.value && queue.value?.name === row.name) drawer.value = false
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '删除队列失败')
  } finally {
    actionBusy.value = false
  }
}

async function removeSelectedQueues() {
  const rows = selection.value.slice()
  if (!rows.length) return
  const names = rows.map(row => row.name)
  try {
    await ElMessageBox.confirm(`将删除 ${rows.length} 个队列：${names.slice(0, 4).join('、')}${names.length > 4 ? ' 等' : ''}。队列中的消息会一并删除。`, '批量删除队列', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  actionBusy.value = true
  const failed = []
  for (const row of rows) {
    try {
      await api('queue.delete', { vhost: queueVhost(row), name: row.name }, 'delete')
    } catch (e) {
      failed.push(`${row.name}：${e.message}`)
    }
  }
  actionBusy.value = false
  if (failed.length) ElMessage.warning(`已删除 ${rows.length - failed.length} 个，失败 ${failed.length} 个。首个失败：${failed[0]}`)
  else ElMessage.success(`已删除 ${rows.length} 个队列`)
  if (drawer.value && names.includes(queue.value?.name)) drawer.value = false
  await reload()
}

async function deleteExchange(row) {
  try {
    await ElMessageBox.confirm(`将删除交换机 ${row.name}，已存在的绑定关系会失效（队列本身不受影响）。`, '删除交换机', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  actionBusy.value = true
  try {
    await api('exchange.delete', { vhost: queueVhost(row), name: row.name }, 'delete')
    ElMessage.success('交换机已删除')
    if (drawer.value && exchange.value?.name === row.name) drawer.value = false
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '删除交换机失败')
  } finally {
    actionBusy.value = false
  }
}

async function closeConnection(row) {
  try {
    await ElMessageBox.confirm(`将强制关闭连接 ${row.name}，该连接上的通道与消费者会一并断开。`, '关闭连接', { type: 'warning', confirmButtonText: '确认关闭', cancelButtonText: '取消' })
  } catch { return }
  actionBusy.value = true
  try {
    await api('connection.close', { name: row.name }, 'delete')
    ElMessage.success('连接已关闭')
    await reload()
  } catch (e) {
    ElMessage.error(e.message || '关闭连接失败')
  } finally {
    actionBusy.value = false
  }
}

function back() { router.push('/assets/middleware/rabbitmq') }
</script>

<template>
  <section class="rmq-page">
    <header class="rmq-head">
      <el-button link type="primary" :icon="ArrowLeft" @click="back">数据源</el-button>
      <div class="rmq-title">
        <h1>{{ source?.name || 'RabbitMQ' }}</h1>
        <p>{{ source?.description || '管理队列、交换机、连接与消费者，让消息流转清晰可见。' }}</p>
      </div>
      <el-button :icon="Refresh" :loading="refreshing" :disabled="!canRead" @click="refresh">刷新</el-button>
    </header>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>
    <el-alert v-if="valid && !canRead" title="当前账号只有查看数据源的权限，浏览资源需要额外授权。" type="info" :closable="false"/>

    <div v-if="valid" class="rmq-body">
      <aside class="rmq-side">
        <div class="side-head"><span>虚拟主机</span><em>{{ vhosts.length }}</em></div>
        <div class="side-search"><el-input v-model="vhostKeyword" :prefix-icon="Search" placeholder="搜索虚拟主机" clearable aria-label="搜索虚拟主机"/></div>
        <ul class="vhost-list">
          <li :class="{ active: vhost === rabbitAllVhosts }" @click="selectVhost(rabbitAllVhosts)">
            <span class="vhost-name">全部虚拟主机</span><em>{{ rabbitCount(vhostTotal) }}</em>
          </li>
          <li v-for="item in filteredVhosts" :key="item.name" :class="{ active: vhost === item.name }" @click="selectVhost(item.name)">
            <span class="vhost-name">{{ item.name }}</span><em>{{ rabbitCount(item.messages) }}</em>
          </li>
          <li v-if="!filteredVhosts.length" class="side-empty">无匹配虚拟主机</li>
        </ul>
        <p class="side-note">队列和交换机按虚拟主机隔离，选择后列表只展示该主机下的资源。</p>
      </aside>

      <main class="rmq-main">
        <div class="rmq-toolbar">
          <template v-if="tab === 'queues'">
            <el-button type="primary" :icon="Plus" :disabled="!can('write')" @click="openQueueDialog">创建队列</el-button>
            <el-button :disabled="!can('delete') || !selection.length || actionBusy" @click="removeSelectedQueues">批量删除<span v-if="selection.length">（{{ selection.length }}）</span></el-button>
          </template>
          <template v-else-if="tab === 'exchanges'">
            <el-button type="primary" :icon="Plus" :disabled="!can('write')" @click="openExchangeDialog">创建交换机</el-button>
          </template>
          <span v-else class="toolbar-label">{{ tabLabel }}</span>
          <div class="toolbar-right">
            <el-input v-if="tab === 'queues'" v-model="queueKeyword" :prefix-icon="Search" placeholder="队列名称" clearable aria-label="按名称筛选队列" @keyup.enter="reload"/>
            <el-button v-if="tab === 'queues'" :disabled="!canRead" @click="reload">查询</el-button>
            <el-button :icon="Refresh" circle :loading="loading" :disabled="!canRead" aria-label="刷新当前列表" @click="refresh"/>
          </div>
        </div>

        <el-tabs v-model="tab" class="rmq-tabs" @tab-change="changeTab">
          <el-tab-pane v-for="[key, label] in tabs" :key="key" :label="label" :name="key"/>
        </el-tabs>

        <div v-loading="loading" class="rmq-results">
          <template v-if="tab === 'overview'">
            <div v-if="!canRead" class="rmq-empty">当前账号没有查询权限。</div>
            <template v-else>
              <div class="stat-grid">
                <article v-for="card in overviewCards" :key="card.label">
                  <span>{{ card.label }}</span>
                  <strong>{{ card.value }}</strong>
                  <small v-if="card.hint">{{ card.hint }}</small>
                </article>
              </div>
              <div class="section-head"><h2>虚拟主机</h2><span>{{ vhosts.length }} 个 · 消息总数 {{ rabbitCount(vhostTotal) }}</span></div>
              <el-table :data="rabbitVhostRows(vhosts)" empty-text="暂无虚拟主机" @row-click="openVhost">
                <el-table-column prop="name" label="虚拟主机" min-width="180"><template #default="{ row }"><el-button link type="primary" @click.stop="openVhost(row)">{{ row.name }}</el-button></template></el-table-column>
                <el-table-column label="消息数" width="120" align="right"><template #default="{ row }">{{ rabbitCount(row.messages) }}</template></el-table-column>
                <el-table-column label="待消费" width="120" align="right"><template #default="{ row }">{{ rabbitCount(row.ready) }}</template></el-table-column>
                <el-table-column label="未确认" width="120" align="right"><template #default="{ row }">{{ rabbitCount(row.unacked) }}</template></el-table-column>
                <el-table-column prop="description" label="说明" min-width="200" show-overflow-tooltip><template #default="{ row }">{{ row.description || '—' }}</template></el-table-column>
                <el-table-column label="操作" width="110"><template #default="{ row }"><el-button link type="primary" @click.stop="openVhost(row)">查看队列</el-button></template></el-table-column>
              </el-table>
            </template>
          </template>

          <template v-else-if="tab === 'queues'">
            <el-table :data="filteredQueues" row-key="name" empty-text="暂无队列，可调整虚拟主机或新建队列" @selection-change="onSelectionChange">
              <el-table-column type="selection" width="46"/>
              <el-table-column label="队列名称" min-width="230">
                <template #default="{ row }">
                  <el-button link type="primary" class="resource-name" @click="openQueue(row)">{{ row.name }}</el-button>
                  <span v-if="vhost === rabbitAllVhosts" class="resource-sub">{{ row.vhost }}</span>
                </template>
              </el-table-column>
              <el-table-column label="消息" min-width="150">
                <template #default="{ row }">
                  <span class="metric-main">{{ rabbitCount(rabbitQueueMessages(row).messages) }}</span>
                  <span class="metric-sub">待消费 {{ rabbitCount(row.messages_ready) }} · 未确认 {{ rabbitCount(row.messages_unacknowledged) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="消费者" width="92" align="right"><template #default="{ row }">{{ rabbitCount(row.consumers) }}</template></el-table-column>
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag size="small" :type="rabbitQueueState(row).type" effect="light">{{ rabbitQueueState(row).label }}</el-tag></template></el-table-column>
              <el-table-column label="操作" width="170" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openQueue(row)">查看</el-button>
                  <el-button link type="primary" :disabled="!can('delete')" @click="purgeQueue(row)">清空</el-button>
                  <el-button link type="danger" :disabled="!can('delete')" @click="deleteQueue(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p class="result-note">共 {{ filteredQueues.length }} 个队列<span v-if="queueKeyword">（按名称过滤）</span><span v-if="vhost === rabbitAllVhosts"> · 跨全部虚拟主机</span></p>
          </template>

          <template v-else-if="tab === 'exchanges'">
            <el-table :data="exchanges" row-key="name" empty-text="暂无交换机">
              <el-table-column label="交换机" min-width="200">
                <template #default="{ row }">
                  <el-button link type="primary" class="resource-name" @click="openExchange(row)">{{ row.name || '(默认交换机)' }}</el-button>
                  <span v-if="vhost === rabbitAllVhosts" class="resource-sub">{{ row.vhost }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="type" label="类型" width="110"/>
              <el-table-column label="持久化" width="90"><template #default="{ row }">{{ row.durable ? '是' : '否' }}</template></el-table-column>
              <el-table-column label="自动删除" width="95"><template #default="{ row }">{{ row.auto_delete ? '是' : '否' }}</template></el-table-column>
              <el-table-column label="参数" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ rabbitArguments(row.arguments).map(item => `${item.key}=${item.value}`).join(' · ') || '—' }}</template></el-table-column>
              <el-table-column label="操作" width="120" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openExchange(row)">详情</el-button>
                  <el-button link type="danger" :disabled="!can('delete') || !row.name" @click="deleteExchange(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p class="result-note">共 {{ exchanges.length }} 个交换机</p>
          </template>

          <template v-else-if="tab === 'connections'">
            <el-table :data="connections" row-key="name" empty-text="暂无连接">
              <el-table-column label="连接" min-width="220" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.name }}</span></template></el-table-column>
              <el-table-column prop="user" label="用户" width="110"/>
              <el-table-column label="客户端" min-width="130"><template #default="{ row }">{{ row.peer_host }}<span v-if="row.peer_port">:{{ row.peer_port }}</span></template></el-table-column>
              <el-table-column prop="vhost" label="Virtual host" width="110"/>
              <el-table-column label="状态" width="95"><template #default="{ row }"><el-tag size="small" :type="rabbitConnectionState(row.state).type" effect="light">{{ rabbitConnectionState(row.state).label }}</el-tag></template></el-table-column>
              <el-table-column label="通道" width="80" align="right"><template #default="{ row }">{{ rabbitCount(row.channels) }}</template></el-table-column>
              <el-table-column label="操作" width="110" align="right"><template #default="{ row }"><el-button link type="danger" :disabled="!can('delete')" @click="closeConnection(row)">关闭连接</el-button></template></el-table-column>
            </el-table>
            <p class="result-note">共 {{ connections.length }} 个连接 · 关闭连接会同时断开其通道与消费者</p>
          </template>

          <template v-else>
            <el-table :data="consumers" empty-text="当前虚拟主机下暂无消费者">
              <el-table-column prop="consumer_tag" label="消费者" min-width="200" show-overflow-tooltip/>
              <el-table-column label="队列" min-width="190"><template #default="{ row }">{{ row.queue?.name || '—' }}</template></el-table-column>
              <el-table-column label="连接" min-width="200" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.channel_details?.connection_name || '—' }}</span></template></el-table-column>
              <el-table-column label="通道" width="80"><template #default="{ row }">{{ row.channel_details?.name ?? '—' }}</template></el-table-column>
              <el-table-column label="消息确认" width="100"><template #default="{ row }">{{ row.ack_required ? '需要' : '自动' }}</template></el-table-column>
              <el-table-column label="预取数量" width="95" align="right"><template #default="{ row }">{{ rabbitCount(row.prefetch_count) }}</template></el-table-column>
            </el-table>
            <p class="result-note">共 {{ consumers.length }} 个消费者 · 消费者随连接生命周期管理，可在“连接”页关闭对应连接</p>
          </template>
        </div>
      </main>
    </div>

    <div v-else class="rmq-placeholder">请先选择 RabbitMQ 数据源</div>

    <el-dialog v-model="queueDialog" title="创建队列" width="min(680px, 94vw)" :close-on-click-modal="false" class="rmq-dialog">
      <el-form label-position="top" :disabled="actionBusy">
        <div class="form-columns">
          <el-form-item label="队列名称" required><el-input v-model="queueForm.name" placeholder="例如：order.created" maxlength="255"/></el-form-item>
          <el-form-item label="Virtual host" required><el-input v-model="queueForm.vhost" placeholder="/" maxlength="255"/></el-form-item>
        </div>
        <div class="form-columns">
          <el-form-item label="队列类型"><el-select v-model="queueForm.queueType"><el-option v-for="[value, label] in rabbitQueueTypeOptions" :key="value" :label="label" :value="value"/></el-select></el-form-item>
          <el-form-item label="特性"><div class="switch-row"><el-checkbox v-model="queueForm.durable">持久化</el-checkbox><el-checkbox v-model="queueForm.autoDelete">自动删除</el-checkbox></div></el-form-item>
        </div>
        <el-form-item label="扩展参数（JSON）"><el-input v-model="queueForm.arguments" type="textarea" :rows="5" spellcheck="false" class="mono-input" placeholder='例如：{"x-max-length":1000,"x-message-ttl":60000}'/></el-form-item>
      </el-form>
      <p class="dialog-note">队列声明幂等：同名队列已存在且参数一致时不会重建；参数不一致会被服务端拒绝。</p>
      <template #footer><el-button :disabled="actionBusy" @click="queueDialog = false">取消</el-button><el-button type="primary" :loading="actionBusy" @click="saveQueue">创建</el-button></template>
    </el-dialog>

    <el-dialog v-model="exchangeDialog" title="创建交换机" width="min(680px, 94vw)" :close-on-click-modal="false" class="rmq-dialog">
      <el-form label-position="top" :disabled="actionBusy">
        <div class="form-columns">
          <el-form-item label="交换机名称" required><el-input v-model="exchangeForm.name" placeholder="例如：order.events" maxlength="255"/></el-form-item>
          <el-form-item label="Virtual host" required><el-input v-model="exchangeForm.vhost" placeholder="/" maxlength="255"/></el-form-item>
        </div>
        <div class="form-columns">
          <el-form-item label="类型"><el-select v-model="exchangeForm.type"><el-option v-for="type in rabbitExchangeTypes" :key="type" :label="type" :value="type"/></el-select></el-form-item>
          <el-form-item label="特性"><div class="switch-row"><el-checkbox v-model="exchangeForm.durable">持久化</el-checkbox><el-checkbox v-model="exchangeForm.autoDelete">自动删除</el-checkbox></div></el-form-item>
        </div>
        <el-form-item label="扩展参数（JSON）"><el-input v-model="exchangeForm.arguments" type="textarea" :rows="4" spellcheck="false" class="mono-input" placeholder="通常留空，alternate-exchange 等参数可在此填写"/></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="actionBusy" @click="exchangeDialog = false">取消</el-button><el-button type="primary" :loading="actionBusy" @click="saveExchange">创建</el-button></template>
    </el-dialog>

    <el-drawer v-model="drawer" :title="drawerKind === 'queue' ? (queue?.name || '队列详情') : (exchange?.name || '交换机详情')" size="min(780px, 96vw)" class="rmq-drawer">
      <div v-loading="detailLoading" class="drawer-body">
        <el-alert v-if="detailError" :title="detailError" type="warning" :closable="false" show-icon/>
        <template v-if="drawerKind === 'queue' && queue">
          <div class="drawer-tags">
            <el-tag size="small" effect="light">{{ queue.type || 'classic' }}</el-tag>
            <el-tag size="small" :type="rabbitQueueState(queue).type" effect="light">{{ rabbitQueueState(queue).label }}</el-tag>
            <el-tag size="small" type="info" effect="plain">{{ queue.vhost }}</el-tag>
            <el-tag v-if="queue.durable" size="small" type="success" effect="plain">持久化</el-tag>
          </div>
          <dl class="meta-grid">
            <div v-for="[label, value] in queueMeta" :key="label"><dt>{{ label }}</dt><dd>{{ value }}</dd></div>
          </dl>

          <h3 class="drawer-section">绑定关系 <span>{{ bindings.length }} 条</span></h3>
          <el-table :data="bindings" size="small" empty-text="该队列尚未绑定任何交换机">
            <el-table-column prop="source" label="来源交换机" min-width="180" show-overflow-tooltip/>
            <el-table-column prop="routingKey" label="Routing key" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ row.routingKey || '—' }}</template></el-table-column>
            <el-table-column prop="destination" label="目标" min-width="160" show-overflow-tooltip/>
          </el-table>

          <h3 class="drawer-section">扩展参数 <span>{{ queueArgs.length }} 项</span></h3>
          <dl v-if="queueArgs.length" class="arg-grid"><div v-for="item in queueArgs" :key="item.key"><dt>{{ item.key }}</dt><dd>{{ item.value }}</dd></div></dl>
          <p v-else class="drawer-empty">未声明扩展参数。</p>

          <h3 class="drawer-section">消费者 <span>{{ queueConsumers.length }} 个</span></h3>
          <el-table :data="queueConsumers" size="small" empty-text="暂无消费者">
            <el-table-column prop="consumer_tag" label="Consumer tag" min-width="200" show-overflow-tooltip/>
            <el-table-column label="连接" min-width="220" show-overflow-tooltip><template #default="{ row }">{{ row.channel_details?.connection_name || '—' }}</template></el-table-column>
            <el-table-column label="预取" width="90" align="right"><template #default="{ row }">{{ rabbitCount(row.prefetch_count) }}</template></el-table-column>
          </el-table>
        </template>

        <template v-else-if="exchange">
          <div class="drawer-tags">
            <el-tag size="small" effect="light">{{ exchange.type || 'direct' }}</el-tag>
            <el-tag size="small" type="info" effect="plain">{{ exchange.vhost }}</el-tag>
            <el-tag v-if="exchange.durable" size="small" type="success" effect="plain">持久化</el-tag>
          </div>
          <dl class="meta-grid">
            <div><dt>Virtual host</dt><dd>{{ exchange.vhost }}</dd></div>
            <div><dt>类型</dt><dd>{{ exchange.type || 'direct' }}</dd></div>
            <div><dt>持久化</dt><dd>{{ exchange.durable ? '是' : '否' }}</dd></div>
            <div><dt>自动删除</dt><dd>{{ exchange.auto_delete ? '是' : '否' }}</dd></div>
            <div><dt>内部交换机</dt><dd>{{ exchange.internal ? '是' : '否' }}</dd></div>
            <div><dt>消息流入</dt><dd>{{ rabbitRate(exchange.message_stats?.publish_in_details?.rate) }}</dd></div>
          </dl>
          <h3 class="drawer-section">扩展参数 <span>{{ exchangeArgs.length }} 项</span></h3>
          <dl v-if="exchangeArgs.length" class="arg-grid"><div v-for="item in exchangeArgs" :key="item.key"><dt>{{ item.key }}</dt><dd>{{ item.value }}</dd></div></dl>
          <p v-else class="drawer-empty">未声明扩展参数。</p>
          <h3 class="drawer-section">绑定关系 <span>{{ bindings.length }} 条</span></h3>
          <el-table :data="bindings" size="small" empty-text="该交换机暂无绑定">
            <el-table-column prop="source" label="来源交换机" min-width="180" show-overflow-tooltip/>
            <el-table-column prop="routingKey" label="Routing key" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ row.routingKey || '—' }}</template></el-table-column>
            <el-table-column prop="destination" label="目标" min-width="160" show-overflow-tooltip/>
            <el-table-column prop="kind" label="类型" width="90"/>
          </el-table>
        </template>
      </div>
      <template #footer>
        <div class="drawer-foot">
          <el-button :disabled="detailLoading" @click="drawer = false">关闭</el-button>
          <div class="drawer-actions">
            <template v-if="drawerKind === 'queue' && queue">
              <el-button :disabled="!can('delete') || actionBusy" @click="purgeQueue(queue)">清空消息</el-button>
              <el-button type="danger" plain :disabled="!can('delete') || actionBusy" @click="deleteQueue(queue)">删除队列</el-button>
            </template>
            <template v-else-if="exchange && exchange.name">
              <el-button type="danger" plain :disabled="!can('delete') || actionBusy" @click="deleteExchange(exchange)">删除交换机</el-button>
            </template>
          </div>
        </div>
      </template>
    </el-drawer>
  </section>
</template>

<style scoped>
.rmq-page{display:flex;flex-direction:column;gap:14px;height:calc(100vh - 190px);min-height:560px;color:var(--el-text-color-primary,#23364f)}
.rmq-head{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:16px 20px;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px}
.rmq-title{margin-right:auto;min-width:0}
.rmq-title h1{margin:0;font-size:22px}
.rmq-title p{margin:6px 0 0;font-size:14px;color:var(--el-text-color-secondary,#8795aa);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.rmq-body{display:grid;grid-template-columns:268px minmax(0,1fr);gap:14px;flex:1;min-height:0}
.rmq-side,.rmq-main{display:flex;flex-direction:column;min-height:0;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px;overflow:hidden}
.side-head{display:flex;align-items:center;justify-content:space-between;padding:16px 16px 10px;font-size:15px;font-weight:600}
.side-head em{font-style:normal;font-size:12px;font-weight:500;color:#8795aa;background:#f2f5fa;border-radius:9px;padding:1px 8px}
.side-search{padding:0 12px 10px}
.side-search :deep(.el-input__inner){font-size:15px}
.vhost-list{flex:1;min-height:0;overflow:auto;list-style:none;margin:0;padding:2px 8px 12px;display:flex;flex-direction:column;gap:2px}
.vhost-list li{display:flex;align-items:center;justify-content:space-between;gap:10px;min-height:40px;padding:0 10px;border-radius:7px;font-size:15px;color:#33465e;cursor:pointer}
.vhost-list li:hover{background:var(--el-fill-color-light,#f3f6fb)}
.vhost-list li.active{background:#e8f1fb;color:#2f6bb5;font-weight:600}
.vhost-list li.side-empty{justify-content:center;color:#9aa7ba;font-size:14px;cursor:default;min-height:70px}
.vhost-list li.side-empty:hover{background:none}
.vhost-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.vhost-list em{font-style:normal;font-size:12.5px;color:#98a5b8;font-variant-numeric:tabular-nums}
.vhost-list li.active em{color:#3b68b0}
.side-note{margin:0;padding:12px 16px 14px;border-top:1px solid var(--el-border-color-lighter,#eef2f8);font-size:13px;line-height:1.7;color:#8b98ab}
.rmq-main{padding:14px 18px 18px}
.rmq-toolbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;min-height:40px}
.toolbar-label{font-size:15px;font-weight:600;color:#41546d}
.toolbar-right{display:flex;align-items:center;gap:8px;margin-left:auto}
.toolbar-right :deep(.el-input){width:220px}
.rmq-tabs :deep(.el-tabs__item){font-size:15px;height:44px}
.rmq-tabs :deep(.el-tabs__header){margin:6px 0 4px}
.rmq-results{flex:1;min-height:0;overflow:auto;padding-top:2px}
.rmq-results :deep(.el-table th){font-size:13.5px;color:#7b889d;font-weight:600}
.rmq-results :deep(.el-table td){font-size:14.5px}
.resource-name{font-size:15px;font-weight:550}
.resource-sub{margin-left:8px;font-size:12.5px;color:#9aa7ba;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.metric-main{display:block;font-size:14.5px;font-weight:550;color:#33465e}
.metric-sub{display:block;font-size:12px;color:#98a5b8;line-height:1.5;white-space:nowrap}
.result-note{font-size:13.5px;color:#8795aa;margin:12px 0 0}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13.5px}
.stat-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;margin-bottom:22px}
.stat-grid article{display:grid;gap:6px;padding:16px 18px;border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:10px;background:linear-gradient(180deg,#fbfcfe,#fff)}
.stat-grid span{font-size:13.5px;color:#8391a6}
.stat-grid strong{font-size:20px;color:#22364f;font-weight:650;overflow-wrap:anywhere}
.stat-grid small{font-size:12.5px;color:#98a5b8}
.section-head{display:flex;align-items:baseline;gap:12px;margin:0 0 12px}
.section-head h2{margin:0;font-size:16px}
.section-head span{font-size:13.5px;color:#8795aa}
.rmq-placeholder{display:grid;place-items:center;flex:1;border:1px dashed var(--el-border-color,#dfe6f1);border-radius:12px;background:var(--el-bg-color,#fff);font-size:15px;color:#8795aa;min-height:320px}
.rmq-empty{display:grid;place-items:center;min-height:240px;font-size:15px;color:#8795aa}
.form-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 16px}
.switch-row{display:flex;align-items:center;gap:16px;min-height:32px}
.mono-input :deep(textarea){font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14px;line-height:1.7}
.dialog-note{font-size:13px;color:#8795aa;line-height:1.7;margin:4px 0 0}
.drawer-body{min-height:200px}
.drawer-tags{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:18px}
.meta-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px 28px;margin:0}
.meta-grid>div{display:grid;grid-template-columns:112px minmax(0,1fr);gap:8px;align-items:baseline}
.meta-grid dt{font-size:13.5px;color:#8a97ab;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.meta-grid dd{margin:0;font-size:14.5px;color:#2f6bb5;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.drawer-section{display:flex;align-items:baseline;gap:10px;font-size:15px;margin:24px 0 12px}
.drawer-section span{font-size:13px;color:#8795aa;font-weight:400}
.arg-grid{display:grid;gap:10px;margin:0}
.arg-grid>div{display:grid;grid-template-columns:200px minmax(0,1fr);gap:10px;padding:8px 12px;border-radius:7px;background:#f7f9fc}
.arg-grid dt,.arg-grid dd{margin:0;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13.5px;overflow-wrap:anywhere}
.arg-grid dt{color:#6d7d93}
.arg-grid dd{color:#33465e}
.drawer-empty{margin:0;font-size:13.5px;color:#8b98ab}
.drawer-foot{display:flex;align-items:center;gap:10px}
.drawer-actions{display:flex;gap:8px;margin-left:auto}
@media (max-width:1080px){
  .rmq-page{height:auto}
  .rmq-body{grid-template-columns:1fr}
  .vhost-list{max-height:260px}
  .meta-grid,.form-columns{grid-template-columns:1fr}
}
</style>
