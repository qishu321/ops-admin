<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { addRole, assignPermissions, deleteRole, queryGlobalReadOnlyTemplate, queryMenuList, queryRoleList, queryRoleMenuIdList, roleInfo, roleUpdate, updateRoleStatus } from '../../api/system'
import { buildTree } from '../../utils/tree'

const loading = ref(false)
const dialogVisible = ref(false)
const permissionVisible = ref(false)
const isEdit = ref(false)
const tableData = ref([])
const menuTree = ref([])
const roleIdForPermission = ref()
const readonlyRoleForPermission = ref(false)
const treeRef = ref()

const form = reactive({
  id: undefined,
  roleName: '',
  roleKey: '',
  status: 1,
  description: ''
})

function resetForm() {
  Object.assign(form, {
    id: undefined,
    roleName: '',
    roleKey: '',
    status: 1,
    description: ''
  })
}

async function loadData() {
  loading.value = true
  try {
    const data = await queryRoleList()
    tableData.value = data.list || []
  } finally {
    loading.value = false
  }
}

async function loadMenus() {
  const data = await queryMenuList()
  menuTree.value = buildTree(data || [])
}

function openCreate() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row) {
  isEdit.value = true
  const data = await roleInfo(row.id)
  Object.assign(form, data)
  dialogVisible.value = true
}

async function submit() {
  if (isEdit.value) {
    await roleUpdate(form)
    ElMessage.success('角色已更新')
  } else {
    await addRole(form)
    ElMessage.success('角色已创建')
  }
  dialogVisible.value = false
  await loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除角色 ${row.roleName} 吗？`, '提示', { type: 'warning' })
  await deleteRole(row.id)
  ElMessage.success('删除成功')
  await loadData()
}

async function handleStatus(row) {
  await updateRoleStatus(row.id, row.status === 1 ? 2 : 1)
  ElMessage.success('状态已更新')
  await loadData()
}

async function openPermission(row) {
  roleIdForPermission.value = row.id
  readonlyRoleForPermission.value = Boolean(row.isReadOnly)
  if (!menuTree.value.length) {
    await loadMenus()
  }
  permissionVisible.value = true
  const data = await queryRoleMenuIdList(row.id)
  treeRef.value?.setCheckedKeys((data || []).map((item) => item.id))
}

async function applyReadOnlyTemplate() {
  const ids = await queryGlobalReadOnlyTemplate()
  treeRef.value?.setCheckedKeys(ids || [])
}

async function savePermission() {
  const checkedKeys = treeRef.value?.getCheckedKeys() || []
  const halfKeys = treeRef.value?.getHalfCheckedKeys() || []
  await assignPermissions(roleIdForPermission.value, [...new Set([...checkedKeys, ...halfKeys])])
  ElMessage.success('权限分配成功')
  permissionVisible.value = false
}

onMounted(loadData)
</script>

<template>
  <div class="page-card console-card-page">
    <h2 class="page-title">角色管理</h2>
    <div class="toolbar">
      <div class="toolbar-left"></div>
      <div class="toolbar-right">
        <el-button v-permission="'system:role:add'" type="primary" @click="openCreate">新增角色</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="roleName" label="角色名称" min-width="160" />
      <el-table-column prop="roleKey" label="角色标识" min-width="160" />
      <el-table-column label="类型" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.isReadOnly" type="info">全局只读</el-tag>
          <span v-else>自定义</span>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="220" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'danger'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button v-if="!row.isReadOnly" v-permission="'system:role:edit'" link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button v-permission="'system:role:assign'" link type="success" @click="openPermission(row)">分配权限</el-button>
          <el-button v-if="!row.isReadOnly" v-permission="'system:role:status'" link type="warning" @click="handleStatus(row)">{{ row.status === 1 ? '停用' : '启用' }}</el-button>
          <el-button v-if="!row.isReadOnly" v-permission="'system:role:delete'" link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑角色' : '新增角色'" width="600px">
      <el-form label-width="90px">
        <el-form-item label="角色名称"><el-input v-model="form.roleName" /></el-form-item>
        <el-form-item label="角色标识"><el-input v-model="form.roleKey" /></el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :value="1">启用</el-radio>
            <el-radio :value="2">停用</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permissionVisible" title="分配菜单权限" width="520px">
      <el-alert v-if="readonlyRoleForPermission" type="info" :closable="false" show-icon>
        全局只读角色仅允许安全的查看菜单；操作权限和敏感入口会由服务端自动过滤。
      </el-alert>
      <el-tree
        ref="treeRef"
        :data="menuTree"
        show-checkbox
        node-key="id"
        check-strictly
        default-expand-all
        :props="{ label: 'menuName', children: 'children' }"
      />
      <template #footer>
        <el-button v-if="readonlyRoleForPermission" @click="applyReadOnlyTemplate">恢复全局只读模板</el-button>
        <el-button @click="permissionVisible = false">取消</el-button>
        <el-button type="primary" @click="savePermission">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
