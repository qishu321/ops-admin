import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('网络控制台将 Service、Ingress 和 Ingress Class 放在三个标签页中', () => {
  const page = read('src/views/assets/k8s/K8sSectionContent.vue')
  assert.match(page, /currentTab === 'network'/)
  assert.match(page, /v-model="page\.networkTab"/)
  assert.match(page, /label="服务 SVC" name="services"/)
  assert.match(page, /label="路由 Ingress" name="ingresses"/)
  assert.match(page, /label="Ingress Class" name="ingressclasses"/)
})

test('旧 Service 和 Ingress 地址会保留为网络标签页的兼容跳转', () => {
  const routes = read('src/router/index.js')
  assert.match(routes, /containers\/k8s\/services', redirect: '\/containers\/k8s\/network\?tab=services'/)
  assert.match(routes, /containers\/k8s\/ingresses', redirect: '\/containers\/k8s\/network\?tab=ingresses'/)
  const apps = read('src/utils/apps.js')
  assert.match(apps, /titleKey: 'k8sNetwork', path: '\/containers\/k8s\/network'/)
  assert.doesNotMatch(apps, /titleKey: 'k8sServices', path: '\/containers\/k8s\/services'/)
})
