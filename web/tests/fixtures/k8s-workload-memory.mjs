// Browser acceptance fixture only. Requests never leave this in-memory server.
import { newWorkload, podTemplate, validateWorkload } from '../../src/utils/k8s-workload-editor.js'
export function createK8sWorkloadMemory() {
  const workload=newWorkload('Deployment','demo');workload.metadata={name:'orders',namespace:'demo',uid:'workload-old',resourceVersion:'42'}
  workload.spec.selector.matchLabels={app:'orders'};podTemplate(workload).metadata.labels={app:'orders'};podTemplate(workload).spec.containers[0].image='nginx:1.27'
  podTemplate(workload).spec.containers[0].env=[{name:'TOKEN',valueFrom:{secretKeyRef:{name:'app-secret',key:'token'}}}]
  podTemplate(workload).spec.dnsConfig={nameservers:['192.0.2.10']}
  podTemplate(workload).spec.volumes=[{name:'settings',configMap:{name:'app-config'}}]
  const saved=new Map([['orders',workload]]), writes=[], validations=[], deleted=new Set(), requested=new Set()
  const pod={name:'orders-old',namespace:'demo',uid:'pod-old',workloadName:'orders',workloadType:'Deployment',status:'Terminating',phase:'Running',readyContainers:0,totalContainers:1,node:'test-cp-01',nodeIP:'192.0.2.10',age:'35m',ip:'192.0.2.100',restarts:0,deletionTimestamp:new Date(Date.now()-180000).toISOString()}
  function handle(req,url,body,respond,res) {
    const path=url.pathname
    if(path==='/api/v1/k8s/cluster/detail' && url.searchParams.get('clusterId')==='901') {
      respond(res,{cluster:{id:901,name:'研发测试集群 · 模拟',version:'v1.34.9',apiServer:'https://192.0.2.1:6443',nodeCount:1,env:'test'},overview:{},namespaces:[{name:'demo',status:'Active'}],pods:deleted.has('pod-old')?[]:[pod],workloads:[...saved.values()].map(r=>({name:r.metadata.name,namespace:'demo',type:r.kind,ready:'1/1',updated:1,available:1,age:'1h'})),nodes:[],network:{services:[],ingresses:[]},configStorage:{configMaps:[{name:'app-config',namespace:'demo'}],secrets:[{name:'app-secret',namespace:'demo',type:'Opaque'}],storage:[{name:'app-data',namespace:'demo',kind:'PVC'}]}});return true
    }
    if(path==='/api/v1/k8s/workload/detail') {const resource=saved.get(url.searchParams.get('workloadName')) || workload;respond(res,{name:resource.metadata.name,namespace:'demo',type:resource.kind,ready:'1/1',resource,containers:podTemplate(resource).spec.containers,yaml:JSON.stringify(resource)});return true}
    if(path==='/api/v1/k8s/pod/deletion-status') {respond(res,deleted.has('pod-old')?{exists:false,replacementUID:'pod-new'}:{exists:true,uid:'pod-old',status:'Terminating',phase:'Running',deletionTimestamp:pod.deletionTimestamp,node:'test-cp-01',nodeConditions:[{type:'Ready',status:'False'}],containers:[{name:'app',ready:false,state:{waiting:{reason:'ContainerStatusUnknown'}}}],events:[{lastTime:'2026-10-30 10:00:00',reason:'FailedKillPod',message:'Acceptance fixture: runtime timeout'}],terminationGracePeriodSeconds:30,volumes:podTemplate(workload).spec.volumes,finalizers:[]});return true}
    if(path==='/api/v1/k8s/resource/delete') {if(body.uid!=='pod-old')respond(res,null,409,'UID conflict');else{writes.push(body);requested.add(body.uid);if(body.force)deleted.add(body.uid);respond(res,{accepted:true,deleted:false})}return true}
    if(path==='/api/v1/k8s/workload/create') {
      const r=JSON.parse(body.workloadYaml),error=validateWorkload(r)
      if(error){respond(res,null,400,error);return true}
      const result={complete:true}
      for(const [key,content,skip] of [['workload',body.workloadYaml,body.skipWorkload],['service',body.serviceYaml,body.skipService],['ingress',body.ingressYaml,false]]){
        if(!content)continue
        const resource=JSON.parse(content)
        if(!body.dryRun && !skip) {writes.push({key,resource});if(key==='workload')saved.set(resource.metadata.name,{...resource,metadata:{...resource.metadata,resourceVersion:'43',uid:'new-workload'}})}
        else if(body.dryRun)validations.push(resource)
        result[key]={status:skip?'reused':body.dryRun?'validated':'created',name:resource.metadata.name}
      }
      respond(res,result);return true
    }
    if(path==='/api/v1/k8s/resource/yaml') {
      const r=JSON.parse(body.yaml),current=saved.get(r.metadata.name)
      if(r.metadata.resourceVersion!==current?.metadata.resourceVersion){respond(res,null,409,'Resource version conflict');return true}
      if(body.dryRun)validations.push(r);else{saved.set(r.metadata.name,r);writes.push({key:'update',resource:r})}
      respond(res,{updated:!body.dryRun,validated:body.dryRun});return true
    }
    return false
  }
  return {handle,inspect:()=>({writes,validations,resources:[...saved.values()],externalRequests:0})}
}
