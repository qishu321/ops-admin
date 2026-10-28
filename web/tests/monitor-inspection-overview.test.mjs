import test from 'node:test'
import assert from 'node:assert/strict'
import { inspectionSourceSummaries, inspectionSourceState } from '../src/views/monitor/dashboard/utils/inspectionReport.js'

test('source overview counts saved results separately for the same rule across sources', () => {
  const run = { datasourceId: 1, datasourceName: '上海旧名称', datasourceIds: [1, 2], datasourceNames: ['上海快照', '东京快照'] }
  const results = [{ panelId: 10, datasourceId: 1, status: 'healthy' }, { panelId: 11, datasourceId: 1, status: 'warning' }, { panelId: 10, datasourceId: 2, status: 'danger' }]
  const sources = inspectionSourceSummaries(run, results)
  assert.deepEqual(sources, [
    { id: 1, name: '上海快照', total: 2, healthy: 1, warning: 1, danger: 0, unknown: 0 },
    { id: 2, name: '东京快照', total: 1, healthy: 0, warning: 0, danger: 1, unknown: 0 }
  ])
  assert.deepEqual(inspectionSourceState(sources[0]), { key: 'warning', label: '需要核查' })
  assert.equal(inspectionSourceState(sources[1]).key, 'danger')
  assert.equal(results.length, 3, 'aggregation must not alter report snapshots')
})

test('legacy single-source reports use the saved header when result source fields are missing', () => {
  const sources = inspectionSourceSummaries({ datasourceId: 7, datasourceName: '历史数据源' }, [{ panelId: 1, status: 'healthy' }])
  assert.equal(sources[0].id, 7)
  assert.equal(sources[0].name, '历史数据源')
  assert.equal(sources[0].healthy, 1)
  assert.equal(inspectionSourceState(sources[0]).key, 'healthy')
})

test('missing or unrecognized results are never presented as all healthy', () => {
  assert.deepEqual(inspectionSourceSummaries(null, []), [])
  const sources = inspectionSourceSummaries({ datasourceIds: [1, 2, 3] }, [{ datasourceId: 2, status: 'unknown-future-state' }, { datasourceId: 3, status: 'danger' }, { datasourceId: 3, status: 'warning' }])
  assert.equal(inspectionSourceState(sources[0]).key, 'uninspected')
  assert.equal(inspectionSourceState(sources[1]).key, 'uninspected')
  assert.equal(sources[1].unknown, 1)
  assert.equal(inspectionSourceState(sources[2]).key, 'danger')
  sources.forEach(source => assert.equal(source.total, source.healthy + source.warning + source.danger + source.unknown))
})
