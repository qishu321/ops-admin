<script setup>
import { useDashboardDomain } from '../context'
import PanelContent from './PanelContent.vue'
const { focusedPanel, closePanelFocus, explorePanel, copyViewLink, shareDialogVisible, shareUrl } = useDashboardDomain('view')
const { isDark } = useDashboardDomain('theme')
const { refreshPanel, panelPending } = useDashboardDomain('queries')
const { selectTimeRange, timeRangeSeconds } = useDashboardDomain('time')
</script>
<template>
  <el-dialog :model-value="!!focusedPanel" :title="focusedPanel?.title" fullscreen destroy-on-close class="monitor-dialog panel-focus-dialog" :class="{ 'is-dark': isDark }" @close="closePanelFocus">
    <template v-if="focusedPanel">
      <div class="focus-controls">
        <span>时间范围</span>
        <el-select :model-value="timeRangeSeconds" aria-label="聚焦面板时间范围" @change="selectTimeRange">
          <el-option v-for="item in [{ label: '15 分钟', value: 900 }, { label: '1 小时', value: 3600 }, { label: '6 小时', value: 21600 }, { label: '24 小时', value: 86400 }]" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button :loading="!!panelPending[focusedPanel.id]" @click="refreshPanel(focusedPanel)">刷新面板</el-button>
        <el-button @click="explorePanel(focusedPanel)">查询此指标</el-button>
        <el-button @click="copyViewLink">复制视图链接</el-button>
      </div>
      <div class="focus-content"><PanelContent :panel="focusedPanel" /></div>
    </template>
  </el-dialog>
  <el-dialog v-model="shareDialogVisible" title="复制当前视图链接" width="min(560px, 92vw)" class="monitor-dialog" :class="{ 'is-dark': isDark }">
    <p>浏览器未允许自动复制，请选中链接复制。</p>
    <el-input :model-value="shareUrl" readonly aria-label="当前视图链接" @focus="$event.target.select()" />
  </el-dialog>
</template>
<style scoped>
.focus-controls { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin: 0 0 24px; }
.focus-controls .el-select { width: 140px; }
.focus-content { min-width: 0; padding: 24px; border: 1px solid var(--dash-border); border-radius: 10px; background: var(--dash-surface); }
.focus-content :deep(.trend-chart) { height: clamp(280px, 56vh, 800px); }
.focus-content :deep(.trend-chart svg) { height: 100%; }
@media (max-width: 760px) { .focus-content { padding: 12px; } }
</style>
