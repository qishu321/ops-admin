<script setup>
import { useDashboardDomain } from '../context'
import PanelFeedback from './PanelFeedback.vue'
import TableChart from './charts/TableChart.vue'
import BarChart from './charts/BarChart.vue'
import GaugeChart from './charts/GaugeChart.vue'
import TrendChart from './charts/TrendChart.vue'
import StatChart from './charts/StatChart.vue'
defineProps({ panel: { type: Object, required: true } })
const { panelVisualType } = useDashboardDomain('selection')
const { panelResults, refreshPanel } = useDashboardDomain('queries')
const { panelRows } = useDashboardDomain('display')
const charts = { table: TableChart, bar: BarChart, gauge: GaugeChart, line: TrendChart, stat: StatChart }
</script>
<template>
  <!-- The chart sits in a flex column so a panel stretched during layout editing
       makes the plot grow instead of leaving dead space under it. -->
  <div class="panel-body">
    <PanelFeedback v-if="panel.status === 1 && !panelResults[panel.id]" loading />
    <PanelFeedback v-else-if="panelResults[panel.id]?.error" :error="panelResults[panel.id].error" :disabled="panel.status !== 1" @retry="refreshPanel(panel)" />
    <PanelFeedback v-else-if="!panelRows(panel).length && ['stat', 'gauge'].includes(panelVisualType(panel))" :disabled="panel.status !== 1" />
    <component v-else :is="charts[panelVisualType(panel)] || StatChart" :panel="panel" />
  </div>
</template>
<style scoped>
.panel-body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
}
/* Skeleton and empty/error states share the card height instead of hugging the
   header; PanelFeedback already centres its own content. */
.panel-body > :deep(.panel-feedback),
.panel-body > :deep(.panel-skeleton) {
  flex: 1 1 auto;
}
</style>
