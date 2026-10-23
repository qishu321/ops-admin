<script setup>
import { useDashboardDomain } from '../../context'
import { CircleCheck } from '@element-plus/icons-vue'
import { computed, ref, useId, onBeforeUnmount } from 'vue'
import { smoothPath, areaPath } from '../../utils/curve'

const { trendCurrentLabel, panelStats, panelLineSeries, panelLegendSeries, toggleTrendSeries, handleTrendPointer, hideTrendTooltip, trendTooltips } = useDashboardDomain('trends')
const { panelDisplayValue } = useDashboardDomain('display')
const { isK8sDashboard } = useDashboardDomain('selection')
const { rangeStartText, rangeEndText } = useDashboardDomain('time')
const gradientId = 'trend-area-' + useId()
const props = defineProps({ panel: { type: Object, required: true } })

const stats = computed(() => panelStats(props.panel))
const currentValue = computed(() => panelDisplayValue(props.panel))
let pointerFrame = null
let pendingPointer
function onPointer(event) {
  pendingPointer = { currentTarget: event.currentTarget, clientX: event.clientX }
  if (pointerFrame !== null) return
  pointerFrame = requestAnimationFrame(() => {
    pointerFrame = null
    handleTrendPointer(props.panel, pendingPointer, series.value)
  })
}
function clearPointer() {
  if (pointerFrame !== null) cancelAnimationFrame(pointerFrame)
  pointerFrame = null
  pendingPointer = null
  hideTrendTooltip(props.panel)
}
onBeforeUnmount(clearPointer)

const LEGEND_PREVIEW = 4
const legendExpanded = ref(false)
const legend = computed(() => panelLegendSeries(props.panel))
const legendPreview = computed(() => (legendExpanded.value ? legend.value : legend.value.slice(0, LEGEND_PREVIEW)))
const legendExtra = computed(() => Math.max(legend.value.length - LEGEND_PREVIEW, 0))
const allHidden = computed(() => legend.value.length > 0 && legend.value.every(item => item.hidden))
const series = computed(() => panelLineSeries(props.panel).map(item => ({
  ...item,
  paths: item.segmentPoints.map(smoothPath).filter(Boolean),
  areas: item.segmentPoints.map(areaPath).filter(Boolean)
})))
</script>

<template>
<div class="trend-summary">
              <div class="trend-current">
                <span>{{ trendCurrentLabel(panel) }}</span>
                <strong>{{ currentValue }}</strong>
              </div>
              <div class="trend-stats">
                <span>最小 <b>{{ stats.min }}</b></span>
                <span>平均 <b>{{ stats.avg }}</b></span>
                <span>最大 <b>{{ stats.max }}</b></span>
              </div>
            </div>
            <div
              v-if="series.length"
              class="trend-chart"
              @mousemove="onPointer"
              @mouseleave="clearPointer"
            >
              <svg class="sparkline" viewBox="0 0 100 52" preserveAspectRatio="none">
                <defs>
                  <linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stop-color="#4f8cff" stop-opacity="0.28" />
                    <stop offset="100%" stop-color="#4f8cff" stop-opacity="0.02" />
                  </linearGradient>
                </defs>
                <line v-for="y in [8, 18, 28, 38, 48]" :key="y" x1="0" :y1="y" x2="100" :y2="y" class="chart-grid-line" />
                <path v-for="(area, areaIndex) in (series[0]?.areas || [])" :key="`area-${areaIndex}`" :d="area" :fill="`url(#${gradientId})`" />
                <template v-for="(item, index) in series" :key="`line-${item.name}-${index}`">
                  <path
                    v-for="(path, pathIndex) in item.paths"
                    :key="`line-${index}-${pathIndex}`"
                    class="line-series"
                    :d="path"
                    :style="{ stroke: item.color }"
                  />
                </template>
              </svg>
              <div v-if="panel.unit === '%'" class="trend-y-axis" aria-hidden="true">
                <span>100%</span>
                <span>0%</span>
              </div>
              <div
                v-if="trendTooltips[panel.id]"
                class="trend-crosshair"
                :style="{ left: trendTooltips[panel.id].x }"
              ></div>
              <div
                v-if="trendTooltips[panel.id]"
                class="trend-tooltip"
                :style="{ left: trendTooltips[panel.id].left }"
              >
                <strong>{{ trendTooltips[panel.id].time }}</strong>
                <div v-for="item in trendTooltips[panel.id].items" :key="item.name" class="trend-tooltip-item">
                  <i :style="{ background: item.color }"></i>
                  <span :title="item.name">{{ item.name }}</span>
                  <b>{{ item.value }}</b>
                </div>
              </div>
              <div class="trend-axis"><span>{{ isK8sDashboard ? rangeStartText : '起始' }}</span><span>{{ isK8sDashboard ? rangeEndText : '当前' }}</span></div>
            </div>
            <div v-if="legend.length > 1" class="trend-legend" :class="{ 'is-expanded': legendExpanded }">
              <button
                v-for="item in legendPreview"
                :key="item.name"
                type="button"
                class="trend-legend-item"
                :class="{ 'is-hidden': item.hidden }"
                :aria-pressed="!item.hidden"
                :title="item.hidden ? `显示 ${item.name}` : `隐藏 ${item.name}`"
                @click="toggleTrendSeries(panel, item.name)"
              ><i :style="{ background: item.color }"></i>{{ item.name }}</button>
              <button
                v-if="legendExtra"
                type="button"
                class="trend-legend-more"
                :aria-expanded="legendExpanded"
                :title="legendExpanded ? '收起图例' : `展开其余 ${legendExtra} 条序列`"
                @click="legendExpanded = !legendExpanded"
              >{{ legendExpanded ? '收起' : `+${legendExtra}` }}</button>
            </div>
            <div v-if="!series.length && allHidden" class="chart-empty-state">
              <el-icon><CircleCheck /></el-icon>
              <strong>已隐藏全部序列</strong>
              <span>点击上方图例可以重新显示</span>
            </div>
            <div v-else-if="!series.length" class="chart-empty-state">
              <el-icon><CircleCheck /></el-icon>
              <strong>当前时间范围内暂无趋势</strong>
              <span>调整时间范围或检查数据源后重试</span>
            </div>
            <div class="panel-query" :title="panel.promql">{{ panel.promql }}</div>
</template>

<style scoped>
/* 面板卡片的历史覆盖样式集中在 styles/panel-card.css，
   这里只保留趋势图自身的排版；.panel-query 全站隐藏（沿用既有视觉）。 */
.trend-summary {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  padding: 12px 14px 0;
}
.trend-current span,
.trend-current strong {
  display: block;
}
.trend-current span {
  color: var(--dash-muted);
  font-size: 11px;
}
.trend-current strong {
  margin-top: 2px;
  color: var(--dash-text);
  font-size: 24px;
  white-space: nowrap;
}
.trend-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  color: var(--dash-muted);
  font-size: 10px;
}
.trend-stats b {
  margin-left: 3px;
  color: var(--dash-muted);
  font-weight: 600;
}
.trend-chart {
  position: relative;
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  padding: 5px 14px 0;
}
/* Fills the card height, so stretching a panel in layout editing makes the plot
   taller instead of leaving an empty band under the axis. */
.trend-chart .sparkline {
  display: block;
  flex: 1 1 auto;
  width: 100%;
  min-height: 80px;
  height: auto;
}
.chart-empty-state {
  display: grid;
  flex: 1 1 auto;
  place-items: center;
  align-content: center;
  min-height: 160px;
  padding: 28px;
  color: var(--dash-muted);
  text-align: center;
}
.chart-empty-state .el-icon {
  margin-bottom: 9px;
  color: #36b37e;
  font-size: 28px;
}
.chart-empty-state strong {
  display: block;
  margin-bottom: 6px;
  color: var(--dash-text);
  font-size: 14px;
}
.chart-empty-state span {
  font-size: 12px;
}
.line-series {
  fill: none;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}
.chart-grid-line {
  stroke: var(--dash-grid);
  stroke-width: 0.5;
  vector-effect: non-scaling-stroke;
}
.trend-y-axis {
  position: absolute;
  top: 18px;
  bottom: 27px;
  left: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  color: var(--dash-muted);
  font-size: 10px;
  line-height: 1;
  pointer-events: none;
}
.trend-crosshair {
  position: absolute;
  z-index: 2;
  top: 7px;
  bottom: 24px;
  width: 0;
  border-left: 1px dashed #7a9dff;
  pointer-events: none;
}
.trend-tooltip {
  position: absolute;
  z-index: 3;
  top: 9px;
  min-width: 210px;
  max-width: 290px;
  padding: 9px 12px;
  transform: translateX(-50%);
  border: 1px solid var(--dash-border);
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.97);
  box-shadow: 0 10px 24px rgba(42, 77, 143, 0.16);
  color: var(--dash-muted);
  pointer-events: none;
}
.trend-tooltip > strong {
  display: block;
  padding-bottom: 6px;
  margin-bottom: 5px;
  border-bottom: 1px solid var(--dash-border);
  color: var(--dash-text);
  font-size: 13px;
  line-height: 1;
}
.trend-tooltip-item {
  display: grid;
  grid-template-columns: 7px minmax(0, 1fr) auto;
  align-items: center;
  gap: 7px;
  min-height: 20px;
  font-size: 11px;
  line-height: 1.2;
}
.trend-tooltip-item i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.trend-tooltip-item span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trend-tooltip-item b {
  color: var(--dash-muted);
  font-weight: 700;
  white-space: nowrap;
}
.trend-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 2px;
  color: var(--dash-muted);
  font-size: 9px;
}
.trend-legend {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 16px;
  padding: 2px 14px 0;
  overflow: hidden;
  color: var(--dash-muted);
  font-size: 9px;
  white-space: nowrap;
}
.trend-legend.is-expanded {
  overflow-x: auto;
  scrollbar-width: thin;
}
.trend-legend-item {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 4px;
  max-width: 150px;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.trend-legend-item > i {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.trend-legend-item:hover { color: var(--dash-text); }
.trend-legend-item:focus-visible { outline: 2px solid #2563eb; outline-offset: 2px; border-radius: 3px; }
.trend-legend-item.is-hidden { opacity: 0.45; text-decoration: line-through; }
.trend-legend-more {
  flex: 0 0 auto;
  padding: 0 4px;
  border: 0;
  border-radius: 3px;
  background: var(--dash-subtle);
  color: var(--dash-muted);
  font: inherit;
  cursor: pointer;
}
.trend-legend-more:hover { color: var(--dash-accent); }
.chart-empty-state {
  display: grid;
  place-items: center;
  align-content: center;
  min-height: 245px;
  padding: 28px;
  color: var(--dash-muted);
  text-align: center;
  background-image: repeating-linear-gradient(to bottom, transparent 0, transparent 48px, var(--dash-subtle) 49px, transparent 50px);
}
.chart-empty-state .el-icon {
  margin-bottom: 9px;
  color: var(--dash-text);
  font-size: 28px;
}
.chart-empty-state strong {
  color: var(--dash-text);
  font-size: 14px;
}
.chart-empty-state span {
  margin-top: 6px;
  font-size: 12px;
}
.panel-query {
  display: none;
}

/* ── 全屏 / 演示态 ────────────────────────────────────────── */
.observability-canvas:is(:fullscreen, .is-presentation) .trend-current strong {
  color: #e5edf6;
  font-size: 17px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-current span,
.observability-canvas:is(:fullscreen, .is-presentation) .trend-stats,
.observability-canvas:is(:fullscreen, .is-presentation) .trend-axis,
.observability-canvas:is(:fullscreen, .is-presentation) .trend-legend {
  color: #8493a7;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-stats b {
  color: #bdc9d8;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-grid-line {
  stroke: #263241;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-summary {
  padding: 7px 9px 0;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-stats {
  gap: 7px;
  font-size: 8px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-chart {
  padding: 2px 9px 0;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-chart .sparkline {
  height: 70px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .trend-legend {
  display: none;
}

/* ── Kubernetes 大屏 ──────────────────────────────────────── */
.is-k8s-dashboard .trend-summary {
  display: flex;
  min-height: 58px;
}
.is-k8s-dashboard .trend-chart {
  padding: 20px 18px 0;
}
/* 210px 是 k8s 面板的默认绘图高度；这里用 min-height 而不是 height，
   这样在编辑布局里把面板拉高时折线会跟着长，不会在下方留出空白。 */
.is-k8s-dashboard .trend-chart .sparkline {
  min-height: 210px;
}
.is-k8s-dashboard .trend-chart .line-series {
  stroke-width: 2;
}
.is-k8s-dashboard .trend-axis {
  margin-top: 5px;
  font-size: 10px;
}
.is-k8s-dashboard .trend-legend {
  min-height: 24px;
  gap: 12px;
  padding: 7px 18px 0;
  font-size: 10px;
}
.is-k8s-dashboard .trend-legend-item {
  max-width: 190px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-chart .sparkline {
  height: 160px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-summary {
  min-height: 44px;
  padding: 5px 9px 0;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-current strong {
  font-size: 15px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-stats {
  gap: 5px;
  font-size: 8px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-current span,
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-stats {
  color: #9badc2;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-current strong,
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .trend-stats b {
  color: #e6eef8;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state {
  color: #9badc2;
  background-image: repeating-linear-gradient(to bottom, transparent 0, transparent 48px, #263241 49px, transparent 50px);
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state strong {
  color: #d7e3f0;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state span {
  color: #9badc2;
}
</style>
