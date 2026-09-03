<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteNotifyChannel, notifyChannelInfo, queryNotifyChannelList, saveNotifyChannel } from '../../api/ops'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const rows = ref([])
const total = ref(0)

const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', channelType: '', status: '' })
const form = reactive({
  id: undefined,
  name: '',
  channelType: 'dingtalk',
  webhookUrl: '',
  secret: '',
  headersJson: '{}',
  status: 1,
  description: ''
})

const channelTypes = [
  { label: '钉钉机器人', value: 'dingtalk' },
  { label: '企微机器人', value: 'wecom' },
  { label: '飞书机器人', value: 'feishu' },
  { label: '自定义 HTTP Webhook', value: 'webhook' }
]

function channelTypeLabel(value) {
  return channelTypes.find((item) => item.value === value)?.label || value
}

function formatDateTime(value) {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const match = raw.match(/^(\d{4}-\d{2}-\d{2})[T\s](\d{2}:\d{2}:\d{2})/)
  if (match) return `${match[1]} ${match[2]}`
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    channelType: 'dingtalk',
    webhookUrl: '',
    secret: '',
    headersJson: '{}',
    status: 1,
    description: ''
  })
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryNotifyChannelList(query)
    rows.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function openCreate() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row) {
  isEdit.value = true
  const data = await notifyChannelInfo(row.id)
  Object.assign(form, data)
  dialogVisible.value = true
}

async function submit() {
  if (!form.name.trim() || !form.webhookUrl.trim()) {
    ElMessage.warning('请填写媒介名称和 Webhook 地址')
    return
  }
  saving.value = true
  try {
    await saveNotifyChannel(form)
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除通知媒介「${row.name}」吗？`, '提示', { type: 'warning' })
  await deleteNotifyChannel(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

onMounted(loadData)
</script>

<template>
  <div class="page-card notify-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">消息通知媒介</h2>
        <p class="page-desc">维护钉钉、企微、飞书机器人和自定义 HTTP Webhook 的发送通道。</p>
      </div>
      <el-button type="primary" @click="openCreate">新增媒介</el-button>
    </div>

    <div class="toolbar">
      <el-input v-model="query.keyword" clearable placeholder="搜索媒介名称 / 描述" style="width: 260px" @keyup.enter="loadData" />
      <el-select v-model="query.channelType" clearable placeholder="媒介类型" style="width: 180px">
        <el-option v-for="item in channelTypes" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-select v-model="query.status" clearable placeholder="状态" style="width: 120px">
        <el-option label="启用" value="1" />
        <el-option label="禁用" value="2" />
      </el-select>
      <el-button type="primary" @click="loadData">搜索</el-button>
    </div>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="name" label="媒介名称" min-width="180" />
      <el-table-column label="媒介类型" width="180">
        <template #default="{ row }">{{ channelTypeLabel(row.channelType) }}</template>
      </el-table-column>
      <el-table-column prop="webhookUrl" label="Webhook 地址" min-width="320" show-overflow-tooltip />
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="180"><template #default="{ row }">{{ formatDateTime(row.updateTime) }}</template></el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :total="total" layout="total, sizes, prev, pager, next" @current-change="loadData" @size-change="loadData" />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑通知媒介' : '新增通知媒介'" width="760px">
      <el-form label-width="120px">
        <el-form-item label="媒介名称" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="媒介类型">
          <el-select v-model="form.channelType" style="width: 100%">
            <el-option v-for="item in channelTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="Webhook 地址" required><el-input v-model="form.webhookUrl" /></el-form-item>
        <el-form-item label="签名密钥">
          <el-input v-model="form.secret" show-password placeholder="钉钉加签 Secret，可选" />
        </el-form-item>
        <el-form-item label="请求头 JSON">
          <el-input v-model="form.headersJson" type="textarea" :rows="5" placeholder='{"Authorization":"Bearer xxx"}' />
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :value="1">启用</el-radio>
            <el-radio :value="2">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.notify-page { display: flex; flex-direction: column; gap: 18px; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-title { margin: 0 0 8px; font-size: 24px; font-weight: 700; color: #14213d; }
.page-desc { margin: 0; color: #7282a0; }
.toolbar { display: flex; gap: 12px; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; }
</style>
