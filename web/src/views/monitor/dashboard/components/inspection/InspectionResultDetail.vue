<script setup>
import { useDashboardDomain } from '../../context'
import { formatReportTime, reducerLabel } from '../../utils/format.js'

defineProps({ row: { type: Object, default: null } })
defineEmits(['close'])
const { inspectionReport, inspectionState, inspectionStateKey, inspectionDisplayValue, inspectionSeriesCount, inspectionThresholdText } = useDashboardDomain('inspection')
</script>

<template>
  <div class="inspection-detail-host">
    <el-drawer :model-value="!!row" title="巡检结果详情" size="min(520px, 95vw)" :append-to-body="false"
      destroy-on-close @close="$emit('close')">
      <template v-if="row">
        <span class="detail-kicker">报告 #{{ inspectionReport?.id }} · {{ row.snapshotMissing || !row.reportResult ? '未保存结果' : '已保存结果' }}</span>
        <h3>{{ row.title }}</h3>
        <p class="detail-source">{{ row.datasourceName || '—' }}</p>
        <div class="detail-result" :class="`state-${inspectionStateKey(row)}`"><strong>{{ inspectionDisplayValue(row) }}</strong><span>{{ inspectionState(row) }}</span></div>
        <el-alert v-if="row.snapshotMissing || !row.reportResult" title="该巡检项在本次报告中没有已保存的结果。重新执行巡检后生成新结果。" type="info" :closable="false" show-icon />
        <div v-if="row.reportResult?.error" class="detail-error"><strong>查询失败原因</strong><p>{{ row.reportResult.error }}</p></div>
        <dl>
          <div><dt>报告完成时间</dt><dd>{{ formatReportTime(inspectionReport?.completedAt) }}</dd></div>
          <div><dt>查询窗口</dt><dd>{{ formatReportTime(inspectionReport?.startAt) }}<br />至 {{ formatReportTime(inspectionReport?.endAt) }}</dd></div>
          <div><dt>返回序列</dt><dd>{{ inspectionSeriesCount(row) }}<span v-if="row.reportResult?.seriesCount === 0 && !row.reportResult.error"> · 无返回序列</span></dd></div>
          <div><dt>窗口统计</dt><dd>{{ reducerLabel(row.reducer) }}</dd></div>
          <div><dt>判定阈值</dt><dd>{{ inspectionThresholdText(row) }}</dd></div>
          <div><dt>分类</dt><dd>{{ row.category || '自定义规则' }}</dd></div>
        </dl>
        <h4>PromQL</h4><pre>{{ row.promql || '尚未配置查询语句' }}</pre>
        <p v-if="row.description" class="detail-description">{{ row.description }}</p>
      </template>
      <template #footer><el-button @click="$emit('close')">关闭详情</el-button></template>
    </el-drawer>
  </div>
</template>

<style scoped>
.inspection-detail-host :deep(.el-drawer) { background: var(--dash-surface); color: var(--dash-text); }
.inspection-detail-host :deep(.el-drawer__header) { margin-bottom: 0; padding: 20px; border-bottom: 1px solid var(--dash-border); color: var(--dash-text); }
.inspection-detail-host :deep(.el-drawer__title) { color: var(--dash-text); font-size: 15px; font-weight: 600; }
.inspection-detail-host :deep(.el-drawer__body) { padding: 24px; }
.inspection-detail-host :deep(.el-drawer__footer) { padding: 16px 24px; border-top: 1px solid var(--dash-border); }
.detail-kicker { color: var(--dash-muted); font-size: 11px; }
h3 { margin: 12px 0 7px; font-size: 23px; line-height: 1.4; overflow-wrap: anywhere; }
.detail-source { margin: 0; color: var(--dash-muted); font-size: 13px; overflow-wrap: anywhere; }
.detail-result { display: flex; align-items: center; gap: 14px; margin: 22px 0; padding: 20px; border: 1px solid var(--dash-border); border-radius: 10px; background: var(--dash-subtle); color: var(--result-color, var(--dash-muted)); }
.detail-result strong { font-size: 34px; font-weight: 600; overflow-wrap: anywhere; }
.detail-result span { margin-left: auto; white-space: nowrap; font-size: 12px; }
.state-healthy { --result-color: var(--inspect-healthy); }.state-warning { --result-color: var(--inspect-warning); }.state-danger { --result-color: var(--inspect-danger); }
dl { margin: 22px 0; }
dl > div { display: grid; grid-template-columns: 95px minmax(0, 1fr); gap: 14px; padding: 12px 0; border-bottom: 1px solid var(--dash-border); font-size: 12px; line-height: 1.8; }
dt { color: var(--dash-muted); } dd { margin: 0; color: var(--dash-text); overflow-wrap: anywhere; }
h4 { margin: 22px 0 10px; font-size: 12px; font-weight: 600; }
pre { margin: 0; padding: 14px; border: 1px solid var(--dash-border); border-radius: 8px; background: var(--dash-subtle); color: var(--dash-text); font: 12px/1.8 Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.detail-error { padding: 14px; border-radius: 8px; background: var(--inspect-cell-danger); color: var(--inspect-danger); font-size: 12px; overflow-wrap: anywhere; }
.detail-error p { margin: 8px 0 0; line-height: 1.7; }
.detail-description { color: var(--dash-muted); font-size: 12px; line-height: 1.8; }
</style>
