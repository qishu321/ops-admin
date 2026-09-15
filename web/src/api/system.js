import http from './http'

export const login = (data) => http.post('/api/v1/login', data)
export const refreshToken = (lastActivityAt) => http.post('/api/v1/auth/refresh', { lastActivityAt }, { skipAuthRefresh: true })
export const logoutSession = () => http.post('/api/v1/auth/logout', {}, { skipAuthRefresh: true })
export const profile = () => http.get('/api/v1/profile')

export const getPublicSystemConfig = () => http.get('/api/v1/systemConfig/public')
export const getSystemConfig = () => http.get('/api/v1/systemConfig')
export const updateSystemConfig = (data) => http.put('/api/v1/systemConfig', data)
export const getLDAPConfig = () => http.get('/api/v1/system/ldap/config')
export const saveLDAPConfig = (data) => http.put('/api/v1/system/ldap/config', data)
export const testLDAPConfig = (data) => http.post('/api/v1/system/ldap/test', data)
export const uploadSystemAsset = (file) => {
  const formData = new FormData()
  formData.append('file', file)
  return http.post('/api/v1/systemConfig/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
}

export const updatePersonal = (data) => http.put('/api/v1/admin/updatePersonal', data)
export const updatePersonalPassword = (data) => http.put('/api/v1/admin/updatePersonalPassword', data)

export const queryAdminList = (params) => http.get('/api/v1/admin/list', { params })
export const addAdmin = (data) => http.post('/api/v1/admin/add', data)
export const adminInfo = (id) => http.get('/api/v1/admin/info', { params: { id } })
export const adminUpdate = (data) => http.put('/api/v1/admin/update', data)
export const deleteAdmin = (id) => http.delete('/api/v1/admin/delete', { data: { id } })
export const updateAdminStatus = (id, status) => http.put('/api/v1/admin/updateStatus', { id, status })
export const resetPassword = (id, password) => http.put('/api/v1/admin/updatePassword', { id, password })
export const previewLDAPUsers = (keyword = '') => http.get('/api/v1/admin/ldap/users', { params: { keyword } })
export const syncLDAPUsers = (usernames) => http.post('/api/v1/admin/ldap/sync', { usernames })

export const queryRoleList = () => http.get('/api/v1/role/list')
export const querySysRoleVoList = () => http.get('/api/v1/role/vo/list')
export const roleInfo = (id) => http.get('/api/v1/role/info', { params: { id } })
export const addRole = (data) => http.post('/api/v1/role/add', data)
export const roleUpdate = (data) => http.put('/api/v1/role/update', data)
export const deleteRole = (id) => http.delete('/api/v1/role/delete', { data: { id } })
export const updateRoleStatus = (id, status) => http.put('/api/v1/role/updateStatus', { id, status })
export const queryRoleMenuIdList = (id) => http.get('/api/v1/role/vo/idList', { params: { id } })
export const queryGlobalReadOnlyTemplate = () => http.get('/api/v1/role/readonlyTemplate')
export const assignPermissions = (id, menuIds) => http.put('/api/v1/role/assignPermissions', { id, menuIds })

export const queryMenuList = () => http.get('/api/v1/menu/list')
export const querySysMenuVoList = () => http.get('/api/v1/menu/vo/list')
export const menuInfo = (id) => http.get('/api/v1/menu/info', { params: { id } })
export const addMenu = (data) => http.post('/api/v1/menu/add', data)
export const menuUpdate = (data) => http.put('/api/v1/menu/update', data)
export const deleteMenu = (id) => http.delete('/api/v1/menu/delete', { data: { id } })

export const queryDeptList = () => http.get('/api/v1/dept/list')
export const querySysDeptVoList = () => http.get('/api/v1/dept/vo/list')
export const deptInfo = (id) => http.get('/api/v1/dept/info', { params: { id } })
export const deptUsers = (deptId) => http.get('/api/v1/dept/users', { params: { deptId } })
export const addDept = (data) => http.post('/api/v1/dept/add', data)
export const deptUpdate = (data) => http.put('/api/v1/dept/update', data)
export const deleteDept = (id) => http.delete('/api/v1/dept/delete', { data: { id } })

export const queryPostList = () => http.get('/api/v1/post/list')
export const querySysPostVoList = () => http.get('/api/v1/post/vo/list')
export const postInfo = (id) => http.get('/api/v1/post/info', { params: { id } })
export const addPost = (data) => http.post('/api/v1/post/add', data)
export const updatePost = (data) => http.put('/api/v1/post/update', data)
export const deletePost = (id) => http.delete('/api/v1/post/delete', { data: { id } })
export const batchDeleteSysPost = (ids) => http.delete('/api/v1/post/batch/delete', { data: { ids } })
export const updatePostStatus = (id, postStatus) => http.put('/api/v1/post/updateStatus', { id, postStatus })

export const querySysLoginInfoList = (params) => http.get('/api/v1/sysLoginInfo/list', { params })
export const deleteSysLoginInfo = (id) => http.delete('/api/v1/sysLoginInfo/delete', { data: { id } })
export const batchDeleteSysLoginInfo = (ids) => http.delete('/api/v1/sysLoginInfo/batch/delete', { data: { ids } })
export const cleanSysLoginInfo = () => http.delete('/api/v1/sysLoginInfo/clean')

export const querySysOperationLogList = (params) => http.get('/api/v1/sysOperationLog/list', { params })
export const deleteSysOperationLog = (id) => http.delete('/api/v1/sysOperationLog/delete', { data: { id } })
export const batchDeleteSysOperationLog = (ids) => http.delete('/api/v1/sysOperationLog/batch/delete', { data: { ids } })
export const cleanSysOperationLog = () => http.delete('/api/v1/sysOperationLog/clean')
