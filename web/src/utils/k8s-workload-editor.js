export const clone = value => JSON.parse(JSON.stringify(value))

// Presentation only: request bodies stay lossless JSON, which Kubernetes accepts as YAML.
export function manifestYAML(value, indent=0) {
  const pad=' '.repeat(indent)
  const scalar=v => v === null || typeof v !== 'object' || !Object.keys(v).length
  const literal=v => JSON.stringify(v)
  if(Array.isArray(value)) return value.map(item => scalar(item) ? `${pad}- ${literal(item)}` : `${pad}-\n${manifestYAML(item,indent+2)}`).join('\n')
  if(value && typeof value==='object') return Object.entries(value).map(([key,item]) => `${pad}${/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) ? key : JSON.stringify(key)}:${scalar(item) ? ` ${literal(item)}` : `\n${manifestYAML(item,indent+2)}`}`).join('\n')
  return `${pad}${literal(value)}`
}

export function newContainer(name = 'app') {
  return { name, image: '', imagePullPolicy: 'IfNotPresent', resources: { requests: {}, limits: {} } }
}

export function newWorkload(kind = 'Deployment', namespace = '') {
  const template = { metadata: { labels: {} }, spec: { containers: [newContainer()] } }
  const spec = kind === 'CronJob' ? { schedule: '0 * * * *', jobTemplate: { spec: { template } } } : { template }
  if (['Deployment', 'StatefulSet', 'DaemonSet'].includes(kind)) spec.selector = { matchLabels: {} }
  if (['Deployment', 'StatefulSet'].includes(kind)) spec.replicas = 1
  if (['Job', 'CronJob'].includes(kind)) template.spec.restartPolicy = 'Never'
  if (kind === 'StatefulSet') spec.serviceName = ''
  return { apiVersion: ['Job', 'CronJob'].includes(kind) ? 'batch/v1' : 'apps/v1', kind, metadata: { name: '', namespace }, spec }
}

export function podTemplate(resource) {
  return resource.kind === 'CronJob' ? resource.spec.jobTemplate.spec.template : resource.spec.template
}

export function prepareWorkload(resource) {
  const result = clone(resource)
  delete result.status
  delete result.metadata.managedFields
  podTemplate(result).metadata ||= {}
  // Keep the original UID/resourceVersion and every unmodelled spec field.
  for (const c of [...(podTemplate(result).spec.containers || []), ...(podTemplate(result).spec.initContainers || [])]) {
    c.resources ||= {}; c.resources.requests ||= {}; c.resources.limits ||= {}
  }
  return result
}

export function workloadManifest(draft, editing = false) {
  const r = clone(draft), template = podTemplate(r)
  if (!editing) {
    const label = r.metadata.name
    template.metadata ||= {}; template.metadata.labels ||= {}
    template.metadata.labels['app.kubernetes.io/name'] = label
    if (r.spec.selector) r.spec.selector.matchLabels['app.kubernetes.io/name'] = label
  }
  // Empty resource quantities are invalid. Remove only empty quantities from controlled fields.
  for (const c of [...template.spec.containers, ...(template.spec.initContainers || [])]) {
    for (const probe of ['livenessProbe','readinessProbe','startupProbe']) {
      for (const action of ['httpGet','tcpSocket']) {
        const handler = c[probe]?.[action]
        if (handler && /^\d+$/.test(String(handler.port))) handler.port = Number(handler.port)
      }
    }
    for (const key of ['requests', 'limits']) for (const quantity of Object.keys(c.resources?.[key] || {})) {
      if (c.resources[key][quantity] === '') delete c.resources[key][quantity]
    }
    if (c.resources && !Object.keys(c.resources.requests || {}).length) delete c.resources.requests
    if (c.resources && !Object.keys(c.resources.limits || {}).length) delete c.resources.limits
    if (c.resources && !Object.keys(c.resources).length) delete c.resources
  }
  for (const key of ['strategy','updateStrategy']) for (const field of ['maxSurge','maxUnavailable']) {
    const rolling = r.spec[key]?.rollingUpdate
    if (rolling?.[field] === '') delete rolling[field]
    if (rolling && /^\d+$/.test(String(rolling[field]))) rolling[field] = Number(rolling[field])
  }
  for(const volume of template.spec.volumes || []) if(volume.emptyDir?.sizeLimit === '') delete volume.emptyDir.sizeLimit
  for(const container of [...template.spec.containers,...(template.spec.initContainers || [])]) for(const env of container.env || []) if(env.valueFrom?.resourceFieldRef?.divisor === '') delete env.valueFrom.resourceFieldRef.divisor
  if(r.kind === 'CronJob' && r.spec.timeZone === '') delete r.spec.timeZone
  for (const toleration of template.spec.tolerations || []) {
    if(toleration.operator === 'Exists') delete toleration.value
    if(toleration.effect !== 'NoExecute') delete toleration.tolerationSeconds
  }
  for (const kind of ['nodeAffinity','podAffinity','podAntiAffinity']) {
    const affinity = template.spec.affinity?.[kind]
    if (!affinity) continue
    for (const key of ['requiredDuringSchedulingIgnoredDuringExecution','preferredDuringSchedulingIgnoredDuringExecution']) {
      const value=affinity[key], rules=kind==='nodeAffinity' && key.startsWith('required') ? value?.nodeSelectorTerms : value
      if (Array.isArray(rules) && !rules.length) {delete affinity[key];continue}
      for(const rule of rules || []) {
        const term=key.startsWith('preferred') ? rule.preference || rule.podAffinityTerm : rule
        for(const expression of term?.matchExpressions || term?.labelSelector?.matchExpressions || []) if(['Exists','DoesNotExist'].includes(expression.operator)) delete expression.values
      }
    }
    if(!Object.keys(affinity).length) delete template.spec.affinity[kind]
  }
  return r
}

export function validateWorkload(r) {
  if (!r.metadata.namespace || !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(r.metadata.name) || r.metadata.name.length > 63) return '请填写命名空间和有效的工作负载名称'
  const pod = podTemplate(r).spec
  if (!pod.containers?.length) return '至少需要一个业务容器'
  const names = new Set()
  for (const c of [...pod.containers, ...(pod.initContainers || [])]) {
    if (!/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(c.name) || c.name.length > 63 || names.has(c.name) || !c.image?.trim()) return '容器名称必须有效且唯一，并填写镜像'
    names.add(c.name)
    const envNames = new Set()
    for (const env of c.env || []) { if (!env.name || envNames.has(env.name)) return '环境变量名称不能为空或重复'; envNames.add(env.name) }
    for (const mount of c.volumeMounts || []) if (!pod.volumes?.some(v => v.name === mount.name) && !r.spec.volumeClaimTemplates?.some(v => v.metadata?.name === mount.name)) return '挂载引用了不存在的存储卷'
    for (const [key, probe] of Object.entries(c).filter(([key]) => key.endsWith('Probe'))) {
      if (['livenessProbe', 'readinessProbe', 'startupProbe'].includes(key) && !['httpGet','tcpSocket','exec','grpc'].some(type => probe[type])) return '请选择健康检查方式'
    }
  }
  if (r.kind === 'StatefulSet' && !r.spec.serviceName) return 'StatefulSet 必须填写服务名称'
  if (r.kind === 'CronJob' && !r.spec.schedule?.trim()) return '请填写定时任务表达式'
  return ''
}

export function updateImpact(r) {
  if (r.kind === 'Job') return 'Job 的 Pod 模板通常不可修改；请创建新的任务'
  if (r.kind === 'CronJob') return '配置修改影响后续任务，已运行的 Job 不会自动更新'
  if (r.spec.updateStrategy?.type === 'OnDelete') return 'OnDelete：保存模板后，需要手动删除旧 Pod 才会使用新配置'
  return '修改 Pod 模板将触发更新；实际更新进度取决于更新策略和就绪状态'
}

export function buildAccessResources(r, access) {
  const name = access.name || (r.kind === 'StatefulSet' ? r.spec.serviceName : '') || r.metadata.name, namespace = r.metadata.namespace
  const service = access.enabled ? { apiVersion:'v1', kind:'Service', metadata:{name, namespace}, spec:{ type:access.type, selector:clone(podTemplate(r).metadata.labels), ports:clone(access.ports), ...(r.kind === 'StatefulSet' ? {clusterIP:'None'} : {}) } } : null
  for (const port of service?.spec.ports || []) {
    if (/^\d+$/.test(String(port.targetPort))) port.targetPort = Number(port.targetPort)
    if (access.type !== 'NodePort') delete port.nodePort
  }
  const ingress = access.ingress ? {apiVersion:'networking.k8s.io/v1', kind:'Ingress', metadata:{name:`${name}-ingress`,namespace},spec:{...(access.className ? {ingressClassName:access.className}:{}),rules:clone(access.rules),...(access.tls?.length ? {tls:clone(access.tls)}:{})}} : null
  return {service, ingress}
}
