<script setup>
import { useDashboardDomain } from '../../context'

defineProps({ rows: { type: Array, required: true }, sources: { type: Array, required: true } })
defineEmits(['inspect'])
const { inspectionState, inspectionDisplayValue } = useDashboardDomain('inspection')
</script>

<template>
  <div class="matrix-scroll" tabindex="0" role="region" aria-label="巡检项与数据源状态矩阵">
    <table class="inspection-matrix" :style="{ minWidth: `${200 + sources.length * 185}px` }">
      <colgroup><col class="rule-column" /><col v-for="source in sources" :key="source.id" /></colgroup>
      <thead><tr><th scope="col"><span>巡检项</span><small>{{ rows.length }} 项规则</small></th>
        <th v-for="source in sources" :key="source.id" scope="col"><strong :title="source.name">{{ source.name }}</strong><small>统计值 / 状态</small></th>
      </tr></thead>
      <tbody>
        <tr v-for="rule in rows" :key="rule.id">
          <th scope="row"><strong :title="rule.title">{{ rule.title }}</strong><small :title="rule.category">{{ rule.category || '自定义规则' }}</small></th>
          <td v-for="cell in rule.cells" :key="cell.key">
            <button class="matrix-cell" :class="[`cell-${cell.state}`, { 'is-filtered': !cell.matches }]" :disabled="!cell.matches"
              :aria-label="`${rule.title} · ${cell.row.datasourceName} · ${cell.matches ? inspectionDisplayValue(cell.row) + ' · ' + inspectionState(cell.row) : '已筛选'}`"
              @click="$emit('inspect', cell.row)">
              <template v-if="cell.matches">
                <strong :title="inspectionDisplayValue(cell.row)">{{ inspectionDisplayValue(cell.row) }}</strong>
                <span><i></i>{{ inspectionState(cell.row) }}</span>
                <small v-if="cell.row.reportResult?.error">查询失败</small>
                <small v-else-if="cell.missing">未保存结果</small>
              </template>
              <span v-else class="filtered-label">已筛选</span>
            </button>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-if="!rows.length" class="matrix-empty"><strong>没有符合筛选条件的巡检结果</strong><span>切换状态或点击“重置筛选”查看全部结果。</span></div>
  </div>
</template>

<style scoped>
.matrix-scroll { min-width: 0; min-height: 0; overflow: auto; overscroll-behavior: contain; border-top: 1px solid var(--dash-border); scrollbar-color: var(--dash-border) transparent; }
.matrix-scroll:focus-visible { outline: 2px solid var(--dash-accent); outline-offset: -2px; }
.inspection-matrix { width: 100%; border-spacing: 0; table-layout: fixed; font-variant-numeric: tabular-nums; }
.rule-column { width: 200px; }
th { background: var(--dash-surface); color: var(--dash-text); text-align: left; }
thead th { position: sticky; top: 0; z-index: 2; height: 54px; padding: 10px 16px; border-bottom: 1px solid var(--dash-border); background: var(--dash-subtle); box-sizing: border-box; }
thead th:first-child { z-index: 3; }
th:first-child { position: sticky; left: 0; border-right: 1px solid var(--dash-border); }
tbody th { z-index: 1; padding: 7px 16px; border-bottom: 1px solid var(--dash-border); }
th strong, th small, thead th > span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
th strong { font-size: 12px; font-weight: 600; }
th small { margin-top: 4px; color: var(--dash-muted); font-size: 10px; font-weight: 400; }
thead th > span { font-size: 12px; font-weight: 600; }
td { padding: 4px 6px; border-bottom: 1px solid var(--dash-border); }
.matrix-cell { display: flex; align-items: center; flex-wrap: wrap; gap: 7px 12px; width: 100%; min-height: 43px; padding: 7px 12px; border: 1px solid transparent; border-radius: 7px; background: var(--dash-subtle); color: var(--dash-text); text-align: left; cursor: pointer; }
.matrix-cell:hover { border-color: var(--state-color, var(--dash-accent)); }
.matrix-cell > strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 20px; font-weight: 600; line-height: 1.25; }
.matrix-cell > span { display: inline-flex; align-items: center; gap: 5px; color: var(--state-color, var(--dash-muted)); font-size: 11px; white-space: nowrap; }
.matrix-cell i { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.matrix-cell > small { color: var(--state-color, var(--dash-muted)); font-size: 10px; }
.cell-healthy { --state-color: var(--inspect-healthy); background: var(--inspect-cell-healthy); }
.cell-warning { --state-color: var(--inspect-warning); background: var(--inspect-cell-warning); }
.cell-danger { --state-color: var(--inspect-danger); background: var(--inspect-cell-danger); }
.cell-warning > strong, .cell-danger > strong { color: var(--state-color); }
.cell-uninspected, .cell-disabled { color: var(--dash-muted); }
.matrix-cell.is-filtered { background: transparent; cursor: default; border-color: transparent; }
.matrix-cell .filtered-label { color: var(--dash-muted); opacity: .65; }
.matrix-empty { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 45px 20px; color: var(--dash-muted); font-size: 12px; }
.matrix-empty strong { color: var(--dash-text); font-size: 14px; font-weight: 500; }
@media (max-width: 760px) { .rule-column { width: 160px; } th:first-child { max-width: 160px; } .matrix-cell { min-height: 47px; } }
</style>
