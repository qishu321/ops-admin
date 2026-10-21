<script setup>
import { computed, ref } from 'vue'
import { Search, User, UserFilled } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: { type: [Number, Array], default: undefined },
  items: { type: Array, default: () => [] },
  multiple: Boolean,
  groups: Boolean,
  disabled: Boolean
})
const emit = defineEmits(['update:modelValue'])
const visible = ref(false), keyword = ref(''), draft = ref([])
const noun = computed(() => props.groups ? '用户组' : '用户')
const selectedIds = computed(() => props.multiple ? props.modelValue || [] : props.modelValue ? [props.modelValue] : [])
const selectedItems = computed(() => props.items.filter(item => selectedIds.value.includes(item.id)))
const filtered = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  return props.items.filter(item => `${item.name} ${item.username || ''} ${item.email || ''} ${item.description || ''}`.toLowerCase().includes(term))
})
const checkedCount = computed(() => filtered.value.filter(item => draft.value.includes(item.id)).length)
const allChecked = computed(() => filtered.value.length > 0 && checkedCount.value === filtered.value.length)
function open() {
  draft.value = [...selectedIds.value]
  keyword.value = ''
  visible.value = true
}
function select(row) {
  draft.value = props.multiple
    ? draft.value.includes(row.id) ? draft.value.filter(id => id !== row.id) : [...draft.value, row.id]
    : [row.id]
}
function toggleAll(checked) {
  const ids = new Set(filtered.value.map(item => item.id))
  draft.value = checked ? [...new Set([...draft.value, ...ids])] : draft.value.filter(id => !ids.has(id))
}
function remove(id) {
  emit('update:modelValue', props.multiple ? selectedIds.value.filter(value => value !== id) : undefined)
}
function confirm() {
  emit('update:modelValue', props.multiple ? [...draft.value] : draft.value[0])
  visible.value = false
}
</script>

<template>
  <div class="subject-picker">
    <div class="picker-trigger">
      <el-button :disabled="disabled" @click="open"><el-icon><UserFilled v-if="groups" /><User v-else /></el-icon>选择{{ noun }}</el-button>
      <span class="picker-count">已选 {{ selectedIds.length }} {{ groups ? '个用户组' : '位用户' }}</span>
    </div>
    <div v-if="selectedItems.length" class="picked-items">
      <el-tag v-for="item in selectedItems" :key="item.id" :closable="!disabled" effect="plain" @close="remove(item.id)">{{ item.name }}<span v-if="item.username && item.username !== item.name" class="picked-account"> · {{ item.username }}</span></el-tag>
    </div>
    <el-dialog v-model="visible" :title="`选择${noun}`" width="min(880px, 94vw)" top="10vh" append-to-body destroy-on-close class="subject-picker-dialog">
      <div class="picker-toolbar">
        <el-input v-model="keyword" :prefix-icon="Search" :placeholder="groups ? '搜索组名或说明' : '搜索姓名、账号或邮箱'" :aria-label="`搜索${noun}`" clearable />
        <span>共 {{ filtered.length }} {{ groups ? '个用户组' : '位用户' }}</span>
      </div>
      <el-table :data="filtered" row-key="id" border max-height="360" :empty-text="keyword ? '没有匹配结果，请调整搜索条件' : `暂无可选${noun}`" :row-class-name="({ row }) => draft.includes(row.id) ? 'is-picked' : ''" @row-click="select">
        <el-table-column width="54" align="center">
          <template #header><el-checkbox v-if="multiple" :model-value="allChecked" :indeterminate="checkedCount > 0 && !allChecked" aria-label="全选搜索结果" @change="toggleAll"><span class="picker-sr-only">全选搜索结果</span></el-checkbox><span v-else>选择</span></template>
          <template #default="{ row }"><span @click.stop><el-checkbox v-if="multiple" :model-value="draft.includes(row.id)" :aria-label="`选择${row.name}`" @change="select(row)"><span class="picker-sr-only">{{ row.name }}</span></el-checkbox><el-radio v-else :model-value="draft[0]" :value="row.id" :aria-label="`选择${row.name}`" @change="draft = [row.id]"><span class="picker-sr-only">{{ row.name }}</span></el-radio></span></template>
        </el-table-column>
        <el-table-column :label="groups ? '用户组名称' : '姓名'" min-width="170"><template #default="{ row }"><span class="picker-name">{{ row.name }}</span></template></el-table-column>
        <el-table-column v-if="!groups" prop="username" label="账号" min-width="140" show-overflow-tooltip />
        <el-table-column v-if="!groups" label="邮箱" min-width="170" show-overflow-tooltip><template #default="{ row }">{{ row.email || '—' }}</template></el-table-column>
        <el-table-column v-if="groups" prop="memberCount" label="成员数量" width="100" />
        <el-table-column v-if="groups" label="说明" min-width="200" show-overflow-tooltip><template #default="{ row }">{{ row.description || '—' }}</template></el-table-column>
        <el-table-column label="状态" width="92"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" effect="plain" size="small">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
      </el-table>
      <template #footer><div class="picker-footer"><span>已选 <strong>{{ draft.length }}</strong> {{ groups ? '个用户组' : '位用户' }}</span><div><el-button @click="visible = false">取消</el-button><el-button type="primary" @click="confirm">确定</el-button></div></div></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.subject-picker { width:100%; }
.picker-trigger { display:flex; align-items:center; gap:12px; }
.picker-trigger .el-icon { margin-right:6px; }
.picker-count,.picker-toolbar > span,.picker-footer > span { color:#7888a4; font-size:13px; }
.picked-items { display:flex; gap:8px; flex-wrap:wrap; margin-top:12px; }
.picked-account { color:#7888a4; }
.picker-toolbar { display:flex; align-items:center; gap:16px; margin-bottom:18px; }
.picker-toolbar .el-input { width:320px; max-width:70%; }
.picker-toolbar > span { white-space:nowrap; }
.picker-name { display:flex; align-items:center; gap:10px; font-weight:500; color:#344460; }
.picker-footer { display:flex; align-items:center; justify-content:space-between; gap:12px; }
.picker-footer strong { color:var(--el-color-primary); }
.picker-sr-only { position:absolute; width:1px; height:1px; overflow:hidden; clip-path:inset(50%); }
:deep(.el-table__row) { cursor:pointer; }
:deep(.is-picked) { --el-table-tr-bg-color:#f2f6ff; }
:deep(.el-radio) { margin-right:0; }
:deep(.el-radio__label) { padding-left:0; }
:global(.subject-picker-dialog .el-dialog__body) { padding-top:20px; padding-bottom:22px; }
:global(.subject-picker-dialog .el-dialog__footer) { border-top:1px solid #e3e8f0; padding-top:16px; }
@media (max-width:600px) { .picker-toolbar { gap:8px; } .picker-toolbar .el-input { width:auto; flex:1; } }
</style>
