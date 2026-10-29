export function normalizeRedisSchema(value = 'db0') {
  const match = /^(?:db)?(\d+)$/i.exec(String(value).trim())
  const index = match ? Number(match[1]) : NaN
  if (!Number.isSafeInteger(index) || index < 0 || index > 2147483647) throw new Error('请输入有效的 DB 编号，例如 db10 或 10')
  return `db${index}`
}

// The server's command parser accepts quoted literal whitespace and escaped
// quotes/backslashes. Preserve control characters instead of turning them into
// the letters n/t when editing multi-line values.
export const quoteRedisArgument = value => `"${String(value ?? '').replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`

export function buildRedisKeyCommands(form, editing = false) {
  if (!form.key) throw new Error('请输入 Key 名称')
  if (editing && form.type !== 'string' && (form.ttl == null || form.ttl === '')) throw new Error('编辑集合类型时请明确填写 TTL，-1 表示永不过期')
  const key = quoteRedisArgument(form.key), value = String(form.value ?? '')
  let args, parsed
  if (form.type !== 'string') {
    try { parsed = JSON.parse(value) } catch { throw new Error('该类型的值需要填写合法 JSON') }
  }
  switch (form.type) {
    case 'string': args = `SET ${key} ${quoteRedisArgument(value)}${editing && form.ttl == null ? ' KEEPTTL' : ''}`; break
    case 'hash': {
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object' || !Object.keys(parsed).length) throw new Error('Hash 值需要填写非空 JSON 对象')
      args = `HSET ${key} ${Object.entries(parsed).flatMap(([field, val]) => [quoteRedisArgument(field), quoteRedisArgument(val)]).join(' ')}`; break
    }
    case 'list': case 'set': {
      if (!Array.isArray(parsed) || !parsed.length) throw new Error('值需要填写非空 JSON 数组')
      args = `${form.type === 'list' ? 'RPUSH' : 'SADD'} ${key} ${parsed.map(quoteRedisArgument).join(' ')}`; break
    }
    case 'zset': {
      if (!Array.isArray(parsed) || !parsed.length || parsed.some(item => item?.member === undefined || item.score == null || item.score === '' || !Number.isFinite(Number(item.score)))) throw new Error('ZSet 元素需要包含 member 和有效的 score')
      args = `ZADD ${key} ${parsed.flatMap(item => [quoteRedisArgument(item.score), quoteRedisArgument(item.member)]).join(' ')}`; break
    }
    default: throw new Error('该类型暂不支持可视化编辑')
  }
  const commands = editing && form.type !== 'string' ? [`DEL ${key}`, args] : [args]
  if (form.ttl != null && form.ttl !== '') {
    const ttl = Number(form.ttl)
    if (!Number.isSafeInteger(ttl) || ttl < -1) throw new Error('TTL 需要为 -1 或非负整数秒')
    commands.push(ttl === -1 ? `PERSIST ${key}` : `EXPIRE ${key} ${ttl}`)
  }
  return commands
}
