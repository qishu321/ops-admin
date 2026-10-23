import { metricName, numberValue, formatByUnit } from '../utils/format.js'
import { lineColors } from '../config/templates.js'

export function useBarChart({ panelRows, isK8sDashboard }) {
  function barRows(panel) {
    const rawRows = panelRows(panel)
    const rootFilesystemRows = ['磁盘使用率 Top', '磁盘使用率'].includes(panel.title)
      ? rawRows.filter((row) => row.metric?.mountpoint === '/')
      : rawRows
    const sourceRows = rootFilesystemRows.length ? rootFilesystemRows : rawRows
    const rowsByResource = new Map()
    sourceRows.forEach((row) => {
      const name = metricName(row.metric)
      const value = numberValue(row)
      const previous = rowsByResource.get(name)
      if (!previous || value > previous.value) {
        rowsByResource.set(name, { name, value, displayValue: formatByUnit(value, panel.unit) })
      }
    })
    const rows = Array.from(rowsByResource.values())
      .sort((a, b) => b.value - a.value)
      .slice(0, isK8sDashboard.value ? 10 : 8)
    const max = Math.max(...rows.map((item) => item.value), 1)
    return rows.map((item, index) => ({
      ...item,
      color: lineColors[index % lineColors.length],
      percent: item.value === 0 ? 0 : Math.max(4, (item.value / max) * 100),
    }))
  }

  return {
    barRows,
  }
}
