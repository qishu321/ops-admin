<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  addAssetCloudAccount,
  assetCloudAccountInfo,
  deleteAssetCloudAccount,
  queryAssetCloudAccountList,
  updateAssetCloudAccount
} from '../../api/asset'

const router = useRouter()
const loading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const tableData = ref([])
const total = ref(0)
const query = reactive({ pageNum: 1, pageSize: 10, keyword: '', provider: '' })
const form = reactive({ id: undefined, name: '', provider: 'aliyun', accessKey: '', secretKey: '', regions: [], region: '', status: 1, description: '' })

const regionOptions = computed(() => {
  if (form.provider === 'tencent') return [
    'ap-guangzhou', 'ap-shanghai', 'ap-beijing', 'ap-chengdu', 'ap-nanjing',
    'ap-singapore', 'ap-tokyo', 'na-ashburn', 'na-siliconvalley', 'eu-frankfurt'
  ]
  if (form.provider === 'aliyun' || form.provider === 'alicloud') return [
    'cn-guangzhou', 'cn-shenzhen', 'cn-hangzhou', 'cn-shanghai', 'cn-beijing',
    'cn-chengdu', 'cn-hongkong', 'ap-southeast-1', 'ap-northeast-1',
    'us-east-1', 'us-west-1', 'eu-central-1'
  ]
  return []
})

function resetForm() {
  Object.assign(form, { id: undefined, name: '', provider: 'aliyun', accessKey: '', secretKey: '', regions: [], region: '', status: 1, description: '' })
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryAssetCloudAccountList(query)
    tableData.value = data.list || []
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
  const data = await assetCloudAccountInfo(row.id)
  Object.assign(form, data, { secretKey: '', regions: data.regions?.length ? data.regions : (data.region ? data.region.split(/[,，;；\s]+/).filter(Boolean) : []) })
  dialogVisible.value = true
}

async function submit() {
  if (!form.regions.length) {
    ElMessage.warning('请至少选择一个同步地域')
    return
  }
  form.region = form.regions.join(',')
  if (isEdit.value) {
    await updateAssetCloudAccount(form)
    ElMessage.success('云账号已更新')
  } else {
    await addAssetCloudAccount(form)
    ElMessage.success('云账号已创建')
  }
  dialogVisible.value = false
  await loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除云账号 ${row.name} 吗？`, '提示', { type: 'warning' })
  await deleteAssetCloudAccount(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

onMounted(loadData)
</script>

<template>
  <div class="page-card asset-card-page">
    <h2 class="page-title">云账号管理</h2>
    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.keyword" clearable placeholder="搜索名称 / AccessKey" style="width: 240px" @keyup.enter="loadData" />
        <el-select v-model="query.provider" clearable placeholder="云厂商" style="width: 140px">
          <el-option label="阿里云" value="aliyun" />
          <el-option label="腾讯云" value="tencent" />
          <el-option label="华为云" value="huawei" />
          <el-option label="百度云" value="baidu" />
          <el-option label="AWS" value="aws" />
        </el-select>
        <el-button @click="loadData">查询</el-button>
      </div>
      <div class="toolbar-right">
        <el-button @click="router.push('/assets/server/sync')">资产同步</el-button><el-button type="primary" @click="openCreate">新增云账号</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="账号名称" min-width="180" />
      <el-table-column prop="provider" label="云厂商" width="120" />
      <el-table-column prop="accessKey" label="AccessKey" min-width="220" />
      <el-table-column label="同步地域" min-width="220">
        <template #default="{ row }">
          <el-space wrap>
            <el-tag v-for="region in (row.regions?.length ? row.regions : (row.region ? row.region.split(/[,，;；\s]+/).filter(Boolean) : []))" :key="region" size="small">{{ region }}</el-tag>
            <span v-if="!row.regions?.length && !row.region">未配置</span>
          </el-space>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'danger'">{{ row.status === 1 ? '正常' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="备注" min-width="220" />
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button v-if="['aliyun', 'alicloud', 'tencent', 'tencentcloud'].includes(row.provider)" link type="primary" :disabled="row.status !== 1" @click="router.push({ path: '/assets/server/sync', query: { cloudAccountId: row.id } })">同步资产</el-button><el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="query.pageNum"
        v-model:page-size="query.pageSize"
        layout="total, sizes, prev, pager, next"
        :total="total"
        @current-change="loadData"
        @size-change="loadData"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑云账号' : '新增云账号'" width="620px">
      <el-form label-width="96px">
        <el-form-item label="账号名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="云厂商">
          <el-select v-model="form.provider" style="width: 100%">
            <el-option label="阿里云" value="aliyun" />
            <el-option label="腾讯云" value="tencent" />
            <el-option label="华为云" value="huawei" />
            <el-option label="百度云" value="baidu" />
            <el-option label="AWS" value="aws" />
          </el-select>
        </el-form-item>
        <el-form-item label="AccessKey"><el-input v-model="form.accessKey" /></el-form-item>
        <el-form-item label="SecretKey"><el-input v-model="form.secretKey" show-password :placeholder="isEdit ? '不填写则保持不变' : ''" /></el-form-item>
        <el-form-item label="同步地域">
          <el-select
            v-model="form.regions"
            multiple
            filterable
            allow-create
            default-first-option
            clearable
            placeholder="可选择或输入多个地域，例如广州、上海、新加坡"
            style="width: 100%"
          >
            <el-option v-for="region in regionOptions" :key="region" :label="region" :value="region" />
          </el-select>
          <div class="form-tip">必选。可配置多个地域；主机同步时仅查询这里配置的地域。</div>
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :value="1">正常</el-radio>
            <el-radio :value="2">停用</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="form.description" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-card { min-height: 300px; }
.page-title { margin-bottom: 14px; }
.toolbar { padding: 12px; border: 1px solid #e8edf3; border-radius: 9px; background: #f9fafc; }
.pager { margin-top: 16px; padding-top: 14px; border-top: 1px solid #edf0f5; display: flex; justify-content: flex-end; }
.form-tip {
  width: 100%;
  margin-top: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 18px;
}
</style>
