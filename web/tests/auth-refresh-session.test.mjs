import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const moduleSource = readFileSync(new URL('../src/api/http.js', import.meta.url), 'utf8')
const source = moduleSource.slice(moduleSource.indexOf('const ACCESS_REFRESH_WINDOW')).replace('export default http', '')

function sessionFixture() {
  const minute = 60_000
  let now = 55 * minute, token = 'original-access-token', tokenExpires = 60 * minute
  let failure = Object.assign(new Error('network timeout'), { code: 'ECONNABORTED' })
  let logouts = 0, refreshes = 0, requests = 0
  const intervals = [], routes = [], messages = []
  let requestInterceptor, responseError
  const http = Object.assign(async () => { requests++; return {} }, {
    interceptors: {
      request: { use: fn => { requestInterceptor = fn } },
      response: { use: (_, fn) => { responseError = fn } }
    }
  })
  const authHttp = { post: async () => {
    refreshes++
    if (failure) throw failure
    return { data: { code: 200, data: { token: 'renewed-access-token', accessTokenExpiresAt: now + 60 * minute, sessionExpiresAt: 7 * 24 * 60 * minute } } }
  } }
  let clients = 0
  const context = vm.createContext({
    axios: { create: () => clients++ === 0 ? http : authHttp },
    ElMessage: { warning: text => messages.push(text), error: text => messages.push(text) },
    router: { currentRoute: { value: { path: '/assets/terminal' } }, push: path => routes.push(path) },
    Date: { now: () => now },
    window: { addEventListener() {}, setTimeout: () => 1, clearTimeout() {}, setInterval: fn => intervals.push(fn) },
    document: { addEventListener() {} },
    getToken: () => token, getTokenExpiresAt: () => tokenExpires,
    getSessionExpiresAt: () => 7 * 24 * 60 * minute,
    getLastActivityAt: () => now, isRememberLogin: () => false, setLastActivityAt() {},
    clearLocalSession: () => { logouts++; token = '' },
    setToken: (value, expiry) => { token = value; tokenExpires = expiry }
  })
  vm.runInContext(source, context)
  return {
    tick: async () => { intervals[0](); await new Promise(resolve => setImmediate(resolve)) },
    advance: minutes => { now += minutes * minute },
    recover: () => { failure = null },
    rejectAuth: () => { failure = Object.assign(new Error('session revoked'), { response: { status: 401, data: { code: 401 } } }) },
    rejectServer: () => { failure = Object.assign(new Error('service unavailable'), { response: { status: 503 } }) },
    request: () => requestInterceptor({ headers: {} }),
    unauthorizedResponse: () => responseError({ config: { headers: {} }, response: { status: 401, data: { code: 401, message: 'access token expired' } } }),
    snapshot: () => ({ token, logouts, refreshes, requests, routes, messages })
  }
}

test('temporary hourly refresh timeout preserves active terminals and retries successfully', async () => {
  const session = sessionFixture()
  await session.tick()
  assert.equal(session.snapshot().refreshes, 1)
  assert.equal(session.snapshot().logouts, 0, 'a network timeout must not unmount the terminal page')
  assert.deepEqual(session.snapshot().routes, [])
  session.advance(6) // Cross the original access token's 60-minute deadline.
  session.recover()
  await session.tick()
  assert.equal(session.snapshot().token, 'renewed-access-token')
  const request = await session.request()
  assert.equal(request.headers.Authorization, 'Bearer renewed-access-token')
  assert.equal(session.snapshot().logouts, 0)
})

test('refresh service outage preserves login but explicit refresh rejection ends it', async () => {
  const outage = sessionFixture()
  outage.rejectServer()
  await outage.tick()
  assert.equal(outage.snapshot().logouts, 0)
  const revoked = sessionFixture()
  revoked.rejectAuth()
  await revoked.tick()
  assert.equal(revoked.snapshot().logouts, 1)
  assert.deepEqual(revoked.snapshot().routes, ['/login'])
})

test('expired API token plus temporary refresh failure does not destroy the login session', async () => {
  const session = sessionFixture()
  await assert.rejects(session.unauthorizedResponse())
  assert.equal(session.snapshot().logouts, 0)
  assert.equal(session.snapshot().requests, 0, 'failed renewal must not replay a protected request')
  session.recover()
  await session.unauthorizedResponse()
  assert.equal(session.snapshot().requests, 1)
  assert.equal(session.snapshot().logouts, 0)
})

test('explicit refresh rejection during an API request clears the session', async () => {
  const session = sessionFixture()
  session.rejectAuth()
  await assert.rejects(session.request())
  assert.equal(session.snapshot().logouts, 1)
  assert.equal(session.snapshot().requests, 0)
})
