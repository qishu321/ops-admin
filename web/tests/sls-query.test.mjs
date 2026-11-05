import test from 'node:test'
import assert from 'node:assert/strict'
import { appendSLSFilter, slsStatsKey, slsBody } from '../src/views/monitor/sls-query.js'
test('SLS filters quote values and preserve SQL including quoted pipes', () => {
 assert.equal(appendSLSFilter('*', 'level', 'ERROR'), 'level:"ERROR"')
 assert.equal(appendSLSFilter('service:api | select count(*) as count', 'level', 'INFO', true), '(service:api) AND NOT level:"INFO" | select count(*) as count')
 assert.equal(appendSLSFilter('message:"a|b"', 'level', 'x" OR *'), '(message:"a|b") AND level:"x\\" OR *"')
 assert.throws(() => appendSLSFilter('*','level\nunsafe','x'))
})
test('histogram cache separates sources, ranges, Logstores and predicates', () => {
 const range = {startAt:100,endAt:200}, key = slsStatsKey(1,'app','*',range)
 for (const args of [[2,'app','*',range],[1,'other','*',range],[1,'app','ERROR',range],[1,'app','*',{startAt:101,endAt:200}]]) assert.notEqual(slsStatsKey(...args),key)
})

test('real index fields survive empty query results; samples never become indexed', async () => {
 const {mergeSLSFields,slsFieldValue} = await import('../src/views/monitor/sls-query.js')
 const metadata=[{name:'*',kind:'fulltext'},{name:'actionType',type:'long',kind:'field',statistics:true}]
 assert.equal(mergeSLSFields(metadata,[])[0].name,'actionType')
 const fields=mergeSLSFields(metadata,[{source:{actionType:'14',extra:'sample'}}])
 assert.equal(fields.find(field=>field.name==='extra').indexed,false)
 assert.equal(fields.find(field=>field.name==='actionType').statistics,true)
 assert.equal(slsFieldValue({details:'{"code":14}'},'details.code'),14)
 assert.equal(appendSLSFilter('*','actionType','14',false,'long'),'actionType=14')
 assert.equal(appendSLSFilter('*','logId','9007199254740993',false,'long'),'logId=9007199254740993')
})

test('SLS reserved fields and levels are handled without unsafe interpolation', async () => {
 const { slsLogLevel, slsFieldValue } = await import('../src/views/monitor/sls-query.js')
 assert.equal(appendSLSFilter('*','__tag__:__hostname__','node-01'),'__tag__:__hostname__:"node-01"')
 assert.equal(appendSLSFilter('*','file name','a"b'),'"file name":"a\\"b"')
 assert.equal(slsFieldValue({_container_name_:'mgr'},'__tag__:_container_name_'),undefined)
 assert.equal(slsLogLevel({},'traceID=abc'),'-')
 assert.equal(slsLogLevel({},'I1103 10:28:10.232579 1 file.go:974] "traceID"="abc"'),'INFO')
 assert.equal(slsLogLevel({level:'error'},'traceID=abc'),'ERROR')
})

test('wrapped structured logs retain every field and large identifier; text logs stay intact', () => {
 const source = {actionType:'15', behId:'1099724259645358000', contentDetails:{status:'ok'}}
 const row = {source, message:JSON.stringify(source)}
 assert.equal(slsBody(row), row.message)
 assert.equal(slsBody(row,true), JSON.stringify(source,null,2))
 assert.equal(slsBody({source:{content:'first line\nsecond line'}},true),'first line\nsecond line')
})
