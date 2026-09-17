import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const page = readFileSync(new URL('../src/views/assets/K8sPodTerminal.vue', import.meta.url), 'utf8')

test('Pod 终端在后端 exec 就绪前保持连接中状态', () => {
  const onOpen = page.slice(page.indexOf('socket.onopen ='), page.indexOf('socket.onmessage ='))
  assert.doesNotMatch(onOpen, /connected\.value\s*=\s*true/)
  assert.match(page, /payload\?\.operation === 'status'/)
  assert.match(page, /state === 'connected'/)
  assert.match(page, /连接耗时较长/)
})

test('Pod 终端以结构化错误展示网关连接失败', () => {
  assert.match(page, /payload\?\.operation === 'error'/)
  assert.match(page, /ElMessage\.error\(message\)/)
  assert.match(page, /connected\.value && socket\?\.readyState === WebSocket\.OPEN/)
})
