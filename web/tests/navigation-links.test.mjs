import assert from 'node:assert/strict'
import test from 'node:test'
import { normalizeNavigationOrigins, buildPublicNavigationLink } from '../src/utils/navigation-links.js'

test('multiple public navigation origins accept IPs and domains and deduplicate equivalent addresses', () => {
  assert.deepEqual(normalizeNavigationOrigins([' HTTPS://Ops.Example.com:443/ ', 'http://192.0.2.10:8080/public/navigation/old-token', 'https://ops.example.com', 'http://[::1]:8080', '']), ['https://ops.example.com', 'http://192.0.2.10:8080', 'http://[::1]:8080'])
  assert.deepEqual(normalizeNavigationOrigins([]), [])
})

test('switching public entrances preserves the current group token, including after regeneration', () => {
  assert.equal(buildPublicNavigationLink('https://ops.example.com', 'current-token'), 'https://ops.example.com/public/navigation/current-token')
  assert.equal(buildPublicNavigationLink('http://192.0.2.10:8080', 'new-token'), 'http://192.0.2.10:8080/public/navigation/new-token')
  assert.equal(buildPublicNavigationLink('', 'new-token'), '')
})

test('public navigation origins reject invalid schemes, credentials, parameters and unrelated paths', () => {
  for (const value of ['ops.example.com', 'javascript:alert(1)', 'https://user:pass@example.com', 'https://example.com/?secret=x', 'https://example.com/#', 'https://example.com/other/path', 'http://example.com:99999']) assert.throws(() => normalizeNavigationOrigins([value]))
  assert.throws(() => normalizeNavigationOrigins(Array(11).fill('https://ops.example.com')))
})
