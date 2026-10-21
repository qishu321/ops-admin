<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { getResourceAccessData } from '../../api/resource-access'
const router = useRouter()
import { addAdmin, adminInfo, adminUpdate, deleteAdmin, previewLDAPUsers, queryAdminList, resetPassword, syncLDAPUsers, updateAdminStatus } from '../../api/system'

const loading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const tableData = ref([])
const total = ref(0)
const groups = ref([])
const ldapDialogVisible = ref(false)
const ldapLoading = ref(false)
const ldapSyncing = ref(false)
const ldapKeyword = ref('')
const ldapUsers = ref([])
const selectedLDAPUsers = ref([])

const query = reactive({
  pageNum: 1,
  pageSize: 10,
  username: '',
  status: ''
})

const form = reactive({
  id: undefined,
  username: '',
  password: '',
  nickname: '',
 accountType: 'member', groupIds: [],
  email: '',
  phone: '',
  note: '',
  status: 1
})

function resetForm() {
  Object.assign(form, {
    id: undefined,
    username: '',
    password: '',
    nickname: '',
 accountType: 'member', groupIds: [],
    email: '',
    phone: '',
    note: '',
    status: 1
  })
}

async function loadOptions() { const data = await getResourceAccessData(); groups.value = data.groups || [] }
function groupNames(ids) { return (ids || []).map(id => groups.value.find(g => g.id === id)?.name).filter(Boolean).join('、') || '尚未加入用户组' }

async function loadData() {
  loading.value = true
  try {
    const data = await queryAdminList(query)
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
  const data = await adminInfo(row.id)
  Object.assign(form, data, { password: '' })
  dialogVisible.value = true
}

async function submit() {
  if (isEdit.value) {
    await adminUpdate(form)
    ElMessage.success('更新成功')
  } else {
    await addAdmin(form)
    ElMessage.success('创建成功')
  }
  dialogVisible.value = false
  await loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除用户 ${row.username} 吗？`, '提示', { type: 'warning' })
  await deleteAdmin(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

async function toggleStatus(row) {
  const next = row.status === 1 ? 2 : 1
  await updateAdminStatus(row.id, next)
  ElMessage.success('状态已更新')
  await loadData()
}

async function handleResetPassword(row) {
  await resetPassword(row.id, '123456')
  ElMessage.success('密码已重置为 123456')
}

async function openLDAPSync() {
  ldapDialogVisible.value = true
  ldapKeyword.value = ''
  selectedLDAPUsers.value = []
  await loadLDAPUsers()
}

async function loadLDAPUsers() {
  ldapLoading.value = true
  try {
    ldapUsers.value = await previewLDAPUsers(ldapKeyword.value) || []
    selectedLDAPUsers.value = []
  } finally {
    ldapLoading.value = false
  }
}

async function submitLDAPSync() {
  if (!selectedLDAPUsers.value.length) {
    ElMessage.warning('请至少选择一个 LDAP 用户')
    return
  }
  ldapSyncing.value = true
  try {
    const result = await syncLDAPUsers(selectedLDAPUsers.value)
    ElMessage.success(`LDAP 同步完成：新增 ${result.created || 0}，更新 ${result.updated || 0}`)
    ldapDialogVisible.value = false
    await loadData()
  } finally {
    ldapSyncing.value = false
  }
}

onMounted(async () => {
  await loadOptions()
  await loadData()
})
</script>

<template>
  <div class="page-card console-card-page">
    <div class="users-heading"><div><span>IDENTITY & ACCESS</span><h2 class="page-title">用户管理</h2><p>管理账号与团队归属，资源权限统一在访问策略中配置。</p></div><el-button @click="router.push('/system/access-policies')">管理访问策略 <el-icon><Right /></el-icon></el-button></div>
    <div class="toolbar">
      <div class="toolbar-left">
        <el-input v-model="query.username" placeholder="按用户名搜索" clearable style="width: 220px" />
        <el-select v-model="query.status" placeholder="状态" clearable style="width: 140px">
          <el-option label="启用" :value="1" />
          <el-option label="停用" :value="2" />
        </el-select>
        <el-button type="primary" @click="loadData">查询</el-button>
      </div>
      <div class="toolbar-right">
        <el-button v-permission="'system:admin:ldapSync'" @click="openLDAPSync">从 LDAP 同步</el-button>
        <el-button v-permission="'system:admin:add'" type="primary" @click="openCreate">新增用户</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="username" label="账号" min-width="120" />
      <el-table-column prop="nickname" label="昵称" min-width="120" />
      <el-table-column label="账号类型" min-width="140"><template #default="{ row }"><el-tag :type="row.accountType === 'administrator' ? 'warning' : 'info'" effect="plain">{{ row.accountType === 'administrator' ? '平台管理员' : '普通成员' }}</el-tag></template></el-table-column>
 <el-table-column label="用户组" min-width="190" show-overflow-tooltip><template #default="{ row }">{{ groupNames(row.groupIds) }}</template></el-table-column>
      <el-table-column prop="email" label="邮箱" min-width="180" />
      <el-table-column prop="phone" label="手机号" min-width="140" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'danger'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="400" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="router.push({ path: '/system/access-policies', query: { userId: row.id } })">权限预览</el-button>
          <el-button v-permission="'system:admin:edit'" link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button v-if="row.username !== 'admin'" v-permission="'system:admin:status'" link type="warning" @click="toggleStatus(row)">{{ row.status === 1 ? '停用' : '启用' }}</el-button>
          <el-button v-permission="'system:admin:resetpwd'" link type="success" @click="handleResetPassword(row)">重置密码</el-button>
          <el-button v-if="row.username !== 'admin'" v-permission="'system:admin:delete'" link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div style="margin-top:16px;display:flex;justify-content:flex-end;">
      <el-pagination
        v-model:current-page="query.pageNum"
        v-model:page-size="query.pageSize"
        :total="total"
        layout="total, prev, pager, next, sizes"
        @current-change="loadData"
        @size-change="loadData"
      />
    </div>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑用户' : '新增用户'" width="720px">
      <el-form label-width="90px">
        <el-row :gutter="16">
          <el-col :span="12"><el-form-item label="账号"><el-input v-model="form.username" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="昵称"><el-input v-model="form.nickname" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="密码"><el-input v-model="form.password" type="password" show-password :placeholder="isEdit ? '留空则不修改' : '请输入密码'" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="账号类型"><el-select v-model="form.accountType" :disabled="form.username === 'admin'" style="width:100%"><el-option label="普通成员" value="member" /><el-option label="平台管理员" value="administrator" /></el-select></el-form-item></el-col>
 <el-col :span="24"><el-form-item label="用户组"><el-select v-model="form.groupIds" multiple filterable placeholder="选择用户组，继承组内访问策略" style="width:100%"><el-option v-for="g in groups" :key="g.id" :label="g.name + (g.status === 1 ? '' : '（已停用）')" :value="g.id" /></el-select></el-form-item></el-col>
 <el-col :span="24"><el-alert :closable="false" :type="form.accountType === 'administrator' ? 'warning' : 'info'">{{ form.accountType === 'administrator' ? '平台管理员可管理全部资源、用户和系统配置。' : '普通成员通过用户组或个人访问策略获得权限，无需再单独分配角色。' }}</el-alert><p v-if="isEdit && form.roleId && form.accountType === 'member'" class="compat-note">现有账号的兼容角色关联会保留；仅修改资料或用户组不会重置原有权限。</p></el-col>
          <el-col :span="12"><el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="手机号"><el-input v-model="form.phone" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio :value="1">启用</el-radio><el-radio :value="2">停用</el-radio></el-radio-group></el-form-item></el-col>
          <el-col :span="24"><el-form-item label="备注"><el-input v-model="form.note" type="textarea" :rows="3" /></el-form-item></el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button :type="'primary'" @click="submit">{{ isEdit ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="ldapDialogVisible" title="从 LDAP 同步用户" width="900px" destroy-on-close>
      <div class="ldap-dialog-tip">先从目录服务预览用户，再勾选同步。已有本地用户仅更新昵称、邮箱和手机号，不会覆盖密码、角色与状态。</div>
      <div class="ldap-toolbar">
        <el-input v-model="ldapKeyword" placeholder="按用户名过滤 LDAP 用户" clearable @keyup.enter="loadLDAPUsers" />
        <el-button :loading="ldapLoading" @click="loadLDAPUsers">查询 LDAP</el-button>
      </div>
      <el-table v-loading="ldapLoading" :data="ldapUsers" row-key="username" @selection-change="(rows) => { selectedLDAPUsers = rows.map((item) => item.username) }">
        <el-table-column type="selection" width="52" :reserve-selection="true" />
        <el-table-column prop="username" label="用户名" min-width="160" />
        <el-table-column prop="nickname" label="显示名" min-width="160" />
        <el-table-column prop="email" label="邮箱" min-width="210" />
        <el-table-column prop="phone" label="手机号" min-width="150" />
        <el-table-column prop="dn" label="DN" min-width="260" show-overflow-tooltip />
      </el-table>
      <template #footer>
        <el-button @click="ldapDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="ldapSyncing" @click="submitLDAPSync">同步已选用户（{{ selectedLDAPUsers.length }}）</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ldap-dialog-tip { margin-bottom: 14px; padding: 11px 13px; color: #486283; background: #f2f7ff; border: 1px solid #dce9ff; border-radius: 6px; }
.ldap-toolbar { display: flex; gap: 12px; margin-bottom: 14px; }
.ldap-toolbar .el-input { width: 320px; }
</style>

<style scoped>
.users-heading { display:flex;justify-content:space-between;align-items:center;margin-bottom:24px;gap:20px;flex-wrap:wrap; }.users-heading span{font-size:11px;font-weight:700;letter-spacing:1px;color:#356ae6}.users-heading h2{margin:8px 0}.users-heading p,.compat-note{color:#718098;font-size:13px;line-height:1.7}.compat-note{margin:10px 0 18px}.users-heading .el-icon{margin-left:8px}
</style>
