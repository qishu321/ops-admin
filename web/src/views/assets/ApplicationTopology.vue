<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { buildServiceTopology, routeServiceEdge, healthyWorkload as healthy } from '../../utils/service-topology'
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
const layout = computed(() => buildServiceTopology(workloads.value))
const nodes = computed(() => layout.value.nodes)
const canvasWidth = computed(() => layout.value.width)
const canvasHeight = computed(() => layout.value.height)
const statefulGroups = computed(() => layout.value.groups)
const edges = computed(() => layout.value.edges.map(edge => ({...edge,...routeServiceEdge(edge,layout.value)})))
const focusedNode = ref('')
const relationMode = ref('all')
// 默认 100%：节点文字按设计稿的字号显示，不再为了塞进一屏把整张图缩成小字。
const zoom = ref(1)
// 只有用户主动点「适应窗口」才进入自动适配；缩放后拖窗口不重置他的视图。
const autoFit = ref(false)
const topologyScroll = ref(null)
const visibleEdges = computed(() => edges.value.filter(edge => relationMode.value !== 'scheduler' || edge.tone === 'scheduler'))
const linkedNodes = computed(() => new Set(visibleEdges.value.filter(edge => edge.from===focusedNode.value || edge.to===focusedNode.value).flatMap(edge => [edge.from,edge.to])))
const focusedEdges = computed(() => focusedNode.value ? visibleEdges.value.filter(edge=>edge.from===focusedNode.value || edge.to===focusedNode.value) : [])
function connected(edge) { return focusedNode.value && (edge.from===focusedNode.value || edge.to===focusedNode.value) }
function edgeLabel(edge) { return edge.label && (focusedNode.value ? connected(edge) : !edge.secondary && edge.tone!=='scheduler') }
// 画布区高度跟着内容走，而不是反过来把内容裁一刀：以前高度取「视口剩余空间」
// （最小 400），100% 时画布比它高，下半截被卡片边缘切掉，看着像被下面的表格盖住了。
function sizeCanvasBox() {
  const box = topologyScroll.value
  if (!box) return
  const scaledW = Math.round(canvasWidth.value * zoom.value)
  const scaledH = Math.round(canvasHeight.value * zoom.value)
  // 出现横向滚动条时它自己会占掉十几像素，不预留就会再多出一根纵向滚动条。
  const scrollbar = scaledW > box.clientWidth + 1 ? 16 : 0
  box.style.height = `${Math.max(280, scaledH + scrollbar + 2)}px`
}
function fitCanvas() {
  const box = topologyScroll.value
  if (!box) return
  const top = box.getBoundingClientRect().top + window.scrollY
  const available = Math.max(240, window.innerHeight - top - 24)
  zoom.value = Math.max(.45, Math.min(1,
    (box.clientWidth - 24) / canvasWidth.value,
    available / canvasHeight.value))
  autoFit.value = true
}
function changeZoom(delta) { autoFit.value = false; zoom.value=Math.max(.45,Math.min(1.3,Number((zoom.value+delta).toFixed(2)))) }
function originalSize() { autoFit.value = false; zoom.value = 1 }
function openDetail(node) { if (!node.workload) return; selectedWorkload.value = node.workload; selectedLog.value = null; activeDrawerTab.value = 'detail'; detailVisible.value = true }
function openLogs(target = null) { selectedLog.value = target; activeDrawerTab.value = 'logs' }
async function load() {
  if (!serviceId.value) return
  loading.value = true
  try { topology.value = await queryAssetServiceRuntimeTopology(serviceId.value) } finally { loading.value = false }
  // 画布尺寸跟着工作负载数量变，刷新后要重新量一次再适配。
  await nextTick(); sizeCanvasBox(); if (autoFit.value) fitCanvas()
}
function refitOnResize() { sizeCanvasBox(); if (autoFit.value) fitCanvas() }
// 缩放或画布尺寸一变，容器高度就得跟着改，否则又会出现「图被裁掉一截」。
watch([zoom, canvasWidth, canvasHeight], async () => { await nextTick(); sizeCanvasBox() })
onMounted(async () => { await load(); await nextTick(); sizeCanvasBox(); window.addEventListener('resize', refitOnResize) })
onBeforeUnmount(() => window.removeEventListener('resize', refitOnResize))
</script>

<template>
  <div class="service-topology" v-loading="loading">
    <section class="topology-header"><div><el-button text :icon="ArrowLeft" @click="router.push('/containers/services')">返回服务管理</el-button><p class="eyebrow">SERVICE RESOURCE TOPOLOGY</p><h1>{{ topology.service?.name || '服务资源拓扑' }}</h1><p><code>{{ topology.service?.serviceUid }}</code> · {{ topology.cluster?.name || '未绑定集群' }} / {{ topology.namespace || '-' }}</p></div><el-button :icon="Refresh" @click="load">刷新运行状态</el-button></section>
    <el-alert v-if="topology.refreshError" type="warning" :closable="false" show-icon :title="topology.refreshError" />
    <section class="topology-card">
      <header><div><p class="section-kicker">SERVICE RELATION MAP</p><h2>业务通信链路</h2><p>Contact 统一调度，ZooKeeper 注册与发现；悬停节点突出关联链路，点击进入工作负载详情。</p></div><div class="legend"><span class="legend-item"><i class="public"></i>对外入口</span><span class="legend-item"><i class="management"></i>GM 管理</span><span class="legend-item"><i class="scheduler"></i>Contact 调度</span><span class="legend-item"><i class="dependency"></i>注册发现</span><span class="legend-item"><i class="service"></i>内部服务</span><span class="legend-item"><i class="legend-health ok">✓</i>健康</span><span class="legend-item"><i class="legend-health bad">!</i>异常</span></div></header>
      <div class="topology-toolbar"><div class="relation-modes"><button :class="{active:relationMode==='all'}" @click="relationMode='all'">完整链路</button><button :class="{active:relationMode==='scheduler'}" @click="relationMode='scheduler'">调度链路</button></div><span class="topology-tip">{{ focusedNode ? `${focusedEdges.length} 条关联链路` : '悬停查看关系 · 默认 100% 显示，可手动缩放' }}</span><div class="zoom-controls"><el-button size="small" aria-label="缩小拓扑" @click="changeZoom(-.1)">−</el-button><span>{{ Math.round(zoom*100) }}%</span><el-button size="small" aria-label="放大拓扑" @click="changeZoom(.1)">+</el-button><el-button size="small" @click="originalSize">原始大小</el-button><el-button size="small" @click="fitCanvas">适应窗口</el-button></div></div>
      <div ref="topologyScroll" class="topology-scroll"><div class="topology-surface" :style="{width:`${canvasWidth*zoom}px`,height:`${canvasHeight*zoom}px`}"><div class="canvas" :style="{width:`${canvasWidth}px`,height:`${canvasHeight}px`,transform:`scale(${zoom})`}">
        <div class="coordination-group" :style="{left:`${layout.coordination.x}px`,top:`${layout.coordination.y}px`,width:`${layout.coordination.width}px`,height:`${layout.coordination.height}px`}"><b>中心协作</b><small>调度协调 / 注册发现</small></div>
        <div v-for="group in statefulGroups" :key="group.key" class="stateful-group" :class="group.key" :style="{left:`${group.x}px`,top:`${group.y}px`,width:`${group.width}px`,height:`${group.height}px`}"><b class="group-label">{{ group.key.toUpperCase() }} <span>{{ group.count }} 个工作负载</span></b></div>
        <p v-if="layout.extrasCount" class="other-services-label" :style="{left:`${layout.extrasX}px`,top:`${layout.extrasY-36}px`}">其他内部服务</p>
        <svg :viewBox="`0 0 ${canvasWidth} ${canvasHeight}`" :width="canvasWidth" :height="canvasHeight"><defs><marker v-for="tone in ['default','public','management','scheduler']" :id="`service-arrow-${tone}`" :key="tone" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 Z" :fill="{default:'#8ca1bf',public:'#3989ef',management:'#8862e4',scheduler:'#10a6b0'}[tone]" /></marker></defs><g v-for="edge in visibleEdges" :key="edge.id" :class="['relation-edge',{'is-connected':connected(edge),'is-muted':focusedNode && !connected(edge),'is-secondary':edge.secondary}]"><path :d="edge.path" :class="['edge',edge.tone]" :marker-end="`url(#service-arrow-${edge.tone})`"/><text v-if="edgeLabel(edge)" :x="edge.labelPosition.x" :y="edge.labelPosition.y" text-anchor="middle">{{ edge.label }}</text></g></svg>
        <button v-for="node in nodes" :key="node.id" class="topology-node" :class="[`node-${node.kind}`,{clickable:node.workload,'node-stateful':node.statefulGroup,'is-focused':focusedNode===node.id,'is-muted':focusedNode && focusedNode!==node.id && !linkedNodes.has(node.id)}]" :style="{left:`${node.x}px`,top:`${node.y}px`,width:`${node.width}px`,height:`${node.height}px`}" :title="`${node.title} · ${node.note}`" @mouseenter="focusedNode=node.id" @mouseleave="focusedNode=''" @focus="focusedNode=node.id" @blur="focusedNode=''" @click="openDetail(node)">
          <i v-if="node.workload" class="health-badge" :class="node.healthy ? 'is-ok':'is-bad'">{{node.healthy?'✓':'!'}}</i><span v-if="node.statefulGroup" class="instance-chip">{{node.statefulGroup.toUpperCase()}} #{{node.instance}}</span><span v-if="node.kind==='scheduler' || node.kind==='dependency'" class="role-chip">{{node.kind==='scheduler'?'调度器':'注册中心'}}</span><b>{{node.title}}</b><small>{{node.note}}</small><small v-if="node.workload">{{node.workload.type}} · Ready {{node.workload.ready || '0/0'}}</small>
        </button>
      </div></div></div>
    </section>
    <section class="workload-card"><h2>关联工作负载</h2><el-table :data="workloads" size="small"><el-table-column prop="name" label="工作负载"/><el-table-column prop="type" label="类型" width="120"/><el-table-column prop="ready" label="Ready" width="100"/><el-table-column label="健康" width="100"><template #default="{ row }"><el-tag :type="healthy(row) ? 'success' : 'danger'">{{ healthy(row) ? '✓ 正常' : '! 异常' }}</el-tag></template></el-table-column><el-table-column label="操作" width="110"><template #default="{ row }"><el-button link type="primary" @click="openDetail({ workload: row })">服务详情</el-button></template></el-table-column></el-table></section>
    <el-drawer v-model="detailVisible" size="70%" :with-header="false" destroy-on-close><div v-if="selectedWorkload" class="service-drawer-tabs"><el-button :type="activeDrawerTab === 'detail' ? 'primary' : 'default'" @click="activeDrawerTab = 'detail'">服务详情</el-button><el-button :type="activeDrawerTab === 'logs' ? 'primary' : 'default'" @click="openLogs()">服务日志</el-button><el-button :type="activeDrawerTab === 'monitor' ? 'primary' : 'default'" @click="activeDrawerTab = 'monitor'">Pod 监控</el-button></div><ServiceWorkloadDetail v-if="activeDrawerTab === 'detail'" :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" inline @close="detailVisible = false" @show-logs="openLogs" /><ServiceWorkloadLogs v-else-if="activeDrawerTab === 'logs'" :key="selectedLog?.podName || 'default'" :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" :pod-name="selectedLog?.podName || ''" inline @close="detailVisible = false" /><ServicePodMonitor v-else :service-id="serviceId" :workload-type="selectedWorkload.type" :workload-name="selectedWorkload.name" /></el-drawer>
  </div>
</template>

<style scoped>
.service-topology{padding:16px;background:#f3f7fc;min-height:100%}.topology-header,.topology-card,.workload-card{background:#fff;border:1px solid #dfe9f6;border-radius:18px}.topology-header{display:flex;justify-content:space-between;align-items:center;padding:14px 22px;margin-bottom:12px;background:linear-gradient(120deg,#fff,#eff5ff)}.eyebrow,.section-kicker{font-size:11px;letter-spacing:.1em;color:#4c72dd;font-weight:800;margin:6px 0 3px}.section-kicker{margin:0 0 6px}.topology-header h1{margin:0;color:#102b54;font-size:23px}.topology-header p{margin:5px 0 0;color:#7485a0;font-size:13px}.topology-header code{color:#4168b5}.topology-card{overflow:hidden;margin-top:16px;box-shadow:0 10px 28px rgba(42,76,137,.05)}.topology-card>header{display:flex;justify-content:space-between;align-items:center;gap:20px;padding:14px 22px;border-bottom:1px solid #edf2f8}.topology-card h2,.workload-card h2{margin:0;color:#142e57;font-size:19px}.topology-card p{margin:7px 0 0;color:#7789a4;font-size:13px}.legend{display:flex;gap:8px;align-items:center;flex-wrap:wrap;padding:8px 10px;border:1px solid #edf1f7;border-radius:999px;background:#fbfcff;color:#7586a0;font-size:12px;white-space:nowrap}.legend span{width:8px;height:8px;border-radius:50%;margin-left:4px}.legend .public{background:#3989ef}.legend .management{background:#8a61e8}.legend .service{background:#27b783}.legend-health{width:18px;height:18px;border-radius:50%;display:grid;place-items:center;color:#fff;font-style:normal;font-weight:800}.legend-health.ok{background:#19b889}.legend-health.bad{background:#ef5d65}.topology-scroll{min-height:280px;overflow:auto;padding-bottom:2px}.canvas{position:absolute;left:0;top:0;transform-origin:0 0;background-color:#fcfdff;background-image:radial-gradient(#d8e5f5 .9px,transparent .9px);background-size:18px 18px}.canvas>svg{position:absolute;inset:0;width:100%;height:100%;z-index:1}.edge{fill:none;stroke:#9aacc5;stroke-width:1.45;opacity:.75}.edge.public{stroke:#3989ef;stroke-width:2.2;opacity:.95}.edge.management{stroke:#8862e4;stroke-width:1.7;stroke-dasharray:6 5;opacity:.8}.canvas text{font-size:11px;font-weight:600;fill:#607695;paint-order:stroke;stroke:#fcfdff;stroke-width:5px}.topology-node{position:absolute;z-index:2;box-sizing:border-box;width:196px;height:102px;padding:11px 12px;border:1px solid #d8e4f3;border-radius:15px;background:#fff;box-shadow:0 7px 18px rgba(45,78,132,.08);text-align:left;transition:transform .16s ease,box-shadow .16s ease,border-color .16s ease}.topology-node::before{position:absolute;top:14px;bottom:14px;left:-1px;width:3px;border-radius:3px;background:#8ca1bf;content:''}.topology-node.clickable{cursor:pointer}.topology-node.clickable:hover{transform:translateY(-3px);box-shadow:0 13px 28px rgba(45,78,132,.16)}.topology-node b,.topology-node small{display:block;overflow-wrap:anywhere;word-break:break-word;line-height:1.35}.topology-node b{padding-right:24px;color:#132f59;font-size:13px}.topology-node small{margin-top:4px;color:#7a8da9;font-size:10.5px}.health-badge{position:absolute;right:9px;top:9px;width:18px;height:18px;border-radius:50%;display:grid;place-items:center;color:#fff;font-style:normal;font-size:12px;font-weight:800;line-height:1;box-shadow:0 3px 8px rgba(24,139,103,.25)}.health-badge.is-ok{background:#18b889}.health-badge.is-bad{background:#ef5d65}.node-public{border-color:#a8d2fb;background:linear-gradient(135deg,#f8fcff,#eff8ff)}.node-public::before{background:#3989ef}.node-management{border-color:#c9b5f6;background:linear-gradient(135deg,#fff,#f7f3ff)}.node-management::before{background:#8a61e8}.node-dependency{border-color:#f3ce91;background:linear-gradient(135deg,#fffdf8,#fff7e9)}.node-dependency::before{background:#ec9a2e}.node-service{border-color:#9fdfc4;background:linear-gradient(135deg,#fbfffd,#effcf6)}.node-service::before{background:#28b783}.node-external{border-color:#b8c6d9;background:linear-gradient(135deg,#fff,#f4f7fb)}.node-external::before{background:#7085a4}.workload-card{margin-top:16px;padding:20px}.workload-card h2{margin-bottom:16px}.service-drawer-tabs{position:sticky;top:0;z-index:3;display:flex;gap:10px;padding:14px 24px 0;background:#fff;border-bottom:1px solid #e6edf7}.stateful-group{position:absolute;z-index:0;box-sizing:border-box;padding:10px 12px;border:1px dashed #9dceb9;border-radius:18px;background:linear-gradient(135deg,rgba(235,255,246,.9),rgba(247,255,251,.55))}.stateful-group.world{border-color:#a7bfea;background:linear-gradient(135deg,rgba(241,247,255,.9),rgba(250,252,255,.55))}.node-stateful{width:200px;min-height:116px;padding-top:27px;border-color:#66cda1;background:linear-gradient(135deg,#fcfffd,#effcf6)}.node-stateful .instance-chip{position:absolute;top:9px;left:13px;padding:3px 7px;border-radius:999px;background:#ddf7eb;color:#19825e;font-size:10px;font-weight:800;letter-spacing:.05em}.node-stateful b{font-size:14px}.node-stateful small{margin-top:6px}@media(max-width:1200px){.topology-card>header{align-items:flex-start;gap:12px;flex-direction:column}}@media(max-width:720px){.service-topology{padding:14px}.topology-header{align-items:flex-start;gap:16px;flex-direction:column}}

.topology-surface{position:relative;margin:0 auto}.legend .scheduler{background:#10a6b0}.legend .dependency{background:#ec9a2e}.topology-toolbar{display:flex;align-items:center;gap:16px;padding:9px 20px;border-bottom:1px solid #edf2f8;background:#fcfdff;flex-wrap:wrap}.relation-modes{display:flex;border:1px solid #e1e8f2;border-radius:7px;overflow:hidden}.relation-modes button{border:0;background:#fff;padding:7px 12px;color:#7386a0;cursor:pointer;font-size:12px}.relation-modes button.active{color:#2672d9;background:#edf4ff;font-weight:600}.topology-tip{font-size:12px;color:#8391a6;flex:1}.zoom-controls{display:flex;align-items:center;gap:8px}.zoom-controls .el-button{margin:0}.zoom-controls>span{width:42px;text-align:center;color:#72849c;font-size:12px}.coordination-group{position:absolute;box-sizing:border-box;z-index:0;border:1px dashed #b8dce4;border-radius:22px;background:linear-gradient(180deg,#eefbfcaa,#fff9ee88);padding:10px 20px}.coordination-group>b,.coordination-group>small{display:block;color:#7395a2;font-size:12px;line-height:1.8}.coordination-group>small{font-size:11px;opacity:.8}.group-label{position:absolute;top:10px;left:18px;color:#56947d;font-size:11px;letter-spacing:.05em}.group-label span{font-weight:400;color:#8ca69c;margin-left:10px;letter-spacing:0}.stateful-group.world .group-label{color:#668ab9}.other-services-label{position:absolute;left:24px;margin:0;color:#8b9bb1;font-size:12px;letter-spacing:.08em}.node-scheduler{border-color:#8fd6df;background:linear-gradient(135deg,#fcffff,#eafafb);padding-top:26px}.node-scheduler::before{background:#10a6b0}.node-dependency{padding-top:26px}.role-chip{position:absolute;top:8px;left:11px;font-size:10px;color:#1594a0;font-weight:700;letter-spacing:.06em}.node-dependency .role-chip{color:#bb892f}.relation-edge{transition:opacity .15s}.relation-edge.is-secondary{opacity:.32}.relation-edge.is-muted{opacity:.08}.relation-edge.is-connected{opacity:1}.relation-edge.is-connected .edge{stroke-width:2.4;opacity:1}.edge.scheduler{stroke:#10a6b0;stroke-width:1.8;opacity:.85}.topology-node.is-muted{opacity:.4}.topology-node.is-focused{outline:2px solid #80cbd5;outline-offset:4px}.topology-node small{display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}.topology-node:focus-visible{outline:2px solid #3677ff;outline-offset:4px}
.topology-card>header>div:first-child{min-width:0;flex:1}.legend{max-width:620px;white-space:normal}.legend .legend-item{width:auto;height:auto;margin:0;border-radius:0;display:inline-flex;align-items:center;gap:6px;white-space:nowrap}.legend-item>i:not(.legend-health){width:8px;height:8px;border-radius:50%;display:inline-block}.legend .legend-health{width:18px;height:18px}
</style>
