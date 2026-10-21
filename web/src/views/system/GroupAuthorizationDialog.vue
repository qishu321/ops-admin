<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { saveGroupBindings } from '../../api/resource-access'
const props = defineProps({ data: { type: Object, required: true }, resources: { type: Object, required: true }, kinds: { type: Object, required: true }, labels: { type: Object, required: true } })
const emit = defineEmits(['saved'])
const visible = ref(false), saving = ref(false), group = ref(null), tab = ref('policy'), keyword = ref(''), draft = ref([]), templateIds = ref([])
const scope = reactive({ resourceType: 'host_group', resourceId: undefined, scope: '' })
const sourcePolicy = id => props.data.grants.find(g => g.id === id)
const sourceTemplate = id => props.data.templates.find(t => t.id === id)
const resourceName = b => props.resources[b.resourceType]?.find(r => r.id === b.resourceId)?.name || `资源 #${b.resourceId}`
const bindingKey = b => JSON.stringify([b.kind, b.sourceId, b.resourceType || '', b.resourceId || 0, b.scope || ''])
const bindingRows = computed(() => draft.value.map(b => {
  const source = b.kind === 'policy' ? sourcePolicy(b.sourceId) : sourceTemplate(b.sourceId)
  return { ...b, key: bindingKey(b), name: source?.name || `已删除的${b.kind === 'policy' ? '策略' : '模板'} #${b.sourceId}`, active: !!source && (b.kind === 'policy' ? source.enabled !== false : source.status === 1), resource: source && b.kind === 'policy' ? source : b }
}))
const direct = computed(() => props.data.grants.filter(g => g.subjectType === 'group' && g.subjectId === group.value?.id))
const policies = computed(() => props.data.grants.filter(g => !(g.subjectType === 'group' && g.subjectId === group.value?.id) && `${g.name || ''} ${resourceName(g)} ${g.scope || ''}`.toLowerCase().includes(keyword.value.trim().toLowerCase())))
const templates = computed(() => props.data.templates.filter(t => t.name.toLowerCase().includes(keyword.value.trim().toLowerCase())))
function open(value) {
  group.value = value
  draft.value = (props.data.bindings || []).filter(b => b.groupId === value.id).map(b => ({ ...b }))
  tab.value = 'policy'; keyword.value = ''; templateIds.value = []
  Object.assign(scope, { resourceType: 'host_group', resourceId: undefined, scope: '' })
  visible.value = true
}
function togglePolicy(row) {
  const existing = draft.value.some(b => b.kind === 'policy' && b.sourceId === row.id)
  draft.value = existing ? draft.value.filter(b => !(b.kind === 'policy' && b.sourceId === row.id)) : [...draft.value, { kind: 'policy', sourceId: row.id }]
}
function toggleTemplate(row) { templateIds.value = templateIds.value.includes(row.id) ? templateIds.value.filter(id => id !== row.id) : [...templateIds.value, row.id] }
function addTemplates() {
  if (!templateIds.value.length || !scope.resourceId) return ElMessage.warning('请选择权限模板和资源范围')
  for (const id of templateIds.value) {
    const b = { kind: 'template', sourceId: id, ...scope, scope: scope.scope.trim() }
    if (!draft.value.some(item => bindingKey(item) === bindingKey(b))) draft.value.push(b)
  }
  templateIds.value = []
}
async function save() {
  if (templateIds.value.length) return ElMessage.warning('请先将所选模板加入绑定，或取消勾选')
  saving.value = true
  try { await saveGroupBindings({ groupId: group.value.id, bindings: draft.value }); visible.value = false; emit('saved'); ElMessage.success('用户组授权绑定已保存') }
  finally { saving.value = false }
}
defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" :title="`绑定授权 · ${group?.name || ''}`" width="min(980px, 96vw)" top="5vh" class="group-binding-dialog" :close-on-click-modal="!saving" :close-on-press-escape="!saving" :show-close="!saving">
    <el-form :disabled="saving" @submit.prevent="save">
      <p class="binding-help">可绑定多个访问策略或权限模板，权限叠加生效。来源变更、停用或删除后，绑定权限同步更新。</p>
      <el-tabs v-model="tab" @tab-change="keyword = ''">
        <el-tab-pane label="访问策略" name="policy">
          <div class="binding-toolbar"><el-input v-model="keyword" clearable placeholder="搜索策略名称、资源或范围" aria-label="搜索可绑定策略" /><span>已选 {{ draft.filter(b => b.kind === 'policy').length }} 条策略</span></div>
          <el-table :data="policies" border max-height="240" row-key="id" empty-text="暂无可绑定策略，请先在访问策略中创建" @row-click="togglePolicy">
            <el-table-column width="52" align="center"><template #default="{ row }"><span @click.stop><el-checkbox :model-value="draft.some(b => b.kind === 'policy' && b.sourceId === row.id)" :aria-label="`绑定策略${row.name}`" @change="togglePolicy(row)"><span class="binding-sr-only">{{ row.name }}</span></el-checkbox></span></template></el-table-column>
            <el-table-column label="策略名称" min-width="170"><template #default="{ row }">{{ row.name || '已有资源授权' }}</template></el-table-column>
            <el-table-column label="资源范围" min-width="180"><template #default="{ row }">{{ resourceName(row) }}<small>{{ kinds[row.resourceType] }} · {{ row.scope || '整个实例' }}</small></template></el-table-column>
            <el-table-column label="允许操作" min-width="210"><template #default="{ row }">{{ row.actions.map(a => labels[a] || a).join('、') }}</template></el-table-column>
            <el-table-column label="状态" width="80"><template #default="{ row }"><el-tag :type="row.enabled === false ? 'info' : 'success'" size="small" effect="plain">{{ row.enabled === false ? '停用' : '启用' }}</el-tag></template></el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane label="权限模板" name="template">
          <div class="binding-toolbar"><el-input v-model="keyword" clearable placeholder="搜索权限模板" aria-label="搜索可绑定模板" /><span>可多选模板，应用于下方资源范围</span></div>
          <el-table :data="templates" border max-height="190" row-key="id" empty-text="暂无可绑定权限模板" @row-click="toggleTemplate">
            <el-table-column width="52" align="center"><template #default="{ row }"><span @click.stop><el-checkbox :model-value="templateIds.includes(row.id)" :aria-label="`绑定模板${row.name}`" @change="toggleTemplate(row)"><span class="binding-sr-only">{{ row.name }}</span></el-checkbox></span></template></el-table-column>
            <el-table-column prop="name" label="模板名称" min-width="160" />
            <el-table-column label="当前范围允许操作" min-width="260"><template #default="{ row }">{{ (row.actions[scope.resourceType] || []).map(a => labels[a] || a).join('、') || '无操作权限' }}</template></el-table-column>
            <el-table-column label="状态" width="80"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="plain">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
          </el-table>
          <div class="template-scope"><el-select v-model="scope.resourceType" aria-label="模板资源类型" @change="scope.resourceId = undefined; scope.scope = ''"><el-option v-for="(name, key) in kinds" :key="key" :label="name" :value="key" /></el-select><el-select v-model="scope.resourceId" filterable placeholder="选择资源" aria-label="模板资源范围"><el-option v-for="r in resources[scope.resourceType] || []" :key="r.id" :label="r.name" :value="r.id" /></el-select><el-input v-if="['k8s', 'datasource'].includes(scope.resourceType)" v-model="scope.scope" :placeholder="scope.resourceType === 'k8s' ? 'Namespace（可选）' : '索引（可选）'" aria-label="模板子范围" /><el-button @click="addTemplates">加入绑定</el-button></div>
          <p class="binding-help">模板仅在所选资源范围内生效，不赋予平台管理权限。可更换范围继续添加。</p>
        </el-tab-pane>
      </el-tabs>
      <div class="binding-section-title"><h3>已绑定授权 <span>{{ draft.length }}</span></h3></div>
      <el-table :data="bindingRows" border max-height="200" row-key="key" empty-text="尚未绑定授权">
        <el-table-column label="绑定来源" min-width="210"><template #default="{ row }">{{ row.name }}<small>{{ row.kind === 'policy' ? '访问策略' : '权限模板' }} · {{ row.active ? '启用' : '来源已停用或删除，不生效' }}</small></template></el-table-column>
        <el-table-column label="资源范围" min-width="240"><template #default="{ row }">{{ resourceName(row.resource) }}<small>{{ kinds[row.resource.resourceType] }} · {{ row.resource.scope || '整个实例' }}</small></template></el-table-column>
        <el-table-column label="操作" width="80"><template #default="{ row }"><el-button link type="danger" :disabled="saving" @click="draft = draft.filter(b => bindingKey(b) !== row.key)">解绑</el-button></template></el-table-column>
      </el-table>
      <p v-if="direct.length" class="binding-help">另有 {{ direct.length }} 条直接授权，继续保留，可在访问策略中编辑或撤销。</p>
    </el-form>
    <template #footer><div class="binding-footer"><span>共绑定 {{ draft.length }} 项授权</span><div><el-button :disabled="saving" @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存绑定</el-button></div></div></template>
  </el-dialog>
</template>

<style scoped>
.binding-help { margin:0 0 14px; color:#7888a4; line-height:1.7; font-size:13px; }
.binding-toolbar { display:flex; align-items:center; gap:16px; margin-bottom:14px; }
.binding-toolbar .el-input { width:320px; max-width:60%; }
.binding-toolbar > span,.binding-footer > span { font-size:13px; color:#7888a4; }
.template-scope { display:flex; flex-wrap:wrap; gap:10px; margin:14px 0 10px; }
.template-scope .el-select { width:220px; flex:1; min-width:140px; }
.template-scope .el-input { width:180px; }
.binding-section-title h3 { margin:20px 0 12px; font-size:15px; color:#243451; }
.binding-section-title span { margin-left:6px; color:var(--el-color-primary); }
.binding-footer { display:flex; align-items:center; justify-content:space-between; }
.binding-sr-only { position:absolute; width:1px; height:1px; overflow:hidden; clip-path:inset(50%); }
small { display:block; color:#7888a4; font-size:12px; line-height:1.6; }
:global(.group-binding-dialog) { max-height:90vh; display:flex; flex-direction:column; }
:global(.group-binding-dialog .el-dialog__body) { overflow:auto; min-height:0; }
:global(.group-binding-dialog .el-dialog__footer) { border-top:1px solid #e3e8f0; padding-top:16px; }
</style>
