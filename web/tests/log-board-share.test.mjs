import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  LOG_BOARD_TIME_RANGES,
  formatLogBoardNumber,
  logBoardBarHeight,
  logBoardBarWidth,
  logBoardDonutGradient,
  logBoardLevelClass,
  logBoardLevelCount,
  logBoardLevels,
  logBoardPublicPath,
  logBoardPublicUrl,
  logBoardRangeLabel
} from '../src/views/monitor/log-board/logBoardShared.js'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

test('public log board links encode the share token and keep the origin', () => {
  assert.equal(logBoardPublicPath('abc123'), '/public/log-board/abc123')
  assert.equal(logBoardPublicPath('a b/c'), '/public/log-board/a%20b%2Fc')
  assert.equal(logBoardPublicUrl('abc123', 'https://ops.example.com/'), 'https://ops.example.com/public/log-board/abc123')
  assert.equal(logBoardPublicUrl('abc123', 'http://192.0.2.10:8080'), 'http://192.0.2.10:8080/public/log-board/abc123')
})

test('time range labels stay aligned with the board selector', () => {
  for (const item of LOG_BOARD_TIME_RANGES) {
    assert.equal(logBoardRangeLabel(item.seconds), `最近 ${item.label}`)
  }
  assert.equal(logBoardRangeLabel(7200), '最近 2 小时')
})

test('kpi counters split errors and warnings from the level distribution', () => {
  const data = { levels: [{ value: 'ERROR', count: 2 }, { value: 'FATAL', count: 1 }, { value: 'WARN', count: 4 }, { value: 'INFO', count: 93 }] }
  assert.equal(logBoardLevelCount(data, ['ERROR', 'FATAL']), 3)
  assert.equal(logBoardLevelCount(data, ['WARN']), 4)
  assert.deepEqual(logBoardLevels(data).map(item => item.ratio), [2, 1, 4, 93])
})

test('donut gradient keeps every level and falls back for empty data', () => {
  assert.equal(logBoardDonutGradient(logBoardLevels({ levels: [] })), 'conic-gradient(#e8eef8 0 100%)')
  const gradient = logBoardDonutGradient(logBoardLevels({ levels: [{ value: 'ERROR', count: 1 }, { value: 'INFO', count: 3 }] }))
  assert.match(gradient, /^conic-gradient\(#ef5d65 0\.00% 25\.00%, #3f8fe8 25\.00% 100\.00%\)$/)
  assert.equal(logBoardLevelClass('FATAL'), 'is-error')
  assert.equal(logBoardLevelClass('warn'), 'is-warn')
})

test('bars clamp to the panel instead of overflowing on outliers', () => {
  assert.equal(logBoardBarHeight(0, 10), '0.00%')
  assert.equal(logBoardBarHeight(20, 10), '100.00%')
  assert.equal(logBoardBarWidth(0, 10), '2.00%')
  assert.equal(logBoardBarWidth(10, 0), '100.00%')
  assert.equal(formatLogBoardNumber(12345), '1.23 万')
})

test('the public page reads data through the public client, never the session http instance', () => {
  const page = source('../src/views/monitor/PublicLogBoard.vue')
  assert.match(page, /queryPublicLogBoardMeta/)
  assert.match(page, /queryPublicLogBoardData/)
  assert.doesNotMatch(page, /from '\.\.\/\.\.\/api\/http'/)
})

test('the public board keeps the datasource / LogsQL / range controls so links survive datasource changes', () => {
  const page = source('../src/views/monitor/PublicLogBoard.vue')
  assert.match(page, /v-model="form\.datasourceId"/)
  assert.match(page, /v-model="form\.query"/)
  assert.match(page, /changeRange\(item\.seconds\)/)
  assert.match(page, /datasourceId: form\.datasourceId/)
  assert.match(page, /rangeSeconds: form\.rangeSeconds/)
  const api = source('../src/api/logBoardShare.js')
  assert.match(api, /datasourceId: Number\(params\.datasourceId \|\| 0\)/)
  assert.match(api, /rangeSeconds: Number\(params\.rangeSeconds \|\| 0\)/)
  const dialog = source('../src/views/monitor/log-board/LogBoardShareDialog.vue')
  assert.doesNotMatch(dialog, /type="password"/)
  assert.doesNotMatch(dialog, /clearPassword/)
})

test('the public route is exempt from the login guard and shares the same panels', () => {
  const router = source('../src/router/index.js')
  assert.match(router, /path: '\/public\/log-board\/:token'/)
  assert.match(router, /component: PublicLogBoard, meta: \{ title: '日志大屏', public: true \}/)
  const publicPage = source('../src/views/monitor/PublicLogBoard.vue')
  assert.match(publicPage, /LogBoardPanels/)
  assert.match(publicPage, /:interactive="false"/)
})

test('share dialog exposes expiry, refresh and detail switches without a password', () => {
  const dialog = source('../src/views/monitor/log-board/LogBoardShareDialog.vue')
  for (const field of ['expiresInSeconds', 'allowDetail', 'refreshSeconds', 'regenerateLogBoardShareToken', 'updateLogBoardShareStatus']) {
    assert.match(dialog, new RegExp(field))
  }
})

test('recent panel renders error logs only and explains the 50-row cap', () => {
  const panels = source('../src/views/monitor/log-board/LogBoardPanels.vue')
  assert.match(panels, /最近错误日志/)
  assert.match(panels, /最新 \$\{recent\.value\.length\} 条 ERROR/)
  assert.match(panels, /recentLimit/)
  assert.match(panels, /recentTotal/)
  assert.match(panels, /所选时间范围内没有 ERROR 日志/)
  // 条数上限由服务端下发，前端不要写死 20。
  assert.doesNotMatch(panels, /最新 \{\{ recent\.length \}\} 条/)
})
