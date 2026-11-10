<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteLogBoardShare,
  queryLogBoardShares,
  regenerateLogBoardShareToken,
  saveLogBoardShare,
  updateLogBoardShareStatus
} from '../../../api/logBoardShare'
import { LOG_BOARD_TIME_RANGES, logBoardPublicUrl } from './logBoardShared'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  config: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['update:modelValue'])

const REFRESH_OPTIONS = [
  { label: '15 秒', value: 15 },
  { label: '30 秒', value: 30 },
  { label: '1 分钟', value: 60 },
  { label: '2 分钟', value: 120 },
  { label: '5 分钟', value: 300 }
]
const EXPIRY_OPTIONS = [
  { label: '永久有效', value: 0 },
  { label: '1 天后失效', value: 86400 },
  { label: '7 天后失效', value: 604800 },
  { label: '30 天后失效', value: 2592000 }
]

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const loading = ref(false)
const saving = ref(false)
const shares = ref([])
const editingId = ref(0)
const formVisible = ref(false)

const form = reactive({
  id: 0,
  name: '',
  description: '',
  datasourceId: undefined,
  query: '',
  rangeSeconds: 86400,
  refreshSeconds: 60,
  allowDetail: true,
  expiresInSeconds: 0
})

function resetForm() {
  editingId.value = 0
  form.id = 0
  form.name = ''
  form.description = ''
  form.datasourceId = props.config?.datasourceId
  form.query = props.config?.query || ''
  form.rangeSeconds = props.config?.rangeSeconds || 86400
  form.refreshSeconds = 60
  form.allowDetail = true
  form.expiresInSeconds = 0
}

function openCreate() {
  resetForm()
  formVisible.value = true
}

function openEdit(share) {
  editingId.value = share.id
  formVisible.value = true
  form.id = share.id
  form.name = share.name
  form.description = share.description || ''
  form.datasourceId = share.datasourceId
  form.query = share.query
  form.rangeSeconds = share.rangeSeconds
  form.refreshSeconds = share.refreshSeconds
  form.allowDetail = share.allowDetail
  form.expiresInSeconds = remainingExpirySeconds(share)
}

function remainingExpirySeconds(share) {
  if (!share?.expiresAt) return 0
  const remaining = Math.max(0, Number(share.expiresAt) - Date.now())
  const seconds = Math.round(remaining / 1000)
  const matched = EXPIRY_OPTIONS.filter(item => item.value > 0)
    .find(item => Math.abs(item.value - seconds) <= Math.max(600, item.value * 0.05))
  return matched ? matched.value : 0
}

function formatExpiry(share) {
  if (!share.expiresAt) return '永久有效'
  const date = new Date(Number(share.expiresAt))
  if (Number.isNaN(date.getTime())) return '永久有效'
  if (date.getTime() < Date.now()) return '已过期'
  const pad = value => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())} 到期`
}

function shareLink(share) {
  return logBoardPublicUrl(share.token)
}

function rangeText(seconds) {
  return LOG_BOARD_TIME_RANGES.find(item => item.seconds === Number(seconds))?.label || `${Math.round(Number(seconds || 0) / 3600)} 小时`
}

async function load() {
  loading.value = true
  try {
    shares.value = await queryLogBoardShares() || []
  } catch (error) {
    ElMessage.error(error?.message || '加载分享列表失败')
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!form.datasourceId) {
    ElMessage.warning('请先在大屏上选择数据源')
    return
  }
  saving.value = true
  try {
    const payload = {
      id: form.id,
      name: form.name,
      description: form.description,
      datasourceId: form.datasourceId,
      query: form.query,
      rangeSeconds: form.rangeSeconds,
      refreshSeconds: form.refreshSeconds,
      allowDetail: form.allowDetail,
      expiresInSeconds: form.expiresInSeconds
    }
    const share = await saveLogBoardShare(payload)
    await load()
    formVisible.value = false
    if (share?.token) {
      await copyLink(shareLink(share), true)
    } else {
      ElMessage.success('分享已保存')
    }
  } catch (error) {
    ElMessage.error(error?.message || '保存分享失败')
  } finally {
    saving.value = false
  }
}

async function copyLink(link, silent = false) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(link)
    } else {
      const input = document.createElement('textarea')
      input.value = link
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
    }
    ElMessage.success(silent ? `分享已创建，链接已复制：${link}` : '链接已复制')
  } catch {
    ElMessage.warning(`复制失败，请手动复制：${link}`)
  }
}

function openLink(share) {
  window.open(shareLink(share), '_blank', 'noopener,noreferrer')
}

async function toggleStatus(share) {
  try {
    await updateLogBoardShareStatus({ id: share.id, status: share.status === 1 ? 0 : 1 })
    await load()
  } catch (error) {
    ElMessage.error(error?.message || '更新状态失败')
  }
}

async function regenerate(share) {
  try {
    await ElMessageBox.confirm('重置后旧链接会立即失效，需要重新分发新链接。', '重置分享链接', { type: 'warning' })
  } catch {
    return
  }
  try {
    const next = await regenerateLogBoardShareToken(share.id)
    await load()
    if (next?.token) await copyLink(shareLink(next), true)
  } catch (error) {
    ElMessage.error(error?.message || '重置链接失败')
  }
}

async function remove(share) {
  try {
    await ElMessageBox.confirm(`删除后链接「${share.name}」将永久失效，确认删除？`, '删除分享', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteLogBoardShare(share.id)
    if (editingId.value === share.id) formVisible.value = false
    await load()
    ElMessage.success('分享已删除')
  } catch (error) {
    ElMessage.error(error?.message || '删除分享失败')
  }
}

watch(visible, (value) => {
  if (!value) return
  if (!shares.value.length) load()
  if (!formVisible.value) resetForm()
})
</script>

<template>
  <el-dialog v-model="visible" title="公开日志大屏" width="920px" top="6vh" class="log-board-share-dialog" append-to-body>
    <p class="share-intro">
      公开后，拿到链接的同事无需登录即可查看这块大屏，并且能在页面里自己切换数据源、修改 LogsQL、
      调整时间范围，所以换数据源不需要重新发链接。链接随时可以停用、重置或删除。
    </p>

    <div class="share-toolbar">
      <span class="share-count">{{ shares.length }} 个分享链接</span>
      <div class="share-actions">
        <el-button text :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" @click="openCreate">新建分享</el-button>
      </div>
    </div>

    <section v-if="formVisible" class="share-form">
      <header class="form-head">
        <h4>{{ form.id ? '编辑分享' : '新建分享' }}</h4>
        <span v-if="form.id" class="form-meta">修改后旧链接仍可用</span>
      </header>
      <div class="form-grid">
        <label class="field">
          <span>大屏名称</span>
          <el-input v-model="form.name" maxlength="80" show-word-limit placeholder="留空则按过滤条件自动命名" />
        </label>
        <label class="field">
          <span>说明</span>
          <el-input v-model="form.description" maxlength="200" placeholder="例如：客户端错误日志，供客户端组排查" />
        </label>
        <label class="field">
          <span>时间范围</span>
          <el-select v-model="form.rangeSeconds" class="full">
            <el-option v-for="item in LOG_BOARD_TIME_RANGES" :key="item.seconds" :label="item.label" :value="item.seconds" />
          </el-select>
        </label>
        <label class="field">
          <span>自动刷新</span>
          <el-select v-model="form.refreshSeconds" class="full">
            <el-option v-for="item in REFRESH_OPTIONS" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </label>
        <label class="field">
          <span>有效期</span>
          <el-select v-model="form.expiresInSeconds" class="full">
            <el-option v-for="item in EXPIRY_OPTIONS" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </label>
      </div>
      <div class="form-row">
        <el-checkbox v-model="form.allowDetail">允许查看单条日志详情（含堆栈正文）</el-checkbox>
        <span class="form-tip">拿到链接的人无需登录，且可以自己切换数据源、改 LogsQL、换时间范围</span>
      </div>
      <div class="form-footer">
        <el-input v-model="form.query" class="query-preview" readonly>
          <template #prepend>过滤条件</template>
        </el-input>
        <el-button @click="formVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存并复制链接</el-button>
      </div>
    </section>

    <div v-loading="loading" class="share-list">
      <article v-for="share in shares" :key="share.id" class="share-item">
        <div class="item-main">
          <div class="item-title">
            <strong>{{ share.name }}</strong>
            <span v-if="share.status === 1 && share.active" class="tag is-active">生效中</span>
            <span v-else-if="share.status !== 1" class="tag is-off">已停用</span>
            <span v-else class="tag is-warn">{{ share.unavailable || '不可用' }}</span>
            <span v-if="!share.allowDetail" class="tag">精简</span>
          </div>
          <p class="item-desc">{{ share.description || '未填写说明' }}</p>
          <div class="item-meta">
            <span>{{ share.datasourceName || '数据源已删除' }}</span>
            <span>{{ rangeText(share.rangeSeconds) }}</span>
            <span>{{ share.refreshSeconds }} 秒刷新</span>
            <span>{{ formatExpiry(share) }}</span>
            <span>{{ share.viewCount }} 次打开</span>
            <span v-if="share.createdBy">by {{ share.createdBy }}</span>
          </div>
          <code class="item-query" :title="share.query">{{ share.query }}</code>
        </div>
        <div class="item-actions">
          <el-input :model-value="shareLink(share)" readonly size="small" class="link-input" :title="shareLink(share)">
            <template #append>
              <el-button @click="copyLink(shareLink(share))">复制</el-button>
            </template>
          </el-input>
          <div class="action-row">
            <el-button text size="small" @click="openLink(share)">打开</el-button>
            <el-button text size="small" @click="openEdit(share)">编辑</el-button>
            <el-button text size="small" @click="toggleStatus(share)">{{ share.status === 1 ? '停用' : '启用' }}</el-button>
            <el-button text size="small" @click="regenerate(share)">重置链接</el-button>
            <el-button text size="small" type="danger" @click="remove(share)">删除</el-button>
          </div>
        </div>
      </article>
      <el-empty v-if="!shares.length && !loading" description="还没有公开链接，点击右上角新建" />
    </div>
  </el-dialog>
</template>

<style scoped>
.share-intro {
  margin: 0 0 14px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f2f7ff;
  color: #4a618a;
  font-size: 12px;
  line-height: 1.6;
}
.share-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.share-count {
  color: #8b9ab0;
  font-size: 12px;
}
.share-form {
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid #dbe6f6;
  border-radius: 12px;
  background: #fbfdff;
}
.form-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 10px;
}
.form-head h4 {
  margin: 0;
  color: #14294a;
  font-size: 14px;
}
.form-meta {
  color: #93a2b7;
  font-size: 12px;
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px 14px;
}
.field {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
  color: #5c7291;
  font-size: 12px;
}
.full {
  width: 100%;
}
.form-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 10px;
}
.form-tip {
  color: #93a2b7;
  font-size: 12px;
}
.form-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}
.query-preview {
  flex: 1 1 auto;
}
.query-preview :deep(input) {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
}
.share-list {
  display: flex;
  min-height: 120px;
  flex-direction: column;
  gap: 10px;
  max-height: 46vh;
  overflow: auto;
}
.share-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid #e3ebf6;
  border-radius: 12px;
  background: #fff;
}
.item-main {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 6px;
}
.item-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.item-title strong {
  color: #14294a;
  font-size: 14px;
}
.tag {
  padding: 1px 7px;
  border-radius: 999px;
  background: #f1f4f9;
  color: #6b7c93;
  font-size: 11px;
}
.tag.is-active {
  color: #15805c;
  background: #e6f7f0;
}
.tag.is-off {
  color: #98a4b6;
  background: #f1f4f9;
}
.tag.is-warn {
  color: #b57c0c;
  background: #fff6e6;
}
.tag.is-lock {
  color: #2f6cbf;
  background: #edf4ff;
}
.item-desc {
  margin: 0;
  color: #6d7f99;
  font-size: 12px;
}
.item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  color: #93a2b7;
  font-size: 11px;
}
.item-query {
  overflow: hidden;
  padding: 3px 7px;
  border-radius: 6px;
  background: #f5f8fd;
  color: #5c7291;
  font-family: Consolas, Monaco, monospace;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.item-actions {
  display: flex;
  width: 340px;
  flex: 0 0 auto;
  flex-direction: column;
  gap: 6px;
}
.link-input :deep(input) {
  font-family: Consolas, Monaco, monospace;
  font-size: 11px;
}
.action-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 2px;
}
@media (max-width: 900px) {
  .form-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .share-item {
    flex-direction: column;
  }
  .item-actions {
    width: 100%;
  }
}
</style>
