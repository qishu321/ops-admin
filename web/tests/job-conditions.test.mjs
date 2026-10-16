import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import { conditionSummary, validateJobBranches } from '../src/views/ops/job-conditions.js'

function definition() {
  return {
    nodes: [
      { id: 'check', type: 'script', label: '健康检查', config: { failurePolicy: 'branch' } },
      { id: 'choice', type: 'condition', label: '全部成功？', config: { sourceStepId: 'check', field: 'status', operator: 'eq', value: 'success' } },
      { id: 'publish', type: 'script', label: '发布', config: {} },
      { id: 'manual', type: 'approval', label: '人工处理', config: {} }
    ],
    edges: [{ source: 'check', target: 'choice' }, { source: 'choice', target: 'publish', branch: 'true' }, { source: 'choice', target: 'manual', branch: 'false' }]
  }
}

test('conditional job accepts an explicit failure handler and both exclusive paths', () => {
  assert.equal(validateJobBranches(definition()), '')
  assert.equal(conditionSummary({ field: 'failedCount', operator: 'gte', value: 2 }), '失败主机数 大于或等于 2')
  assert.equal(conditionSummary({ field: 'status', operator: 'ne', value: 'success' }), '执行状态 不等于 全部成功')
})

test('conditional job blocks incomplete paths, wrong source and invalid comparisons', () => {
  const missing = definition(); missing.edges.pop()
  assert.match(validateJobBranches(missing), /出口/)
  const wrongSource = definition(); wrongSource.nodes[1].config.sourceStepId = 'publish'
  assert.ok(validateJobBranches(wrongSource))
  const numeric = definition(); numeric.nodes[1].config = { sourceStepId: 'check', field: 'failedCount', operator: 'gt', value: -1 }
  assert.match(validateJobBranches(numeric), /非负整数/)
  const invalidStatus = definition(); invalidStatus.nodes[1].config.operator = 'gt'
  assert.match(validateJobBranches(invalidStatus), /执行状态/)
  const unsafe = definition(); unsafe.edges.push({ source: 'check', target: 'publish' })
  assert.match(validateJobBranches(unsafe), /所有后续步骤/)
})

test('designer serializes branch port identities without losing the false path', () => {
  const page = readFileSync(new URL('../src/views/ops/OpsJobDesigner.vue', import.meta.url), 'utf8')
  const serialize = page.slice(page.indexOf('function serializeDefinition()'), page.indexOf('function getGraphJson()'))
  const data = definition()
  const byId = new Map(data.nodes.map((node) => [node.id, node]))
  const context = vm.createContext({
    graph: {
      getNodes: () => data.nodes.map((data) => ({ id: data.id, getData: () => data, position: () => ({ x: 50, y: 60 }), size: () => ({ width: 248, height: 148 }) })),
      getEdges: () => data.edges.map((edge) => ({ getSourceCellId: () => edge.source, getTargetCellId: () => edge.target, getSourcePortId: () => edge.branch || 'out', getSourceNode: () => ({ getData: () => byId.get(edge.source) }) }))
    },
    normalizeNodeTargets: structuredClone, cloneValue: structuredClone
  })
  const saved = JSON.parse(JSON.stringify(vm.runInContext(`${serialize}\nserializeDefinition()`, context)))
  assert.deepEqual(saved.edges, data.edges)
  assert.equal(saved.nodes[0].config.failurePolicy, 'branch')
  assert.deepEqual(saved.nodes[1].config, data.nodes[1].config)
  assert.equal(validateJobBranches(saved), '')
})
