<script setup>
import { computed } from 'vue'
import { useOpsTaskRetry } from '../../../composables/useOpsTaskRetry'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  task: { type: Object, default: null },
  results: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:modelValue', 'closed', 'retried'])
const { retrying, retryingResultId, retryableTask, canRetryRow, retry } = useOpsTaskRetry(computed(() => props.modelValue ? props.task : null), data => emit('retried', data))
const failedCount = computed(() => props.results.filter(row => ['failed', 'timeout'].includes(row.status)).length)

function updateVisible(value) {
  emit('update:modelValue', value)
}

function handleClosed() {
  emit('closed')
}

function statusTagType(status) {
  if (status === 'success') return 'success'
  if (status === 'partial') return 'warning'
  return 'danger'
}

function rowStatusType(status) {
  if (status === 'success') return 'success'
  if (status === 'running') return 'warning'
  if (status === 'pending') return 'info'
  return 'danger'
}

const completedCount = computed(() =>
  (props.results || []).filter((item) => item.status && item.status !== 'running' && item.status !== 'pending').length
)

const progressPercent = computed(() => {
  const total = Number(props.task?.hostCount || props.results?.length || 0)
  if (!total) return 0
  return Math.min(100, Math.round((completedCount.value / total) * 100))
})
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    width="80%"
    top="5vh"
    destroy-on-close
    title="执行任务"
    @update:model-value="updateVisible"
    @closed="handleClosed"
  >
    <div class="result-dialog">
      <el-alert v-if="task?.id && task.retrySupported !== true" title="逐台失败重试需要更新并重启后端服务后启用。" type="info" :closable="false" />
      <el-alert v-if="task?.retryOfTaskId" :title="`仅执行失败目标的重试任务，原任务 #${task.retryOfTaskId} 的成功主机不会重复执行。`" type="info" :closable="false" />
      <el-alert v-if="task?.taskType === 'file' && task?.fileRetryUntil" title="文件重试副本保留24小时，过期后需要重新上传。" type="info" :closable="false" />
      <div v-if="task" class="task-summary">
        <div class="summary-card">
          <span>任务名称</span>
          <strong>{{ task.title }}</strong>
        </div>
        <div class="summary-card">
          <span>任务类型</span>
          <strong>{{ task.taskType }}</strong>
        </div>
        <div class="summary-card">
          <span>执行状态</span>
          <el-tag :type="statusTagType(task.status)">{{ task.status }}</el-tag>
        </div>
        <div class="summary-card">
          <span>执行摘要</span>
          <strong>{{ task.summary || '-' }}</strong>
        </div>
      </div>

      <div v-if="task" class="progress-panel">
        <div class="progress-head">
          <strong>执行进度</strong>
          <span>{{ completedCount }} / {{ task.hostCount || results.length || 0 }}</span>
        </div>
        <el-progress :percentage="progressPercent" :status="task.status === 'failed' ? 'exception' : task.status === 'success' ? 'success' : undefined" />
      </div>

      <el-skeleton v-if="loading" :rows="8" animated />

      <el-table v-else :data="results" border height="460">
        <el-table-column prop="hostName" label="主机" min-width="180" />
        <el-table-column prop="groupName" label="主机组" min-width="140" />
        <el-table-column prop="sshIp" label="SSH IP" min-width="140" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="rowStatusType(row.status)" effect="light">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="exitCode" label="退出码" width="90" />
        <el-table-column prop="durationMs" label="耗时(ms)" width="110" />
        <el-table-column prop="stdout" label="标准输出" min-width="280" show-overflow-tooltip />
        <el-table-column prop="stderr" label="错误输出" min-width="240" show-overflow-tooltip />
        <el-table-column prop="errorText" label="错误信息" min-width="240" show-overflow-tooltip />
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button v-if="canRetryRow(row)" link type="primary" :aria-label="`重试 ${row.hostName || row.sshIp}`" :loading="retrying && retryingResultId === row.id" :disabled="retrying" @click="retry(row)">重试</el-button>
            <span v-else>—</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <template #footer>
      <el-button v-if="retryableTask && failedCount" type="primary" :loading="retrying && !retryingResultId" :disabled="retrying" @click="retry()">重试失败（{{ failedCount }}）</el-button>
      <el-button @click="updateVisible(false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.result-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.progress-panel {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid #e7ecf5;
  background: #ffffff;
}

.progress-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  color: #14213d;
}

.task-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.summary-card {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid #e7ecf5;
  background: #f8fbff;
}

.summary-card span {
  display: block;
  margin-bottom: 6px;
  color: #7282a0;
  font-size: 13px;
}

.summary-card strong {
  color: #14213d;
}
</style>
