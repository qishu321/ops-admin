import test from 'node:test'
import assert from 'node:assert/strict'
import {exportRuleDocument,parseNativeRuleDocument,prepareImportedRule,collectRulePages,resolveRuleImportTarget} from '../src/views/monitor/alert-rule-transfer.js'
const source=[{id:4,name:'上海-Prometheus',type:'prometheus'}],notify=[{id:8,name:'值班通知'}]
const rule={id:99,name:'CPU',alertType:'metric',datasourceScope:'specific',datasourceIds:[4],promql:'rate(cpu[5m])',comparator:'>',threshold:85,forSeconds:180,evalIntervalSeconds:60,severity:'P1',labelsJson:'{"team":"ops"}',annotationsJson:'{}',status:1,notifyEnabled:true,notifyRuleId:8,notifyRepeatIntervalSeconds:3600,maxNotifyCount:4,notifyRecoveryEnabled:true,lastEvalStatus:'failed',logTimeRangeSeconds:300,env:'test'}
test('rule backup roundtrip preserves condition and notification parameters without source IDs or evaluation state',()=>{
 const doc=exportRuleDocument([rule],source,notify)
 assert.equal(doc.rules[0].id,undefined);assert.equal(doc.rules[0].lastEvalStatus,undefined);assert.equal(doc.rules[0].notifyRuleId,undefined)
 const parsed=parseNativeRuleDocument(JSON.stringify(doc))
 const ready=prepareImportedRule(parsed[0],[{...source[0],id:104}],[{...notify[0],id:108}],{keepStatus:true,keepNotifications:true})
 assert.deepEqual(ready.errors,[]);assert.equal(ready.payload.datasourceId,104);assert.equal(ready.payload.notifyRuleId,108)
 for(const key of ['threshold','forSeconds','evalIntervalSeconds','severity','labelsJson','notifyRepeatIntervalSeconds','maxNotifyCount','notifyRecoveryEnabled','env'])assert.equal(ready.payload[key],rule[key])
 assert.equal(ready.payload.status,1);assert.equal(ready.payload.query,rule.promql)
})
test('unmatched references cannot reuse numeric IDs; operator may explicitly map new sources',()=>{
 const imported={...rule,datasources:[{name:'missing',type:'prometheus'}],notifyRuleName:'missing'}
 const bad=prepareImportedRule(imported,source,notify,{keepNotifications:true});assert.ok(bad.errors.length>=2);assert.deepEqual(bad.payload.datasourceIds,[]);assert.equal(bad.payload.notifyRuleId,0)
 const mapped=prepareImportedRule(imported,source,notify,{datasourceIds:[4]});assert.deepEqual(mapped.errors,[]);assert.equal(mapped.payload.status,2);assert.equal(mapped.payload.notifyEnabled,false);assert.equal(mapped.payload.id,undefined)
})
test('global scope stays global; incompatible data source overrides are rejected',()=>{
 const global=prepareImportedRule({...rule,datasourceScope:'all'},source,notify)
 assert.equal(global.payload.datasourceScope,'all');assert.deepEqual(global.payload.datasourceIds,[])
 const incompatible=prepareImportedRule({...rule,alertType:'log'},source,notify,{datasourceIds:[4]});assert.ok(incompatible.errors.length)
})
test('native file rejects unsupported versions, malformed conditions and non-boolean switches',()=>{
 const doc=exportRuleDocument([rule],source,notify)
 assert.throws(()=>parseNativeRuleDocument(JSON.stringify({...doc,version:2})))
 assert.throws(()=>parseNativeRuleDocument(JSON.stringify({...doc,rules:[{...doc.rules[0],threshold:'85'}]})))
 assert.throws(()=>parseNativeRuleDocument(JSON.stringify({...doc,rules:[{...doc.rules[0],notifyEnabled:'false'}]})))
})
test('exporting the filtered set traverses all pages instead of only the visible page',async()=>{
 const calls=[]
 const rules=await collectRulePages(async params=>{calls.push(params);return {total:201,list:params.pageNum===1?Array.from({length:200},(_,i)=>({id:i})): [{id:200}]}},{keyword:'CPU',severity:'P1',pageNum:9,pageSize:20})
 assert.equal(rules.length,201);assert.deepEqual(calls.map(p=>p.pageNum),[1,2]);assert.ok(calls.every(p=>p.keyword==='CPU' && p.pageSize===200))
})

test('partial import retains only unfinished rules and retry does not recreate the successful rule',async()=>{
 const {computed,nextTick,ref}=await import('vue'), {default:vm}=await import('node:vm'), {readFileSync}=await import('node:fs')
 const setup=readFileSync(new URL('../src/views/monitor/AlertRuleTransfer.vue',import.meta.url),'utf8').match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm,'')
 const saved=[],events=[];let fail=true
 const ctx=vm.createContext({computed,nextTick,ref,defineProps:()=>({datasources:source,notifications:notify}),defineEmits:()=>name=>events.push(name),prepareImportedRule,collectRulePages,exportRuleDocument,parseNativeRuleDocument,resolveRuleImportTarget,ElMessageBox:{confirm:async()=>{}},queryMonitorAlertRuleList:async()=>({list:saved.map((name,id)=>({id,name})),total:saved.length}),saveMonitorAlertRule:async payload=>{if(payload.name==='second' && fail)throw Error('temporary failure');saved.push(payload.name)}})
 const view=vm.runInContext(`${setup};({rules,selected,submit,error,report,saving})`,ctx)
 view.rules.value=[{...rule,name:'first',datasources:[{name:source[0].name,type:'prometheus'}]},{...rule,name:'second',datasources:[{name:source[0].name,type:'prometheus'}]}];view.selected.value=[0,1]
 await view.submit();assert.deepEqual(saved,['first']);assert.deepEqual(Array.from(view.selected.value),[1]);assert.match(view.error.value,/second/)
 fail=false;await view.submit();assert.deepEqual(saved,['first','second']);assert.equal(view.selected.value.length,0);assert.equal(view.saving.value,false);assert.equal(events.length,2)
})
