<script setup>
import { useDashboardDomain } from '../../context'
import { CircleCheck } from '@element-plus/icons-vue'

const { barRows } = useDashboardDomain('bars')
const { isK8sTopRankingPanel, isK8sDistributionPanel, isK8sDashboard } = useDashboardDomain('selection')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
<div v-if="barRows(panel).length" :class="['bar-chart', { 'k8s-top-ranking-list': isK8sTopRankingPanel(panel), 'k8s-distribution-list': isK8sDistributionPanel(panel) }]">
              <div v-for="(item, index) in barRows(panel)" :key="item.name" :class="['bar-row', { 'top-ranking-row': isK8sTopRankingPanel(panel) }]">
                <em v-if="isK8sTopRankingPanel(panel)">{{ index + 1 }}</em>
                <span :title="item.name">{{ item.name }}</span>
                <div v-if="!isK8sTopRankingPanel(panel)"><i :style="{ width: `${item.percent}%`, background: isK8sDashboard ? item.color : undefined }"></i></div>
                <b>{{ item.displayValue }}</b>
              </div>
            </div>
            <div v-else class="chart-empty-state">
              <el-icon><CircleCheck /></el-icon>
              <strong>{{ panel.title === '异常原因 Top' ? '当前时间范围内无异常' : '暂无可展示的序列' }}</strong>
              <span>{{ panel.title === '异常原因 Top' ? '未发现等待或失败的 Pod' : '调整时间范围或检查数据源后重试' }}</span>
            </div>
            <div class="panel-query" :title="panel.promql">{{ panel.promql }}</div>
</template>

<style scoped>
.bar-chart {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 12px;
}
.bar-row {
  display: grid;
  grid-template-columns: 160px 1fr 84px;
  align-items: center;
  gap: 10px;
  color: var(--dash-muted);
  font-size: 13px;
}
.bar-row span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bar-row div {
  height: 11px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(226, 235, 247, 0.9);
}
.bar-row i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #2563eb, #06b6d4 48%, #22c55e);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.18);
}
.bar-row b {
  color: var(--dash-text);
  text-align: right;
}
.chart-panel .bar-chart {
  margin: 14px;
  max-height: 184px;
  gap: 9px;
  overflow: auto;
}
.chart-panel .bar-row div {
  height: 7px;
  border-radius: 2px;
  background: var(--dash-subtle);
}
.chart-panel .bar-row i {
  border-radius: 2px;
  background: linear-gradient(90deg, #2563eb, #14b8a6);
  box-shadow: none;
}
.panel-query {
  display: none;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-row b {
  color: #e5edf6;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-row {
  color: #8493a7;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-row div {
  background: #263241;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-chart {
  max-height: 112px;
  margin: 8px 10px;
  gap: 5px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-row {
  grid-template-columns: minmax(70px, 112px) 1fr 58px;
  gap: 6px;
  font-size: 10px;
}
.observability-canvas:is(:fullscreen, .is-presentation) .chart-panel .bar-row div {
  height: 5px;
}
.is-k8s-dashboard .chart-panel .bar-chart {
  max-height: 252px;
  margin: 18px 16px;
  gap: 10px;
}
.is-k8s-dashboard .chart-panel .bar-row div {
  height: 9px;
  border-radius: 3px;
}
.is-k8s-dashboard .chart-panel .bar-row i {
  border-radius: 3px;
}
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
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-top-ranking-list {
  max-height: 178px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-top-ranking-list .top-ranking-row {
  grid-template-columns: 18px minmax(0, 1fr) auto;
  gap: 7px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-top-ranking-list .top-ranking-row em {
  color: #6f90bd;
  font-size: 10px;
  font-style: normal;
  font-variant-numeric: tabular-nums;
  text-align: center;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-top-ranking-list .top-ranking-row b {
  min-width: 54px;
  color: #dce8f7;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .k8s-distribution-list {
  flex: 1;
  max-height: none;
  justify-content: flex-start;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-panel.panel-bar .chart-empty-state {
  flex: 1;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state span {
  color: #9badc2;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state {
  color: #9badc2;
  background-image: repeating-linear-gradient(to bottom, transparent 0, transparent 48px, #263241 49px, transparent 50px);
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-panel.panel-bar .chart-empty-state {
  min-height: 118px;
  padding: 14px;
  background-image: none;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-panel.panel-bar .chart-empty-state .el-icon {
  margin-bottom: 5px;
  font-size: 22px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-panel.panel-bar .chart-empty-state span {
  margin-top: 3px;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .chart-empty-state strong {
  color: #d7e3f0;
}
.chart-panel .bar-row, .is-k8s-dashboard .chart-panel .bar-row { grid-template-columns: minmax(0, 1.2fr) minmax(24px, 1fr) auto; gap: 8px; }
.chart-panel .bar-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.k8s-top-ranking-list .bar-row { grid-template-columns: 20px minmax(0, 1fr) auto !important; }
.chart-panel .bar-row,
.is-k8s-dashboard .chart-panel .bar-row {
  grid-template-columns: minmax(0, 1.2fr) minmax(24px, 1fr) auto;
  gap: 8px;
  font-size: 12px;
}
</style>
