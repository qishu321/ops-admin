<script setup>
import { useDashboardDomain } from '../context'
import { Rank, BottomRight, MoreFilled } from '@element-plus/icons-vue'
import PanelFeedback from './PanelFeedback.vue'
import { ref } from 'vue'
import { usePanelVisibility } from '../composables/usePanelVisibility'
import PanelContent from './PanelContent.vue'

const { panelVisualType, isK8sDashboard, loading, panelDisplayTitle, currentDatasourceName } = useDashboardDomain('selection')
const { panelGridStyle, layoutEditing, startPanelInteraction, updatePanelInteraction, finishPanelInteraction, cancelPanelInteraction, layoutInteraction, layoutLocked, movePanelWithKeyboard, resizePanelWithKeyboard } = useDashboardDomain('grid')
const { panelPending, refreshPanel, panelResults } = useDashboardDomain('queries')
const { isFullscreen } = useDashboardDomain('fullscreen')
const { panelChartLabel, panelVisibleSeriesCount, panelRows, panelDisplayValue, panelStateType, panelState } = useDashboardDomain('display')
const { openEditPanel, handleDeletePanel } = useDashboardDomain('panelEditor')
defineProps({ panel: { type: Object, required: true } })
const cardElement = ref(null)
const visible = usePanelVisibility(cardElement)
const { openPanelFocus, explorePanel } = useDashboardDomain('view')
const { isDark } = useDashboardDomain('theme')
</script>

<template>
        <div
          ref="cardElement"
          class="metric-panel chart-panel"
          :data-panel-id="panel.id"
           :class="[`panel-${panelVisualType(panel)}`, { 'is-interacting': layoutInteraction?.panel.id === panel.id, disabled: panel.status !== 1, 'k8s-pod-panel': isK8sDashboard && panel.title.startsWith('Pod '), 'k8s-wide-panel': isK8sDashboard && panel.title === 'Pod 资源明细' }]"
            :style="panelGridStyle(panel)"
            v-loading="panelPending[panel.id] && !!panelResults[panel.id]"
            :aria-busy="!!panelPending[panel.id]"
           :element-loading-background="isDark || isFullscreen ? 'rgba(8, 15, 28, 0.82)' : 'rgba(255, 255, 255, 0.72)'"
        >
          <div class="panel-glow"></div>
          <div class="panel-head">
            <div class="panel-identity">
              <div class="panel-title-row">
                <button
                  v-if="layoutEditing"
                  :aria-disabled="layoutLocked"
                  type="button"
                  class="panel-move-handle"
                  aria-label="拖动面板；也可使用方向键移动，按住 Shift 可一次移动四格"
                  @pointerdown.stop="startPanelInteraction($event, panel, 'move')"
                  @pointermove="updatePanelInteraction"
                  @pointerup="finishPanelInteraction"
                  @pointercancel="cancelPanelInteraction"
                  @lostpointercapture="cancelPanelInteraction"
                  @keydown="movePanelWithKeyboard($event, panel)"
                ><el-icon><Rank /></el-icon></button>
                <i class="panel-signal"></i>
                <strong :title="panelDisplayTitle(panel)">{{ panelDisplayTitle(panel) }}</strong>
              </div>
              <span>{{ panelChartLabel(panelVisualType(panel)) }} · {{ panelVisibleSeriesCount(panel) }} 条序列 · {{ currentDatasourceName }}</span>
            </div>
            <div class="panel-actions">
              <template v-if="layoutEditing">
                <span class="panel-drag-hint">{{ panel.gridW }} × {{ panel.gridH }}</span>
              </template>
              <template v-else>
                <span v-if="isK8sDashboard && !['table', 'stat'].includes(panelVisualType(panel)) && panelRows(panel).length" class="k8s-value-badge">{{ panelDisplayValue(panel) }}</span>
                <span class="panel-state" :class="`is-${panelStateType(panel)}`"><i></i>{{ panelState(panel) }}</span>
                <el-dropdown :popper-class="isDark ? 'monitor-dark-popper' : ''" v-if="!isFullscreen" trigger="click">
                  <el-button link :icon="MoreFilled" :aria-label="`${panel.title}面板操作`" />
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item @click="openPanelFocus(panel)">面板全屏</el-dropdown-item>
                      <el-dropdown-item @click="explorePanel(panel)">查询此指标</el-dropdown-item>
                      <el-dropdown-item @click="refreshPanel(panel)">刷新面板</el-dropdown-item>
                      <el-dropdown-item @click="openEditPanel(panel)">编辑面板</el-dropdown-item>
                      <el-dropdown-item divided @click="handleDeletePanel(panel)">删除面板</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </template>
            </div>
          </div>
          <button
            v-if="layoutEditing"
                  :aria-disabled="layoutLocked"
            type="button"
            class="panel-resize-handle"
            aria-label="调整面板大小；也可使用方向键调整，按住 Shift 可一次调整四格"
            @pointerdown.stop="startPanelInteraction($event, panel, 'resize')"
            @pointermove="updatePanelInteraction"
            @pointerup="finishPanelInteraction"
            @pointercancel="cancelPanelInteraction"
                  @lostpointercapture="cancelPanelInteraction"
            @keydown="resizePanelWithKeyboard($event, panel)"
          ><el-icon><BottomRight /></el-icon></button>

          <PanelContent v-if="visible || layoutEditing || isFullscreen" :panel="panel" />
          <PanelFeedback v-else loading />

        </div>
</template>

<style scoped src="../styles/panel-card.css"></style>
