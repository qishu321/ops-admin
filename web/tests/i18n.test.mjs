import test from 'node:test'
import assert from 'node:assert/strict'
import { computed } from 'vue'
import { readFileSync } from 'node:fs'
import { parse as parseSFC } from '@vue/compiler-sfc'
import { baseParse } from '@vue/compiler-dom'
import { appDefinitions } from '../src/utils/apps.js'
import { getLocale, setLocale, tr, translateRoute } from '../src/utils/i18n.js'
import { middlewareTypes } from '../src/views/assets/middleware-schema.js'
import english from '../src/locales/en-ui.js'

const han = /[\u4e00-\u9fff]/

test('language switching updates existing translations and preserves resource data', () => {
  const label = computed(() => tr('消费组'))
  setLocale('en')
  assert.equal(getLocale(), 'en-US')
  assert.equal(label.value, 'Consumer groups')
  assert.equal(tr('内网kafka'), '内网kafka')
  setLocale('zh-CN')
  assert.equal(label.value, '消费组')
})

test('interpolation replaces repeated placeholders once without rewriting user values', () => {
  setLocale('en-US')
  assert.equal(tr('{name} / {name}', { name: '$& {other} 原始数据', other: 'changed' }), '$& {other} 原始数据 / $& {other} 原始数据')
  assert.equal(tr('共 {p0} 个数据源', { p0: 3 }), 'Data sources: 3')
  setLocale('zh-CN')
  assert.equal(tr('共 {p0} 个数据源', { p0: 3 }), '共 3 个数据源')
})

test('every static application menu has an English title and restores its Chinese title', () => {
  function check(items = []) {
    for (const item of items) {
      setLocale('zh-CN')
      const original = translateRoute(item.path, item.title)
      setLocale('en-US')
      const title = translateRoute(item.path, item.title)
      assert.equal(han.test(title), false, `${item.path}: ${title}`)
      setLocale('zh-CN')
      assert.equal(translateRoute(item.path, title), original, item.path)
      check(item.children)
    }
  }
  appDefinitions.forEach(app => check(app.menus))
})

test('middleware metadata labels and operation forms have English translations', () => {
  setLocale('en-US')
  for (const spec of Object.values(middlewareTypes)) {
    const labels = [spec.description, ...spec.tabs.map(x => x[1]), ...Object.values(spec.columns).flat().map(x => x[1]), ...spec.fields.map(x => x.label), ...spec.operations.flatMap(op => [op.label, ...op.fields.map(x => x.label)])]
    for (const label of labels) assert.equal(han.test(tr(label)), false, `${spec.name}: ${label}`)
  }
  setLocale('zh-CN')
})

test('new UI translations preserve every interpolation parameter', () => {
  const parameters = value => [...value.matchAll(/\{([\w]+)\}/g)].map(x => x[1]).sort()
  for (const [source, translation] of Object.entries(english)) {
    assert.equal(han.test(translation), false, source)
    assert.deepEqual(parameters(translation), parameters(source), source)
  }
})

test('Kafka, Redis, and the shared layout contain no untranslated static template copy', () => {
  for (const file of ['layouts/MainLayout.vue', 'views/assets/MiddlewareManager.vue', 'views/assets/KafkaTopicDetail.vue', 'views/assets/KafkaMessagePanel.vue', 'views/assets/database/RedisWorkbench.vue', 'views/assets/database/DatabaseConnectionTree.vue']) {
    const source = readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')
    const { descriptor } = parseSFC(source)
    function check(node) {
      if (node.type === 2) assert.equal(han.test(node.content), false, `${file}: ${node.content}`)
      for (const prop of node.props || []) if (prop.type === 6 && prop.value && ['label', 'title', 'placeholder', 'description', 'empty-text', 'aria-label'].includes(prop.name)) assert.equal(han.test(prop.value.content), false, `${file}: ${prop.value.content}`)
      node.children?.forEach(check)
    }
    check(baseParse(descriptor.template.content))
  }
})

test('detail-page tabs restore Chinese titles after switching back from English', () => {
  for (const [path, source] of [['/assets/middleware/kafka/topic', 'Topic 详情'], ['/assets/middleware/nacos/explorer', 'Nacos 工作台'], ['/business-topology', '业务拓扑图']]) {
    setLocale('en-US')
    const english = translateRoute(path, source)
    assert.equal(han.test(english), false)
    setLocale('zh-CN')
    assert.equal(translateRoute(path, english), source)
  }
})
