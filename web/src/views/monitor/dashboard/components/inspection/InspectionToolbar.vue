<script setup>
import { useDashboardDomain } from '../../context'
import { Plus, Moon, Sunny, Download, VideoPlay, Connection, Clock } from '@element-plus/icons-vue'

const { activeDashboard, activePanels, datasourceOptions } = useDashboardDomain('selection')
const { inspectionReport, inspectionRunning, selectedInspectionDatasourceIds, executeInspection } = useDashboardDomain('inspection')
const { exportInspectionReportPdf } = useDashboardDomain('report')
const { openCreatePanel } = useDashboardDomain('panelEditor')
const { timeRangeOption, handleTimeRangeChange, customTimeRange, disableFutureDate, handleCustomTimeRangeChange } = useDashboardDomain('time')
const { isDark, toggleTheme } = useDashboardDomain('theme')
const ranges = [{ label: '最近 1 小时', value: 3600 }, { label: '最近 6 小时', value: 21600 }, { label: '最近 24 小时', value: 86400 }, { label: '最近三天', value: 259200 }, { label: '最近七天', value: 604800 }, { label: '自定义时间范围', value: 'custom' }]
</script>

<template>
  <section class="inspection-toolbar" aria-label="巡检执行配置">
    <header class="toolbar-heading">
      <div class="heading-copy">
        <span class="section-kicker">巡检工作台 <i></i> 多数据源分析</span>
        <h2>{{ activeDashboard?.name || '巡检大屏' }}</h2>
        <p>{{ activeDashboard?.description || '统一检查多个数据源，快速定位需要关注的指标。' }}</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Download" @click="exportInspectionReportPdf" :disabled="!inspectionReport || inspectionRunning">导出报告 PDF</el-button>
        <el-button :icon="Plus" @click="openCreatePanel" :disabled="!activeDashboard">新增巡检项</el-button>
        <el-button :icon="isDark ? Sunny : Moon" :aria-label="isDark ? '切换浅色主题' : '切换深色主题'" @click="toggleTheme" />
      </div>
    </header>
    <div class="execution-config">
      <div class="config-fields">
        <label class="source-field"><span><el-icon><Connection /></el-icon>巡检数据源 <small>支持多选</small></span>
          <el-select v-model="selectedInspectionDatasourceIds" multiple filterable collapse-tags collapse-tags-tooltip :max-collapse-tags="2" :disabled="inspectionRunning" :popper-class="isDark ? 'monitor-dark-popper' : ''" placeholder="选择一个或多个数据源" aria-label="巡检数据源"><el-option v-for="item in datasourceOptions" :key="item.id" :label="item.name" :value="item.id" /></el-select>
        </label>
        <label class="range-field"><span><el-icon><Clock /></el-icon>时间范围</span>
          <el-select v-model="timeRangeOption" :disabled="inspectionRunning" aria-label="巡检时间范围" :popper-class="isDark ? 'monitor-dark-popper' : ''" @change="handleTimeRangeChange"><el-option v-for="range in ranges" :key="range.value" :label="range.label" :value="range.value" /></el-select>
        </label>
        <label v-if="timeRangeOption === 'custom'" class="custom-range"><span>自定义时间</span><el-date-picker v-model="customTimeRange" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" format="YYYY-MM-DD HH:mm" :disabled="inspectionRunning" :disabled-date="disableFutureDate" :popper-class="isDark ? 'monitor-dark-popper' : ''" :clearable="false" @change="handleCustomTimeRangeChange" /></label>
      </div>
      <div class="execute-action">
        <el-button type="primary" size="large" :icon="VideoPlay" :loading="inspectionRunning" :disabled="!activePanels.length || !selectedInspectionDatasourceIds.length" @click="executeInspection">{{ inspectionRunning ? '巡检执行中' : '执行巡检' }}</el-button>
        <span>{{ selectedInspectionDatasourceIds.length }} 个数据源 × {{ activePanels.length }} 项启用规则</span>
      </div>
    </div>
    <div class="config-footnote"><i></i>{{ inspectionRunning ? '正在生成新报告，完成后将自动更新下方结果。' : '调整配置后执行巡检，生成独立报告；下方展示最近一次已保存的结果。' }}</div>
  </section>
</template>

<style scoped>
.inspection-toolbar { margin-bottom: 16px; padding: 24px 26px 14px; border: 1px solid var(--dash-border); border-radius: 14px; background: var(--dash-surface); box-shadow: var(--inspect-shadow); }
.toolbar-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
.heading-copy { min-width: 0; }
.section-kicker { display: flex; align-items: center; gap: 8px; color: var(--dash-muted); font-size: 12px; }
.section-kicker i { width: 3px; height: 3px; border-radius: 50%; background: var(--dash-muted); }
h2 { margin: 9px 0 6px; color: var(--dash-text); font-size: 25px; font-weight: 650; line-height: 1.35; letter-spacing: -.5px; overflow-wrap: anywhere; }
p { margin: 0; color: var(--dash-muted); font-size: 13px; line-height: 1.7; }
.toolbar-actions { display: flex; gap: 8px; flex-shrink: 0; }
.toolbar-actions .el-button + .el-button { margin-left: 0; }
.execution-config { display: flex; align-items: flex-end; gap: 24px; margin-top: 24px; padding: 20px 0 16px; border-top: 1px solid var(--dash-border); }
.config-fields { display: flex; align-items: flex-end; flex: 1; min-width: 0; flex-wrap: wrap; gap: 16px; }
label { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
label > span { display: flex; align-items: center; gap: 6px; color: var(--dash-text); font-size: 12px; }
label .el-icon { color: var(--dash-muted); font-size: 14px; }
label small { margin-left: 2px; color: var(--dash-muted); font-size: 11px; font-weight: 400; }
.source-field { flex: 1 1 320px; max-width: 560px; }
.range-field { flex: 0 1 170px; }
.custom-range { flex: 1 1 360px; }
.custom-range :deep(.el-date-editor) { width: 100%; box-sizing: border-box; }
.execute-action { display: flex; align-items: flex-end; flex-direction: column; flex-shrink: 0; gap: 7px; }
.execute-action .el-button { min-width: 144px; height: 40px; }
.execute-action > span { color: var(--dash-muted); font-size: 11px; }
.config-footnote { display: flex; align-items: center; gap: 7px; padding-top: 12px; border-top: 1px solid var(--dash-border); color: var(--dash-muted); font-size: 11px; line-height: 1.6; }
.config-footnote i { width: 5px; height: 5px; flex-shrink: 0; border-radius: 50%; background: var(--dash-accent); }
@media (max-width: 1000px) { .toolbar-heading { flex-direction: column; gap: 14px; } .toolbar-actions { flex-wrap: wrap; } }
@media (max-width: 760px) {
  .inspection-toolbar { padding: 18px 16px 12px; }
  h2 { font-size: 21px; }
  .execution-config { flex-direction: column; align-items: stretch; gap: 16px; margin-top: 18px; padding-top: 16px; }
  .config-fields { width: 100%; }
  .source-field, .range-field, .custom-range { flex-basis: 100%; max-width: none; }
  .execute-action { align-items: stretch; text-align: center; }
  .toolbar-actions { gap: 6px; }
  .toolbar-actions .el-button { padding: 8px 10px; font-size: 12px; }
}
</style>
