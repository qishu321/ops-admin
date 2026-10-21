import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { createAccessSynchronizer } from '../src/utils/access-sync.js'
import { createPermissionDirective, PERMISSIONS_CHANGED_EVENT } from '../src/utils/permission-directive.js'

const profile = (revision, permissions = ['assets:host:list']) => ({ user: { id: 7, accountType: 'member' }, accessRevision: revision, permissionList: permissions, leftMenuList: [] })
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }

test('resource-only grants and revocations refresh the open view even if page permissions are unchanged', async () => {
  const changes = [], applied = []
  let current = profile('empty')
  const sync = createAccessSynchronizer({ getIdentity: () => 'session', loadProfile: async () => current, applyProfile: data => applied.push(data), onChanged: detail => changes.push(detail) })
  await sync.sync()
  await sync.sync()
  assert.equal(changes.length, 1)
  current = profile('group-granted')
  await sync.sync()
  current = profile('group-revoked')
  await sync.sync()
  assert.equal(changes.length, 3)
  assert.equal(applied.at(-1).accessRevision, 'group-revoked')
})

test('overlapping timer, focus and route refreshes share one request; harmless permission ordering does not refresh lists', async () => {
  const response = deferred(), changes = []
  let requests = 0
  let current = profile('same', ['assets:host:list', 'assets:terminal'])
  const sync = createAccessSynchronizer({ getIdentity: () => 'session', loadProfile: async () => { requests++; return requests === 1 ? response.promise : current }, applyProfile: () => {}, onChanged: detail => changes.push(detail) })
  const a = sync.sync(), b = sync.sync(), c = sync.sync()
  response.resolve(current)
  await Promise.all([a, b, c])
  assert.equal(requests, 1)
  current = profile('same', ['assets:terminal', 'assets:host:list'])
  await sync.sync()
  assert.equal(changes.length, 1)
})

test('a profile response from a previous login cannot overwrite the current identity', async () => {
  const first = deferred(), applied = []
  let identity = 'old-session', requests = 0
  const sync = createAccessSynchronizer({ getIdentity: () => identity, loadProfile: () => ++requests === 1 ? first.promise : Promise.resolve(profile('new')), applyProfile: data => applied.push(data.accessRevision), onChanged: () => {} })
  const old = sync.sync()
  await Promise.resolve()
  identity = 'new-session'
  await sync.sync()
  first.resolve(profile('old'))
  await old
  assert.deepEqual(applied, ['new'])
})

test('failed refreshes can be retried and disposed layouts cannot apply late results', async () => {
  let reject = true
  const response = deferred(), applied = []
  const sync = createAccessSynchronizer({ getIdentity: () => 'session', loadProfile: () => reject ? Promise.reject(new Error('temporary failure')) : response.promise, applyProfile: data => applied.push(data), onChanged: () => {} })
  await assert.rejects(sync.sync(), /temporary failure/)
  reject = false
  const pending = sync.sync()
  sync.dispose()
  response.resolve(profile('late'))
  await pending
  assert.deepEqual(applied, [])
})

test('permission buttons appear after granting, disappear after revocation and retain their original layout', () => {
  const events = new EventTarget()
  let permissions = []
  const directive = createPermissionDirective(() => permissions, events)
  const el = { style: { display: 'inline-flex' }, parentNode: { removeChild() { assert.fail('permission updates must not detach Vue-owned DOM') } } }
  const binding = { value: 'assets:host:terminal' }
  directive.beforeMount(el, binding)
  directive.mounted(el, binding)
  assert.equal(el.style.display, 'none')
  permissions = ['assets:host:terminal']
  events.dispatchEvent(new Event(PERMISSIONS_CHANGED_EVENT))
  assert.equal(el.style.display, 'inline-flex')
  permissions = []
  events.dispatchEvent(new Event(PERMISSIONS_CHANGED_EVENT))
  assert.equal(el.style.display, 'none')
  directive.unmounted(el)
  permissions = ['assets:host:terminal']
  events.dispatchEvent(new Event(PERMISSIONS_CHANGED_EVENT))
  assert.equal(el.style.display, 'none', 'unmounted elements must no longer subscribe')
})

test('cached inactive pages refresh on activation and unsubscribe when closed', async () => {
  const hooks = {}, events = new EventTarget()
  let refreshes = 0
  const context = vm.createContext({
    window: events, ACCESS_CHANGED_EVENT: 'access-change', refresh: () => { refreshes++ },
    ...Object.fromEntries(['onMounted', 'onActivated', 'onDeactivated', 'onBeforeUnmount'].map(name => [name, fn => { hooks[name] = fn }]))
  })
  const source = readFileSync(new URL('../src/composables/useAccessRefresh.js', import.meta.url), 'utf8').replace(/^import .*$/gm, '').replace('export function', 'function')
  vm.runInContext(`${source}\nuseAccessRefresh(refresh)`, context)
  hooks.onMounted()
  hooks.onDeactivated()
  events.dispatchEvent(new Event('access-change'))
  assert.equal(refreshes, 0)
  hooks.onActivated()
  await Promise.resolve()
  assert.equal(refreshes, 1)
  hooks.onBeforeUnmount()
  events.dispatchEvent(new Event('access-change'))
  assert.equal(refreshes, 1)
})
