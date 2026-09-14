import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const detailSource = readFileSync(new URL('../src/views/assets/AssetDetail.vue', import.meta.url), 'utf8')
const metricsSource = readFileSync(new URL('../src/views/assets/DatabaseMetrics.vue', import.meta.url), 'utf8')

test('database metrics mount only after database detail is ready', () => {
  assert.match(detailSource, /resourceType === 'database' && detailReady/)
  assert.match(detailSource, /:key="`database-metrics-\$\{route\.params\.id\}`"/)
})

test('first valid database id and enabled state trigger metrics loading immediately', () => {
  assert.match(metricsSource, /watch\(\(\) => \[props\.databaseId, props\.enabled\], load, \{ immediate: true \}\)/)
  assert.match(metricsSource, /const version = \+\+loadVersion/)
  assert.match(metricsSource, /if \(version !== loadVersion\) return/)
})
