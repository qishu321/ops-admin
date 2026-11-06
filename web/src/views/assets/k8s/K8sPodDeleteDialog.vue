<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { tr } from '../../../utils/i18n'
import { deleteK8sResource, queryK8sPodDeletionStatus } from '../../../api/k8s'
import { confirmRiskOperation } from '../../../composables/useRiskConfirm'
const props = defineProps({pod:Object,cluster:Object})
const emit = defineEmits(['close','changed'])
const state=ref(null), error=ref(''), busy=ref(false), submitted=ref(false), useDefault=ref(true), grace=ref(30)
let timer, stopped=false, sequence=0
const uid=ref(props.pod.uid || '')
const done=computed(() => state.value?.exists===false)
const elapsed=computed(() => state.value?.deletionTimestamp ? Math.max(0,Math.floor((Date.now()-Date.parse(state.value.deletionTimestamp))/1000)) : 0)
const blocked=computed(() => (state.value?.finalizers || []).length>0)
const nodeReady=computed(() => state.value?.nodeConditions?.find(c => c.type==='Ready')?.status)
async function refresh() {
  const request=++sequence
  try {
    const data=await queryK8sPodDeletionStatus(props.cluster.id,props.pod.namespace,props.pod.name,uid.value)
    if(stopped || request!==sequence) return
    state.value=data; uid.value ||= data.uid; error.value=''
    if(data.exists===false) { if(submitted.value || props.pod.deletionTimestamp) emit('changed'); return }
  } catch(e) { if(!stopped) error.value=e.message || String(e) }
  if(!stopped) timer=setTimeout(refresh,3000)
}
async function remove(force=false) {
  if(!uid.value || done.value) return
  busy.value=true;error.value=''
  try {
    await confirmRiskOperation({operation:tr(force ? '强制删除 Pod' : '删除 Pod'),targetSummary:`${props.cluster.name} / ${props.pod.namespace} / ${props.pod.name} / UID ${uid.value}`,destructive:true,production:['prod','production','生产','线上'].includes(props.cluster.env),targetCount:1})
    if(stopped) return
    await deleteK8sResource({clusterId:props.cluster.id,resourceType:'pod',namespace:props.pod.namespace,name:props.pod.name,uid:uid.value,force,confirmed:true,...(!useDefault.value && !force ? {gracePeriodSeconds:grace.value}:{})})
    submitted.value=true;emit('changed');clearTimeout(timer);await refresh()
  } catch(e) { if(!['cancel','close'].includes(e)) error.value=e.message || String(e) } finally {busy.value=false}
}
onMounted(refresh)
onUnmounted(() => {stopped=true;sequence++;clearTimeout(timer)})
</script>
<template><el-dialog :model-value="true" :title="tr('Pod 删除与诊断')" width="820px" :close-on-click-modal="false" @close="emit('close')"><div v-loading="busy" class="pod-delete-content"><div class="target"><strong>{{ pod.name }}</strong><span>{{ cluster.name }} / {{ pod.namespace }}</span><small>UID {{ uid || '—' }}</small></div><el-alert v-if="done" :title="tr(state.replacementUID ? '原 Pod 已删除，同名的新 Pod 已由控制器创建' : '已确认原 Pod 对象不存在')" type="success" :closable="false" show-icon /><template v-else><el-alert :title="tr('删除 Pod 后，工作负载控制器可能自动补建；停止业务请调整工作负载副本数')" type="info" :closable="false" /><el-alert v-if="submitted" :title="tr('删除请求已提交，正在等待原 Pod 对象消失')" type="warning" :closable="false" /><el-descriptions :column="2" border><el-descriptions-item :label="tr('状态')">{{ state?.status || pod.status }}</el-descriptions-item><el-descriptions-item :label="tr('原始阶段')">{{ state?.phase || pod.phase }}</el-descriptions-item><el-descriptions-item :label="tr('节点')">{{ state?.node || pod.node }} · Ready {{ nodeReady || tr('未知') }}</el-descriptions-item><el-descriptions-item :label="tr('终止时长')">{{ elapsed }} s</el-descriptions-item></el-descriptions><el-alert v-if="nodeReady && nodeReady!=='True'" :title="tr('节点未就绪，需排查节点、kubelet 与容器运行时')" type="warning" :closable="false" /><el-alert v-if="elapsed > Math.max(120, (state?.terminationGracePeriodSeconds || 30) + 60)" :title="tr('终止时间较长，请结合事件、节点状态与清理钩子排查')" type="warning" :closable="false" /><el-alert v-if="blocked" :title="tr('存在 finalizer，需要相应控制器完成清理；强制删除不会自动移除 finalizer')" type="warning" :closable="false" /><p v-if="blocked"><code v-for="finalizer in state.finalizers" :key="finalizer">{{ finalizer }} </code></p><p v-if="state?.message">{{ state.message }}</p><p v-if="state?.nodeError">{{ tr('节点信息不可用') }}: {{ state.nodeError }}</p><el-table :data="[...(state?.initContainers || []),...(state?.containers || [])]" size="small"><el-table-column prop="name" :label="tr('容器')" /><el-table-column :label="tr('状态原因')"><template #default="{row}">{{ row.state?.waiting?.reason || row.state?.terminated?.reason || (row.ready ? 'Ready':'—') }}</template></el-table-column><el-table-column :label="tr('信息')"><template #default="{row}">{{ row.state?.waiting?.message || row.state?.terminated?.exitCode || '—' }}</template></el-table-column></el-table><el-collapse><el-collapse-item :title="tr('存储卷与退出钩子')"><pre>{{ JSON.stringify({terminationGracePeriodSeconds:state?.terminationGracePeriodSeconds,volumes:state?.volumes,lifecycle:state?.lifecycle},null,2) }}</pre></el-collapse-item><el-collapse-item :title="tr('最近事件')"><p v-if="state?.eventsError">{{ state.eventsError }}</p><el-table :data="state?.events || []" size="small"><el-table-column prop="lastTime" :label="tr('时间')" width="155" /><el-table-column prop="reason" :label="tr('原因')" width="150" /><el-table-column prop="message" :label="tr('信息')" /></el-table></el-collapse-item></el-collapse><div class="grace"><el-checkbox v-model="useDefault">{{ tr('使用 Pod 默认终止宽限时间') }}</el-checkbox><el-input-number v-if="!useDefault" v-model="grace" :min="1" :max="86400" /><span v-if="!useDefault">s</span></div><el-alert :title="tr('强制删除仅立即移除 API 对象，不保证旧进程已停止。StatefulSet 或共享存储业务可能出现重复实例与数据冲突，请先核实节点上的旧实例')" type="warning" :closable="false" /></template><el-alert v-if="error" :title="error" type="error" :closable="false" /></div><template #footer><el-button @click="emit('close')">{{ tr('关闭') }}</el-button><el-button :disabled="busy" @click="clearTimeout(timer);refresh()">{{ tr('刷新') }}</el-button><el-button v-if="!done" type="danger" plain :disabled="busy || !state || !uid" @click="remove(true)">{{ tr('强制删除') }}</el-button><el-button v-if="!done && !state?.deletionTimestamp && !submitted" type="danger" :disabled="busy || !state || !uid" @click="remove(false)">{{ tr('正常删除') }}</el-button></template></el-dialog></template>
<style scoped>.target{display:flex;flex-wrap:wrap;gap:8px 20px;align-items:center;margin-bottom:20px}.target strong{font-size:18px}.target span,.target small{color:#7e8b9c}.target small{width:100%;font-family:Consolas,monospace}.pod-delete-content>.el-alert,.pod-delete-content>.el-descriptions,.pod-delete-content>.el-table{margin:12px 0}.pod-delete-content pre{white-space:pre-wrap;max-height:250px;overflow:auto;font:12px/1.6 Consolas,monospace}.grace{display:flex;align-items:center;gap:12px;padding:14px 0}</style>
