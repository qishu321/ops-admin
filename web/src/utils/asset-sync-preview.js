export function pendingAssetSyncItems(items = [], status = '') {
  const completed = ['success', 'partial', 'collecting'].includes(status)
  return items.filter(item => completed
    ? ['conflict', 'skip'].includes(item.action) || ['pending', 'failed', 'skipped'].includes(item.collectStatus)
    : item.action !== 'unchanged')
}

export function assetSyncProvider(provider = '') {
  const value = String(provider || '').trim().toLowerCase()
  return ({ alicloud: 'aliyun', tencentcloud: 'tencent' })[value] || value || 'jumpserver'
}

export function assetSyncAccountRegions(account) {
  const regions = account?.regions?.length ? account.regions : (account?.region || '').split(/[,，;；\s]+/)
  return [...new Set(regions.map(r => r.trim()).filter(Boolean))]
}
