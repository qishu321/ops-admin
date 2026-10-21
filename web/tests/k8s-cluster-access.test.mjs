import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const page = readFileSync(new URL('../src/views/assets/K8sClusterManage.vue', import.meta.url), 'utf8')
const script = page.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import\s[\s\S]*?\sfrom\s+['"][^'"]+['"]\s*$/gm, '')

function setup({ administrator = false, response } = {}) {
  const hooks = [], changes = [], calls = [], routes = []
  const context = vm.createContext({
    ref: value => ({ value }), reactive: value => value,
    computed: fn => ({ get value() { return fn() } }),
    onMounted: fn => hooks.push(fn), useAccessRefresh: fn => changes.push(fn),
    useRouter: () => ({ push: target => routes.push(target) }),
    getUser: () => ({ accountType: administrator ? 'administrator' : 'member' }),
    useEnvironmentOptions: () => ({}), t: key => key,
    queryAssetGatewayOptions: async () => { calls.push('gateway'); throw new Error('gateway forbidden') },
    queryMonitorDatasourceOptions: async () => { calls.push('datasource'); throw new Error('datasource forbidden') },
    queryK8sClusterInfo: async () => { calls.push('configuration'); throw new Error('configuration forbidden') },
    queryK8sClusterList: async () => { calls.push('list'); return response ? response() : [{ id: 3, name: 'authorized-cluster', env: 'test' }] }
  })
  vm.runInContext(script, context)
  return { hooks, changes, calls, routes, rows: () => vm.runInContext('clusterList.value', context), load: () => vm.runInContext('loadClusters()', context), open: () => vm.runInContext('openDetail({ id: 3 })', context) }
}

test('K8s-granted members see their clusters without gateway, datasource or kubeconfig management access', async () => {
  const page = setup()
  await Promise.allSettled(page.hooks.map(fn => fn()))
  assert.equal(page.rows()[0]?.name, 'authorized-cluster')
  assert.deepEqual(page.calls, ['list'])
})

test('cluster grants and revocations update the open inventory', async () => {
  let granted = false
  const page = setup({ response: () => granted ? [{ id: 3, name: 'authorized-cluster' }] : [] })
  await Promise.allSettled(page.hooks.map(fn => fn()))
  assert.equal(page.rows().length, 0)
  assert.equal(page.changes.length, 1)
  granted = true; await page.changes[0]()
  assert.equal(page.rows().length, 1)
  granted = false; await page.changes[0]()
  assert.equal(page.rows().length, 0)
})

test('ordinary members open the authorized cluster console instead of its privileged configuration detail', () => {
  const page = setup()
  page.open()
  assert.equal(page.routes[0].path, '/containers/k8s/overview')
  assert.equal(Number(page.routes[0].query.clusterId), 3)
  assert.deepEqual(page.calls, [])
  const admin = setup({ administrator: true })
  admin.open()
  assert.equal(admin.routes[0].name, 'K8sClusterDetail')
})

test('late inventory results do not restore clusters after revocation', async () => {
  let oldDone, newDone
  const old = new Promise(resolve => { oldDone = resolve })
  const next = new Promise(resolve => { newDone = resolve })
  const results = [old, next]
  const page = setup({ response: () => results.shift() })
  const a = page.load(), b = page.load()
  newDone([]); await b
  oldDone([{ id: 3, name: 'revoked-cluster' }]); await a
  assert.equal(page.rows().length, 0)
})
