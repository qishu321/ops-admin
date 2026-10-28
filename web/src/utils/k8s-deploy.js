export const deploymentStates = {
  draft: ['草稿', 'info'], prechecking: ['预检中', 'primary'], ready: ['待部署', 'success'],
  precheck_failed: ['预检未通过', 'danger'], deploying: ['部署中', 'primary'], stopping: ['停止中', 'warning'],
  stopped: ['已停止', 'info'], failed: ['部署失败', 'danger'], needs_review: ['待核对', 'warning'], success: ['部署成功', 'success']
}
export const runStates = { running: '执行中', success: '成功', failed: '失败', warning: '需处理', stopped: '已停止', needs_review: '待核对' }
export const activeDeployment = (status) => ['prechecking', 'deploying', 'stopping'].includes(status)
export const deploymentEditable = (status) => ['draft', 'ready', 'precheck_failed', 'failed', 'stopped'].includes(status)
export const statusText = (status) => deploymentStates[status]?.[0] || status
export const statusType = (status) => deploymentStates[status]?.[1] || 'info'
export const formatDeployTime = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
export const roleText = (role) => role === 'master' ? '控制平面' : '工作节点'

export function emptyDeployment() {
  return {
    id: 0, revision: 0, name: '', env: '', description: '', nodes: [],
    config: {
      version: '1.34.9', runtimeDirectory: '', networkPlugin: 'calico', podCIDR: '172.30.0.0/16', serviceCIDR: '10.254.0.0/16',
      interface: 'eth0', autoHostname: false, disableSELinux: true, disableFirewalld: false, imageRepository: 'registry.cn-hangzhou.aliyuncs.com/google_containers',
      customSource: '', kubeletConfig: '', ha: false, builtinLB: false, apiServer: '', apiPort: 6443, routerId: 68,
      proxyMode: 'iptables', customCertificates: false, certificateYears: 1, caYears: 10,
      metricsServer: false, allowControlPlaneWorkloads: false
    }
  }
}

export function nodeFromAsset(asset, config, index) {
  const ip = asset.privateIp || asset.sshIp || asset.publicIp || ''
  return {
    hostId: asset.id, hostName: asset.hostName, os: asset.os, arch: asset.arch, cpu: asset.cpu, memory: asset.memory,
    role: index === 0 ? 'master' : 'worker', ip,
    name: config.autoHostname ? `k8s-${ip.replaceAll('.', '-')}` : asset.hostName?.toLowerCase() || '',
    interface: config.interface || 'eth0'
  }
}

export function deploymentSummary(plan) {
  const c = plan.config || {}
  const masters = (plan.nodes || []).filter(n => n.role === 'master').length
  return { masters, workers: (plan.nodes || []).length - masters,
    endpoint: c.apiServer ? `${c.apiServer}:${c.apiPort}` : `${(plan.nodes || []).find(n => n.role === 'master')?.ip || '首个控制平面 IP'}:${c.apiPort || 6443}` }
}
