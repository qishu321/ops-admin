<script setup>
import { computed } from 'vue'
import { useDashboardDomain } from '../../context'
import { buildInspectionMatrix } from '../../utils/inspectionMatrix.js'
import InspectionAttention from './InspectionAttention.vue'
import InspectionResultFilters from './InspectionResultFilters.vue'
import InspectionMatrix from './InspectionMatrix.vue'

defineEmits(['inspect'])
const { inspectionReport, inspectionRows, inspectionReportDatasources, inspectionScopedRows, inspectionDatasourceFilter, inspectionFilter, inspectionStateKey, inspectionRunning } = useDashboardDomain('inspection')
const { isFullscreen } = useDashboardDomain('fullscreen')
const matrix = computed(() => buildInspectionMatrix(inspectionRows.value, inspectionReportDatasources.value, {
  datasourceId: inspectionDatasourceFilter.value, status: inspectionFilter.value,
}))
const attention = computed(() => inspectionScopedRows.value.filter(row => ['danger', 'warning'].includes(inspectionStateKey(row)))
  .slice().sort((a, b) => Number(inspectionStateKey(b) === 'danger') - Number(inspectionStateKey(a) === 'danger')))
const metrics = computed(() => [
  { key: 'all', label: '检查总数', value: inspectionReport.value?.totalCount || 0 },
  { key: 'healthy', label: '正常', value: inspectionReport.value?.healthyCount || 0 },
  { key: 'warning', label: '待核查', value: inspectionReport.value?.warningCount || 0 },
  { key: 'danger', label: '异常', value: inspectionReport.value?.dangerCount || 0 },
])
function selectStatus(key) {
  inspectionFilter.value = key
  inspectionDatasourceFilter.value = 'all'
}
</script>

<template>
  <div class="inspection-board" v-loading="inspectionRunning" element-loading-text="正在生成新的巡检报告…">
    <div v-if="isFullscreen" class="screen-totals" aria-label="完整报告统计">
      <button v-for="metric in metrics" :key="metric.key" :class="`total-${metric.key}`"
        :aria-label="`查看${metric.label} ${metric.value} 项`" @click="selectStatus(metric.key)">
        <span>{{ metric.label }}</span><strong>{{ metric.value }}</strong>
      </button>
    </div>
    <InspectionAttention :rows="attention" @inspect="$emit('inspect', $event)" />
    <InspectionResultFilters />
    <div class="matrix-caption"><span>{{ matrix.rows.length }} 项规则<span class="caption-separator">×</span>{{ matrix.sources.length }} 个数据源</span><span>点击数值查看详情<span class="caption-separator">·</span>淡色文字表示已筛选</span></div>
    <InspectionMatrix :rows="matrix.rows" :sources="matrix.sources" @inspect="$emit('inspect', $event)" />
    <footer><span>匹配 {{ matrix.resultCount }} 条已保存结果</span><span>状态与数值来自本次报告快照，筛选不会重新执行巡检。</span></footer>
  </div>
</template>

<style scoped>
.inspection-board { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
.screen-totals { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin: 0 24px 18px; }
.screen-totals button { display: flex; align-items: center; gap: 18px; padding: 12px 18px; border: 1px solid var(--dash-border); border-radius: 9px; background: var(--dash-subtle); text-align: left; cursor: pointer; }
.screen-totals button:hover { border-color: var(--dash-accent); }
.screen-totals span { color: var(--dash-muted); font-size: 12px; }
.screen-totals strong { margin-left: auto; color: var(--dash-text); font-size: 30px; font-weight: 600; line-height: 1.15; }
.total-healthy strong { color: var(--inspect-healthy); }.total-warning strong { color: var(--inspect-warning); }.total-danger strong { color: var(--inspect-danger); }
.matrix-caption, footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; padding: 0 24px 11px; color: var(--dash-muted); font-size: 11px; }
.caption-separator { margin: 0 8px; opacity: .7; }
footer { padding: 12px 24px; border-top: 1px solid var(--dash-border); }
.inspection-board :deep(.matrix-scroll) { max-height: 76vh; }
@media (max-width: 760px) {
  .matrix-caption, footer { padding-right: 16px; padding-left: 16px; }
  .screen-totals { margin: 0 16px 16px; gap: 7px; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .screen-totals button { padding: 10px; gap: 6px; }
  .screen-totals strong { font-size: 25px; }
}
</style>
