<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { FullScreen, Refresh, Search } from '@element-plus/icons-vue'
import { queryPublicLogBoardData, queryPublicLogBoardMeta } from '../../api/logBoardShare'
import LogBoardPanels from './log-board/LogBoardPanels.vue'
import { LOG_BOARD_TIME_RANGES, logBoardRangeLabel } from './log-board/logBoardShared'

const route = useRoute()
const token = String(route.params.token || '')
const STORAGE_KEY = `ops-admin-public-logboard:${token}`

const state = ref('loading') // loading | ready | error
const meta = ref(null)
const data = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const updatedAt = ref(0)
const nowAt = ref(Date.now())
const autoRefresh = ref(true)
const form = reactive({ datasourceId: 0, query: '', rangeSeconds: 86400 })
let refreshTimer
let clockTimer

const datasources = computed(() => meta.value?.datasources || [])
const rangeLabel = computed(() => logBoardRangeLabel(form.rangeSeconds))
const refreshSeconds = computed(() => Math.max(15, Number(meta.value?.refreshSeconds || 60)))
const updatedLabel = computed(() => {
  if (!updatedAt.value) return ''
  const date = new Date(updatedAt.value)
  const pad = value => String(value).padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
})
const clockLabel = computed(() => {
  const date = new Date(nowAt.value)
  const pad = value => String(value).padStart(2, '0')
  const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())} ${weekdays[date.getDay()]}`
})
const isCustomQuery = computed(() => Boolean(meta.value) && form.query.trim() !== String(meta.value.query || '').trim())
const emptyHint = computed(() => (data.value ? '当前过滤条件下没有日志' : '正在按当前条件查询日志'))

function restoreSelection(payload) {
  let saved = null
  try {
    saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null')
  } catch {
    saved = null
  }
  const source = saved && typeof saved === 'object' ? saved : {}
  const known = (payload.datasources || []).map(item => Number(item.id))
  const preferred = Number(source.datasourceId || payload.datasourceId || 0)
  form.datasourceId = known.includes(preferred) ? preferred : known[0] || 0
  form.query = typeof source.query === 'string' && source.query.trim() ? source.query : String(payload.query || '')
  const range = Number(source.rangeSeconds || payload.rangeSeconds || 86400)
  form.rangeSeconds = range >= 300 && range <= 7 * 24 * 3600 ? range : Number(payload.rangeSeconds || 86400)
}

function persistSelection() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...form }))
  } catch {
    // 隐私模式下写不了 localStorage，不影响使用。
  }
}

async function loadMeta() {
  try {
    const payload = await queryPublicLogBoardMeta(token)
    meta.value = payload
    document.title = `${payload.name} - 日志大屏`
    restoreSelection(payload)
    await analyze()
  } catch (error) {
    state.value = 'error'
    errorMessage.value = error?.message || '公开大屏不可用'
  }
}

async function analyze(options = {}) {
  if (!meta.value) return
  if (!form.datasourceId) {
    errorMessage.value = '暂无可用的 VictoriaLogs 数据源'
    state.value = 'ready'
    return
  }
  loading.value = true
  try {
    const payload = await queryPublicLogBoardData(token, {
      datasourceId: form.datasourceId,
      query: form.query.trim() || '*',
      rangeSeconds: form.rangeSeconds
    })
    data.value = payload.analysis
    if (payload.meta) meta.value = { ...meta.value, ...payload.meta }
    updatedAt.value = Number(payload.refreshedAt || Date.now())
    errorMessage.value = ''
    state.value = 'ready'
    persistSelection()
  } catch (error) {
    const code = error?.code
    if (code === 404 || code === 410) {
      state.value = 'error'
      errorMessage.value = error?.message || '公开大屏不可用'
      return
    }
    errorMessage.value = error?.message || '日志数据加载失败'
    if (!data.value) state.value = 'error'
  } finally {
    loading.value = false
  }
}

function changeRange(seconds) {
  if (form.rangeSeconds === seconds) return
  form.rangeSeconds = seconds
  analyze()
}

function resetQuery() {
  if (!meta.value) return
  form.query = String(meta.value.query || '')
  form.rangeSeconds = Number(meta.value.rangeSeconds || 86400)
  analyze()
}

function schedule() {
  window.clearInterval(refreshTimer)
  if (!autoRefresh.value || state.value !== 'ready') return
  refreshTimer = window.setInterval(() => {
    if (!loading.value && state.value === 'ready') analyze()
  }, refreshSeconds.value * 1000)
}

function toggleAutoRefresh(value) {
  autoRefresh.value = value
  schedule()
}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen?.()
  } else {
    document.documentElement.requestFullscreen?.()
  }
}

onMounted(() => {
  clockTimer = window.setInterval(() => { nowAt.value = Date.now() }, 1000)
  loadMeta()
})

onBeforeUnmount(() => {
  window.clearInterval(refreshTimer)
  window.clearInterval(clockTimer)
})

// 首次查询成功或刷新频率变化后重新挂载定时器。
watch([state, refreshSeconds], () => schedule())
</script>

<template>
  <main class="public-board">
    <div v-if="state === 'loading'" class="board-state">
      <span class="loader" />
      <p>正在加载日志大屏…</p>
    </div>

    <div v-else-if="state === 'error'" class="board-state">
      <div class="state-icon">!</div>
      <h1>公开大屏不可用</h1>
      <p>{{ errorMessage }}</p>
      <small>请联系运维同学确认链接是否已停用或过期。</small>
    </div>

    <template v-else>
      <header class="board-hero">
        <div class="hero-left">
          <div class="hero-brand"><span>OA</span>Ops Admin · 日志大屏</div>
          <h1>{{ meta?.name }}</h1>
          <p>{{ meta?.description || '实时日志聚合视图' }}</p>
        </div>
        <div class="hero-right">
          <div class="hero-clock">{{ clockLabel }}</div>
          <div class="hero-meta">
            <span>{{ rangeLabel }}</span>
            <span>{{ refreshSeconds }} 秒刷新</span>
            <span>{{ datasources.length }} 个数据源可选</span>
            <span v-if="updatedLabel">更新于 {{ updatedLabel }}</span>
          </div>
          <div class="hero-actions">
            <el-button text :icon="Refresh" :loading="loading" @click="analyze()">刷新</el-button>
            <el-switch :model-value="autoRefresh" inline-prompt active-text="自动" inactive-text="暂停" @update:model-value="toggleAutoRefresh" />
            <el-button text :icon="FullScreen" @click="toggleFullscreen">全屏</el-button>
          </div>
        </div>
      </header>

      <section class="public-toolbar">
        <div class="toolbar-main">
          <el-select v-model="form.datasourceId" class="ds-select" placeholder="VictoriaLogs 数据源" @change="analyze()">
            <el-option v-for="item in datasources" :key="item.id" :label="item.name" :value="item.id" />
          </el-select>
          <el-input
            v-model="form.query"
            class="query-input"
            clearable
            :maxlength="meta?.maxQueryLength || 2000"
            placeholder='LogsQL 过滤条件，例如 kubernetes.container_name:"notice"'
            @keyup.enter="analyze()"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-button type="primary" :loading="loading" @click="analyze()">分析</el-button>
        </div>
        <div class="toolbar-sub">
          <div class="range-group">
            <button
              v-for="item in LOG_BOARD_TIME_RANGES"
              :key="item.seconds"
              type="button"
              :class="['range-chip', { 'is-active': form.rangeSeconds === item.seconds }]"
              :disabled="item.seconds > (meta?.maxRangeSeconds || 604800)"
              @click="changeRange(item.seconds)"
            >{{ item.label }}</button>
          </div>
          <div class="toolbar-right">
            <button v-if="isCustomQuery" type="button" class="filter-chip" @click="resetQuery">恢复默认条件</button>
            <span class="toolbar-hint">数据源、过滤条件和时间范围都可以自行调整</span>
          </div>
        </div>
      </section>

      <section class="board-body">
        <el-alert v-if="errorMessage" :title="errorMessage" type="error" show-icon :closable="false" class="board-alert" />
        <LogBoardPanels
          :data="data"
          :loading="loading"
          :range-label="rangeLabel"
          :interactive="false"
          :allow-detail="Boolean(meta?.allowDetail)"
          :empty-hint="emptyHint"
        />
      </section>

      <footer class="board-footer">该页面由 Ops Admin 监控中心公开分享，可在页面内切换数据源与过滤条件</footer>
    </template>
  </main>
</template>

<style scoped>
.public-board {
  min-height: 100vh;
  background: #f1f5fb;
  color: #23364f;
}
.board-state {
  display: grid;
  min-height: 100vh;
  place-content: center;
  justify-items: center;
  gap: 10px;
  color: #6b7c93;
  text-align: center;
}
.board-state h1 {
  margin: 6px 0 0;
  color: #16243c;
  font-size: 22px;
}
.board-state p {
  margin: 0;
  max-width: 520px;
  line-height: 1.6;
}
.board-state small {
  color: #93a2b7;
}
.state-icon {
  display: grid;
  place-items: center;
  width: 54px;
  height: 54px;
  border-radius: 50%;
  background: #fff0df;
  color: #c56a25;
  font-size: 26px;
  font-weight: 800;
}
.loader {
  width: 30px;
  height: 30px;
  border: 3px solid #d6e2f3;
  border-top-color: #3275d9;
  border-radius: 50%;
  animation: spin 0.9s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.board-hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  padding: 22px 26px 18px;
  border-bottom: 1px solid #dde7f4;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
}
.hero-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #5b7597;
  font-size: 12px;
  letter-spacing: 0.04em;
}
.hero-brand span {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: #3275d9;
  color: #fff;
  font-size: 11px;
  font-weight: 800;
}
.board-hero h1 {
  margin: 10px 0 4px;
  color: #12233c;
  font-size: 26px;
  line-height: 1.25;
}
.board-hero p {
  margin: 0;
  color: #718099;
  font-size: 13px;
}
.hero-right {
  display: flex;
  align-items: flex-end;
  flex-direction: column;
  gap: 8px;
  flex: 0 0 auto;
}
.hero-clock {
  color: #1d3a63;
  font-family: Consolas, Monaco, monospace;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.hero-meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
  max-width: 620px;
}
.hero-meta span {
  padding: 2px 9px;
  border: 1px solid #e2eaf6;
  border-radius: 999px;
  background: #fff;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}
.hero-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}
.public-toolbar {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 14px 18px 0;
  padding: 12px 14px;
  border: 1px solid #e0e8f4;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 6px 18px rgba(49, 78, 125, 0.05);
}
.toolbar-main {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ds-select {
  width: 200px;
  flex: 0 0 auto;
}
.query-input {
  flex: 1 1 auto;
}
.query-input :deep(input) {
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
}
.toolbar-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.range-group {
  display: flex;
  padding: 3px;
  border: 1px solid #e2e9f4;
  border-radius: 9px;
  background: #f7f9fd;
}
.range-chip {
  padding: 5px 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: #61728d;
  font-size: 12px;
  cursor: pointer;
}
.range-chip.is-active {
  color: #1f5fd0;
  background: #fff;
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(46, 86, 158, 0.16);
}
.range-chip:disabled {
  color: #b6c1d1;
  cursor: not-allowed;
}
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.filter-chip {
  padding: 4px 10px;
  border: 1px solid #cfe0fb;
  border-radius: 999px;
  background: #eef4ff;
  color: #2765d4;
  font-size: 12px;
  cursor: pointer;
}
.toolbar-hint {
  color: #93a2b7;
  font-size: 12px;
}
.board-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 18px 8px;
}
.board-alert {
  border-radius: 10px;
}
.board-footer {
  padding: 14px 18px 22px;
  color: #9aa8bd;
  font-size: 11px;
  text-align: center;
}
@media (max-width: 900px) {
  .board-hero {
    align-items: flex-start;
    flex-direction: column;
  }
  .hero-right {
    align-items: flex-start;
  }
  .hero-meta {
    justify-content: flex-start;
  }
  .toolbar-main {
    flex-wrap: wrap;
  }
  .ds-select {
    width: 100%;
  }
}
</style>
