<script setup>
import { onActivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, Search, ArrowRight } from '@element-plus/icons-vue'
import { deployList, deployDelete } from '../../../api/k8s-deploy'
import { activeDeployment, deploymentEditable, deploymentStates, formatDeployTime, statusText, deploymentSummary } from '../../../utils/k8s-deploy'
import './deploy.css'
import DeployMark from './DeployMark.vue'

const router = useRouter(), rows = ref([]), total = ref(0), page = ref(1), pageSize = ref(10)
const keyword = ref(''), status = ref(''), loading = ref(false)
let request = 0, timer
async function load(quiet = false) {
  const version = ++request
  if (!quiet) loading.value = true
  try { const result = await deployList({ pageNum: page.value, pageSize: pageSize.value, keyword: keyword.value, status: status.value }); if (version === request) { rows.value = result.list || []; total.value = result.total || 0 } }
  finally { if (version === request) loading.value = false }
}
function search() { page.value = 1; load() }
async function remove(row) {
  await ElMessageBox.confirm(`删除草稿「${row.name}」？`, '删除部署草稿', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }).catch(() => { throw new Error('cancelled') })
  await deployDelete(row.id); ElMessage.success('草稿已删除'); await load()
}
onMounted(() => { load(); timer = setInterval(() => { if (rows.value.some(row => activeDeployment(row.status))) load(true).catch(() => {}) }, 5000) })
onActivated(() => load())
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="k8s-deploy">
    <header class="deploy-heading"><div class="deploy-heading-main"><div class="deploy-heading-symbol"><DeployMark /></div><div><div class="deploy-eyebrow">KUBERNETES / DEPLOYMENT</div><h1>K8s 部署</h1><p>连接你的服务器，构建下一个 Kubernetes 集群。</p></div></div><el-button type="primary" :icon="Plus" size="large" @click="router.push('/containers/k8s-deployments/create')">创建部署</el-button></header>
    <section class="deploy-card deploy-list-card">
      <div class="deploy-list-header"><h2>部署计划<span class="deploy-title-count">{{ total }}</span></h2><div class="deploy-list-filters"><el-input v-model="keyword" :prefix-icon="Search" placeholder="搜索部署名称" clearable @keyup.enter="search" @clear="search" /><el-select v-model="status" placeholder="全部状态" clearable @change="search"><el-option v-for="(value, key) in deploymentStates" :key="key" :label="value[0]" :value="key" /></el-select><el-button :icon="Refresh" aria-label="刷新部署列表" @click="load()" /></div></div>
      <div class="deploy-list-body">
      <el-table v-loading="loading" :data="rows" row-key="id">
        <el-table-column label="集群 / 部署名称" min-width="240"><template #default="{ row }"><div class="deploy-cluster-cell"><div class="deploy-cluster-icon"><DeployMark /></div><div><el-button class="deploy-cluster-name" link @click="router.push(`/containers/k8s-deployments/${row.id}`)">{{ row.name }}</el-button><div class="deploy-help">{{ row.description || `计划 #${row.id} · ${row.env || '未分配环境'}` }}</div></div></div></template></el-table-column>
        <el-table-column label="部署状态" min-width="120"><template #default="{ row }"><span class="deploy-status" :class="row.status">{{ statusText(row.status) }}</span></template></el-table-column>
        <el-table-column label="Kubernetes" min-width="115"><template #default="{ row }"><span class="deploy-mono deploy-version">v{{ row.config?.version }}</span></template></el-table-column>
        <el-table-column label="节点构成" min-width="160"><template #default="{ row }"><div class="deploy-node-pair"><span><strong>{{ deploymentSummary(row).masters }}</strong>控制平面</span><span><strong>{{ deploymentSummary(row).workers }}</strong>工作节点</span></div></template></el-table-column>
        <el-table-column label="集群接入" min-width="110"><template #default="{ row }"><span v-if="row.clusterId" class="deploy-status-text">已接入</span><el-tag v-else-if="row.registrationStatus === 'failed'" type="warning" effect="plain">接入失败</el-tag><span v-else class="deploy-help">{{ row.registrationStatus === 'registering' ? '接入中' : '待接入' }}</span></template></el-table-column>
        <el-table-column label="创建时间" min-width="175"><template #default="{ row }">{{ formatDeployTime(row.createTime) }}<div class="deploy-help">{{ row.createdBy }}</div></template></el-table-column>
        <el-table-column label="操作" width="155" fixed="right"><template #default="{ row }"><el-button v-if="deploymentEditable(row.status)" link type="primary" @click="router.push(`/containers/k8s-deployments/${row.id}/edit`)">配置</el-button><el-button link type="primary" @click="router.push(`/containers/k8s-deployments/${row.id}`)">详情<el-icon style="margin-left:3px"><ArrowRight /></el-icon></el-button><el-button v-if="row.status === 'draft' && !row.lastRunId" link type="danger" @click="remove(row).catch(() => {})">删除</el-button></template></el-table-column>
        <template #empty><div class="deploy-empty"><div class="deploy-empty-icon"><DeployMark /></div><h3>从第一台服务器开始</h3><p>关联 CMDB 节点，配置网络与组件，在这里跟踪每一次集群部署。</p><el-button type="primary" :icon="Plus" @click="router.push('/containers/k8s-deployments/create')">创建第一个部署</el-button></div></template>
      </el-table>
      <div class="deploy-pagination"><span class="deploy-page-note">CMDB 资产关联 · 配置与执行记录统一保留</span><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[10, 20, 50]" layout="total, sizes, prev, pager, next" @current-change="load()" @size-change="search" /></div>
      </div>
    </section>
  </div>
</template>
