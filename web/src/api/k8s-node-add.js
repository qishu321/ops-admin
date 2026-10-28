import http from './http'
const base = '/api/v1/k8s/node-add'
const executionPayload = data => ({
  clusterId: data.clusterId,
  nodes: data.nodes.map(node => ({ hostId: node.hostId, nodeName: node.nodeName })),
  labels: data.labels || '', taints: data.taints || '', retryOf: data.retryOf || 0,
  precheckId: data.precheckId, idempotencyKey: data.idempotencyKey
})
export const nodeAddContext = clusterId => http.get(`${base}/context`, { params: { clusterId }, timeout: 60000 })
export const nodeAddHosts = params => http.get(`${base}/hosts`, { params, timeout: 60000 })
export const nodeAddPrecheck = data => http.post(`${base}/prechecks`, executionPayload(data), { timeout: 60000 })
export const nodeAddStart = data => http.post(`${base}/tasks`, executionPayload(data), { timeout: 60000 })
export const nodeAddTasks = clusterId => http.get(`${base}/tasks`, { params: { clusterId } })
export const nodeAddTask = (clusterId, id) => http.get(`${base}/tasks/${id}`, { params: { clusterId } })
export const nodeAddCancel = (clusterId, id) => http.post(`${base}/tasks/${id}/cancel`, { clusterId })
