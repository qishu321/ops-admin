<script setup>
import { computed, nextTick, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryMonitorAlertRuleList, monitorAlertRuleInfo, parsePrometheusAlertTemplates, saveMonitorAlertRule } from '../../api/monitor'
import { collectRulePages, exportRuleDocument, parseNativeRuleDocument, prepareImportedRule } from './alert-rule-transfer'
const props=defineProps({ selectedIds:Array, filters:Object, datasources:Array, notifications:Array })
const emit=defineEmits(['imported'])
const restoring=ref(false)
const exporting=ref(false), visible=ref(false), parsing=ref(false), saving=ref(false), content=ref(''), error=ref(''), stage=ref('input'), rules=ref([]), selected=ref([]), table=ref(null), datasourceIds=ref([]), strategy=ref('skip'), keepStatus=ref(false), keepNotifications=ref(false), report=ref('')
const settings=computed(()=>({datasourceIds:datasourceIds.value,keepStatus:keepStatus.value,keepNotifications:keepNotifications.value}))
const previews=computed(()=>rules.value.map((rule,index)=>({...rule,_index:index,...prepareImportedRule(rule,props.datasources,props.notifications,settings.value)})))
const selectedPreviews=computed(()=>previews.value.filter(row=>selected.value.includes(row._index)))
async function exportRules(scope) {
 if(exporting.value)return
 const ids=[...(props.selectedIds || [])]
 if(scope==='selected' && !ids.length)return ElMessage.warning('请先选择规则')
 exporting.value=true
 try {
  const data=scope==='selected'?await Promise.all(ids.map(id=>monitorAlertRuleInfo(id))):await collectRulePages(queryMonitorAlertRuleList,{...props.filters})
  if(!data.length)return ElMessage.warning('当前范围内没有可导出的规则')
  if(data.length>500)throw new Error('每个文件最多 500 条规则，请缩小筛选范围或分批选择导出')
  const documentData=exportRuleDocument(data,props.datasources,props.notifications)
  const blob=new Blob([JSON.stringify(documentData,null,2)],{type:'application/json;charset=utf-8'})
  if(blob.size>2*1024*1024)throw new Error('规则文件超过 2 MB，请分批选择导出')
  const url=URL.createObjectURL(blob)
  const link=document.createElement('a');link.href=url;link.download='ops-admin-alert-rules.json';document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),1000)
  ElMessage.success(`已导出 ${data.length} 条规则`)
 } catch(e){ElMessage.error(e.message || '导出失败')}finally{exporting.value=false}
}
function openImport(){content.value='';rules.value=[];selected.value=[];datasourceIds.value=[];strategy.value='skip';keepStatus.value=false;keepNotifications.value=false;error.value='';report.value='';stage.value='input';visible.value=true}
async function readFile(file){
 error.value=''
 if(file.size>2*1024*1024){error.value='文件不能超过 2 MB';return}
 content.value=await file.raw.text()
}
async function parse(){
 if(parsing.value)return
 if(!content.value.trim()){error.value='请上传文件或粘贴规则内容';return}
 if(new Blob([content.value]).size>2*1024*1024){error.value='内容不能超过 2 MB';return}
 parsing.value=true;error.value=''
 try{
  const text=content.value.trim().replace(/^\uFEFF/,'')
  if(text.startsWith('{'))rules.value=parseNativeRuleDocument(text)
  else {
   const rows=await parsePrometheusAlertTemplates(text)
   if(!rows?.length || rows.length>500)throw new Error('每次导入应包含 1 至 500 条告警规则')
   rules.value=rows.map(row=>({...row,promql:row.queryText,alertType:'metric',datasourceScope:'specific',datasources:[],notifyEnabled:false,status:2,notifyRecoveryEnabled:true,notifyRepeatIntervalSeconds:1800,maxNotifyCount:0}))
  }
  selected.value=rules.value.map((_,index)=>index);stage.value='preview';await nextTick();table.value?.toggleAllSelection()
 }catch(e){error.value=e.message || '文件解析失败'}finally{parsing.value=false}
}
async function submit(){
 const items=selectedPreviews.value
 if(saving.value || !items.length)return
 if(items.some(item=>item.errors.length)){error.value='请先处理已选规则的数据源或通知匹配问题';return}
 try{await ElMessageBox.confirm(`将新增导入 ${items.length} 条规则。${keepStatus.value?'按文件中的状态启用或禁用。':'全部先保存为禁用状态。'}${keepNotifications.value?'保留文件中的通知配置。':'导入后暂不开启通知。'}`,'确认导入',{confirmButtonText:'开始导入',cancelButtonText:'取消'})}catch{return}
 saving.value=true;error.value='';report.value=''
 let created=0,skipped=0;const finished=new Set()
 try{
  const existing=await collectRulePages(queryMonitorAlertRuleList)
  const names=new Set(existing.map(rule=>rule.name))
  for(const item of items){
   const payload={...item.payload};const base=payload.name
   if(names.has(base) && strategy.value==='skip'){skipped++;finished.add(item._index);continue}
   for(let n=2;names.has(payload.name);n++)payload.name=`${base}（导入 ${n}）`
   try{await saveMonitorAlertRule(payload)}catch(e){throw new Error(`“${payload.name}”导入失败：${e.message || '保存失败'}`)}
   names.add(payload.name);created++;finished.add(item._index)
  }
  report.value=`导入完成：新增 ${created} 条，跳过同名 ${skipped} 条。`
 }catch(e){error.value=`${e.message || '导入失败'}；已新增 ${created} 条，跳过 ${skipped} 条，未完成的规则已保留，可继续重试。`}
 finally{
  const remaining=selected.value.filter(index=>!finished.has(index));restoring.value=true
  selected.value=remaining;saving.value=false
  await nextTick();table.value?.clearSelection();for(const row of previews.value)if(remaining.includes(row._index))table.value?.toggleRowSelection(row,true)
  restoring.value=false
  if(created)emit('imported')
 }
}
</script>
<template>
 <div class="rule-transfer-actions"><el-button @click="openImport">导入规则</el-button><el-dropdown :disabled="exporting" @command="exportRules"><el-button :loading="exporting">导出规则 ▾</el-button><template #dropdown><el-dropdown-menu><el-dropdown-item command="selected" :disabled="!selectedIds?.length">导出选中 {{ selectedIds?.length || 0 }} 条</el-dropdown-item><el-dropdown-item command="filtered">导出当前筛选全部规则</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div>
 <el-dialog v-model="visible" append-to-body title="导入告警规则" width="min(1100px,96vw)" top="5vh" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving">
  <el-alert title="支持 Ops Admin 规则备份 JSON 和标准 Prometheus Rule YAML；每次最多 500 条、2 MB。" type="info" :closable="false" show-icon/>
  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon class="transfer-notice"/>
  <el-alert v-if="report" :title="report" type="success" :closable="false" show-icon class="transfer-notice"/>
  <div v-if="stage==='input'" class="transfer-input"><el-upload action="#" accept=".json,.yaml,.yml" :auto-upload="false" :show-file-list="false" :on-change="readFile"><el-button>选择规则文件</el-button></el-upload><el-input v-model="content" type="textarea" :rows="14" placeholder="上传文件，或在此粘贴规则 JSON / Prometheus YAML" aria-label="导入规则内容" spellcheck="false"/></div>
  <template v-else>
   <el-form label-position="top" class="transfer-settings" :disabled="saving"><el-form-item label="数据源（可重新指定，按规则类型匹配）"><el-select v-model="datasourceIds" multiple filterable clearable placeholder="留空按文件中的名称与类型匹配"><el-option v-for="ds in datasources" :key="ds.id" :label="`${ds.name} · ${ds.type}`" :value="ds.id"/></el-select></el-form-item><el-form-item label="同名规则处理"><el-select v-model="strategy"><el-option label="跳过同名规则" value="skip"/><el-option label="自动重命名后新增" value="rename"/></el-select></el-form-item><div class="transfer-toggles"><el-checkbox v-model="keepStatus">保留原启用状态</el-checkbox><el-checkbox v-model="keepNotifications">保留通知配置（按名称匹配）</el-checkbox></div></el-form>
   <p class="transfer-hint">默认导入为禁用状态且不开启通知。数据源 ID 不跨环境复用；未匹配项需在上方重新指定。</p>
   <el-table ref="table" :data="previews" row-key="_index" max-height="360" :disabled="saving" @selection-change="items=>{if(!saving && !restoring)selected=items.map(row=>row._index)}"><el-table-column type="selection" :selectable="()=>!saving" :reserve-selection="true" width="45"/><el-table-column prop="name" label="规则名称" min-width="180" show-overflow-tooltip/><el-table-column label="查询与条件" min-width="270"><template #default="{row}"><code class="transfer-query">{{row.promql}}</code><small>{{row.comparator}} {{row.threshold}} · 持续 {{row.forSeconds || 0}} 秒 · 每 {{row.evalIntervalSeconds || 60}} 秒评估</small></template></el-table-column><el-table-column prop="severity" label="等级" width="70"/><el-table-column label="导入检查" min-width="210"><template #default="{row}"><span v-if="row.errors.length" class="transfer-error">{{row.errors.join('；')}}</span><span v-else>{{row.payload.datasourceScope==='all'?'全部同类数据源':row.payload.datasourceIds.map(id=>datasources.find(ds=>ds.id===id)?.name).join('、')}}</span></template></el-table-column></el-table>
   <p class="transfer-hint">共 {{rules.length}} 条，已选 {{selected.length}} 条。成功导入和跳过的规则会从选择中移除，重试只处理剩余项。</p>
  </template>
  <template #footer><el-button :disabled="saving || parsing" @click="visible=false">关闭</el-button><el-button v-if="stage==='preview'" :disabled="saving" @click="stage='input';error='';report=''">返回修改</el-button><el-button v-if="stage==='input'" type="primary" :loading="parsing" @click="parse">解析并预览</el-button><el-button v-else type="primary" :loading="saving" :disabled="!selected.length || selectedPreviews.some(row=>row.errors.length)" @click="submit">导入 {{selected.length}} 条规则</el-button></template>
 </el-dialog>
</template>
<style scoped>
.rule-transfer-actions{display:inline-flex;gap:10px;align-items:center}.transfer-notice{margin-top:12px}.transfer-input{display:grid;gap:14px;margin-top:18px}.transfer-input :deep(textarea){font:12px/1.7 Consolas,monospace}.transfer-settings{display:grid;grid-template-columns:2fr 1fr;gap:0 18px;margin-top:18px}.transfer-settings :deep(.el-select){width:100%}.transfer-toggles{grid-column:1/-1;display:flex;gap:20px}.transfer-hint{font-size:12px;color:#8090a6;line-height:1.7}.transfer-query{display:block;font:12px/1.6 Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:400px}small{display:block;color:#8090a6;margin-top:6px}.transfer-error{color:var(--el-color-danger);font-size:12px}@media(max-width:700px){.transfer-settings{grid-template-columns:1fr}.transfer-toggles{flex-wrap:wrap;gap:4px}}
</style>
