<script setup>
import { useDashboardDomain } from '../context'
import { MoreFilled, DataLine } from '@element-plus/icons-vue'

const { isListLayout, headlinePanels, loading, headlineSupplement, panels, activePanels } = useDashboardDomain('selection')
const { panelTone, panelIcon, panelDisplayValue, panelStateType, panelState, panelResultCount, dashboardHealth } = useDashboardDomain('display')
const { panelPending, refreshPanel } = useDashboardDomain('queries')
const { isFullscreen } = useDashboardDomain('fullscreen')
const { openEditPanel, handleDeletePanel } = useDashboardDomain('panelEditor')
const { autoRefreshSeconds } = useDashboardDomain('autoRefresh')
const { isDark } = useDashboardDomain('theme')
</script>

<template>
      <section v-if="!isListLayout && headlinePanels.length" class="dashboard-kpi-grid">
        <article v-for="panel in headlinePanels" :key="panel.id" :class="['dashboard-kpi-card', `tone-${panelTone(panel)}`]" v-loading="panelPending[panel.id]">
          <div class="dashboard-kpi-head"><span><el-icon><component :is="panelIcon(panel)" /></el-icon>{{ panel.title }}</span><el-dropdown :popper-class="isDark ? 'monitor-dark-popper' : ''" v-if="!isFullscreen" trigger="click"><el-button link :icon="MoreFilled" aria-label="面板操作" /><template #dropdown><el-dropdown-menu><el-dropdown-item @click="refreshPanel(panel)">刷新面板</el-dropdown-item><el-dropdown-item @click="openEditPanel(panel)">编辑面板</el-dropdown-item><el-dropdown-item divided @click="handleDeletePanel(panel)">删除面板</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div>
          <strong>{{ panelDisplayValue(panel) }}</strong>
          <div class="dashboard-kpi-meta"><span :class="`is-${panelStateType(panel)}`"><i></i>{{ panelState(panel) }}</span><small>{{ panelResultCount(panel) }} 条序列</small></div>
        </article>
        <article v-if="headlineSupplement" class="dashboard-kpi-card tone-cyan dashboard-kpi-card--supplement">
          <div class="dashboard-kpi-head"><span><el-icon><DataLine /></el-icon>监控面板</span></div>
          <strong>{{ headlineSupplement.value }}</strong>
          <div class="dashboard-kpi-meta"><span class="is-success"><i></i>持续采集</span><small>{{ headlineSupplement.healthy }} 个正常</small></div>
        </article>
      </section>

      <section v-else-if="!isListLayout" class="dashboard-summary">
        <div class="summary-card">
          <span>面板数量</span>
          <strong>{{ panels.length }}</strong>
        </div>
        <div class="summary-card">
          <span>启用面板</span>
          <strong>{{ activePanels.length }}</strong>
        </div>
        <div class="summary-card">
          <span>刷新周期</span>
          <strong>{{ autoRefreshSeconds ? `${autoRefreshSeconds}s` : '关闭' }}</strong>
        </div>
        <div class="summary-card">
          <span>大屏状态</span>
          <strong :class="`state-${dashboardHealth.type}`">{{ dashboardHealth.text }}</strong>
        </div>
      </section>
</template>

<style scoped>
.dashboard-main:is(:fullscreen, .is-presentation) .dashboard-summary {
  background: rgba(15, 23, 42, 0.92);
  border-color: rgba(96, 165, 250, 0.28);
  color: #dbeafe;
}
.dashboard-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(160px, 1fr));
  gap: 14px;
  margin-bottom: 16px;
}
.summary-card {
  position: relative;
  padding: 16px;
  border: 1px solid var(--dash-border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.9);
  overflow: hidden;
}
.summary-card::after {
  content: '';
  position: absolute;
  right: 14px;
  bottom: 12px;
  width: 54px;
  height: 8px;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(37, 99, 235, 0.16), rgba(34, 197, 94, 0.22));
}
.summary-card span {
  display: block;
  color: var(--dash-muted);
}
.summary-card strong {
  display: block;
  margin-top: 8px;
  color: var(--dash-text);
  font-size: 24px;
}
.observability-canvas .dashboard-summary {
  grid-template-columns: repeat(4, minmax(130px, 1fr));
  gap: 8px;
  margin-bottom: 10px;
}
.observability-canvas .summary-card {
  min-height: 72px;
  padding: 12px 14px;
  border-color: var(--dash-border);
  border-radius: 6px;
  background: var(--dash-surface);
}
.observability-canvas .summary-card::after {
  right: 12px;
  bottom: 11px;
  width: 34px;
  height: 3px;
  background: #3b82f6;
  opacity: 0.55;
}
.observability-canvas .summary-card span {
  font-size: 12px;
}
.observability-canvas .summary-card strong {
  margin-top: 5px;
  font-size: 21px;
}
.panel-state.is-success i { background: #22c55e; }
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-summary {
  gap: 6px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .summary-card {
  min-height: 64px;
  border-color: #283442;
  background: #111923;
}
.observability-canvas:is(:fullscreen, .is-presentation) .summary-card::after {
  display: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .summary-card span {
  color: #7f8ea3;
}
.observability-canvas:is(:fullscreen, .is-presentation) .summary-card strong {
  color: #e7edf5;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-summary {
  display: none;
}
@media (max-width: 1280px) {
  .dashboard-summary {
    grid-template-columns: repeat(2, minmax(220px, 1fr));
  }
}
.dashboard-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(150px, 1fr));
  gap: 9px;
  margin-bottom: 10px;
}
.dashboard-kpi-card {
  position: relative;
  min-height: 112px;
  padding: 13px 14px;
  overflow: hidden;
  border: 1px solid var(--dash-border);
  border-radius: 9px;
  background: var(--dash-surface);
  box-sizing: border-box;
}
.dashboard-kpi-card::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 3px;
  background: #4d7df4;
}
.dashboard-kpi-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.dashboard-kpi-head > span {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  overflow: hidden;
  color: var(--dash-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dashboard-kpi-head .el-icon {
  color: #4d7df4;
}
.dashboard-kpi-card > strong {
  display: block;
  margin-top: 9px;
  color: var(--dash-text);
  font-size: 25px;
  line-height: 1.15;
}
.dashboard-kpi-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  color: var(--dash-muted);
  font-size: 10px;
}
.dashboard-kpi-meta > span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.dashboard-kpi-meta i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22b786;
}
.dashboard-kpi-meta .is-warning i { background: #e7a517; }
.dashboard-kpi-meta .is-danger i { background: #e55858; }
.dashboard-kpi-card.tone-success::after { background: #24b982; }
.dashboard-kpi-card.tone-success .dashboard-kpi-head .el-icon { color: var(--dash-text); }
.dashboard-kpi-card.tone-warning::after { background: #f0a633; }
.dashboard-kpi-card.tone-warning .dashboard-kpi-head .el-icon { color: #df9120; }
.dashboard-kpi-card.tone-danger::after { background: #ea5b64; }
.dashboard-kpi-card.tone-danger .dashboard-kpi-head .el-icon { color: #dc4b56; }
.dashboard-kpi-card.tone-purple::after { background: #8a6deb; }
.dashboard-kpi-card.tone-purple .dashboard-kpi-head .el-icon { color: #7b60db; }
.dashboard-kpi-card.tone-cyan::after { background: #22aebd; }
.dashboard-kpi-card.tone-cyan .dashboard-kpi-head .el-icon { color: var(--dash-text); }
.is-k8s-dashboard .dashboard-kpi-grid {
  grid-template-columns: repeat(20, minmax(0, 1fr));
  gap: 10px;
}
.is-k8s-dashboard .dashboard-kpi-card {
  grid-column: span 4;
  min-height: 86px;
  padding: 14px 16px;
}
.is-k8s-dashboard .dashboard-kpi-card:nth-child(n + 6) {
  grid-column: span 5;
}
.is-k8s-dashboard .chart-panel .panel-state.is-success {
  display: none !important;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-card {
  border-color: #283442;
  background: #111923;
  color: #d7e0ea;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-head > span {
  color: #a9bad0;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-head .el-icon {
  color: #72a8ff;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-card > strong {
  color: #f1f6fc;
}
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-meta,
.observability-canvas:is(:fullscreen, .is-presentation) .dashboard-kpi-meta small {
  color: #879ab1;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-card {
  border-color: #283442;
  background: #111923;
  color: #d7e0ea;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-head > span {
  color: #a9bad0;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-head .el-icon {
  color: #72a8ff;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-card > strong {
  color: #f1f6fc;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-meta,
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .dashboard-kpi-meta small {
  color: #879ab1;
}
@media (max-width: 1180px) {
  .dashboard-kpi-grid { grid-template-columns: repeat(2, minmax(180px, 1fr)); }
}
@media (max-width: 1180px) {
  .is-k8s-dashboard .dashboard-kpi-grid { grid-template-columns: repeat(2, minmax(180px, 1fr)); }
}
@media (max-width: 1180px) {
  .is-k8s-dashboard .dashboard-kpi-card,
.is-k8s-dashboard .dashboard-kpi-card:nth-child(n + 6) { grid-column: span 1; }
}
@media (max-width: 1180px) {
  .is-k8s-dashboard .dashboard-kpi-card:last-child:nth-child(odd) { grid-column: span 2; }
}
@media (max-width: 760px) {
  .dashboard-kpi-grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .is-k8s-dashboard .dashboard-kpi-grid { grid-template-columns: 1fr; }
}
@media (max-width: 760px) {
  .is-k8s-dashboard .dashboard-kpi-card,
.is-k8s-dashboard .dashboard-kpi-card:nth-child(n + 6),
.is-k8s-dashboard .dashboard-kpi-card:last-child:nth-child(odd) { grid-column: span 1; }
}
</style>
