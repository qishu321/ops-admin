<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { copyTextToClipboard } from '../../utils/clipboard'
import { normalizeNavigationOrigins, buildPublicNavigationLink } from '../../utils/navigation-links'
import {
  deleteNavigation,
  deleteNavigationGroup,
  queryNavigationGroups,
  queryNavigations,
  regenerateNavigationGroupToken,
  saveNavigation,
  saveNavigationGroup
} from '../../api/integration'

const loading = ref(false)
const groups = ref([])
const navigations = ref([])
const selectedGroupId = ref(0)
const keyword = ref('')
const groupDialogVisible = ref(false)
const navigationDialogVisible = ref(false)
const saving = ref(false)
const copying = ref(false)
const copyDialogVisible = ref(false)
const manualCopyURL = ref('')
const manualCopyInput = ref()
const linkDialogVisible = ref(false), savingLinks = ref(false), linkGroupId = ref(0)
const originChoices = reactive({})
const linkDraft = ref([]), defaultOriginId = ref(0)
let originRowId = 0
const currentOrigin = window.location.origin
const publicOrigins = computed(() => [...new Set([...(selectedGroup.value?.publicOrigins || []), window.location.origin])])
const activePublicOrigin = computed({
  get: () => publicOrigins.value.includes(originChoices[selectedGroupId.value]) ? originChoices[selectedGroupId.value] : publicOrigins.value[0],
  set: value => { originChoices[selectedGroupId.value] = value }
})
function addOrigin(value = '') {
  if (linkDraft.value.length >= 10) return ElMessage.warning('最多配置 10 个访问入口')
  const id = ++originRowId
  linkDraft.value.push({ id, value })
  if (!defaultOriginId.value) defaultOriginId.value = id
}
function removeOrigin(id) {
  linkDraft.value = linkDraft.value.filter(row => row.id !== id)
  if (defaultOriginId.value === id) defaultOriginId.value = linkDraft.value[0]?.id || 0
}
function openLinkEditor() {
  linkGroupId.value = selectedGroupId.value
  linkDraft.value = []; defaultOriginId.value = 0
  const values = selectedGroup.value?.publicOrigins?.length ? selectedGroup.value.publicOrigins : [window.location.origin]
  values.forEach(value => addOrigin(value))
  linkDialogVisible.value = true
}
function linkPreview(value) {
  try { return buildPublicNavigationLink(normalizeNavigationOrigins([value])[0], selectedGroup.value?.publicToken) } catch { return '' }
}
async function savePublicLinks() {
  const group = groups.value.find(item => item.id === linkGroupId.value)
  if (!group) return
  let origins
  try {
    const preferred = linkDraft.value.find(row => row.id === defaultOriginId.value)
    origins = normalizeNavigationOrigins([...(preferred ? [preferred.value] : []), ...linkDraft.value.filter(row => row.id !== defaultOriginId.value).map(row => row.value)])
  } catch (error) { ElMessage.warning(error.message); return }
  savingLinks.value = true
  try {
    await saveNavigationGroup({ ...group, publicOrigins: origins })
    await loadGroups(group.id)
    delete originChoices[group.id]
    linkDialogVisible.value = false
    ElMessage.success('公开链接入口已保存')
  } finally { savingLinks.value = false }
}

const groupForm = reactive({ id: undefined, name: '', description: '', isPublic: false, status: 1, sort: 0, publicOrigins: [] })
const navigationForm = reactive({
  id: undefined,
  groupId: 0,
  name: '',
  description: '',
  url: '',
  iconUrl: '',
  openMode: 'new',
  status: 1,
  sort: 0
})

const selectedGroup = computed(() => groups.value.find((item) => item.id === selectedGroupId.value))
const totalNavigationCount = computed(() => groups.value.reduce((total, item) => total + Number(item.itemCount || 0), 0))
const publicGroupCount = computed(() => groups.value.filter((item) => item.isPublic).length)
const publicURL = computed(() => {
  if (!selectedGroup.value?.isPublic || !selectedGroup.value?.publicToken) return ''
  return buildPublicNavigationLink(activePublicOrigin.value, selectedGroup.value.publicToken)
})

function resetGroupForm() {
  Object.assign(groupForm, { id: undefined, name: '', description: '', isPublic: false, status: 1, sort: 0, publicOrigins: [] })
}

function resetNavigationForm() {
  Object.assign(navigationForm, {
    id: undefined,
    groupId: selectedGroupId.value,
    name: '',
    description: '',
    url: '',
    iconUrl: '',
    openMode: 'new',
    status: 1,
    sort: 0
  })
}

async function loadGroups(preferredId = selectedGroupId.value) {
  const data = await queryNavigationGroups()
  groups.value = data || []
  if (preferredId && groups.value.some((item) => item.id === preferredId)) {
    selectedGroupId.value = preferredId
  } else {
    selectedGroupId.value = groups.value[0]?.id || 0
  }
}

async function loadNavigations() {
  if (!selectedGroupId.value) {
    navigations.value = []
    return
  }
  loading.value = true
  try {
    const data = await queryNavigations({ groupId: selectedGroupId.value, keyword: keyword.value })
    navigations.value = data || []
  } finally {
    loading.value = false
  }
}

async function selectGroup(id) {
  selectedGroupId.value = id
  keyword.value = ''
  await loadNavigations()
}

function openCreateGroup() {
  resetGroupForm()
  groupDialogVisible.value = true
}

function openEditGroup() {
  if (!selectedGroup.value) return
  Object.assign(groupForm, selectedGroup.value)
  groupDialogVisible.value = true
}

async function submitGroup() {
  if (!groupForm.name.trim()) {
    ElMessage.warning('请输入导航组名称')
    return
  }
  saving.value = true
  try {
    const data = await saveNavigationGroup(groupForm)
    groupDialogVisible.value = false
    await loadGroups(data.id)
    await loadNavigations()
    ElMessage.success('导航组已保存')
  } finally {
    saving.value = false
  }
}

async function removeGroup() {
  if (!selectedGroup.value) return
  await ElMessageBox.confirm(`删除“${selectedGroup.value.name}”后，组内导航也会一并删除，确认继续吗？`, '删除导航组', {
    type: 'warning',
    confirmButtonText: '删除',
    cancelButtonText: '取消'
  })
  await deleteNavigationGroup(selectedGroup.value.id)
  await loadGroups(0)
  await loadNavigations()
  ElMessage.success('导航组已删除')
}

function openCreateNavigation() {
  if (!selectedGroupId.value) {
    ElMessage.warning('请先创建导航组')
    return
  }
  resetNavigationForm()
  navigationDialogVisible.value = true
}

function openEditNavigation(item) {
  Object.assign(navigationForm, item)
  navigationDialogVisible.value = true
}

async function submitNavigation() {
  if (!navigationForm.name.trim() || !navigationForm.url.trim()) {
    ElMessage.warning('请输入导航名称和访问地址')
    return
  }
  saving.value = true
  try {
    await saveNavigation(navigationForm)
    navigationDialogVisible.value = false
    await loadGroups(selectedGroupId.value)
    await loadNavigations()
    ElMessage.success('导航已保存')
  } finally {
    saving.value = false
  }
}

async function removeNavigation(item) {
  await ElMessageBox.confirm(`确认删除导航“${item.name}”吗？`, '删除导航', { type: 'warning' })
  await deleteNavigation(item.id)
  await loadGroups(selectedGroupId.value)
  await loadNavigations()
  ElMessage.success('导航已删除')
}

function openNavigation(item) {
  if (item.openMode === 'current') {
    window.location.href = item.url
    return
  }
  const target = window.open(item.url, '_blank', 'noopener,noreferrer')
  if (target) target.opener = null
}

async function copyPublicURL() {
  const url = publicURL.value
  if (!url || copying.value) return
  copying.value = true
  try {
    if (await copyTextToClipboard(url)) {
      ElMessage.success('公开链接已复制')
      return
    }
  } catch {
    // Always leave a usable manual path when browser clipboard access fails.
  } finally {
    copying.value = false
  }
  manualCopyURL.value = url
  copyDialogVisible.value = true
}

async function regenerateToken() {
  await ElMessageBox.confirm('重新生成后，原公开链接将立即失效，确认继续吗？', '重新生成公开链接', { type: 'warning' })
  await regenerateNavigationGroupToken(selectedGroupId.value)
  await loadGroups(selectedGroupId.value)
  ElMessage.success('公开链接已重新生成')
}

function iconText(name) {
  return String(name || 'N').trim().slice(0, 1).toUpperCase()
}

function displayHost(value) {
  try {
    return new URL(value).host
  } catch {
    return value || '-'
  }
}

function toneClass(item) {
  return `tone-${Math.abs(Number(item?.id) || 0) % 5}`
}

onMounted(async () => {
  await loadGroups()
  await loadNavigations()
})
</script>

<template>
  <div class="integration-page">
    <section class="integration-header">
      <div class="header-copy">
        <span class="hub-mark">NH</span>
        <div>
          <span class="eyebrow">INTEGRATION NAVIGATION</span>
          <h1>导航管理</h1>
          <p>按场景沉淀内部系统、运维工具和第三方平台入口，并支持按组公开分享。</p>
        </div>
      </div>
      <div class="header-actions">
        <el-button @click="openCreateGroup">新增分组</el-button>
        <el-button type="primary" :disabled="!selectedGroupId" @click="openCreateNavigation">新增导航</el-button>
      </div>
    </section>

    <section class="navigation-summary" aria-label="导航资源概览">
      <div class="summary-cell tone-blue">
        <div><i></i><span>导航分组</span><em>GROUP</em></div>
        <strong>{{ groups.length }}</strong>
      </div>
      <div class="summary-cell tone-cyan">
        <div><i></i><span>系统入口</span><em>ENTRY</em></div>
        <strong>{{ totalNavigationCount }}</strong>
      </div>
      <div class="summary-cell tone-green">
        <div><i></i><span>公开分组</span><em>PUBLIC</em></div>
        <strong>{{ publicGroupCount }}</strong>
      </div>
      <div class="summary-cell tone-violet">
        <div><i></i><span>当前分组</span><em>ACTIVE</em></div>
        <strong class="active-group-name">{{ selectedGroup?.name || '-' }}</strong>
      </div>
    </section>

    <section class="integration-workspace">
      <aside class="group-panel">
        <div class="panel-title">
          <div><span class="panel-kicker">GROUP DIRECTORY</span><strong>导航分组</strong><small>按使用场景组织入口</small></div>
          <el-button link type="primary" @click="openCreateGroup">新增</el-button>
        </div>
        <div v-if="groups.length" class="group-list">
          <button
            v-for="group in groups"
            :key="group.id"
            type="button"
            class="group-item"
            :class="{ active: selectedGroupId === group.id }"
            @click="selectGroup(group.id)"
          >
            <span class="group-mark" :class="toneClass(group)">{{ iconText(group.name) }}</span>
            <span class="group-copy">
              <strong>{{ group.name }}</strong>
              <small>{{ group.itemCount }} 个导航</small>
            </span>
            <el-tag v-if="group.isPublic" size="small" type="success" effect="light">公开</el-tag>
          </button>
        </div>
        <el-empty v-else description="暂无导航组" :image-size="72" />
      </aside>

      <main class="navigation-panel">
        <template v-if="selectedGroup">
          <div class="group-heading">
            <div>
              <span class="section-label">GROUP WORKSPACE</span>
              <div class="heading-line">
                <h2>{{ selectedGroup.name }}</h2>
                <el-tag :type="selectedGroup.status === 1 ? 'success' : 'info'" effect="light">
                  {{ selectedGroup.status === 1 ? '启用' : '禁用' }}
                </el-tag>
              </div>
              <p>{{ selectedGroup.description || '暂无分组说明' }}</p>
            </div>
            <div class="group-actions">
              <el-button @click="openEditGroup">编辑分组</el-button>
              <el-button type="danger" plain @click="removeGroup">删除分组</el-button>
            </div>
          </div>

          <div v-if="publicURL" class="public-link-bar">
            <div class="public-status"><span></span><strong>公开访问已开启</strong></div>
            <div class="public-link-address"><el-select v-if="publicOrigins.length > 1" v-model="activePublicOrigin" size="small" aria-label="公开链接访问入口"><el-option v-for="origin in publicOrigins" :key="origin" :value="origin" :label="origin === currentOrigin ? `当前地址 · ${origin}` : origin" /></el-select><code :title="publicURL">{{ publicURL }}</code></div>
            <el-button size="small" @click="openLinkEditor">编辑链接</el-button>
            <el-button size="small" :loading="copying" @click="copyPublicURL">复制链接</el-button>
            <el-button size="small" @click="regenerateToken">重新生成</el-button>
          </div>

          <div class="navigation-toolbar">
            <el-input v-model="keyword" clearable placeholder="搜索导航名称、说明或地址" @keyup.enter="loadNavigations" />
            <el-button @click="loadNavigations">搜索</el-button>
            <span class="navigation-count"><strong>{{ navigations.length }}</strong> 个导航</span>
          </div>

          <div v-loading="loading" class="navigation-grid">
            <article v-for="item in navigations" :key="item.id" class="navigation-card" :class="toneClass(item)" @click="openNavigation(item)">
              <div class="card-main">
              <div class="navigation-icon" :class="toneClass(item)">
                <img v-if="item.iconUrl" :src="item.iconUrl" alt="" />
                <span v-else>{{ iconText(item.name) }}</span>
              </div>
              <div class="navigation-info">
                <div class="navigation-name">
                  <strong>{{ item.name }}</strong>
                  <el-tag v-if="item.status !== 1" size="small" type="info">禁用</el-tag>
                </div>
                <p>{{ item.description || '暂无入口说明' }}</p>
              </div>
              <span class="open-arrow">↗</span>
              </div>
              <div class="card-footer">
                <span class="host-name">{{ displayHost(item.url) }}</span>
                <div class="card-actions" @click.stop>
                <el-button link type="primary" @click="openEditNavigation(item)">编辑</el-button>
                <el-button link type="danger" @click="removeNavigation(item)">删除</el-button>
                </div>
              </div>
            </article>
            <button type="button" class="navigation-card add-card" @click="openCreateNavigation">
              <span class="add-symbol">+</span><strong>新增导航</strong><small>添加新的系统或工具入口</small>
            </button>
          </div>
        </template>
        <el-empty v-else description="创建一个导航组后开始添加导航" />
      </main>
    </section>

    <el-dialog v-model="linkDialogVisible" title="编辑公开链接" width="min(740px, calc(100vw - 32px))" :close-on-click-modal="false" :close-on-press-escape="!savingLinks" :show-close="!savingLinks" class="public-links-dialog">
      <div class="public-link-intro"><strong>为同一个导航组配置多个访问入口</strong><p>填写已部署平台的 IP、域名及端口，也可粘贴完整公开链接。分享标识自动保留；新增地址需要已能访问本平台。</p></div>
      <div class="origin-editor-heading"><span>访问入口 <small>最多 10 个，选中一项作为默认入口</small></span><el-button size="small" type="primary" plain :disabled="savingLinks || linkDraft.length >= 10" @click="addOrigin()">新增地址</el-button></div>
      <el-radio-group v-model="defaultOriginId" class="origin-editor" :disabled="savingLinks">
        <div v-for="(row, index) in linkDraft" :key="row.id" class="origin-editor-row"><div class="origin-row-heading"><el-radio :value="row.id">入口 {{ index + 1 }}<el-tag v-if="defaultOriginId === row.id" size="small" effect="plain">默认</el-tag></el-radio><el-button link type="danger" :disabled="savingLinks" :aria-label="`移除入口 ${index + 1}`" @click="removeOrigin(row.id)">移除</el-button></div><el-input v-model="row.value" :disabled="savingLinks" :aria-label="`入口地址 ${index + 1}`" placeholder="例如：https://ops.example.com 或 http://192.168.1.10:8080" maxlength="2048" /><small v-if="row.value && linkPreview(row.value)" class="origin-link-preview">{{ linkPreview(row.value) }}</small></div>
      </el-radio-group>
      <el-empty v-if="!linkDraft.length" description="未配置入口时，使用当前访问地址" :image-size="55" />
      <template #footer><el-button :disabled="savingLinks" @click="linkDialogVisible = false">取消</el-button><el-button type="primary" :loading="savingLinks" @click="savePublicLinks">保存链接</el-button></template>
    </el-dialog>

    <el-dialog v-model="copyDialogVisible" title="手动复制公开链接" width="min(640px, calc(100vw - 32px))" @opened="manualCopyInput?.select()">
      <p>浏览器限制了自动复制，请选中下方链接后按 Ctrl+C（Mac 使用 ⌘C）复制。</p>
      <el-input ref="manualCopyInput" :model-value="manualCopyURL" readonly aria-label="公开导航链接" @focus="$event.target.select()" />
      <template #footer><el-button @click="copyDialogVisible = false">关闭</el-button></template>
    </el-dialog>

    <el-dialog v-model="groupDialogVisible" :title="groupForm.id ? '编辑导航组' : '新增导航组'" width="620px" destroy-on-close>
      <el-form label-position="top">
        <div class="form-grid two-columns">
          <el-form-item label="导航组名称" required><el-input v-model="groupForm.name" maxlength="128" /></el-form-item>
          <el-form-item label="排序"><el-input-number v-model="groupForm.sort" :min="0" :max="9999" controls-position="right" /></el-form-item>
        </div>
        <el-form-item label="分组说明"><el-input v-model="groupForm.description" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
        <div class="form-grid two-columns">
          <el-form-item label="状态">
            <el-radio-group v-model="groupForm.status"><el-radio :value="1">启用</el-radio><el-radio :value="2">禁用</el-radio></el-radio-group>
          </el-form-item>
          <el-form-item label="公开访问">
            <el-switch v-model="groupForm.isPublic" active-text="允许免登录访问" />
          </el-form-item>
        </div>
        <el-alert v-if="groupForm.isPublic" type="warning" :closable="false" show-icon title="公开后，任何获得链接的人都可以查看并打开组内启用的导航。" />
      </el-form>
      <template #footer><el-button @click="groupDialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="submitGroup">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="navigationDialogVisible" :title="navigationForm.id ? '编辑导航' : '新增导航'" width="680px" destroy-on-close>
      <el-form label-position="top">
        <div class="form-grid two-columns">
          <el-form-item label="导航名称" required><el-input v-model="navigationForm.name" maxlength="128" /></el-form-item>
          <el-form-item label="所属分组" required>
            <el-select v-model="navigationForm.groupId" style="width: 100%"><el-option v-for="group in groups" :key="group.id" :label="group.name" :value="group.id" /></el-select>
          </el-form-item>
        </div>
        <el-form-item label="访问地址" required><el-input v-model="navigationForm.url" placeholder="https://example.com" /></el-form-item>
        <el-form-item label="图标地址"><el-input v-model="navigationForm.iconUrl" placeholder="可选，填写 HTTP/HTTPS 图片地址；留空显示名称首字母" /></el-form-item>
        <el-form-item label="导航说明"><el-input v-model="navigationForm.description" type="textarea" :rows="2" maxlength="500" show-word-limit /></el-form-item>
        <div class="form-grid three-columns">
          <el-form-item label="打开方式"><el-radio-group v-model="navigationForm.openMode"><el-radio value="new">新窗口</el-radio><el-radio value="current">当前窗口</el-radio></el-radio-group></el-form-item>
          <el-form-item label="状态"><el-switch v-model="navigationForm.status" :active-value="1" :inactive-value="2" active-text="启用" /></el-form-item>
          <el-form-item label="排序"><el-input-number v-model="navigationForm.sort" :min="0" :max="9999" controls-position="right" /></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="navigationDialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="submitNavigation">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.integration-page { display: flex; flex-direction: column; gap: 14px; color: #10213e; }
.integration-header,
.navigation-summary,
.integration-workspace { border: 1px solid #dfe8f5; border-radius: 8px; box-shadow: 0 8px 24px rgba(35, 63, 112, .05); }
.integration-header { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 16px 18px; background: #fff; }
.header-copy { display: flex; min-width: 0; align-items: center; gap: 14px; }
.hub-mark { display: grid; width: 42px; height: 42px; flex: 0 0 42px; color: #fff; background: #2563eb; border-radius: 8px; font: 800 14px/1 ui-monospace, SFMono-Regular, Menlo, monospace; place-items: center; box-shadow: 0 8px 18px rgba(37, 99, 235, .24); }
.eyebrow, .section-label { color: #356fd6; font-size: 11px; font-weight: 800; letter-spacing: 1.1px; }
.integration-header h1 { margin: 4px 0; font-size: 23px; line-height: 1.2; }
.integration-header p, .group-heading p { margin: 0; color: #74839d; }
.header-actions { display: flex; flex-shrink: 0; gap: 9px; }
.navigation-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); overflow: hidden; background: #fff; }
.summary-cell { min-width: 0; padding: 11px 14px; border-right: 1px solid #e8eef7; }
.summary-cell:last-child { border-right: 0; }
.summary-cell > div { display: flex; min-width: 0; align-items: center; gap: 6px; }
.summary-cell i { width: 6px; height: 6px; flex: none; background: var(--tone); border-radius: 2px; }
.summary-cell span { color: #52637f; font-size: 11px; font-weight: 700; white-space: nowrap; }
.summary-cell em { margin-left: auto; overflow: hidden; color: var(--tone); font: 700 9px/1 ui-monospace, SFMono-Regular, Menlo, monospace; font-style: normal; text-overflow: ellipsis; white-space: nowrap; opacity: .72; }
.summary-cell strong { display: block; margin-top: 7px; color: #10213e; font-size: 22px; line-height: 1; }
.summary-cell .active-group-name { overflow: hidden; font-size: 16px; text-overflow: ellipsis; white-space: nowrap; }
.tone-blue { --tone: #3b82f6; }
.tone-cyan { --tone: #06b6d4; }
.tone-green { --tone: #10b981; }
.tone-violet { --tone: #7c3aed; }
.integration-workspace { display: grid; grid-template-columns: 252px minmax(0, 1fr); min-height: 630px; overflow: hidden; background: #fff; }
.group-panel { padding: 17px 10px; border-right: 1px solid #e5ebf4; background: linear-gradient(180deg, #f7faff, #f9fbfe); }
.panel-title { display: flex; align-items: center; justify-content: space-between; padding: 0 9px 15px; }
.panel-title div { display: flex; flex-direction: column; gap: 3px; }
.panel-kicker { color: #6c86b2; font: 700 9px/1 ui-monospace, SFMono-Regular, Menlo, monospace; letter-spacing: .8px; }
.panel-title small { color: #8a97ac; font-size: 12px; }
.group-list { display: flex; flex-direction: column; gap: 5px; }
.group-item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 58px; padding: 10px; border: 1px solid transparent; border-radius: 7px; color: #34445f; background: transparent; text-align: left; cursor: pointer; transition: background-color .16s, border-color .16s, box-shadow .16s; }
.group-item:hover { border-color: #dce7f7; background: #fff; }
.group-item.active { border-color: #b8cff4; background: #fff; color: #1e5dbb; box-shadow: 0 4px 12px rgba(40, 89, 154, .07); }
.group-mark { display: grid; place-items: center; width: 34px; height: 34px; flex: 0 0 34px; border-radius: 7px; background: #e5efff; color: #2563c3; font-weight: 800; }
.group-copy { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 3px; }
.group-copy strong, .group-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-copy small { color: #8996aa; }
.navigation-panel { min-width: 0; padding: 20px 22px 26px; }
.group-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; padding-bottom: 16px; border-bottom: 1px solid #edf1f6; }
.heading-line { display: flex; align-items: center; gap: 10px; margin-top: 4px; }
.heading-line h2 { margin: 0 0 4px; font-size: 22px; }
.group-actions { display: flex; flex-shrink: 0; }
.public-link-bar { display: grid; grid-template-columns: auto minmax(120px, 1fr) auto auto auto; align-items: center; gap: 9px; margin-top: 14px; padding: 9px 11px; border: 1px solid #bce5cd; border-radius: 7px; background: #f3fbf6; }
.public-status { display: flex; align-items: center; gap: 7px; color: #187847; white-space: nowrap; }
.public-status span { width: 7px; height: 7px; border-radius: 50%; background: #22b66c; }
.public-link-bar code { display:block; font-size:11px; overflow: hidden; color: #46617c; text-overflow: ellipsis; white-space: nowrap; }
.navigation-toolbar { display: flex; align-items: center; gap: 9px; margin: 16px 0 13px; }
.navigation-toolbar .el-input { width: min(420px, 55%); }
.navigation-count { margin-left: auto; color: #8996aa; }
.navigation-count strong { color: #2a4e7f; }
.navigation-grid { display: grid; grid-template-columns: repeat(4, minmax(210px, 1fr)); gap: 12px; align-content: start; min-height: 300px; }
.navigation-card { position: relative; display: flex; min-width: 0; min-height: 130px; padding: 0; overflow: hidden; border: 1px solid #dfe7f1; border-left: 4px solid #5b8def; border-radius: 7px; color: inherit; background: #fff; flex-direction: column; cursor: pointer; transition: border-color .18s, box-shadow .18s, transform .18s; }
.navigation-card:hover { transform: translateY(-2px); border-color: #a7c1ea; box-shadow: 0 9px 22px rgba(34, 82, 145, .1); }
.card-main { display: flex; min-width: 0; flex: 1; align-items: flex-start; gap: 11px; padding: 14px 13px 10px; }
.navigation-icon { display: grid; place-items: center; width: 38px; height: 38px; flex: 0 0 38px; overflow: hidden; border-radius: 7px; background: #eaf2ff; color: #2f6ed3; font-size: 16px; font-weight: 800; }
.navigation-icon img { width: 27px; height: 27px; object-fit: contain; }
.navigation-info { min-width: 0; flex: 1; }
.navigation-name { display: flex; align-items: center; gap: 6px; min-height: 24px; }
.navigation-name strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.navigation-info p { display: -webkit-box; min-height: 35px; margin: 6px 0 0; overflow: hidden; color: #6b7b92; font-size: 13px; line-height: 1.4; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.open-arrow { color: #9aa8ba; font-size: 16px; }
.card-footer { display: flex; height: 36px; padding: 0 11px 0 14px; border-top: 1px solid #eef2f7; background: #fafbfd; align-items: center; justify-content: space-between; gap: 8px; }
.host-name { min-width: 0; overflow: hidden; color: #8492a6; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.card-actions { display: flex; flex: 0 0 auto; }
.card-actions :deep(.el-button) { margin-left: 7px; font-size: 12px; }
.tone-1 { border-left-color: #21a675; }
.tone-2 { border-left-color: #de8a24; }
.tone-3 { border-left-color: #18a3b7; }
.tone-4 { border-left-color: #7359c9; }
.navigation-icon.tone-1, .group-mark.tone-1 { color: #14805b; background: #e5f6ef; }
.navigation-icon.tone-2, .group-mark.tone-2 { color: #b76c12; background: #fff2df; }
.navigation-icon.tone-3, .group-mark.tone-3 { color: #127d8c; background: #e5f7fa; }
.navigation-icon.tone-4, .group-mark.tone-4 { color: #6046b3; background: #eeeafe; }
.add-card { justify-content: center; gap: 5px; min-height: 130px; border-left-width: 1px; border-style: dashed; color: #6f7f97; align-items: center; }
.add-symbol { font-size: 26px; line-height: 1; color: #477fd8; }
.add-card small { color: #9aa5b5; }
.form-grid { display: grid; gap: 16px; }
.two-columns { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
.three-columns { grid-template-columns: 1.4fr 1fr 1fr; }
@media (max-width: 1450px) { .navigation-grid { grid-template-columns: repeat(3, minmax(220px, 1fr)); } }
@media (max-width: 1100px) { .navigation-grid { grid-template-columns: repeat(2, minmax(220px, 1fr)); } }
@media (max-width: 800px) { .integration-header { align-items: flex-start; flex-direction: column; } .navigation-summary { grid-template-columns: repeat(2, 1fr); } .summary-cell:nth-child(2) { border-right: 0; } .summary-cell:nth-child(-n + 2) { border-bottom: 1px solid #e8eef7; } .header-actions, .group-heading { align-items: flex-start; } .group-heading { flex-direction: column; } .integration-workspace { grid-template-columns: 1fr; } .group-panel { border-right: 0; border-bottom: 1px solid #e7edf6; } .group-list { flex-direction: row; overflow-x: auto; } .group-item { min-width: 210px; } .navigation-grid { grid-template-columns: 1fr; } .public-link-bar { grid-template-columns: 1fr auto; } .public-link-address { grid-column: 1 / -1; } .form-grid { grid-template-columns: 1fr; } }
.public-link-address { min-width:0; display:grid; gap:6px; }.public-link-address .el-select { max-width:100%; width:100%; }
.public-link-intro { padding:14px 16px; border:1px solid #dfe9f7; border-radius:8px; background:#f5f9ff; color:#294c78; }.public-link-intro strong { font-size:14px; }.public-link-intro p { margin:8px 0 0; color:#7b8da6; line-height:1.8; font-size:12px; }
.origin-editor-heading { display:flex; justify-content:space-between; align-items:center; gap:12px; margin:20px 0 12px; font-weight:600; color:#354d6c; }.origin-editor-heading small { margin-left:10px; font-weight:400; color:#8c9bb0; font-size:11px; }
.origin-editor { display:flex; flex-direction:column; width:100%; gap:12px; align-items:stretch; max-height:42vh; overflow:auto; padding:2px; }.origin-editor-row { padding:12px 14px; border:1px solid #e3eaf4; border-radius:8px; background:#fcfdff; }.origin-row-heading { display:flex; align-items:center; justify-content:space-between; margin-bottom:6px; }.origin-row-heading .el-tag { margin-left:8px; font-size:10px; }.origin-link-preview { display:block; margin-top:8px; overflow-wrap:anywhere; color:#8394aa; font:11px/1.8 Consolas,monospace; }
</style>
