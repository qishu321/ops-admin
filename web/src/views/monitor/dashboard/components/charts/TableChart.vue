<script setup>
import { useDashboardDomain } from '../../context'
import { metricText } from '../../utils/format.js'
import PodResourceTable from './PodResourceTable.vue'
import HostResourceTable from './HostResourceTable.vue'

const { isPodDetailPanel, isHostDetailPanel } = useDashboardDomain('selection')
const { panelRows } = useDashboardDomain('display')
defineProps({ panel: { type: Object, required: true } })
</script>

<template>
  <PodResourceTable v-if="isPodDetailPanel(panel)" :panel="panel" />
  <HostResourceTable v-else-if="isHostDetailPanel(panel)" :panel="panel" />
  <!-- 表格高度必须由卡片高度推出来：写死像素值在编辑布局拉伸面板后，
       卡片会剩下一截空白。外层容器负责拿到确定高度，el-table 用 100% 跟随。 -->
  <div v-else class="table-panel-body">
    <el-table :data="panelRows(panel)" size="small" height="100%">
      <el-table-column label="Metric" min-width="240" show-overflow-tooltip><template #default="{ row }">{{ metricText(row.metric) }}</template></el-table-column>
      <el-table-column label="Value" width="130"><template #default="{ row }">{{ row.value?.[1] ?? '-' }}</template></el-table-column>
    </el-table>
  </div>
</template>

<style scoped src="../../styles/table-panel.css"></style>
