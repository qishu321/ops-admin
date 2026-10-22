import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import {exportRuleDocument,parseNativeRuleDocument,prepareImportedRule,resolveRuleImportTarget} from '../src/views/monitor/alert-rule-transfer.js'

const doc=JSON.parse(readFileSync(new URL('../../docs/alert-rules/ops-admin-alert-rules.optimized.json',import.meta.url),'utf8'))
const sources=[{id:1,name:'上海-Prometheus',type:'prometheus'}],notifications=[{id:2,name:'企微告警'}]

test('all 62 real revised rules can be parsed, mapped and re-exported without dropping recovery policy',()=>{
 const rows=parseNativeRuleDocument(JSON.stringify(doc))
 assert.equal(rows.length,62)
 const payloads=rows.map(rule=>{
  const result=prepareImportedRule(rule,sources,notifications,{keepStatus:true,keepNotifications:true})
  assert.deepEqual(result.errors,[])
  return result.payload
 })
 const roundtrip=parseNativeRuleDocument(JSON.stringify(exportRuleDocument(payloads,sources,notifications)))
 for(let i=0;i<rows.length;i++)for(const key of ['recoveryQuery','recoveryComparator','recoveryThreshold','recoveryForSeconds','missingDataPolicy','identityLabels','healthQuery','discoveryWindowSeconds','status'])assert.deepEqual(roundtrip[i][key],rows[i][key],`${i+1}: ${key}`)
 const ready=roundtrip[2]
 assert.equal(ready.missingDataPolicy,'alert');assert.equal(ready.discoveryWindowSeconds,3600);assert.equal(ready.recoveryThreshold,1)
 assert.equal(roundtrip.filter(rule=>rule.status===2).length,10)
})

test('update import preserves an existing ID and rejects ambiguous names',()=>{
 const payload={name:'NotReady',threshold:0}
 assert.deepEqual(resolveRuleImportTarget(payload,[{id:7,name:'NotReady'}],'update'),{...payload,id:7})
 assert.equal(resolveRuleImportTarget(payload,[{id:7,name:'NotReady'}],'skip'),null)
 assert.throws(()=>resolveRuleImportTarget(payload,[{id:7,name:'NotReady'},{id:8,name:'NotReady'}],'update'),/同名/)
 assert.equal(resolveRuleImportTarget(payload,[{id:7,name:'NotReady'}],'rename').name,'NotReady（导入 2）')
})

test('unsafe missing policy and unsupported features fail before any saves',()=>{
 const copy=structuredClone(doc);copy.rules[2].healthQuery=''
 assert.throws(()=>parseNativeRuleDocument(JSON.stringify(copy)),/缺失告警/)
 copy.requiresFeatures=['future-policy']
 assert.throws(()=>parseNativeRuleDocument(JSON.stringify(copy)),/不支持/)
})
