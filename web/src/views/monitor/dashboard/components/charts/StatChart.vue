<script setup>
import { useDashboardDomain } from '../../context'

const { panelValue } = useDashboardDomain('display')
const { sparklinePoints } = useDashboardDomain('trends')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
<div class="stat-row">
              <div>
                <div class="stat-value">{{ panelValue(panel) }}<small>{{ panel.unit }}</small></div>
                <div class="stat-caption">
                  <span></span>
                  实时采样
                </div>
              </div>
              <svg v-if="panel.chartType === 'line'" class="sparkline" viewBox="0 0 100 52" preserveAspectRatio="none">
                <polyline :points="sparklinePoints(panel)" />
              </svg>
            </div>
            <div class="promql">{{ panel.promql }}</div>
</template>

<style scoped>
.stat-row {
  position: relative;
  z-index: 1;
  display: grid;
  flex: 1 1 auto;
  grid-template-columns: 180px 1fr;
  align-content: center;
  align-items: center;
  gap: 18px;
}
.stat-value {
  color: var(--dash-accent);
  font-size: 44px;
  font-weight: 850;
  line-height: 1.1;
  text-shadow: 0 8px 24px rgba(37, 99, 235, 0.18);
}
.stat-value small {
  margin-left: 6px;
  color: var(--dash-text);
  font-size: 14px;
}
.stat-caption {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  margin-top: 12px;
  padding: 5px 10px;
  border-radius: 999px;
  color: var(--dash-muted);
  background: var(--dash-subtle);
  font-size: 12px;
}
.stat-caption span {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.14);
}
.sparkline {
  width: 100%;
  height: 92px;
}
.sparkline polyline {
  fill: none;
  stroke: #3b82f6;
  stroke-width: 4;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.chart-panel .stat-row {
  display: block;
  margin: 14px;
}
.chart-panel .stat-value {
  font-size: 36px;
  text-shadow: none;
}
.chart-panel.panel-stat .stat-row {
  margin: 22px 14px 10px;
}
/* 统计卡保持紧凑：PromQL 只在趋势/柱状卡片上展示。 */
.chart-panel .promql {
  display: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-value {
  color: #60a5fa;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-value small {
  color: #7f8ea3;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel.panel-stat .stat-row {
  margin-top: 34px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-caption {
  color: #9aabc0;
  background: #182330;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel.panel-stat .stat-row,
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-row {
  margin: 20px 10px 8px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-value {
  font-size: 30px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .stat-caption {
  margin-top: 7px;
  padding: 3px 7px;
  font-size: 9px;
}
</style>
