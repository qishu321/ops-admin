import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const source = readFileSync(new URL('../src/views/assets/K8s.vue', import.meta.url), 'utf8')
const loads = source.slice(source.indexOf('let clusterListVersion = 0'), source.indexOf('async function refreshCurrentClusterData()'))

function setup({ list = async () => [{ id: 3 }, { id: 4 }], overview = async id => ({ cluster: { id }, overview: {}, pods: [] }) } = {}) {
  const storage = new Map([['cluster', '999']]), requests = []
  const refs = Object.fromEntries(['loading', 'clusterOptions', 'cluster', 'overview', 'nodes', 'namespaces', 'pods', 'workloads', 'services', 'ingresses', 'ingressClasses', 'gatewayApiGateways', 'httpRoutes', 'configMaps', 'secrets', 'storages', 'selectedWorkloads', 'namespaceFilter'].map(key => [key, { value: key === 'loading' ? false : [] }]))
  const context = vm.createContext({
    ...refs, workloadImageMap: {}, CLUSTER_KEY: 'cluster', NAMESPACE_FILTER_KEY: 'namespace',
    localStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) },
    queryK8sClusterList: list,
    queryK8sClusterOverview: async id => { requests.push(id); return overview(id) },
    restoreNamespaceFilter: () => {}, hydrateWorkloadImages: () => {}
  })
  vm.runInContext(loads, context)
  return { refs, requests, load: id => vm.runInContext(`loadClusters(${id ?? 'undefined'})`, context) }
}

test('the console selects the cluster clicked in inventory and ignores an inaccessible previously stored cluster', async () => {
  const page = setup()
  await page.load(4)
  assert.deepEqual(page.requests, [4])
  assert.equal(page.refs.cluster.value.id, 4)
  const fallback = setup()
  await fallback.load(999)
  assert.deepEqual(fallback.requests, [3])
})

test('live overview failure does not erase the authorized cluster selector', async () => {
  const page = setup({ overview: async () => { throw new Error('cluster unavailable') } })
  await assert.rejects(page.load(3), /cluster unavailable/)
  assert.equal(page.refs.clusterOptions.value.length, 2)
  assert.equal(page.refs.cluster.value, null)
  assert.equal(page.refs.loading.value, false)
})

test('revocation clears console resources and ignores a late overview response', async () => {
  let finish, allowed = true
  const response = new Promise(resolve => { finish = resolve })
  const page = setup({ list: async () => allowed ? [{ id: 3 }] : [], overview: () => response })
  const old = page.load(3)
  await Promise.resolve(); await Promise.resolve()
  allowed = false
  await page.load()
  finish({ cluster: { id: 3 }, pods: [{ name: 'revoked-pod' }] })
  await old
  assert.equal(page.refs.cluster.value, null)
  assert.equal(page.refs.pods.value.length, 0)
  assert.equal(page.refs.clusterOptions.value.length, 0)
  assert.equal(page.refs.loading.value, false)
})
