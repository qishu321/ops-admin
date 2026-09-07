<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryAssetServiceWorkloadTopology, rollbackAssetServiceWorkload } from '../../api/asset'

const props = defineProps({ serviceId: { type: Number, default: 0 }, workloadType: String, workloadName: String, inline: Boolean })
const emit = defineEmits(['show-logs'])
const route = useRoute(); const router = useRouter(); const loading = ref(false); const data = ref({})
const params = computed(() => ({ serviceId: props.serviceId || Number(route.query.serviceId), workloadType: props.workloadType || route.query.workloadType, workloadName: props.workloadName || route.query.workloadName }))
const hasWorkloadContext = computed(() => Boolean(params.value.serviceId && params.value.workloadType && params.value.workloadName))
const workload = computed(() => data.value.workload || {}); const services = computed(() => data.value.services || []); const replicaSets = computed(() => data.value.replicaSets || [])
const isStatefulSet = computed(() => String(workload.value.type || params.value.workloadType || '').toLowerCase() === 'statefulset')
const statefulSet = computed(() => data.value.statefulSet || null)
const runtimePods = computed(() => isStatefulSet.value ? (statefulSet.value?.pods || workload.value.pods || []) : replicaSets.value.flatMap((item) => item.pods || []))
const rollbackSaving = ref(false)
const resourceDetail = ref(null)
const canRollback = computed(() => String(workload.value.type || params.value.workloadType || '').toLowerCase() === 'deployment')
function healthy(item) { if (typeof item?.healthy === 'boolean') return item.healthy; const [ready, expected] = String(item?.ready || '').split('/').map(Number); return expected > 0 && ready === expected && Number(item.available) >= expected }
function viewLogs(pod) { const target = { ...params.value, podName: pod.name }; if (props.inline) emit('show-logs', target); else router.push({ path: '/containers/services/logs', query: target }) }
function openResourceDetail(kind, item) { resourceDetail.value = { kind, item } }
function closeResourceDetail() { resourceDetail.value = null }
function resourcePods() { const item = resourceDetail.value?.item; return Array.isArray(item?.pods) && item.pods.length ? item.pods : runtimePods.value }
function viewResourceLogs() { const pod = resourcePods()[0]; if (pod) viewLogs(pod) }
async function load() { if (!hasWorkloadContext.value) { data.value = {}; return }; loading.value = true; try { data.value = await queryAssetServiceWorkloadTopology(params.value) } finally { loading.value = false } }
async function rollbackRevision(row) { if (!row.revision || row.current || rollbackSaving.value) return; await ElMessageBox.confirm(`将 ${workload.value.name || params.value.workloadName} 回滚至 ReplicaSet ${row.name}（修订版本 ${row.revision}）。Kubernetes 会创建一次新的发布版本，确认继续？`, '确认回滚版本', { type: 'warning', confirmButtonText: '确认回滚', cancelButtonText: '取消' }); rollbackSaving.value = true; try { await rollbackAssetServiceWorkload({ ...params.value, revision: row.revision }); ElMessage.success(`已提交回滚到版本 ${row.revision}`); await load() } finally { rollbackSaving.value = false } }
onMounted(load)
</script>

<template>
  <div class="service-detail" v-loading="loading">
    <div class="detail-toolbar"><span>&#26381;&#21153;&#36164;&#28304;&#35814;&#24773;</span><el-button link :icon="Refresh" @click="load">&#21047;&#26032;&#29366;&#24577;</el-button></div>
    <el-empty v-if="!hasWorkloadContext" description="请选择服务和工作负载后查看运行时资源详情" />
    <section v-else class="argo-canvas">
      <div class="argo-column root-column">
        <article class="argo-card root-card"><span class="resource-icon root-icon">&#9671;</span><div><b>{{ workload.name || params.workloadName }}</b><small>{{ workload.type || params.workloadType }}</small><span class="health-line"><i :class="healthy(workload) ? 'ok' : 'bad'"><template v-if="healthy(workload)">&#10003;</template><template v-else>!</template></i>{{ healthy(workload) ? 'Healthy' : 'Degraded' }}</span></div><em class="node-menu">&#8942;</em></article>
      </div>
      <div class="connector connector-root"></div>
      <div class="argo-column workload-column">
        <button v-for="service in services" :key="service.name" type="button" class="argo-card service-card interactive-card" title="查看 Service 详情" @click="openResourceDetail('Service', service)"><span class="resource-icon svc-icon">&#8862;</span><div><b>{{ service.name }}</b><small>SVC / {{ service.type }}</small><span class="health-line"><i class="ok">&#10003;</i>{{ service.clusterIP || 'ClusterIP' }}</span></div><em class="node-menu">&#8942;</em></button>
        <article v-if="!services.length" class="argo-card muted-card"><span class="resource-icon svc-icon">&#8862;</span><div><b>暂无匹配 Service</b><small>{{ isStatefulSet ? '可通过 Headless Service / Pod DNS 发现' : 'Service selector 未匹配工作负载标签' }}</small></div></article>
        <button type="button" class="argo-card deploy-card interactive-card" :class="{ 'stateful-controller-card': isStatefulSet }" :title="`查看 ${isStatefulSet ? 'StatefulSet' : 'Deployment'} 详情`" @click="openResourceDetail(isStatefulSet ? 'StatefulSet' : 'Deployment', workload)"><span class="resource-icon deploy-icon">&#10227;</span><div><b>{{ workload.name || params.workloadName }}</b><small>{{ isStatefulSet ? 'STATEFULSET' : 'DEPLOYMENT' }}</small><span class="health-line"><i :class="healthy(workload) ? 'ok' : 'bad'"><template v-if="healthy(workload)">&#10003;</template><template v-else>!</template></i>Ready {{ workload.ready || '0/0' }}</span></div><em class="node-menu">&#8942;</em></button>
      </div>
      <div class="connector connector-deploy"></div>
      <div class="argo-column rs-column">
        <template v-if="isStatefulSet">
          <button v-if="statefulSet" type="button" class="argo-card rs-card stateful-runtime-card interactive-card" :class="healthy(statefulSet) ? 'healthy' : 'unhealthy'" title="查看 StatefulSet 运行详情" @click="openResourceDetail('StatefulSet', statefulSet)"><span class="resource-icon rs-icon">&#9634;</span><div><b>{{ statefulSet.name }}</b><small>StatefulSet · {{ statefulSet.age }}</small><span class="health-line"><i :class="healthy(statefulSet) ? 'ok' : 'bad'"><template v-if="healthy(statefulSet)">&#10003;</template><template v-else>!</template></i>Ready {{ statefulSet.ready }}</span></div><em class="node-menu">&#8942;</em></button>
          <el-empty v-else description="暂无 StatefulSet 运行数据" />
        </template>
        <template v-else>
          <button v-for="replicaSet in replicaSets" :key="replicaSet.name" type="button" class="argo-card rs-card rs-version-card interactive-card" :class="[healthy(replicaSet) ? 'healthy' : 'unhealthy', { 'is-rollback-target': canRollback && replicaSet.revision && !replicaSet.current }]" title="查看 ReplicaSet 详情" @click="openResourceDetail('ReplicaSet', replicaSet)"><span class="resource-icon rs-icon">&#9634;</span><div><b>{{ replicaSet.name }}</b><small>RS{{ replicaSet.revision ? ` / v${replicaSet.revision}` : '' }} · {{ replicaSet.age }}</small><span class="health-line"><i :class="healthy(replicaSet) ? 'ok' : 'bad'"><template v-if="healthy(replicaSet)">&#10003;</template><template v-else>!</template></i>Ready {{ replicaSet.ready }}</span></div><span class="node-menu" aria-hidden="true">&#8942;</span><span v-if="replicaSet.current" class="current-version">当前</span></button>
          <el-empty v-if="!replicaSets.length" description="暂无 ReplicaSet 运行数据" />
        </template>
      </div>
      <div class="connector connector-pods"></div>
      <div class="argo-column pod-column">
        <button v-for="pod in runtimePods" :key="pod.name" type="button" class="argo-card pod-card interactive-card" :class="String(pod.status).toLowerCase() === 'running' ? 'healthy' : 'unhealthy'" title="查看此 Pod 日志" @click="viewLogs(pod)"><span class="resource-icon pod-icon">&#9670;</span><div><b>{{ pod.name }}</b><small>POD / {{ pod.status }}</small><span class="health-line"><i :class="String(pod.status).toLowerCase() === 'running' ? 'ok' : 'bad'"><template v-if="String(pod.status).toLowerCase() === 'running'">&#10003;</template><template v-else>!</template></i>{{ pod.restarts }} restarts / {{ pod.node || 'unassigned' }}</span></div><em class="node-menu">&#8942;</em></button>
        <div v-if="!runtimePods.length" class="no-pod">{{ isStatefulSet ? '暂无 StatefulSet Pod' : '暂无运行中的 Pod' }}</div>
      </div>
    </section>
    <el-dialog :model-value="Boolean(resourceDetail)" :title="`${resourceDetail?.kind || '资源'} 详情`" width="520px" append-to-body @close="closeResourceDetail">
      <el-descriptions v-if="resourceDetail" :column="1" border>
        <el-descriptions-item label="名称">{{ resourceDetail.item.name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ resourceDetail.kind }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.revision" label="修订版本">v{{ resourceDetail.item.revision }}<el-tag v-if="resourceDetail.item.current" size="small" type="success" class="detail-tag">当前</el-tag></el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.type" label="访问类型">{{ resourceDetail.item.type }}</el-descriptions-item>
        <el-descriptions-item label="就绪状态">{{ resourceDetail.item.ready || (healthy(resourceDetail.item) ? 'Healthy' : 'Degraded') }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.available !== undefined" label="可用副本">{{ resourceDetail.item.available }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.clusterIP" label="Cluster IP">{{ resourceDetail.item.clusterIP }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.age" label="创建至今">{{ resourceDetail.item.age }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceDetail.item.pods?.length" label="关联 Pod">{{ resourceDetail.item.pods.map((pod) => pod.name).join('、') }}</el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="closeResourceDetail">关闭</el-button>
        <el-button v-if="resourcePods().length" type="primary" @click="viewResourceLogs">查看日志</el-button>
        <el-button v-if="resourceDetail?.kind === 'ReplicaSet' && canRollback && resourceDetail.item.revision && !resourceDetail.item.current" type="warning" :loading="rollbackSaving" @click="rollbackRevision(resourceDetail.item).then(closeResourceDetail)">回滚至此版本</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.service-detail { min-height: 100%; padding: 22px; background: #f4f7fc; }
.detail-toolbar { display: flex; justify-content: space-between; align-items: center; padding: 0 4px 14px; color: #6d809e; font-size: 13px; }
.argo-canvas { position: relative; display: grid; grid-template-columns: 250px 60px 320px 60px minmax(0, 1fr) 60px minmax(0, 1fr); gap: 0; min-width: 1280px; min-height: 520px; padding: 54px 46px; overflow: hidden; border: 1px solid #dbe6f1; border-radius: 16px; background: #f4f7fc; }
.argo-column { position: relative; z-index: 2; display: flex; min-width: 0; flex-direction: column; justify-content: center; gap: 22px; }.root-column { justify-content: flex-start; padding-top: 88px; }.workload-column { justify-content: flex-start; padding-top: 38px; }.rs-column,.pod-column { justify-content: flex-start; gap: 16px; }
.argo-card { position: relative; display: flex; min-width: 0; align-items: center; gap: 13px; min-height: 70px; padding: 12px 36px 12px 16px; border: 0; border-radius: 6px; background: #fff; box-shadow: 0 2px 10px rgba(55, 81, 104, .16); text-align: left; }.argo-card div { display: grid; min-width: 0; gap: 3px; }.argo-card b { overflow: hidden; color: #1d385e; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }.argo-card small { color: #7386a3; font-size: 11px; }.resource-icon { display: grid; flex: 0 0 34px; width: 34px; height: 34px; place-items: center; color: #91a8b8; font-size: 28px; line-height: 1; }.root-icon { width: 46px; height: 46px; border-radius: 50%; background: #91a8b8; color: #fff; font-size: 24px; }.svc-icon { color: #829ead; }.deploy-icon { color: #829ead; font-size: 38px; }.rs-icon { color: #829ead; }.pod-icon { color: #829ead; }.interactive-card { cursor: pointer; transition: transform .16s ease, box-shadow .16s ease, outline-color .16s ease; }.interactive-card:hover { box-shadow: 0 8px 20px rgba(47, 91, 145, .2); transform: translateY(-2px); }.interactive-card:focus-visible { outline: 2px solid #3d72ff; outline-offset: 3px; }
.health-line { display: flex; align-items: center; gap: 4px; color: #68809e; font-size: 11px; }.health-line i { display: grid; width: 15px; height: 15px; place-items: center; border-radius: 50%; color: #fff; font-style: normal; font-size: 10px; font-weight: 800; }.health-line i.ok { background: #16bb8a; }.health-line i.bad { background: #f15d64; }.node-menu { position: absolute; top: 18px; right: 12px; color: #0da4b7; font-size: 24px; font-style: normal; line-height: 1; }.service-card { border-left: 3px solid #14aabd; }.deploy-card { border-left: 3px solid #7890a0; }.rs-card { border-left: 3px solid #8960eb; }.rs-card.unhealthy { border-left-color: #f25f67; }.pod-card { width: 100%; cursor: pointer; }.pod-card.healthy { border-left: 3px solid #14b98b; background: #e5ffff; }.pod-card.unhealthy { border-left: 3px solid #f25f67; }.muted-card { opacity: .72; }
.connector { position: relative; align-self: stretch; background-image: repeating-linear-gradient(to bottom, transparent 0, transparent 5px, #9aaebf 5px, #9aaebf 7px, transparent 7px, transparent 12px); background-size: 2px 100%; background-repeat: no-repeat; background-position: center; }.connector::before,.connector::after { position: absolute; left: 50%; width: 50%; height: 1px; border-top: 1px dashed #9aaebf; content: ''; }.connector-root::before { top: 150px; }.connector-root::after { top: 238px; }.connector-deploy::before { top: 210px; }.connector-deploy::after { top: 340px; }.connector-pods::before { top: 72px; }.connector-pods::after { bottom: 72px; }.no-pod { padding: 16px; border-radius: 6px; background: rgba(255,255,255,.75); color: #8293a9; font-size: 12px; }
.rs-version-card { width: 100%; border-top: 0; border-right: 0; border-bottom: 0; }.rs-version-card.is-rollback-target:hover { background: #fffaf0; }.current-version { position: absolute; top: 10px; right: 31px; padding: 2px 5px; border-radius: 3px; background: #e8f8f2; color: #13a374; font-size: 10px; font-weight: 700; }.detail-tag { margin-left: 8px; }
.stateful-controller-card,.stateful-runtime-card { border-left-color: #22aa81; }.stateful-runtime-card { width: 100%; background: #f3fffa; }
@media (max-width: 1280px) { .service-detail { overflow-x: auto; } }
</style>
