<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { DataLine, Loading, Refresh, WarningFilled } from '@element-plus/icons-vue'
import { useDashboardDomain } from '../context'
import PanelContent from './PanelContent.vue'

const { panelDialogVisible, editingPanel, panelForm, submitPanel } = useDashboardDomain('panelEditor')
const { isListLayout, datasourceOptions, panelVisualType } = useDashboardDomain('selection')
const { isDark } = useDashboardDomain('theme')
const { previewPanel, previewState, previewRangeText, previewSeriesText, runPreview } = useDashboardDomain('panelPreview')

const previewStateText = computed(() => {
  if (previewState.refreshing) return '正在查询'
  return {
    idle: '等待预览',
    loading: '正在查询',
    ready: '预览已更新',
    empty: '无数据',
    error: '查询失败'
  }[previewState.status] || '等待预览'
})

const canPreview = computed(() => !!String(panelForm.promql || '').trim() && !!panelForm.datasourceId)

// 面板弹窗带实时预览，占视口 75% 才有足够的绘图宽度；巡检项只有表单，保持原宽。
const dialogWidth = computed(() => (isListLayout.value ? '860px' : '75%'))

// K8s 大屏会把部分标题强制成趋势图，预览高度必须跟实际挂载的图表一致。
const previewChartType = computed(() => panelVisualType(previewPanel.value))

// 矮屏（如 1366×768）里表单加预览会超过 90vh；出图或切换图表类型后若预览没
// 完全露出，就把它滚进视野。已完整可见时不做任何移动。
const previewStageRef = ref(null)

watch([() => previewState.status, previewChartType], async ([status]) => {
  if (status !== 'ready') return
  await nextTick()
  const stage = previewStageRef.value
  const scroller = stage?.closest('.el-dialog__body')
  if (!stage || !scroller) return
  const gap = stage.getBoundingClientRect().bottom - scroller.getBoundingClientRect().bottom
  if (gap > 4) scroller.scrollTo({ top: scroller.scrollTop + gap + 16, behavior: 'smooth' })
})
</script>

<template>
    <el-dialog class="monitor-dialog panel-editor-dialog" :class="{ 'is-dark': isDark }" v-model="panelDialogVisible" :title="editingPanel ? (isListLayout ? '编辑巡检项' : '编辑面板') : (isListLayout ? '新增巡检项' : '新增面板')" :width="dialogWidth">
      <el-form label-width="110px">
        <el-form-item label="标题" required><el-input v-model="panelForm.title" /></el-form-item>
        <el-form-item label="数据源" required>
          <el-select v-model="panelForm.datasourceId" filterable style="width: 100%">
            <el-option v-for="item in datasourceOptions" :key="item.id" :label="item.name" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="PromQL" required><el-input v-model="panelForm.promql" type="textarea" :rows="3" placeholder="例如：up 或 sum(rate(http_requests_total[5m]))" /></el-form-item>
        <template v-if="isListLayout">
          <el-form-item label="巡检用途">
            <el-radio-group v-model="panelForm.inspectionKind"><el-radio value="rule">阈值判定</el-radio><el-radio value="context">仅记录快照</el-radio></el-radio-group>
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="8"><el-form-item label="分类"><el-input v-model="panelForm.category" placeholder="例如：可用性" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="窗口统计"><el-select v-model="panelForm.reducer" style="width: 100%"><el-option label="末值" value="last" /><el-option label="最大值" value="max" /><el-option label="最小值" value="min" /><el-option label="平均值" value="avg" /><el-option label="P95" value="p95" /><el-option label="合计" value="sum" /><el-option label="计数器增量" value="increase" /><el-option label="样本数" value="count" /></el-select></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="异常方向"><el-select v-model="panelForm.operator" style="width: 100%"><el-option label="数值越大越异常（≥）" value="gte" /><el-option label="数值越小越异常（≤）" value="lte" /></el-select></el-form-item></el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :span="8"><el-form-item label="待核查阈值"><el-input-number v-model="panelForm.warningValue" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="异常阈值"><el-input-number v-model="panelForm.criticalValue" :controls="false" style="width: 100%" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="无数据判定"><el-select v-model="panelForm.noDataState" style="width: 100%"><el-option label="待核查" value="warning" /><el-option label="异常" value="danger" /><el-option label="正常" value="healthy" /></el-select></el-form-item></el-col>
          </el-row>
        </template>
        <el-row :gutter="12">
          <el-col v-if="!isListLayout" :span="8">
            <el-form-item label="图表类型">
              <el-select v-model="panelForm.chartType">
                <el-option label="指标卡" value="stat" />
                <el-option label="折线趋势" value="line" />
                <el-option label="柱状排行" value="bar" />
                <el-option label="仪表盘" value="gauge" />
                <el-option label="表格" value="table" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="isListLayout ? 12 : 8"><el-form-item label="单位"><el-input v-model="panelForm.unit" placeholder="%, ms, 个" /></el-form-item></el-col>
          <el-col v-if="!isListLayout" :span="8"><el-form-item label="宽度"><el-input-number v-model="panelForm.span" :min="6" :max="24" :step="6" style="width: 100%" /></el-form-item></el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="8"><el-form-item label="排序"><el-input-number v-model="panelForm.sort" style="width: 100%" /></el-form-item></el-col>
          <el-col :span="16">
            <el-form-item label="状态">
              <el-radio-group v-model="panelForm.status">
                <el-radio :value="1">启用</el-radio>
                <el-radio :value="2">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="描述"><el-input v-model="panelForm.description" type="textarea" :rows="2" /></el-form-item>
        <el-form-item v-if="!isListLayout" label="实时预览">
          <div class="panel-preview">
            <div class="panel-preview-bar">
              <span class="panel-preview-state" :class="`is-${previewState.status}`">
                <i></i>
                {{ previewStateText }}
                <em v-if="previewState.status === 'ready' && previewSeriesText">{{ previewSeriesText }}</em>
              </span>
              <span v-if="previewRangeText" class="panel-preview-range">{{ previewRangeText }}</span>
              <el-button class="panel-preview-refresh" link :disabled="!canPreview" @click="runPreview">
                <el-icon :class="{ 'is-spinning': previewState.refreshing }"><Refresh /></el-icon>
                {{ previewState.refreshing ? '查询中' : '重新预览' }}
              </el-button>
            </div>
            <div ref="previewStageRef" class="panel-preview-stage" :class="`is-${previewChartType}`">
              <PanelContent v-if="previewState.status === 'ready'" :panel="previewPanel" />
              <div v-else class="panel-preview-hint" :class="{ 'is-error': previewState.status === 'error' }" role="status" aria-live="polite">
                <el-icon v-if="previewState.status === 'loading'" class="is-spinning"><Loading /></el-icon>
                <el-icon v-else-if="previewState.status === 'error'"><WarningFilled /></el-icon>
                <el-icon v-else><DataLine /></el-icon>
                <p>{{ previewState.status === 'error' ? previewState.error : previewState.message }}</p>
                <small v-if="previewState.status === 'error' && previewState.errorDetail" :title="previewState.errorDetail">{{ previewState.errorDetail }}</small>
              </div>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="panelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitPanel">保存</el-button>
      </template>
    </el-dialog>
</template>

<style scoped>
.panel-preview {
  width: 100%;
}
/* The preview makes this dialog tall: cap it at 90vh and let the form area
   scroll, so the footer buttons stay reachable on any viewport. The dialog
   teleports outside this component's scope, so the selector has to be global. */
:global(.panel-editor-dialog) {
  --el-dialog-margin-top: 5vh;
  display: flex;
  flex-direction: column;
  max-height: 90vh;
  margin-bottom: 5vh;
}
:global(.panel-editor-dialog .el-dialog__body) {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}
.panel-preview-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
  color: var(--dash-muted);
  font-size: 12px;
  line-height: 1.4;
}
.panel-preview-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--dash-text);
  font-weight: 600;
}
.panel-preview-state i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--dash-muted);
  opacity: 0.55;
}
.panel-preview-state.is-loading i,
.panel-preview-state.is-idle i {
  background: var(--dash-accent);
  opacity: 1;
}
.panel-preview-state.is-ready i {
  background: #22c55e;
  opacity: 1;
}
.panel-preview-state.is-empty i {
  background: #eab308;
  opacity: 1;
}
.panel-preview-state.is-error i {
  background: var(--dash-danger);
  opacity: 1;
}
.panel-preview-state em {
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--dash-subtle);
  color: var(--dash-muted);
  font-size: 11px;
  font-style: normal;
  font-weight: 600;
}
.panel-preview-range {
  overflow: hidden;
  color: var(--dash-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.panel-preview-refresh {
  margin-left: auto;
}
.panel-preview-stage {
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: 260px;
  padding: 4px 0;
  overflow: auto;
  border: 1px solid var(--dash-border);
  border-radius: 9px;
  background: var(--dash-canvas);
  box-sizing: border-box;
}
/* Height follows the chart that is actually mounted, so the dialog does not
   jump around while the user compares chart types. */
.panel-preview-stage.is-stat {
  min-height: 160px;
}
.panel-preview-stage.is-gauge {
  min-height: 215px;
}
.panel-preview-stage.is-bar {
  min-height: 210px;
  justify-content: flex-start;
}
.panel-preview-stage.is-table {
  height: clamp(240px, 30vh, 360px);
  min-height: 0;
  justify-content: flex-start;
}
/* Inside a dashboard card the chart stretches to the card height. In the dialog
   the stage owns the height instead, so the natural content height is kept and
   the .is-line rule below decides how tall the plot is. */
.panel-preview-stage :deep(.panel-body),
.panel-preview-stage :deep(.trend-chart),
.panel-preview-stage :deep(.stat-row),
.panel-preview-stage :deep(.gauge-wrap),
.panel-preview-stage :deep(.bar-chart) {
  flex: none;
}
/* The 75%-wide dialog gives the trend chart room to breathe; the plate grows
   with the viewport so an ultra-wide window does not end up with a flat strip.
   Stroke width stays 1.8px because the paths use non-scaling-stroke. */
.panel-preview-stage.is-line :deep(.trend-chart .sparkline) {
  height: clamp(150px, 11vw, 250px);
}
.panel-preview-stage.is-table :deep(.panel-body) {
  flex: 1 1 auto;
  min-height: 0;
}
.panel-preview-stage :deep(.el-table) {
  height: 100% !important;
}
/* The dialog already shows the PromQL textarea right above the preview, so the
   stat card's inline query caption would only repeat it. */
.panel-preview-stage :deep(.promql) {
  display: none;
}
.panel-preview-hint {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 24px;
  color: var(--dash-muted);
  text-align: center;
}
.panel-preview-hint .el-icon {
  font-size: 22px;
  color: var(--dash-accent);
}
.panel-preview-hint.is-error .el-icon {
  color: var(--dash-danger);
}
.panel-preview-hint p {
  margin: 0;
  max-width: 100%;
  color: var(--dash-text);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.panel-preview-hint small {
  max-width: 100%;
  overflow: hidden;
  color: var(--dash-muted);
  font-size: 11px;
  line-height: 1.5;
  text-overflow: ellipsis;
  overflow-wrap: anywhere;
}
.is-spinning {
  animation: preview-spin 1s linear infinite;
}
@media (max-height: 820px) {
  .panel-preview-stage {
    min-height: 176px;
  }
  .panel-preview-stage.is-stat {
    min-height: 128px;
  }
  .panel-preview-stage.is-gauge {
    min-height: 180px;
  }
  .panel-preview-stage.is-bar {
    min-height: 150px;
  }
  .panel-preview-stage.is-table {
    height: clamp(160px, 22vh, 220px);
  }
  .panel-preview-stage.is-line :deep(.trend-chart .sparkline) {
    height: 108px;
  }
}
@keyframes preview-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .is-spinning {
    animation: none;
  }
}
</style>
