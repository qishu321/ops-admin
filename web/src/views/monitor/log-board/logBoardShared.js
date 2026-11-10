// 日志大屏内部页面和公开分享页共用的常量与格式化函数。
// 两个页面展示同一套面板，避免样式和口径各自漂移。

export const LOG_BOARD_TIME_RANGES = [
  { label: '15 分钟', seconds: 15 * 60 },
  { label: '1 小时', seconds: 3600 },
  { label: '6 小时', seconds: 6 * 3600 },
  { label: '24 小时', seconds: 24 * 3600 },
  { label: '7 天', seconds: 7 * 24 * 3600 }
]

export const LOG_BOARD_DEFAULT_QUERY = 'kubernetes.container_name:"notice"'

export const LOG_BOARD_FIELD_LABELS = {
  exceptionType: '异常类型',
  businessModule: '业务模块',
  platform: '客户端平台',
  appVersion: 'App 版本',
  logType: '日志类型',
  category: '业务分类',
  deviceModel: '设备型号',
  operatingSystem: '操作系统',
  uri: '上报接口',
  remoteAddr: '来源 IP',
  logger: '日志来源',
  thread: '处理线程'
}

export const LOG_BOARD_LEVEL_TONES = { ERROR: '#ef5d65', FATAL: '#c0392b', WARN: '#efa31c', INFO: '#3f8fe8', DEBUG: '#8b98ab', OTHER: '#9aa8bd' }

export function formatLogBoardNumber(value) {
  const number = Number(value || 0)
  if (number >= 100000000) return `${(number / 100000000).toFixed(2)} 亿`
  if (number >= 10000) return `${(number / 10000).toFixed(2)} 万`
  return number.toLocaleString('zh-CN')
}

export function formatLogBoardStep(step) {
  const seconds = Number(step || 0)
  if (!seconds) return '时间桶'
  if (seconds < 60) return `${seconds} 秒`
  if (seconds < 3600) return `${Math.round(seconds / 60)} 分钟`
  return `${Math.round(seconds / 3600)} 小时`
}

export function logBoardBarHeight(value, max) {
  return `${Math.max(0, Math.min(100, (Number(value || 0) / Math.max(1, Number(max || 0))) * 100)).toFixed(2)}%`
}

export function logBoardBarWidth(value, max) {
  const ceiling = Math.max(1, Number(max || 0))
  const percent = (Number(value || 0) / ceiling) * 100
  return `${Math.min(100, Math.max(2, percent)).toFixed(2)}%`
}

export function logBoardClockLabel(ts) {
  if (!ts) return ''
  const date = new Date(Number(ts))
  const pad = value => String(value).padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function logBoardLevelClass(level) {
  const value = String(level || '').toUpperCase()
  if (value === 'ERROR' || value === 'FATAL') return 'is-error'
  if (value === 'WARN' || value === 'WARNING') return 'is-warn'
  if (value === 'INFO') return 'is-info'
  return 'is-other'
}

export function logBoardFieldLabel(key) {
  return LOG_BOARD_FIELD_LABELS[key] || key
}

export function logBoardShortMessage(item) {
  const message = String(item?.summary || item?.message || '')
  return message.length > 200 ? `${message.slice(0, 200)}…` : message
}

// 搜索窗口是 12 小时，凌晨搜到前一天下午很常见；只显示时刻会让人误以为顺序乱了。
export function logBoardLogTime(value) {
  const text = String(value || '')
  return text.length >= 19 ? text.slice(5, 19) : text
}

export function logBoardLevels(data) {
  const list = data?.levels || []
  const total = list.reduce((sum, item) => sum + (item.count || 0), 0) || 1
  return list.map(item => ({ ...item, ratio: Math.round((item.count / total) * 1000) / 10 }))
}

export function logBoardDonutGradient(levels) {
  const items = (levels || []).filter(item => item.count > 0)
  if (!items.length) return 'conic-gradient(#e8eef8 0 100%)'
  const total = items.reduce((sum, item) => sum + item.count, 0)
  let cursor = 0
  const stops = items.map(item => {
    const start = cursor
    cursor += (item.count / total) * 100
    return `${LOG_BOARD_LEVEL_TONES[item.value] || LOG_BOARD_LEVEL_TONES.OTHER} ${start.toFixed(2)}% ${cursor.toFixed(2)}%`
  })
  return `conic-gradient(${stops.join(', ')})`
}

export function logBoardLevelCount(data, names) {
  return (data?.levels || [])
    .filter(item => names.includes(item.value))
    .reduce((sum, item) => sum + (item.count || 0), 0)
}

export function logBoardRangeLabel(seconds) {
  const matched = LOG_BOARD_TIME_RANGES.find(item => item.seconds === Number(seconds))
  return matched ? `最近 ${matched.label}` : `最近 ${Math.round(Number(seconds || 0) / 3600)} 小时`
}

export function logBoardPublicPath(token) {
  return `/public/log-board/${encodeURIComponent(String(token || ''))}`
}

export function logBoardPublicUrl(token, origin) {
  const base = String(origin || (typeof window !== 'undefined' ? window.location.origin : '')).replace(/\/+$/, '')
  return `${base}${logBoardPublicPath(token)}`
}
