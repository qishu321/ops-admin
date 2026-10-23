import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { retryOpsExecTask } from '../api/ops'

/** Share failed-target retry behavior between live results and history. */
export function useOpsTaskRetry(task, onRetried) {
  const retrying = ref(false)
  const retryingResultId = ref(null)
  const retryableTask = computed(() => task.value?.retrySupported === true && !!task.value?.id && ['command', 'script', 'file'].includes(task.value.taskType) && ['failed', 'partial'].includes(task.value.status))
  function canRetryRow(row) {
    return retryableTask.value && !!row?.id && ['failed', 'timeout'].includes(row.status)
  }
  async function retry(row) {
    if (retrying.value || !retryableTask.value || (row && !canRetryRow(row))) return
    const taskId = task.value.id
    retrying.value = true
    retryingResultId.value = row?.id || null
    try {
      const data = await retryOpsExecTask(taskId, row?.id)
      ElMessage.success(row ? `已创建 ${row.hostName || row.sshIp} 的重试任务` : '已创建失败目标重试任务')
      // A response for a closed/switched task must not replace the new dialog.
      if (task.value?.id === taskId) onRetried(data)
      return data
    } catch {
      // The HTTP layer already displays the server's actionable failure.
      // Keep the original results and allow another click after it is fixed.
      return null
    } finally {
      retrying.value = false
      retryingResultId.value = null
    }
  }
  return { retrying, retryingResultId, retryableTask, canRetryRow, retry }
}
