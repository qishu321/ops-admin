<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { queryAssetHostGroupList, queryAssetHostList } from '../../api/asset'
import { executeOpsFileDispatch, queryOpsExecHistoryDetail } from '../../api/ops'
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
const fileUploadRef = ref(null)
let pollTimer = null
const maxLocalUploadBytes = 500 * 1024 * 1024

const form = reactive({
  title: '',
  sourceType: 'upload',
  sourceHostId: undefined,
  sourcePath: '',
  targetDir: '',
  targetFileName: '',
  hostIds: [],
  groupId: undefined,
  concurrency: 5,
  timeoutSeconds: 600,
  overwrite: false,
  file: null
})

async function loadOptions() {
  const [hosts, groups] = await Promise.all([
    queryAssetHostList({ pageNum: 1, pageSize: 1000 }),
    queryAssetHostGroupList()
  ])
  hostOptions.value = hosts.list || []
  groupOptions.value = groups.tree || []
}

function handleFileChange(file) {
  const raw = file?.raw || null
  if (raw?.size > maxLocalUploadBytes) {
    form.file = null
    fileUploadRef.value?.clearFiles()
    ElMessage.error('本地上传文件大小不能超过 500 MB')
    return
  }
  form.file = raw
}

function handleFileExceed() {
  ElMessage.warning('一次只能选择一个文件')
}

function handleFileRemove() {
  form.file = null
}

const sourceFileName = computed(() => {
  if (form.sourceType === 'upload') return form.file?.name || ''
  return form.sourcePath.trim().split('/').filter(Boolean).pop() || ''
})

const finalTargetPreview = computed(() => {
  if (!form.targetDir.trim()) return ''
  const fileName = form.targetFileName.trim() || sourceFileName.value
  return fileName ? `${form.targetDir.replace(/\/+$/, '')}/${fileName}` : form.targetDir
})

async function submit() {
  if (!form.targetDir.trim()) {
    ElMessage.warning('请输入目标目录')
    return
  }
  if (!form.hostIds.length && !form.groupId) {
    ElMessage.warning('请选择目标主机或主机组')
    return
  }
  if (form.sourceType === 'upload' && !form.file) {
    ElMessage.warning('请上传待分发文件')
    return
  }
  if (form.sourceType === 'server' && (!form.sourceHostId || !form.sourcePath.trim())) {
    ElMessage.warning('请选择源服务器并填写源文件路径')
    return
  }
  const highRisk = /^\/(etc|boot|usr)\//.test(form.targetDir.trim())
  const selectedHosts = hostOptions.value.filter((host) => {
    if (form.groupId) {
      return Number(host.groupId) === Number(form.groupId) || (host.hostGroups || []).some((group) => Number(group.id) === Number(form.groupId))
    }
    return form.hostIds.map(Number).includes(Number(host.id))
  })
  const includesProduction = selectedHosts.some((host) => ['prod', 'production'].includes(String(host.environment || '').toLowerCase()) || String(host.environment || '').includes('生产'))
  const confirmationRequired = highRisk || includesProduction || form.overwrite
  if (confirmationRequired) {
    await confirmRiskOperation({
      operation: `文件分发至 ${finalTargetPreview.value || form.targetDir}`,
      targetSummary: selectedHosts.length ? selectedHosts.map((host) => host.hostName || host.sshIp || host.id).slice(0, 4).join('、') : '所选主机组',
      targetCount: selectedHosts.length,
      production: includesProduction,
      destructive: highRisk || form.overwrite
    })
  }

  const payload = new FormData()
  payload.append('title', form.title)
  payload.append('sourceType', form.sourceType)
  payload.append('sourceHostId', String(form.sourceHostId || 0))
  payload.append('sourcePath', form.sourcePath)
  payload.append('targetDir', form.targetDir)
  payload.append('targetFileName', form.targetFileName)
  payload.append('hostIds', JSON.stringify(form.hostIds))
  payload.append('groupIds', JSON.stringify(form.groupId ? [form.groupId] : []))
  payload.append('concurrency', String(form.concurrency))
  payload.append('timeoutSeconds', String(form.timeoutSeconds))
  payload.append('overwrite', String(form.overwrite))
  payload.append('riskConfirmed', String(confirmationRequired))
  if (form.file) payload.append('file', form.file)

  submitting.value = true
  resultVisible.value = true
  resultLoading.value = true
  resultTask.value = {
    title: form.title || '文件分发任务',
    taskType: 'file',
    status: 'running',
    summary: '正在执行中，请稍候...'
  }
  resultRows.value = []
  try {
    const data = await executeOpsFileDispatch(payload)
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
        <h2 class="page-title">文件分发</h2>
        <p class="page-desc">支持本地上传或从服务器读取源文件，面向一对多分发场景。</p>
      </div>
    </div>

    <el-form label-width="110px">
      <el-form-item label="任务名称">
        <el-input v-model="form.title" placeholder="可选，默认自动生成" />
      </el-form-item>
      <el-form-item label="来源方式">
        <el-radio-group v-model="form.sourceType">
          <el-radio value="upload">本地上传</el-radio>
          <el-radio value="server">服务器源文件</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="form.sourceType === 'upload'" label="上传文件">
        <el-upload ref="fileUploadRef" :auto-upload="false" :limit="1" :on-change="handleFileChange" :on-exceed="handleFileExceed" :on-remove="handleFileRemove">
          <el-button>选择文件</el-button>
          <template #tip>
            <div class="upload-hint">本地上传文件最大 500 MB。</div>
          </template>
        </el-upload>
      </el-form-item>
      <template v-else>
        <el-form-item label="源服务器">
          <el-select v-model="form.sourceHostId" filterable placeholder="选择源服务器" style="width: 100%">
            <el-option v-for="item in hostOptions" :key="item.id" :label="`${item.hostName} (${item.sshIp || item.privateIp || '-'})`" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="源文件路径">
          <el-input v-model="form.sourcePath" placeholder="例如：/data/release/app.tar.gz" />
        </el-form-item>
      </template>
      <el-form-item label="目标目录" required>
        <el-input v-model="form.targetDir" placeholder="例如：/opt/apps" />
      </el-form-item>
      <el-form-item label="目标文件名">
        <el-input v-model="form.targetFileName" :placeholder="sourceFileName ? `默认沿用源文件名：${sourceFileName}` : '可选，默认沿用源文件名'" />
        <div v-if="finalTargetPreview" class="target-preview">最终保存为：{{ finalTargetPreview }}</div>
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
      <el-form-item label="覆盖已有">
        <el-switch v-model="form.overwrite" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="submitting" @click="submit">立即分发</el-button>
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
.inline-hint, .upload-hint { margin-left: 12px; color: #7282a0; font-size: 13px; }
.upload-hint { margin-left: 0; margin-top: 6px; }
.target-preview { margin-top: 6px; color: #7282a0; font-size: 13px; }
</style>
