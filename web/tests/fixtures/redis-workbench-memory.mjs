// Browser acceptance data only. No Redis/SQL connection and no upstream proxy.
export function createRedisWorkbenchMemory() {
  const store = new Map(Array.from({ length: 16 }, (_, i) => [`db${i}`, new Map(i === 10 ? [['demo:key', '这条数据只存在于 db10']] : [])]))
  const history = [], calls = []
  const connection = { id: 7001, name: 'Redis 验收连接', host: '192.0.2.57', port: 6379, dbType: 'redis', dbName: '0', env: 'test', environment: 'test', accessMode: 'readwrite', connectStatus: 1,
    capabilities: { sql: false, schema: true, resourceData: true, tableData: false, keyEdit: true, transfer: false, backup: false } }
  function handle(req, url, body, respond, res) {
    const path = url.pathname
    if (path === '/api/v1/asset/database/list') { respond(res, { list: [connection], total: 1 }); return true }
    if (!path.startsWith('/api/v1/dbms/')) return false
    const schema = String(body.schema || url.searchParams.get('schema') || 'db0')
    calls.push({ path, schema, body: structuredClone(body) })
    if (path === '/api/v1/dbms/workbench') { respond(res, connection); return true }
    if (path === '/api/v1/dbms/schema/tree') { respond(res, { schemas: [...store].map(([name, keys]) => ({ name, keyCount: keys.size, tables: [] })), defaultSchema: 'db0', databaseCount: 16, clusterMode: false, warnings: [] }); return true }
    if (path === '/api/v1/dbms/sql/history') { respond(res, { list: [...history].reverse(), total: history.length }); return true }
    if (!store.has(schema)) { respond(res, null, 400, 'DB 编号超出范围'); return true }
    const keys = store.get(schema)
    if (path === '/api/v1/dbms/redis/keys') {
      const pattern = url.searchParams.get('pattern') || '*'
      const matcher = new RegExp('^' + pattern.split('*').map(part => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('.*') + '$')
      respond(res, { schema, keys: [...keys.keys()].filter(key => matcher.test(key)).map(key => ({ key, type: 'string', ttl: '永不过期', ttlSeconds: -1 })), total: keys.size, nextCursor: '0', complete: true, warnings: [] }); return true
    }
    if (path === '/api/v1/dbms/resource/data') { const key = url.searchParams.get('table'); respond(res, { rows: keys.has(key) ? [{ key, type: 'string', ttl: '永不过期', ttlSeconds: -1, value: keys.get(key), truncated: false }] : [] }); return true }
    const parts = String(body.commandText || '').match(/"(?:\\.|[^"\\])*"|\S+/g) || []
    const args = parts.map(part => part.startsWith('"') ? part.slice(1, -1).replace(/\\(["\\])/g, '$1') : part)
    const command = (args[0] || '').toUpperCase(), writeOperation = !['GET', 'DBSIZE', 'TTL', 'TYPE', 'SCAN'].includes(command)
    if (path === '/api/v1/dbms/redis/analyze') { respond(res, { schema, command, writeOperation, accessMode: 'readwrite' }); return true }
    if (path === '/api/v1/dbms/redis/execute') {
      if (writeOperation && !body.confirmed) { respond(res, null, 400, '写入需要确认'); return true }
      let reply = '(nil)'
      if (command === 'DBSIZE') reply = String(keys.size)
      if (command === 'GET') reply = keys.get(args[1]) ?? '(nil)'
      if (command === 'TYPE') reply = keys.has(args[1]) ? 'string' : 'none'
      if (command === 'TTL') reply = keys.has(args[1]) ? '-1' : '-2'
      if (command === 'SET') { keys.set(args[1], args[2]); reply = 'OK' }
      if (command === 'DEL') reply = keys.delete(args[1]) ? '1' : '0'
      const entry = { id: history.length + 1, databaseId: 7001, schemaName: schema, sqlText: body.commandText, status: 1, durationMs: 1, operator: 'test-user', createTime: new Date().toISOString() }
      history.push(entry)
      respond(res, { schema, command, rows: [{ command: body.commandText, result: reply }], durationMs: 1, executionId: `fixture-${entry.id}` }); return true
    }
    return false
  }
  return { handle, inspect: () => ({ calls, databases: [...store].map(([schema, keys]) => ({ schema, keys: Object.fromEntries(keys) })) }) }
}
