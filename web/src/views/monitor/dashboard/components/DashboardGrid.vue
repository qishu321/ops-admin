<script setup>
import { useDashboardDomain } from '../context'
import { Rank } from '@element-plus/icons-vue'
import PanelCard from './PanelCard.vue'

const { gridElement, layoutEditing, layoutSaving, layoutInteraction, layoutPreviewStyle, layoutPositionText, layoutMessage, layoutRecovery, retryLayoutRecovery } = useDashboardDomain('grid')
const { isK8sDashboard, isCustomGridLayout, visualPanels } = useDashboardDomain('selection')
const { isFullscreen } = useDashboardDomain('fullscreen')
</script>

<template>
      <section class="dashboard-grid-shell">
        <div v-if="layoutEditing || layoutRecovery" class="dashboard-layout-tip" role="status" aria-live="polite">
          <el-icon><Rank /></el-icon>
          <span>拖动十字手柄移动，右下角缩放 · 方向键微调 / Shift 四格 · Esc 取消 · 小屏可横向滑动编辑画布</span>
          <span class="dashboard-layout-saving">{{ layoutMessage }}</span>
          <el-button v-if="layoutRecovery" size="small" :loading="layoutSaving" @click="retryLayoutRecovery">重试恢复</el-button>
        </div>




        <div class="grid-viewport" :class="{ 'is-editing': layoutEditing }">
        <div ref="gridElement" class="panel-grid" :class="{ 'k8s-panel-grid': isK8sDashboard, 'is-custom-layout': isCustomGridLayout && !isFullscreen, 'is-layout-editing': layoutEditing }" :aria-busy="layoutSaving">
        <div v-if="layoutInteraction" class="layout-placeholder" :style="layoutPreviewStyle" aria-hidden="true"><span>{{ layoutPositionText }}</span></div>
        <PanelCard v-for="panel in visualPanels" :key="panel.id" :panel="panel" />

        </div>
        </div>
      </section>
</template>

<style scoped>
.grid-viewport { min-width: 0; }
.grid-viewport.is-editing { overflow: auto; padding: 5px; margin: -5px; }
.panel-grid,
.k8s-panel-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  grid-auto-flow: row dense;
  gap: 9px;
}
.panel-grid.is-layout-editing {
  position: relative;
  min-width: 720px;
  grid-template-columns: repeat(24, minmax(0, 1fr)) !important;
  grid-auto-rows: 32px;
  grid-auto-flow: row;
  gap: 9px !important;
  background-image: linear-gradient(to right, var(--dash-border) 1px, transparent 1px), linear-gradient(to bottom, var(--dash-border) 1px, transparent 1px);
  background-size: calc((100% + 9px) / 24) 41px;
  background-position: -1px -1px;
}
.layout-placeholder {
  z-index: 2;
  pointer-events: none;
  min-width: 0;
  border: 2px dashed var(--dash-accent);
  border-radius: 10px;
  background: color-mix(in srgb, var(--dash-accent) 12%, transparent);
}
.layout-placeholder span {
  display: inline-block;
  padding: 5px 8px;
  background: var(--dash-accent);
  color: var(--dash-canvas);
  border-radius: 5px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.dashboard-grid-shell {
  min-width: 0;
  padding: 0;
  background: transparent;
}
.dashboard-layout-tip {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-height: 40px;
  margin-bottom: 12px;
  padding: 8px 12px;
  border: 1px solid var(--dash-border);
  border-radius: 9px;
  background: var(--dash-subtle);
  color: var(--dash-muted);
  font-size: 12px;
}
.dashboard-layout-tip .el-icon { color: var(--dash-accent); font-size: 16px; }
.dashboard-layout-saving { margin-left: auto; color: var(--dash-muted); white-space: nowrap; }
.is-k8s-dashboard .k8s-panel-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  grid-auto-flow: row;
  gap: 14px;
}
@media (min-width: 761px) {
  .panel-grid.is-custom-layout {
    grid-template-columns: repeat(24, minmax(0, 1fr));
    grid-auto-rows: 32px;
    grid-auto-flow: row;
    gap: 9px;
  }
}
.observability-canvas:is(:fullscreen, .is-presentation) .panel-grid {
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 5px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-panel-grid {
  grid-template-columns: repeat(6, minmax(0, 1fr)) !important;
  grid-auto-flow: row;
  align-items: stretch;
  gap: 10px;
}
@media (max-width: 1180px) {
  .panel-grid,
  .k8s-panel-grid { grid-template-columns: repeat(2, minmax(240px, 1fr)); }
  .is-k8s-dashboard .k8s-panel-grid { grid-template-columns: repeat(2, minmax(240px, 1fr)); }
}
@media (max-width: 760px) {
  .panel-grid,
  .k8s-panel-grid { grid-template-columns: 1fr; }
}
</style>
