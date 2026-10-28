import assert from 'node:assert/strict'
import test from 'node:test'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'
import { ref, computed, watch, nextTick } from 'vue'
import { escapeHtml, reducerLabel, formatReportTime } from '../src/views/monitor/dashboard/utils/format.js'

function load(name, overrides={}) {
  const source=readFileSync(new URL(`../src/views/monitor/dashboard/composables/${name}.js`,import.meta.url),'utf8').replace(/^import .*$/gm,'').replaceAll('export ','')
  return vm.runInNewContext(`${source};${name}`,{ref,computed,watch,ElMessage:{warning(){},success(){},error(){}},...overrides})
}
function report(ids=[1,2]) {
  return {run:{id:5,datasourceId:ids[0],datasourceName:`集群${ids[0]}`,datasourceIds:ids,datasourceNames:ids.map(id=>`集群${id}`),totalCount:ids.length*2,dangerCount:2},results:ids.flatMap(id=>[1,2].map(panelId=>({id:id*10+panelId,panelId,datasourceId:id,datasourceName:`集群${id}`,title:`规则${panelId}`,status:id===2?'danger':'healthy',valueText:String(id*10),promql:'vector(1)'})))}
}
function fixture(overrides={}) {
  const calls=[],errors=[]
  const input={panels:ref([{id:1,status:1,title:'规则1',datasourceId:1},{id:2,status:1,title:'规则2',datasourceId:1}]),activeDashboardId:ref(10),selectedDatasourceId:ref(1),datasourceOptions:ref([{id:1,name:'集群1'},{id:2,name:'集群2'}]),isListLayout:ref(true),panelForm:{inspectionKind:'context'},timeRangeOption:ref(3600),customTimeRange:ref([]),currentTimeBounds:()=>({startAt:100,endAt:200}),inspectionRangeLabel:()=> '测试窗口'}
  input.activePanels=computed(()=>input.panels.value.filter(panel=>panel.status===1))
  const inspection=load('useInspection',{
    runMonitorInspection:async(data,config)=>{calls.push({data,config});return report(Array.from(data.datasourceIds))},
    queryLatestMonitorInspectionRun:async()=>({run:null,results:[]}),
    ElMessage:{success(){},warning:message=>errors.push(message),error:message=>errors.push(message)},...overrides
  })(input)
  return {input,inspection,calls,errors}
}

test('multiple sources execute one run without panel-ID collisions and can be filtered independently',async()=>{
  const {inspection,calls}=fixture()
  inspection.selectedInspectionDatasourceIds.value=[1,2]
  await inspection.executeInspection()
  assert.deepEqual(Array.from(calls[0].data.datasourceIds),[1,2])
  assert.equal(calls[0].data.datasourceId,1)
  assert.ok(calls[0].config.timeout>=60000)
  const rows=inspection.inspectionRows.value
  assert.equal(rows.length,4)
  assert.equal(new Set(rows.map(row=>row.rowKey)).size,4)
  assert.ok(rows.every(row=>row.panelExists))
  assert.equal(rows[2].datasourceId,2)
  assert.equal(rows[2].panelDefinition.datasourceId,1,'editing a result must open the original rule definition')
  assert.equal(inspection.inspectionResultFor(rows[0]).valueText,'10')
  assert.equal(inspection.inspectionResultFor(rows[2]).valueText,'20')
  inspection.inspectionDatasourceFilter.value=2
  assert.equal(inspection.inspectionScopedRows.value.length,2)
  assert.equal(inspection.inspectionSummary.value.danger,2)
  assert.equal(inspection.inspectionSummary.value.healthy,0)
  inspection.inspectionFilter.value='healthy'
  assert.equal(inspection.inspectionPanels.value.length,0)
})

test('empty selection stays empty and cannot silently fall back to the first source',async()=>{
  const {inspection,input,calls}=fixture()
  inspection.selectedInspectionDatasourceIds.value=[]
  input.selectedDatasourceId.value=2
  await nextTick()
  await inspection.executeInspection()
  assert.equal(calls.length,0)
  assert.equal(inspection.selectedInspectionDatasourceIds.value.length,0)
})

test('single-source history loads compatibly and restores its selected datasource',async()=>{
  const {inspection}=fixture({queryLatestMonitorInspectionRun:async()=>({run:{id:7,datasourceId:2,datasourceName:'历史集群'},results:[{panelId:1,title:'旧规则',status:'healthy',valueText:'7'}]})})
  await inspection.loadLatestInspectionReport()
  assert.deepEqual(Array.from(inspection.selectedInspectionDatasourceIds.value),[2])
  assert.equal(inspection.inspectionRows.value[0].datasourceName,'历史集群')
  assert.equal(inspection.inspectionReportDatasources.value[0].id,2)
})

test('late runs cannot overwrite another scheme and repeat clicks do not submit twice',async()=>{
  let finish,count=0
  const {inspection,input}=fixture({runMonitorInspection:()=>{count++;return new Promise(resolve=>{finish=resolve})}})
  inspection.selectedInspectionDatasourceIds.value=[1,2]
  const pending=inspection.executeInspection()
  await inspection.executeInspection()
  assert.equal(count,1)
  input.activeDashboardId.value=20
  finish(report())
  await pending
  assert.equal(inspection.inspectionReport.value,null)
  assert.equal(inspection.inspectionRunning.value,false)
})

test('an older backend cannot present a single-source response as a successful multi-source report',async()=>{
  const {inspection,errors}=fixture({runMonitorInspection:async()=>({run:{id:9,datasourceId:1},results:[]})})
  inspection.selectedInspectionDatasourceIds.value=[1,2]
  await inspection.executeInspection()
  assert.equal(inspection.inspectionReport.value,null)
  assert.match(errors[0],/后端已更新/)
})

test('PDF includes every source snapshot, escapes names, and exports all results regardless of the view filter',()=>{
  let html=''
  const exporter=load('useInspectionExport',{escapeHtml,reducerLabel,formatReportTime,setTimeout:fn=>fn(),window:{open:()=>({document:{write:text=>{html=text},close(){}},focus(){},print(){}})}})
  const data=report()
  data.run.datasourceNames=['集群1','<script>源2</script>']
  data.results[2].datasourceName='<script>源2</script>'
  exporter({inspectionReport:ref(data.run),inspectionReportResults:ref(data.results)}).exportInspectionReportPdf()
  assert.match(html,/<th>数据源<\/th>/)
  assert.match(html,/&lt;script&gt;源2&lt;\/script&gt;/)
  assert.equal((html.match(/vector\(1\)/g)||[]).length,4)
  assert.ok(!html.includes('<script>源2'))
})

test('presentation refresh preserves the saved report on failure while a scheme load still clears it',async()=>{
  let reads=0
  const {inspection,input}=fixture({queryLatestMonitorInspectionRun:async()=>{
    if (++reads>1) throw new Error('报告读取失败')
    return report()
  }})
  await inspection.loadLatestInspectionReport()
  inspection.inspectionDatasourceFilter.value=2
  await assert.rejects(inspection.loadLatestInspectionReport(undefined,{preserveCurrent:true}),/报告读取失败/)
  assert.equal(inspection.inspectionReport.value.id,5)
  assert.equal(inspection.inspectionReportResults.value.length,4)
  assert.equal(inspection.inspectionDatasourceFilter.value,2)
  input.activeDashboardId.value=20
  await assert.rejects(inspection.loadLatestInspectionReport(),/报告读取失败/)
  assert.equal(inspection.inspectionReport.value,null,'a different scheme must not inherit the last report')
  assert.equal(inspection.inspectionReportResults.value.length,0)
})
