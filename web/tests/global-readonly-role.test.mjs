import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('role editor keeps page and action permissions independently selectable', () => {
  const source = read('src/views/system/Role.vue')
  assert.match(source, /check-strictly/)
  assert.match(source, /恢复全局只读模板/)
  assert.match(source, /row\.isReadOnly/)
})

test('read-only role calls the platform template endpoint', () => {
  const source = read('src/api/system.js')
  assert.match(source, /role\/readonlyTemplate/)
})

test('terminal entry points reject global read-only users server-side', () => {
  assert.match(read('../backend/controller/controller.go'), /全局只读角色不能打开终端/)
  assert.match(read('../backend/controller/k8s.go'), /全局只读角色不能打开 Pod 终端/)
})
