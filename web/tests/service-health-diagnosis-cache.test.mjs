import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { computed, nextTick, onActivated, onDeactivated, onBeforeUnmount, onMounted, ref, createRenderer, defineComponent, h, KeepAlive } from 'vue'
const source = readFileSync(new URL('../src/views/assets/ServiceHealthDiagnosis.vue',import.meta.url),'utf8')
const setup = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm,'')
const renderer = createRenderer({
 createElement:()=>({children:[]}),createText:()=>({}),createComment:()=>({}),setText(){},setElementText(){},patchProp(){},
 insert(node,parent){parent.children ||= [];parent.children.push(node);node.parent=parent},remove(node){if(node.parent)node.parent.children=node.parent.children.filter(item=>item!==node)},parentNode:node=>node.parent,nextSibling:()=>null
})
function mountPage() {
 const events=new EventTarget(), calls=[], requests=[];let state
 const ctx=vm.createContext({ computed,nextTick,onActivated,onDeactivated,onBeforeUnmount,onMounted,ref,window:events,ElMessage:{warning(){}},requestAnimationFrame:fn=>fn(),setTimeout:fn=>fn(),queryAssetServiceList:async()=>{calls.push('services');return {list:[{id:1,name:'app'}]}},queryAssetServiceDiagnosisRun:()=>new Promise(resolve=>requests.push(resolve)) })
 const Diagnosis=defineComponent({setup(){state=vm.runInContext(`${setup};({target,activeTab,environment,diagnostic,flameResults,runDiagnostic})`,ctx);return ()=>h('div')}})
 const visible=ref(true)
 const app=renderer.createApp({setup:()=>()=>h(KeepAlive,null,{default:()=>visible.value?h(Diagnosis,{key:'/containers/services/health-diagnosis'}):h('span')})})
 app.mount({children:[]})
 return {state,events,calls,requests,visible,app}
}
test('diagnosis route opts into the layout cache',()=>{
 const router=readFileSync(new URL('../src/router/index.js',import.meta.url),'utf8')
 assert.match(router,/path: '\/containers\/services\/health-diagnosis'.*keepAlive: true/)
})
test('switching top tabs retains diagnosis target, result and selected panel without rerunning',async()=>{
 const view=mountPage();await nextTick()
 view.state.target.value={serviceId:1,workloadKey:'Deployment:api',podName:'api-1',container:'app',pid:'1'}
 view.state.environment.value={ready:true};view.state.diagnostic.value={dashboard:{heapUsed:'69M'}};view.state.activeTab.value='memory'
 view.visible.value=false;await nextTick();view.visible.value=true;await nextTick()
 assert.equal(view.state.target.value.podName,'api-1');assert.equal(view.state.activeTab.value,'memory');assert.equal(view.state.environment.value.ready,true);assert.equal(view.state.diagnostic.value.dashboard.heapUsed,'69M');assert.equal(view.calls.length,1);assert.equal(view.requests.length,0)
 view.app.unmount()
})
test('closing diagnosis clears cached state and ignores an in-flight result',async()=>{
 const view=mountPage();await nextTick();view.state.target.value.pid='1';view.state.environment.value={ready:true}
 const pending=view.state.runDiagnostic('dashboard')
 const event=new Event('ops-admin:tab-closed');event.detail={path:'/containers/services/health-diagnosis'};view.events.dispatchEvent(event)
 view.requests[0]({data:{dashboard:{heapUsed:'stale'}}});await pending
 assert.equal(view.state.diagnostic.value,null);assert.equal(view.state.target.value.pid,'');assert.equal(view.state.environment.value,null)
 view.visible.value=false;await nextTick();view.visible.value=true;await nextTick();assert.equal(view.calls.length,2)
 view.app.unmount()
})
