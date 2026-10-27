<script setup>
import { ref, watch } from 'vue'
import { FullScreen, Refresh, Grid, List } from '@element-plus/icons-vue'
import { useDashboardDomain } from '../../context'
import { formatReportTime } from '../../utils/format.js'
import InspectionBoard from './InspectionBoard.vue'
import InspectionResults from './InspectionResults.vue'
import InspectionResultDetail from './InspectionResultDetail.vue'

const { inspectionReport, inspectionRunning, loadLatestInspectionReport } = useDashboardDomain('inspection')
const { activeDashboard, activeDashboardId } = useDashboardDomain('selection')
const { isFullscreen, toggleFullscreen } = useDashboardDomain('fullscreen')
const mode = ref('board')
const selectedRow = ref(null)
const reading = ref(false)
const refreshError = ref(false)
let refreshSequence = 0
watch(activeDashboardId, () => {
  refreshSequence++
  mode.value = 'board'
  selectedRow.value = null
  refreshError.value = false
  reading.value = false
})
watch(inspectionReport, () => { selectedRow.value = null })
watch(isFullscreen, value => { selectedRow.value = null; if (value) mode.value = 'board' })
async function refreshReport() {
  if (reading.value || inspectionRunning.value) return
  const sequence = ++refreshSequence
  const dashboardId = activeDashboardId.value
  reading.value = true
  refreshError.value = false
  try { await loadLatestInspectionReport(dashboardId, { preserveCurrent: true }) }
  catch { if (sequence === refreshSequence) refreshError.value = true }
  finally { if (sequence === refreshSequence) reading.value = false }
}
</script>

<template>
  <section class="inspection-result-area" :class="{ 'is-screen': isFullscreen, 'has-report': !!inspectionReport }" aria-label="巡检结果视图">
    <header v-if="inspectionReport || isFullscreen" class="result-area-heading">
      <div class="result-area-title">
        <span v-if="isFullscreen" class="screen-kicker">巡检报告大屏</span>
        <h3>{{ isFullscreen ? activeDashboard?.name : '巡检结果' }}<span v-if="inspectionReport">#{{ inspectionReport.id }}</span></h3>
        <p>{{ inspectionReport ? `${formatReportTime(inspectionReport.completedAt)} 完成 · 已保存报告` : '正在读取最近一次已保存的报告' }}</p>
      </div>
      <div class="result-area-actions">
        <div v-if="!isFullscreen && inspectionReport" class="view-toggle" aria-label="巡检结果展示方式">
          <button :aria-pressed="mode === 'board'" :class="{ active: mode === 'board' }" @click="mode = 'board'"><el-icon><Grid /></el-icon>大屏视图</button>
          <button :aria-pressed="mode === 'list'" :class="{ active: mode === 'list' }" @click="mode = 'list'"><el-icon><List /></el-icon>明细列表</button>
        </div>
        <el-button v-if="isFullscreen" :icon="Refresh" :loading="reading" :disabled="inspectionRunning" @click="refreshReport">刷新报告</el-button>
        <el-button v-if="isFullscreen || mode === 'board'" :icon="FullScreen" :disabled="!inspectionReport && !isFullscreen" @click="toggleFullscreen">{{ isFullscreen ? '退出全屏' : '全屏展示' }}</el-button>
      </div>
    </header>
    <el-alert v-if="refreshError" class="report-refresh-error" :title="inspectionReport ? '刷新失败，当前仍显示上次保存的报告。请点击刷新报告重试。' : '读取最新报告失败，请点击刷新报告重试。'" type="error" :closable="false" show-icon />
    <InspectionBoard v-if="inspectionReport && mode === 'board'" @inspect="selectedRow = $event" />
    <InspectionResults v-else-if="!isFullscreen" :embedded="!!inspectionReport" />
    <div v-else class="screen-empty" v-loading="reading"><p>{{ refreshError ? '报告读取失败' : '暂无已保存的巡检报告' }}</p><span>刷新只读取报告，不会重新执行巡检。可退出全屏后执行一次巡检。</span></div>
    <InspectionResultDetail :row="selectedRow" @close="selectedRow = null" />
  </section>
</template>

<style scoped>
.inspection-result-area { min-width: 0; }
.inspection-result-area.has-report, .inspection-result-area.is-screen { border: 1px solid var(--dash-border); border-radius: 14px; background: var(--dash-surface); box-shadow: var(--inspect-shadow); overflow: hidden; }
.result-area-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; padding: 22px 24px; }
.result-area-title { min-width: 0; }
h3 { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 0; color: var(--dash-text); font-size: 17px; font-weight: 650; line-height: 1.5; }
h3 > span { padding: 1px 6px; border: 1px solid var(--dash-border); border-radius: 4px; color: var(--dash-muted); font: 11px/1.5 Consolas, monospace; }
p { margin: 5px 0 0; color: var(--dash-muted); font-size: 11px; line-height: 1.7; }
.result-area-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.result-area-actions .el-button { margin-left: 0; }
.view-toggle { display: flex; gap: 3px; padding: 3px; border: 1px solid var(--dash-border); border-radius: 8px; background: var(--dash-subtle); }
.view-toggle button { display: flex; align-items: center; gap: 5px; padding: 6px 10px; border: 0; border-radius: 5px; background: transparent; color: var(--dash-muted); font-size: 12px; cursor: pointer; }
.view-toggle button.active { background: var(--dash-surface); color: var(--dash-accent); box-shadow: var(--inspect-shadow); }
.screen-kicker { display: block; margin-bottom: 4px; color: var(--dash-accent); font-size: 10px; letter-spacing: .08em; }
.is-screen { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.is-screen .result-area-heading { flex-shrink: 0; padding: 12px 24px; }
.is-screen h3 { font-size: 21px; }
.is-screen :deep(.inspection-board) { flex: 1; min-height: 0; }
.is-screen :deep(.matrix-scroll) { flex: 1; max-height: none; }
.is-screen :deep(.screen-totals) { margin-bottom: 12px; }
.is-screen :deep(.screen-totals button) { padding-top: 10px; padding-bottom: 10px; }
.is-screen :deep(.attention-section) { margin-bottom: 12px; }
.is-screen :deep(.attention-item) { padding-top: 8px; padding-bottom: 8px; }
.is-screen :deep(.results-filters) { padding-bottom: 10px; }
.is-screen :deep(.inspection-matrix tbody th) { padding-top: 4px; padding-bottom: 4px; }
.is-screen :deep(.inspection-matrix th small) { margin-top: 2px; }
.is-screen :deep(.inspection-matrix td) { padding-top: 2px; padding-bottom: 2px; }
.is-screen :deep(.matrix-cell) { min-height: 32px; padding-top: 5px; padding-bottom: 5px; }
.is-screen :deep(.matrix-cell > strong) { font-size: 18px; }
.screen-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; padding: 30px; color: var(--dash-muted); font-size: 12px; }
.screen-empty p { font-size: 20px; color: var(--dash-text); }
.report-refresh-error { width: auto; margin: 0 24px 16px; flex-shrink: 0; }
@media (max-width: 760px) { .result-area-heading { padding: 18px 16px; gap: 12px; } .result-area-actions { gap: 7px; } .view-toggle button { padding: 6px 8px; } .is-screen .result-area-heading { padding: 14px 16px; } .is-screen h3 { font-size: 17px; } }
</style>
