import test from 'node:test'
import assert from 'node:assert/strict'
import { newWorkload, newContainer, podTemplate, prepareWorkload, workloadManifest, validateWorkload, buildAccessResources, updateImpact } from '../src/utils/k8s-workload-editor.js'
import { setLocale, tr } from '../src/utils/i18n.js'
import translations from '../src/locales/k8s-editor-en.js'

test('all five workload kinds produce correct Pod template paths and restart policies', () => {
  for (const kind of ['Deployment','StatefulSet','DaemonSet','Job','CronJob']) {
    const draft=newWorkload(kind,'dev'); draft.metadata.name='example'; podTemplate(draft).spec.containers[0].image='example:v1'
    if(kind==='StatefulSet') draft.spec.serviceName='example'
    const resource=workloadManifest(draft)
    assert.equal(validateWorkload(resource),'')
    assert.equal(podTemplate(resource).metadata.labels['app.kubernetes.io/name'],'example')
    assert.equal(['Job','CronJob'].includes(kind),podTemplate(resource).spec.restartPolicy==='Never')
    if(resource.spec.selector) assert.deepEqual(resource.spec.selector.matchLabels,podTemplate(resource).metadata.labels)
    if(kind==='CronJob') assert.equal(resource.spec.jobTemplate.spec.template.spec.containers.length,1)
  }
})

test('editing one image preserves unknown fields, references, security, volumes, and optimistic version', () => {
  const resource=newWorkload('StatefulSet','dev'); resource.metadata={name:'orders',namespace:'dev',uid:'original',resourceVersion:'42',managedFields:[{}]};resource.spec.serviceName='orders';resource.spec.updateStrategy={type:'OnDelete'}
  const pod=podTemplate(resource).spec
  pod.containers[0]={...newContainer(),image:'v1',env:[{name:'TOKEN',valueFrom:{secretKeyRef:{name:'token',key:'value'}}}],envFrom:[{configMapRef:{name:'settings'}}],securityContext:{capabilities:{drop:['ALL']}},startupProbe:{exec:{command:['/app/check']},failureThreshold:20},volumeMounts:[{name:'config',mountPath:'/config',subPath:'app.yaml'}]}
  pod.volumes=[{name:'config',configMap:{name:'settings',items:[{key:'app',path:'app.yaml'}]}}];pod.affinity={podAntiAffinity:{requiredDuringSchedulingIgnoredDuringExecution:[{labelSelector:{matchLabels:{app:'orders'}},topologyKey:'zone'}]}};pod.dnsConfig={nameservers:['192.0.2.1']};resource.status={readyReplicas:1}
  const draft=prepareWorkload(resource);podTemplate(draft).spec.containers[0].image='v2'
  const result=workloadManifest(draft,true)
  assert.equal(result.metadata.resourceVersion,'42');assert.equal(result.metadata.uid,'original');assert.equal(result.status,undefined)
  assert.equal(podTemplate(resource).spec.containers[0].image,'v1')
  result.spec.template.spec.containers[0].image='v1'
  const expected=structuredClone(resource);delete expected.status;delete expected.metadata.managedFields;delete expected.spec.template.spec.containers[0].resources
  assert.deepEqual(result,expected)
  assert.match(updateImpact(result),/OnDelete/)
})

test('numeric probe ports and rolling counts remain integers while named ports and percentages remain strings', () => {
  const draft=newWorkload();draft.metadata.name='app';draft.metadata.namespace='dev';draft.spec.strategy={type:'RollingUpdate',rollingUpdate:{maxSurge:'1',maxUnavailable:'25%'}}
  const c=podTemplate(draft).spec.containers[0];c.image='v1';c.livenessProbe={httpGet:{port:'8080',path:'/'}};c.readinessProbe={tcpSocket:{port:'http'}}
  const result=workloadManifest(draft)
  assert.equal(podTemplate(result).spec.containers[0].livenessProbe.httpGet.port,8080)
  assert.equal(podTemplate(result).spec.containers[0].readinessProbe.tcpSocket.port,'http')
  assert.deepEqual(result.spec.strategy.rollingUpdate,{maxSurge:1,maxUnavailable:'25%'})
  const {service}=buildAccessResources(result,{enabled:true,type:'ClusterIP',ports:[{port:80,targetPort:'8080',nodePort:32001},{port:90,targetPort:'http'}]})
  assert.equal(service.spec.ports[0].targetPort,8080);assert.equal(service.spec.ports[0].nodePort,undefined);assert.equal(service.spec.ports[1].targetPort,'http')
})

test('invalid references and duplicate names fail before any request', () => {
  const draft=newWorkload();draft.metadata.name='app';draft.metadata.namespace='dev';const c=podTemplate(draft).spec.containers[0];c.image='v1';c.volumeMounts=[{name:'missing',mountPath:'/data'}]
  assert.match(validateWorkload(draft),/存储卷/)
  delete c.volumeMounts;c.env=[{name:'A',value:'1'},{name:'A',value:'2'}];assert.match(validateWorkload(draft),/环境变量/)
  delete c.env;podTemplate(draft).spec.initContainers=[{...newContainer('app'),image:'v1'}];assert.match(validateWorkload(draft),/唯一/)
})

test('new editor translations switch reactively without changing resource text', () => {
  setLocale('en-US')
  for(const [source,english] of Object.entries(translations)){assert.equal(tr(source),english);assert.doesNotMatch(english,/\p{Script=Han}/u)}
  assert.equal(tr('用户的资源名称'),'用户的资源名称')
  setLocale('zh-CN');assert.equal(tr('健康检查'),'健康检查')
})
