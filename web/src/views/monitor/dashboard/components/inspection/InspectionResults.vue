<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useDashboardDomain } from '../../context'
import { reducerLabel } from '../../utils/format.js'
import { Search } from '@element-plus/icons-vue'
import InspectionResultCard from './InspectionResultCard.vue'
import InspectionResultFilters from './InspectionResultFilters.vue'

const { inspectionReport, inspectionFilter, inspectionScopedRows, inspectionPanels, inspectionRunning, inspectionStateKey, inspectionState, inspectionDisplayValue, inspectionSeriesCount, inspectionResultFor, inspectionThresholdText, inspectionDatasourceFilter } = useDashboardDomain('inspection')
const { openEditPanel, handleDeletePanel } = useDashboardDomain('panelEditor')
defineProps({ embedded: Boolean })
function resetFilters() { inspectionFilter.value = 'all'; inspectionDatasourceFilter.value = 'all' }
// Mount just one results renderer, so large reports do not duplicate every row.
const compact = ref(false)
let media
function updateLayout() { compact.value = media.matches }
onMounted(() => {
  media = window.matchMedia('(max-width: 760px)')
  updateLayout()
  media.addEventListener('change', updateLayout)
})
onBeforeUnmount(() => media?.removeEventListener('change', updateLayout))
</script>

<template>
  <section class="inspection-results" :class="{ 'is-embedded': embedded }" aria-label="巡检结果明细">
    <header v-if="!embedded" class="results-heading"><div><h3>{{ inspectionReport ? '巡检结果' : '巡检规则' }} <span>{{ inspectionPanels.length }}</span></h3><p>{{ inspectionReport ? '异常优先展示，展开行查看查询语句与错误详情。' : '执行巡检后，每个数据源将分别生成以下规则的检查结果。' }}</p></div><span v-if="inspectionReport" class="snapshot-note">报告快照 #{{ inspectionReport.id }}</span></header>
    <InspectionResultFilters />
    <div v-if="compact" v-loading="inspectionRunning" element-loading-text="正在执行巡检…" class="compact-results">
      <InspectionResultCard v-for="row in inspectionPanels" :key="row.rowKey" :row="row" />
      <div v-if="!inspectionPanels.length" class="results-empty"><el-icon><Search /></el-icon><strong>没有符合筛选条件的结果</strong><span>试试其他状态或数据源</span><el-button link type="primary" @click="resetFilters">查看全部巡检项</el-button></div>
    </div>
    <el-table v-else v-loading="inspectionRunning" element-loading-text="正在按所选数据源和时间范围执行巡检…" :data="inspectionPanels" class="inspection-table" row-key="rowKey">
      <el-table-column type="expand" width="42">
        <template #default="{ row }"><div class="query-details"><div class="query-label">PromQL <span>本次检查使用的查询语句</span></div><pre>{{ row.promql || '尚未配置查询语句' }}</pre><div v-if="inspectionResultFor(row)?.error" class="query-error"><strong>查询失败原因</strong><p>{{ inspectionResultFor(row).error }}</p></div><div v-else class="query-evidence">返回 {{ inspectionSeriesCount(row) }} 条序列<span>·</span>{{ reducerLabel(inspectionResultFor(row)?.reducer || row.reducer) }}<span>·</span>{{ inspectionThresholdText(row) }}</div></div></template>
      </el-table-column>
      <el-table-column label="巡检项" min-width="210"><template #default="{ row }"><div class="rule-name"><strong>{{ row.title }}</strong><span>{{ row.category || '自定义规则' }}<template v-if="row.description"> · {{ row.description }}</template></span></div></template></el-table-column>
      <el-table-column label="数据源" min-width="185" show-overflow-tooltip><template #default="{ row }"><span class="row-source">{{ row.reportResult ? row.datasourceName || `数据源 #${row.datasourceId}` : '按所选数据源执行' }}</span></template></el-table-column>
      <el-table-column label="状态" width="110"><template #default="{ row }"><span class="result-status" :class="`state-${inspectionStateKey(row)}`"><i></i>{{ inspectionState(row) }}</span></template></el-table-column>
      <el-table-column label="统计值 / 序列" min-width="140"><template #default="{ row }"><div class="result-value" :class="`state-${inspectionStateKey(row)}`"><strong>{{ inspectionDisplayValue(row) }}</strong><span>{{ row.reportResult ? `返回 ${inspectionSeriesCount(row)} 条序列` : '等待执行' }}</span></div></template></el-table-column>
      <el-table-column label="判定规则" min-width="230"><template #default="{ row }"><div class="rule-threshold"><span>{{ reducerLabel(inspectionResultFor(row)?.reducer || row.reducer) }}</span><small>{{ inspectionThresholdText(row) }}</small></div></template></el-table-column>
      <el-table-column label="操作" width="110" fixed="right"><template #default="{ row }"><template v-if="row.panelExists !== false"><el-button link type="primary" @click="openEditPanel(row.panelDefinition || row)">编辑</el-button><el-button link type="danger" @click="handleDeletePanel(row.panelDefinition || row)">删除</el-button></template><span v-else class="snapshot-note">报告快照</span></template></el-table-column>
      <template #empty><div class="results-empty"><el-icon><Search /></el-icon><strong>没有符合筛选条件的结果</strong><span>试试其他状态或数据源</span><el-button link type="primary" @click="resetFilters">查看全部巡检项</el-button></div></template>
    </el-table>
    <footer class="results-footer"><span>共 {{ inspectionScopedRows.length }} 项<span class="footer-separator">/</span>当前显示 {{ inspectionPanels.length }} 项</span><span>{{ inspectionReport ? '修改规则不影响已保存的报告；重新执行后生成新结果。' : '仅启用的规则会参与巡检。' }}</span></footer>
  </section>
</template>

<style scoped>
.inspection-results { min-width: 0; border: 1px solid var(--dash-border); border-radius: 14px; background: var(--dash-surface); box-shadow: var(--inspect-shadow); overflow: hidden; }
.inspection-results.is-embedded { border: 0; border-radius: 0; box-shadow: none; }
.results-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 22px 26px 18px; }
h3 { display: flex; align-items: center; gap: 9px; margin: 0; color: var(--dash-text); font-size: 16px; font-weight: 650; }
h3 span { padding: 2px 7px; border-radius: 5px; background: var(--dash-subtle); color: var(--dash-muted); font-size: 11px; font-weight: 500; }
p { margin: 7px 0 0; color: var(--dash-muted); font-size: 12px; line-height: 1.6; }
.snapshot-note { color: var(--dash-muted); font-size: 11px; white-space: nowrap; }
.inspection-table { width: 100%; color: var(--dash-text); border-top: 1px solid var(--dash-border); }
.inspection-table :deep(th.el-table__cell) { height: 44px; padding: 8px 0; font-size: 11px; font-weight: 500; }
.inspection-table :deep(td.el-table__cell) { padding: 17px 0; }
.inspection-table :deep(.cell) { line-height: 1.6; }
.inspection-table :deep(.el-table__expanded-cell) { padding: 0; }
.inspection-table :deep(.el-table__expand-icon) { color: var(--dash-muted); }
.compact-results { border-top: 1px solid var(--dash-border); }
.rule-name, .result-value, .rule-threshold { display: flex; flex-direction: column; gap: 4px; }
.rule-name strong { color: var(--dash-text); font-size: 13px; font-weight: 600; }
.rule-name > span, .result-value > span { color: var(--dash-muted); font-size: 11px; }
.row-source { color: var(--dash-text); font-size: 12px; }
.result-status { display: inline-flex; align-items: center; gap: 6px; padding: 3px 8px; border: 1px solid var(--dash-border); border-radius: 6px; color: var(--state-color, var(--dash-muted)); font-size: 11px; white-space: nowrap; }
.state-healthy { --state-color: var(--inspect-healthy); }.state-warning { --state-color: var(--inspect-warning); }.state-danger { --state-color: var(--inspect-danger); }
.result-value strong { color: var(--state-color, var(--dash-muted)); font-size: 19px; font-weight: 600; line-height: 1.35; }
.rule-threshold > span { color: var(--dash-text); font-size: 12px; }
.rule-threshold small { color: var(--dash-muted); font-size: 11px; }
.query-details { margin: 0; padding: 20px 26px 20px 42px; background: var(--dash-subtle); border-bottom: 1px solid var(--dash-border); }
.query-label { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; color: var(--dash-text); font-size: 12px; font-weight: 600; }
.query-label span { color: var(--dash-muted); font-size: 11px; font-weight: 400; }
pre { margin: 10px 0; padding: 13px 16px; border: 1px solid var(--dash-border); border-radius: 8px; background: var(--dash-surface); color: var(--dash-text); font: 12px/1.8 Consolas, "SFMono-Regular", monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.query-evidence { color: var(--dash-muted); font-size: 11px; }
.query-evidence > span { padding: 0 9px; }
.query-error { color: var(--inspect-danger); font-size: 12px; }
.query-error p { color: inherit; overflow-wrap: anywhere; }
.results-empty { display: flex; align-items: center; flex-direction: column; gap: 10px; padding: 36px 16px; line-height: 1.4; }
.results-empty .el-icon { color: var(--dash-muted); font-size: 27px; }
.results-empty strong { color: var(--dash-text); font-size: 13px; font-weight: 500; }.results-empty > span { color: var(--dash-muted); font-size: 12px; }
.results-footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; padding: 14px 26px; color: var(--dash-muted); font-size: 11px; line-height: 1.5; }
.footer-separator { padding: 0 8px; opacity: .6; }
@media (max-width: 760px) { .results-heading { padding: 18px 16px; align-items: flex-start; flex-direction: column; } .results-footer { padding: 12px 16px; } .query-details { padding: 16px; } }
</style>
