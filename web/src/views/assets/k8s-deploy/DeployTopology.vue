<script setup>
import { computed } from 'vue'
import { Connection, Cpu } from '@element-plus/icons-vue'
import { deploymentSummary } from '../../../utils/k8s-deploy'
const props = defineProps({ plan: { type: Object, required: true } })
const summary = computed(() => deploymentSummary(props.plan))
const controlPlanes = computed(() => (props.plan.nodes || []).filter(n => n.role === 'master'))
const workers = computed(() => (props.plan.nodes || []).filter(n => n.role === 'worker'))
</script>

<template>
  <div class="deploy-topology">
    <div class="deploy-topology-heading"><span>集群预览</span><span class="deploy-topology-version">v{{ plan.config?.version || '—' }}</span></div>
    <div class="deploy-topology-canvas" aria-label="集群节点构成预览">
      <div class="deploy-topology-api"><el-icon><Connection /></el-icon><span>API Server</span></div>
      <div class="deploy-topology-stem"></div>
      <div class="deploy-topology-row">
        <div v-for="node in controlPlanes.slice(0, 3)" :key="node.hostId" class="deploy-topology-node control" :title="node.name || node.hostName"><el-icon><Cpu /></el-icon><span>{{ node.name || node.hostName }}</span></div>
        <div v-if="!controlPlanes.length" class="deploy-topology-node ghost"><el-icon><Cpu /></el-icon><span>控制平面</span></div>
      </div>
      <div class="deploy-topology-stem"></div>
      <div class="deploy-topology-row">
        <div v-for="node in workers.slice(0, 3)" :key="node.hostId" class="deploy-topology-node worker" :title="node.name || node.hostName"><span class="deploy-node-light"></span><span>{{ node.name || node.hostName }}</span></div>
        <div v-if="!workers.length" class="deploy-topology-node ghost"><span class="deploy-node-light"></span><span>工作节点</span></div>
      </div>
    </div>
    <div class="deploy-topology-counts"><span><i></i>控制平面 <strong>{{ summary.masters }}</strong></span><span><i></i>工作节点 <strong>{{ summary.workers }}</strong></span></div>
    <div class="deploy-topology-meta"><span>网络插件</span><strong>{{ ({ calico: 'Calico', flannel: 'Flannel', cilium: 'Cilium' })[plan.config?.networkPlugin] || '—' }}</strong></div>
    <div class="deploy-topology-meta"><span>部署架构</span><strong>{{ plan.config?.ha ? '高可用' : '单控制平面' }}</strong></div>
  </div>
</template>
