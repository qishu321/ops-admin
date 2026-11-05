// Preserve the SQL tail while adding a predicate to the SLS search expression.
export function appendSLSFilter(query, field, value, exclude = false, type = 'text') {
  if (!field || /[\u0000-\u001f]/.test(field)) throw new Error('字段名无效')
  const identifier = /^[a-zA-Z_][a-zA-Z0-9_.]*$/.test(field) && !/^(and|or|not|in)$/i.test(field) || /^__tag__:[a-zA-Z0-9_.-]+$/.test(field) ? field : JSON.stringify(field)
  let quote = '', split = -1
  for (let i = 0; i < query.length; i++) {
    const char = query[i]
    if (quote) {
      if (char === '\\') i++
      else if (char === quote) quote = ''
    } else if (char === '"' || char === "'") quote = char
    else if (char === '|') { split = i; break }
  }
  const search = (split < 0 ? query : query.slice(0, split)).trim()
  const sql = split < 0 ? '' : query.slice(split)
  const text = String(value)
  const numeric = type === 'long' ? /^-?\d+$/.test(text) : type === 'double' && /^-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(text) && Number.isFinite(Number(text))
  const literal = numeric ? text : JSON.stringify(text)
  const predicate = `${exclude ? 'NOT ' : ''}${identifier}${numeric ? '=' : ':'}${literal}`
  return `${search && search !== '*' ? `(${search}) AND ` : ''}${predicate}${sql ? ` ${sql}` : ''}`
}
export function slsStatsKey(datasourceId, logstore, query, range) {
  return JSON.stringify([datasourceId, logstore, query, range.startAt, range.endAt])
}

export function slsFieldValue(source, field) {
 if (Object.hasOwn(source || {}, field)) return source[field]
 let value = source
 for (const part of field.split('.')) {
  if (typeof value === 'string') { try { value = JSON.parse(value) } catch { return undefined } }
  value = value?.[part]
 }
 return value
}
export function mergeSLSFields(indexFields, items) {
 const indexed = indexFields.filter(field => field.kind !== 'fulltext').map(field => ({...field,indexed:true}))
 const names = new Set(indexed.map(field => field.name))
 const sampled = [...new Set(items.flatMap(item => Object.keys(item.source || {})))].filter(name => !names.has(name)).sort().map(name => ({name,type:'未建索引',indexed:false,statistics:false}))
 return [...indexed, ...sampled]
}

export function slsLogLevel(source, message = '') {
 const normalize = value => {
  const level = String(value || '').toUpperCase()
  return ({WARNING:'WARN',ERR:'ERROR',CRITICAL:'FATAL'})[level] || (/^(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)$/.test(level) ? level : '')
 }
 for (const key of ['level','severity','log_level','log.level']) {const level = normalize(source?.[key]); if (level) return level}
 try {const data = JSON.parse(message); for (const key of ['level','severity','log_level']) {const level=normalize(data[key]);if(level)return level}} catch {}
 const klog = message.match(/^\[?([IWEF])\d{4}\s/)
 if (klog) return ({I:'INFO',W:'WARN',E:'ERROR',F:'FATAL'})[klog[1]]
 return message.match(/(?:^|[\s\[])(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)(?=[\s\]:]|$)/)?.[1] || '-'
}
export function slsBody(row, pretty = false) {
 const source=row.source || {}
 if (pretty && !source.message && !source.content && !source.log && Object.keys(source).length) return JSON.stringify(source, null, 2)
 return source.message || source.content || source.log || row.message || JSON.stringify(source)
}
