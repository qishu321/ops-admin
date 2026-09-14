<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Connection, EditPen, Monitor } from '@element-plus/icons-vue'
import { assetDatabaseInfo, assetHostInfo, queryAssetChangeLogs } from '../../api/asset'
import { queryK8sClusterInfo } from '../../api/k8s'
import { useEnvironmentOptions } from '../../composables/useEnvironmentOptions'
import HostMetrics from './HostMetrics.vue'
import DatabaseMetrics from './DatabaseMetrics.vue'

const props = defineProps({ resourceType: { type: String, required: true } })
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const detailReady = ref(false)
const asset = ref({})
const changes = ref([])
const { environmentName } = useEnvironmentOptions()

const config = computed(() => ({
  host: { title: '主机详情', list: '/assets/server/hosts', name: asset.value.hostName, env: asset.value.environment },
  database: { title: '数据库详情', list: '/assets/databases', name: asset.value.name, env: asset.value.env },
  k8s: { title: 'K8s 集群详情', list: '/containers/k8s/clusters', name: asset.value.name, env: asset.value.env }
}[props.resourceType]))

const status = computed(() => {
  if (props.resourceType === 'host') return asset.value.aliveStatus === 1 ? ['在线', 'success'] : ['离线', 'danger']
  if (props.resourceType === 'database') return asset.value.connectStatus === 1 ? ['已连接', 'success'] : asset.value.connectStatus === 2 ? ['连接异常', 'danger'] : ['未检测', 'info']
  return asset.value.status === 'running' ? ['运行中', 'success'] : asset.value.status === 'offline' ? ['离线', 'danger'] : ['异常', 'warning']
})

const basics = computed(() => {
  if (props.resourceType === 'host') return [
    ['主机名', asset.value.hostName], ['SSH 地址', `${asset.value.sshIp || '-'}:${asset.value.sshPort || 22}`],
    ['操作系统', asset.value.os], ['架构', asset.value.arch], ['公网 IP', asset.value.publicIp], ['私网 IP', asset.value.privateIp]
  ]
  if (props.resourceType === 'database') return [
    ['类型', asset.value.dbType], ['连接地址', `${asset.value.host || '-'}:${asset.value.port || 3306}`],
    ['默认库', asset.value.dbName], ['账号', asset.value.username], ['版本', asset.value.version], ['访问模式', asset.value.accessMode === 'readonly' ? '只读' : '读写']
  ]
  return [
    ['API Server', asset.value.apiServer], ['Kubernetes 版本', asset.value.version], ['节点数量', asset.value.nodeCount],
    ['访问方式', asset.value.connectionMode === 'gateway' ? '通过网关' : '直连'], ['最近同步', formatDateTime(asset.value.lastSyncAt)], ['创建时间', formatDateTime(asset.value.createTime)]
  ]
})

const gatewayName = computed(() => asset.value.gateway?.name || asset.value.gatewayName || '-')
const groupNames = computed(() => (asset.value.hostGroups || []).map((item) => item.name).join('、') || asset.value.group?.name || '-')

function formatDateTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false }).replaceAll('/', '-')
}

function actionText(action) {
  return ({ create: '创建', update: '更新', delete: '删除', sync: '同步', test: '连通性检测' })[action] || action
}

async function loadData() {
  loading.value = true
  detailReady.value = false
  asset.value = {}
  try {
    const id = Number(route.params.id)
    const loaders = { host: assetHostInfo, database: assetDatabaseInfo, k8s: queryK8sClusterInfo }
    const [detail, logs] = await Promise.all([
      loaders[props.resourceType](id),
      queryAssetChangeLogs({ resourceType: props.resourceType, resourceId: id, limit: 30 })
    ])
    asset.value = detail || {}
    changes.value = logs || []
  } finally {
    detailReady.value = true
    loading.value = false
  }
}

function enterConsole() {
  if (props.resourceType === 'database') router.push({ name: 'DatabaseWorkbench', params: { id: route.params.id } })
  if (props.resourceType === 'k8s') router.push({ path: '/containers/k8s/overview', query: { clusterId: route.params.id } })
}

watch(() => [props.resourceType, route.params.id], loadData, { immediate: true })
</script>

<template>
  <div v-loading="loading" class="asset-detail-page">
    <header class="detail-header">
      <div class="detail-title">
        <el-button :icon="ArrowLeft" circle title="返回" @click="router.push(config.list)" />
        <div>
          <div class="detail-kicker">{{ config.title }}</div>
          <h2>{{ config.name || '-' }}</h2>
        </div>
        <el-tag :type="status[1]" effect="light">{{ status[0] }}</el-tag>
      </div>
      <el-button v-if="resourceType !== 'host'" type="primary" :icon="Monitor" @click="enterConsole">
        {{ resourceType === 'database' ? '进入 DBMS 工作台' : '进入集群控制台' }}
      </el-button>
    </header>

    <section class="detail-section">
      <div class="section-heading"><h3>基本信息</h3><span>资产身份与连接信息</span></div>
      <el-descriptions :column="3" border>
        <el-descriptions-item v-for="item in basics" :key="item[0]" :label="item[0]">{{ item[1] ?? '-' }}</el-descriptions-item>
        <el-descriptions-item label="所属环境">{{ environmentName(config.env) }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceType === 'host'" label="主机组">{{ groupNames }}</el-descriptions-item>
        <el-descriptions-item label="访问网关"><el-icon><Connection /></el-icon> {{ gatewayName }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceType !== 'database'" label="最近检测">{{ formatDateTime(asset.lastCheckTime || asset.lastSyncAt) }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatDateTime(asset.updateTime) }}</el-descriptions-item>
        <el-descriptions-item v-if="resourceType === 'k8s'" label="标签" :span="3">
          <el-tag v-for="tag in asset.tags || []" :key="tag" class="asset-tag" effect="plain">{{ tag }}</el-tag>
          <span v-if="!asset.tags?.length">-</span>
        </el-descriptions-item>
        <el-descriptions-item label="备注" :span="3">{{ asset.description || '-' }}</el-descriptions-item>
      </el-descriptions>
    </section>

    <HostMetrics v-if="resourceType === 'host'" :host-id="Number(route.params.id)" />
    <DatabaseMetrics v-if="resourceType === 'database' && detailReady" :key="`database-metrics-${route.params.id}`" :database-id="Number(route.params.id)" :enabled="Boolean(asset.monitorEnabled)" />

    <section class="detail-section">
      <div class="section-heading"><h3>最近变更</h3><span>记录关键资产操作，便于追溯</span></div>
      <el-table :data="changes" empty-text="暂无变更记录">
        <el-table-column label="时间" width="190"><template #default="{ row }">{{ formatDateTime(row.createTime) }}</template></el-table-column>
        <el-table-column label="动作" width="110"><template #default="{ row }"><el-tag effect="plain">{{ actionText(row.action) }}</el-tag></template></el-table-column>
        <el-table-column prop="summary" label="变更内容" min-width="280" />
        <el-table-column label="操作人" width="160"><template #default="{ row }"><el-icon><EditPen /></el-icon> {{ row.operator || 'system' }}</template></el-table-column>
      </el-table>
    </section>
  </div>
</template>

<style scoped>
.asset-detail-page { display: flex; flex-direction: column; gap: 16px; }
.detail-header, .detail-section { background: #fff; border: 1px solid #e3eaf5; border-radius: 8px; }
.detail-header { min-height: 92px; padding: 18px 22px; display: flex; align-items: center; justify-content: space-between; }
.detail-title { display: flex; align-items: center; gap: 14px; }
.detail-title h2 { margin: 3px 0 0; color: #0f2445; font-size: 24px; letter-spacing: 0; }
.detail-kicker, .section-heading span { color: #7b8ca8; font-size: 13px; }
.detail-section { padding: 20px; }
.section-heading { display: flex; align-items: baseline; gap: 12px; margin-bottom: 16px; }
.section-heading h3 { margin: 0; color: #13294b; font-size: 17px; letter-spacing: 0; }
.asset-tag { margin-right: 6px; }
</style>
