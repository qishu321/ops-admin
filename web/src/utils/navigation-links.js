export function normalizeNavigationOrigins(values = []) {
  if (values.length > 10) throw new Error('最多配置 10 个访问入口')
  const result = []
  for (const value of values) {
    const text = String(value || '').trim()
    if (!text) continue
    let url
    try { url = new URL(text) } catch { throw new Error('入口地址必须是完整的 HTTP/HTTPS 地址') }
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || text.includes('?') || text.includes('#')) {
      throw new Error('入口地址需包含 http:// 或 https://，不能包含账号、查询参数或片段')
    }
    if (url.pathname !== '/' && !/^\/public\/navigation\/[^/]+\/?$/.test(url.pathname)) {
      throw new Error('请填写平台 IP/域名和端口，或粘贴完整的公开导航链接')
    }
    if (!result.includes(url.origin)) result.push(url.origin)
  }
  return result
}

export function buildPublicNavigationLink(origin, token) {
  return token && origin ? `${origin}/public/navigation/${encodeURIComponent(token)}` : ''
}
