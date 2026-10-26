import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
const read = file => readFileSync(new URL(file, import.meta.url), 'utf8')
test('command filter navigation cannot be delegated to members', () => {
  const source = read('../src/layouts/MainLayout.vue')
  const fn = source.slice(source.indexOf('function normalizeStaticMenus('), source.indexOf('function findMenuTitle('))
  const context = vm.createContext({ user: { value: { accountType: 'member' } } })
  vm.runInContext(fn, context)
  const menu = [{ path: '/assets/server', children: [{ path: '/assets/server/hosts' }, { path: '/assets/server/command-filters', adminOnly: true }] }]
  context.menu = menu
  assert.equal(vm.runInContext('normalizeStaticMenus(menu)[0].children.length', context), 1)
  context.user.value.accountType = 'administrator'
  assert.equal(vm.runInContext('normalizeStaticMenus(menu)[0].children.length', context), 2)
})
test('filter settings are excluded from delegable permissions and have a direct-route administrator gate', () => {
  const router = read('../src/router/index.js')
  assert.match(router, /path: '\/assets\/server\/command-filters'.*adminOnly: true/)
  assert.match(router, /to.meta.adminOnly && getUser\(\).accountType !== 'administrator'/)
  assert.doesNotMatch(read('../../backend/store/seed.go'), /assets:commandfilter/)
})
test('SSH filtering is wired on backend input while Pod terminal stays unfiltered', () => {
  const controller = read('../../backend/controller/controller.go')
  const ssh = controller.slice(controller.indexOf('func (ctl *Controller) AssetTerminalWS'), controller.indexOf('func (ctl *Controller) UploadAssetTerminalFile'))
  assert.match(ssh, /commandFilter.WriteInput\(message, terminal.Stdin\)/)
  assert.match(ssh, /writeAssetTerminalOutput\(conn, &writeMu, reader, commandFilter.ObserveOutput(?:, [^)]*)?\)/)
  assert.doesNotMatch(read('../../backend/service/k8s_terminal.go'), /NewTerminalCommandFilter|CheckCommandFilters/)
})
