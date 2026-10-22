<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Refresh, Plus, Folder, Document, Search } from '@element-plus/icons-vue'
import { operateMiddleware } from '../../api/middleware'
const props = defineProps({ source: Object, can: Function })
defineEmits(['back'])
const prefix = ref(''), loadedPrefix = ref(''), keys = ref([]), more = ref(false), loading = ref(false), error = ref(''), selected = ref(null), reading = ref(false), mode = ref('formatted'), tab = ref('keys'), members = ref([])
const dialog = ref(false), editing = ref(false), keyInput = ref(''), valueInput = ref(''), saving = ref(false)
let generation = 0, readGeneration = 0, alive = true
onUnmounted(() => { alive = false; generation++; readGeneration++ })
const tree = computed(() => {
 const root = { children: [], map: new Map() }
 for (const item of keys.value) {
  let parent = root, path = ''
  const parts = item.key.split('/')
  parts.forEach((part, index) => {
   path += (index ? '/' : '') + part
   if (!part && index === 0) return
   let node = parent.map.get(path)
   if (!node) { node = { id: path, label: part || '/', children: [], map: new Map(), key: null }; parent.map.set(path, node); parent.children.push(node) }
   if (index === parts.length - 1) node.key = item.key
   parent = node
  })
 }
 return root.children
})
const content = computed(() => {
 const value = selected.value?.value ?? ''
 if (mode.value === 'formatted') { try { return JSON.stringify(JSON.parse(value), null, 2) } catch {} }
 return value
})
async function load(append = false) {
 if (!props.can('query') || (append && loading.value)) return
 const g = ++generation; loading.value = true; error.value = ''
 if (!append) { loadedPrefix.value = prefix.value; keys.value = []; more.value = false; selected.value = null; readGeneration++; reading.value = false }
 try {
  const data = await operateMiddleware(props.source.id, 'keys', { key: loadedPrefix.value, after: append ? keys.value.at(-1)?.key || '' : '', keysOnly: 'true' })
  if (g !== generation) return
  keys.value = [...(append ? keys.value : []), ...(data?.kvs || [])]; more.value = !!data?.more
 } catch (e) { if (g === generation) error.value = e.message || 'Key 加载失败' }
 finally { if (g === generation) loading.value = false }
}
async function select(node) {
 if (node.key === null) return
 const g = ++readGeneration; reading.value = true; selected.value = null; error.value = ''
 try { const data = await operateMiddleware(props.source.id, 'key.get', { key: node.key }); if (g === readGeneration) { selected.value = data?.kvs?.[0] || null; if (!selected.value) error.value = '该 Key 已不存在，请刷新目录' } }
 catch (e) { if (g === readGeneration) error.value = e.message || '读取失败' }
 finally { if (g === readGeneration) reading.value = false }
}
async function changeTab() {
 if (tab.value === 'keys') { if (!keys.value.length) await load(); return }
 const g = ++generation; loading.value = true; error.value = ''
 try { const data = await operateMiddleware(props.source.id, 'members', {}); if (g === generation) members.value = data?.members || [] }
 catch (e) { if (g === generation) error.value = e.message || '成员加载失败' }
 finally { if (g === generation) loading.value = false }
}
function openEditor(edit) { editing.value = edit; keyInput.value = edit ? selected.value.key : loadedPrefix.value; valueInput.value = edit ? selected.value.value : ''; dialog.value = true }
async function write() {
 if (saving.value || !keyInput.value.trim() || !props.can('write')) return
 const key = keyInput.value, value = valueInput.value; saving.value = true
 try {
  await ElMessageBox.confirm(`确认${editing.value ? '更新' : '写入'} Key：${key}？已有内容将被覆盖。`, '保存键值', { confirmButtonText: '保存', cancelButtonText: '取消', type: 'warning' })
  if (!alive || !props.can('write')) return
  await operateMiddleware(props.source.id, 'key.put', { key, value }, 'write', true)
  if (!alive) return
  dialog.value = false; ElMessage.success('键值已保存'); await load(); await select({ key })
 } catch (e) { if (e !== 'cancel' && e !== 'close' && alive) error.value = e.message || '保存失败' }
 finally { saving.value = false }
}
async function remove() {
 const key = selected.value?.key
 if (!key || saving.value || !props.can('delete')) return
 saving.value = true
 try {
  await ElMessageBox.confirm(`仅删除 Key：${key}，不会删除同名前缀下的其他 Key。`, '删除键值', { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' })
  if (!alive || !props.can('delete')) return
  await operateMiddleware(props.source.id, 'key.delete', { key }, 'delete', true)
  if (alive) { ElMessage.success('Key 已删除'); await load() }
 } catch (e) { if (e !== 'cancel' && e !== 'close' && alive) error.value = e.message || '删除失败' }
 finally { saving.value = false }
}
onMounted(() => load())
</script>
<template>
 <section class="etcd-explorer">
  <header class="explorer-heading"><el-button :icon="ArrowLeft" @click="$emit('back')">数据源</el-button><div><h2>{{ source.name }} <span>etcd</span></h2><p>键值管理</p></div><div class="heading-actions"><el-button v-if="can('write')" type="primary" :icon="Plus" @click="openEditor(false)">新增 Key</el-button><el-button :icon="Refresh" :loading="loading" :disabled="!can('query')" @click="tab === 'keys' ? load() : changeTab()">刷新</el-button></div></header>
  <el-alert v-if="!can('query')" title="当前没有查询权限，请联系管理员授权。" type="info" :closable="false"/>
  <template v-else>
   <el-tabs v-model="tab" @tab-change="changeTab"><el-tab-pane label="键值浏览" name="keys"/><el-tab-pane label="集群成员" name="members"/></el-tabs>
   <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>
   <div v-if="tab === 'keys'" class="key-workspace">
    <aside class="key-directory"><form class="prefix-search" @submit.prevent="load()"><el-input v-model="prefix" :prefix-icon="Search" placeholder="搜索 Key 前缀，例如 /registry/" aria-label="Key 前缀" clearable/><el-button native-type="submit" :loading="loading">查询</el-button></form><div class="directory-caption"><strong>KEY 路径</strong><span>已加载 {{ keys.length }} 个</span></div><div class="tree-scroll" v-loading="loading && !keys.length"><el-tree :data="tree" node-key="id" highlight-current :expand-on-click-node="true" :default-expanded-keys="tree.map(n => n.id)" @node-click="select"><template #default="{ data }"><span class="tree-label" :title="data.id"><el-icon :class="{ folder: data.children.length }"><Folder v-if="data.children.length"/><Document v-else/></el-icon><span>{{ data.label }}</span><small v-if="data.key !== null && data.children.length">Key</small></span></template></el-tree><el-empty v-if="!keys.length && !loading" description="没有匹配的 Key" :image-size="64"/></div><div class="directory-footer"><el-button v-if="more" :loading="loading" @click="load(true)">加载更多 Key</el-button><span>{{ more ? '还有更多结果，可继续加载或缩小前缀' : '当前前缀已全部加载' }}</span></div></aside>
    <main class="key-detail" v-loading="reading"><template v-if="selected"><div class="detail-heading"><div><span class="field-caption">KEY</span><h3>{{ selected.key }}</h3></div><div class="key-actions"><el-button v-if="can('write')" :disabled="saving" @click="openEditor(true)">编辑</el-button><el-button v-if="can('delete')" type="danger" plain :disabled="saving" @click="remove">删除</el-button></div></div><dl class="key-metadata"><div><dt>版本</dt><dd>{{ selected.version || '—' }}</dd></div><div><dt>创建修订</dt><dd>{{ selected.create_revision || '—' }}</dd></div><div><dt>修改修订</dt><dd>{{ selected.mod_revision || '—' }}</dd></div><div><dt>租约 ID</dt><dd>{{ selected.lease || '无租约' }}</dd></div></dl><div class="value-heading"><strong>Value</strong><el-radio-group v-model="mode" size="small"><el-radio-button value="formatted">格式化</el-radio-button><el-radio-button value="raw">原始文本</el-radio-button></el-radio-group></div><pre class="value-editor" tabindex="0" aria-label="Key 完整内容">{{ content }}</pre></template><el-empty v-else description="从左侧选择 Key，查看完整内容与版本信息" :image-size="96"/></main>
   </div>
   <el-table v-else :data="members" v-loading="loading" empty-text="暂无集群成员"><el-table-column prop="name" label="成员名称" min-width="160"/><el-table-column prop="ID" label="成员 ID" min-width="190"/><el-table-column label="客户端地址" min-width="250"><template #default="{ row }">{{ row.clientURLs?.join(', ') || '—' }}</template></el-table-column><el-table-column label="Peer 地址" min-width="250"><template #default="{ row }">{{ row.peerURLs?.join(', ') || '—' }}</template></el-table-column><el-table-column label="角色" width="110"><template #default="{ row }">{{ row.isLearner ? 'Learner' : '投票成员' }}</template></el-table-column></el-table>
  </template>
  <el-dialog v-model="dialog" :title="editing ? '编辑键值' : '新增键值'" width="min(860px,94vw)" :close-on-click-modal="false"><el-form label-position="top"><el-form-item label="Key" required><el-input v-model="keyInput" :disabled="editing || saving" placeholder="例如 /app/config"/></el-form-item><el-form-item label="Value"><el-input v-model="valueInput" type="textarea" :rows="16" :disabled="saving" class="value-input" placeholder="输入完整内容，支持空值"/></el-form-item></el-form><template #footer><el-button :disabled="saving" @click="dialog = false">取消</el-button><el-button type="primary" :loading="saving" :disabled="!keyInput.trim()" @click="write">保存</el-button></template></el-dialog>
 </section>
</template>
<style scoped>
.etcd-explorer{background:#fff;border:1px solid #e3e9f2;border-radius:10px;padding:20px 24px;min-width:0}.explorer-heading{display:flex;align-items:center;gap:16px;margin-bottom:12px}.explorer-heading h2{margin:0;font-size:18px;color:#24354e}.explorer-heading h2 span{font-size:12px;color:#74849b;margin-left:10px;font-weight:400}.explorer-heading p{margin:5px 0 0;color:#8291a6;font-size:12px}.heading-actions{margin-left:auto;display:flex;gap:8px}.key-workspace{display:grid;grid-template-columns:minmax(270px,30%) minmax(0,1fr);border:1px solid #e3e9f2;border-radius:7px;overflow:hidden;min-height:570px;height:calc(100vh - 310px)}.key-directory{display:flex;flex-direction:column;min-height:0;border-right:1px solid #e3e9f2;background:#fafbfd}.prefix-search{display:flex;gap:8px;padding:16px 12px}.directory-caption{display:flex;justify-content:space-between;padding:0 16px 12px;font-size:12px;color:#8391a4}.directory-caption strong{font-weight:600;letter-spacing:.05em}.tree-scroll{flex:1;overflow:auto;min-height:0;padding:4px 8px}.tree-scroll :deep(.el-tree){background:transparent;min-width:max-content}.tree-scroll :deep(.el-tree-node__content){height:34px;border-radius:4px}.tree-label{display:flex;align-items:center;gap:8px;padding-right:12px;font:12px Consolas,monospace;color:#35465e}.tree-label .el-icon{color:#8c9bb1}.tree-label .folder{color:#5185d7}.tree-label small{color:#8c9bb1}.directory-footer{padding:12px 16px;border-top:1px solid #e3e9f2;display:grid;gap:8px;font-size:11px;color:#8291a6}.key-detail{padding:22px;display:flex;flex-direction:column;min-width:0;min-height:0;overflow:auto}.detail-heading{display:flex;align-items:flex-start;gap:16px}.detail-heading>div:first-child{flex:1;min-width:0}.field-caption{font-size:11px;color:#8796aa;font-weight:600}.detail-heading h3{font:14px/1.65 Consolas,monospace;overflow-wrap:anywhere;color:#28538c;margin:8px 0 18px}.key-actions{display:flex;white-space:nowrap}.key-metadata{display:flex;flex-wrap:wrap;gap:28px;margin:0 0 22px;padding:14px 0;border-top:1px solid #edf0f5;border-bottom:1px solid #edf0f5}.key-metadata dt{font-size:12px;color:#8391a4}.key-metadata dd{margin:6px 0 0;font:13px Consolas,monospace;color:#35465e}.value-heading{display:flex;justify-content:space-between;align-items:center;font-size:13px;margin-bottom:12px}.value-editor{flex:1;min-height:260px;overflow:auto;background:#f7f9fc;border:1px solid #e5ebf4;border-radius:6px;margin:0;padding:18px;font:13px/1.75 Consolas,monospace;white-space:pre-wrap;overflow-wrap:anywhere;color:#35465e;tab-size:2}.value-input :deep(textarea){font:13px/1.7 Consolas,monospace}.key-detail>.el-empty{margin:auto}.etcd-explorer>.el-alert{margin-bottom:12px}@media(max-width:900px){.key-workspace{grid-template-columns:1fr;height:auto}.key-directory{max-height:350px;border-right:0;border-bottom:1px solid #e3e9f2}.key-detail{min-height:450px}.explorer-heading{flex-wrap:wrap}.heading-actions{margin-left:0}.etcd-explorer{padding:16px}}
</style>
