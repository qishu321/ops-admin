import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('login exposes an opt-in seven-day session controlled by public config', () => {
  const source = read('src/views/Login.vue')
  assert.match(source, /rememberLogin:\s*false/)
  assert.match(source, /v-if="configLoaded && config\.rememberLoginEnabled"/)
  assert.match(source, /setAuthPersistence\(data\.rememberLogin === true\)/)
  assert.match(source, /data\.sessionExpiresAt/)
})

test('system settings enable remembered login by default', () => {
  const source = read('src/views/system/BasicConfig.vue')
  assert.match(source, /rememberLoginEnabled:\s*true/)
  assert.match(source, /v-model="form\.rememberLoginEnabled"/)
})

test('remembered and ordinary sessions use distinct browser storage', () => {
  const source = read('src/utils/auth.js')
  assert.match(source, /\? localStorage : sessionStorage/)
  assert.match(source, /SESSION_EXPIRES_AT_KEY/)
  assert.match(source, /now >= expiresAt/)
})

test('remembered sessions bypass only the client idle timeout', () => {
  const source = read('src/api/http.js')
  assert.match(source, /if \(isRememberLogin\(\)\) return false/)
  assert.match(source, /payload\.data\.sessionExpiresAt/)
  assert.match(source, /setTimeout\(\(\) => expireLocalSession\(\), remaining\)/)
})
