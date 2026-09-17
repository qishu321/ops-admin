import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('file dispatch shows and enforces the 500 MB local upload limit', () => {
  const source = read('src/views/ops/OpsFileDispatch.vue')
  assert.match(source, /500 \* 1024 \* 1024/)
  assert.match(source, /本地上传文件最大 500 MB/)
  assert.match(source, /本地上传文件大小不能超过 500 MB/)
})

test('file dispatch defaults to a large-file-safe timeout', () => {
  assert.match(read('src/views/ops/OpsFileDispatch.vue'), /timeoutSeconds: 600/)
})

test('file dispatch keeps the source filename unless a rename is explicitly entered', () => {
  const source = read('src/views/ops/OpsFileDispatch.vue')
  assert.match(source, /targetDir/)
  assert.match(source, /targetFileName/)
  assert.match(source, /默认沿用源文件名/)
  assert.match(source, /最终保存为/)
})

test('backend stages uploads and streams through SSH stdin instead of command base64', () => {
  const service = read('../backend/service/ops.go')
  const controller = read('../backend/controller/ops.go')
  assert.match(service, /OpsFileDispatchMaxUploadBytes int64 = 500 \* 1024 \* 1024/)
  assert.match(controller, /http\.MaxBytesReader/)
  assert.match(service, /session\.StdinPipe\(\)/)
  assert.match(service, /io\.Copy\(stdin, reader\)/)
  assert.doesNotMatch(service, /base64 -d <<'__OPS_FILE__'/)
})

test('existing target files report the actionable overwrite error instead of pipe EOF', () => {
  const service = read('../backend/service/ops.go')
  assert.match(service, /目标文件已存在，未开启“覆盖已有”/)
  assert.match(service, /target file already exists/)
})
