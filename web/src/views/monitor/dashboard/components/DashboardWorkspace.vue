<script setup>
import { useDashboardDomain } from '../context'
import { Monitor, Refresh, Setting, DataLine, Plus } from '@element-plus/icons-vue'

const { isListLayout, pageDescription, loading, activeDashboard, pageTitle, visibleDashboards, activeDashboardId } = useDashboardDomain('selection')
const { lastRefreshText, refreshAllPanels, isRefreshing, syncStatus } = useDashboardDomain('queries')
const { loadLatestInspectionReport, inspectionReport } = useDashboardDomain('inspection')
const { handleDashboardManageCommand, openCreateDashboard } = useDashboardDomain('dashboardEditor')
const { loadDashboard } = useDashboardDomain('loader')
const { isDark } = useDashboardDomain('theme')
</script>

<template>
    <section class="dashboard-workspace" :class="{ 'is-inspection-workspace': isListLayout }">
      <div v-if="!isListLayout" class="workspace-title dashboard-page-title">
        <span class="brand-mark"><el-icon><Monitor /></el-icon></span>
        <div>
          <strong>监控大屏</strong>
          <p>{{ pageDescription }}</p>
        </div>
      </div>

      <div class="workspace-status">
        <span :class="isListLayout ? 'sync-idle' : `sync-${syncStatus.type}`" role="status"><i class="health-dot"></i>{{ isListLayout ? (inspectionReport ? `报告 #${inspectionReport.id}` : '等待巡检') : syncStatus.text }}</span>
        <small v-if="!isListLayout" title="最近查询完成时间">{{ lastRefreshText }}</small>
        <el-button :icon="Refresh" :loading="loading || (!isListLayout && isRefreshing)" @click="isListLayout ? loadLatestInspectionReport() : refreshAllPanels()">{{ isListLayout ? '刷新报告' : '刷新' }}</el-button>
        <el-dropdown :popper-class="isDark ? 'monitor-dark-popper' : ''" v-if="activeDashboard" trigger="click" @command="handleDashboardManageCommand">
          <el-button :icon="Setting" :aria-label="isListLayout ? '巡检方案设置' : '大屏设置'" />
          <template #dropdown><el-dropdown-menu><el-dropdown-item command="edit">编辑{{ isListLayout ? '巡检方案' : pageTitle }}</el-dropdown-item><el-dropdown-item command="delete" divided>删除{{ isListLayout ? '巡检方案' : pageTitle }}</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
      </div>

      <div class="dashboard-switcher">
        <el-scrollbar>
          <div class="dashboard-list">
          <button
            v-for="item in visibleDashboards"
            :key="item.id"
            class="dashboard-item"
            :class="{ active: item.id === activeDashboardId }"
            @click="loadDashboard(item.id)"
          >
            <span class="dashboard-item-icon"><el-icon><DataLine /></el-icon></span>
            <strong>{{ item.name }}</strong>
            <span class="dashboard-item-meta">{{ item.panelCount || 0 }} 个{{ isListLayout ? '巡检项' : '面板' }}</span>
            <span class="dashboard-item-arrow">›</span>
          </button>
          <div v-if="!visibleDashboards.length" class="empty-switcher">暂无{{ isListLayout ? '巡检方案' : pageTitle }}</div>
          </div>
        </el-scrollbar>
        <el-button class="create-screen-btn" type="primary" plain :icon="Plus" @click="openCreateDashboard">{{ isListLayout ? '新建巡检方案' : `创建${pageTitle}` }}</el-button>
      </div>
    </section>
</template>

<style scoped>
.workspace-status .sync-error { color: var(--dash-danger); }
.workspace-status .sync-idle { color: var(--dash-muted); }
.workspace-status .sync-loading { color: var(--dash-accent); }
.workspace-status .health-dot { background: currentColor; box-shadow: none; }
.dashboard-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px 20px;
  padding: 14px 16px 12px;
  border: 1px solid var(--dash-border);
  border-radius: 10px;
  background: var(--dash-surface);
  box-shadow: 0 3px 10px rgba(26, 54, 93, 0.05);
}
.dashboard-workspace.is-inspection-workspace {
  padding: 8px 10px;
}
.is-inspection-workspace .dashboard-switcher {
  grid-column: 1;
  grid-row: 1;
}
.is-inspection-workspace .workspace-status {
  grid-column: 2;
  grid-row: 1;
}
.workspace-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.dashboard-page-title {
  display: flex;
  align-items: center;
  min-width: 0;
}
.brand-mark {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: linear-gradient(135deg, #2563eb, #60a5fa);
  color: var(--dash-muted);
  font-weight: 800;
}
.dashboard-page-title .brand-mark {
  width: 34px;
  height: 34px;
  border: 1px solid var(--dash-border);
  border-radius: 8px;
  background: var(--dash-subtle);
  color: var(--dash-accent);
  font-size: 18px;
  box-shadow: none;
}
.workspace-title strong,
.workspace-title p {
  display: block;
  margin: 0;
}
.workspace-title strong {
  color: var(--dash-text);
  font-size: 16px;
}
.workspace-title p {
  margin-top: 4px;
  color: var(--dash-muted);
  font-size: 13px;
}
.dashboard-page-title strong {
  font-size: 17px;
}
.dashboard-page-title p {
  margin-top: 2px;
  font-size: 11px;
}
.create-screen-btn {
  min-width: 132px;
  height: 42px;
}
.workspace-status {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  color: var(--dash-muted);
  font-size: 12px;
}
.workspace-status > span {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--dash-muted);
  font-weight: 600;
}
.workspace-status small {
  color: var(--dash-muted);
}
.workspace-status .el-button {
  margin-left: 0;
}
.dashboard-switcher {
  grid-column: 1 / -1;
  display: flex;
  align-items: stretch;
  gap: 10px;
  min-width: 0;
  padding: 3px;
  border: 0;
  border-radius: 8px;
  background: var(--dash-subtle);
}
.dashboard-switcher .el-scrollbar {
  min-width: 0;
  flex: 1;
}
.dashboard-list {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 56px;
}
.dashboard-item {
  position: relative;
  display: grid;
  grid-template-columns: 30px minmax(80px, 1fr) auto 14px;
  align-items: center;
  gap: 8px;
  min-width: 210px;
  flex: 1 0 240px;
  padding: 9px 12px;
  border: 1px solid transparent;
  border-radius: 7px;
  background: transparent;
  color: var(--dash-muted);
  text-align: left;
  cursor: pointer;
  transition: 0.18s ease;
}
.dashboard-item strong,
.dashboard-item span {
  display: block;
}
.dashboard-item span {
  margin-top: 5px;
  color: var(--dash-muted);
  font-size: 12px;
}
.dashboard-item strong {
  color: var(--dash-text);
  font-size: 13px;
}
.dashboard-item.active,
.dashboard-item:hover {
  border-color: var(--dash-border);
  background: var(--dash-surface);
  color: var(--dash-muted);
  box-shadow: 0 2px 7px rgba(44, 88, 170, 0.08);
}
.dashboard-item.active span,
.dashboard-item:hover span {
  color: var(--dash-muted);
}
.dashboard-item.active::after {
  content: '';
  position: absolute;
  right: 10px;
  bottom: -4px;
  left: 10px;
  height: 2px;
  border-radius: 2px;
  background: #4d7df4;
}
.dashboard-item.active strong {
  color: var(--dash-accent);
}
.dashboard-item-icon {
  display: grid !important;
  place-items: center;
  width: 28px;
  height: 28px;
  margin: 0 !important;
  border-radius: 7px;
  background: var(--dash-subtle);
  color: #4a7ff5 !important;
  font-size: 15px !important;
}
.dashboard-item-meta {
  margin: 0 !important;
  color: var(--dash-muted) !important;
  font-size: 11px !important;
  white-space: nowrap;
}
.dashboard-item-arrow {
  margin: 0 !important;
  color: var(--dash-muted) !important;
  font-size: 18px !important;
}
.empty-switcher {
  padding: 0 14px;
  color: var(--dash-muted);
  white-space: nowrap;
}
.dashboard-switcher .create-screen-btn {
  width: auto;
  min-width: 132px;
  margin: 7px;
}
.health-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.12);
}
.workspace-status .health-dot {
  width: 7px;
  height: 7px;
  box-shadow: 0 0 0 3px rgba(38, 185, 139, 0.12);
}
@media (max-width: 1280px) {
  .dashboard-workspace {
    grid-template-columns: 1fr;
    align-items: stretch;
  }
  .create-screen-btn {
    width: 100%;
  }
}
@media (max-width: 760px) {
  .dashboard-workspace { grid-template-columns: 1fr; }
  .workspace-status,
  .dashboard-switcher { grid-column: 1; }
  .is-inspection-workspace .dashboard-switcher,
  .is-inspection-workspace .workspace-status { grid-row: auto; }
  .workspace-status { justify-content: flex-start; flex-wrap: wrap; }
  .dashboard-switcher { align-items: stretch; flex-direction: column; }
  .dashboard-switcher .create-screen-btn { width: auto; }
}
</style>
