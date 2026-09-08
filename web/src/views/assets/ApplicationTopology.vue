<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Refresh } from '@element-plus/icons-vue'
import { queryAssetServiceRuntimeTopology } from '../../api/asset'
import ServiceWorkloadDetail from './ServiceWorkloadDetail.vue'
import ServiceWorkloadLogs from './ServiceWorkloadLogs.vue'
import ServicePodMonitor from './ServicePodMonitor.vue'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const topology = ref({})
const detailVisible = ref(false)
const activeDrawerTab = ref('detail')
const selectedWorkload = ref(null)
const selectedLog = ref(null)
const serviceId = computed(() => Number(route.query.serviceId))
const workloads = computed(() => topology.value.workloads || [])
const normalize = (value = '') => String(value).toLowerCase()
const find = (predicate) => workloads.value.find((item) => predicate(normalize(item.name)))
const byInstance = (items) => [...items].sort((left, right) => String(left.name).localeCompare(String(right.name), undefined, { numeric: true, sensitivity: 'base' }))
const homeWorkloads = computed(() => byInstance(workloads.value.filter((item) => normalize(item.name).includes('home'))))
const worldWorkloads = computed(() => byInstance(workloads.value.filter((item) => normalize(item.name).includes('world'))))
const gameColumns = computed(() => Math.min(3, Math.max(1, Math.ceil(Math.sqrt(Math.max(homeWorkloads.value.length, worldWorkloads.value.length, 1))))))
const gameStartX = 1010
const nodeStepX = 214
const nodeStepY = 122
const statefulGroupPadding = 22
const homeRows = computed(() => Math.max(1, Math.ceil(homeWorkloads.value.length / gameColumns.value)))
const worldRows = computed(() => Math.max(1, Math.ceil(worldWorkloads.value.length / gameColumns.value)))
const statefulGroupWidth = computed(() => Math.max(236, gameColumns.value * nodeStepX + statefulGroupPadding * 2 - 28))
const homeGroupY = 122
const homeGroupHeight = computed(() => statefulGroupPadding * 2 + homeRows.value * nodeStepY + 4)
const worldGroupY = computed(() => homeGroupY + homeGroupHeight.value + 38)
const worldGroupHeight = computed(() => statefulGroupPadding * 2 + worldRows.value * nodeStepY + 4)
const worldStartY = computed(() => worldGroupY.value + statefulGroupPadding)
const gameBottomY = computed(() => worldGroupY.value + worldGroupHeight.value)
const canvasWidth = computed(() => Math.max(1240, gameStartX + statefulGroupWidth.value + 30))
const canvasHeight = computed(() => Math.max(600, gameBottomY.value + 130))
const statefulGroups = computed(() => [
  { key: 'home', items: homeWorkloads.value, x: gameStartX - statefulGroupPadding, y: homeGroupY, width: statefulGroupWidth.value, height: homeGroupHeight.value },
  { key: 'world', items: worldWorkloads.value, x: gameStartX - statefulGroupPadding, y: worldGroupY.value, width: statefulGroupWidth.value, height: worldGroupHeight.value }
].filter((group) => group.items.length))
function healthy(item) {
  if (!item) return true
  const [ready, expected] = String(item.ready || '').split('/').map(Number)
  return expected > 0 && ready === expected && Number(item.available) >= expected
}
const nodes = computed(() => {
  const add = (id, title, x, y, kind, note, workload = null) => ({ id, title, x, y, kind, note, workload, healthy: healthy(workload) })
  const nginx = find((name) => name.includes('nginx-gm')); const mgr = find((name) => name.includes('mgr')); const gate = find((name) => name.includes('gate')); const login = find((name) => name.includes('login')); const notice = find((name) => name.includes('notice')); const social = find((name) => name.includes('social')); const zk = find((name) => name.includes('zookeeper') || name === 'zk')
  const result = [add('player', '玩家客户端', 34, 270, 'external', '仅通过 Gate / Notice 通信')]
  if (nginx) result.push(add('nginx', nginx.name, 250, 74, 'management', '独立 GM 入口', nginx))
  if (mgr) result.push(add('mgr', mgr.name, 460, 74, 'management', 'GM 后台', mgr))
  if (gate) result.push(add('gate', gate.name, 270, 280, 'public', 'WebSocket · 对外入口', gate))
  if (login) result.push(add('login', login.name, 505, 280, 'service', '登录校验', login))
  result.push(add('zk', zk?.name || 'ZooKeeper', 740, 280, 'dependency', '服务注册 / 启用状态', zk || null))
  const addGameGroup = (items, prefix, startY) => items.forEach((item, index) => {
    const column = index % gameColumns.value
    const row = Math.floor(index / gameColumns.value)
    const node = add(`${prefix}-${index}`, item.name, gameStartX + column * nodeStepX, startY + row * nodeStepY, 'service', '有状态游戏服务', item)
    node.statefulGroup = prefix
    node.instance = index + 1
    result.push(node)
  })
  addGameGroup(homeWorkloads.value, 'home', homeGroupY + statefulGroupPadding)
  addGameGroup(worldWorkloads.value, 'world', worldStartY.value)
  if (social) result.push(add('social', social.name, 740, Math.max(470, gameBottomY.value + 18), 'service', '社交服务 · 通过 ZooKeeper 发现', social))
  if (notice) result.push(add('notice', notice.name, 270, 470, 'public', '日志上报 · 对外入口', notice))
  const known = new Set([nginx, mgr, gate, login, notice, social, zk, ...homeWorkloads.value, ...worldWorkloads.value].filter(Boolean).map((item) => item.name))
  workloads.value.filter((item) => !known.has(item.name)).forEach((item, index) => result.push(add(`extra-${index}`, item.name, 740, 74 + index * 105, 'service', `${item.type} · Ready ${item.ready || '0/0'}`, item)))
  return result
})
const positions = computed(() => Object.fromEntries(nodes.value.map((item) => [item.id, item])))
const edges = computed(() => {
  const result = []; const has = (id) => Boolean(positions.value[id]); const add = (from, to, label, tone = 'default') => { if (has(from) && has(to)) result.push({ from, to, label, tone }) }
  add('player', 'gate', 'WebSocket 登录', 'public'); add('player', 'notice', '日志上报', 'public'); add('gate', 'login', '登录请求', 'public'); add('login', 'zk', '读取启用服务'); add('zk', 'social', '服务注册'); add('mgr', 'zk', '启停管理', 'management')
  homeWorkloads.value.forEach((_, index) => { const id = `home-${index}`; add('zk', id, index === 0 ? '服务注册' : ''); add('gate', id, index === 0 ? '游戏会话' : ''); add(id, 'social', index === 0 ? '社交通信' : ''); add('mgr', id, index === 0 ? '管理游戏服' : '', 'management') })
  worldWorkloads.value.forEach((_, index) => { const id = `world-${index}`; add('zk', id, index === 0 ? '服务注册' : ''); add('gate', id, index === 0 ? '游戏会话' : ''); add(id, 'social', index === 0 ? '社交通信' : ''); add('mgr', id, index === 0 ? '管理游戏服' : '', 'management') })
  nodes.value.filter((item) => !['player', 'nginx', 'zk'].includes(item.id)).forEach((item, index) => add(item.id, 'nginx', index === 0 ? '访问 GM' : '', 'management'))
  return result
})
function nodeWidth(node) { return node?.statefulGroup ? 190 : 174 }
function path(edge) { const from = positions.value[edge.from]; const to = positions.value[edge.to]; const x1 = from.x + nodeWidth(from); const y1 = from.y + 46; const x2 = to.x; const y2 = to.y + 46; const middle = Math.round((x1 + x2) / 2); return `M ${x1} ${y1} H ${middle} V ${y2} H ${x2}` }
function label(edge) { const from = positions.value[edge.from]; const to = positions.value[edge.to]; return { x: Math.round((from.x + nodeWidth(from) + to.x) / 2), y: Math.round((from.y + to.y) / 2) - 8 } }
function openDetail(node) { if (!node.workload) return; selectedWorkload.value = node.workload; selectedLog.value = null; activeDrawerTab.value = 'detail'; detailVisible.value = true }
function openLogs(target = null) { selectedLog.value = target; activeDrawerTab.value = 'logs' }
async function load() { if (!serviceId.value) return; loading.value = true; try { topology.value = await queryAssetServiceRuntimeTopology(serviceId.value) } finally { loading.value = false } }
onMounted(load)
</script>

<template>
  <div class="service-topology" v-loading="loading">
    <section class="topology-header"><div><el-button text :icon="ArrowLeft" @click="router.push('/containers/services')">返回服务管理</el-button><p class="eyebrow">SERVICE RESOURCE TOPOLOGY</p><h1>{{ topology.service?.name || '服务资源拓扑' }}</h1><p><code>{{ topology.service?.serviceUid }}</code> · {{ topology.cluster?.name || '未绑定集群' }} / {{ topology.namespace || '-' }}</p></div><el-button :icon="Refresh" @click="load">刷新运行状态</el-button></section>
    <el-alert v-if="topology.refreshError" type="warning" :closable="false" show-icon :title="topology.refreshError" />
    <section class="topology-card"><header><div><p class="section-kicker">SERVICE RELATION MAP</p><h2>业务通信链路</h2><p>按入口、管理与内部服务梳理核心通信路径；点击节点可进入对应工作负载详情。</p></div><div class="legend"><span class="public"></span>对外入口 <span class="management"></span>GM 管理 <span class="service"></span>内部服务 <i class="legend-health ok">✓</i>健康 <i class="legend-health bad">!</i>异常</div></header><div class="topology-scroll"><div class="canvas" :style="{ width: `${canvasWidth}px`, height: `${canvasHeight}px` }"><div v-for="group in statefulGroups" :key="group.key" class="stateful-group" :class="group.key" :style="{ left: `${group.x}px`, top: `${group.y}px`, width: `${group.width}px`, height: `${group.height}px` }"></div><svg :viewBox="`0 0 ${canvasWidth} ${canvasHeight}`" :width="canvasWidth" :height="canvasHeight"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="#8ca1bf" /></marker></defs><g v-for="edge in edges" :key="`${edge.from}-${edge.to}`"><path :d="path(edge)" :class="['edge', edge.tone]" marker-end="url(#arrow)"/><text v-if="edge.label" :x="label(edge).x" :y="label(edge).y" text-anchor="middle">{{ edge.label }}</text></g></svg><button v-for="node in nodes" :key="node.id" class="topology-node" :class="[`node-${node.kind}`, { clickable: node.workload, 'node-stateful': node.statefulGroup }]" :style="{ left: `${node.x}px`, top: `${node.y}px` }" @click="openDetail(node)"><i v-if="node.workload" class="health-badge" :class="node.healthy ? 'is-ok' : 'is-bad'">{{ node.healthy ? '✓' : '!' }}</i><span v-if="node.statefulGroup" class="instance-chip">{{ node.statefulGroup.toUpperCase() }} #{{ node.instance }}</span><b>{{ node.title }}</b><small>{{ node.note }}</small><small v-if="node.workload">{{ node.workload.type }} · Ready {{ node.workload.ready || '0/0' }}</small></button></div></div></section>
    <section class="workload-card"><h2>关联工作负载</h2><el-table :data="workloads" size="small"><el-table-column prop="name" label="工作负载"/><el-table-column prop="type" label="类型" width="120"/><el-table-column prop="ready" label="Ready" width="100"/><el-table-column label="健康" width="100"><template #default="{ row }"><el-tag :type="healthy(row) ? 'success' : 'danger'">{{ healthy(row) ? '✓ 正常' : '! 异常' }}</el-tag></template></el-table-column><el-table-column label="操作" width="110"><template #default="{ row }"><el-button link type="primary" @click="openDetail({ workload: row })">服务详情</el-button></template></el-table-column></el-table></section>
    <el-drawer v-model="detailVisible" size="70%" :with-header="false" destroy-on-close><div v-if="selectedWorkload" class="service-drawer-tabs"><el-button :type="activeDrawerTab === 'detail' ? 'primary' : 'default'" @click="activeDrawerTab = 'detail'">服务详情</el-button><el-button :type="activeDrawerTab === 'logs' ? 'primary' : 'default'" @click="openLogs()">服务日志</el-button><el-button :type="activeDrawerTab === 'monitor' ? 'primary' : 'default'" @click="activeDrawerTab = 'monitor'">Pod 监控</el-button></div><ServiceWorkloadDetail v-if="activeDrawerTab === 'detail'" :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" inline @close="detailVisible = false" @show-logs="openLogs" /><ServiceWorkloadLogs v-else-if="activeDrawerTab === 'logs'" :key="selectedLog?.podName || 'default'" :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" :pod-name="selectedLog?.podName || ''" inline @close="detailVisible = false" /><ServicePodMonitor v-else :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" /></el-drawer>
  </div>
</template>

<style scoped>
.service-topology{padding:24px;background:#f3f7fc;min-height:100%}.topology-header,.topology-card,.workload-card{background:#fff;border:1px solid #dfe9f6;border-radius:18px}.topology-header{display:flex;justify-content:space-between;align-items:center;padding:22px 28px;margin-bottom:16px;background:linear-gradient(120deg,#fff,#eff5ff)}.eyebrow,.section-kicker{font-size:11px;letter-spacing:.1em;color:#4c72dd;font-weight:800;margin:10px 0 5px}.section-kicker{margin:0 0 6px}.topology-header h1{margin:0;color:#102b54}.topology-header p{margin:8px 0 0;color:#7485a0}.topology-header code{color:#4168b5}.topology-card{overflow:hidden;margin-top:16px;box-shadow:0 10px 28px rgba(42,76,137,.05)}.topology-card>header{display:flex;justify-content:space-between;align-items:center;gap:24px;padding:20px 26px;border-bottom:1px solid #edf2f8}.topology-card h2,.workload-card h2{margin:0;color:#142e57;font-size:19px}.topology-card p{margin:7px 0 0;color:#7789a4;font-size:13px}.legend{display:flex;gap:8px;align-items:center;flex-wrap:wrap;padding:8px 10px;border:1px solid #edf1f7;border-radius:999px;background:#fbfcff;color:#7586a0;font-size:12px;white-space:nowrap}.legend span{width:8px;height:8px;border-radius:50%;margin-left:4px}.legend .public{background:#3989ef}.legend .management{background:#8a61e8}.legend .service{background:#27b783}.legend-health{width:18px;height:18px;border-radius:50%;display:grid;place-items:center;color:#fff;font-style:normal;font-weight:800}.legend-health.ok{background:#19b889}.legend-health.bad{background:#ef5d65}.topology-scroll{overflow:auto;padding-bottom:2px}.canvas{position:relative;min-width:1200px;height:600px;background-color:#fcfdff;background-image:radial-gradient(#d8e5f5 .9px,transparent .9px);background-size:18px 18px}.canvas>svg{position:absolute;inset:0;width:100%;height:100%;z-index:1}.edge{fill:none;stroke:#9aacc5;stroke-width:1.45;opacity:.75}.edge.public{stroke:#3989ef;stroke-width:2.2;opacity:.95}.edge.management{stroke:#8862e4;stroke-width:1.7;stroke-dasharray:6 5;opacity:.8}.canvas text{font-size:11px;font-weight:600;fill:#607695;paint-order:stroke;stroke:#fcfdff;stroke-width:5px}.topology-node{position:absolute;z-index:2;width:174px;min-height:94px;padding:13px 14px;border:1px solid #d8e4f3;border-radius:15px;background:#fff;box-shadow:0 7px 18px rgba(45,78,132,.08);text-align:left;transition:transform .16s ease,box-shadow .16s ease,border-color .16s ease}.topology-node::before{position:absolute;top:14px;bottom:14px;left:-1px;width:3px;border-radius:3px;background:#8ca1bf;content:''}.topology-node.clickable{cursor:pointer}.topology-node.clickable:hover{transform:translateY(-3px);box-shadow:0 13px 28px rgba(45,78,132,.16)}.topology-node b,.topology-node small{display:block;overflow-wrap:anywhere;word-break:break-word;line-height:1.35}.topology-node b{padding-right:28px;color:#132f59;font-size:14px}.topology-node small{margin-top:5px;color:#7a8da9;font-size:11px}.health-badge{position:absolute;right:12px;top:12px;width:20px;height:20px;border-radius:50%;display:grid;place-items:center;color:#fff;font-style:normal;font-size:13px;font-weight:800;line-height:1;box-shadow:0 3px 8px rgba(24,139,103,.25)}.health-badge.is-ok{background:#18b889}.health-badge.is-bad{background:#ef5d65}.node-public{border-color:#a8d2fb;background:linear-gradient(135deg,#f8fcff,#eff8ff)}.node-public::before{background:#3989ef}.node-management{border-color:#c9b5f6;background:linear-gradient(135deg,#fff,#f7f3ff)}.node-management::before{background:#8a61e8}.node-dependency{border-color:#f3ce91;background:linear-gradient(135deg,#fffdf8,#fff7e9)}.node-dependency::before{background:#ec9a2e}.node-service{border-color:#9fdfc4;background:linear-gradient(135deg,#fbfffd,#effcf6)}.node-service::before{background:#28b783}.node-external{border-color:#b8c6d9;background:linear-gradient(135deg,#fff,#f4f7fb)}.node-external::before{background:#7085a4}.workload-card{margin-top:16px;padding:20px}.workload-card h2{margin-bottom:16px}.service-drawer-tabs{position:sticky;top:0;z-index:3;display:flex;gap:10px;padding:14px 24px 0;background:#fff;border-bottom:1px solid #e6edf7}.stateful-group{position:absolute;z-index:0;padding:10px 12px;border:1px dashed #9dceb9;border-radius:18px;background:linear-gradient(135deg,rgba(235,255,246,.9),rgba(247,255,251,.55))}.stateful-group.world{border-color:#a7bfea;background:linear-gradient(135deg,rgba(241,247,255,.9),rgba(250,252,255,.55))}.node-stateful{width:190px;min-height:104px;padding-top:29px;border-color:#66cda1;background:linear-gradient(135deg,#fcfffd,#effcf6)}.node-stateful .instance-chip{position:absolute;top:9px;left:13px;padding:3px 7px;border-radius:999px;background:#ddf7eb;color:#19825e;font-size:10px;font-weight:800;letter-spacing:.05em}.node-stateful b{font-size:15px}.node-stateful small{margin-top:6px}@media(max-width:1200px){.topology-card>header{align-items:flex-start;gap:12px;flex-direction:column}}@media(max-width:720px){.service-topology{padding:14px}.topology-header{align-items:flex-start;gap:16px;flex-direction:column}}
</style>
