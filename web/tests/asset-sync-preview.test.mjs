import test from 'node:test'
import assert from 'node:assert/strict'
import { pendingAssetSyncItems, assetSyncProvider, assetSyncAccountRegions } from '../src/utils/asset-sync-preview.js'

const snapshot = ['add', 'update', 'unchanged', 'conflict', 'missing', 'skip'].map(action => ({ id: action, action }))
test('sync preview hides previously synchronized assets while retaining changed and unresolved assets', () => {
  assert.deepEqual(pendingAssetSyncItems(snapshot, 'ready').map(i => i.id), ['add', 'update', 'conflict', 'missing', 'skip'])
  assert.equal(snapshot.length, 6, 'history snapshot remains complete')
})
test('completed synchronization removes successful rows and leaves conflicts available for handling', () => {
  for (const status of ['success', 'partial']) {
    assert.deepEqual(pendingAssetSyncItems(snapshot, status).map(i => i.id), ['conflict', 'skip'])
  }
  assert.deepEqual(pendingAssetSyncItems([{ action: 'unchanged' }], 'ready'), [])
})
test('failed synchronization retains changes for investigation instead of treating them as synchronized', () => {
  assert.equal(pendingAssetSyncItems(snapshot, 'failed').some(i => i.action === 'add'), true)
  assert.deepEqual(pendingAssetSyncItems(), [])
})

test('collection failures stay actionable after inventory succeeds, success disappears', () => {
  const items = [
    { id: 'success', hostId: 1, action: 'add', collectStatus: 'success' },
    { id: 'failed', hostId: 2, action: 'update', collectStatus: 'failed' },
    { id: 'pending', hostId: 3, action: 'add', collectStatus: 'pending' },
    { id: 'skipped', hostId: 4, action: 'add', collectStatus: 'skipped' },
    { id: 'unchanged', action: 'unchanged' }
  ]
  for (const status of ['collecting', 'partial', 'success']) {
    assert.deepEqual(pendingAssetSyncItems(items, status).map(i => i.id), ['failed', 'pending', 'skipped'])
  }
  assert.equal(items[0].collectStatus, 'success', 'full history remains untouched')
})
test('legacy source providers and account regions normalize for the new editor', () => {
  assert.equal(assetSyncProvider(), 'jumpserver')
  assert.equal(assetSyncProvider('alicloud'), 'aliyun')
  assert.equal(assetSyncProvider('tencentcloud'), 'tencent')
  assert.deepEqual(assetSyncAccountRegions({ region: 'ap-shanghai, ap-beijing;ap-shanghai' }), ['ap-shanghai', 'ap-beijing'])
  assert.deepEqual(assetSyncAccountRegions({ regions: ['cn-shanghai', 'cn-shanghai'], region: 'old' }), ['cn-shanghai'])
  assert.deepEqual(assetSyncAccountRegions(), [])
})
