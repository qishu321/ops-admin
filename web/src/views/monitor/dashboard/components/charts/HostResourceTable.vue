<script setup>
import { useDashboardDomain } from '../../context'
import { formatPodResourceValue } from '../../utils/format.js'

const { hostDetailRows } = useDashboardDomain('display')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
  <div class="host-resource-toolbar">
    <strong>主机资源明细</strong>
    <span>可按 CPU、内存、磁盘、负载和网络指标排序</span>
  </div>
  <div class="host-resource-table-body">
    <el-table
      class="host-resource-table"
      :data="hostDetailRows(panel)"
      size="small"
      width="100%"
      height="100%"
      :default-sort="{ prop: 'memoryUsagePercent', order: 'descending' }"
    >
      <el-table-column prop="instance" label="主机" min-width="180" align="center" header-align="center" show-overflow-tooltip />
      <el-table-column prop="os" label="操作系统" min-width="180" align="center" header-align="center" show-overflow-tooltip />
      <el-table-column prop="cpuUsagePercent" label="CPU 使用率" min-width="110" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.cpuUsagePercent, '%') }}</template>
      </el-table-column>
      <el-table-column prop="memoryUsagePercent" label="内存使用率" min-width="110" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.memoryUsagePercent, '%') }}</template>
      </el-table-column>
      <el-table-column prop="diskUsagePercent" label="磁盘使用率" min-width="110" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.diskUsagePercent, '%') }}</template>
      </el-table-column>
      <el-table-column prop="load5" label="5 分钟负载" min-width="105" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.load5, '') }}</template>
      </el-table-column>
      <el-table-column prop="networkReceive" label="网络流入" min-width="110" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.networkReceive, 'B/s') }}</template>
      </el-table-column>
      <el-table-column prop="networkTransmit" label="网络流出" min-width="110" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.networkTransmit, 'B/s') }}</template>
      </el-table-column>
      <el-table-column prop="uptimeDays" label="运行时长" min-width="100" align="center" header-align="center" sortable>
        <template #default="{ row }">{{ formatPodResourceValue(row.uptimeDays, '天') }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped src="../../styles/table-panel.css"></style>
<style scoped>
.host-resource-toolbar {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--dash-border);
  background: var(--dash-surface);
}
.host-resource-toolbar strong { color: var(--dash-text); font-size: 13px; }
.host-resource-toolbar span { color: var(--dash-muted); font-size: 12px; }
.host-resource-table .el-scrollbar__bar.is-horizontal { display: none; }
.observability-canvas:is(:fullscreen, .is-presentation) .host-resource-toolbar {
  border-bottom-color: #283442;
  background: #151e29;
}
.observability-canvas:is(:fullscreen, .is-presentation) .host-resource-toolbar strong,
.observability-canvas:is(:fullscreen, .is-presentation) .host-resource-toolbar span {
  color: #9badc2;
}
</style>
