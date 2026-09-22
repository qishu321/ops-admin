import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const taskView = readFileSync(new URL('../src/views/ops/OpsScheduleTaskList.vue', import.meta.url), 'utf8')
const opsApi = readFileSync(new URL('../src/api/ops.js', import.meta.url), 'utf8')

test('HTTP scheduled task exposes retry controls and ordinary 4xx guidance', () => {
  assert.match(taskView, /v-model="form\.retryEnabled"/)
  assert.match(taskView, /v-model="form\.maxRetries"/)
  assert.match(taskView, /v-model="form\.retryIntervalSeconds"/)
  assert.match(taskView, /400、401、403、404 等所有 4xx/)
})

test('unsafe HTTP retries require an explicit confirmation', () => {
  assert.match(taskView, /\['POST', 'PATCH', 'DELETE'\]/)
  assert.match(taskView, /v-model="form\.allowUnsafeRetry"/)
  assert.match(taskView, /请确认允许该请求方法重复提交后再保存/)
})

test('notification preview is failure-only when the configured policy is failure-only', () => {
  assert.match(taskView, /if \(form\.notifyOnFailureOnly\) previewStatus\.value = 'failed'/)
  assert.match(taskView, /v-if="!form\.notifyOnFailureOnly" value="success"/)
  assert.match(taskView, /当前为“仅失败时通知”，发送预览只展示失败通知/)
  assert.match(opsApi, /\/ops\/schedule\/task\/notify-preview/)
})
