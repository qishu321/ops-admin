<script setup>
import { useDashboardDomain } from '../context'
import { formatReportTime, reducerLabel } from '../utils/format.js'
import { Warning } from '@element-plus/icons-vue'

const { inspectionReport, inspectionFilter, inspectionRows, inspectionSummary, inspectionPanels, inspectionRunning, executeInspection, inspectionStateType, inspectionState, inspectionDisplayValue, inspectionSeriesCount, inspectionResultFor, inspectionThresholdText } = useDashboardDomain('inspection')
const { loading, activePanels } = useDashboardDomain('selection')
const { openEditPanel, handleDeletePanel } = useDashboardDomain('panelEditor')
</script>

<template>
      <section class="inspection-list">
        <div v-if="inspectionReport" class="inspection-report-strip">
          <div><span>巡检报告</span><strong>#{{ inspectionReport.id }}</strong></div>
          <div><span>时间范围</span><strong>{{ formatReportTime(inspectionReport.startAt) }} 至 {{ formatReportTime(inspectionReport.endAt) }}</strong></div>
          <div><span>完成时间</span><strong>{{ formatReportTime(inspectionReport.completedAt) }}</strong></div>
          <div><span>耗时</span><strong>{{ inspectionReport.durationMs || 0 }} ms</strong></div>
        </div>
        <div v-else class="inspection-empty-report">
          <el-icon><Warning /></el-icon>
          <div><strong>尚未生成巡检报告</strong><span>选择数据源和时间范围后点击“执行巡检”，系统才会查询并保存本次结果。</span></div>
        </div>
        <div class="inspection-command-bar">
          <div class="inspection-filter">
            <button :class="{ active: inspectionFilter === 'all' }" @click="inspectionFilter = 'all'">全部 {{ inspectionRows.length }}</button>
            <button :class="{ active: inspectionFilter === 'danger' }" @click="inspectionFilter = 'danger'">异常 {{ inspectionSummary.danger }}</button>
            <button :class="{ active: inspectionFilter === 'warning' }" @click="inspectionFilter = 'warning'">待核查 {{ inspectionSummary.warning }}</button>
            <button :class="{ active: inspectionFilter === 'healthy' }" @click="inspectionFilter = 'healthy'">正常 {{ inspectionSummary.healthy }}</button>
            <button v-if="inspectionSummary.uninspected" :class="{ active: inspectionFilter === 'uninspected' }" @click="inspectionFilter = 'uninspected'">未巡检 {{ inspectionSummary.uninspected }}</button>
          </div>
          <div class="inspection-command-actions">
            <span>{{ inspectionReport ? `报告结果 · 当前展示 ${inspectionPanels.length} 项` : `规则清单 · ${inspectionPanels.length} 项等待巡检` }}</span>
            <el-button type="primary" :loading="inspectionRunning" @click="executeInspection" :disabled="!activePanels.length">{{ inspectionRunning ? '巡检执行中' : '执行巡检' }}</el-button>
          </div>
        </div>
        <div class="inspection-head">
          <div>
            <h3>巡检项目</h3>
            <p>结果由所选时间窗口内的样本聚合后判定；修改时间范围不会自动执行巡检。</p>
          </div>
        </div>
        <el-table v-loading="inspectionRunning" element-loading-text="正在按所选时间范围执行巡检…" :data="inspectionPanels" class="inspection-table" row-key="id" empty-text="当前筛选条件下暂无巡检项">
          <el-table-column label="巡检项" min-width="180">
            <template #default="{ row }">
              <div class="inspection-name">
                <strong>{{ row.title }}</strong>
                <span>{{ row.description || '未填写描述' }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag :type="inspectionStateType(row)" effect="light">{{ inspectionState(row) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="统计值" width="150">
            <template #default="{ row }">
              <strong class="inspection-value">{{ inspectionDisplayValue(row) }}</strong>
            </template>
          </el-table-column>
          <el-table-column label="返回序列" width="110">
            <template #default="{ row }">{{ inspectionSeriesCount(row) }}</template>
          </el-table-column>
          <el-table-column label="窗口统计 / 阈值" min-width="250">
            <template #default="{ row }">
              <div class="inspection-meta">
                <span>{{ row.category || '自定义' }} · {{ reducerLabel(inspectionResultFor(row)?.reducer || row.reducer) }}</span>
                <b>{{ inspectionThresholdText(row) }}</b>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="PromQL" min-width="320" show-overflow-tooltip>
            <template #default="{ row }">
              <code class="inspection-promql">{{ row.promql }}</code>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <template v-if="row.panelExists !== false">
                <el-button link type="primary" @click="openEditPanel(row)">编辑</el-button>
                <el-button link type="danger" @click="handleDeletePanel(row)">删除</el-button>
              </template>
              <span v-else class="inspection-snapshot-label">报告快照</span>
            </template>
          </el-table-column>
        </el-table>
      </section>
</template>

<style scoped>
.inspection-list {
  padding: 18px;
  border: 1px solid rgba(169, 190, 222, 0.72);
  border-radius: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(250, 253, 255, 0.92)),
    radial-gradient(circle at 90% 0%, rgba(37, 99, 235, 0.12), transparent 32%);
  box-shadow: 0 14px 32px rgba(31, 54, 92, 0.08);
}
.inspection-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.inspection-head h3 {
  margin: 0 0 6px;
  color: var(--dash-text);
  font-size: 20px;
}
.inspection-head p {
  margin: 0;
  color: var(--dash-muted);
}
.inspection-command-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  margin: -2px 0 18px;
  padding: 10px 12px;
  border: 1px solid var(--dash-border);
  border-radius: 12px;
  background: linear-gradient(100deg, var(--dash-subtle), var(--dash-subtle));
}
.inspection-command-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--dash-muted);
  font-size: 12px;
}
.inspection-filter {
  display: inline-flex;
  padding: 3px;
  border: 1px solid var(--dash-border);
  border-radius: 10px;
  background: var(--dash-subtle);
}
.inspection-filter button {
  padding: 6px 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--dash-muted);
  cursor: pointer;
  font-size: 12px;
}
.inspection-filter button:hover,
.inspection-filter button.active {
  background: var(--dash-surface);
  color: var(--dash-accent);
  box-shadow: 0 2px 6px rgba(36, 99, 212, 0.12);
}
.inspection-table {
  border-radius: 14px;
  overflow: hidden;
}
.inspection-name strong,
.inspection-name span,
.inspection-meta span,
.inspection-meta b {
  display: block;
}
.inspection-name strong {
  color: var(--dash-text);
}
.inspection-name span {
  margin-top: 4px;
  color: var(--dash-muted);
  font-size: 12px;
}
.inspection-value {
  color: var(--dash-accent);
  font-size: 18px;
}
.inspection-meta span {
  color: var(--dash-muted);
}
.inspection-meta b {
  margin-top: 3px;
  color: var(--dash-muted);
  font-size: 12px;
  font-weight: 600;
}
.inspection-promql {
  display: inline-block;
  max-width: 100%;
  padding: 5px 8px;
  border-radius: 8px;
  background: #0f172a;
  color: var(--dash-muted);
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
}
.inspection-snapshot-label { color: var(--dash-muted); font-size: 12px; }
.inspection-report-strip {
  display: grid;
  grid-template-columns: 120px minmax(360px, 1.5fr) minmax(230px, 1fr) 120px;
  gap: 1px;
  margin-bottom: 14px;
  overflow: hidden;
  border: 1px solid var(--dash-border);
  border-radius: 12px;
  background: #dce8f8;
}
.inspection-report-strip > div {
  min-width: 0;
  padding: 11px 14px;
  background: var(--dash-subtle);
}
.inspection-report-strip span,
.inspection-report-strip strong {
  display: block;
}
.inspection-report-strip span {
  color: var(--dash-muted);
  font-size: 11px;
}
.inspection-report-strip strong {
  margin-top: 4px;
  overflow: hidden;
  color: var(--dash-text);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.inspection-empty-report {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  padding: 13px 15px;
  border: 1px dashed var(--dash-border);
  border-radius: 12px;
  background: var(--dash-subtle);
  color: #2a66c8;
}
.inspection-empty-report .el-icon { font-size: 20px; }
.inspection-empty-report strong,
.inspection-empty-report span { display: block; }
.inspection-empty-report span { margin-top: 3px; color: var(--dash-muted); font-size: 12px; }
@media (max-width: 760px) {
  .inspection-command-bar {
    align-items: flex-start;
    flex-direction: column;
  }
}
@media (max-width: 760px) {
  .inspection-command-actions { justify-content: flex-start; }
}
</style>
