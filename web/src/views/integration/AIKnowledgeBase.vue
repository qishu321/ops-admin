<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Delete, Document, DocumentCopy, Edit, EditPen, Plus, Upload, View } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteAIKnowledgeDocument, queryAIKnowledgeDocuments, saveAIKnowledgeDocument, uploadAIKnowledgeDocument } from '../../api/integration'
import './ai.css'

const loading = ref(false)
const saving = ref(false)
const uploading = ref(false)
const dialogVisible = ref(false)
const editorMode = ref('edit')
const keyword = ref('')
const documents = ref([])
const form = reactive({ id: undefined, name: '', fileName: '', content: '', status: 1, sourceType: 'manual' })
const activeCount = computed(() => documents.value.filter((item) => item.status === 1).length)

function reset() { Object.assign(form, { id: undefined, name: '', fileName: '', content: '', status: 1, sourceType: 'manual' }) }
async function load() { loading.value = true; try { documents.value = (await queryAIKnowledgeDocuments({ keyword: keyword.value })) || [] } finally { loading.value = false } }
function createDocument() { reset(); editorMode.value = 'edit'; dialogVisible.value = true }
function editDocument(row) { Object.assign(form, { ...row, sourceType: row.sourceType || 'manual' }); editorMode.value = 'edit'; dialogVisible.value = true }
async function save() {
  if (!form.name.trim() || !form.content.trim()) return ElMessage.warning('请填写文档名称和 Markdown 内容')
  saving.value = true
  try { await saveAIKnowledgeDocument(form); dialogVisible.value = false; await load(); ElMessage.success('知识库文档已保存') } finally { saving.value = false }
}
async function remove(row) {
  await ElMessageBox.confirm(`确定删除知识库文档“${row.name}”吗？`, '删除文档', { type: 'warning' })
  await deleteAIKnowledgeDocument(row.id); await load(); ElMessage.success('文档已删除')
}
async function upload(file) {
  if (!file.name.toLowerCase().endsWith('.md')) { ElMessage.error('仅支持上传 .md 文件'); return false }
  if (file.size > 2 * 1024 * 1024) { ElMessage.error('Markdown 文件不能超过 2MB'); return false }
  uploading.value = true
  try { const data = new FormData(); data.append('file', file); await uploadAIKnowledgeDocument(data); await load(); ElMessage.success('Markdown 文档已导入') } finally { uploading.value = false }
  return false
}
function preview(content) { return (content || '').replace(/\s+/g, ' ').slice(0, 140) || '暂无内容' }
function formatDateTime(value) {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const match = raw.match(/^(\d{4}-\d{2}-\d{2})[T\s](\d{2}:\d{2}:\d{2})/)
  if (match) return `${match[1]} ${match[2]}`
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}
const contentLines = computed(() => (form.content || '').split('\n'))
onMounted(load)
</script>

<template>
  <div class="ai-page knowledge-page">
    <section class="ai-hero">
      <div><div class="ai-kicker">LOCAL MARKDOWN KNOWLEDGE</div><h1>知识库管理</h1><p>上传或在线维护本地 Markdown 文档；启用后的文档可由“知识库检索”工具提供给 AI 助手使用。</p></div>
      <div class="knowledge-actions"><el-upload :show-file-list="false" :before-upload="upload" accept=".md"><el-button :loading="uploading" :icon="Upload">上传 .md</el-button></el-upload><el-button type="primary" :icon="Plus" @click="createDocument">新建 .md</el-button></div>
    </section>
    <section class="ai-panel">
      <div class="ai-panel-head knowledge-head"><div><h2>本地知识文档</h2><span class="ai-muted">已启用 {{ activeCount }} / {{ documents.length }} 篇，仅启用内容会被 AI 检索。</span></div><el-input v-model="keyword" clearable placeholder="搜索文档名称" style="width: 240px" @keyup.enter="load" @clear="load"><template #append><el-button @click="load">搜索</el-button></template></el-input></div>
      <el-table v-loading="loading" :data="documents" empty-text="尚未添加 Markdown 文档" style="width: 100%">
        <el-table-column label="文档" min-width="260"><template #default="{ row }"><div class="doc-name"><el-icon><Document /></el-icon><div><strong>{{ row.name }}</strong><small>{{ row.fileName }}</small></div></div></template></el-table-column>
        <el-table-column label="内容摘要" min-width="380" show-overflow-tooltip><template #default="{ row }">{{ preview(row.content) }}</template></el-table-column>
        <el-table-column label="来源" width="100"><template #default="{ row }"><el-tag size="small" effect="plain">{{ row.sourceType === 'upload' ? '上传' : '新建' }}</el-tag></template></el-table-column>
        <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
        <el-table-column label="更新时间" min-width="170"><template #default="{ row }">{{ formatDateTime(row.updateTime) }}</template></el-table-column>
        <el-table-column label="操作" width="160"><template #default="{ row }"><div class="doc-actions"><el-button link type="primary" :icon="Edit" @click="editDocument(row)">编辑</el-button><el-button link type="danger" :icon="Delete" @click="remove(row)">删除</el-button></div></template></el-table-column>
      </el-table>
    </section>
    <el-dialog v-model="dialogVisible" width="min(1120px, 92vw)" class="knowledge-editor-dialog" destroy-on-close>
      <template #header>
        <div class="editor-dialog-head"><span class="editor-dialog-icon"><el-icon><DocumentCopy /></el-icon></span><div><div class="ai-kicker">MARKDOWN KNOWLEDGE DOCUMENT</div><h2>{{ form.id ? '编辑知识库文档' : '新建知识库文档' }}</h2><p>维护团队可检索的 Markdown 内容，保存后可由 AI 助手按需引用。</p></div><el-tag class="editor-source" effect="plain">{{ form.sourceType === 'upload' ? '已上传文档' : '在线编辑' }}</el-tag></div>
      </template>
      <el-form class="knowledge-editor-form" label-position="top">
        <section class="editor-meta-card"><div class="editor-meta-title"><el-icon><EditPen /></el-icon><span>文档信息</span></div><div class="document-form-grid"><el-form-item label="文档名称" required><el-input v-model="form.name" placeholder="例如：生产发布规范"/></el-form-item><el-form-item label="文件名"><el-input v-model="form.fileName" placeholder="默认为“文档名称.md”"><template #append>.md</template></el-input></el-form-item></div></section>
        <section class="editor-workspace"><div class="editor-workspace-head"><div><strong>Markdown 内容</strong><span>{{ form.content.length.toLocaleString() }} 字符</span></div><el-radio-group v-model="editorMode" size="small"><el-radio-button value="edit"><el-icon><EditPen /></el-icon> 编辑</el-radio-button><el-radio-button value="preview"><el-icon><View /></el-icon> 预览</el-radio-button></el-radio-group></div><el-input v-if="editorMode === 'edit'" v-model="form.content" class="markdown-input" type="textarea" :rows="20" resize="none" placeholder="# 标题\n\n在此编写知识库内容…"/><div v-else class="markdown-preview"><template v-for="(line, index) in contentLines" :key="index"><h1 v-if="line.startsWith('# ')" >{{ line.slice(2) }}</h1><h2 v-else-if="line.startsWith('## ')" >{{ line.slice(3) }}</h2><blockquote v-else-if="line.startsWith('> ')" >{{ line.slice(2) }}</blockquote><pre v-else-if="line.startsWith('```')" >{{ line }}</pre><p v-else>{{ line || ' ' }}</p></template></div></section>
        <section class="retrieval-setting"><div><strong>AI 检索</strong><span>启用后，知识库检索工具可以将此文档的相关片段提供给智能对话。</span></div><el-switch v-model="form.status" :active-value="1" :inactive-value="2" active-text="已启用" inactive-text="已停用"/></section>
      </el-form>
      <template #footer><div class="editor-footer"><span>仅保存到平台本地数据库</span><div><el-button @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存文档</el-button></div></div></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.knowledge-actions { display: flex; gap: 10px; }.knowledge-head { align-items: center; }.doc-name { display: flex; align-items: center; gap: 10px; }.doc-name > .el-icon { display: grid; place-items: center; width: 34px; height: 34px; color: #356fe5; background: #eaf1ff; border-radius: 6px; }.doc-name strong, .doc-name small { display: block; }.doc-name small { margin-top: 3px; color: #8090a8; }.doc-actions { display: flex; align-items: center; gap: 14px; white-space: nowrap; }.doc-actions :deep(.el-button + .el-button) { margin-left: 0; }.document-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }.document-form-grid :deep(.el-form-item) { margin-bottom: 0; }.editor-dialog-head { display: flex; align-items: center; gap: 12px; min-height: 42px; }.editor-dialog-head h2 { margin: 2px 0 3px; color: #122543; font-size: 21px; }.editor-dialog-head p { margin: 0; color: #8190a7; font-size: 13px; }.editor-dialog-icon { display: grid; place-items: center; width: 42px; height: 42px; color: #fff; background: linear-gradient(145deg, #3c78ed, #7058f5); border-radius: 9px; font-size: 21px; }.editor-source { margin-left: auto; }.knowledge-editor-form { display: grid; gap: 14px; }.editor-meta-card, .editor-workspace, .retrieval-setting { border: 1px solid #e1e9f5; border-radius: 9px; }.editor-meta-card { padding: 15px 17px; background: #f7f9fd; }.editor-meta-title { display: flex; align-items: center; gap: 7px; margin-bottom: 12px; color: #304968; font-size: 14px; font-weight: 700; }.editor-meta-title .el-icon { color: #4779e9; }.editor-workspace { overflow: hidden; background: #fff; }.editor-workspace-head { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid #e7edf6; background: #fbfcff; }.editor-workspace-head > div { display: flex; align-items: center; gap: 10px; }.editor-workspace-head strong { color: #243b5b; }.editor-workspace-head span { color: #94a0b3; font-size: 12px; }.markdown-input :deep(textarea) { min-height: 405px !important; padding: 18px 20px; color: #d8e6ff; font: 13px/1.75 Consolas, 'Microsoft YaHei Mono', monospace; background: #101a2d; border: 0; border-radius: 0; }.markdown-preview { height: 405px; padding: 18px 28px; overflow: auto; color: #3d506e; line-height: 1.75; background: #fff; }.markdown-preview h1 { margin: 0 0 18px; padding-bottom: 10px; color: #142b4b; font-size: 23px; border-bottom: 1px solid #e5ecf5; }.markdown-preview h2 { margin: 21px 0 8px; color: #254f8d; font-size: 17px; }.markdown-preview p { min-height: 12px; margin: 5px 0; white-space: pre-wrap; }.markdown-preview blockquote { margin: 12px 0; padding: 8px 13px; color: #5e7190; background: #f3f7fd; border-left: 3px solid #6084e8; }.markdown-preview pre { padding: 10px; color: #d7e6ff; background: #101a2d; border-radius: 5px; }.retrieval-setting { display: flex; align-items: center; justify-content: space-between; padding: 14px 17px; background: #fbfdfb; }.retrieval-setting div { display: grid; gap: 4px; }.retrieval-setting strong { color: #25405f; }.retrieval-setting span { color: #8391a7; font-size: 12px; }.editor-footer { display: flex; align-items: center; justify-content: space-between; width: 100%; }.editor-footer > span { color: #8896aa; font-size: 12px; } @media(max-width: 760px) { .knowledge-actions, .knowledge-head { align-items: stretch; flex-direction: column; }.document-form-grid { grid-template-columns: 1fr; }.editor-source { display: none; }.editor-dialog-head p { display: none; }.editor-workspace-head { align-items: flex-start; gap: 10px; flex-direction: column; } }
</style>
