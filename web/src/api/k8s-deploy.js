import http from './http'

const base = '/api/v1/k8s/deploy'
export const deployCapabilities = () => http.get(`${base}/capabilities`)
export const deployList = (params) => http.get(`${base}/list`, { params })
export const deployInfo = (id) => http.get(`${base}/info`, { params: { id } })
export const deployHosts = (params) => http.get(`${base}/host-options`, { params })
export const deploySave = (data) => http.post(`${base}/save`, data)
export const deployPrecheck = (id, revision) => http.post(`${base}/precheck`, { id, revision })
export const deployStart = (id, revision) => http.post(`${base}/start`, { id, revision })
export const deployStop = (id) => http.post(`${base}/stop`, { id })
export const deployDelete = (id) => http.post(`${base}/delete`, { id })
export const deployRegister = (id) => http.post(`${base}/register-cluster`, { id })
export const deployLogs = (id, runId, after = 0) => http.get(`${base}/logs`, { params: { id, runId, after } })
