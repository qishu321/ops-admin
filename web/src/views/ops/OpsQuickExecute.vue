<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { queryAssetHostGroupList, queryAssetHostList } from '../../api/asset'
import {
  executeOpsCommand,
  executeOpsFileDispatch,
  executeOpsScript,
  queryOpsScriptOptions
} from '../../api/ops'

const activeTab = ref('command')
const submitting = ref(false)
const hostOptions = ref([])
const groupOptions = ref([])
const scriptOptions = ref([])
const latestResult = ref(null)

const commandForm = reactive({
  title: '',
  commandText: '',
  parameters: '',
  hostIds: [],
  groupIds: [],
  concurrency: 5
})

const scriptForm = reactive({
  title: '',
  scriptId: undefined,
  parameters: '',
  hostIds: [],
  groupIds: [],
  concurrency: 5
})

const fileForm = reactive({
  title: '',
  sourceType: 'upload',
  sourceHostId: undefined,
  sourcePath: '',
  targetPath: '',
  hostIds: [],
  groupIds: [],
  concurrency: 5,
  overwrite: false,
  file: null
})

const flatGroupOptions = computed(() => flattenGroups(groupOptions.value))

function flattenGroups(nodes = [], prefix = '') {
  return nodes.flatMap((item) => {
    const label = prefix ? `${prefix} / ${item.name}` : item.name
    return [{ label, value: item.id }, ...flattenGroups(item.children || [], label)]
  })
}

async function loadOptions() {
  const [hosts, groups, scripts] = await Promise.all([
    queryAssetHostList({ pageNum: 1, pageSize: 500 }),
    queryAssetHostGroupList(),
    queryOpsScriptOptions()
  ])
  hostOptions.value = hosts.list || []
  groupOptions.value = groups.tree || []
  scriptOptions.value = scripts || []
}

function handleFileChange(file) {
  fileForm.file = file?.raw || null
}

function validateTargets(hostIds, groupIds) {
  if (!hostIds.length && !groupIds.length) {
    ElMessage.warning('请至少选择一台主机或一个主机组')
    return false
  }
  return true
}

async function submitCommand() {
  if (!commandForm.commandText.trim()) {
    ElMessage.warning('请输入执行命令')
    return
  }
  if (!validateTargets(commandForm.hostIds, commandForm.groupIds)) return
  submitting.value = true
  try {
    latestResult.value = await executeOpsCommand(commandForm)
    ElMessage.success('命令执行完成，结果已写入执行历史')
  } finally {
    submitting.value = false
  }
}

async function submitScript() {
  if (!scriptForm.scriptId) {
    ElMessage.warning('请选择脚本')
    return
  }
  if (!validateTargets(scriptForm.hostIds, scriptForm.groupIds)) return
  submitting.value = true
  try {
    latestResult.value = await executeOpsScript(scriptForm)
    ElMessage.success('脚本执行完成，结果已写入执行历史')
  } finally {
    submitting.value = false
  }
}

async function submitFileDispatch() {
  if (!fileForm.targetPath.trim()) {
    ElMessage.warning('请输入目标路径')
    return
  }
  if (!validateTargets(fileForm.hostIds, fileForm.groupIds)) return
  if (fileForm.sourceType === 'upload' && !fileForm.file) {
    ElMessage.warning('请上传待分发文件')
    return
  }
  if (fileForm.sourceType === 'server' && (!fileForm.sourceHostId || !fileForm.sourcePath.trim())) {
    ElMessage.warning('请选择源服务器并填写源文件路径')
    return
  }

  const formData = new FormData()
  formData.append('title', fileForm.title)
  formData.append('sourceType', fileForm.sourceType)
  formData.append('sourceHostId', String(fileForm.sourceHostId || 0))
  formData.append('sourcePath', fileForm.sourcePath)
  formData.append('targetPath', fileForm.targetPath)
  formData.append('hostIds', JSON.stringify(fileForm.hostIds))
  formData.append('groupIds', JSON.stringify(fileForm.groupIds))
  formData.append('concurrency', String(fileForm.concurrency))
  formData.append('overwrite', String(fileForm.overwrite))
  if (fileForm.file) {
    formData.append('file', fileForm.file)
  }

  submitting.value = true
  try {
    latestResult.value = await executeOpsFileDispatch(formData)
    ElMessage.success('文件分发完成，结果已写入执行历史')
  } finally {
    submitting.value = false
  }
}

onMounted(loadOptions)
</script>

<template>
  <div class="ops-page">
    <div class="page-card">
      <div class="page-header">
        <div>
          <h2 class="page-title">快速执行</h2>
          <p class="page-desc">通过 SSH 执行命令、脚本或一对多分发文件。并发默认 5，最大 10。</p>
        </div>
      </div>

      <el-tabs v-model="activeTab" class="ops-tabs">
        <el-tab-pane label="命令执行" name="command">
          <div class="form-grid">
            <el-form label-width="100px">
              <el-form-item label="任务名称">
                <el-input v-model="commandForm.title" placeholder="可选，默认自动生成" />
              </el-form-item>
              <el-form-item label="执行命令" required>
                <el-input v-model="commandForm.commandText" type="textarea" :rows="8" placeholder="例如：systemctl status nginx" />
              </el-form-item>
              <el-form-item label="执行参数">
                <el-input v-model="commandForm.parameters" placeholder="例如：--env prod" />
              </el-form-item>
              <el-form-item label="目标主机">
                <el-select v-model="commandForm.hostIds" multiple filterable collapse-tags style="width: 100%" placeholder="选择主机">
                  <el-option v-for="item in hostOptions" :key="item.id" :label="`${item.hostName} (${item.sshIp || item.privateIp || '-'})`" :value="item.id" />
                </el-select>
              </el-form-item>
              <el-form-item label="主机组">
                <el-select v-model="commandForm.groupIds" multiple filterable collapse-tags style="width: 100%" placeholder="选择主机组">
                  <el-option v-for="item in flatGroupOptions" :key="item.value" :label="item.label" :value="item.value" />
                </el-select>
              </el-form-item>
              <el-form-item label="并发数">
                <el-input-number v-model="commandForm.concurrency" :min="1" :max="10" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="submitting" @click="submitCommand">立即执行</el-button>
              </el-form-item>
            </el-form>
          </div>
        </el-tab-pane>

        <el-tab-pane label="脚本执行" name="script">
          <el-form label-width="100px">
            <el-form-item label="任务名称">
              <el-input v-model="scriptForm.title" placeholder="可选，默认自动生成" />
            </el-form-item>
            <el-form-item label="脚本" required>
              <el-select v-model="scriptForm.scriptId" filterable style="width: 100%" placeholder="选择脚本库中的脚本">
                <el-option v-for="item in scriptOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="执行参数">
              <el-input v-model="scriptForm.parameters" placeholder="例如：--force（作为命令行参数传入）" />
            </el-form-item>
            <el-form-item label="目标主机">
              <el-select v-model="scriptForm.hostIds" multiple filterable collapse-tags style="width: 100%">
                <el-option v-for="item in hostOptions" :key="item.id" :label="`${item.hostName} (${item.sshIp || item.privateIp || '-'})`" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="主机组">
              <el-select v-model="scriptForm.groupIds" multiple filterable collapse-tags style="width: 100%">
                <el-option v-for="item in flatGroupOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="并发数">
              <el-input-number v-model="scriptForm.concurrency" :min="1" :max="10" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="submitting" @click="submitScript">立即执行</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane label="文件分发" name="file">
          <el-form label-width="110px">
            <el-form-item label="任务名称">
              <el-input v-model="fileForm.title" placeholder="可选，默认自动生成" />
            </el-form-item>
            <el-form-item label="来源方式">
              <el-radio-group v-model="fileForm.sourceType">
                <el-radio value="upload">本地上传</el-radio>
                <el-radio value="server">服务器源文件</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="fileForm.sourceType === 'upload'" label="上传文件">
              <el-upload :auto-upload="false" :show-file-list="true" :limit="1" :on-change="handleFileChange">
                <el-button>选择文件</el-button>
              </el-upload>
            </el-form-item>
            <template v-else>
              <el-form-item label="源服务器">
                <el-select v-model="fileForm.sourceHostId" filterable style="width: 100%" placeholder="选择源服务器">
                  <el-option v-for="item in hostOptions" :key="item.id" :label="`${item.hostName} (${item.sshIp || item.privateIp || '-'})`" :value="item.id" />
                </el-select>
              </el-form-item>
              <el-form-item label="源文件路径">
                <el-input v-model="fileForm.sourcePath" placeholder="例如：/data/release/app.tar.gz" />
              </el-form-item>
            </template>
            <el-form-item label="目标路径" required>
              <el-input v-model="fileForm.targetPath" placeholder="例如：/opt/apps/app.tar.gz" />
            </el-form-item>
            <el-form-item label="目标主机">
              <el-select v-model="fileForm.hostIds" multiple filterable collapse-tags style="width: 100%">
                <el-option v-for="item in hostOptions" :key="item.id" :label="`${item.hostName} (${item.sshIp || item.privateIp || '-'})`" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="主机组">
              <el-select v-model="fileForm.groupIds" multiple filterable collapse-tags style="width: 100%">
                <el-option v-for="item in flatGroupOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="并发数">
              <el-input-number v-model="fileForm.concurrency" :min="1" :max="10" />
            </el-form-item>
            <el-form-item label="覆盖已有">
              <el-switch v-model="fileForm.overwrite" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="submitting" @click="submitFileDispatch">立即分发</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </div>

    <div v-if="latestResult?.task" class="page-card">
      <div class="result-header">
        <div>
          <h3>最近一次执行结果</h3>
          <p>{{ latestResult.task.title }} · {{ latestResult.task.summary || '-' }}</p>
        </div>
        <el-tag :type="latestResult.task.status === 'success' ? 'success' : latestResult.task.status === 'partial' ? 'warning' : 'danger'">
          {{ latestResult.task.status }}
        </el-tag>
      </div>
      <el-table :data="latestResult.results || []" border>
        <el-table-column prop="hostName" label="主机" min-width="180" />
        <el-table-column prop="sshIp" label="SSH IP" min-width="140" />
        <el-table-column prop="status" label="状态" width="100" />
        <el-table-column prop="exitCode" label="退出码" width="90" />
        <el-table-column prop="durationMs" label="耗时(ms)" width="110" />
        <el-table-column prop="stdout" label="输出" min-width="260" show-overflow-tooltip />
        <el-table-column prop="errorText" label="错误" min-width="220" show-overflow-tooltip />
      </el-table>
    </div>
  </div>
</template>

<style scoped>
.ops-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.page-title {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 700;
  color: #14213d;
}

.page-desc {
  margin: 0;
  color: #7282a0;
}

.ops-tabs :deep(.el-tabs__header) {
  margin-bottom: 18px;
}

.result-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.result-header h3 {
  margin: 0 0 6px;
}

.result-header p {
  margin: 0;
  color: #7282a0;
}
</style>
