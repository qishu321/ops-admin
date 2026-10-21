<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Filter, Plus, Search } from '@element-plus/icons-vue'
import { listCommandFilters, saveCommandFilter, deleteCommandFilter } from '../../api/command-filter'
const rules = ref([]), loading = ref(false), saving = ref(false), dialog = ref(false), keyword = ref(''), status = ref('')
const form = reactive({ id: 0, name: '', matchType: 'keyword', pattern: '', scopes: ['terminal', 'job'], enabled: false, description: '' })
const filtered = computed(() => rules.value.filter(r => `${r.name} ${r.pattern} ${r.description}`.toLowerCase().includes(keyword.value.trim().toLowerCase()) && (status.value === '' || r.enabled === status.value)))
const enabledCount = computed(() => rules.value.filter(r => r.enabled).length)
const scopeName = r => r.scopes.length === 2 ? '全局' : r.scopes.includes('terminal') ? '终端' : '作业'
async function load() { loading.value = true; try { rules.value = await listCommandFilters() || [] } finally { loading.value = false } }
function open(row) { Object.assign(form, { id: 0, name: '', matchType: 'keyword', pattern: '', scopes: ['terminal', 'job'], enabled: false, description: '' }, row || {}, { scopes: [...(row?.scopes || ['terminal', 'job'])] }); dialog.value = true }
async function save() {
  if (!form.name.trim() || !form.pattern.trim() || !form.scopes.length) return ElMessage.warning('请填写名称、匹配内容并选择适用范围')
  saving.value = true
  try { await saveCommandFilter({ ...form }); dialog.value = false; await load(); ElMessage.success(form.enabled ? '规则已保存并启用' : '规则已保存，启用后生效') } finally { saving.value = false }
}
async function toggle(row) {
  try { await saveCommandFilter({ ...row, enabled: !row.enabled }); await load(); ElMessage.success(row.enabled ? '规则已停用' : '规则已启用') } catch { /* HTTP layer displays errors; no optimistic status change. */ }
}
async function remove(row) { await ElMessageBox.confirm(`删除规则“${row.name}”？`, '删除命令过滤规则', { type: 'warning' }); await deleteCommandFilter(row.id); await load() }
onMounted(load)
</script>

<template>
  <div class="command-filter-page" v-loading="loading">
    <header class="filter-header"><div class="filter-symbol"><el-icon :size="26"><Filter /></el-icon></div><div><span class="eyebrow">COMMAND FILTER</span><h1>命令过滤</h1><p>设置终端与作业的命令拦截规则，仅平台管理员可见和维护。</p></div><el-button type="primary" :icon="Plus" @click="open()">新增规则</el-button></header>
    <div class="filter-summary"><span>全部规则 <strong>{{ rules.length }}</strong></span><span><i />已启用 <strong>{{ enabledCount }}</strong></span><span>未启用 <strong>{{ rules.length - enabledCount }}</strong></span><p>命中启用规则即拦截；未命中的高危作业操作仍需二次确认。</p></div>
    <section class="filter-panel">
      <div class="filter-toolbar"><el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索规则名称、匹配内容" aria-label="搜索命令过滤规则" /><el-select v-model="status" aria-label="规则状态" placeholder="全部状态"><el-option label="全部状态" value="" /><el-option label="已启用" :value="true" /><el-option label="未启用" :value="false" /></el-select><el-button @click="load">刷新</el-button></div>
      <el-table :data="filtered" row-key="id" empty-text="暂无规则，新增并启用后开始拦截">
        <el-table-column label="规则名称" min-width="170"><template #default="{ row }"><strong>{{ row.name }}</strong><small>{{ row.description || '暂无说明' }}</small></template></el-table-column>
        <el-table-column label="匹配内容" min-width="230"><template #default="{ row }"><code>{{ row.pattern }}</code><small>{{ row.matchType === 'regex' ? '正则表达式' : '关键词 · 任一命中即拦截' }}</small></template></el-table-column>
        <el-table-column label="适用范围" width="150"><template #default="{ row }"><el-tag :type="row.scopes.length === 2 ? 'primary' : 'info'" effect="plain">{{ scopeName(row) }}</el-tag></template></el-table-column>
        <el-table-column label="状态" width="120"><template #default="{ row }"><el-switch :model-value="row.enabled" :aria-label="`${row.enabled ? '停用' : '启用'}规则${row.name}`" @change="toggle(row)" /></template></el-table-column>
        <el-table-column label="操作" width="140"><template #default="{ row }"><el-button link type="primary" @click="open(row)">编辑</el-button><el-button link type="danger" @click="remove(row)">删除</el-button></template></el-table-column>
      </el-table>
    </section>
    <div class="filter-note"><strong>适用范围</strong><p>终端：SSH 终端输入（不包含 Pod 终端）；作业：命令执行、脚本执行，以及作业编排、定时任务中的脚本。两项都选即全局生效。</p><p>按提交文本匹配，不解析远端别名、外部文件或程序运行后的行为。历史记录和 Tab 补全可正常使用，不会作为额外拦截条件。</p></div>
    <el-dialog v-model="dialog" class="command-filter-dialog" top="5vh" :title="form.id ? '编辑过滤规则' : '新增过滤规则'" width="min(660px, 94vw)" :close-on-click-modal="!saving" :close-on-press-escape="!saving" :show-close="!saving">
      <el-form label-position="top" :disabled="saving" @submit.prevent="save">
        <el-form-item label="规则名称" required><el-input v-model="form.name" maxlength="60" placeholder="例如：禁止格式化磁盘" /></el-form-item>
        <el-form-item label="适用范围" required><el-checkbox-group v-model="form.scopes"><el-checkbox value="terminal" border>终端</el-checkbox><el-checkbox value="job" border>作业（命令 / 脚本）</el-checkbox></el-checkbox-group><small class="field-help">两项都选即全局生效，对所有账号适用，包括平台管理员。</small></el-form-item>
        <el-form-item label="匹配方式"><el-radio-group v-model="form.matchType"><el-radio-button value="keyword">关键词</el-radio-button><el-radio-button value="regex">正则表达式</el-radio-button></el-radio-group></el-form-item>
        <el-form-item label="匹配内容" required><el-input v-model="form.pattern" type="textarea" :rows="3" :placeholder="form.matchType === 'keyword' ? '每行一个，或用逗号、顿号分隔。例如：rm -rf、mkfs、shutdown' : 'Go 正则，例如：(?i)\\bmkfs\\b'" /><small class="field-help">{{ form.matchType === 'keyword' ? '支持换行、英文逗号、中文逗号或顿号分隔；任一关键词命中即拦截，忽略大小写与连续空白。' : '正则表达式使用 Go RE2 语法，整体匹配，不按分隔符拆分。' }}</small></el-form-item>
        <el-form-item label="状态"><el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" /><small class="field-help">停用规则不参与匹配。新规则默认停用。</small></el-form-item>
        <el-form-item label="说明"><el-input v-model="form.description" maxlength="160" placeholder="规则用途和适用场景" /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="saving" @click="dialog = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存规则</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
:global(.command-filter-dialog) { max-height:90vh; display:flex; flex-direction:column; }
:global(.command-filter-dialog .el-dialog__body) { min-height:0; overflow:auto; }
:global(.command-filter-dialog .el-dialog__footer) { border-top:1px solid #e3e8f0; padding-top:16px; }
.command-filter-page { display:grid; gap:20px; color:#243451; }
.filter-header,.filter-panel,.filter-summary,.filter-note { background:#fff; border:1px solid #e2e9f4; border-radius:12px; }
.filter-header { padding:26px; display:flex; align-items:center; gap:18px; }
.filter-symbol { width:52px; height:52px; display:grid; place-items:center; color:#4b6deb; background:#edf2ff; border-radius:12px; }
.eyebrow { font-size:11px; letter-spacing:1.2px; color:#4b6deb; font-weight:700; }
h1 { margin:4px 0 8px; font-size:25px; } p { margin:0; color:#7888a4; line-height:1.7; font-size:13px; }
.filter-header > .el-button { margin-left:auto; }
.filter-summary { display:flex; align-items:center; gap:26px; padding:18px 24px; color:#7888a4; font-size:13px; flex-wrap:wrap; }
.filter-summary strong { margin-left:8px; color:#243451; font-size:20px; }
.filter-summary i { display:inline-block; width:7px; height:7px; border-radius:50%; background:#21b98b; margin-right:6px; }
.filter-summary p { margin-left:auto; }
.filter-panel { padding:22px; }
.filter-toolbar { display:flex; gap:12px; margin-bottom:20px; }
.filter-toolbar .el-input { width:340px; max-width:60%; }.filter-toolbar .el-select { width:150px; }
small { display:block; margin-top:6px; color:#7888a4; font-size:12px; line-height:1.6; }
code { display:inline-block; max-width:100%; white-space:pre-wrap; overflow-wrap:anywhere; color:#4d5d7d; background:#f5f7fc; border:1px solid #e8edf6; padding:4px 8px; border-radius:5px; }
.filter-note { padding:18px 24px; background:#f8faff; }.filter-note strong { font-size:13px; }.filter-note p { margin-top:5px; }
.field-help { width:100%; }
@media(max-width:700px) { .filter-header { flex-wrap:wrap; padding:20px; }.filter-header > .el-button { margin-left:0; }.filter-summary { gap:16px; }.filter-summary p { margin-left:0; }.filter-panel { padding:14px; } }
</style>
