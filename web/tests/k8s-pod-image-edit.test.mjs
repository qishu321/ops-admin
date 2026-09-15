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
