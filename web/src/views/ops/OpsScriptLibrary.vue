<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  addOpsScript,
  deleteOpsScript,
  opsScriptInfo,
  queryOpsScriptVersions,
  queryOpsScriptList,
  rollbackOpsScript,
  updateOpsScript,
  updateOpsScriptStatus
} from '../../api/ops'

const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const isEdit = ref(false)
const tableData = ref([])
const total = ref(0)
const scriptEditorRef = ref(null)
const scriptScrollTop = ref(0)
const scriptScrollLeft = ref(0)
const versionVisible = ref(false)
const versionLoading = ref(false)
const versionList = ref([])
const versionScript = ref(null)
const scriptTimeoutPresets = [
  { label: '30 秒', value: 30 },
  { label: '1 分钟', value: 60 },
  { label: '5 分钟（推荐）', value: 300 },
  { label: '10 分钟', value: 600 },
  { label: '30 分钟', value: 1800 }
]

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  keyword: '',
  status: ''
})

const form = reactive({
  id: undefined,
  name: '',
  scriptType: 'shell',
  interpreter: 'bash',
  content: '',
  variables: [],
  timeoutSeconds: 300,
  status: 1,
  description: '',
  changeSummary: ''
})

const scriptLineNumbers = computed(() => {
  const totalLines = Math.max(1, (form.content || '').split('\n').length)
  return Array.from({ length: totalLines }, (_, index) => index + 1)
})

const highlightedScript = computed(() => highlightScript(form.content || '', form.scriptType))
const timeoutPreset = computed({
  get: () => scriptTimeoutPresets.some((item) => item.value === form.timeoutSeconds) ? form.timeoutSeconds : 'custom',
  set: (value) => {
    if (value !== 'custom') form.timeoutSeconds = Number(value)
  }
})
const timeoutDescription = computed(() => {
  const seconds = Number(form.timeoutSeconds) || 0
  if (seconds < 60) return `${seconds} 秒`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
})

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    scriptType: 'shell',
    interpreter: 'bash',
    content: '',
    variables: [],
    timeoutSeconds: 300,
    status: 1,
    description: '',
    changeSummary: ''
  })
}

function compatibleInterpreter(scriptType, interpreter) {
  const allowed = scriptType === 'python' ? ['python', 'python3'] : ['bash', 'sh']
  return allowed.includes(interpreter) ? interpreter : (scriptType === 'python' ? 'python' : 'bash')
}

function handleScriptTypeChange(scriptType) {
  form.interpreter = scriptType === 'python' ? 'python' : 'bash'
}

function addVariable() {
  form.variables.push({ name: '', defaultValue: '', description: '', required: false, secret: false })
}

function removeVariable(index) { form.variables.splice(index, 1) }

function variableDisplay(variable) {
  const name = `VARIABLE_${variable.name || ''}`
  if (variable.secret) return `${name}=******`
  if (String(variable.defaultValue || '').trim()) return `${name}=${variable.defaultValue}`
  return `${name}${variable.required ? '（必填）' : ''}`
}

function normalizeVariableName(variable) {
  variable.name = String(variable.name || '').toUpperCase().replace(/[^A-Z0-9_]/g, '')
}

watch(
  () => dialogVisible.value,
  (visible) => {
    if (visible) {
      nextTick(() => {
        scriptEditorRef.value?.focus()
      })
    } else {
      scriptScrollTop.value = 0
      scriptScrollLeft.value = 0
    }
  }
)

function onEditorScroll(event) {
  scriptScrollTop.value = event.target.scrollTop || 0
  scriptScrollLeft.value = event.target.scrollLeft || 0
}

function escapeHTML(value) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightCommon(source) {
  return source
    .replace(/\b(true|false|null)\b/g, '<span class="token builtin">$1</span>')
    .replace(/\b\d+(\.\d+)?\b/g, '<span class="token number">$&</span>')
}

function highlightShell(source) {
  let html = escapeHTML(source)
  html = html.replace(/(^|\s)(#[^\n]*)/gm, '$1<span class="token comment">$2</span>')
  html = html.replace(/("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')/g, '<span class="token string">$1</span>')
  html = html.replace(/\$(\w+|\{[^}]+\})/g, '<span class="token variable">$$$1</span>')
  html = html.replace(/\b(function|if|then|else|elif|fi|for|in|do|done|case|esac|while|export|return|exit)\b/g, '<span class="token keyword">$1</span>')
  html = html.replace(/\b(echo|grep|awk|sed|curl|scp|ssh|tar|systemctl|kubectl|python3?|bash|sh)\b/g, '<span class="token command">$1</span>')
  return highlightCommon(html)
}

function highlightPython(source) {
  let html = escapeHTML(source)
  html = html.replace(/(^|\s)(#[^\n]*)/gm, '$1<span class="token comment">$2</span>')
  html = html.replace(/("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')/g, '<span class="token string">$1</span>')
  html = html.replace(/\b(def|class|if|elif|else|for|while|in|return|import|from|as|try|except|finally|with|pass|break|continue|lambda)\b/g, '<span class="token keyword">$1</span>')
  html = html.replace(/\b(print|len|range|open|json|dict|list|str|int|float|bool)\b/g, '<span class="token command">$1</span>')
  return highlightCommon(html)
}

function highlightScript(source, scriptType) {
  if (!source) return ''
  return scriptType === 'python' ? highlightPython(source) : highlightShell(source)
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryOpsScriptList(query)
    tableData.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, {
    pageNum: 1,
    pageSize: 10,
    keyword: '',
    status: ''
  })
  loadData()
}

function openCreate() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row) {
  isEdit.value = true
  const data = await opsScriptInfo(row.id)
  const scriptType = data.scriptType || 'shell'
  Object.assign(form, {
    id: data.id,
    name: data.name || '',
    scriptType,
    interpreter: compatibleInterpreter(scriptType, data.interpreter),
    content: data.content || '',
    variables: (data.variables || []).map((item) => ({ name: item.name || '', defaultValue: item.secret ? '' : (item.defaultValue || ''), description: item.description || '', required: Boolean(item.required), secret: Boolean(item.secret) })),
    timeoutSeconds: data.timeoutSeconds || 300,
    status: data.status || 1,
    description: data.description || '',
    changeSummary: ''
  })
  dialogVisible.value = true
}

async function submit() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入脚本名称')
    return
  }
  if (!form.content.trim()) {
    ElMessage.warning('请输入脚本内容')
    return
  }
  saving.value = true
  try {
    if (isEdit.value) {
      await updateOpsScript(form)
      ElMessage.success('脚本已更新')
    } else {
      await addOpsScript(form)
      ElMessage.success('脚本已创建')
    }
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function handleStatusChange(row) {
  const nextStatus = row.status === 1 ? 2 : 1
  await updateOpsScriptStatus({ id: row.id, status: nextStatus })
  ElMessage.success(nextStatus === 1 ? '脚本已启用' : '脚本已禁用')
  await loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除脚本“${row.name}”吗？`, '提示', { type: 'warning' })
  await deleteOpsScript(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

async function openVersions(row) {
  versionScript.value = row
  versionVisible.value = true
  versionLoading.value = true
  try { versionList.value = await queryOpsScriptVersions(row.id) } finally { versionLoading.value = false }
}

async function handleRollback(row) {
  await ElMessageBox.confirm(`确认将脚本回滚到 v${row.version}？回滚会生成一个新版本。`, '回滚脚本', { type: 'warning' })
  await rollbackOpsScript({ id: versionScript.value.id, version: row.version })
  ElMessage.success('脚本已回滚')
  versionList.value = await queryOpsScriptVersions(versionScript.value.id)
  await loadData()
}

onMounted(loadData)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">脚本库</h2>
        <p class="page-desc">集中维护常用运维脚本，支持启用禁用、构建参数和解释器配置。</p>
      </div>
      <el-button type="primary" @click="openCreate">新增脚本</el-button>
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索脚本名称 / 描述" style="width: 280px" @keyup.enter="loadData" />
        <el-select v-model="query.status" clearable placeholder="状态" style="width: 140px">
          <el-option label="启用" value="1" />
          <el-option label="禁用" value="2" />
        </el-select>
        <el-button type="primary" @click="loadData">搜索</el-button>
        <el-button @click="resetQuery">重置</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="name" label="脚本名称" min-width="180" />
      <el-table-column prop="scriptType" label="类型" width="120" />
      <el-table-column prop="interpreter" label="解释器" width="120" />
      <el-table-column label="构建参数" min-width="240">
        <template #default="{ row }">
          <div v-if="row.variables?.length" class="script-variable-tags">
            <el-tag v-for="variable in row.variables" :key="variable.name" effect="plain" size="small">{{ variableDisplay(variable) }}</el-tag>
          </div>
          <span v-else class="empty-value">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="timeoutSeconds" label="超时(秒)" width="120" />
      <el-table-column label="版本" width="90"><template #default="{ row }">v{{ row.currentVersion || 1 }}</template></el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'warning'" effect="light" :class="{ 'script-status-disabled': row.status !== 1 }">
            {{ row.status === 1 ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
      <el-table-column prop="updateTime" label="更新时间" min-width="180" />
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="primary" @click="openVersions(row)">版本</el-button>
          <el-button link :type="row.status === 1 ? 'warning' : 'success'" :class="{ 'disable-action': row.status === 1 }" @click="handleStatusChange(row)">
            {{ row.status === 1 ? '禁用' : '启用' }}
          </el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="query.pageNum"
        v-model:page-size="query.pageSize"
        :total="total"
        layout="total, sizes, prev, pager, next"
        @current-change="loadData"
        @size-change="loadData"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑脚本' : '新增脚本'" width="980px">
      <el-form label-width="110px">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="脚本名称" required>
              <el-input v-model="form.name" placeholder="例如：重启 nginx" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="脚本类型">
              <el-select v-model="form.scriptType" style="width: 100%" @change="handleScriptTypeChange">
                <el-option label="Shell" value="shell" />
                <el-option label="Python" value="python" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="解释器">
              <el-select v-model="form.interpreter" style="width: 100%">
                <template v-if="form.scriptType === 'python'">
                  <el-option label="python（默认）" value="python" />
                  <el-option label="python3" value="python3" />
                </template>
                <template v-else>
                  <el-option label="bash（默认）" value="bash" />
                  <el-option label="sh" value="sh" />
                </template>
              </el-select>
            </el-form-item>
          </el-col>

          <el-col :span="24">
            <el-form-item label="脚本内容" required>
              <div class="script-editor-shell">
                <div class="script-editor-toolbar">
                  <div class="editor-badges">
                    <span class="editor-badge">{{ form.scriptType === 'python' ? 'Python' : 'Shell' }}</span>
                    <span class="editor-badge subtle">{{ form.interpreter }}</span>
                  </div>
                  <span class="editor-meta">共 {{ scriptLineNumbers.length }} 行</span>
                </div>
                <div class="script-editor-body">
                  <div class="script-gutter">
                    <div class="script-gutter-inner" :style="{ transform: `translateY(-${scriptScrollTop}px)` }">
                      <span v-for="line in scriptLineNumbers" :key="line">{{ line }}</span>
                    </div>
                  </div>
                  <div class="script-editor-stage">
                    <pre class="script-highlight" :style="{ transform: `translate(${-scriptScrollLeft}px, ${-scriptScrollTop}px)` }" v-html="highlightedScript + '\n'"></pre>
                    <textarea
                      ref="scriptEditorRef"
                      v-model="form.content"
                      class="script-editor"
                      spellcheck="false"
                      placeholder="请输入可通过 SSH 执行的脚本内容"
                      @scroll="onEditorScroll"
                    />
                  </div>
                </div>
              </div>
            </el-form-item>
          </el-col>

          <el-col :span="24">
            <div class="variables-panel">
              <div class="variables-heading"><div><h3>构建参数</h3><p>参数会以环境变量注入脚本，可使用 <code>$VARIABLE_NAME</code> 或 <code>${VARIABLE_NAME}</code>。</p></div><el-button type="primary" @click="addVariable">新增参数</el-button></div>
              <div v-if="!form.variables.length" class="variables-empty">暂无自定义参数，可按需新增构建变量。</div>
              <div v-else class="variables-list">
                <div v-for="(variable, index) in form.variables" :key="index" class="variable-row">
                  <div class="variable-name"><span>VARIABLE_</span><el-input v-model="variable.name" placeholder="ENV" @input="normalizeVariableName(variable)" /></div>
                  <el-input v-model="variable.defaultValue" :type="variable.secret ? 'password' : 'text'" :show-password="variable.secret" :disabled="variable.secret" :placeholder="variable.secret ? '保密变量不保存默认值' : '默认值（可选）'" />
                  <el-input v-model="variable.description" placeholder="说明（可选）" />
                  <el-checkbox v-model="variable.required">必填</el-checkbox>
                  <el-checkbox v-model="variable.secret" @change="variable.secret && (variable.defaultValue = '')">保密</el-checkbox>
                  <el-button link type="danger" @click="removeVariable(index)">删除</el-button>
                </div>
              </div>
            </div>
          </el-col>
          <el-col :span="12">
            <el-form-item label="执行超时">
              <div class="timeout-control">
                <el-select v-model="timeoutPreset" class="timeout-preset" aria-label="常用超时时长">
                  <el-option v-for="item in scriptTimeoutPresets" :key="item.value" :label="item.label" :value="item.value" />
                  <el-option label="自定义" value="custom" />
                </el-select>
                <el-input-number v-model="form.timeoutSeconds" :min="30" :max="3600" :step="30" controls-position="right" aria-label="超时秒数" />
              </div>
              <div class="timeout-tip">{{ timeoutDescription }} · 最长 60 分钟</div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="状态">
              <el-radio-group v-model="form.status">
                <el-radio :value="1">启用</el-radio>
                <el-radio :value="2">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="3" />
            </el-form-item>
          </el-col>
          <el-col v-if="isEdit" :span="24">
            <el-form-item label="变更说明"><el-input v-model="form.changeSummary" placeholder="简要说明本次脚本调整" /></el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
    <el-drawer v-model="versionVisible" :title="`${versionScript?.name || ''} - 版本历史`" size="62%">
      <el-table v-loading="versionLoading" :data="versionList" border>
        <el-table-column label="版本" width="90"><template #default="{ row }">v{{ row.version }}</template></el-table-column>
        <el-table-column prop="changeSummary" label="变更说明" min-width="180" />
        <el-table-column prop="operator" label="操作人" width="130"><template #default="{ row }">{{ row.operator || 'system' }}</template></el-table-column>
        <el-table-column prop="createTime" label="创建时间" width="190" />
        <el-table-column prop="content" label="脚本内容" min-width="300" show-overflow-tooltip />
        <el-table-column label="构建参数" min-width="240"><template #default="{ row }"><div v-if="row.variables?.length" class="script-variable-tags"><el-tag v-for="variable in row.variables" :key="variable.name" effect="plain" size="small">{{ variableDisplay(variable) }}</el-tag></div><span v-else class="empty-value">-</span></template></el-table-column>
        <el-table-column label="操作" width="100"><template #default="{ row }"><el-button link type="primary" @click="handleRollback(row)">回滚</el-button></template></el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<style scoped>
.ops-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}

.page-title {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 700;
  color: #14213d;
}

.page-desc {
  margin: 0;
  color: #7282a0;
}
.script-variable-tags { display: flex; flex-wrap: wrap; gap: 5px; }
.script-variable-tags :deep(.el-tag) { max-width: 100%; font-family: 'JetBrains Mono', Consolas, monospace; }
.empty-value { color: #9aa8bd; }

.toolbar {
  display: flex;
  justify-content: space-between;
  gap: 16px;
}

.toolbar-left {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.pager {
  display: flex;
  justify-content: flex-end;
}

.script-status-disabled {
  font-weight: 700;
  letter-spacing: 0.04em;
}

.disable-action {
  color: #e6a23c !important;
  font-weight: 600;
}

.disable-action:hover,
.disable-action:focus-visible {
  color: #d68f24 !important;
}

.timeout-control {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 104px;
  width: 100%;
  overflow: hidden;
  border: 1px solid #d7dfec;
  border-radius: 8px;
  background: #fff;
}

.timeout-control:focus-within {
  border-color: #4f7df3;
  box-shadow: 0 0 0 3px rgba(79, 125, 243, 0.12);
}

.timeout-control :deep(.el-select__wrapper),
.timeout-control :deep(.el-input__wrapper) {
  min-height: 34px;
  box-shadow: none !important;
}

.timeout-preset {
  border-right: 1px solid #e3eaf4;
}

.timeout-tip {
  margin-top: 6px;
  color: #8090a8;
  font-size: 12px;
  line-height: 1.2;
}

.variables-panel { margin: 2px 0 18px; padding: 18px; border: 1px solid #d7e4fb; border-radius: 10px; background: linear-gradient(135deg, #fbfdff, #f5f8ff); }
.variables-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; margin-bottom: 14px; }.variables-heading h3 { margin: 0 0 4px; color: #172b4d; font-size: 16px; }.variables-heading p { margin: 0; color: #7184a2; font-size: 13px; }.variables-heading code { color: #456ee8; font-family: 'JetBrains Mono', Consolas, monospace; }
.variables-empty { padding: 16px 18px; border: 1px dashed #bdd2fa; border-radius: 7px; color: #7184a2; font-size: 13px; }
.variables-list { display: flex; flex-direction: column; gap: 9px; }.variable-row { display: grid; grid-template-columns: 1.25fr 1fr 1.2fr auto auto auto; align-items: center; gap: 9px; padding: 10px; border: 1px solid #e0e8f5; border-radius: 8px; background: #fff; }.variable-name { display: flex; align-items: center; overflow: hidden; border: 1px solid #d7dfec; border-radius: 6px; }.variable-name span { padding: 0 8px; color: #5473a5; font: 12px 'JetBrains Mono', Consolas, monospace; white-space: nowrap; }.variable-name :deep(.el-input__wrapper) { box-shadow: none; }.variable-row :deep(.el-checkbox) { margin-right: 0; white-space: nowrap; }
@media (max-width: 900px) { .variable-row { grid-template-columns: 1fr 1fr; }.variables-heading { align-items: flex-start; flex-direction: column; } }

.script-editor-shell {
  width: 100%;
  border: 1px solid #d7dfec;
  border-radius: 10px;
  overflow: hidden;
  background: #111827;
}

.script-editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: #0f172a;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.editor-badges {
  display: flex;
  gap: 8px;
}

.editor-badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(96, 165, 250, 0.18);
  color: #bfdbfe;
  font-size: 12px;
  font-weight: 600;
}

.editor-badge.subtle {
  background: rgba(148, 163, 184, 0.14);
  color: #cbd5e1;
}

.editor-meta {
  color: #94a3b8;
  font-size: 12px;
}

.script-editor-body {
  display: flex;
  min-height: 420px;
}

.script-gutter {
  width: 58px;
  flex-shrink: 0;
  background: #0f172a;
  border-right: 1px solid rgba(148, 163, 184, 0.16);
  overflow: hidden;
}

.script-gutter-inner {
  padding: 14px 0;
}

.script-gutter span {
  display: block;
  height: 24px;
  line-height: 24px;
  padding-right: 12px;
  text-align: right;
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-size: 13px;
  color: #64748b;
}

.script-editor-stage {
  position: relative;
  flex: 1;
  min-height: 420px;
  overflow: hidden;
}

.script-highlight,
.script-editor {
  margin: 0;
  padding: 14px 16px;
  min-height: 420px;
  width: 100%;
  box-sizing: border-box;
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-size: 14px;
  line-height: 24px;
  white-space: pre;
  tab-size: 2;
}

.script-highlight {
  position: absolute;
  inset: 0;
  overflow: hidden;
  color: #e5e7eb;
  pointer-events: none;
}

.script-editor {
  position: relative;
  z-index: 1;
  border: 0;
  resize: none;
  outline: none;
  /* Keep the raw input readable even if syntax highlighting cannot render. */
  background: #111827;
  color: #e5e7eb;
  caret-color: #f8fafc;
  overflow: auto;
}

.script-editor::placeholder {
  color: #64748b;
}

:deep(.token.comment) {
  color: #6b7280;
}

:deep(.token.keyword) {
  color: #f472b6;
  font-weight: 600;
}

:deep(.token.string) {
  color: #fde047;
}

:deep(.token.variable) {
  color: #a3e635;
}

:deep(.token.command) {
  color: #60a5fa;
}

:deep(.token.number) {
  color: #c084fc;
}

:deep(.token.builtin) {
  color: #34d399;
}
</style>
