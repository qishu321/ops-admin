<script setup>
import { useDashboardDomain } from '../../context'
import { reducerLabel } from '../../utils/format.js'

defineProps({ row: { type: Object, required: true } })
const { inspectionStateKey, inspectionState, inspectionDisplayValue, inspectionSeriesCount, inspectionResultFor, inspectionThresholdText } = useDashboardDomain('inspection')
const { openEditPanel, handleDeletePanel } = useDashboardDomain('panelEditor')
</script>

<template>
  <article class="result-card" :class="`state-${inspectionStateKey(row)}`">
    <header>
      <div><h4>{{ row.title }}</h4><p>{{ row.reportResult ? row.datasourceName || `数据源 #${row.datasourceId}` : '按所选数据源执行' }}</p></div>
      <span class="card-status"><i></i>{{ inspectionState(row) }}</span>
    </header>
    <div class="card-evidence">
      <div class="card-value"><strong>{{ inspectionDisplayValue(row) }}</strong><span>{{ row.reportResult ? `返回 ${inspectionSeriesCount(row)} 条序列` : '等待执行' }}</span></div>
      <div class="card-threshold"><span>{{ reducerLabel(inspectionResultFor(row)?.reducer || row.reducer) }}</span><small>{{ inspectionThresholdText(row) }}</small></div>
    </div>
    <details>
      <summary>查询与判定详情</summary>
      <pre>{{ row.promql || '尚未配置查询语句' }}</pre>
      <p v-if="inspectionResultFor(row)?.error" class="card-error">{{ inspectionResultFor(row).error }}</p>
      <p v-if="row.description" class="rule-description">{{ row.description }}</p>
    </details>
    <footer v-if="row.panelExists !== false"><el-button link type="primary" @click="openEditPanel(row.panelDefinition || row)">编辑规则</el-button><el-button link type="danger" @click="handleDeletePanel(row.panelDefinition || row)">删除规则</el-button></footer>
    <footer v-else class="snapshot-label">报告快照</footer>
  </article>
</template>

<style scoped>
.result-card { padding: 18px 16px 12px; border-bottom: 1px solid var(--dash-border); color: var(--dash-text); }
.result-card:last-child { border-bottom: 0; }
.state-healthy { --state-color: var(--inspect-healthy); }.state-warning { --state-color: var(--inspect-warning); }.state-danger { --state-color: var(--inspect-danger); }
header { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
header > div { min-width: 0; }
h4 { margin: 0; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
p { margin: 6px 0 0; color: var(--dash-muted); font-size: 11px; line-height: 1.7; overflow-wrap: anywhere; }
.card-status { display: inline-flex; align-items: center; gap: 5px; flex-shrink: 0; padding: 3px 6px; border: 1px solid var(--dash-border); border-radius: 6px; color: var(--state-color, var(--dash-muted)); font-size: 11px; }
.card-status i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.card-evidence { display: grid; grid-template-columns: minmax(90px, .7fr) minmax(0, 1.3fr); gap: 14px; margin: 18px 0; }
.card-value, .card-threshold { display: flex; flex-direction: column; gap: 5px; }
.card-value strong { color: var(--state-color, var(--dash-muted)); font-size: 25px; font-weight: 600; line-height: 1.3; }
.card-value > span, .card-threshold small { color: var(--dash-muted); font-size: 11px; line-height: 1.7; }
.card-threshold { padding-left: 14px; border-left: 1px solid var(--dash-border); font-size: 12px; }
details { border-top: 1px solid var(--dash-border); padding-top: 11px; }
summary { width: fit-content; color: var(--dash-muted); font-size: 11px; cursor: pointer; }
summary:focus-visible { outline: 2px solid var(--dash-accent); outline-offset: 3px; }
pre { margin: 10px 0; padding: 10px 12px; border-radius: 7px; background: var(--dash-subtle); font: 11px/1.8 Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.card-error { color: var(--inspect-danger); }
footer { display: flex; justify-content: flex-end; gap: 12px; padding-top: 10px; }
footer .el-button { margin-left: 0; font-size: 11px; }
.snapshot-label { color: var(--dash-muted); font-size: 11px; }
</style>
