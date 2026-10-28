const priority = { danger: 0, warning: 1, uninspected: 2, healthy: 3, disabled: 4 }

export function inspectionMatrixState(row) {
  if (!row) return 'uninspected'
  if (row.status !== 1) return 'disabled'
  const state = row.reportResult?.status
  return Object.hasOwn(priority, state) ? state : 'uninspected'
}

/** Keep rule identity and source identity separate; filters never rewrite snapshots. */
export function buildInspectionMatrix(rows, sources, { datasourceId = 'all', status = 'all' } = {}) {
  const columns = sources.filter(source => datasourceId === 'all' || Number(source.id) === Number(datasourceId))
  const rules = new Map()
  rows.forEach(row => {
    const id = Number(row.id)
    if (!rules.has(id)) rules.set(id, { id, definition: row, bySource: new Map() })
    if (row.reportResult) rules.get(id).bySource.set(Number(row.datasourceId), row)
  })
  let resultCount = 0
  const matrixRows = Array.from(rules.values()).map(rule => {
    const cells = columns.map(source => {
      const saved = rule.bySource.get(Number(source.id))
      const row = saved || {
        ...rule.definition,
        rowKey: `missing-${rule.id}-${source.id}`,
        reportResult: undefined,
        datasourceId: Number(source.id),
        datasourceName: source.name,
        snapshotMissing: true,
      }
      const state = inspectionMatrixState(row)
      const matches = status === 'all' || state === status
      if (saved && matches) resultCount++
      return { key: `${rule.id}:${source.id}`, row, state, matches, missing: !saved }
    })
    const rank = Math.min(...cells.filter(cell => cell.matches).map(cell => priority[cell.state]))
    return { id: rule.id, title: rule.definition.title, category: rule.definition.category, sort: Number(rule.definition.sort || 0), rank, cells }
  }).filter(rule => rule.cells.some(cell => cell.matches))
    .sort((left, right) => left.rank - right.rank || left.sort - right.sort || left.id - right.id)
  return { sources: columns, rows: matrixRows, resultCount }
}
