<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { queryAssetHostGroupList, queryAssetHostList } from '../../api/asset'
import { executeOpsCommand, queryOpsExecHistoryDetail } from '../../api/ops'
import OpsTargetSelector from './components/OpsTargetSelector.vue'
import OpsExecutionResultDialog from './components/OpsExecutionResultDialog.vue'
import { confirmRiskOperation } from '../../composables/useRiskConfirm'

const submitting = ref(false)
const hostOptions = ref([])
const groupOptions = ref([])
const resultVisible = ref(false)
const resultLoading = ref(false)
const resultTask = ref(null)
const resultRows = ref([])
let pollTimer = null

const form = reactive({
  title: '',
  commandText: '',
  hostIds: [],
  groupId: undefined,
  concurrency: 5,
  timeoutSeconds: 10
})

const canSubmit = computed(() => Boolean(
  form.commandText.trim() && (form.hostIds.length || form.groupId)
))

async function loadOptions() {
  const [hosts, groups] = await Promise.all([
    queryAssetHostList({ pageNum: 1, pageSize: 1000 }),
    queryAssetHostGroupList()
  ])
  hostOptions.value = hosts.list || []
  groupOptions.value = groups.tree || []
}

async function submit() {
  if (!form.commandText.trim()) {
    ElMessage.warning('请输入执行命令')
    return
  }
  if (!form.hostIds.length && !form.groupId) {
    ElMessage.warning('请选择目标主机或主机组')
    return
  }
  const highRisk = /rm\s+-rf|mkfs|shutdown|reboot|init\s+[06]|dd\s+if=|iptables\s+-f|systemctl\s+stop|userdel|drop\s+database|truncate\s+table/i.test(form.commandText)
  const selectedHosts = hostOptions.value.filter((host) => {
    if (form.groupId) {
      return Number(host.groupId) === Number(form.groupId) || (host.hostGroups || []).some((group) => Number(group.id) === Number(form.groupId))
    }
    return form.hostIds.map(Number).includes(Number(host.id))
  })
  const includesProduction = selectedHosts.some((host) => ['prod', 'production'].includes(String(host.environment || '').toLowerCase()) || String(host.environment || '').includes('生产'))
  const confirmationRequired = highRisk || includesProduction
  if (confirmationRequired) {
    await confirmRiskOperation({
      operation: '批量命令执行',
      targetSummary: selectedHosts.length ? selectedHosts.map((host) => host.hostName || host.sshIp || host.id).slice(0, 4).join('、') : '所选主机组',
      targetCount: selectedHosts.length,
      production: includesProduction,
      destructive: highRisk
    })
  }
  submitting.value = true
  resultVisible.value = true
  resultLoading.value = true
  resultTask.value = {
    title: form.title || '命令执行任务',
    taskType: 'command',
    status: 'running',
    summary: '正在执行中，请稍候...'
  }
  resultRows.value = []
  try {
    const data = await executeOpsCommand({
      title: form.title,
      commandText: form.commandText,
      hostIds: form.hostIds,
      groupIds: form.groupId ? [form.groupId] : [],
      concurrency: form.concurrency,
      timeoutSeconds: form.timeoutSeconds,
      riskConfirmed: confirmationRequired
    })
    resultTask.value = data.task || null
    resultRows.value = data.results || []
    startPolling()
  } finally {
    submitting.value = false
  }
}

async function refreshTaskDetail() {
  if (!resultTask.value?.id) return
  const data = await queryOpsExecHistoryDetail(resultTask.value.id)
  resultTask.value = data.task || null
  resultRows.value = data.results || []
  resultLoading.value = false
  if (resultTask.value?.status && resultTask.value.status !== 'running') {
    stopPolling()
  }
}

function startPolling() {
  stopPolling()
  refreshTaskDetail().catch(() => {})
  pollTimer = setInterval(() => {
    refreshTaskDetail().catch(() => {})
  }, 1200)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function handleResultClosed() {
  stopPolling()
  if (resultTask.value?.id) {
    ElMessage.info('执行结果已保存在快速执行历史中，可随时前往查看。')
  }
}

onMounted(loadOptions)
onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="page-card ops-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">命令执行</h2>
        <p class="page-desc">手动输入命令，通过 SSH 批量下发到目标主机。主机与主机组互斥选择。</p>
      </div>
    </div>

    <el-form label-width="110px">
      <el-form-item label="任务名称">
        <el-input v-model="form.title" placeholder="可选，默认自动生成" />
      </el-form-item>
      <el-form-item label="执行命令" required>
        <el-input v-model="form.commandText" type="textarea" :rows="10" placeholder="例如：systemctl restart nginx" />
      </el-form-item>
      <OpsTargetSelector
        :host-options="hostOptions"
        :group-options="groupOptions"
        :host-ids="form.hostIds"
        :group-id="form.groupId"
        @update:host-ids="form.hostIds = $event"
        @update:group-id="form.groupId = $event"
      />
      <el-form-item label="并发数">
        <el-input-number v-model="form.concurrency" :min="1" :max="10" />
      </el-form-item>
      <el-form-item label="超时时间">
        <el-input-number v-model="form.timeoutSeconds" :min="10" :max="3600" />
        <span class="inline-hint">秒，超过后自动终止执行</span>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="submitting" :disabled="!canSubmit" @click="submit">立即执行</el-button>
        <span v-if="!canSubmit" class="inline-hint">填写命令并选择目标主机或主机组后可执行</span>
      </el-form-item>
    </el-form>
    <OpsExecutionResultDialog
      v-model="resultVisible"
      :loading="resultLoading"
      :task="resultTask"
      :results="resultRows"
      @closed="handleResultClosed"
    />
  </div>
</template>

<style scoped>
.ops-page { display: flex; flex-direction: column; gap: 18px; }
.page-title { margin: 0 0 8px; font-size: 22px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.inline-hint { margin-left: 12px; color: #7282a0; font-size: 13px; }
</style>
