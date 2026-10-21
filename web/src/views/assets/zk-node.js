// ZooKeeper 节点展示与路径处理：只做纯计算，避免把格式化逻辑散落在页面里。
export function zkParentPath(path = '/') {
  if (!path || path === '/') return '/'
  const index = path.lastIndexOf('/')
  return index <= 0 ? '/' : path.slice(0, index)
}

export function zkJoinPath(parent = '/', name = '') {
  if (parent === '/' || parent === '') return `/${name}`
  return `${parent}/${name}`
}

export function zkSegments(path = '/') {
  const segments = [{ name: '/', path: '/' }]
  if (path === '/') return segments
  path.split('/').filter(Boolean).forEach((name, index, list) => {
    segments.push({ name, path: `/${list.slice(0, index + 1).join('/')}` })
  })
  return segments
}

export function zkNameValid(name = '') {
  return Boolean(name) && !name.includes('/') && name !== '.' && name !== '..' && !/[\u0000-\u001f]/.test(name)
}

export function zkFormatBytes(size) {
  const n = Number(size)
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function zkHex(value) {
  const n = Number(value)
  if (!Number.isFinite(n)) return '—'
  return `0x${n.toString(16)}`
}

export function zkTime(ms) {
  const n = Number(ms)
  if (!Number.isFinite(n) || n <= 0) return '—'
  const d = new Date(n)
  const pad = v => String(v).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function zkPretty(text) {
  const raw = typeof text === 'string' ? text : ''
  if (!raw.trim()) return { ok: false, value: raw }
  try {
    return { ok: true, value: JSON.stringify(JSON.parse(raw), null, 2) }
  } catch {
    return { ok: false, value: raw }
  }
}

// 字段顺序与 ZooKeeper 命令行 stat 输出一致，便于和 zkCli 对照。
export function zkStatRows(stat = {}) {
  const pick = key => stat?.[key]
  return [
    { key: 'ephemeralOwner', value: zkHex(pick('EphemeralOwner') ?? 0) },
    { key: 'mtime', value: zkTime(pick('Mtime')) },
    { key: 'ctime', value: zkTime(pick('Ctime')) },
    { key: 'mZxid', value: zkHex(pick('Mzxid')) },
    { key: 'pZxid', value: zkHex(pick('Pzxid')) },
    { key: 'cZxid', value: zkHex(pick('Czxid')) },
    { key: 'dataLength', value: String(pick('DataLength') ?? 0) },
    { key: 'numChildren', value: String(pick('NumChildren') ?? 0) },
    { key: 'dataVersion', value: String(pick('Version') ?? 0) },
    { key: 'aclVersion', value: String(pick('Aversion') ?? 0) },
    { key: 'cVersion', value: String(pick('Cversion') ?? 0) }
  ]
}

// 树节点由后端摘要构造；缺少摘要时退化为可按需展开的普通节点。
export function zkTreeNodes(payload) {
  const items = Array.isArray(payload?.items) ? payload.items : []
  if (items.length) {
    return items.map(item => ({
      name: item.name,
      path: item.path,
      hasChildren: Boolean(item.hasChildren),
      isLeaf: !item.hasChildren,
      numChildren: item.numChildren || 0,
      dataLength: item.dataLength || 0
    }))
  }
  const names = Array.isArray(payload?.children) ? payload.children : []
  const base = payload?.path === '/' ? '' : payload?.path || ''
  return names.map(name => ({ name, path: `${base}/${name}`, hasChildren: true, isLeaf: false, numChildren: 0, dataLength: 0 }))
}
