<script setup>
import { computed } from 'vue'
import { deploymentSummary, roleText } from '../../../utils/k8s-deploy'
const props = defineProps({ plan: { type: Object, required: true }, editable: Boolean })
const summary = computed(() => deploymentSummary(props.plan))
const c = computed(() => props.plan.config || {})
const emit = defineEmits(['edit'])
</script>

<template>
  <div class="deploy-summary">
    <div class="deploy-toolbar"><h2>集群信息</h2><el-button v-if="editable" link type="primary" @click="emit('edit', 0)">修改</el-button></div>
    <el-descriptions :column="2" border>
      <el-descriptions-item label="部署名称">{{ plan.name || '—' }}</el-descriptions-item>
      <el-descriptions-item label="Kubernetes">{{ c.version }}</el-descriptions-item>
      <el-descriptions-item label="运行时">containerd · {{ c.runtimeDirectory || '默认数据目录' }}</el-descriptions-item>
      <el-descriptions-item label="网络插件">{{ c.networkPlugin }}</el-descriptions-item>
      <el-descriptions-item label="Pod CIDR"><span class="deploy-mono">{{ c.podCIDR }}</span></el-descriptions-item>
      <el-descriptions-item label="Service CIDR"><span class="deploy-mono">{{ c.serviceCIDR }}</span></el-descriptions-item>
      <el-descriptions-item label="节点命名">{{ c.autoHostname ? '自动设置主机名' : '保留主机名' }}</el-descriptions-item>
      <el-descriptions-item label="关闭 SELinux">{{ c.disableSELinux ? '是（已安装时执行）' : '否' }}</el-descriptions-item>
      <el-descriptions-item label="镜像仓库" :span="2">{{ c.imageRepository }}</el-descriptions-item>
      <el-descriptions-item label="自定义软件源" :span="2"><pre v-if="c.customSource" class="deploy-pre">{{ c.customSource }}</pre><span v-else>使用节点软件源</span></el-descriptions-item>
      <el-descriptions-item label="自定义 kubelet" :span="2"><pre v-if="c.kubeletConfig" class="deploy-pre">{{ c.kubeletConfig }}</pre><span v-else>默认配置</span></el-descriptions-item>
      <el-descriptions-item label="环境">{{ plan.env || '未分配' }}</el-descriptions-item>
      <el-descriptions-item label="描述">{{ plan.description || '—' }}</el-descriptions-item>
    </el-descriptions>
  </div>
  <div class="deploy-summary">
    <div class="deploy-toolbar"><h2>高级配置</h2><el-button v-if="editable" link type="primary" @click="emit('edit', 1)">修改</el-button></div>
    <el-descriptions :column="2" border>
      <el-descriptions-item label="高可用">{{ c.ha ? '三控制平面 · 堆叠 etcd' : '单控制平面' }}</el-descriptions-item>
      <el-descriptions-item label="负载均衡">{{ c.builtinLB ? 'HAProxy + Keepalived' : c.ha ? '已有负载均衡' : '无' }}</el-descriptions-item>
      <el-descriptions-item label="API 入口"><span class="deploy-mono">{{ summary.endpoint }}</span></el-descriptions-item>
      <el-descriptions-item label="Kube-proxy">{{ c.proxyMode }}</el-descriptions-item>
      <el-descriptions-item v-if="c.builtinLB" label="虚拟路由 ID">{{ c.routerId }}</el-descriptions-item>
      <el-descriptions-item label="证书有效期">{{ c.customCertificates ? `${c.certificateYears} 年 / CA ${c.caYears} 年` : 'kubeadm 默认' }}</el-descriptions-item>
      <el-descriptions-item label="Metrics Server">{{ c.metricsServer ? '安装' : '不安装' }}</el-descriptions-item>
      <el-descriptions-item label="控制平面业务调度">{{ c.allowControlPlaneWorkloads ? '允许' : '禁止' }}</el-descriptions-item>
    </el-descriptions>
  </div>
  <div class="deploy-summary">
    <div class="deploy-toolbar"><h2>关联节点</h2><div class="deploy-inline"><span class="deploy-node-count">{{ summary.masters }} 控制平面 / {{ summary.workers }} 工作节点</span><el-button v-if="editable" link type="primary" @click="emit('edit', 2)">修改</el-button></div></div>
    <el-table :data="plan.nodes || []" border>
      <el-table-column prop="hostName" label="CMDB 主机" min-width="150" />
      <el-table-column prop="name" label="节点名" min-width="150" />
      <el-table-column label="角色" width="110"><template #default="{ row }">{{ roleText(row.role) }}</template></el-table-column>
      <el-table-column prop="ip" label="部署 IP" min-width="145" />
      <el-table-column prop="interface" label="网口" width="100" />
      <el-table-column label="系统（自动识别）" min-width="200"><template #default="{ row }">{{ row.os || '待预检识别' }}<div class="deploy-help">{{ row.arch }}</div></template></el-table-column>
    </el-table>
  </div>
</template>
