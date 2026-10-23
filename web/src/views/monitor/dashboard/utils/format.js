function metricText(metric) {
  return Object.entries(metric || {})
    .map(([key, value]) => `${key}="${value}"`)
    .join(', ')
}

function metricName(metric) {
  if (!metric) return 'metric'
  if (metric.namespace && metric.pod) return `${metric.namespace}/${metric.pod}`
  return metric.node || metric.instance || metric.pod || metric.namespace || metric.deployment || metric.reason || metric.job || metric.__name__ || 'metric'
}

function numberValue(row) {
	const raw = row?.value?.[1] ?? row?.values?.[row.values.length - 1]?.[1]
	const value = Number(raw)
  return Number.isFinite(value) ? value : 0
}

function formatByUnit(value, unit = '') {
  if (!Number.isFinite(value)) return '-'
  if (unit === 'Core') {
    return Math.abs(value) < 1
      ? `${(value * 1000).toFixed(Math.abs(value * 1000) >= 10 ? 0 : 1)} m`
      : `${value.toFixed(2).replace(/\.?0+$/, '')} Core`
  }
  if (unit === 'B') {
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
    let current = Math.abs(value)
    let index = 0
    while (current >= 1024 && index < units.length - 1) {
      current /= 1024
      index += 1
    }
    const signed = value < 0 ? -current : current
    return `${signed.toFixed(current >= 100 ? 0 : current >= 10 ? 1 : 2)} ${units[index]}`
  }
  if (unit === 'B/s') {
    const units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s']
    let current = Math.abs(value)
    let index = 0
    while (current >= 1024 && index < units.length - 1) {
      current /= 1024
      index += 1
    }
    const signed = value < 0 ? -current : current
    return `${signed.toFixed(current >= 100 ? 0 : current >= 10 ? 1 : 2)} ${units[index]}`
  }
  if (unit === '天') return `${value.toFixed(value >= 10 ? 0 : 1)} 天`
  if (unit === '%') return `${value.toFixed(value >= 10 ? 1 : 2).replace(/\.?0+$/, '')}%`
  if (unit && unit !== '个' && unit !== '台') return `${value.toFixed(value >= 100 ? 0 : 2).replace(/\.?0+$/, '')}${unit}`
  return value.toLocaleString(undefined, { maximumFractionDigits: value >= 100 ? 0 : 2 })
}

function formatPodResourceValue(value, unit) {
  return Number.isFinite(Number(value)) ? formatByUnit(Number(value), unit) : '-'
}

function reducerLabel(value) {
  return { last: '末值', max: '最大值', min: '最小值', avg: '平均值', p95: 'P95', sum: '合计', increase: '增量', count: '样本数' }[value] || value || '末值'
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function formatReportTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN', { hour12: false })
}

export { metricText, metricName, numberValue, formatByUnit, formatPodResourceValue, reducerLabel, escapeHtml, formatReportTime }
