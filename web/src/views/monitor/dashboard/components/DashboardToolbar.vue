<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useDashboardDomain } from '../context'
import { FullScreen, Plus, Moon, Sunny } from '@element-plus/icons-vue'

const { isListLayout, activeDashboard, pageTitle, pageDescription, loading, isCustomGridLayout, missingK8sPodPanels, selectedDatasourceId, datasourceOptions } = useDashboardDomain('selection')
const { toggleFullscreen, isFullscreen } = useDashboardDomain('fullscreen')
const { layoutEditing, enableLayoutEditing, layoutSaving, layoutLocked, restoreDefaultLayout } = useDashboardDomain('grid')
const { exportInspectionReportPdf } = useDashboardDomain('report')
const { inspectionReport, inspectionRunning } = useDashboardDomain('inspection')
const { syncingK8sPodPanels, syncK8sPodPanels } = useDashboardDomain('k8s')
const { openCreatePanel } = useDashboardDomain('panelEditor')
const { handleDatasourceChange } = useDashboardDomain('queries')
const { timeRangeOption, handleTimeRangeChange, customTimeRange, disableFutureDate, handleCustomTimeRangeChange, timeRangeSeconds, selectTimeRange } = useDashboardDomain('time')
const { autoRefreshSeconds, restartAutoRefresh } = useDashboardDomain('autoRefresh')
const { resetDashboardFilters } = useDashboardDomain('filters')
const { copyViewLink } = useDashboardDomain('view')
const { isDark, toggleTheme } = useDashboardDomain('theme')
const heroElement = ref(null), stickyTop = ref('0px')
let heroObserver
onMounted(() => {
  heroObserver = new ResizeObserver(() => {
    if (heroElement.value) stickyTop.value = '-' + (heroElement.value.offsetHeight + parseFloat(getComputedStyle(heroElement.value).marginBottom || 0)) + 'px'
  })
  if (heroElement.value) heroObserver.observe(heroElement.value)
})
onBeforeUnmount(() => heroObserver?.disconnect())
</script>

<template>
      <section class="dashboard-control-card" :class="{ 'is-live-toolbar': !isListLayout }" :style="{ top: stickyTop }">
        <div ref="heroElement" class="dashboard-hero">
          <div>
            <div class="eyebrow">{{ isListLayout ? 'INSPECTION DASHBOARD' : 'MONITORING DASHBOARD' }}</div>
            <h2>{{ activeDashboard?.name || pageTitle }}</h2>
            <p>{{ activeDashboard?.description || pageDescription }}</p>
          </div>
          <div class="dashboard-primary-actions">
            <el-button v-if="!isListLayout && !isFullscreen" :disabled="!activeDashboard" @click="copyViewLink">复制视图链接</el-button>
            <el-button v-if="!isFullscreen" :icon="isDark ? Sunny : Moon" :aria-label="isDark ? '切换浅色主题' : '切换深色主题'" @click="toggleTheme">{{ isDark ? '浅色' : '深色' }}</el-button>
            <el-button v-if="!isListLayout" :icon="FullScreen" @click="toggleFullscreen" :disabled="!activeDashboard">{{ isFullscreen ? '退出全屏' : '大屏展示' }}</el-button>
            <el-button v-if="!isListLayout && !isFullscreen" :type="layoutEditing ? 'primary' : 'default'" @click="enableLayoutEditing" :loading="layoutSaving" :disabled="!activeDashboard || layoutLocked">{{ layoutEditing ? '完成布局' : '编辑布局' }}</el-button>
            <el-button v-if="!isListLayout && !isFullscreen && isCustomGridLayout" @click="restoreDefaultLayout" :disabled="layoutLocked">恢复默认布局</el-button>
            <el-button v-if="isListLayout" @click="exportInspectionReportPdf" :disabled="!inspectionReport || inspectionRunning">导出巡检报告 PDF</el-button>
            <el-button v-if="!isFullscreen && missingK8sPodPanels.length" type="warning" plain :loading="syncingK8sPodPanels" @click="syncK8sPodPanels">补全 Pod 监控（{{ missingK8sPodPanels.length }}）</el-button>
            <el-button v-if="!isFullscreen" type="primary" :icon="Plus" @click="openCreatePanel" :disabled="!activeDashboard">{{ isListLayout ? '新增巡检项' : '新增面板' }}</el-button>
          </div>
        </div>

        <div class="dashboard-filter-bar">
          <div class="dashboard-filter-main">
            <label><span>数据源</span><el-select :popper-class="isDark || isFullscreen ? 'monitor-dark-popper' : ''" v-model="selectedDatasourceId" :teleported="!isFullscreen" placeholder="选择数据源" @change="handleDatasourceChange"><el-option v-for="item in datasourceOptions" :key="item.id" :label="item.name" :value="item.id" /></el-select></label>
            <label><span>时间范围</span><el-select :popper-class="isDark || isFullscreen ? 'monitor-dark-popper' : ''" v-model="timeRangeOption" :teleported="!isFullscreen" @change="handleTimeRangeChange">
              <template v-if="isListLayout">
                <el-option label="最近 1 小时" :value="3600" />
                <el-option label="最近 6 小时" :value="21600" />
                <el-option label="最近 24 小时" :value="86400" />
                <el-option label="最近三天" :value="259200" />
                <el-option label="最近七天" :value="604800" />
                <el-option label="自定义时间范围" value="custom" />
              </template>
              <template v-else>
                <el-option label="最近 15 分钟" :value="900" />
                <el-option label="最近 1 小时" :value="3600" />
                <el-option label="最近 6 小时" :value="21600" />
                <el-option label="最近 24 小时" :value="86400" />
              </template>
            </el-select></label>
            <label v-if="isListLayout && timeRangeOption === 'custom'" class="custom-time-range"><span>自定义时间</span><el-date-picker v-model="customTimeRange" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" format="YYYY-MM-DD HH:mm" :disabled-date="disableFutureDate" :teleported="true" :clearable="false" @change="handleCustomTimeRangeChange" /></label>
            <label v-if="!isListLayout"><span>自动刷新</span><el-select :popper-class="isDark || isFullscreen ? 'monitor-dark-popper' : ''" v-model="autoRefreshSeconds" :teleported="!isFullscreen" @change="restartAutoRefresh"><el-option label="关闭刷新" :value="0" /><el-option label="10 秒刷新" :value="10" /><el-option label="30 秒刷新" :value="30" /><el-option label="60 秒刷新" :value="60" /></el-select></label>
          </div>
          <div v-if="!isListLayout" class="dashboard-range-presets"><el-button v-for="item in [{ label: '15分钟', value: 900 }, { label: '1小时', value: 3600 }, { label: '6小时', value: 21600 }, { label: '24小时', value: 86400 }]" :key="item.value" :type="timeRangeSeconds === item.value ? 'primary' : 'default'" @click="selectTimeRange(item.value)">{{ item.label }}</el-button><el-button link @click="resetDashboardFilters">重置筛选</el-button></div>
        </div>
      </section>
</template>

<style scoped>
.dashboard-main:is(:fullscreen, .is-presentation) .dashboard-hero {
  background: rgba(15, 23, 42, 0.92);
  border-color: rgba(96, 165, 250, 0.28);
  color: #dbeafe;
}
.dashboard-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 24px;
  margin-bottom: 16px;
  border: 1px solid var(--dash-border);
  border-radius: 16px;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.96) 0%, rgba(248, 251, 255, 0.92) 58%, rgba(230, 241, 255, 0.92) 100%),
    radial-gradient(circle at right top, rgba(37, 99, 235, 0.18), transparent 36%);
}
.eyebrow {
  margin-bottom: 8px;
  color: var(--dash-accent);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.dashboard-hero h2 {
  margin: 0 0 8px;
  color: var(--dash-text);
  font-size: 30px;
}
.dashboard-hero p {
  margin: 0;
  color: var(--dash-muted);
}
.observability-canvas .dashboard-hero {
  align-items: flex-start;
  padding: 16px 18px;
  margin-bottom: 10px;
  border-color: var(--dash-border);
  border-radius: 6px;
  background: var(--dash-surface);
  box-shadow: none;
}
.observability-canvas .eyebrow {
  margin-bottom: 4px;
  color: var(--dash-accent);
  font-size: 10px;
  letter-spacing: 0.06em;
}
.observability-canvas .dashboard-hero h2 {
  margin-bottom: 4px;
  font-size: 22px;
  letter-spacing: 0;
}
.observability-canvas .dashboard-hero p {
  font-size: 13px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero {
  align-items: center;
  min-height: 94px;
  padding: 12px 16px;
  border-color: #283442;
  background: #111923;
}
.observability-canvas:is(:fullscreen, .is-presentation) .eyebrow {
  color: #60a5fa;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero h2 {
  color: #f1f5f9;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero p {
  color: #8291a5;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero {
  min-height: 62px;
  margin-bottom: 6px;
  padding: 8px 12px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero p,
.observability-canvas:is(:fullscreen, .is-presentation) .eyebrow {
  display: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-hero h2 {
  margin: 0;
  font-size: 18px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-control-card {
  overflow: visible;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-bar {
  position: relative;
  z-index: 10;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__popper) {
  z-index: 30;
}
.dashboard-control-card {
  overflow: hidden;
  margin-bottom: 10px;
  border: 1px solid var(--dash-border);
  border-radius: 9px;
  background: var(--dash-surface);
}
.observability-canvas .dashboard-control-card .dashboard-hero {
  min-height: 68px;
  margin: 0;
  padding: 13px 16px;
  border: 0;
  border-radius: 0;
}
.observability-canvas .dashboard-control-card .dashboard-hero h2 {
  margin: 2px 0 1px;
  font-size: 19px;
}
.observability-canvas .dashboard-control-card .dashboard-hero p {
  color: var(--dash-muted);
  font-size: 12px;
}
.dashboard-primary-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.dashboard-primary-actions .el-button + .el-button {
  margin-left: 0;
}
.dashboard-filter-bar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
  padding: 12px 16px;
  border-top: 1px solid var(--dash-border);
  background: var(--dash-surface);
}
.dashboard-filter-main,
.dashboard-range-presets {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.dashboard-filter-main label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  color: var(--dash-muted);
  font-size: 11px;
}
.dashboard-filter-main label:first-child .el-select {
  width: 190px;
}
.dashboard-filter-main .el-select {
  width: 128px;
}
.dashboard-filter-main .custom-time-range :deep(.el-date-editor) {
  width: 360px;
}
.dashboard-range-presets .el-button {
  min-width: 62px;
  margin-left: 0;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-control-card {
  border-color: #283442;
  background: #111923;
  color: #d7e0ea;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-bar {
  border-top-color: #283442;
  background: #151e29;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main label {
  color: #9badc2;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__wrapper) {
  border-color: #3b4c60;
  background: #0c131c;
  box-shadow: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__selected-item),
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__placeholder),
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__caret) {
  color: #d7e0ea;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-range-presets :deep(.el-button:not(.el-button--primary)) {
  border-color: #3b4c60;
  background: #0c131c;
  color: #b9c8d8;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-range-presets :deep(.el-button.is-link) {
  border-color: transparent;
  background: transparent;
  color: #9badc2;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-control-card {
  border-color: #283442;
  background: #111923;
  color: #d7e0ea;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-bar {
  position: relative;
  z-index: 10;
  border-top-color: #283442;
  background: #151e29;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-control-card {
  overflow: visible;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__popper) {
  z-index: 30;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main label {
  color: #9badc2;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__wrapper) {
  border-color: #3b4c60;
  background: #0c131c;
  box-shadow: none;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__selected-item),
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__placeholder),
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-filter-main :deep(.el-select__caret) {
  color: #d7e0ea;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-range-presets :deep(.el-button:not(.el-button--primary)) {
  border-color: #3b4c60;
  background: #0c131c;
  color: #b9c8d8;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-range-presets :deep(.el-button:not(.el-button--primary):hover) {
  border-color: #5b7cff;
  color: #8bb5ff;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-range-presets :deep(.el-button.is-link) {
  border-color: transparent;
  background: transparent;
  color: #9badc2;
}
@media (max-width: 1180px) {
  .dashboard-filter-bar { align-items: flex-start; flex-direction: column; }
}
@media (max-width: 760px) {
  .observability-canvas .dashboard-control-card .dashboard-hero { align-items: flex-start; flex-direction: column; }
}
@media (max-width: 760px) {
  .dashboard-filter-main,
.dashboard-range-presets { align-items: stretch; flex-wrap: wrap; }
}
@media (max-width: 760px) {
  .dashboard-filter-main label:first-child .el-select,
.dashboard-filter-main .el-select { width: 100%; }
}
@media (max-width: 760px) {
  .dashboard-filter-main label { min-width: calc(50% - 8px); }
}
</style>

<style scoped>
.is-live-toolbar { position: sticky; top: -87px; z-index: 15; background: var(--dash-canvas); border-radius: 10px; }
.is-live-toolbar .dashboard-filter-bar { background: var(--dash-surface); box-shadow: 0 3px 12px rgb(0 0 0 / 5%); }

</style>
