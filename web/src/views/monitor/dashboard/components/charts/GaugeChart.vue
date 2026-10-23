<script setup>
import { useDashboardDomain } from '../../context'

const { gaugePercent, panelValue } = useDashboardDomain('display')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
<div class="gauge-wrap">
              <div class="gauge" :style="{ '--value': `${gaugePercent(panel) * 3.6}deg` }">
                <div>
                  <strong>{{ panelValue(panel) }}</strong>
                  <span>{{ panel.unit }}</span>
                </div>
              </div>
            </div>
            <div class="panel-query" :title="panel.promql">{{ panel.promql }}</div>
</template>

<style scoped>
.gauge-wrap {
  position: relative;
  z-index: 1;
  display: grid;
  flex: 1 1 auto;
  place-items: center;
  min-height: 185px;
}
.gauge {
  position: relative;
  display: grid;
  place-items: center;
  width: 180px;
  height: 180px;
  border-radius: 50%;
  background: conic-gradient(#2563eb var(--value), var(--dash-subtle) 0deg);
  box-shadow: inset 0 0 0 1px rgba(37, 99, 235, 0.08), 0 18px 32px rgba(37, 99, 235, 0.16);
}
.gauge::before {
  content: '';
  position: absolute;
}
.gauge > div {
  display: grid;
  place-items: center;
  width: 126px;
  height: 126px;
  border-radius: 50%;
  background: var(--dash-surface);
}
.gauge strong {
  color: var(--dash-text);
  font-size: 34px;
}
.gauge span {
  color: var(--dash-muted);
  font-size: 13px;
}
.chart-panel .gauge-wrap {
  margin: 14px;
  min-height: 142px;
}
.chart-panel .gauge {
  width: 138px;
  height: 138px;
  box-shadow: none;
}
.chart-panel .gauge > div {
  width: 100px;
  height: 100px;
}
.chart-panel .gauge strong {
  font-size: 28px;
}
/* 仪表卡保持紧凑：PromQL 只在趋势/柱状卡片上展示。 */
.panel-query {
  display: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .gauge {
  width: 96px;
  height: 96px;
  background: conic-gradient(#3b82f6 var(--value), #263241 0deg);
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .gauge > div {
  width: 68px;
  height: 68px;
  background: #111923;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .gauge strong {
  color: #e8eef6;
  font-size: 20px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .gauge span {
  color: #7f8ea3;
  font-size: 10px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .gauge-wrap {
  min-height: 112px;
  margin: 6px;
}
.is-k8s-dashboard .chart-panel.panel-gauge .gauge-wrap {
  min-height: 230px;
}
</style>
