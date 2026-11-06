<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import { queryAssetServiceWorkloadLogs, queryAssetServiceWorkloadRuntime } from '../../api/asset'
import { usePodLogSearch } from './k8s/usePodLogSearch'

const props = defineProps({ serviceId: { type: Number, default: 0 }, workloadType: String, workloadName: String, podName: String, inline: Boolean })
const route = useRoute()
const loading = ref(false)
const logLoading = ref(false)
const runtime = ref({})
const logs = ref('')
const { keyword, searching, matchingLines } = usePodLogSearch(() => logs.value)
const displayedLogs = computed(() => {
  if (!searching.value) return logs.value || (selectedPod.value ? '暂无日志' : '请选择 Pod 查看日志')
  return matchingLines.value.length
    ? matchingLines.value.map(({ text }) => text).join('\n')
    : '没有匹配的日志'
})
const logLines = ref(200)
const selectedPod = ref(props.podName || route.query.podName || '')
const logCountLabel = (count) => `\u6700\u8fd1 ${count} \u6761`
const params = computed(() => ({ serviceId: props.serviceId || Number(route.query.serviceId), workloadType: props.workloadType || route.query.workloadType, workloadName: props.workloadName || route.query.workloadName }))
const hasWorkloadContext = computed(() => Boolean(params.value.serviceId && params.value.workloadType && params.value.workloadName))
async function loadRuntime() { if (!hasWorkloadContext.value) { runtime.value = {}; logs.value = ''; return }; loading.value = true; try { runtime.value = await queryAssetServiceWorkloadRuntime(params.value); if (!selectedPod.value) selectedPod.value = runtime.value?.pods?.[0]?.name || ''; if (selectedPod.value) await loadLogs() } finally { loading.value = false } }
async function loadLogs() { if (!selectedPod.value) return; logLoading.value = true; try { const data = await queryAssetServiceWorkloadLogs({ ...params.value, podName: selectedPod.value, tailLines: logLines.value }); logs.value = data.content || '' } finally { logLoading.value = false } }
onMounted(loadRuntime)
</script>

<template>
  <div class="logs-page" v-loading="loading">
    <el-empty v-if="!hasWorkloadContext" description="请选择服务和工作负载后查看 Pod 日志" />
    <section v-else class="logs-card">
      <header>
        <div><h2>Pod &#26085;&#24535;</h2><p>&#40664;&#35748;&#26368;&#36817; 200 &#26465;&#65292;&#26368;&#22810;&#21487;&#35835;&#21462;&#26368;&#36817; 1000 &#26465;&#12290;</p></div>
        <div class="controls"><el-select v-model="selectedPod" style="width:300px" placeholder="Pod" @change="loadLogs"><el-option v-for="pod in runtime.pods || []" :key="pod.name" :label="pod.name" :value="pod.name"/></el-select><el-select v-model="logLines" style="width:142px" @change="loadLogs"><el-option v-for="item in [100, 200, 500, 800, 1000]" :key="item" :label="logCountLabel(item)" :value="item"/></el-select><el-button :icon="Refresh" @click="loadLogs">&#21047;&#26032;&#26085;&#24535;</el-button></div>
      </header>
      <div class="log-search">
        <el-input v-model="keyword" clearable aria-label="搜索已加载日志" placeholder="搜索已加载日志" />
        <span v-if="searching" role="status">命中 {{ matchingLines.length }} 行</span>
      </div>
      <pre v-loading="logLoading">{{ displayedLogs }}</pre>
    </section>
  </div>
</template>

<style scoped>
.log-search { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 0 22px 12px; background: #f8faff; color: #7485a0; font-size: 13px; }
.log-search .el-input { width: 280px; max-width: 100%; }
.logs-page { min-height: 100%; padding: 18px 22px 22px; background: #f4f7fc; }.logs-card { overflow: hidden; border: 1px solid #e0e8f5; border-radius: 16px; background: #fff; }.logs-card header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 22px; background: #f8faff; }.logs-card h2 { margin: 0; color: #142e57; font-size: 19px; }.logs-card p { margin: 5px 0 0; color: #7485a0; font-size: 13px; }.controls { display: flex; align-items: center; gap: 10px; }.logs-card pre { min-height: 600px; max-height: calc(100vh - 220px); margin: 0; overflow: auto; padding: 20px; background: #101c33; color: #dbe8ff; white-space: pre-wrap; word-break: break-all; font: 13px/1.7 Consolas, Monaco, monospace; }@media(max-width:800px){.logs-card header{align-items:flex-start;flex-direction:column}.controls{width:100%;flex-wrap:wrap}}
</style>
