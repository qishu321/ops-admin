import http from './http'
export const listCommandFilters = () => http.get('/api/v1/asset/command-filter/list')
export const saveCommandFilter = data => http.post('/api/v1/asset/command-filter/save', data)
export const deleteCommandFilter = id => http.delete('/api/v1/asset/command-filter/delete', { data: { id } })
