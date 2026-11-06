import { tr, getLocale } from '../src/utils/i18n.js'
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import AsyncValidator from 'async-validator'
const Schema = AsyncValidator.default || AsyncValidator
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { middlewareTypes, middlewareRows, middlewareValue } from '../src/views/assets/middleware-schema.js'

const setup = readFileSync(new URL('../src/views/assets/MiddlewareManager.vue', import.meta.url), 'utf8')
  .match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm, '')
const tick = () => new Promise(resolve => setImmediate(resolve))
function deferred() { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }
function createPage(kind = 'etcd', confirm = () => Promise.resolve()) {
  const requests = [], props = reactive({ kind }), hooks = {}, stops = []
  let administrator = true
  const request = (type, details) => { const result = deferred(); requests.push({ type, ...details, ...result }); return result.promise }
  const context = vm.createContext({
    tr, getLocale, computed, nextTick, reactive, ref, defineProps: () => props,
    watch: (...args) => { const stop = watch(...args); stops.push(stop); return stop },
    onMounted: callback => { hooks.mounted = callback }, onUnmounted: callback => { hooks.unmounted = callback },
    useRouter: () => ({ push: () => {} }), useRoute: () => ({ query: {} }), window: new EventTarget(),
    getUser: () => ({ accountType: administrator ? 'administrator' : 'user' }), getPermissions: () => ['assets:middleware:list'],
    listMiddleware: kind => request('list', { kind }),
    queryAssetCredentialOptions: filter => request('credentials', { filter }),
    operateMiddleware: (id, operation, params) => request('resource', { id, operation, params }),
    ElMessage: { warning: () => {}, success: () => {}, error: () => {} }, ElMessageBox: { confirm },
    middlewareTypes, middlewareRows, middlewareValue,
  })
  const page = vm.runInContext(`${setup}\n;({ openSource, enter, load, permissionsChanged, openOperation, executeOperation, operationForm, actionPending, saving, sources, credentials, credentialLoading, selected, result, busy, loading, sourceDialog, form, endpointText, sourceRules, payload, onAuthChange })`, context)
  hooks.mounted()
  return { page, props, requests, revoke: () => { administrator = false }, close: () => { hooks.unmounted(); stops.forEach(stop => stop()) } }
}

test('middleware type switch ignores the previous credential response and retains current loading', async () => {
  const view = createPage(), { page, props, requests } = view
  try {
    requests.find(r => r.type === 'list').resolve([])
    const oldDialog = page.openSource()
    await tick()
    const oldCredentials = requests.find(r => r.type === 'credentials')
    props.kind = 'kafka'
    await nextTick()
    const newDialog = page.openSource()
    await tick()
    const newCredentials = requests.filter(r => r.type === 'credentials').at(-1)
    assert.notEqual(oldCredentials, newCredentials)
    oldCredentials.resolve([{ id: 1, name: 'old certificate', authType: 'tls' }, { id: 2, name: 'old password', authType: 'password' }])
    await oldDialog
    assert.equal(page.credentials.value.length, 0)
    assert.equal(page.credentialLoading.value, true)
    newCredentials.resolve([{ id: 3, name: 'Kafka password', authType: 'password' }])
    await newDialog
    assert.equal(page.credentials.value[0].id, 3)
    assert.equal(page.credentialLoading.value, false)
  } finally { view.close() }
})

test('middleware permission refresh invalidates earlier resource and source-list responses', async () => {
  const view = createPage('kafka'), { page, requests } = view
  try {
    const source = { id: 7, name: 'authorized Kafka' }
    requests.find(r => r.type === 'list').resolve([source])
    await tick()
    page.enter(source)
    const resource = requests.find(r => r.type === 'resource')
    const earlierList = page.load()
    const staleList = requests.filter(r => r.type === 'list').at(-1)
    view.revoke()
    page.permissionsChanged()
    requests.filter(r => r.type === 'list').at(-1).resolve([])
    await tick()
    staleList.resolve([source])
    resource.resolve([{ Name: 'previously authorized topic' }])
    await earlierList
    await tick()
    assert.equal(page.sources.value.length, 0)
    assert.equal(page.selected.value, null)
    assert.equal(page.result.value, null)
    assert.equal(page.busy.value, false)
    assert.equal(page.loading.value, false)
  } finally { view.close() }
})

test('middleware confirmation cannot apply an earlier operation to a newly selected source', async () => {
  const confirmation = deferred()
  const view = createPage('etcd', () => confirmation.promise), { page, requests } = view
  try {
    requests.find(r => r.type === 'list').resolve([])
    await tick()
    page.enter({ id: 7, name: 'original etcd' })
    page.openOperation(middlewareTypes.etcd.operations.find(op => op.key === 'key.put'))
    page.operationForm.key = '/app/config'; page.operationForm.value = 'original value'
    const operation = page.executeOperation()
    assert.equal(page.actionPending.value, true)
    page.enter({ id: 8, name: 'another etcd' })
    confirmation.resolve()
    await operation
    assert.equal(requests.some(r => r.type === 'resource' && r.operation === 'key.put'), false)
    assert.equal(page.selected.value.id, 8)
    assert.equal(page.actionPending.value, false)
    assert.equal(page.saving.value, false)
  } finally { view.close() }
})

test('filled connection address satisfies both the custom and required form-item validators', async () => {
 const view = createPage('etcd'), {page} = view
 try {
  Object.assign(page.form,{name:'57-etcd',authType:'tls',credentialId:5})
  page.endpointText.value='http://192.168.171.57:2379'
  const source = readFileSync(new URL('../src/views/assets/MiddlewareManager.vue', import.meta.url),'utf8')
  const input = source.indexOf('v-model="endpointText"')
  const formItem = source.slice(source.lastIndexOf('<el-form-item',input),input)
  const prop = formItem.match(/prop="([^"]+)"/)[1]
  const rules = [...page.sourceRules.value[prop]]
  if (!rules.some(rule => rule.required !== undefined)) rules.push({required:true})
  await new Schema({[prop]:rules}).validate(page.form)
  page.endpointText.value='   '
  await assert.rejects(new Schema({[prop]:rules}).validate(page.form), error => error.errors[0].message === '请输入连接地址')
 }finally{view.close()}
})

test('etcd TLS is derived from certificate authentication or HTTPS without a separate switch', () => {
 const view=createPage('etcd'),{page}=view
 try {
  Object.assign(page.form,{name:'etcd',authType:'tls',credentialId:5,tls:false})
  page.endpointText.value='http://192.168.171.57:2379'
  assert.equal(page.payload().endpoints[0],'https://192.168.171.57:2379')
  assert.equal(page.payload().tls,true)
  assert.equal('endpointText' in page.payload(),false)
  page.form.authType='basic';page.onAuthChange()
  assert.equal(page.payload().tls,false)
  page.endpointText.value='https://etcd.example:2379'
  assert.equal(page.payload().tls,true)
  page.form.authType='none'
  assert.equal(page.payload().credentialId,0)
 }finally{view.close()}
})
