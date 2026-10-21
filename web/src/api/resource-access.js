import http from './http'

export const getResourceAccessData = () => http.get('/api/v1/system/resource-access/data')
export const getResourceAccessOptions = () => http.get('/api/v1/system/resource-access/options')
export const saveUserGroup = data => http.post('/api/v1/system/user-group/save', data)
export const deleteUserGroup = id => http.delete('/api/v1/system/user-group/delete', { data: { id } })
export const saveResourceGrant = data => http.post('/api/v1/system/resource-access/grant', data)
export const deleteResourceGrant = id => http.delete('/api/v1/system/resource-access/grant', { data: { id } })

export const saveAccessPolicy = (data) => http.post('/api/v1/system/access-policies/save', data)
export const getEffectiveAccess = (userId) => http.get('/api/v1/system/access-policies/effective', { params: { userId } })

export const saveGroupBindings = data => http.post('/api/v1/system/user-group/bindings', data)
