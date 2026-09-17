import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const readRoot = (path) => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')

test('工作负载伸缩页面提供 HPA 与定时伸缩两个标签页', () => {
  const routes = read('src/router/index.js')
  assert.match(routes, /K8sScalingPolicies from '..\/views\/assets\/K8sScalingPolicies\.vue'/)
  assert.match(routes, /path: '\/containers\/k8s\/scaling-policies', component: K8sScalingPolicies/)

  const apps = read('src/utils/apps.js')
  assert.match(apps, /titleKey: 'k8sWorkloadScaling', path: '\/containers\/k8s\/scaling-policies'/)

  const page = read('src/views/assets/K8sScalingPolicies.vue')
  assert.match(page, /name="hpa"/)
  assert.match(page, /name="scheduled"/)
  assert.match(page, /v-model="form\.targets" multiple/)
  assert.match(page, /保存为未启用/)
  assert.match(page, /批量启用/)
  assert.match(page, /批量删除/)
  assert.match(page, /@click="openEdit\(row\)"/)
  assert.match(page, /cronPresets/)
  assert.match(page, /cronFieldOptions/)
  assert.match(page, /生成表达式/)
  assert.match(page, /activeTab === 'scheduled' \? '1120px' : '780px'/)
})

test('启用策略通过后端状态接口控制真实伸缩资源', () => {
  const api = read('src/api/k8s.js')
  assert.match(api, /updateK8sScalingPolicyStatus = .*scaling-policy\/status/)
  assert.match(api, /createK8sScalingPolicy = .*scaling-policy\/create/)
  assert.match(api, /updateK8sScalingPolicy = .*scaling-policy\/update/)
  assert.match(api, /batchUpdateK8sScalingPolicyStatus = .*scaling-policy\/status\/batch/)
  assert.match(api, /batchDeleteK8sScalingPolicies = .*scaling-policy\/delete\/batch/)

  const backend = readRoot('backend/service/k8s_scaling.go')
  assert.match(backend, /Status: 2/)
  assert.match(backend, /for _, target := range decodeK8sScalingTargets\(policy\.TargetsJSON\)/)
  assert.match(backend, /http\.MethodPost, collectionPath/)
  assert.match(backend, /func \(s \*Service\) UpdateK8sScalingPolicy/)
  assert.match(backend, /func \(s \*Service\) BatchUpdateK8sScalingPolicyStatus/)
  assert.match(backend, /func \(s \*Service\) BatchDeleteK8sScalingPolicies/)
})
