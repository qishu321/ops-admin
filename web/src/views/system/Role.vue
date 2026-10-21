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
    tableData.value = (data.list || []).filter(r => !['super-admin', 'member'].includes(r.roleKey))
  } finally {
    loading.value = false
  }
}

async function loadMenus() {
  const data = await queryMenuList()
  menuTree.value = buildTree((data || []).filter(m => m.menuStatus === 1 && !/^system:(dept|post):/.test(m.value || '')).map(m => ({ ...m, disabled: /^(system(?::|$)|logs$)/.test(m.value || '') })))
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
    ElMessage.success('模板已更新')
  } else {
    await addRole(form)
    ElMessage.success('模板已创建')
  }
  dialogVisible.value = false
  await loadData()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确认删除模板 ${row.roleName} 吗？`, '提示', { type: 'warning' })
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
  await ElMessageBox.confirm('此修改会影响仍关联该兼容模板的旧账号；已保存的统一访问策略保持不变。', '保存模板', { confirmButtonText: '保存配置', type: 'warning' })
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
    <h2 class="page-title">权限模板</h2><p class="template-explanation">将常用功能配置保存为模板，新增策略时可复制对应的资源操作。原有角色以兼容模板保留，已关联账号继续生效。</p><el-alert type="info" :closable="false" show-icon title="模板是可选起点。新账号通过访问策略授权，无需再分配模板；系统管理能力仅对平台管理员开放。" />
    <div class="toolbar">
      <div class="toolbar-left"></div>
      <div class="toolbar-right">
        <el-button v-permission="'system:role:add'" type="primary" @click="openCreate">新增模板</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="tableData" border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="roleName" label="模板名称" min-width="160" />
      <el-table-column prop="roleKey" label="模板标识" min-width="160" />
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
          <el-button v-permission="'system:role:assign'" link type="success" @click="openPermission(row)">配置功能</el-button>
          <el-button v-if="!row.isReadOnly" v-permission="'system:role:status'" link type="warning" @click="handleStatus(row)">{{ row.status === 1 ? '停用' : '启用' }}</el-button>
          <el-button v-if="!row.isReadOnly" v-permission="'system:role:delete'" link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑模板' : '新增模板'" width="600px">
      <el-form label-width="90px">
        <el-form-item label="模板名称"><el-input v-model="form.roleName" /></el-form-item>
        <el-form-item label="模板标识"><el-input v-model="form.roleKey" /></el-form-item>
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

    <el-dialog v-model="permissionVisible" title="配置兼容功能" width="520px">
      <el-alert v-if="readonlyRoleForPermission" type="info" :closable="false" show-icon>
        全局只读模板仅允许安全的查看菜单；操作权限和敏感入口会由服务端自动过滤。
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

<style scoped>.template-explanation{font-size:13px;line-height:1.8;color:#718098;margin:0 0 18px}.toolbar{margin-top:20px}</style>
