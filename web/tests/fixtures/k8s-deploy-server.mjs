// Isolated browser acceptance fixture. In-memory data only; NO proxy, database or SSH client.
// Run after `npm run build`: node tests/fixtures/k8s-deploy-server.mjs
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, extname, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { emptyDeployment } from '../../src/utils/k8s-deploy.js'
import { NodeAddMemory } from './k8s-node-add-memory.mjs'
import { createRedisWorkbenchMemory } from './redis-workbench-memory.mjs'
import { createK8sWorkloadMemory } from './k8s-workload-memory.mjs'

const root = resolve(fileURLToPath(new URL('../../dist', import.meta.url)))
const user = { id: 990001, username: 'acceptance-test', nickname: '隔离验收', accountType: 'administrator' }
const pipelinePlans = new Map()
const plans = new Map(), runs = new Map(), steps = new Map(), logs = new Map()
const requests = []
let planId = 100, runId = 200, stepId = 300, logId = 400
const hosts = Array.from({ length: 12 }, (_, i) => ({
  id: i + 1, hostName: i === 0 ? 'test-cp-01' : `test-worker-${String(i).padStart(2, '0')}`, privateIp: `192.0.2.${i + 10}`,
  publicIp: '', sshIp: `192.0.2.${i + 10}`, os: i === 9 ? 'Debian GNU/Linux 12' : 'Ubuntu 24.04.3 LTS',
  arch: 'x86_64', cpu: '4', memory: '8G', disk: '50G', environment: 'test', groupId: 1,
  connectionMode: i === 1 ? 'gateway' : 'direct', gatewayName: i === 1 ? 'test-gateway' : '', credentialName: '模拟 SSH 凭据', authType: 'privateKey',
  aliveStatus: 1, authStatus: 1, disabledReason: i === 10 ? '请先在 CMDB 配置有效凭据' : '', occupiedPlanId: i === 11 ? 999 : 0
}))
const nodeAdd = new NodeAddMemory(hosts)
const redisWorkbench = createRedisWorkbenchMemory()
const workloadMemory = createK8sWorkloadMemory()
setInterval(() => { for (const task of nodeAdd.tasks.values()) nodeAdd.advance(task.id) }, 900).unref()
function respond(res, data, status = 200, message = 'success') { res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' }); res.end(JSON.stringify({ code: status, data, message })) }
function step(run, host, title, status = 'success', summary = '模拟执行通过') {
  const record = { id: ++stepId, runId: run.id, hostId: host.hostId, hostName: host.hostName, title, status, summary, startedAt: new Date(), finishedAt: new Date() }
  steps.get(run.id).push(record)
  logs.get(run.id).push({ id: ++logId, runId: run.id, stepId: record.id, hostId: host.hostId, content: `[模拟] ${host.hostName} · ${title}：${summary}\n` })
}
const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url, 'http://127.0.0.1:18081')
    if (url.pathname === '/__test/state') return respond(res, { plans: [...plans.values()], requests, redisWorkbench: redisWorkbench.inspect(), workloadMemory: workloadMemory.inspect(), externalRequests: 0 })
    if (url.pathname.startsWith('/api/')) {
      let raw = ''; for await (const chunk of req) { raw += chunk; if (raw.length > 1e6) throw new Error('payload too large') }
      const body = raw ? JSON.parse(raw) : {}; requests.push({ method: req.method, path: url.pathname })
      if (url.pathname === '/api/v1/k8s/cluster/list') return respond(res, [])
      if (url.pathname === '/api/v1/asset/host/list') return respond(res, { list: hosts, total: hosts.length })
      // Pipeline configuration acceptance only. Never execute a release or SSH command.
      if (url.pathname === '/api/v1/ops/application/options') return respond(res, [{ id: 8801, name: 'SSH 发布验收应用', code: 'demo', branch: 'main', env: 'test' }])
      if (['/api/v1/ops/application/pipeline/template/list', '/api/v1/ops/application/artifact/list', '/api/v1/ops/application/image-registry/list', '/api/v1/notify/rule/options'].includes(url.pathname)) return respond(res, [])
      if (url.pathname === '/api/v1/ops/application/pipeline/list') return respond(res, { list: [...pipelinePlans.values()], total: pipelinePlans.size })
      if (url.pathname === '/api/v1/ops/application/pipeline/save') { const item = { ...body, id: body.id || 8801 }; pipelinePlans.set(item.id, item); return respond(res, true) }
      if (url.pathname === '/api/v1/ops/application/pipeline/info') { const pipeline = pipelinePlans.get(Number(url.searchParams.get('id'))); return respond(res, { pipeline, stages: JSON.parse(pipeline.definitionJson).stages }) }
      // SLS acceptance data is local only; no Alibaba Cloud requests or credentials.
      const slsSource = {id: 8901, name: 'SLS 隔离验收', type:'sls', url:'https://cn-hangzhou.log.aliyuncs.com', status:1, sls:{region:'cn-hangzhou', project:'demo-project', cloudAccountId:1, defaultLogstore:'app', logstores:[]}}
      if (url.pathname === '/api/v1/asset/cloudAccount/options') return respond(res,[{id:1,name:'阿里云测试账号',provider:'aliyun'}])
      if (url.pathname === '/api/v1/monitor/datasource/options') return respond(res,[slsSource])
      if (url.pathname === '/api/v1/monitor/datasource/list') return respond(res,{list:[slsSource],total:1})
      if (url.pathname === '/api/v1/monitor/datasource/info') return respond(res,slsSource)
      if (url.pathname === '/api/v1/monitor/logs/indices') return respond(res,[{name:'app'},{name:'audit-op-log'},{name:'restricted-index'}])
      if (url.pathname === '/api/v1/monitor/logs/shortcuts') return respond(res,[])
      if (url.pathname === '/api/v1/monitor/logs/fields' && url.searchParams.get('index') === 'restricted-index') return respond(res,null,403,'读取索引失败：需要 log:GetIndex 权限；日志查询权限不受影响')
      if (url.pathname === '/api/v1/monitor/logs/fields') return respond(res,[{name:'*',kind:'fulltext',type:'text'}, ...(url.searchParams.get('index') === 'audit-op-log' ? [{name:'actionType',type:'long',indexed:true,statistics:true,kind:'field'}] : [{name:'level',type:'text',indexed:true,statistics:false,kind:'field'},{name:'details.code',type:'long',indexed:true,statistics:true,kind:'field'}])])
      if (url.pathname === '/api/v1/monitor/logs/field-values') return respond(res,[{value:'ERROR',hits:3,field:'level'},{value:'INFO',hits:5,field:'level'}])
      if (url.pathname === '/api/v1/monitor/logs/query') { if (body.query === 'slow-test') await new Promise(resolve => setTimeout(resolve, 3000)); if (body.query === 'error-test') return respond(res,null,400,'SLS 查询日志失败：访问被拒绝，请检查云账号 RAM 权限 [Unauthorized] Request ID: test-request'); const sql = (body.query || '').includes('|'); return respond(res,{items:sql?[{count:'42'}]:[{timestamp:new Date().toISOString(),level:'ERROR',message:'SLS 验收日志',source:{level:'ERROR',content:'SLS 验收日志'}}],columns:sql?['count']:[],isSQL:sql,complete:true,totalExact:true,histogramSkipped:!!body.skipHistogram,total:sql?1:1,histogram:sql?[]:[{key:Date.now(),doc_count:1}],hasMore:false,took:12}) }
      if (url.pathname === '/api/v1/profile') return respond(res, { user, permissionList: ['*:*:*'], leftMenuList: [], accessRevision: 'isolated', systemConfig: { siteName: 'Ops Admin · 隔离验收', primaryColor: '#3677ff' } })
      if (url.pathname === '/api/v1/systemConfig/public') return respond(res, { siteName: 'Ops Admin · 隔离验收' })
      if (url.pathname === '/api/v1/ops/environment/list') return respond(res, [{ id: 1, code: 'test', name: '测试环境' }])
      if (url.pathname === '/api/v1/asset/hostGroup/list') return respond(res, { list: [{ id: 1, name: '模拟服务器组' }] })
      if (url.pathname === '/api/v1/auth/refresh') return respond(res, { token: 'isolated-test-token', accessTokenExpiresAt: Date.now() + 3600000, sessionExpiresAt: Date.now() + 3600000 })
      if (url.pathname === '/api/v1/asset/service/runtime-topology') return respond(res, {service:{id:10,name:'服务拓扑验收',serviceUid:'acceptance-service-10'},cluster:{id:901,name:'测试集群'},namespace:'demo',workloads:['nginx-gm','szfc-mgr','szfc-gate','szfc-notice','szfc-login','szfc-contact','szfc-social','szfc-battle','szfc-home','szfc-world'].map(name=>({name,type:name.includes('home')||name.includes('world')?'StatefulSet':'Deployment',ready:'2/2',available:2}))})
      // Read-only Kafka responses for language-switch acceptance; never contact a broker.
      if (url.pathname === '/api/v1/asset/middleware/list') return respond(res, url.searchParams.get('type') === 'kafka' ? [{ id: 7101, name: '内网kafka', type: 'kafka', endpoints: ['192.0.2.169:9094'], description: '测试消息集群', authType: 'none' }] : [])
      if (url.pathname === '/api/v1/asset/credential/options') return respond(res, [])
      if (url.pathname === '/api/v1/asset/middleware/query') {
        const partition = { id: 0, leader: 0, replicas: [0], isr: [0], firstOffset: '0', nextOffset: '1', offsetSpan: '1' }
        const responses = {
          topics: [{ Name: '业务订单', Partitions: [partition], Internal: false }], brokers: [{ ID: 0, Host: '192.0.2.169', Port: 9094 }], groups: [{ GroupID: '业务消费组', ProtocolType: 'consumer' }],
          'topic.overview': { partitions: [partition], offsetSpan: '1', isrCount: 1, replicaCount: 1, underReplicated: 0, fetchedAt: '2026-10-29T08:00:00Z' },
          'topic.configs': [{ name: 'cleanup.policy', value: 'delete', source: 1, readOnly: false, sensitive: false }],
          'topic.messages': { items: [{ partition: 0, offset: '0', timestamp: '2026-10-29T08:00:00Z', key: { text: '订单编号', encoding: 'utf8', size: 12 }, value: { text: '中文业务消息保持原文', encoding: 'utf8', size: 30 }, headers: [] }], nextOffset: '1', hasMore: false },
          'topic.consumers': { items: [], warnings: [], scanned: 0, totalGroups: 0, hasMore: false }
        }
        return Object.hasOwn(responses, body.operation) ? respond(res, responses[body.operation]) : respond(res, null, 400, 'Unsupported acceptance query')
      }
      if (redisWorkbench.handle(req, url, body, respond, res)) return
      if (workloadMemory.handle(req, url, body, respond, res)) return
      if (nodeAdd.handle(req, url, body, respond, res)) return
      const action = url.pathname.replace('/api/v1/k8s/deploy/', '')
      const id = Number(url.searchParams.get('id') || body.id), plan = plans.get(id)
      if (action === 'capabilities') return respond(res, { versions: ['1.34.9', '1.33.5'], versionRange: '1.31.x–1.34.x', systems: 'Ubuntu 22.04 / 24.04、Debian 12 / 13', plugins: [{ value: 'calico', label: 'Calico' }, { value: 'flannel', label: 'Flannel' }, { value: 'cilium', label: 'Cilium（待适配）', disabled: true }] })
      if (action === 'list') {
        const list = [...plans.values()].filter(p => p.name.includes(url.searchParams.get('keyword') || '') && (!url.searchParams.get('status') || p.status === url.searchParams.get('status')))
        const size = Number(url.searchParams.get('pageSize') || 10), page = Number(url.searchParams.get('pageNum') || 1)
        return respond(res, { list: list.slice((page - 1) * size, page * size), total: list.length })
      }
      if (action === 'host-options') {
        const keyword = url.searchParams.get('keyword') || '', filtered = hosts.filter(h => (h.hostName + h.privateIp).includes(keyword)), page = Number(url.searchParams.get('pageNum') || 1)
        return respond(res, { list: filtered.slice((page - 1) * 10, page * 10), total: filtered.length })
      }
      if (action === 'save') {
        if (plan && plan.revision !== body.revision) return respond(res, null, 400, '计划版本冲突')
        const saved = { ...emptyDeployment(), ...body, id: plan?.id || ++planId, revision: (plan?.revision || 0) + 1, status: 'draft', createdBy: user.username, createTime: plan?.createTime || new Date().toISOString(), registrationStatus: 'pending' }
        plans.set(saved.id, saved); return respond(res, saved)
      }
      if (!plan) return respond(res, null, 404, '模拟部署不存在')
      if (action === 'info') return respond(res, { plan, runs: [...runs.values()].filter(r => r.planId === id).reverse(), steps: steps.get(plan.lastRunId) || [] })
      if (action === 'logs') { const r = runs.get(Number(url.searchParams.get('runId'))); return respond(res, { run: r, steps: steps.get(r.id), logs: (logs.get(r.id) || []).filter(l => l.id > Number(url.searchParams.get('after') || 0)) }) }
      if (['precheck', 'start'].includes(action)) {
        if (['deploying', 'prechecking'].includes(plan.status)) return respond(res, runs.get(plan.lastRunId))
        const run = { id: ++runId, planId: id, revision: plan.revision, kind: action === 'precheck' ? 'precheck' : 'deploy', status: 'running', config: structuredClone(plan.config), nodes: structuredClone(plan.nodes), startedAt: new Date(), operator: user.username }
        runs.set(run.id, run); steps.set(run.id, []); logs.set(run.id, []); plan.lastRunId = run.id; plan.status = action === 'precheck' ? 'prechecking' : 'deploying'
        setTimeout(() => {
          const fail = plan.nodes.some(n => n.hostId === 10)
          for (const node of plan.nodes) step(run, node, '节点预检', fail ? 'failed' : 'success', fail ? '混合系统不受支持' : 'Ubuntu 24.04 / x86_64，系统和主机身份核验通过')
          if (run.kind === 'deploy' && !fail) { for (const title of ['安装组件', '初始化控制平面', '安装容器网络', '验证节点就绪', '接入集群管理']) step(run, plan.nodes[0], title); plan.registrationStatus = 'failed'; plan.registrationError = '模拟网络不可达：安装完成，可单独重试接入。' }
          run.status = fail ? 'failed' : 'success'; run.error = fail ? '所有节点需要一致的发行版和版本' : ''; run.finishedAt = new Date(); plan.status = fail ? 'precheck_failed' : run.kind === 'precheck' ? 'ready' : 'success'
        }, 1200)
        return respond(res, run)
      }
      if (action === 'register-cluster') { plan.registrationStatus = 'registered'; plan.clusterId = 900; return respond(res, 900) }
      if (action === 'delete') { plans.delete(id); return respond(res, true) }
      return respond(res, null, 404, '模拟接口未实现，未转发至任何真实服务')
    }
    let filename = resolve(root, '.' + decodeURIComponent(url.pathname))
    if (!filename.startsWith(root + sep) && filename !== root) return respond(res, null, 403, 'invalid path')
    let content
    try { content = await readFile(filename) } catch { filename = resolve(root, 'index.html'); content = await readFile(filename) }
    if (extname(filename) === '.html') {
      const boot = `<script>sessionStorage.setItem('ops-admin-token','isolated-test-token');sessionStorage.setItem('ops-admin-token-expires-at',String(Date.now()+3600000));sessionStorage.setItem('ops-admin-session-expires-at',String(Date.now()+3600000));sessionStorage.setItem('ops-admin-last-activity-at',String(Date.now()));sessionStorage.setItem('ops-admin-user',${JSON.stringify(JSON.stringify(user))});</script>`
      content = Buffer.from(content.toString().replace('<head>', '<head>' + boot))
    }
    res.writeHead(200, { 'Content-Type': ({ '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.svg': 'image/svg+xml' })[extname(filename)] || 'application/octet-stream', 'Content-Security-Policy': "connect-src 'self'; img-src 'self' data:; frame-src 'none'" })
    res.end(content)
  } catch (error) { respond(res, null, 500, error.message) }
})
const port = Number(process.env.OPS_ADMIN_MOCK_PORT || 18081)
if (![18081, 18082].includes(port)) throw new Error('Acceptance fixture only supports isolated ports 18081 or 18082')
server.listen(port, '127.0.0.1', () => console.log(`Isolated acceptance fixture: http://127.0.0.1:${port}/containers/k8s/nodes (memory only; no upstream)`))
