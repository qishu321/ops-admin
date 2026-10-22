// Pure helpers for the Nacos workbench: result envelopes differ between Nacos 2.x
// (doms/hosts/count) and 3.x (pageItems/totalCount), so normalise them here.
export const nacosFormats = [
  ['text', 'TEXT'],
  ['json', 'JSON'],
  ['xml', 'XML'],
  ['yaml', 'YAML'],
  ['html', 'HTML'],
  ['properties', 'PROPERTIES']
]

export const nacosDefaultGroup = 'DEFAULT_GROUP'

export function nacosConfigRows(data) {
  return Array.isArray(data?.pageItems) ? data.pageItems : []
}

export function nacosConfigTotal(data, fallback = 0) {
  const total = Number(data?.totalCount)
  if (Number.isFinite(total) && total >= 0) return total
  return Number(fallback) || nacosConfigRows(data).length
}

export function nacosServiceRows(data) {
  if (Array.isArray(data?.serviceList)) return data.serviceList
  if (Array.isArray(data?.pageItems)) return data.pageItems
  if (Array.isArray(data?.doms)) return data.doms.map(name => ({ name }))
  if (Array.isArray(data?.services)) return data.services
  return []
}

export function nacosServiceTotal(data, fallback = 0) {
  const total = Number(data?.count ?? data?.totalCount)
  if (Number.isFinite(total) && total >= 0) return total
  return Number(fallback) || nacosServiceRows(data).length
}

export function nacosInstanceRows(data) {
  if (Array.isArray(data?.hosts)) return data.hosts
  if (Array.isArray(data?.pageItems)) return data.pageItems
  return []
}

export function nacosNamespaceRows(data) {
  if (Array.isArray(data)) return data
  if (Array.isArray(data?.pageItems)) return data.pageItems
  return []
}

export function nacosNamespaceView(namespace) {
  // 2.x 用空 ID 表示 public，3.x 用 public，这里统一成 public 便于选择器展示。
  const id = namespace.namespace || namespace.namespaceId || 'public'
  return {
    id,
    name: namespace.namespaceShowName || namespace.namespaceName || namespace.namespace || 'public',
    desc: namespace.namespaceDesc || '',
    configCount: Number(namespace.configCount || 0),
    quota: Number(namespace.quota || 0),
    type: Number(namespace.type || 0)
  }
}

// 默认命名空间在 2.x 用空 ID、3.x 用 public，统一补一项供选择器使用。
export function nacosNamespaceOptions(data) {
  const rows = nacosNamespaceRows(data).map(nacosNamespaceView)
  if (!rows.some(row => row.id === 'public')) rows.unshift({ id: 'public', name: 'public', desc: '默认命名空间', configCount: 0, quota: 0, type: 0 })
  return rows
}

export function nacosConfigKey(row) {
  return `${row?.group || row?.groupName || ''}::${row?.dataId || ''}`
}

export function nacosFormatLabel(type) {
  const value = String(type || '').trim().toLowerCase()
  return (nacosFormats.find(([key]) => key === value) || ['text', 'TEXT'])[1]
}

export function nacosTime(value) {
  const num = Number(value)
  if (!Number.isFinite(num) || num <= 0) return '—'
  const date = new Date(num)
  if (Number.isNaN(date.getTime())) return '—'
  const pad = n => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export function nacosPretty(content, type) {
  if (String(type || '').toLowerCase() !== 'json') return { ok: false }
  try {
    return { ok: true, value: JSON.stringify(JSON.parse(String(content ?? '')), null, 2) }
  } catch {
    return { ok: false }
  }
}

export function nacosConfigPayload(form) {
  const dataId = String(form.dataId || '').trim()
  if (!dataId) return { ok: false, error: '请填写 Data ID' }
  if (dataId.length > 255) return { ok: false, error: 'Data ID 最长 255 个字符' }
  const type = String(form.type || 'text').toLowerCase()
  return {
    ok: true,
    params: {
      dataId,
      groupName: String(form.groupName || '').trim() || nacosDefaultGroup,
      namespaceId: String(form.namespaceId || '').trim(),
      type,
      content: String(form.content ?? ''),
      appName: String(form.appName || '').trim(),
      desc: String(form.desc || '').trim()
    }
  }
}

export function nacosInstancePayload(service, instance, enabled) {
  return {
    serviceName: service.name,
    groupName: service.groupName || nacosDefaultGroup,
    namespaceId: service.namespaceId || '',
    ip: String(instance.ip || ''),
    port: String(instance.port ?? ''),
    enabled: enabled ? 'true' : 'false',
    weight: String(instance.weight ?? 1),
    ephemeral: String(instance.ephemeral !== false),
    clusterName: instance.clusterName || ''
  }
}

export function nacosNamespacePayload(form) {
  const name = String(form.namespaceName || '').trim()
  if (!name) return { ok: false, error: '请填写命名空间名称' }
  const id = String(form.namespaceId || '').trim()
  if (id && !/^[a-zA-Z0-9-_.]+$/.test(id)) return { ok: false, error: '命名空间 ID 只能包含字母、数字、- _ .' }
  return {
    ok: true,
    params: {
      namespaceId: id,
      namespaceName: name,
      namespaceDesc: String(form.namespaceDesc || '').trim()
    }
  }
}

// 服务实例的上下线/移除都依赖服务身份与实例端口，统一在此拼装，避免页面遗漏字段。
export function nacosServiceIdentity(service, namespaceId) {
  return {
    serviceName: service.name,
    groupName: service.groupName || nacosDefaultGroup,
    namespaceId: service.namespaceId || namespaceId || ''
  }
}
