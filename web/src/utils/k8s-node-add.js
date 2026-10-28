export const nodeAddStages = ['准备', '环境检查', '安装组件', '加入集群', '等待 Ready', '完成']
export const nodeAddStatus = { running: '执行中', success: '成功', partial: '部分失败', failed: '失败', cancelled: '已取消', needs_review: '需要核对', pending: '待执行' }
export const nodeAddTone = status => ({ success: 'success', failed: 'danger', needs_review: 'danger', partial: 'warning', cancelled: 'info', running: 'primary' })[status] || 'info'

function keyValid(key) {
  const parts = key.split('/')
  const name = parts.pop()
  return parts.length <= 1 && name.length <= 63 && /^[a-zA-Z0-9]([a-zA-Z0-9_.-]*[a-zA-Z0-9])?$/.test(name) &&
    (!parts.length || (parts[0].length <= 253 && parts[0].split('.').every(p => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(p))))
}
const valueValid = value => value.length <= 63 && (!value || /^[a-zA-Z0-9]([a-zA-Z0-9_.-]*[a-zA-Z0-9])?$/.test(value))
export function parseNodeMetadata(text, taints = false) {
  const values = [], keys = new Set()
  for (const line of String(text || '').split('\n').map(l => l.trim()).filter(Boolean)) {
    let pair = line, effect
    if (taints) {
      const index = line.lastIndexOf(':')
      pair = line.slice(0, index); effect = line.slice(index + 1)
      if (index < 1 || !['NoSchedule', 'PreferNoSchedule', 'NoExecute'].includes(effect)) throw new Error('污点格式：key=value:NoSchedule，每行一条')
    }
    const index = pair.indexOf('=')
    const key = index < 0 ? pair : pair.slice(0, index), value = index < 0 ? '' : pair.slice(index + 1)
    if (!keyValid(key) || !valueValid(value)) throw new Error(`标签或污点格式无效：${line}`)
    if (['node-role.kubernetes.io/', 'node-restriction.kubernetes.io/', 'kubernetes.io/', 'k8s.io/'].some(prefix => key.startsWith(prefix))) throw new Error('不能用自定义标签设置节点角色或受保护的系统标签')
    const identity = `${key}:${effect || ''}`
    if (keys.has(identity)) throw new Error(`存在重复配置：${key}`)
    keys.add(identity); values.push({ key, value, ...(taints ? { effect } : {}) })
  }
  return values
}
export function validateNodeDraft(draft) {
  if (!draft.nodes?.length) throw new Error('请至少选择一台服务器')
  if (draft.nodes.length > 50) throw new Error('单次最多添加 50 台服务器')
  const names = new Set(), hosts = new Set()
  for (const node of draft.nodes) {
    const name = String(node.nodeName || '')
    if (!name || name.length > 63 || !name.split('.').every(part => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(part))) throw new Error(`节点名称无效（最多 63 字符）：${name || '未填写'}`)
    if (names.has(name) || hosts.has(node.hostId)) throw new Error('不能重复选择服务器或使用相同节点名称')
    names.add(name); hosts.add(node.hostId)
  }
  parseNodeMetadata(draft.labels); parseNodeMetadata(draft.taints, true)
}
