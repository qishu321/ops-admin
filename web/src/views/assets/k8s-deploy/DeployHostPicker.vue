<script setup>
import { computed, ref, watch } from 'vue'
import { deployHosts } from '../../../api/k8s-deploy'
import { queryAssetHostGroupList } from '../../../api/asset'
import { useEnvironmentOptions } from '../../../composables/useEnvironmentOptions'

const props = defineProps({ modelValue: Boolean, existing: { type: Array, default: () => [] }, planId: { type: Number, default: 0 } })
const emit = defineEmits(['update:modelValue', 'confirm'])
const { environmentOptions } = useEnvironmentOptions()
const keyword = ref(''), environment = ref(''), groupId = ref(''), groups = ref([])
const rows = ref([]), total = ref(0), page = ref(1), loading = ref(false), selected = ref(new Map())
let request = 0
const visible = computed({ get: () => props.modelValue, set: v => emit('update:modelValue', v) })
const alreadySelected = (row) => props.existing.some(n => n.hostId === row.id)
const reason = (row) => alreadySelected(row) ? '已关联' : row.disabledReason || (row.occupiedPlanId && row.occupiedPlanId !== props.planId ? `已被部署 #${row.occupiedPlanId} 占用` : '')
const available = computed(() => rows.value.filter(row => !reason(row)))
const allChecked = computed(() => available.value.length > 0 && available.value.every(row => selected.value.has(row.id)))
function selectRow(row, checked) { const next = new Map(selected.value); if (checked) next.set(row.id, row); else next.delete(row.id); selected.value = next }
function selectAll(checked) { available.value.forEach(row => selectRow(row, checked)) }
async function load() {
  const version = ++request; loading.value = true
  try {
    const result = await deployHosts({ keyword: keyword.value, environment: environment.value, groupId: groupId.value || undefined, pageNum: page.value, pageSize: 10 })
    if (version === request) { rows.value = result.list || []; total.value = result.total || 0 }
  } finally { if (version === request) loading.value = false }
}
function search() { page.value = 1; load() }
watch(() => props.modelValue, async open => {
  if (!open) return
  selected.value = new Map(); page.value = 1
  await Promise.all([load(), queryAssetHostGroupList().then(data => { groups.value = data.list || [] })])
})
function confirm() { emit('confirm', [...selected.value.values()]); visible.value = false }
</script>

<template>
  <el-dialog v-model="visible" class="deploy-host-dialog" title="从 CMDB 关联节点" width="min(1180px, 94vw)" destroy-on-close>
    <p class="deploy-help" style="margin:0 0 20px">选择已有服务器加入部署计划。系统、认证和网关均自动关联，安装前将重新核验。</p>
    <div class="deploy-toolbar">
      <div class="filters">
        <el-input v-model="keyword" placeholder="主机名称 / IP" clearable @keyup.enter="search" @clear="search" />
        <el-select v-model="environment" placeholder="全部环境" clearable @change="search"><el-option v-for="env in environmentOptions" :key="env.code" :label="env.name" :value="env.code" /></el-select>
        <el-select v-model="groupId" placeholder="全部主机组" clearable @change="search"><el-option v-for="group in groups" :key="group.id" :label="group.name" :value="group.id" /></el-select>
        <el-button @click="search">搜索</el-button>
      </div>
      <span class="deploy-selected-count">已选择<strong>{{ selected.size }}</strong>台服务器</span>
    </div>
    <el-table v-loading="loading" :data="rows" row-key="id" max-height="440">
      <el-table-column width="45"><template #header><el-checkbox :model-value="allChecked" :disabled="!available.length" aria-label="选择本页可用主机" @change="selectAll" /></template><template #default="{ row }"><el-checkbox :model-value="selected.has(row.id) || alreadySelected(row)" :disabled="Boolean(reason(row))" :aria-label="`选择 ${row.hostName}`" @change="value => selectRow(row, value)" /></template></el-table-column>
      <el-table-column label="主机名称 / IP" min-width="180"><template #default="{ row }"><strong>{{ row.hostName }}</strong><div class="deploy-help deploy-mono">{{ row.privateIp || row.sshIp || row.publicIp }}</div></template></el-table-column>
      <el-table-column label="配置 / 系统" min-width="210"><template #default="{ row }"><span>{{ row.cpu || '—' }} 核 / {{ row.memory || '—' }}</span><div class="deploy-help">{{ row.os || '待预检识别' }}</div></template></el-table-column>
      <el-table-column label="访问与认证" min-width="150"><template #default="{ row }">{{ row.connectionMode === 'gateway' ? `网关：${row.gatewayName}` : '直连' }}<div class="deploy-help">{{ row.credentialName || '未配置凭据' }} · {{ row.authType || '—' }}</div></template></el-table-column>
      <el-table-column label="可用情况" min-width="170"><template #default="{ row }"><span v-if="reason(row)" class="deploy-help">{{ reason(row) }}</span><el-tag v-else type="success" effect="plain">可关联 · 待预检</el-tag></template></el-table-column>
    </el-table>
    <div class="deploy-pagination"><span class="deploy-page-note">支持跨页选择 · 已关联节点不会重复添加</span><el-pagination v-model:current-page="page" :total="total" :page-size="10" layout="total, prev, pager, next" @current-change="load" /></div>
    <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :disabled="!selected.size" @click="confirm">关联 {{ selected.size || '' }} 台主机</el-button></template>
  </el-dialog>
</template>
