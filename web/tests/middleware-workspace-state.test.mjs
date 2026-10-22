import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import {computed,ref,reactive} from 'vue'
import * as rabbit from '../src/views/assets/rabbitmq-view.js'
import * as nacos from '../src/views/assets/nacos-view.js'
function page(file, exports, helpers) {
 const code=readFileSync(new URL(`../src/views/assets/${file}.vue`,import.meta.url),'utf8').match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[\s\S]*?from ['"][^'"\n]+['"]\r?$/gm,'')
 const requests=[]
 const ctx=vm.createContext({...helpers,computed,ref,reactive,defineProps:()=>({sourceId:1}),useRoute:()=>({query:{}}),useRouter:()=>({}),onMounted:()=>{},onUnmounted:()=>{},getUser:()=>({accountType:'administrator'}),getPermissions:()=>[],operateMiddleware:(...args)=>new Promise(resolve=>requests.push({args,resolve})),ElMessage:{error(){}},ElMessageBox:{}})
 return {view:vm.runInContext(`${code};({${exports}})`,ctx),requests}
}
test('RabbitMQ switching virtual host closes detail and ignores pending queue response', async()=>{
 const {view,requests}=page('RabbitMqExplorer','openQueue,selectVhost,drawer,queue,detailLoading',rabbit)
 const old=view.openQueue({name:'same-name',vhost:'/old'})
 view.selectVhost('/new')
 requests[0].resolve({name:'stale'});requests[1].resolve([]);await old
 assert.equal(view.drawer.value,false);assert.equal(view.detailLoading.value,false);assert.equal(view.queue.value.name,'same-name')
})
test('Nacos switching namespace invalidates old configuration content',async()=>{
 const {view,requests}=page('NacosExplorer','openConfig,selectNamespace,configDrawer,configContent,configDetailLoading,namespaceId',nacos)
 const old=view.openConfig({dataId:'app.yaml',group:'DEFAULT_GROUP'})
 view.selectNamespace('new-space')
 requests[0].resolve('old-secret');await old
 assert.equal(view.namespaceId.value,'new-space');assert.equal(view.configDrawer.value,false);assert.equal(view.configContent.value,'');assert.equal(view.configDetailLoading.value,false)
})
