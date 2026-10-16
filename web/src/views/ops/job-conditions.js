export const conditionFields = [
  { value: 'status', label: '执行状态' },
  { value: 'successCount', label: '成功主机数' },
  { value: 'failedCount', label: '失败主机数' }
]
export const conditionStatuses = [
  { value: 'success', label: '全部成功' },
  { value: 'partial', label: '部分失败' },
  { value: 'failed', label: '全部失败' }
]
export const conditionOperators = [
  { value: 'eq', label: '等于' }, { value: 'ne', label: '不等于' },
  { value: 'gt', label: '大于' }, { value: 'gte', label: '大于或等于' },
  { value: 'lt', label: '小于' }, { value: 'lte', label: '小于或等于' }
]

export function conditionSummary(config = {}) {
  const field = conditionFields.find((item) => item.value === config.field)?.label || '执行状态'
  const operator = conditionOperators.find((item) => item.value === config.operator)?.label || '等于'
  const value = config.field === 'status' ? conditionStatuses.find((item) => item.value === config.value)?.label || '全部成功' : config.value ?? 0
  return `${field} ${operator} ${value}`
}

export function validateJobBranches(definition) {
  const nodes = new Map(definition.nodes.map((node) => [node.id, node]))
  for (const node of definition.nodes) {
    const incoming = definition.edges.filter((edge) => edge.target === node.id)
    const outgoing = definition.edges.filter((edge) => edge.source === node.id)
    if (node.config?.failurePolicy === 'branch' && (!outgoing.length || outgoing.some((edge) => nodes.get(edge.target)?.type !== 'condition' || nodes.get(edge.target)?.config?.sourceStepId !== node.id))) {
      return `“${node.label}”设置失败后进入分支时，所有后续步骤必须是判断此步骤的条件分支`
    }
    if (node.type !== 'condition') continue
    const source = nodes.get(incoming[0]?.source)
    if (incoming.length !== 1 || !['script', 'file'].includes(source?.type) || node.config?.sourceStepId !== source?.id) return `请将“${node.label}”连接到一个脚本或文件步骤，并选择该步骤作为判断来源`
    if (outgoing.filter((edge) => edge.branch === 'true').length !== 1 || outgoing.filter((edge) => edge.branch === 'false').length !== 1 || outgoing.length !== 2) return `请为“${node.label}”的满足条件和不满足出口各连接一个后续步骤`
    const config = node.config || {}
    if (!conditionFields.some((item) => item.value === config.field) || !conditionOperators.some((item) => item.value === config.operator)) return `“${node.label}”的判断条件不完整`
    if (config.field === 'status') {
      if (!['eq', 'ne'].includes(config.operator) || !conditionStatuses.some((item) => item.value === config.value)) return `“${node.label}”的执行状态条件不正确`
    } else if (!Number.isInteger(config.value) || config.value < 0 || config.value > 2147483647) return `“${node.label}”的比较值必须是有效的非负整数`
  }
  return ''
}
