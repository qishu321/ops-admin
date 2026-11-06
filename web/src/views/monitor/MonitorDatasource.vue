<script setup>
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteMonitorDatasource,
  monitorDatasourceInfo,
  queryMonitorDatasourceList,
  saveMonitorDatasource,
  testMonitorDatasource
} from '../../api/monitor'
import { queryAssetCloudAccountOptions } from '../../api/asset'
import { useEnvironmentOptions } from '../../composables/useEnvironmentOptions'

const cloudAccounts = ref([])
const slsDefaults = () => ({region:'cn-hangzhou', project:'', cloudAccountId:undefined, defaultLogstore:'', logstores:[]})
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const rows = ref([])
const total = ref(0)
const { environmentOptions, environmentLoading, environmentName } = useEnvironmentOptions()
const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', type: '', status: '', env: '' })
const form = reactive({
  id: undefined,
  name: '',
  type: 'prometheus',
  sls: slsDefaults(),
  env: 'test',
  url: '',
  authType: 'none',
  username: '',
  password: '',
  token: '',
  isDefault: false,
  status: 1,
  description: ''
})

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    type: 'prometheus',
    sls: slsDefaults(),
    env: 'test',
    url: '',
    authType: 'none',
    username: '',
    password: '',
    token: '',
    isDefault: false,
    status: 1,
    description: ''
  })
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryMonitorDatasourceList(query)
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
  const data = await monitorDatasourceInfo(row.id)
  Object.assign(form, data)
  dialogVisible.value = true
}

async function submit() {
  if (!form.name.trim() || !form.url.trim()) {
    ElMessage.warning('请填写数据源名称和地址')
    return
  }
  saving.value = true
  try {
    await saveMonitorDatasource(form)
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function handleTest(payload = form) {
  testing.value = true
  try {
    await testMonitorDatasource(payload)
    ElMessage.success('数据源连接成功')
    if (payload.id) await loadData()
  } finally {
    testing.value = false
  }
}

function healthType(status) {
  return ({ healthy: 'success', unhealthy: 'danger', unknown: 'info' }[status] || 'info')
}

function healthText(status) {
  return ({ healthy: '健康', unhealthy: '异常', unknown: '待检测' }[status] || '待检测')
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除数据源「${row.name}」吗？`, '提示', { type: 'warning' })
  await deleteMonitorDatasource(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

watch(() => form.type, async type => {
 if (type !== 'sls') return
 form.sls ||= slsDefaults()
 if (!form.url) form.url = `https://${form.sls.region}.log.aliyuncs.com`
 try { cloudAccounts.value = (await queryAssetCloudAccountOptions()).filter(account => ['aliyun', 'alicloud'].includes(account.provider)) } catch { cloudAccounts.value = [] }
})
watch(() => form.sls?.region, (region, previous) => {
 if (form.type !== 'sls' || !previous || !region) return
 for (const suffix of ['.log.aliyuncs.com', '-intranet.log.aliyuncs.com']) {
  if (form.url === `https://${previous}${suffix}`) form.url = `https://${region}${suffix}`
 }
})
onMounted(loadData)
</script>

<template>
  <div class="monitor-page monitor-datasource-page">
    <div class="page-header">
      <div>
        <h2>数据源管理</h2>
        <p>接入 Prometheus、VictoriaMetrics、Elasticsearch、VictoriaLogs、阿里云 SLS 或 Jaeger，统一管理指标、日志与链路追踪入口。</p>
      </div>
      <el-button type="primary" @click="openCreate">新增数据源</el-button>
    </div>

    <div class="toolbar">
      <el-input v-model="query.keyword" clearable placeholder="搜索名称 / 地址" style="width: 260px" @keyup.enter="loadData" />
      <el-select v-model="query.type" clearable placeholder="类型" style="width: 170px">
        <el-option label="Prometheus" value="prometheus" />
        <el-option label="VictoriaMetrics" value="victoriametrics" />
        <el-option label="VictoriaLogs" value="victorialogs" />
        <el-option label="Elasticsearch" value="elasticsearch" />
        <el-option label="阿里云 SLS" value="sls" /><el-option label="Jaeger" value="jaeger" />
      </el-select>
      <el-select v-model="query.status" clearable placeholder="状态" style="width: 130px">
        <el-option label="启用" value="1" />
        <el-option label="禁用" value="2" />
      </el-select>
      <el-select v-model="query.env" clearable placeholder="全部环境" style="width: 150px" @change="loadData">
        <el-option v-for="item in environmentOptions" :key="item.code" :label="item.name" :value="item.code" />
      </el-select>
      <el-button type="primary" @click="loadData">搜索</el-button>
    </div>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="name" label="名称" min-width="180" />
      <el-table-column prop="type" label="类型" width="150" />
      <el-table-column label="环境" width="120">
        <template #default="{ row }"><el-tag effect="plain">{{ environmentName(row.env) }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="url" label="地址" min-width="260" show-overflow-tooltip />
      <el-table-column label="认证" width="120">
        <template #default="{ row }">{{ row.authType || 'none' }}</template>
      </el-table-column>
      <el-table-column label="健康状态" width="120">
        <template #default="{ row }"><el-tag :type="healthType(row.healthStatus)" effect="light">{{ healthText(row.healthStatus) }}</el-tag></template>
      </el-table-column>
      <el-table-column label="查询延迟" width="110"><template #default="{ row }">{{ row.lastCheckAt ? `${row.latencyMs || 0} ms` : '-' }}</template></el-table-column>
      <el-table-column label="最近检测" width="180"><template #default="{ row }">{{ formatTime(row.lastCheckAt) }}</template></el-table-column>
      <el-table-column prop="lastError" label="最近错误" min-width="180" show-overflow-tooltip />
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="210" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="handleTest(row)">测试</el-button>
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination v-model:current-page="query.pageNum" v-model:page-size="query.pageSize" :total="total" layout="total, sizes, prev, pager, next" @current-change="loadData" @size-change="loadData" />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑数据源' : '新增数据源'" width="760px">
      <el-form label-width="110px">
        <el-form-item label="名称" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio-button label="prometheus">Prometheus</el-radio-button>
            <el-radio-button label="victoriametrics">VictoriaMetrics</el-radio-button>
            <el-radio-button label="victorialogs">VictoriaLogs</el-radio-button>
            <el-radio-button label="elasticsearch">Elasticsearch</el-radio-button>
            <el-radio-button label="sls">阿里云 SLS</el-radio-button><el-radio-button label="jaeger">Jaeger</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="所属环境" required>
          <el-select v-model="form.env" :loading="environmentLoading" style="width: 100%" placeholder="请选择环境">
            <el-option v-for="item in environmentOptions" :key="item.code" :label="`${item.name} / ${item.code}`" :value="item.code" />
          </el-select>
        </el-form-item>
        <el-form-item label="地址" required><el-input v-model="form.url" :placeholder="form.type === 'elasticsearch' ? 'http://elasticsearch:9200' : (form.type === 'victorialogs' ? 'http://victorialogs:9428' : (form.type === 'jaeger' ? 'http://jaeger-query:16686' : 'http://prometheus:9090'))" /></el-form-item>
        <el-alert v-if="form.type === 'elasticsearch'" title="Elasticsearch 数据源用于日志查询与日志告警；PromQL 即时查询、监控大屏和巡检大屏仍需选择 Prometheus 或 VictoriaMetrics。" type="info" :closable="false" style="margin-bottom: 18px" />
        <el-alert v-else-if="form.type === 'victorialogs'" title="VictoriaLogs 数据源用于 LogsQL 日志查询；PromQL 即时查询、监控大屏和巡检大屏仍需选择 Prometheus 或 VictoriaMetrics。" type="info" :closable="false" style="margin-bottom: 18px" />
        <el-alert v-else-if="form.type === 'jaeger'" title="Jaeger 数据源用于链路追踪连接管理和健康检查；当前 PromQL、日志查询与监控大屏不使用此数据源。请填写 Jaeger Query 服务地址，例如 http://jaeger-query:16686。" type="info" :closable="false" style="margin-bottom: 18px" />
        <template v-if="form.type === 'sls'">
          <el-form-item label="地域" required><el-input v-model="form.sls.region" placeholder="cn-hangzhou" /></el-form-item>
          <el-form-item label="Project" required><el-input v-model="form.sls.project" placeholder="SLS 项目名称" /></el-form-item>
          <el-form-item label="阿里云账号" required><el-select v-model="form.sls.cloudAccountId" filterable placeholder="选择云账号管理中已配置 AK/SK 的账号"><el-option v-for="account in cloudAccounts" :key="account.id" :label="account.name" :value="account.id" /></el-select></el-form-item>
          <el-alert title="此数据源绑定一个 Project。保存后，在日志查询页浏览和选择该 Project 下的 Logstore。" type="info" :closable="false" />
          <el-collapse><el-collapse-item title="高级：Logstore 访问范围与默认项" name="logstore-scope">
          <el-form-item label="Logstore 范围"><el-select v-model="form.sls.logstores" multiple filterable allow-create default-first-option placeholder="输入名称后回车；留空允许 Project 下全部 Logstore" /></el-form-item>
          <el-form-item label="默认 Logstore"><el-input v-model="form.sls.defaultLogstore" placeholder="可选，必须在允许范围内" /></el-form-item>
          </el-collapse-item></el-collapse>
          <el-alert title="使用地域级 HTTPS Endpoint，支持同地域内网地址。账号需具备日志库读取、日志检索和分布统计权限。仅查询现有日志，不创建采集配置。" type="info" :closable="false" />
        </template>
        <el-form-item v-if="form.type !== 'sls'" label="认证方式">
          <el-select v-model="form.authType" style="width: 100%">
            <el-option label="无认证" value="none" />
            <el-option label="Basic" value="basic" />
            <el-option label="Bearer Token" value="bearer" />
            <el-option label="Elasticsearch API Key" value="apikey" />
          </el-select>
        </el-form-item>
        <template v-if="form.type !== 'sls' && form.authType === 'basic'">
          <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
          <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
        </template>
        <el-form-item v-if="form.type !== 'sls' && (form.authType === 'bearer' || form.authType === 'apikey')" :label="form.authType === 'apikey' ? 'API Key' : 'Token'">
          <el-input v-model="form.token" type="textarea" :rows="3" />
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
        <el-button :loading="testing" @click="handleTest()">测试连接</el-button>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.monitor-page { display: flex; flex-direction: column; gap: 18px; padding: 24px; background: #fff; border-radius: 18px; box-shadow: 0 12px 30px rgba(36, 54, 90, 0.08); }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.page-header h2 { margin: 0 0 8px; font-size: 26px; color: #10213f; }
.page-header p { margin: 0; color: #7282a0; }
.toolbar { display: flex; flex-wrap: wrap; gap: 12px; }
.pager { display: flex; justify-content: flex-end; }
</style>
