import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const source = readFileSync(new URL('../src/views/assets/Host.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import\s[\s\S]*?\sfrom\s+['"][^'"]+['"]\s*$/gm, '')

function hostPage({ rejectGroups = false, hostResponse } = {}) {
  const mounted = [], calls = [], changes = []
  const context = vm.createContext({
    onMounted: fn => mounted.push(fn), watch: () => {},
    computed: fn => ({ get value() { return fn() } }),
    ref: value => ({ value }), reactive: value => value,
    useRoute: () => ({ query: {} }), useRouter: () => ({}),
    useEnvironmentOptions: () => ({}),
    useAccessRefresh: fn => changes.push(fn),
    queryAssetHostGroupList: async () => { calls.push('groups'); if (rejectGroups) throw new Error('没有访问该资源或执行该操作的权限'); return { list: [{ id: 8, name: 'ops' }] } },
    queryAssetHostList: async () => { calls.push('hosts'); return hostResponse ? hostResponse() : { list: [{ id: 1, hostName: 'authorized-host' }], total: 1 } },
    queryAssetCredentialOptions: async () => { calls.push('credentials'); throw new Error('credentials denied') },
    queryAssetGatewayOptions: async () => { calls.push('gateways'); throw new Error('gateway denied') },
    getUser: () => ({ accountType: 'member' }),
    getPermissions: () => ['assets:host:list', 'assets:hostgroup:list']
  })
  vm.runInContext(script, context)
  return { calls, mounted, changes, rows: () => vm.runInContext('tableData.value', context), load: () => vm.runInContext('loadData()', context) }
}

test('authorized host browsing does not request credential or gateway management options', async () => {
  const page = hostPage()
  await Promise.allSettled(page.mounted.map(fn => fn()))
  assert.equal(page.rows()[0]?.hostName, 'authorized-host', 'configuration-option rejection must not prevent loading authorized hosts')
  assert.deepEqual(page.calls.filter(c => ['credentials', 'gateways'].includes(c)), [])
})

test('host list is loaded even if its group filter request is rejected', async () => {
  const page = hostPage({ rejectGroups: true })
  await Promise.allSettled(page.mounted.map(fn => fn()))
  assert.equal(page.rows().length, 1)
})

test('a policy change reloads the currently open host list without remounting the page', async () => {
  const page = hostPage()
  await Promise.allSettled(page.mounted.map(fn => fn()))
  const before = page.calls.filter(c => c === 'hosts').length
  assert.equal(page.changes.length, 1, 'page must subscribe to effective-access changes')
  await page.changes[0]()
  assert.equal(page.calls.filter(c => c === 'hosts').length, before + 1)
})

test('a late list response from the old permission snapshot cannot restore revoked hosts', async () => {
  let completeOld, completeNew
  const old = new Promise(resolve => { completeOld = resolve })
  const next = new Promise(resolve => { completeNew = resolve })
  const responses = [old, next]
  const page = hostPage({ hostResponse: () => responses.shift() })
  const first = page.load(), second = page.load()
  completeNew({ list: [], total: 0 })
  await second
  completeOld({ list: [{ id: 1, hostName: 'revoked-host' }], total: 1 })
  await first
  assert.equal(page.rows().length, 0)
})
