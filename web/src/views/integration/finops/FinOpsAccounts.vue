<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import FinOpsHeader from './FinOpsHeader.vue'
import {
  deleteFinOpsAccount,
  queryFinOpsAccounts,
  saveFinOpsAccount,
  testFinOpsAccount
} from '../../../api/integration'
import './finops.css'

const providers = [
  ['alicloud', '阿里云（官方账单）'],
  ['tencent', '腾讯云（官方账单）'],
  ['aws', 'AWS（自定义账单适配）'],
  ['azure', 'Azure（自定义账单适配）'],
  ['gcp', 'GCP（自定义账单适配）'],
  ['custom', '自定义账单适配']
]
const frequencies = [
  ['manual', '手动'],
  ['hourly', '每小时'],
  ['daily', '每天'],
  ['weekly', '每周'],
  ['monthly', '每月']
]

const rows = ref([])
const loading = ref(false)
const visible = ref(false)
const saving = ref(false)
const keyword = ref('')
const provider = ref('')

const initial = () => ({
  id: 0,
  name: '',
  provider: 'alicloud',
  accountIdentifier: '',
  accessKey: '',
  secretKey: '',
  region: '',
  currency: 'CNY',
  billingEndpoint: '',
  billingToken: '',
  syncEnabled: false,
  syncFrequency: 'daily',
  status: 1,
  description: ''
})
const form = reactive(initial())
const builtinBillingProvider = computed(() => ['alicloud', 'tencent'].includes(form.provider))
const accessKeyLabel = computed(() => form.provider === 'tencent' ? 'SecretId' : 'Access Key')
const billingProviderHint = computed(() => {
  if (form.provider === 'alicloud') {
    return '阿里云已内置官方账单接口。填写 AccessKey、SecretKey 后即可校验和同步当前月账单；请为该账号授予账单读取权限。'
  }
  if (form.provider === 'tencent') {
    return '腾讯云已内置官方账单接口。填写 SecretId、SecretKey 后即可校验和同步当前月账单；请为该账号授予账单读取权限。'
  }
  return ''
})

async function load() {
  loading.value = true
  try {
    rows.value = await queryFinOpsAccounts({ keyword: keyword.value, provider: provider.value }) || []
  } finally {
    loading.value = false
  }
}

function create() {
  Object.assign(form, initial())
  visible.value = true
}

function edit(row) {
  Object.assign(form, initial(), row, { accessKey: '', secretKey: '', billingToken: '' })
  visible.value = true
}

async function save() {
  if (!form.name || !form.provider) {
    ElMessage.warning('请填写账号名称和云厂商')
    return
  }
  saving.value = true
  try {
    await saveFinOpsAccount(form)
    visible.value = false
    await load()
    ElMessage.success('云账号已保存')
  } finally {
    saving.value = false
  }
}

async function test(row = form) {
  const result = await testFinOpsAccount({ ...row })
  ElMessage.success(`账单连接校验通过${result?.source ? `，${result.source}` : ''}`)
}

function handleProviderChange() {
  if (builtinBillingProvider.value) {
    form.billingEndpoint = ''
    form.billingToken = ''
  }
}

async function remove(row) {
  await ElMessageBox.confirm(`确认删除云账号“${row.name}”及其账单数据吗？`, '删除云账号', { type: 'warning' })
  await deleteFinOpsAccount(row.id)
  await load()
  ElMessage.success('已删除')
}

const providerName = value => providers.find(item => item[0] === value)?.[1] || value
const frequencyName = value => frequencies.find(item => item[0] === value)?.[1] || value
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

onMounted(load)
</script>

<template>
  <div class="finops-page">
    <FinOpsHeader />
    <section class="finops-panel">
      <div class="finops-head">
        <div>
          <h2>云账号</h2>
          <p>阿里云、腾讯云使用平台内置官方账单接口；其他云厂商可通过自定义账单适配服务同步。</p>
        </div>
        <el-button type="primary" @click="create">新增云账号</el-button>
      </div>
      <div class="finops-filter">
        <el-input v-model="keyword" clearable placeholder="搜索账号名称或标识" style="width: 260px" @keyup.enter="load" />
        <el-select v-model="provider" clearable placeholder="全部厂商" style="width: 160px">
          <el-option v-for="item in providers" :key="item[0]" :label="item[1]" :value="item[0]" />
        </el-select>
        <el-button @click="load">查询</el-button>
      </div>
      <el-table v-loading="loading" :data="rows" style="margin-top: 16px">
        <el-table-column prop="name" label="账号名称" min-width="150" />
        <el-table-column label="云厂商" width="110">
          <template #default="{ row }"><span class="finops-provider">{{ providerName(row.provider) }}</span></template>
        </el-table-column>
        <el-table-column label="账单接入" width="140">
          <template #default="{ row }">
            <el-tag size="small" :type="row.billingCapability?.mode === 'builtin' ? 'success' : 'info'">{{ row.billingCapability?.label || '账单适配器' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="accountIdentifier" label="账号标识" min-width="140" />
        <el-table-column prop="region" label="默认区域" width="130" />
        <el-table-column prop="currency" label="币种" width="75" />
        <el-table-column label="同步策略" width="140">
          <template #default="{ row }">{{ row.syncEnabled ? frequencyName(row.syncFrequency) : '未启用' }}</template>
        </el-table-column>
        <el-table-column label="最后同步" min-width="170">
          <template #default="{ row }">{{ formatDateTime(row.lastSyncAt) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="190" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="test(row)">校验</el-button>
            <el-button link type="primary" @click="edit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <el-dialog v-model="visible" :title="form.id ? '编辑云账号' : '新增云账号'" width="920px">
      <el-alert title="凭据仅加密保存于服务端。编辑时留空表示保留原凭据。" type="info" :closable="false" show-icon />
      <el-form label-position="top" style="margin-top: 18px">
        <el-row :gutter="18">
          <el-col :span="8"><el-form-item label="账号名称" required><el-input v-model="form.name" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="云厂商" required><el-select v-model="form.provider" style="width: 100%" @change="handleProviderChange"><el-option v-for="item in providers" :key="item[0]" :label="item[1]" :value="item[0]" /></el-select></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="账号标识"><el-input v-model="form.accountIdentifier" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item :label="accessKeyLabel"><el-input v-model="form.accessKey" :placeholder="form.id ? '留空保持不变' : ''" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="Secret Key"><el-input v-model="form.secretKey" show-password :placeholder="form.id ? '留空保持不变' : ''" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="默认区域"><el-input v-model="form.region" placeholder="例如 cn-shanghai" /></el-form-item></el-col>
          <el-col v-if="builtinBillingProvider" :span="24"><el-alert :title="billingProviderHint" type="success" :closable="false" show-icon /></el-col>
          <template v-else>
            <el-col :span="16"><el-form-item label="账单 HTTP 地址"><el-input v-model="form.billingEndpoint" placeholder="自定义适配服务地址，响应需包含 records 数组" /></el-form-item></el-col>
            <el-col :span="8"><el-form-item label="Bearer Token"><el-input v-model="form.billingToken" show-password :placeholder="form.id ? '留空保持不变' : ''" /></el-form-item></el-col>
          </template>
          <el-col :span="8"><el-form-item label="币种"><el-select v-model="form.currency" style="width: 100%"><el-option label="CNY" value="CNY" /><el-option label="USD" value="USD" /><el-option label="EUR" value="EUR" /></el-select></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="同步频率"><el-select v-model="form.syncFrequency" style="width: 100%"><el-option v-for="item in frequencies" :key="item[0]" :label="item[1]" :value="item[0]" /></el-select></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="自动同步"><el-switch v-model="form.syncEnabled" /></el-form-item></el-col>
          <el-col :span="24"><el-form-item label="备注"><el-input v-model="form.description" type="textarea" :rows="2" /></el-form-item></el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="test(form)">校验配置</el-button>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
