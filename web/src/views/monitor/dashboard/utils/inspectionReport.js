// Aggregate saved report snapshots, never the editable source/rule selections.
export function inspectionSourceSummaries(run, results = []) {
  if (!run) return []
  const ids = run.datasourceIds?.length ? run.datasourceIds : run.datasourceId ? [run.datasourceId] : []
  const sources = new Map()
  const create = (id, name) => ({ id, name: name || `数据源 #${id}`, total: 0, healthy: 0, warning: 0, danger: 0, unknown: 0 })
  ids.forEach((id, index) => sources.set(Number(id), create(Number(id), run.datasourceNames?.[index] || (Number(id) === Number(run.datasourceId) ? run.datasourceName : ''))))
  results.forEach(result => {
    const id = Number(result.datasourceId || run.datasourceId)
    if (!sources.has(id)) sources.set(id, create(id, result.datasourceName))
    const source = sources.get(id)
    source.total++
    source[['healthy', 'warning', 'danger'].includes(result.status) ? result.status : 'unknown']++
  })
  return Array.from(sources.values())
}

export function inspectionSourceState(source) {
  if (source.danger) return { key: 'danger', label: '存在异常' }
  if (source.warning) return { key: 'warning', label: '需要核查' }
  if (!source.total || source.unknown) return { key: 'uninspected', label: '结果待确认' }
  return { key: 'healthy', label: '全部正常' }
}
