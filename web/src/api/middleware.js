import http from './http'
export const listMiddleware = type => http.get('/api/v1/asset/middleware/list', { params: { type } })
export const saveMiddleware = data => http.post('/api/v1/asset/middleware/save', data)
export const deleteMiddleware = id => http.delete('/api/v1/asset/middleware/delete', { data: { id } })
export const testMiddleware = data => http.post('/api/v1/asset/middleware/test', data, { timeout: 20000 })
export const operateMiddleware = (id, operation, params, action = 'query', confirmed = false) => http.post(`/api/v1/asset/middleware/${action === 'delete' ? 'remove' : action}`, { id, operation, params, confirmed }, { timeout: 20000 })
