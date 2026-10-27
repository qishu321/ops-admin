// Isolated browser acceptance fixture. In-memory data only; NO proxy, database or SSH client.
// Run after `npm run build`: node tests/fixtures/k8s-deploy-server.mjs
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, extname, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { emptyDeployment } from '../../src/utils/k8s-deploy.js'

const root = resolve(fileURLToPath(new URL('../../dist', import.meta.url)))
const user = { id: 990001, username: 'acceptance-test', nickname: '隔离验收', accountType: 'administrator' }
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
function respond(res, data, status = 200, message = 'success') { res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' }); res.end(JSON.stringify({ code: status, data, message })) }
function step(run, host, title, status = 'success', summary = '模拟执行通过') {
  const record = { id: ++stepId, runId: run.id, hostId: host.hostId, hostName: host.hostName, title, status, summary, startedAt: new Date(), finishedAt: new Date() }
  steps.get(run.id).push(record)
  logs.get(run.id).push({ id: ++logId, runId: run.id, stepId: record.id, hostId: host.hostId, content: `[模拟] ${host.hostName} · ${title}：${summary}\n` })
}
const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url, 'http://127.0.0.1:18081')
    if (url.pathname === '/__test/state') return respond(res, { plans: [...plans.values()], requests, externalRequests: 0 })
    if (url.pathname.startsWith('/api/')) {
      let raw = ''; for await (const chunk of req) { raw += chunk; if (raw.length > 1e6) throw new Error('payload too large') }
      const body = raw ? JSON.parse(raw) : {}; requests.push({ method: req.method, path: url.pathname })
      if (url.pathname === '/api/v1/profile') return respond(res, { user, permissionList: ['*:*:*'], leftMenuList: [], accessRevision: 'isolated', systemConfig: { siteName: 'Ops Admin · 隔离验收', primaryColor: '#3677ff' } })
      if (url.pathname === '/api/v1/systemConfig/public') return respond(res, { siteName: 'Ops Admin · 隔离验收' })
      if (url.pathname === '/api/v1/ops/environment/list') return respond(res, [{ id: 1, code: 'test', name: '测试环境' }])
      if (url.pathname === '/api/v1/asset/hostGroup/list') return respond(res, { list: [{ id: 1, name: '模拟服务器组' }] })
      if (url.pathname === '/api/v1/auth/refresh') return respond(res, { token: 'isolated-test-token', accessTokenExpiresAt: Date.now() + 3600000, sessionExpiresAt: Date.now() + 3600000 })
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
server.listen(18081, '127.0.0.1', () => console.log('Isolated acceptance fixture: http://127.0.0.1:18081/containers/k8s-deployments (memory only; no upstream)'))
