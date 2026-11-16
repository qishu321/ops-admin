import axios from 'axios'
import http from './http'

// 公开大屏不能复用全局 http 实例：它对 401 的默认行为是跳转登录页。
const publicHttp = axios.create({ baseURL: '', timeout: 60000 })

function normalizePublicError(error) {
  const payload = error?.response?.data
  if (payload) {
    const wrapped = new Error(payload.message || '请求失败')
    wrapped.code = payload.code || error.response.status
    wrapped.status = error.response.status
    return wrapped
  }
  return error
}

function unwrapPublic(response) {
  const payload = response?.data
  if (payload?.code !== 200) {
    const error = new Error(payload?.message || '请求失败')
    error.code = payload?.code
    throw error
  }
  return payload.data
}

export const queryLogBoardShares = () => http.get('/api/v1/monitor/log-board/shares')
export const saveLogBoardShare = (data) => http.post('/api/v1/monitor/log-board/shares/save', data)
export const deleteLogBoardShare = (id) => http.delete('/api/v1/monitor/log-board/shares/delete', { data: { id } })
export const updateLogBoardShareStatus = (data) => http.put('/api/v1/monitor/log-board/shares/status', data)
export const regenerateLogBoardShareToken = (id) => http.post('/api/v1/monitor/log-board/shares/token', { id })

export async function queryPublicLogBoardMeta(token) {
  try {
    return unwrapPublic(await publicHttp.get(`/api/v1/public/log-board/${encodeURIComponent(token)}`))
  } catch (error) {
    throw normalizePublicError(error)
  }
}

// 公开页的「最近错误日志」关键字搜索：窗口跟随页面时间范围，最多 12 小时。
export async function searchPublicLogBoardRecent(token, params = {}) {
  const payload = {
    datasourceId: Number(params.datasourceId || 0),
    query: String(params.query || ''),
    keywords: Array.isArray(params.keywords) ? params.keywords : [],
    endAt: Number(params.endAt || 0),
    rangeSeconds: Number(params.rangeSeconds || 0)
  }
  try {
    return unwrapPublic(await publicHttp.post(`/api/v1/public/log-board/${encodeURIComponent(token)}/recent`, payload))
  } catch (error) {
    throw normalizePublicError(error)
  }
}

// 公开页可以自己切换数据源、改 LogsQL、换时间范围，所以参数由页面传入。
export async function queryPublicLogBoardData(token, params = {}) {
  const payload = {
    datasourceId: Number(params.datasourceId || 0),
    query: String(params.query || ''),
    rangeSeconds: Number(params.rangeSeconds || 0)
  }
  try {
    return unwrapPublic(await publicHttp.post(`/api/v1/public/log-board/${encodeURIComponent(token)}/data`, payload))
  } catch (error) {
    throw normalizePublicError(error)
  }
}
