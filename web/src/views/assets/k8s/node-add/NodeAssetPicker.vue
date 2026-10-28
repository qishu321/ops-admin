<script setup>
import { computed, onMounted, ref } from 'vue'
import { nodeAddHosts } from '../../../../api/k8s-node-add'
const props = defineProps({ clusterId: Number, selected: { type: Array, default: () => [] }, environments: { type: Array, default: () => [] }, groups: { type: Array, default: () => [] } })
const emit = defineEmits(['pick', 'remove'])
const rows = ref([]), page = ref(1), total = ref(0), loading = ref(false), error = ref('')
const keyword = ref(''), environment = ref(''), groupId = ref('')
let version = 0
const chosen = row => props.selected.some(n => n.hostId === row.id)
const available = computed(() => rows.value.filter(row => !row.disabledReason))
const allChecked = computed(() => available.value.length > 0 && available.value.every(chosen))
async function load() {
  const current = ++version; loading.value = true; error.value = ''
  try {
    const data = await nodeAddHosts({ clusterId: props.clusterId, keyword: keyword.value, environment: environment.value, groupId: groupId.value, pageNum: page.value, pageSize: 5 })
    if (current === version) { rows.value = data.list; total.value = data.total }
  } catch (e) { if (current === version) { rows.value = []; error.value = '资产加载失败，请重试' } }
  finally { if (current === version) loading.value = false }
}
function search() { page.value = 1; load() }
function toggle(row, value) { if (value) emit('pick', [row]); else emit('remove', row.id) }
function togglePage(value) { if (value) emit('pick', available.value); else available.value.forEach(row => emit('remove', row.id)) }
onMounted(load)
</script>
<template>
  <div class="na-asset-filters">
    <el-input v-model="keyword" placeholder="搜索主机名称 / IP" aria-label="搜索主机名称或IP" clearable @keyup.enter="search" @clear="search" />
    <el-select v-model="environment" placeholder="全部环境" aria-label="筛选资产环境" clearable @change="search"><el-option v-for="env in environments" :key="env.code" :label="env.name" :value="env.code" /></el-select>
    <el-select v-model="groupId" placeholder="全部主机组" aria-label="筛选主机组" clearable @change="search"><el-option v-for="group in groups" :key="group.id" :label="group.name" :value="group.id" /></el-select>
    <el-button @click="search">搜索</el-button>
  </div>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <el-table v-loading="loading" :data="rows" class="na-assets" row-key="id" empty-text="没有匹配的服务器，请调整筛选条件">
    <el-table-column width="42"><template #header><el-checkbox :model-value="allChecked" :disabled="!available.length" aria-label="选择本页可用服务器" @change="togglePage" /></template><template #default="{ row }"><el-checkbox :model-value="chosen(row)" :disabled="!!row.disabledReason" :aria-label="`选择 ${row.hostName}`" @change="value => toggle(row, value)" /></template></el-table-column>
    <el-table-column label="服务器 / IP" min-width="164"><template #default="{ row }"><b>{{ row.hostName }}</b><small class="na-mono">{{ row.privateIp || row.sshIp || row.publicIp }}</small></template></el-table-column>
    <el-table-column label="配置 / 系统" min-width="180"><template #default="{ row }">{{ row.cpu }} 核 · {{ row.memory }}<small>{{ row.os || '待识别' }} · {{ row.arch }}</small></template></el-table-column>
    <el-table-column label="访问与认证" min-width="150"><template #default="{ row }">{{ row.connectionMode === 'gateway' ? `网关 ${row.gatewayName}` : '直连' }}<small>{{ row.credentialName || '未配置凭据' }}</small></template></el-table-column>
    <el-table-column label="可用情况" min-width="130"><template #default="{ row }"><span v-if="row.disabledReason" class="na-unavailable">{{ row.disabledReason }}</span><span v-else class="na-available">可关联</span></template></el-table-column>
  </el-table>
  <div class="na-pagination"><span>已选 <b>{{ selected.length }}</b> 台 · 支持跨页选择</span><el-pagination v-model:current-page="page" :page-size="5" :total="total" layout="prev, pager, next" @current-change="load" /></div>
</template>
