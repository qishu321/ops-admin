import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('login blocks empty submission and contains rejected authentication requests', () => {
  const source = read('src/views/Login.vue')
  assert.match(source, /const canSubmit = computed/)
  assert.match(source, /ElMessage\.warning\('请输入用户名和密码'\)/)
  assert.match(source, /:disabled="!canSubmit"/)
  assert.match(source, /catch \{[\s\S]*避免 Vue 未捕获事件告警/)
})

test('login displays an action-specific success message', () => {
  const source = read('src/views/Login.vue')
  assert.match(source, /ElMessage\.success\('登录成功'\)/)
  assert.doesNotMatch(source, /ElMessage\.success\(t\('saveSuccess'\)\)/)
})

test('command execution is disabled until command and target are present', () => {
  const source = read('src/views/ops/OpsCommandExecute.vue')
  assert.match(source, /form\.commandText\.trim\(\) && \(form\.hostIds\.length \|\| form\.groupId\)/)
  assert.match(source, /:disabled="!canSubmit"/)
  assert.match(source, /填写命令并选择目标主机或主机组后可执行/)
})
