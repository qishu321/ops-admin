<script setup>
import { RefreshLeft } from '@element-plus/icons-vue'
import { useDashboardDomain } from '../../context'

const { inspectionFilter, inspectionScopedRows, inspectionSummary, inspectionDatasourceFilter, inspectionReportDatasources } = useDashboardDomain('inspection')
const { isDark } = useDashboardDomain('theme')
const { isFullscreen } = useDashboardDomain('fullscreen')
const states = [{ key: 'all', label: '全部' }, { key: 'danger', label: '异常' }, { key: 'warning', label: '待核查' }, { key: 'healthy', label: '正常' }, { key: 'uninspected', label: '未巡检' }]
function resetFilters() {
  inspectionFilter.value = 'all'
  inspectionDatasourceFilter.value = 'all'
}
</script>

<template>
  <div class="results-filters">
    <div class="status-tabs" aria-label="按巡检状态筛选">
      <template v-for="state in states" :key="state.key">
        <button v-if="state.key !== 'uninspected' || inspectionSummary.uninspected"
          :class="[{ active: inspectionFilter === state.key }, `tab-${state.key}`]"
          :aria-pressed="inspectionFilter === state.key" @click="inspectionFilter = state.key">
          <i v-if="state.key !== 'all'"></i>{{ state.label }}
          <b>{{ state.key === 'all' ? inspectionScopedRows.length : inspectionSummary[state.key] }}</b>
        </button>
      </template>
    </div>
    <div class="filter-options">
      <el-select v-if="inspectionReportDatasources.length > 1" v-model="inspectionDatasourceFilter"
        aria-label="筛选报告数据源" :popper-class="isDark || isFullscreen ? 'monitor-dark-popper' : ''"
        :teleported="!isFullscreen" class="source-select">
        <el-option label="全部数据源" value="all" />
        <el-option v-for="source in inspectionReportDatasources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-button v-if="inspectionFilter !== 'all' || inspectionDatasourceFilter !== 'all'"
        link :icon="RefreshLeft" @click="resetFilters">重置筛选</el-button>
    </div>
  </div>
</template>

<style scoped>
.results-filters { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; padding: 0 24px 16px; }
.status-tabs { display: flex; align-items: center; flex-wrap: wrap; gap: 3px; }
.status-tabs button { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border: 1px solid transparent; border-radius: 7px; background: transparent; color: var(--dash-muted); font-size: 12px; cursor: pointer; }
.status-tabs button:hover { background: var(--dash-subtle); }
.status-tabs button.active { background: var(--inspect-accent-soft); color: var(--dash-accent); }
.status-tabs i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.tab-danger i { color: var(--inspect-danger); }.tab-warning i { color: var(--inspect-warning); }.tab-healthy i { color: var(--inspect-healthy); }
.status-tabs b { font-weight: 500; font-size: 11px; }
.filter-options { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.source-select { width: 230px; }
@media (max-width: 760px) {
  .results-filters { padding: 0 12px 14px; }
  .status-tabs { gap: 0; }
  .status-tabs button { padding: 7px 8px; }
  .filter-options { padding: 0 4px; max-width: 100%; }
  .source-select { max-width: 100%; }
}
</style>
