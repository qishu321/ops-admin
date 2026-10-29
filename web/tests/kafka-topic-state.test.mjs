import { tr, getLocale } from '../src/utils/i18n.js'
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { computed, reactive, ref } from 'vue'
import * as kafka from '../src/views/assets/kafka-topic.js'

const setup = readFileSync(new URL('../src/views/assets/KafkaTopicDetail.vue', import.meta.url), 'utf8')
  .match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm, '')
const tick = () => new Promise(resolve => setImmediate(resolve))
function deferred() { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }
function createPage(confirm = () => Promise.resolve()) {
  const requests = [], props = { sourceId: 1, topic: 'orders' }
  let permissions = ['assets:middleware:query', 'assets:middleware:write', 'assets:middleware:delete']
  const context = vm.createContext({
    tr, getLocale, computed, reactive, ref, defineProps: () => props,
    watch: (source, callback, options) => { if (options?.immediate) callback() }, onUnmounted: () => {},
    useRouter: () => ({ push: () => {} }), window: new EventTarget(),
    getUser: () => ({ accountType: 'user' }), getPermissions: () => permissions,
    operateMiddleware: (id, operation, params) => { const result = deferred(); requests.push({ id, operation, params, ...result }); return result.promise },
    ElMessage: { warning: () => {}, success: () => {}, error: () => {} }, ElMessageBox: { confirm, close: () => {} },
    ...kafka, count: kafka.formatKafkaCount,
  })
  const page = vm.runInContext(`${setup}\n;({ reset, permissionsChanged, loadBase, loadMessages, loadConsumers, clearMessages, openProduce, produce, overview, configs, messages, consumers, baseLoading, configLoading, messagesLoading, consumersLoading, saving, clearing, confirming, messageFilter, configFilter, groupFilter, consumerDetail, messageScope, cursorHistory, expandedKeys, produceDialog, configDialog, partitionsDialog, query })`, context)
  return { page, props, requests, revoke: () => { permissions = [] } }
}
function resolveBase(requests) {
  for (const request of requests) {
    if (request.operation === 'topic.overview') request.resolve({ partitions: [{ id: 0, replicas: [1], isr: [1] }], offsetSpan: '2' })
    if (request.operation === 'topic.configs') request.resolve([])
  }
}

test('Kafka permission refresh invalidates all earlier responses before clearing visible data', async () => {
  const { page, requests, revoke } = createPage()
  const messages = page.loadMessages(), consumers = page.loadConsumers()
  revoke()
  page.permissionsChanged()
  resolveBase(requests)
  requests.find(r => r.operation === 'topic.messages').resolve({ items: [{ offset: '1' }] })
  requests.find(r => r.operation === 'topic.consumers').resolve({ items: [{ groupId: 'old-group' }] })
  await Promise.all([messages, consumers])
  await tick()
  assert.equal(page.overview.value, null)
  assert.equal(page.configs.value.length, 0)
  assert.equal(page.messages.value, null)
  assert.equal(page.consumers.value, null)
  for (const loading of [page.baseLoading, page.configLoading, page.messagesLoading, page.consumersLoading]) assert.equal(loading.value, false)
})

test('Kafka reused topic view clears previous filters, detail, cursors and clear-operation loading', async () => {
  const { page, props, requests } = createPage()
  resolveBase(requests)
  await tick()
  page.messageFilter.value = 'previous value'; page.configFilter.value = 'old config'; page.groupFilter.value = 'old group'
  page.consumerDetail.value = { groupId: 'old group' }; page.messageScope.value = { partition: '9' }
  page.cursorHistory.value = ['30']; page.expandedKeys.value = ['9:30']
  const clear = page.clearMessages()
  await tick()
  assert.equal(page.clearing.value, true)
  props.topic = 'payments'
  page.reset()
  assert.equal(page.clearing.value, false)
  assert.equal(page.confirming.value, false)
  assert.equal(page.consumerDetail.value, null)
  assert.equal(page.messageScope.value, null)
  for (const filter of [page.messageFilter, page.configFilter, page.groupFilter]) assert.equal(filter.value, '')
  assert.equal(page.cursorHistory.value.length, 0)
  assert.equal(page.expandedKeys.value.length, 0)
  requests.find(r => r.operation === 'topic.clear').resolve({})
  await clear
  assert.equal(page.clearing.value, false)
})

test('Kafka confirmation from a previous topic cannot execute against the next topic', async () => {
  const confirmation = deferred()
  const { page, props, requests } = createPage(() => confirmation.promise)
  page.openProduce()
  const send = page.produce()
  props.topic = 'payments'
  page.reset()
  confirmation.resolve()
  await tick()
  assert.equal(requests.some(r => r.operation === 'topic.produce'), false)
  await send
})

test('Kafka overlapping overview refreshes retain the newest result', async () => {
  const { page, requests } = createPage()
  const refresh = page.loadBase()
  requests[2].resolve({ partitions: [], offsetSpan: '20' }); requests[3].resolve([{ name: 'new' }])
  await refresh
  requests[0].resolve({ partitions: [], offsetSpan: '10' }); requests[1].resolve([{ name: 'old' }])
  await tick()
  assert.equal(page.overview.value.offsetSpan, '20')
  assert.equal(page.configs.value[0].name, 'new')
})
