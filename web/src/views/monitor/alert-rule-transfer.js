const fields = ['recoveryQuery','recoveryComparator','recoveryThreshold','recoveryForSeconds','missingDataPolicy','identityLabels','discoveryWindowSeconds','healthQuery','name','alertType','datasourceScope','promql','logIndex','logTimeRangeSeconds','comparator','threshold','forSeconds','evalIntervalSeconds','notifyRepeatIntervalSeconds','maxNotifyCount','severity','labelsJson','annotationsJson','notifyEnabled','notifyRecoveryEnabled','env','status','description']
export const ruleFileFormat = 'ops-admin-alert-rules'
export function resolveRuleImportTarget(payload, existing, strategy) {
 const matches=existing.filter(rule=>rule.name===payload.name)
 if(matches.length && strategy==='skip')return null
 if(matches.length && strategy==='update') {
  if(matches.length!==1)throw new Error(`规则“${payload.name}”存在多个同名项，无法安全更新`)
  return {...payload,id:matches[0].id}
 }
 const names=new Set(existing.map(rule=>rule.name)),base=payload.name
 let name=base
 for(let n=2;names.has(name);n++)name=`${base}（导入 ${n}）`
 return {...payload,name}
}
export function ruleTypeMatches(type, datasourceType) {
 return type === 'datasource_health' || (type === 'log' ? datasourceType === 'elasticsearch' : type === 'victorialogs' ? datasourceType === 'victorialogs' : ['prometheus','victoriametrics'].includes(datasourceType))
}
export function exportRuleDocument(rules, datasources, notifications) {
 return { format:ruleFileFormat, version:1, requiresFeatures:['alert-recovery-policy-v1'], rules:rules.map(rule => {
  const data = Object.fromEntries(fields.filter(key => rule[key] !== undefined).map(key => [key,rule[key]]))
  const ids = [...new Set(rule.datasourceIds?.length ? rule.datasourceIds : rule.datasourceId ? [rule.datasourceId] : [])]
  data.datasources = rule.datasourceScope === 'all' ? [] : ids.map(id => {
   const source = datasources.find(item => item.id === id)
   if (!source) throw new Error(`规则“${rule.name}”的数据源信息不完整，请刷新后重试`)
   return {name:source.name,type:source.type}
  })
  const notify = notifications.find(item => item.id === rule.notifyRuleId)
  if (rule.notifyEnabled && rule.notifyRuleId && !notify) throw new Error(`规则“${rule.name}”的通知规则信息不完整，请刷新后重试`)
  data.notifyRuleName = notify?.name || ''
  return data
 }) }
}
export function parseNativeRuleDocument(text) {
 const doc = JSON.parse(text.replace(/^\uFEFF/,''))
 if (doc.requiresFeatures !== undefined && (!Array.isArray(doc.requiresFeatures) || doc.requiresFeatures.some(feature=>feature!=='alert-recovery-policy-v1'))) throw new Error('规则文件包含当前版本不支持的功能，请先升级系统')
 if (doc?.format !== ruleFileFormat || doc.version !== 1 || !Array.isArray(doc.rules)) throw new Error('不是受支持的 Ops Admin 告警规则文件（version: 1）')
 if (!doc.rules.length || doc.rules.length > 500) throw new Error('每次导入应包含 1 至 500 条规则')
 return doc.rules.map((rule,index) => {
  const label = `第 ${index+1} 条规则`
  if (!rule || typeof rule !== 'object' || typeof rule.name !== 'string' || !rule.name.trim()) throw new Error(`${label}缺少名称`)
  const type = rule.alertType || 'metric'
  if (!['metric','log','victorialogs','datasource_health'].includes(type)) throw new Error(`${label}的告警类型不支持`)
  if (type !== 'datasource_health' && (typeof rule.promql !== 'string' || !rule.promql.trim())) throw new Error(`${label}缺少查询语句`)
  if (!['>','>=','<','<=','==','!='].includes(rule.comparator) || !Number.isFinite(rule.threshold)) throw new Error(`${label}的比较符或阈值无效`)
  for (const key of ['notifyEnabled','notifyRecoveryEnabled']) if(rule[key] !== undefined && typeof rule[key] !== 'boolean')throw new Error(`${label}的 ${key} 必须为布尔值`)
  if(rule.status !== undefined && ![1,2].includes(rule.status))throw new Error(`${label}的状态无效`)
  if(rule.severity !== undefined && !['P0','P1','P2','P3'].includes(rule.severity))throw new Error(`${label}的等级无效`)
  for (const key of ['forSeconds','evalIntervalSeconds','logTimeRangeSeconds','notifyRepeatIntervalSeconds','maxNotifyCount','recoveryForSeconds','discoveryWindowSeconds']) {
   if (rule[key] !== undefined && (!Number.isInteger(rule[key]) || rule[key] < 0)) throw new Error(`${label}的 ${key} 必须为非负整数`)
  }
  if (rule.missingDataPolicy && !['keep','recover','alert'].includes(rule.missingDataPolicy)) throw new Error(`${label}的无数据策略无效`)
  for (const key of ['recoveryQuery','healthQuery','identityLabels']) if (rule[key] !== undefined && typeof rule[key] !== 'string') throw new Error(`${label}的 ${key} 必须为文本`)
  if (rule.recoveryQuery && (!['>','>=','<','<=','==','!='].includes(rule.recoveryComparator) || !Number.isFinite(rule.recoveryThreshold) || !rule.identityLabels?.trim())) throw new Error(`${label}的恢复条件或关联标签不完整`)
  if (rule.missingDataPolicy === 'alert' && (!rule.healthQuery?.trim() || !rule.recoveryQuery?.trim() || !rule.discoveryWindowSeconds || rule.comparator !== '==' || rule.threshold !== 0)) throw new Error(`${label}的缺失告警需要健康查询、历史窗口、恢复查询及 == 0 触发条件`)
  for (const key of ['labelsJson','annotationsJson']) {
   if (rule[key] !== undefined) { const value = JSON.parse(rule[key]); if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${label}的 ${key} 必须为 JSON 对象`) }
  }
  if (rule.datasources !== undefined && (!Array.isArray(rule.datasources) || rule.datasources.some(ds => typeof ds?.name !== 'string' || typeof ds?.type !== 'string'))) throw new Error(`${label}的数据源引用无效`)
  return {...Object.fromEntries(fields.filter(key=>rule[key]!==undefined).map(key=>[key,rule[key]])),alertType:type,datasources:rule.datasources || [],notifyRuleName:typeof rule.notifyRuleName === 'string' ? rule.notifyRuleName : ''}
 })
}
export function prepareImportedRule(rule, datasources, notifications, settings = {}) {
 const type = rule.alertType || 'metric', errors = []
 let ids = [], scope = rule.datasourceScope === 'all' ? 'all' : 'specific'
 if (settings.datasourceIds?.length) { scope='specific';ids=settings.datasourceIds.filter(id=>datasources.some(ds=>ds.id===id && ruleTypeMatches(type,ds.type))) }
 else if (scope === 'specific') {
  for (const reference of rule.datasources || []) {
   const matches = datasources.filter(ds=>ds.name===reference.name && ds.type===reference.type && ruleTypeMatches(type,ds.type))
   if (matches.length !== 1) errors.push(`数据源“${reference.name}”${matches.length ? '存在同名项' : '未匹配'}`)
   else ids.push(matches[0].id)
  }
 }
 ids=[...new Set(ids)]
 if (scope==='specific' && !ids.length) errors.push('请选择匹配的数据源')
 if (scope==='all' && !datasources.some(ds=>ruleTypeMatches(type,ds.type))) errors.push('没有可用的同类数据源')
 let notifyRuleId=0
 const notifyEnabled=!!settings.keepNotifications && !!rule.notifyEnabled
 if (notifyEnabled && rule.notifyRuleName) {
  const matches=notifications.filter(n=>n.name===rule.notifyRuleName)
  if(matches.length!==1)errors.push(`通知规则“${rule.notifyRuleName}”未唯一匹配`)
  else notifyRuleId=matches[0].id
 } else if(notifyEnabled) errors.push('缺少通知规则名称，请关闭保留通知或先配置通知规则')
 return { errors, payload:{...Object.fromEntries(fields.filter(key=>rule[key]!==undefined).map(key=>[key,rule[key]])),name:rule.name.trim(),alertType:type,query:rule.promql || rule.queryText || '',datasourceScope:scope,datasourceIds:ids,datasourceId:ids[0] || 0,notifyEnabled,notifyRuleId,status:settings.keepStatus ? (rule.status===1?1:2) : 2} }
}
export async function collectRulePages(fetchPage, filters = {}) {
 const rules = [], seen = new Set()
 for(let pageNum=1; ; pageNum++) {
  const data=await fetchPage({...filters,pageNum,pageSize:200})
  const batch=data.list || []
  for(const rule of batch)if(!seen.has(rule.id)){seen.add(rule.id);rules.push(rule)}
  if(pageNum*200>=Number(data.total || 0))return rules
  if(!batch.length || pageNum>500)throw new Error('规则列表发生变化或数量过多，请缩小范围后重试')
 }
}
