import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('Pod 镜像编辑使用单 Pod 接口并明确提示临时覆盖', () => {
  assert.match(read('src/api/k8s.js'), /http\.put\('\/api\/v1\/k8s\/pod\/images'/)
  assert.match(read('src/views/assets/k8s/K8sSectionContent.vue'), /编辑镜像/)
  const page = read('src/views/assets/K8s.vue')
  assert.match(page, /StatefulSet 等控制器重建/)
  assert.match(page, /updateK8sPodImages/)
  assert.match(read('src/views/assets/k8s/K8sDialogs.vue'), /仅修改当前 Pod/)
})

test('Pod 列表将运行阶段和容器就绪状态分开显示', () => {
  const section = read('src/views/assets/k8s/K8sSectionContent.vue')
  assert.match(section, /运行阶段/)
  assert.match(section, /容器就绪/)
  assert.match(section, /readyContainers/)
})

test('资源更新后的集群刷新绕过短期概览缓存', () => {
  assert.match(read('src/api/k8s.js'), /queryK8sClusterOverview = \(clusterId, fresh = false\)/)
  assert.match(read('src/views/assets/K8s.vue'), /loadClusterData\(cluster\.value\.id, true\)/)
})

test('Pod 监控将当前筛选的 Namespace 和 Pod 直接写入 PromQL', () => {
  const page = read('src/views/assets/K8s.vue')
  assert.match(page, /function monitorPodMetricSelector\(\)/)
  assert.match(page, /namespace=~"\^\(\$\{namespaces\.map\(escapePrometheusRegex\)/)
  assert.match(page, /pod=~"\^\(\$\{podNames\.map\(escapePrometheusRegex\)/)
  assert.match(page, /max by\(namespace, pod, container\)/)
  assert.match(page, /max by\(namespace, pod, interface\)/)
  assert.match(page, /const podScopeKey = monitorView\.value === 'pod'/)
  assert.match(page, /monitorPodNamespace\.value, monitorPodNode\.value, monitorPodWorkload\.value, monitorPodName\.value/)
})

test('监控悬浮提示会根据鼠标位置向图表内侧展开', () => {
  const page = read('src/views/assets/K8s.vue')
  const section = read('src/views/assets/k8s/K8sSectionContent.vue')
  assert.match(page, /function monitorTooltipStyle\(\)/)
  assert.match(page, /const openLeft = position > \.56/)
  assert.match(page, /translateX\(calc\(-100% - 12px\)\)/)
  assert.equal((section.match(/:style="page\.monitorTooltipStyle\(\)"/g) || []).length, 2)
})
