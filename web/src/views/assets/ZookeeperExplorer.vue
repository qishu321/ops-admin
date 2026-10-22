<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Document, Folder, FolderOpened, Refresh, Search } from '@element-plus/icons-vue'
import { getUser, getPermissions } from '../../utils/auth'
import { listMiddleware, operateMiddleware } from '../../api/middleware'
import { zkFormatBytes, zkHex, zkJoinPath, zkNameValid, zkParentPath, zkPretty, zkSegments, zkStatRows, zkTime, zkTreeNodes } from './zk-node'

const props = defineProps({ sourceId: Number })
const router = useRouter(), route = useRoute()
const treeRef = ref(null), treeKey = ref(0)
const source = ref(null), current = ref(null), currentPath = ref('/')
const keyword = ref(''), error = ref(''), loading = ref(false), refreshing = ref(false), saving = ref(false)
const formatted = ref(false), truncatedChildren = ref(false)
const editDialog = ref(false), createDialog = ref(false)
const editForm = reactive({ value: '' }), createForm = reactive({ name: '', value: '' })
const revision = ref(0)
let generation = 0, treeGeneration = 0, revealGeneration = 0

const can = action => { revision.value; return getUser().accountType === 'administrator' || getPermissions().includes(`assets:middleware:${action}`) }
const canRead = computed(() => can('query'))
const valid = computed(() => Number.isSafeInteger(props.sourceId) && props.sourceId > 0)
const treeProps = { label: 'name', isLeaf: 'isLeaf' }
const segments = computed(() => zkSegments(currentPath.value))
const stat = computed(() => current.value?.stat || {})
const statRows = computed(() => zkStatRows(stat.value))
const valueText = computed(() => String(current.value?.value ?? ''))
const shownValue = computed(() => formatted.value ? zkPretty(valueText.value).value : valueText.value)
const aclRows = computed(() => (current.value?.acl || []).map(a => ({ id: `${a.Scheme}:${a.ID}`, perms: aclPerms(a.Perms) })))
const isEphemeral = computed(() => Number(stat.value.EphemeralOwner || 0) !== 0)

function aclPerms(perms) {
  const mask = Number(perms) || 0
  return [['read', 1, 'r'], ['write', 2, 'w'], ['create', 4, 'c'], ['delete', 8, 'd'], ['admin', 16, 'a']].filter(([, bit]) => mask & bit).map(([, , code]) => code).join('/') || '—'
}

onMounted(() => {
  window.addEventListener('ops-admin-permissions-changed', permissionsChanged)
  if (!valid.value) { error.value = '数据源参数不正确，请从中间件列表进入。'; return }
  loadSource()
  const initial = String(route.query.path || '/')
  if (canRead.value) loadDetail(initial.startsWith('/') ? initial : '/')
})
onUnmounted(() => { generation++; treeGeneration++; revealGeneration++; window.removeEventListener('ops-admin-permissions-changed', permissionsChanged) })

function permissionsChanged() {
  revision.value++;generation++;treeGeneration++;loading.value=false;current.value=null;editDialog.value=false;createDialog.value=false
  treeKey.value++
  revealGeneration++
  error.value = ''
  if (canRead.value) { loadDetail(currentPath.value); reloadRoot() } else { current.value = null }
}

async function loadSource() {
  try {
    const rows = await listMiddleware('zookeeper')
    source.value = (rows || []).find(x => x.id === props.sourceId) || null
  } catch { source.value = null }
}

function api(operation, params, action = 'query') {
  return operateMiddleware(props.sourceId, operation, params, action, action !== 'query')
}

async function loadTreeNode(node, resolve) {
  if (!canRead.value) { resolve([]); return }
  const path = node.level === 0 ? '/' : node.data.path
  const g = treeGeneration
  try {
    const data = await api('children', { path, detail: '1' })
    if (g !== treeGeneration) return resolve([])
    if (path === '/') truncatedChildren.value = Boolean(data?.truncated)
    resolve(zkTreeNodes(data))
  } catch (e) {
    resolve([])
    if(g!==treeGeneration)return
    if (node.level === 0) error.value = e.message || '读取根节点失败'
    else ElMessage.warning(e.message || `读取 ${path} 的子节点失败`)
  }
}

async function loadDetail(path) {
  if (!canRead.value) { error.value = '当前没有该数据源的查询权限，请联系管理员授权。'; return }
  const g = ++generation
  loading.value = true
  error.value = ''
  try {
    const data = await api('node.get', { path })
    if (g !== generation) return
    current.value = data
    currentPath.value = data?.path || path
    formatted.value = false
    void revealPath(currentPath.value)
  } catch (e) {
    if (g !== generation) return
    current.value = null
    currentPath.value = path
    error.value = e.message || '读取节点失败，请检查节点是否存在。'
  } finally {
    if (g === generation) loading.value = false
  }
}

// 单击节点即展开目录（只展开不收起，收起请点左侧箭头），同时加载右侧详情。
function onNodeClick(data, node) {
  if (node && !node.isLeaf && !node.expanded) node.expand()
  currentPath.value = data.path
  loadDetail(data.path)
}

const wait = ms => new Promise(resolve => setTimeout(resolve, ms))
// 懒加载节点要等子节点回填后才能继续往下找，这里简单轮询等待。
async function waitChildren(node, timeout = 4000) {
  const start = Date.now()
  while (node.loading && Date.now() - start < timeout) await wait(50)
  await nextTick()
}
async function nodeWhenReady(path, token, timeout = 2500) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    if (token !== revealGeneration) return null
    const node = treeRef.value?.getNode(path)
    if (node) return node
    await wait(60)
  }
  return null
}
// 深链、面包屑跳转后把目标节点的祖先逐级展开，并把选中项滚到可视区。
async function revealPath(path) {
  const token = ++revealGeneration
  const segments = zkSegments(path)
  for (let i = 1; i < segments.length; i++) {
    const node = await nodeWhenReady(segments[i].path, token)
    if (!node || token !== revealGeneration) return
    if (i < segments.length - 1 && !node.expanded) {
      node.expand()
      await waitChildren(node)
    }
  }
  if (token !== revealGeneration) return
  treeRef.value?.setCurrentKey(path)
  await nextTick()
  treeRef.value?.$el?.querySelector('.el-tree-node.is-current')?.scrollIntoView({ block: 'nearest' })
}

function filterNode(value, data) {
  return !value || data.path.toLowerCase().includes(String(value).toLowerCase())
}
watch(keyword, value => treeRef.value?.filter(value))

async function jumpTo() {
  const raw = keyword.value.trim()
  if (!raw.startsWith('/')) return
  const path = raw.length > 1 ? raw.replace(/\/+$/, '') : '/'
  keyword.value = ''
  await loadDetail(path)
}

async function reloadRoot() {
  treeKey.value++
  await nextTick()
  if (currentPath.value !== '/') void revealPath(currentPath.value)
}

async function reload() {
  refreshing.value = true
  try {
    await loadDetail(currentPath.value)
    await syncTreeChildren(currentPath.value)
    if (currentPath.value === '/') await reloadRoot()
  } finally {
    refreshing.value = false
  }
}

async function syncTreeChildren(path) {
  const node = treeRef.value?.getNode(path)
  if (!node) return
  try {
    const data = await api('children', { path, detail: '1' })
    treeRef.value?.updateKeyChildren(path, zkTreeNodes(data))
  } catch { /* 目录同步失败不影响详情展示 */ }
}

function toggleFormat() {
  if (formatted.value) { formatted.value = false; return }
  if (!zkPretty(valueText.value).ok) { ElMessage.warning('当前内容不是合法 JSON，无法格式化'); return }
  formatted.value = true
}

function openEdit() {
  if (!current.value) return
  editForm.value = valueText.value
  editDialog.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    await api('node.put', { path: currentPath.value, version: String(stat.value.Version ?? 0), value: editForm.value }, 'write')
    editDialog.value = false
    ElMessage.success('节点已保存')
    await loadDetail(currentPath.value)
  } catch (e) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function openCreate() {
  createForm.name = ''
  createForm.value = ''
  createDialog.value = true
}

async function saveCreate() {
  const name = createForm.name.trim()
  if (!zkNameValid(name)) { ElMessage.warning('节点名称不能为空，且不能包含 /'); return }
  const path = zkJoinPath(currentPath.value, name)
  saving.value = true
  try {
    await api('node.create', { path, value: createForm.value }, 'write')
    createDialog.value = false
    ElMessage.success('节点已创建')
    await syncTreeChildren(currentPath.value)
    await loadDetail(path)
  } catch (e) {
    ElMessage.error(e.message || '创建失败')
  } finally {
    saving.value = false
  }
}

async function removeNode() {
  if (!current.value) return
  const children = Number(stat.value.NumChildren || 0)
  const detail = children > 0
    ? `节点 ${currentPath.value} 下还有 ${children} 个子节点，ZooKeeper 要求先清空子节点。`
    : `将删除节点 ${currentPath.value}，该操作不可恢复。`
  try {
    await ElMessageBox.confirm(detail, '删除节点', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  } catch { return }
  const parent = zkParentPath(currentPath.value)
  try {
    await api('node.delete', { path: currentPath.value, version: String(stat.value.Version ?? 0) }, 'delete')
    ElMessage.success('节点已删除')
    if (currentPath.value === '/') { await reloadRoot(); return }
    await loadDetail(parent)
    await syncTreeChildren(parent)
  } catch (e) {
    ElMessage.error(e.message || '删除失败')
  }
}

function back() { router.push('/assets/middleware/zookeeper') }
</script>

<template>
  <section class="zk-page">
    <header class="zk-head">
      <el-button link type="primary" :icon="ArrowLeft" @click="back">数据源</el-button>
      <div class="zk-title"><h1>{{ source?.name || 'ZooKeeper' }}</h1><p>{{ source?.description || '浏览 ZNode 层级、节点数据与 ACL，变更使用版本校验。' }}</p></div>
      <el-button :icon="Refresh" :loading="refreshing" :disabled="!canRead" @click="reload">刷新</el-button>
      <el-button type="primary" :disabled="!can('write') || !current" @click="openCreate">新建 Znode</el-button>
    </header>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon/>

    <div class="zk-body">
      <aside class="zk-side">
        <div class="zk-search">
          <el-input v-model="keyword" :prefix-icon="Search" placeholder="路径示例，例如 /app/" clearable aria-label="筛选或跳转节点路径" @keyup.enter="jumpTo"/>
        </div>
        <div class="zk-tree">
          <el-tree
            ref="treeRef"
            :key="treeKey"
            lazy
            :load="loadTreeNode"
            :props="treeProps"
            node-key="path"
            highlight-current
            :expand-on-click-node="false"
            :filter-node-method="filterNode"
            empty-text="没有匹配的节点"
            @node-click="onNodeClick"
          >
            <template #default="{ node, data }">
              <span class="zk-node">
                <el-icon class="zk-node-icon"><FolderOpened v-if="data.hasChildren && node.expanded"/><Folder v-else-if="data.hasChildren"/><Document v-else/></el-icon>
                <span class="zk-node-label">{{ data.name }}</span>
              </span>
            </template>
          </el-tree>
          <p v-if="truncatedChildren" class="zk-note">根目录节点较多，仅展示前 500 个子节点。</p>
        </div>
      </aside>

      <main v-loading="loading" class="zk-main">
        <div v-if="!canRead" class="zk-empty">当前账号只有查看数据源的权限，查询节点需要额外授权。</div>
        <div v-else-if="!current && !loading" class="zk-empty">从左侧选择节点，或输入完整路径后回车。</div>
        <template v-else-if="current">
          <div class="zk-detail-head">
            <nav class="zk-crumbs" aria-label="节点路径">
              <template v-for="(seg, index) in segments" :key="seg.path">
                <span v-if="index > 1" class="zk-crumb-sep">/</span>
                <button type="button" class="zk-crumb" :class="{ active: seg.path === currentPath }" @click="loadDetail(seg.path)">{{ seg.name }}</button>
              </template>
            </nav>
            <div class="zk-detail-actions">
              <el-button v-if="can('write')" @click="openEdit">编辑</el-button>
              <el-button v-if="can('delete')" type="danger" plain :disabled="currentPath === '/'" @click="removeNode">删除</el-button>
            </div>
          </div>

          <div class="zk-meta">
            <span>修订 <b>{{ stat.Mzxid ?? '—' }}</b></span>
            <span>版本 <b>{{ stat.Version ?? '—' }}</b></span>
            <span>租约 <b>{{ isEphemeral ? zkHex(stat.EphemeralOwner) : '-' }}</b></span>
            <span>大小 <b>{{ zkFormatBytes(stat.DataLength) }}</b></span>
            <span v-if="isEphemeral" class="zk-badge">临时节点</span>
          </div>

          <div class="zk-value">
            <pre>{{ shownValue || '（空节点，无数据）' }}</pre>
            <div class="zk-value-foot">
              <span>{{ formatted ? '已格式化为缩进 JSON' : '' }}</span>
              <el-button size="small" :disabled="!valueText" @click="toggleFormat">{{ formatted ? '还原' : '格式化' }}</el-button>
            </div>
          </div>

          <div class="zk-section-head"><h2>节点信息</h2><span>{{ stat.NumChildren ?? 0 }} 个子节点 · 最后修改 {{ zkTime(stat.Mtime) }}</span></div>
          <dl class="zk-info">
            <div v-for="row in statRows" :key="row.key"><dt>{{ row.key }}</dt><dd>{{ row.value }}</dd></div>
          </dl>

          <details v-if="aclRows.length" class="zk-acl">
            <summary>ACL 权限（{{ aclRows.length }} 条）</summary>
            <ul><li v-for="row in aclRows" :key="row.id"><code>{{ row.id }}</code><span>{{ row.perms }}</span></li></ul>
          </details>
        </template>
      </main>
    </div>

    <el-dialog v-model="editDialog" :title="`编辑节点 · ${currentPath}`" width="min(880px, 94vw)" top="6vh" :close-on-click-modal="false" class="zk-dialog">
      <p class="zk-dialog-note">基于当前版本 {{ stat.Version ?? 0 }} 保存，若期间有其他客户端修改，保存会被拒绝。</p>
      <el-input v-model="editForm.value" type="textarea" :rows="14" spellcheck="false" class="zk-editor" placeholder="节点数据，可为空"/>
      <template #footer>
        <el-button :disabled="saving" @click="editDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="createDialog" title="新建 Znode" width="min(720px, 94vw)" :close-on-click-modal="false" class="zk-dialog">
      <el-form label-position="top" :disabled="saving">
        <el-form-item label="父节点"><el-input :model-value="currentPath" disabled/></el-form-item>
        <el-form-item label="节点名称" required><el-input v-model="createForm.name" placeholder="例如：Cluster" maxlength="255"/></el-form-item>
        <el-form-item label="节点数据"><el-input v-model="createForm.value" type="textarea" :rows="8" spellcheck="false" class="zk-editor" placeholder="可选，创建后可在编辑中修改"/></el-form-item>
      </el-form>
      <p class="zk-dialog-note">新节点为持久节点，ACL 继承父节点：{{ current?.acl?.[0] ? `${current.acl[0].Scheme}:${current.acl[0].ID}` : '默认' }}。</p>
      <template #footer>
        <el-button :disabled="saving" @click="createDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveCreate">创建</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.zk-page{min-width:0;display:flex;flex-direction:column;gap:14px;height:calc(100vh - 190px);min-height:560px;color:var(--el-text-color-primary,#23364f)}
.zk-head{box-shadow:0 1px 2px #1d345b04;display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:16px 20px;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px}
.zk-title{margin-right:auto;min-width:0}.zk-title h1{margin:0;font-size:22px}.zk-title p{margin:6px 0 0;font-size:14px;color:var(--el-text-color-secondary,#8795aa);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.zk-body{display:grid;grid-template-columns:360px minmax(0,1fr);gap:14px;flex:1;min-height:0}
.zk-side,.zk-main{min-width:0;display:flex;flex-direction:column;min-height:0;background:var(--el-bg-color,#fff);border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:12px;overflow:hidden}
.zk-search{padding:12px;border-bottom:1px solid var(--el-border-color-lighter,#eef2f8)}
.zk-search :deep(.el-input__inner){font-size:15px}
.zk-head :deep(.el-button),.zk-detail-actions :deep(.el-button),.zk-value-foot :deep(.el-button){font-size:15px}
.zk-head :deep(.el-button.is-link){font-size:15px}
.zk-tree{flex:1;min-height:0;overflow:auto;padding:8px 6px 14px}
.zk-tree :deep(.el-tree-node__content){height:38px;border-radius:6px}
.zk-tree :deep(.el-tree-node__content:hover){background:var(--el-fill-color-light,#f3f6fb)}
.zk-tree :deep(.el-tree-node.is-current > .el-tree-node__content){background:var(--el-color-primary-light-9,#e8f1fb);color:var(--el-color-primary,#3b68b0)}
.zk-node{display:flex;align-items:center;gap:8px;min-width:0;font-size:16px}
.zk-node-icon{color:#5b8dd6;font-size:18px;flex-shrink:0}
.zk-node-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.zk-note{margin:8px 12px 0;font-size:14px;color:var(--el-text-color-secondary,#8795aa)}
.zk-main{padding:18px 22px 24px;overflow:auto}
.zk-empty{display:grid;place-items:center;flex:1;min-height:220px;font-size:15px;color:var(--el-text-color-secondary,#8795aa)}
.zk-detail-head{display:flex;align-items:center;gap:16px;margin-bottom:10px}
.zk-crumbs{display:flex;align-items:center;gap:0;flex-wrap:wrap;min-width:0;font-size:16px}
.zk-crumb{border:0;background:none;padding:0 1px;color:var(--el-text-color-primary,#23364f);font:inherit;cursor:pointer;border-radius:4px}
.zk-crumb:hover{color:var(--el-color-primary,#3b68b0);text-decoration:underline}
.zk-crumb.active{font-weight:600}
.zk-crumb-sep{color:#bdc6d3;padding:0 1px}
.zk-detail-actions{display:flex;gap:8px;margin-left:auto;flex-shrink:0}
.zk-meta{display:flex;align-items:center;gap:26px;flex-wrap:wrap;font-size:14.5px;color:var(--el-text-color-secondary,#8a97ab);margin-bottom:16px}
.zk-meta b{color:var(--el-text-color-primary,#23364f);font-weight:600;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;margin-left:7px}
.zk-badge{padding:2px 10px;border-radius:10px;background:#fdf1e3;color:#b8722a;font-size:12.5px}
.zk-value{border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:8px;background:var(--el-fill-color-blank,#fff)}
.zk-value pre{margin:0;padding:14px 16px;max-height:280px;overflow:auto;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:15px;line-height:1.75;color:#2d3f5c;white-space:pre-wrap;overflow-wrap:anywhere}
.zk-value-foot{display:flex;align-items:center;justify-content:space-between;gap:12px;min-height:46px;padding:0 10px 0 16px;border-top:1px solid var(--el-border-color-lighter,#eef2f8)}
.zk-value-foot>span{font-size:14px;color:var(--el-text-color-secondary,#8795aa)}
.zk-section-head{display:flex;align-items:baseline;gap:12px;margin:24px 0 14px;flex-wrap:wrap}
.zk-section-head h2{margin:0;font-size:16px;font-weight:600}
.zk-section-head span{font-size:14px;color:var(--el-text-color-secondary,#8795aa)}
.zk-info{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px 40px;margin:0}
.zk-info>div{display:grid;grid-template-columns:168px minmax(0,1fr);align-items:baseline;gap:8px}
.zk-info dt{font-size:14px;color:var(--el-text-color-secondary,#8a97ab);font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.zk-info dd{margin:0;font-size:15px;color:#2f6bb5;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.zk-acl{margin-top:22px;border-top:1px dashed var(--el-border-color-light,#e4eaf4);padding-top:14px;font-size:14px;color:var(--el-text-color-secondary,#8a97ab)}
.zk-acl summary{cursor:pointer;font-size:15px;color:var(--el-text-color-primary,#23364f)}
.zk-acl ul{list-style:none;margin:10px 0 0;padding:0;display:grid;gap:8px}
.zk-acl li{display:flex;gap:16px;align-items:center}.zk-acl code{color:#2f6bb5}.zk-acl span{font-family:ui-monospace,monospace}
.zk-dialog-note{margin:0 0 12px;font-size:14px;line-height:1.8;color:var(--el-text-color-secondary,#8795aa)}
.zk-editor :deep(textarea){font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:15px;line-height:1.75}
@media (max-width:1080px){.zk-page{height:auto}.zk-body{grid-template-columns:1fr}.zk-tree{max-height:320px}.zk-info{grid-template-columns:1fr}}
</style>
