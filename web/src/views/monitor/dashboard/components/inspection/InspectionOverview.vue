<script setup>
import { computed, ref } from 'vue'
import { CircleCheck, Warning, DocumentChecked, Connection, ArrowRight } from '@element-plus/icons-vue'
import { useDashboardDomain } from '../../context'
import { formatReportTime } from '../../utils/format.js'
import { inspectionSourceSummaries, inspectionSourceState } from '../../utils/inspectionReport.js'

const { inspectionReport, inspectionReportResults, inspectionDatasourceFilter, inspectionFilter, selectedInspectionDatasourceIds } = useDashboardDomain('inspection')
const { activePanels } = useDashboardDomain('selection')
const expanded = ref(false)
const sources = computed(() => inspectionSourceSummaries(inspectionReport.value, inspectionReportResults.value))
const visibleSources = computed(() => expanded.value ? sources.value : sources.value.slice(0, 6))
const conclusion = computed(() => {
  const run = inspectionReport.value
  if (!run) return { key: 'uninspected', label: '准备好开始巡检', icon: DocumentChecked }
  if (run.dangerCount) return { key: 'danger', label: `${run.dangerCount} 项异常需要关注`, icon: Warning }
  if (run.warningCount) return { key: 'warning', label: `${run.warningCount} 项结果需要核查`, icon: Warning }
  if (!run.totalCount || run.healthyCount !== run.totalCount) return { key: 'uninspected', label: '巡检结果待确认', icon: DocumentChecked }
  return { key: 'healthy', label: '本次巡检全部正常', icon: CircleCheck }
})
const metrics = computed(() => [
  { key: 'total', label: '检查总数', value: inspectionReport.value?.totalCount || 0 },
  { key: 'healthy', label: '正常', value: inspectionReport.value?.healthyCount || 0 },
  { key: 'warning', label: '待核查', value: inspectionReport.value?.warningCount || 0 },
  { key: 'danger', label: '异常', value: inspectionReport.value?.dangerCount || 0 }
])
function selectSource(id) {
  inspectionDatasourceFilter.value = inspectionDatasourceFilter.value === id ? 'all' : id
  inspectionFilter.value = 'all'
}
function selectStatus(key) {
  inspectionDatasourceFilter.value = 'all'
  inspectionFilter.value = key === 'total' ? 'all' : key
}
</script>

<template>
  <section class="report-overview" aria-label="巡检报告总览">
    <div class="report-summary">
      <div class="report-conclusion" :class="`tone-${conclusion.key}`">
        <span class="conclusion-icon"><el-icon><component :is="conclusion.icon" /></el-icon></span>
        <div>
          <div class="report-caption">{{ inspectionReport ? '最近一次巡检' : '尚未生成报告' }}<span v-if="inspectionReport" class="report-number">#{{ inspectionReport.id }}</span></div>
          <h3>{{ conclusion.label }}</h3>
          <p v-if="inspectionReport">覆盖 {{ sources.length }} 个数据源<span class="dot">·</span>{{ formatReportTime(inspectionReport.completedAt) }} 完成<span class="dot">·</span>耗时 {{ inspectionReport.durationMs || 0 }} ms</p>
          <p v-else>已选 {{ selectedInspectionDatasourceIds.length }} 个数据源、{{ activePanels.length }} 项启用规则。执行后在这里查看结论与数据源分布。</p>
        </div>
      </div>
      <div v-if="inspectionReport" class="report-metrics" aria-label="报告统计">
        <button v-for="metric in metrics" :key="metric.key" :class="`metric tone-${metric.key}`" :aria-label="`查看${metric.label} ${metric.value} 项`" @click="selectStatus(metric.key)"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong></button>
      </div>
      <div v-else class="ready-count"><strong>{{ selectedInspectionDatasourceIds.length * activePanels.length }}</strong><span>项检查待执行</span></div>
    </div>
    <template v-if="inspectionReport">
      <div class="report-window"><el-icon><DocumentChecked /></el-icon><span>报告窗口</span><time>{{ formatReportTime(inspectionReport.startAt) }}<span class="range-separator">—</span>{{ formatReportTime(inspectionReport.endAt) }}</time><small>完整报告 · 不随下方筛选变化</small></div>
      <div class="source-section">
        <div class="source-heading"><h4>数据源概览 <span>{{ sources.length }}</span></h4><span>点击数据源查看对应结果</span></div>
        <div class="source-grid">
          <button v-for="source in visibleSources" :key="source.id" class="source-card" :class="[{ selected: inspectionDatasourceFilter === source.id }, `tone-${inspectionSourceState(source).key}`]" :aria-label="`查看数据源 ${source.name} 的巡检结果`" :aria-pressed="inspectionDatasourceFilter === source.id" @click="selectSource(source.id)">
            <div class="source-identity"><span class="source-icon"><el-icon><Connection /></el-icon></span><strong :title="source.name">{{ source.name }}</strong><el-icon class="source-arrow"><ArrowRight /></el-icon></div>
            <div class="source-state"><span><i></i>{{ inspectionSourceState(source).label }}</span><small>{{ source.total }} 项检查</small></div>
            <div class="source-distribution" aria-hidden="true"><i v-for="state in ['healthy', 'warning', 'danger', 'unknown']" :key="state" :class="`bar-${state}`" :style="{ flexGrow: source[state] }"></i></div>
            <div class="source-counts"><span>正常 <b>{{ source.healthy }}</b></span><span>待核查 <b>{{ source.warning }}</b></span><span>异常 <b>{{ source.danger }}</b></span><span v-if="source.unknown">未确认 <b>{{ source.unknown }}</b></span></div>
          </button>
        </div>
        <button v-if="sources.length > 6" class="show-sources" @click="expanded = !expanded">{{ expanded ? '收起数据源' : `展开全部 ${sources.length} 个数据源` }}</button>
      </div>
    </template>
  </section>
</template>

<style scoped>
.report-overview { margin-bottom: 16px; border: 1px solid var(--dash-border); border-radius: 14px; background: var(--dash-surface); box-shadow: var(--inspect-shadow); overflow: hidden; }
.report-summary { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 24px 26px; }
.report-conclusion { display: flex; align-items: flex-start; gap: 15px; min-width: 0; }
.conclusion-icon { display: grid; place-items: center; width: 46px; height: 46px; flex-shrink: 0; border: 1px solid var(--dash-border); border-radius: 12px; color: var(--status-color, var(--dash-accent)); background: var(--dash-subtle); font-size: 24px; }
.report-caption { display: flex; align-items: center; gap: 8px; color: var(--dash-muted); font-size: 11px; }
.report-number { padding: 1px 6px; border: 1px solid var(--dash-border); border-radius: 4px; font-family: Consolas, monospace; }
h3 { margin: 7px 0 7px; color: var(--dash-text); font-size: 20px; font-weight: 650; line-height: 1.4; }
p { margin: 0; color: var(--dash-muted); font-size: 12px; line-height: 1.8; }
.dot { margin: 0 7px; }
.report-metrics { display: grid; grid-template-columns: repeat(4, minmax(65px, 1fr)); min-width: 350px; }
.metric { display: flex; flex-direction: column; gap: 6px; padding: 7px 20px; border: 0; border-left: 1px solid var(--dash-border); background: transparent; text-align: left; cursor: pointer; }
.metric:hover { background: var(--dash-subtle); }
.metric span { color: var(--dash-muted); font-size: 12px; white-space: nowrap; }
.metric strong { color: var(--status-color, var(--dash-text)); font-size: 30px; font-weight: 600; line-height: 1.2; }
.tone-healthy { --status-color: var(--inspect-healthy); }
.tone-warning { --status-color: var(--inspect-warning); }
.tone-danger { --status-color: var(--inspect-danger); }
.ready-count { display: flex; flex-direction: column; gap: 4px; flex-shrink: 0; text-align: right; color: var(--dash-muted); font-size: 12px; }
.ready-count strong { color: var(--dash-accent); font-size: 36px; font-weight: 600; line-height: 1.2; }
.report-window { display: flex; align-items: center; flex-wrap: wrap; gap: 9px; padding: 11px 26px; border-top: 1px solid var(--dash-border); border-bottom: 1px solid var(--dash-border); background: var(--dash-subtle); color: var(--dash-muted); font-size: 11px; }
.report-window time { color: var(--dash-text); line-height: 1.7; }
.range-separator { padding: 0 10px; color: var(--dash-muted); }
.report-window small { margin-left: auto; color: var(--dash-muted); font-size: 11px; }
.source-section { padding: 18px 26px 22px; }
.source-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
h4 { margin: 0; font-size: 13px; font-weight: 600; }
h4 span { margin-left: 5px; color: var(--dash-muted); font-size: 11px; font-weight: 400; }
.source-heading > span { color: var(--dash-muted); font-size: 11px; }
.source-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 12px; }
.source-card { min-width: 0; padding: 14px 16px 12px; border: 1px solid var(--dash-border); border-radius: 10px; background: var(--dash-surface); color: var(--dash-text); text-align: left; cursor: pointer; transition: border-color .15s, background-color .15s; }
.source-card:hover, .source-card.selected { border-color: var(--dash-accent); background: var(--dash-subtle); }
.source-card.selected { box-shadow: 0 0 0 1px var(--dash-accent) inset; }
.source-identity { display: flex; align-items: center; gap: 9px; min-width: 0; }
.source-icon { display: grid; place-items: center; width: 28px; height: 28px; flex-shrink: 0; border-radius: 7px; background: var(--inspect-accent-soft); color: var(--dash-accent); font-size: 16px; }
.source-identity strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; }
.source-arrow { margin-left: auto; color: var(--dash-muted); font-size: 12px; }
.source-state { display: flex; justify-content: space-between; gap: 8px; margin-top: 13px; font-size: 11px; }
.source-state > span { display: flex; align-items: center; gap: 5px; color: var(--status-color, var(--dash-muted)); }
.source-state i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.source-state small { color: var(--dash-muted); font-size: 11px; }
.source-distribution { display: flex; gap: 3px; height: 4px; margin: 10px 0; border-radius: 4px; background: var(--dash-subtle); overflow: hidden; }
.source-distribution i { flex-basis: 0; }
.bar-healthy { background: var(--inspect-healthy); }.bar-warning { background: var(--inspect-warning); }.bar-danger { background: var(--inspect-danger); }.bar-unknown { background: var(--dash-muted); }
.source-counts { display: flex; gap: 16px; flex-wrap: wrap; color: var(--dash-muted); font-size: 10px; }
.source-counts b { margin-left: 3px; color: var(--dash-text); font-size: 11px; font-weight: 600; }
.show-sources { display: block; margin: 14px auto 0; padding: 6px 12px; border: 0; border-radius: 6px; background: var(--dash-subtle); color: var(--dash-accent); cursor: pointer; }
@media (max-width: 1100px) { .report-summary { align-items: stretch; flex-direction: column; gap: 20px; } .report-metrics { min-width: 0; } .metric:first-child { border-left: 0; padding-left: 0; } .ready-count { align-self: flex-end; } }
@media (max-width: 760px) { .report-summary { padding: 20px 16px; } .report-conclusion { gap: 10px; } .conclusion-icon { width: 36px; height: 36px; font-size: 20px; } h3 { font-size: 18px; } .source-section { padding: 16px; } .source-grid { grid-template-columns: minmax(0, 1fr); } .report-window { padding: 10px 16px; } .report-window small { margin-left: 0; flex-basis: 100%; } .metric { padding: 7px 10px; } .metric strong { font-size: 25px; } .report-window time { display: flex; flex-wrap: wrap; } }
</style>
