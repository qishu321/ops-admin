<script setup>
import { useDashboardDomain } from '../../context'
import { formatPodResourceValue } from '../../utils/format.js'

const { podResourceNamespaces, podDetailRows } = useDashboardDomain('display')
const { podResourceNamespace, handlePodNamespaceChange } = useDashboardDomain('queries')
const { isFullscreen } = useDashboardDomain('fullscreen')
const { isDark } = useDashboardDomain('theme')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
  <div class="pod-resource-toolbar">
    <div class="pod-resource-filter">
      <strong>命名空间</strong>
      <el-select
        aria-label="命名空间"
        :popper-class="isDark || isFullscreen ? 'monitor-dark-popper' : ''"
        v-model="podResourceNamespace"
        :teleported="!isFullscreen"
        clearable
        placeholder="全部命名空间"
        @change="handlePodNamespaceChange(panel)"
      >
        <el-option label="全部命名空间" value="" />
        <el-option v-for="namespace in podResourceNamespaces(panel)" :key="namespace" :label="namespace" :value="namespace" />
      </el-select>
      <span>{{ podResourceNamespace ? '已选择命名空间，展示该命名空间内全部 Pod，并按内存使用量降序排列' : '默认展示全部命名空间，按内存使用量降序取前 10 个 Pod' }}</span>
    </div>
  </div>
  <div class="pod-resource-table-body">
    <el-table
      class="pod-resource-table"
      :data="podDetailRows(panel)"
      size="small"
      height="100%"
      :fit="true"
      :default-sort="{ prop: 'memoryBytes', order: 'descending' }"
    >
      <el-table-column prop="namespace" label="命名空间" width="130" align="center" show-overflow-tooltip />
      <el-table-column prop="pod" label="Pod" width="200" align="center" show-overflow-tooltip />
      <el-table-column prop="node" label="节点" width="200" align="center" show-overflow-tooltip />
      <el-table-column prop="cpuCores" label="CPU 使用" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.cpuCores, 'Core') }}</template></el-table-column>
      <el-table-column prop="cpuUsagePercent" label="CPU 使用率" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.cpuUsagePercent, '%') }}</template></el-table-column>
      <el-table-column prop="cpuRequest" label="CPU Request" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.cpuRequest, 'Core') }}</template></el-table-column>
      <el-table-column prop="cpuLimit" label="CPU Limit" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.cpuLimit, 'Core') }}</template></el-table-column>
      <el-table-column prop="memoryBytes" label="内存使用" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.memoryBytes, 'B') }}</template></el-table-column>
      <el-table-column prop="memoryUsagePercent" label="内存使用率" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.memoryUsagePercent, '%') }}</template></el-table-column>
      <el-table-column prop="memoryRequest" label="内存 Request" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.memoryRequest, 'B') }}</template></el-table-column>
      <el-table-column prop="memoryLimit" label="内存 Limit" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.memoryLimit, 'B') }}</template></el-table-column>
      <el-table-column prop="networkReceive" label="网络流入" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.networkReceive, 'B/s') }}</template></el-table-column>
      <el-table-column prop="networkTransmit" label="网络流出" width="120" align="center" sortable><template #default="{ row }">{{ formatPodResourceValue(row.networkTransmit, 'B/s') }}</template></el-table-column>
    </el-table>
  </div>
</template>

<style scoped src="../../styles/table-panel.css"></style>
<style scoped>
.pod-resource-toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--dash-border);
  background: var(--dash-surface);
}
.pod-resource-filter {
  display: flex;
  align-items: baseline;
  gap: 10px;
  width: 100%;
}
.pod-resource-toolbar strong {
  color: var(--dash-text);
  font-size: 13px;
}
.pod-resource-toolbar span {
  overflow: hidden;
  color: var(--dash-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pod-resource-toolbar .el-select {
  width: 330px;
  flex: 0 0 auto;
}
.pod-resource-table .el-scrollbar__bar.is-horizontal { display: none; }
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar {
  border-bottom-color: #283442;
  background: #151e29;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar span,
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar strong {
  color: #9badc2;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar :deep(.el-select__popper) {
  z-index: 30;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar :deep(.el-input__wrapper),
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar :deep(.el-select__wrapper) {
  border-color: #3b4c60;
  background: #0c131c;
  box-shadow: none;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar :deep(.el-select__selected-item),
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-toolbar :deep(.el-select__placeholder) {
  color: #d7e0ea;
}
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-table .el-table,
.observability-canvas.is-k8s-dashboard:is(:fullscreen, .is-presentation) .pod-resource-table .el-table__inner-wrapper::before {
  --el-table-bg-color: #111923;
  --el-table-tr-bg-color: #111923;
  --el-table-header-bg-color: #151e29;
  --el-table-border-color: #283442;
  --el-table-text-color: #c7d2df;
  --el-table-header-text-color: #a8b8cb;
  background: #111923;
}
@media (max-width: 760px) {
  .pod-resource-toolbar { align-items: flex-start; flex-direction: column; }
  .pod-resource-toolbar .el-select { width: 100%; }
}
</style>
