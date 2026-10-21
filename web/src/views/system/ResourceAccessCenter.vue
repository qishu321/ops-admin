<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoute } from 'vue-router'
import AccessSubjectPicker from './AccessSubjectPicker.vue'
import GroupAuthorizationDialog from './GroupAuthorizationDialog.vue'
const route = useRoute()
import { getResourceAccessData, getResourceAccessOptions, saveUserGroup, deleteUserGroup, saveAccessPolicy, getEffectiveAccess, deleteResourceGrant } from '../../api/resource-access'

const props = defineProps({ section: { type: String, default: 'grants' } })
const data = reactive({ groups: [], members: [], users: [], grants: [], actions: {}, templates: [], bindings: [] })
const resources = ref({})
const loading = ref(false), saving = ref(false), groupDialog = ref(false), grantDialog = ref(false)
const bindingDialog = ref(null)
const effective = ref(null), previewLoading = ref(false)
let previewRequest = 0
const keyword = ref(''), resourceType = ref(''), subjectType = ref(''), previewUserId = ref()
const groupForm = reactive({ id: 0, name: '', description: '', status: 1, memberIds: [] })
const grantForm = reactive({ name: '', enabled: true, templateId: undefined, id: 0, subjectType: 'group', subjectId: undefined, resourceType: 'host_group', resourceId: undefined, scope: '', actions: ['read'], description: '' })
const kinds = { host: '主机', host_group: '主机组', k8s: 'K8s 集群', database: '数据库 / Redis', middleware: '中间件', datasource: '监控 / 日志数据源', credential: '凭据' }
const labels = { read: '查看', terminal: '终端登录', upload: '上传文件', download: '下载文件', execute: '执行命令 / 脚本', query: '查询', write: '写入 / 变更', export: '导出', backup: '备份', restore: '恢复', restart: '重启', scale: '伸缩', logs: '查看日志', delete: '删除资源', use: '使用', manage: '全部资源操作' }
const usersById = computed(() => Object.fromEntries(data.users.map(u => [u.id, u])))
const groupsById = computed(() => Object.fromEntries(data.groups.map(g => [g.id, g])))
const isGroups = computed(() => props.section === 'groups')
const userOptions = computed(() => data.users.map(u => ({ ...u, name: u.nickname || u.username })))
const subjectOptions = computed(() => grantForm.subjectType === 'group' ? data.groups.map(g => ({ ...g, memberCount: membersOf(g.id).length })) : userOptions.value)
const actionOptions = computed(() => data.actions[grantForm.resourceType] || [])
const rows = computed(() => data.grants.filter(g => (!resourceType.value || g.resourceType === resourceType.value) && (!subjectType.value || g.subjectType === subjectType.value) && `${g.name || ''} ${subjectName(g)} ${resourceName(g)} ${g.scope || ''} ${g.description || ''}`.toLowerCase().includes(keyword.value.toLowerCase())))
const groupRows = computed(() => data.groups.filter(g => `${g.name} ${g.description}`.toLowerCase().includes(keyword.value.toLowerCase())))
const preview = computed(() => effective.value?.grants || [])
async function loadPreview() {
 const request = ++previewRequest
 effective.value = null
 if (!previewUserId.value) { previewLoading.value = false; return }
 previewLoading.value = true
 try { const value = await getEffectiveAccess(previewUserId.value); if (request === previewRequest) effective.value = value }
 finally { if (request === previewRequest) previewLoading.value = false }
}
function applyTemplate() {
 const template = data.templates.find(t => t.id === grantForm.templateId)
 if (template) grantForm.actions = [...(template.actions[grantForm.resourceType] || ['read'])]
}

function userName(u) { return u ? `${u.nickname || u.username} · ${u.username}` : '用户已删除' }
function subjectName(g) { return g.subjectType === 'user' ? userName(usersById.value[g.subjectId]) : groupsById.value[g.subjectId]?.name || '用户组已删除' }
function resourceName(g) { return resources.value[g.resourceType]?.find(r => r.id === g.resourceId)?.name || `资源 #${g.resourceId}` }
function membersOf(id) { return data.members.filter(m => m.groupId === id).map(m => usersById.value[m.adminId]).filter(Boolean) }
function grantsOf(id) { return data.grants.filter(g => g.subjectType === 'group' && g.subjectId === id).length + (data.bindings || []).filter(b => b.groupId === id).length }
async function load() {
  loading.value = true
  try { const [value, options] = await Promise.all([getResourceAccessData(), getResourceAccessOptions()]); Object.assign(data, value); resources.value = options || {} } finally { loading.value = false }
}
function openGroup(g) { Object.assign(groupForm, { id: 0, name: '', description: '', status: 1, memberIds: [] }, g || {}, { memberIds: g ? membersOf(g.id).map(u => u.id) : [] }); groupDialog.value = true }
function openGrant(g, groupId) { Object.assign(grantForm, { unified: false, name: '', enabled: true, templateId: undefined, id: 0, subjectType: 'group', subjectId: groupId, resourceType: 'host_group', resourceId: undefined, scope: '', actions: ['read'], description: '' }, g || {}, { name: g ? g.name || `${resourceName(g)}访问策略` : '', enabled: g?.enabled !== false, templateId: data.templates.some(t => t.id === g?.templateId && t.status === 1) ? g.templateId : undefined }); grantForm.actions = [...(g?.actions || ['read'])]; grantDialog.value = true }
function changeKind() { grantForm.resourceId = undefined; grantForm.scope = ''; grantForm.actions = ['read']; applyTemplate() }
async function submitGroup() {
  if (!groupForm.name.trim()) return ElMessage.warning('请输入用户组名称')
  saving.value = true
  try { await saveUserGroup(groupForm); groupDialog.value = false; await load(); ElMessage.success('用户组已保存') } finally { saving.value = false }
}
async function submitGrant() {
  if (!grantForm.name.trim()) return ElMessage.warning('请输入策略名称')
  if (!grantForm.subjectId || !grantForm.resourceId) return ElMessage.warning('请选择授权对象和资源')
  if (!grantForm.actions.length) return ElMessage.warning('至少选择一项操作')
  saving.value = true
  try { await saveAccessPolicy(grantForm); grantDialog.value = false; await load(); await loadPreview(); ElMessage.success('访问策略已保存') } finally { saving.value = false }
}
async function removeGroup(g) {
  await ElMessageBox.confirm(`删除“${g.name}”会同时撤销该组的 ${grantsOf(g.id)} 条授权，成员的个人授权仍会保留。`, '删除用户组', { type: 'warning', confirmButtonText: '删除用户组' })
  await deleteUserGroup(g.id); await load(); ElMessage.success('用户组已删除')
}
async function revoke(g) {
  await ElMessageBox.confirm(`撤销“${subjectName(g)}”对“${resourceName(g)}”的这条授权？其他授权来源仍然有效。`, '撤销授权', { type: 'warning', confirmButtonText: '撤销授权' })
  await deleteResourceGrant(g.id); await load(); await loadPreview(); ElMessage.success('授权已撤销')
}
watch(() => props.section, () => { keyword.value = '' })
watch(previewUserId, loadPreview)
watch(() => route.query.userId, id => { previewUserId.value = id ? Number(id) : undefined })
onMounted(async () => { await load(); if (route.query.userId) previewUserId.value = Number(route.query.userId) })
</script>

<template>
  <div class="access-center" v-loading="loading">
    <header class="access-heading page-card">
      <div class="heading-symbol"><el-icon :size="25"><UserFilled v-if="isGroups" /><Lock v-else /></el-icon></div>
      <div class="heading-copy"><span class="access-eyebrow">{{ isGroups ? 'TEAM ACCESS' : 'ACCESS POLICIES' }}</span><h1>{{ isGroups ? '用户组' : '访问策略' }}</h1><p>{{ isGroups ? '将协作成员放在一起，统一授予资源访问权限。' : '一次配置授权对象、资源范围和操作，功能入口随策略自动生效。' }}</p></div>
      <div class="heading-actions"><el-button @click="load">刷新</el-button><el-button type="primary" @click="isGroups ? openGroup() : openGrant()"><el-icon><Plus /></el-icon>{{ isGroups ? '新增用户组' : '新增策略' }}</el-button></div>
    </header>

    <section v-if="isGroups" class="page-card">
      <div class="access-toolbar"><h2>协作用户组</h2><el-input v-model="keyword" placeholder="搜索组名或说明" clearable aria-label="搜索用户组" class="search-input" /></div>
      <div v-if="groupRows.length" class="group-grid">
        <article v-for="g in groupRows" :key="g.id" class="user-group-card">
          <div class="group-card-top"><span class="group-monogram">{{ g.name.slice(0, 1) }}</span><el-tag :type="g.status === 1 ? 'success' : 'info'" effect="plain">{{ g.status === 1 ? '启用' : '已停用' }}</el-tag></div>
          <h3>{{ g.name }}</h3><p class="group-description">{{ g.description || '暂无说明' }}</p>
          <div class="member-stack"><span v-for="u in membersOf(g.id).slice(0, 5)" :key="u.id" :title="userName(u)">{{ (u.nickname || u.username).slice(0, 1) }}</span><small>{{ membersOf(g.id).length }} 位成员 · {{ grantsOf(g.id) }} 条授权</small></div>
          <div class="group-card-actions"><el-button link type="primary" @click="openGroup(g)">编辑成员</el-button><el-button link type="primary" @click="bindingDialog.open(g)">绑定授权</el-button><el-button link type="danger" @click="removeGroup(g)">删除</el-button></div>
        </article>
      </div>
      <el-empty v-else description="创建用户组，将团队成员统一授权"><el-button type="primary" @click="openGroup()">新增用户组</el-button></el-empty>
    </section>

    <template v-else>
      <section class="page-card grants-section">
        <div class="access-toolbar"><h2>访问策略</h2><div class="access-filters"><el-input v-model="keyword" placeholder="搜索对象、资源或范围" clearable aria-label="搜索授权" /><el-select v-model="subjectType" placeholder="全部授权对象" clearable aria-label="授权对象类型"><el-option label="用户组" value="group" /><el-option label="用户" value="user" /></el-select><el-select v-model="resourceType" placeholder="全部资源类型" clearable aria-label="资源类型筛选"><el-option v-for="(name, key) in kinds" :key="key" :label="name" :value="key" /></el-select></div></div>
        <el-table :data="rows" row-key="id" empty-text="暂无匹配的策略，点击右上角新增策略">
          <el-table-column label="策略" min-width="190"><template #default="{ row }"><strong class="resource-title">{{ row.name || '已有资源授权' }}</strong><small class="resource-scope">{{ row.enabled === false ? '已停用' : '启用中' }} · {{ row.unified ? '统一策略' : '兼容模式' }}</small></template></el-table-column>
          <el-table-column label="授权对象" min-width="190"><template #default="{ row }"><div class="subject-cell"><span class="subject-icon"><el-icon><UserFilled v-if="row.subjectType === 'group'" /><User v-else /></el-icon></span><div><strong>{{ subjectName(row) }}</strong><small>{{ row.subjectType === 'group' ? '用户组' : '直接授权' }}<span v-if="row.subjectType === 'group' && groupsById[row.subjectId]?.status !== 1"> · 已停用，不生效</span></small></div></div></template></el-table-column>
          <el-table-column label="资源与范围" min-width="230"><template #default="{ row }"><strong class="resource-title">{{ resourceName(row) }}</strong><small class="resource-scope">{{ kinds[row.resourceType] }} · {{ row.scope || (row.resourceType === 'host_group' ? '本组及所有下级组' : '整个实例') }}</small></template></el-table-column>
          <el-table-column label="允许操作" min-width="260"><template #default="{ row }"><div class="permission-tags"><el-tag v-for="a in row.actions" :key="a" :type="a === 'manage' ? 'warning' : 'info'" effect="plain" size="small">{{ labels[a] || a }}</el-tag></div></template></el-table-column>
          <el-table-column label="说明" prop="description" min-width="140" show-overflow-tooltip />
          <el-table-column label="操作" width="110" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openGrant(row)">编辑</el-button><el-button link type="danger" @click="revoke(row)">撤销</el-button></template></el-table-column>
        </el-table>
      </section>
      <section class="page-card effective-section" v-loading="previewLoading">
 <div class="access-toolbar"><div><h2>用户有效权限</h2><p>查看实际生效的操作、授权来源及只读限制。</p></div><el-select v-model="previewUserId" filterable clearable placeholder="选择用户查看权限" aria-label="查看用户的有效授权"><el-option v-for="u in data.users" :key="u.id" :label="userName(u)" :value="u.id" /></el-select></div>
 <template v-if="effective">
 <el-alert v-if="!effective.enabled" type="warning" :closable="false" title="账号已停用，当前无法登录或访问资源。" />
 <el-alert v-else-if="effective.super" :type="effective.readOnly ? 'warning' : 'success'" :closable="false" :title="effective.readOnly ? '平台管理员同时具有全局只读限制，只允许查看、查询和日志。' : '平台管理员：拥有全部资源访问与系统管理权限。'" />
 <template v-else><el-alert v-if="effective.readOnly" type="warning" :closable="false" title="全局只读限制生效：终端、上传及变更等操作不会开放。" />
 <div class="effective-list"><div v-for="entry in preview" :key="entry.grant.bindingId ? `binding-${entry.grant.bindingId}` : `grant-${entry.grant.id}`" class="grant-flow"><span>{{ entry.grant.subjectType === 'user' ? '个人策略' : subjectName(entry.grant) }}<small v-if="entry.grant.bindingId">{{ entry.grant.bindingKind === 'template' ? '绑定模板' : '绑定策略' }} · {{ entry.grant.name }}</small></span><el-icon><Right /></el-icon><div><strong>{{ resourceName(entry.grant) }}</strong><small class="resource-scope">{{ entry.grant.scope || '整个实例' }}</small></div><div class="flow-actions"><span>{{ entry.allowed.map(a => labels[a] || a).join('、') || '暂无生效操作' }}</span><small v-if="entry.blocked.length" class="blocked-actions">受限：{{ entry.blocked.map(a => labels[a] || a).join('、') }}</small></div></div><el-empty v-if="!preview.length" :image-size="56" description="暂无生效的资源访问策略" /></div>
 <p v-if="effective.legacyRoles?.some(r => !['member', 'super-admin'].includes(r.roleKey))" class="field-help">兼容角色：{{ effective.legacyRoles.filter(r => !['member', 'super-admin'].includes(r.roleKey)).map(r => r.roleName + (r.status === 1 ? '' : '（停用）')).join('、') }}。原有功能权限继续保留。</p>
 </template></template><p v-else-if="!previewUserId" class="preview-placeholder">选择一位用户，查看其个人策略和用户组继承结果。</p>
 </section>
    </template>

    <GroupAuthorizationDialog ref="bindingDialog" :data="data" :resources="resources" :kinds="kinds" :labels="labels" @saved="load" />
    <el-dialog class="access-edit-dialog" top="5vh" v-model="groupDialog" :title="groupForm.id ? '编辑用户组' : '新增用户组'" width="min(620px, 94vw)" :close-on-click-modal="!saving" :show-close="!saving" :close-on-press-escape="!saving">
      <el-form label-position="top" :disabled="saving" @submit.prevent="submitGroup"><el-form-item label="用户组名称" required><el-input v-model="groupForm.name" maxlength="80" placeholder="例如：测试环境开发组" /></el-form-item><el-form-item label="组内成员"><AccessSubjectPicker v-model="groupForm.memberIds" :items="userOptions" multiple :disabled="saving" /></el-form-item><el-form-item label="状态"><el-radio-group v-model="groupForm.status"><el-radio :value="1">启用</el-radio><el-radio :value="2">停用</el-radio></el-radio-group><span class="field-help">停用后，该组授权不再生效。</span></el-form-item><el-form-item label="说明"><el-input v-model="groupForm.description" type="textarea" :rows="3" maxlength="160" placeholder="说明团队职责和使用范围" /></el-form-item></el-form>
      <template #footer><el-button :disabled="saving" @click="groupDialog = false">取消</el-button><el-button type="primary" :loading="saving" @click="submitGroup">保存用户组</el-button></template>
    </el-dialog>

    <el-dialog class="access-edit-dialog" top="5vh" v-model="grantDialog" :title="grantForm.id ? '编辑访问策略' : '新增访问策略'" width="min(780px, 94vw)" :close-on-click-modal="!saving" :show-close="!saving" :close-on-press-escape="!saving">
      <el-form label-position="top" :disabled="saving" @submit.prevent="submitGrant">
        <div class="form-columns"><el-form-item label="策略名称" required><el-input v-model="grantForm.name" maxlength="42" placeholder="例如：测试环境日常排障" /></el-form-item><el-form-item label="状态"><el-switch v-model="grantForm.enabled" active-text="启用" inactive-text="停用" /></el-form-item></div>
        <el-alert v-if="grantForm.id && !grantForm.unified" type="info" :closable="false" class="legacy-note">这条授权使用原有角色功能权限。保存后将转为统一访问策略，所选操作的功能入口会自动开放。</el-alert>
        <el-form-item label="从权限模板开始（可选）"><el-select v-model="grantForm.templateId" clearable placeholder="手动配置，或复制一个模板的操作" style="width:100%" @change="applyTemplate"><el-option v-for="t in data.templates.filter(t => t.status === 1)" :key="t.id" :label="t.name" :value="t.id" /></el-select><span class="field-help">只复制当前资源类型的操作，可继续调整；后续修改模板不会改变已保存策略。</span></el-form-item>
        <div class="grant-form-section"><h3><el-icon><UserFilled /></el-icon>授权给谁</h3><el-radio-group v-model="grantForm.subjectType" @change="grantForm.subjectId = undefined"><el-radio-button value="group">用户组</el-radio-button><el-radio-button value="user">指定用户</el-radio-button></el-radio-group><el-form-item label="授权对象" required><AccessSubjectPicker :key="grantForm.subjectType" v-model="grantForm.subjectId" :items="subjectOptions" :groups="grantForm.subjectType === 'group'" :disabled="saving" /></el-form-item></div>
        <div class="grant-form-section"><h3><el-icon><Box /></el-icon>选择资源范围</h3><div class="form-columns"><el-form-item label="资源类型" required><el-select v-model="grantForm.resourceType" style="width:100%" @change="changeKind"><el-option v-for="(name, key) in kinds" :key="key" :label="name" :value="key" /></el-select></el-form-item><el-form-item label="资源" required><el-select v-model="grantForm.resourceId" filterable placeholder="请选择资源" style="width:100%"><el-option v-for="r in resources[grantForm.resourceType] || []" :key="r.id" :label="r.name" :value="r.id" /></el-select></el-form-item></div><el-form-item v-if="['k8s', 'datasource'].includes(grantForm.resourceType)" :label="grantForm.resourceType === 'k8s' ? '指定 Namespace（可选）' : '指定日志索引（可选）'"><el-input v-model="grantForm.scope" :placeholder="grantForm.resourceType === 'k8s' ? '留空覆盖整个集群，填写后仅授权该命名空间' : '留空覆盖整个数据源；Elasticsearch 可填写具体索引，不支持通配符'" maxlength="255" /><span class="field-help">需要多个范围时，分别添加授权。</span></el-form-item><p v-if="grantForm.resourceType === 'host_group'" class="scope-note">覆盖当前主机组及下级组，新增的组内主机会自动继承授权。</p><p v-if="grantForm.resourceType === 'database'" class="scope-note">按连接实例授权，适用于数据库及 Redis 等已纳管的中间件。</p></div>
        <div class="grant-form-section"><h3><el-icon><Key /></el-icon>允许的操作</h3><el-checkbox-group v-model="grantForm.actions" class="action-grid"><el-checkbox v-for="a in actionOptions" :key="a" :value="a" :disabled="a === 'read'" border>{{ labels[a] || a }}</el-checkbox></el-checkbox-group><p class="field-help">查看权限随授权保留；终端、查询和变更操作需要单独勾选。</p></div>
        <el-form-item label="授权说明"><el-input v-model="grantForm.description" maxlength="160" placeholder="例如：测试环境日常排障" /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="saving" @click="grantDialog = false">取消</el-button><el-button type="primary" :loading="saving" @click="submitGrant">保存策略</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
:global(.access-edit-dialog) { max-height:90vh; display:flex; flex-direction:column; }
:global(.access-edit-dialog .el-dialog__body) { overflow:auto; min-height:0; }
:global(.access-edit-dialog .el-dialog__footer) { border-top:1px solid #e3e8f0; padding-top:16px; }

.legacy-note{margin-bottom:18px}.blocked-actions{display:block;color:#bb7337;margin-top:5px}.effective-list{margin-top:16px}.access-center { --access-blue:#356ae6; --access-ink:#18243a; --access-muted:#66758d; --access-border:#e3e8f0; display:grid; grid-template-columns:minmax(0,1fr); min-width:0; gap:20px; }
.access-center > * { min-width:0; }
.access-heading { display:flex; align-items:center; gap:16px; padding:26px 28px; }
.heading-symbol { width:52px; height:52px; border-radius:14px; display:grid; place-items:center; color:var(--access-blue); background:#edf3ff; }
.heading-copy { flex:1; }.access-eyebrow { color:var(--access-blue); font-size:11px; font-weight:700; letter-spacing:1.2px; }
h1 { margin:5px 0 7px; font-size:25px; letter-spacing:-.5px; }p { margin:0; color:var(--access-muted); line-height:1.7; }.heading-actions { display:flex; gap:8px; }.heading-actions .el-icon { margin-right:5px; }
.access-toolbar { display:flex; align-items:center; gap:16px; justify-content:space-between; margin-bottom:20px; }h2 { font-size:16px; margin:0; }.access-toolbar p { font-size:12px; margin-top:7px; }.search-input { max-width:270px; }.access-filters { display:flex; gap:10px; }.access-filters .el-input { width:230px; }.access-filters .el-select { width:170px; }
.group-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:18px; }.user-group-card { padding:22px; border:1px solid var(--access-border); border-radius:12px; background:#fff; transition:border-color .15s; }.user-group-card:hover { border-color:#9bb8f4; }.group-card-top { display:flex; justify-content:space-between; align-items:center; }.group-monogram { display:grid; place-items:center; width:40px; height:40px; background:#edf3ff; color:var(--access-blue); border-radius:11px; font-size:20px; font-weight:650; }h3 { font-size:16px; margin:17px 0 9px; }.group-description { min-height:44px; font-size:12px; overflow-wrap:anywhere; }.member-stack { display:flex; align-items:center; margin:20px 0; }.member-stack > span { width:28px; height:28px; border-radius:50%; background:#eaf0fa; border:2px solid white; display:grid; place-items:center; color:#4a6083; margin-left:-5px; font-size:11px; }.member-stack > span:first-child { margin-left:0; }.member-stack small { margin-left:10px; color:var(--access-muted); }.group-card-actions { border-top:1px solid #edf0f6; padding-top:15px; display:flex; justify-content:space-between; }
.subject-cell { display:flex; gap:10px; align-items:center; }.subject-icon { width:32px; height:32px; border-radius:8px; background:#edf3ff; color:var(--access-blue); display:grid; place-items:center; }.subject-cell strong,.resource-title { font-size:13px; font-weight:600; }.subject-cell small,.resource-scope { display:block; margin-top:5px; font-size:11px; color:var(--access-muted); }.permission-tags { display:flex; flex-wrap:wrap; gap:5px; }.effective-section .el-select { width:280px; }.preview-placeholder { padding:15px; text-align:center; background:#f8faff; border-radius:8px; font-size:12px; }.effective-list { display:grid; gap:8px; }.grant-flow { display:flex; align-items:center; gap:12px; border:1px solid var(--access-border); border-radius:8px; padding:12px 16px; font-size:12px; }.grant-flow > span:first-child { color:#3464b6; background:#edf3ff; padding:4px 8px; border-radius:5px; }.grant-flow small { color:var(--access-muted); }.flow-actions { margin-left:auto; color:var(--access-muted); }
.grant-form-section { border:1px solid var(--access-border); border-radius:10px; padding:18px; margin-bottom:16px; }.grant-form-section h3 { display:flex; align-items:center; gap:8px; margin:0 0 16px; font-size:14px; }.grant-form-section h3 .el-icon { color:var(--access-blue); }.grant-form-section .el-radio-group { margin-bottom:18px; }.grant-form-section .el-form-item:last-child { margin-bottom:0; }.form-columns { display:grid; grid-template-columns:1fr 1fr; gap:16px; }.field-help { display:block; font-size:12px; color:var(--access-muted); margin-top:8px; line-height:1.65; }.scope-note { padding:10px 12px; background:#f5f8ff; border-radius:6px; font-size:12px; }.action-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:10px; }.action-grid .el-checkbox { margin:0; }.action-grid .el-checkbox.is-bordered { width:100%; }.action-grid :deep(.el-checkbox__label) { font-size:12px; }
@media(max-width:1000px) { .access-toolbar,.access-heading { flex-wrap:wrap; }.access-filters { flex-wrap:wrap; }.heading-copy { min-width:200px; }.grant-flow { flex-wrap:wrap; } }
@media(max-width:600px) { .access-heading { padding:20px; }.form-columns { grid-template-columns:1fr; gap:0; }.action-grid { grid-template-columns:repeat(2,1fr); }.access-filters .el-input,.access-filters .el-select { width:100%; }.heading-actions { width:100%; }.group-grid { grid-template-columns:1fr; }.effective-section .el-select { width:100%; } }
@media(prefers-reduced-motion:reduce) { .user-group-card { transition:none; } }
</style>
