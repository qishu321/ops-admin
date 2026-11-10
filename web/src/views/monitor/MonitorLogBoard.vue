<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Refresh, Search, Share } from '@element-plus/icons-vue'
import { analyzeMonitorLogBoard, queryMonitorDatasourceList } from '../../api/monitor'
import LogBoardPanels from './log-board/LogBoardPanels.vue'
import LogBoardShareDialog from './log-board/LogBoardShareDialog.vue'
import { LOG_BOARD_DEFAULT_QUERY, LOG_BOARD_TIME_RANGES, logBoardRangeLabel } from './log-board/logBoardShared'

const REFRESH_INTERVAL = 30000

const datasources = ref([])
const datasourceId = ref(undefined)
const query = ref(LOG_BOARD_DEFAULT_QUERY)
const rangeSeconds = ref(24 * 3600)
const autoRefresh = ref(true)
const loading = ref(false)
const errorMessage = ref('')
const data = ref(null)
const updatedAt = ref(0)
const shareVisible = ref(false)
let refreshTimer

const rangeLabel = computed(() => logBoardRangeLabel(rangeSeconds.value))
const emptyHint = computed(() => (data.value ? '当前过滤条件下没有日志' : '选择数据源与过滤条件后开始分析'))
const updatedLabel = computed(() => {
  if (!updatedAt.value) return ''
  const date = new Date(updatedAt.value)
  const pad = value => String(value).padStart(2, '0')
  return `更新于 ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
})
const shareConfig = computed(() => ({
  datasourceId: datasourceId.value,
  datasourceName: datasources.value.find(item => item.id === datasourceId.value)?.name || '',
  query: query.value.trim() || '*',
  rangeSeconds: rangeSeconds.value
}))

async function loadDatasources() {
  try {
    const response = await queryMonitorDatasourceList({ pageNum: 1, pageSize: 100 })
    const rows = response?.list || response?.rows || response || []
    datasources.value = rows.filter(item => item.type === 'victorialogs')
    const saved = Number(localStorage.getItem('ops-admin-logboard-datasource') || 0)
    const preferred = datasources.value.find(item => item.id === saved) || datasources.value[0]
    datasourceId.value = preferred?.id
  } catch (error) {
    errorMessage.value = error?.message || '加载数据源失败'
  }
}

async function analyze() {
  if (!datasourceId.value) {
    errorMessage.value = '暂无 VictoriaLogs 数据源，请先在数据源管理中接入'
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const endAt = Date.now()
    const response = await analyzeMonitorLogBoard({
      datasourceId: datasourceId.value,
      query: query.value.trim() || '*',
      startAt: endAt - rangeSeconds.value * 1000,
      endAt,
      limit: 5000
    })
    data.value = response
    updatedAt.value = Date.now()
    localStorage.setItem('ops-admin-logboard-datasource', String(datasourceId.value))
    localStorage.setItem('ops-admin-logboard-query', query.value)
  } catch (error) {
    errorMessage.value = error?.message || '日志分析失败'
  } finally {
    loading.value = false
  }
}

function changeRange(seconds) {
  if (rangeSeconds.value === seconds) return
  rangeSeconds.value = seconds
  analyze()
}

// 面板里的维度（异常类型 / 平台 / 版本…）是从日志正文里解析出来的，不是
// VictoriaLogs 的存储字段，所以下钻只能用全文短语过滤，写 key:value 会命中 0 条。
function handleDrill({ key, value }) {
  const text = String(value || '').replace(/"/g, '').trim()
  if (!text) return
  const snippet = key ? `"${key}=${text}"` : `"${text}"`
  query.value = query.value.trim() ? `${query.value.trim()} AND ${snippet}` : snippet
  analyze()
}

function resetFilters() {
  query.value = LOG_BOARD_DEFAULT_QUERY
  analyze()
}

function toggleAutoRefresh(value) {
  autoRefresh.value = value
  schedule()
}

function schedule() {
  window.clearInterval(refreshTimer)
  if (!autoRefresh.value) return
  refreshTimer = window.setInterval(() => {
    if (!loading.value && !shareVisible.value) analyze()
  }, REFRESH_INTERVAL)
}

watch(autoRefresh, () => schedule())

onMounted(async () => {
  const savedQuery = localStorage.getItem('ops-admin-logboard-query')
  const savedRange = Number(localStorage.getItem('ops-admin-logboard-range') || 0)
  if (savedRange && LOG_BOARD_TIME_RANGES.some(item => item.seconds === savedRange)) rangeSeconds.value = savedRange
  await loadDatasources()
  if (savedQuery) query.value = savedQuery
  await analyze()
  schedule()
})

watch(rangeSeconds, value => localStorage.setItem('ops-admin-logboard-range', String(value)))

onBeforeUnmount(() => window.clearInterval(refreshTimer))
</script>

<template>
  <div class="log-board">
    <section class="board-toolbar">
      <div class="toolbar-main">
        <el-select v-model="datasourceId" class="ds-select" placeholder="VictoriaLogs 数据源" @change="analyze">
          <el-option v-for="item in datasources" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
        <el-input v-model="query" class="query-input" clearable placeholder='LogsQL 过滤条件，例如 kubernetes.container_name:"notice"' @keyup.enter="analyze">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button type="primary" :loading="loading" @click="analyze">分析</el-button>
        <el-button :icon="Share" @click="shareVisible = true">公开分享</el-button>
      </div>
      <div class="toolbar-sub">
        <div class="range-group">
          <button v-for="item in LOG_BOARD_TIME_RANGES" :key="item.seconds" type="button" :class="['range-chip', { 'is-active': rangeSeconds === item.seconds }]" @click="changeRange(item.seconds)">{{ item.label }}</button>
        </div>
        <div class="toolbar-right">
          <button v-if="query.trim() !== LOG_BOARD_DEFAULT_QUERY" type="button" class="filter-chip" @click="resetFilters">恢复默认过滤</button>
          <el-switch :model-value="autoRefresh" inline-prompt active-text="自动" inactive-text="手动" @update:model-value="toggleAutoRefresh" />
          <span class="updated-at">{{ updatedLabel }}</span>
          <el-button text :icon="Refresh" :loading="loading" @click="analyze" />
        </div>
      </div>
    </section>

    <el-alert v-if="errorMessage" :title="errorMessage" type="error" show-icon :closable="false" class="board-alert" />

    <LogBoardPanels
      :data="data"
      :loading="loading"
      :range-label="rangeLabel"
      :empty-hint="emptyHint"
      @drill="handleDrill"
    />

    <LogBoardShareDialog v-model="shareVisible" :config="shareConfig" />
  </div>
</template>

<style scoped>
.log-board {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 100%;
  padding: 18px;
  background: #f4f7fc;
  color: #23364f;
}
.board-toolbar {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid #e0e8f4;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 8px 22px rgba(49, 78, 125, 0.06);
}
.toolbar-main {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ds-select {
  width: 190px;
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
.updated-at {
  color: #8b9ab0;
  font-size: 12px;
}
.board-alert {
  border-radius: 10px;
}
</style>
