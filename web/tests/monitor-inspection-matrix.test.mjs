import test from 'node:test'
import assert from 'node:assert/strict'
import { buildInspectionMatrix, inspectionMatrixState } from '../src/views/monitor/dashboard/utils/inspectionMatrix.js'

const sources = [{ id: 1, name: '上海' }, { id: 2, name: '压测' }, { id: 3, name: '测试' }]
const result = (id, source, state = 'healthy', title = `规则${id}`) => ({
  id, title, status: 1, datasourceId: source, datasourceName: sources.find(item => item.id === source)?.name,
  rowKey: `result-${id}-${source}`, reportResult: { panelId: id, datasourceId: source, status: state, valueText: `${id}.${source}` },
})

test('three sources share 17 rule rows while retaining all 51 distinct snapshots', () => {
  const rows = sources.flatMap(source => Array.from({ length: 17 }, (_, index) => result(index + 1, source.id)))
  rows[2].reportResult.status = 'warning'
  rows[19].reportResult.status = 'warning'
  const before = JSON.stringify(rows)
  const matrix = buildInspectionMatrix(rows, sources)
  assert.equal(matrix.rows.length, 17)
  assert.equal(matrix.sources.length, 3)
  assert.equal(matrix.resultCount, 51)
  assert.equal(matrix.rows[0].id, 3, 'the rule with warnings must be shown first')
  assert.deepEqual(matrix.rows[0].cells.map(cell => cell.row.reportResult.valueText), ['3.1', '3.2', '3.3'])
  assert.equal(new Set(matrix.rows.flatMap(row => row.cells.map(cell => cell.key))).size, 51)
  assert.equal(JSON.stringify(rows), before, 'view aggregation cannot rewrite saved reports')
})

test('same titles do not merge different rules, and query failures sort ahead of warnings', () => {
  const matrix = buildInspectionMatrix([result(1, 1, 'healthy', '同名'), result(2, 1, 'warning', '同名'), result(3, 1, 'danger', '错误')], [sources[0]])
  assert.deepEqual(matrix.rows.map(row => row.id), [3, 2, 1])
  assert.equal(matrix.rows.length, 3)
})

test('source and status filters do not mix cells or change the underlying report', () => {
  const rows = [result(1, 1), result(1, 2, 'warning'), result(2, 1, 'danger'), result(2, 2)]
  const warning = buildInspectionMatrix(rows, sources.slice(0, 2), { status: 'warning' })
  assert.equal(warning.resultCount, 1)
  assert.equal(warning.rows.length, 1)
  assert.equal(warning.rows[0].cells[0].matches, false)
  assert.equal(warning.rows[0].cells[0].row.reportResult.valueText, '1.1')
  assert.equal(warning.rows[0].cells[1].matches, true)
  const tokyo = buildInspectionMatrix(rows, sources.slice(0, 2), { datasourceId: 2, status: 'healthy' })
  assert.deepEqual(tokyo.sources.map(source => source.id), [2])
  assert.deepEqual(tokyo.rows.map(row => row.id), [2])
  assert.equal(tokyo.rows[0].cells[0].key, '2:2')
  assert.equal(buildInspectionMatrix(rows, sources, { datasourceId: 99 }).rows.length, 0)
})

test('missing source results and new rules remain uninspected, never copied from another source', () => {
  const saved = result(1, 1)
  const pending = { id: 2, title: '新增规则', status: 1, rowKey: 'rule-2' }
  const matrix = buildInspectionMatrix([saved, pending], sources.slice(0, 2))
  const missing = matrix.rows.find(row => row.id === 1).cells[1]
  assert.equal(missing.state, 'uninspected')
  assert.equal(missing.row.reportResult, undefined)
  assert.equal(missing.row.datasourceId, 2)
  assert.equal(missing.row.snapshotMissing, true)
  assert.equal(matrix.resultCount, 1)
  assert.ok(matrix.rows.find(row => row.id === 2).cells.every(cell => cell.missing && cell.state === 'uninspected'))
})

test('disabled, unknown and legacy single-source rows render without claiming success', () => {
  assert.equal(inspectionMatrixState(undefined), 'uninspected')
  assert.equal(inspectionMatrixState({ status: 0 }), 'disabled')
  assert.equal(inspectionMatrixState(result(1, 1, 'future-state')), 'uninspected')
  const legacy = buildInspectionMatrix([result(1, 1)], [sources[0]])
  assert.equal(legacy.resultCount, 1)
  assert.equal(legacy.rows[0].cells[0].state, 'healthy')
  assert.deepEqual(buildInspectionMatrix([], sources).rows, [])
})
