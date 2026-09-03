<script setup>
import K8sNamespaceBoard from './K8sNamespaceBoard.vue'
import K8sWorkloadBoard from './K8sWorkloadBoard.vue'
import { Connection, CopyDocument } from '@element-plus/icons-vue'

function serviceTypeTagType(type) {
  return {
    ClusterIP: 'success',
    Headless: 'info',
    NodePort: 'warning',
    LoadBalancer: 'primary',
    ExternalName: 'info'
  }[type] || 'info'
}

function serviceTypeTagClass(type) {
  return type === 'Headless' ? 'service-type-headless' : ''
}

function servicePorts(ports) {
  return String(ports || '').split(',').map((item) => item.trim()).filter(Boolean)
}

function controlPlaneNodeCount(nodes) {
  return (nodes || []).filter((node) => String(node.role || '').toLowerCase().match(/control-plane|master/)).length
}

function workerNodeCount(nodes) {
  return (nodes || []).filter((node) => !String(node.role || '').toLowerCase().match(/control-plane|master/)).length
}

function workloadTypeCount(workloads, type) {
  return (workloads || []).filter((workload) => workload.type === type).length
}

function distributionRingStyle(parts) {
  const total = Math.max(parts.reduce((sum, part) => sum + part.value, 0), 1)
  let offset = 0
  const gradient = parts.map((part) => {
    const end = offset + (part.value / total) * 100
    const segment = `${part.color} ${offset}% ${end}%`
    offset = end
    return segment
  })
  return { background: `conic-gradient(${gradient.join(', ')})` }
}

defineProps({
  page: {
    type: Object,
    required: true
  }
})
</script>

<template>
  <section v-if="page.hasCluster && page.currentTab === 'overview' && page.overview" class="section-body overview-dashboard">
    <div class="overview-section-title"><strong>集群配置</strong><span>网络与版本信息</span></div>
    <div class="overview-config-card"><div v-for="item in page.overview.distribution" :key="item.label" class="overview-config-item"><span>{{ item.label }}</span><strong>{{ item.value }}</strong></div></div>

    <div class="overview-section-title"><strong>资源概览</strong><span>集群当前资源分布与运行状态</span></div>
    <div class="overview-glance-grid">
      <article class="overview-distribution-card">
        <div class="overview-card-head"><strong>节点</strong><span>{{ page.nodes.length }} 个节点</span></div>
        <div class="overview-card-content">
          <div class="overview-donut" :style="distributionRingStyle([{ value: controlPlaneNodeCount(page.nodes), color: '#6688ff' }, { value: workerNodeCount(page.nodes), color: '#43baf4' }])"><b>{{ page.nodes.length }}</b></div>
          <div class="overview-legend"><div><i class="control-plane"></i><span>控制平面节点</span><strong>{{ controlPlaneNodeCount(page.nodes) }}</strong></div><div><i class="worker-node"></i><span>工作节点</span><strong>{{ workerNodeCount(page.nodes) }}</strong></div></div>
        </div>
      </article>
      <article class="overview-distribution-card">
        <div class="overview-card-head"><strong>工作负载</strong><span>{{ page.workloads.length }} 个工作负载</span></div>
        <div class="overview-card-content">
          <div class="overview-donut" :style="distributionRingStyle([{ value: workloadTypeCount(page.workloads, 'Deployment'), color: '#6688ff' }, { value: workloadTypeCount(page.workloads, 'StatefulSet'), color: '#1bc8aa' }, { value: workloadTypeCount(page.workloads, 'DaemonSet'), color: '#f5a623' }, { value: workloadTypeCount(page.workloads, 'Job') + workloadTypeCount(page.workloads, 'CronJob'), color: '#a5afbd' }])"><b>{{ page.workloads.length }}</b></div>
          <div class="overview-legend workload-legend"><div><i class="deployment"></i><span>Deployment</span><strong>{{ workloadTypeCount(page.workloads, 'Deployment') }}</strong></div><div><i class="statefulset"></i><span>StatefulSet</span><strong>{{ workloadTypeCount(page.workloads, 'StatefulSet') }}</strong></div><div><i class="daemonset"></i><span>DaemonSet</span><strong>{{ workloadTypeCount(page.workloads, 'DaemonSet') }}</strong></div><div><i class="other-workload"></i><span>Job / CronJob</span><strong>{{ workloadTypeCount(page.workloads, 'Job') + workloadTypeCount(page.workloads, 'CronJob') }}</strong></div></div>
        </div>
      </article>
    </div>

    <div class="overview-section-title"><strong>资源预留概览</strong><span>按容器 Requests ÷ 节点可分配资源计算，不含 Limit 与实时用量</span></div>
    <article class="overview-usage-card">
      <div class="overview-usage-item"><span>{{ page.t('k8sCpuUsage') }}</span><strong>{{ page.overview.cpuUsage }}</strong><small>Requests / Allocatable · {{ page.t('k8sWorkloads') }} · {{ page.overview.requestRate }}</small></div>
      <div class="overview-usage-divider"></div>
      <div class="overview-usage-item"><span>{{ page.t('k8sMemoryUsage') }}</span><strong>{{ page.overview.memoryUsage }}</strong><small>Requests / Allocatable · {{ page.t('k8sPodUsage', { value: page.overview.podUsage }) }}</small></div>
      <div class="overview-usage-divider"></div>
      <div class="overview-usage-item"><span>{{ page.t('k8sHealthScore') }}</span><strong>{{ page.overview.healthScore }}</strong><small>{{ page.t('k8sCurrentAlerts', { count: page.overview.alertCount }) }}</small></div>
    </article>

    <div v-if="page.overview.certificates?.length" class="cert-band">
      <div class="cert-band-head">
        <div>
          <strong>{{ page.t('k8sCertificates') }}</strong>
          <span>{{ page.t('k8sCertificatesDesc') }}</span>
        </div>
      </div>
      <div class="cert-grid">
        <article v-for="item in page.overview.certificates" :key="item.type" class="cert-card">
          <div class="cert-card-head">
            <div class="cert-title">
              <strong>{{ item.name }}</strong>
              <span>{{ item.subject }}</span>
            </div>
            <el-tag size="small" :type="page.certificateStatusType(item.status)" effect="light">
              {{ page.certificateStatusText(item.status) }}
            </el-tag>
          </div>
          <div class="cert-meta-grid">
            <div class="cert-meta-item">
              <span>{{ page.t('k8sIssuer') }}</span>
              <strong>{{ item.issuer }}</strong>
            </div>
            <div class="cert-meta-item">
              <span>{{ page.t('k8sRemaining') }}</span>
              <strong>{{ page.certificateRemainText(item.daysRemaining) }}</strong>
            </div>
            <div class="cert-meta-item">
              <span>{{ page.t('k8sValidFrom') }}</span>
              <strong>{{ item.notBefore }}</strong>
            </div>
            <div class="cert-meta-item">
              <span>{{ page.t('k8sExpiresAt') }}</span>
              <strong>{{ item.notAfter }}</strong>
            </div>
          </div>
        </article>
      </div>
    </div>
  </section>

  <section v-if="page.hasCluster && page.currentTab === 'nodes'" class="section-body node-workspace">
    <div v-if="page.hasItems(page.nodes)" class="node-management-card">
      <div class="node-management-intro">
        <div><strong>节点运行概览</strong><span>查看节点角色、容量与 Pod 分配；标签管理会直接写入 Kubernetes Node。</span></div>
        <el-tag type="info" effect="plain">{{ page.nodes.length }} 个节点</el-tag>
      </div>
      <el-table :data="page.nodes" class="data-table node-table">
      <el-table-column :label="page.t('k8sName')" min-width="210">
        <template #default="{ row }"><div class="node-name-cell"><i></i><div><strong>{{ row.name }}</strong><small>{{ row.internalIP }}</small></div></div></template>
      </el-table-column>
      <el-table-column :label="page.t('k8sRole')" min-width="140"><template #default="{ row }"><el-tag effect="plain" type="info">{{ row.role || '-' }}</el-tag></template></el-table-column>
      <el-table-column :label="page.t('k8sStatus')" width="120"><template #default="{ row }"><el-tag :type="row.status === 'Ready' ? 'success' : 'danger'" effect="light">{{ row.status }}</el-tag></template></el-table-column>
      <el-table-column prop="version" :label="page.t('k8sVersion')" width="130" />
      <el-table-column prop="os" :label="page.t('k8sOs')" min-width="180" show-overflow-tooltip />
      <el-table-column :label="page.t('k8sCpu')" width="118"><template #default="{ row }"><div class="resource-cell"><b>{{ row.cpu }}</b><span>核</span></div></template></el-table-column>
      <el-table-column :label="page.t('k8sMemory')" min-width="145"><template #default="{ row }"><div class="resource-cell"><b>{{ row.memory }}</b></div></template></el-table-column>
      <el-table-column :label="page.t('k8sPodAllocation')" width="138"><template #default="{ row }"><div class="pod-allocation"><b>{{ row.pods }}</b><el-progress :percentage="page.nodePodPercent(row.pods)" :show-text="false" :stroke-width="5" /></div></template></el-table-column>
      <el-table-column :label="page.t('k8sActions')" width="180" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="page.openNodeLabels(row)">标签管理</el-button>
          <el-button link type="primary" @click="page.openNodeDetail(row)">{{ page.t('k8sDetail') }}</el-button>
        </template>
      </el-table-column>
      </el-table>
    </div>
    <el-empty v-else :description="page.t('k8sNoRealtimeNodeData')" />
  </section>

  <K8sNamespaceBoard v-if="page.hasCluster && page.currentTab === 'namespaces'" :page="page" />

  <section v-if="page.hasCluster && page.currentTab === 'pods'" class="section-body pod-workspace">
    <el-table v-if="page.hasItems(page.filteredPods)" :data="page.pagedPods" class="data-table pod-management-table">
      <el-table-column prop="name" :label="page.t('k8sPodName')" min-width="260" />
      <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="140" />
      <el-table-column label="工作负载" min-width="200">
        <template #default="{ row }">
          <div v-if="row.workloadName" class="pod-workload-cell">
            <el-tag size="small" effect="plain" class="pod-workload-type">{{ row.workloadName }}</el-tag>
          </div>
          <span v-else class="pod-workload-empty">独立 Pod</span>
        </template>
      </el-table-column>
      <el-table-column :label="page.t('k8sStatus')" width="110">
        <template #default="{ row }">
          <el-tag :type="page.podStatusTagType(row.status)" effect="light" round>{{ row.status || '-' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="page.t('k8sNode')" min-width="180">
        <template #default="{ row }">
          <div class="pod-node-cell">
            <span>节点：{{ row.node || '-' }}</span>
            <small>节点 IP：{{ row.nodeIP || '-' }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="page.t('k8sPodIp')" width="135">
        <template #default="{ row }">{{ row.ip || '-' }}</template>
      </el-table-column>
      <el-table-column prop="restarts" :label="page.t('k8sRestarts')" width="90" />
      <el-table-column prop="age" :label="page.t('k8sAge')" width="90" />
      <el-table-column :label="page.t('k8sActions')" width="230">
        <template #default="{ row }">
          <div class="pod-row-actions">
            <el-button link type="primary" @click="page.openPodDetail(row)">{{ page.t('k8sDetail') }}</el-button>
            <el-button link type="primary" @click="page.openPodLogs(row)">日志</el-button>
            <el-button link type="primary" @click="page.openPodYAML(row)">{{ page.t('k8sYaml') }}</el-button>
            <el-button link type="primary" @click="page.openPodTerminal(row)">{{ page.t('k8sTerminal') }}</el-button>
            <el-button link type="danger" @click="page.handleDeletePod(row)">{{ page.t('k8sDelete') }}</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>
    <div v-if="page.hasItems(page.filteredPods)" class="pod-pagination">
      <el-pagination
        background
        layout="total, sizes, prev, pager, next, jumper"
        :total="page.filteredPods.length"
        :current-page="page.podPage"
        :page-size="page.podPageSize"
        :page-sizes="[20, 30, 50, 100]"
        @size-change="page.handlePodPageSizeChange"
        @current-change="page.handlePodPageChange"
      />
    </div>
    <el-empty v-else :description="page.t('k8sNoRealtimePodData')" />
  </section>

  <K8sWorkloadBoard v-if="page.hasCluster && page.currentTab === 'workloads'" :page="page" />

  <section v-if="page.hasCluster && page.currentTab === 'services'" class="section-body service-workspace">
    <el-table v-if="page.hasItems(page.filteredServices)" :data="page.filteredServices" class="data-table service-resource-table">
      <el-table-column :label="page.t('k8sName')" min-width="220">
        <template #default="{ row }">
          <div class="service-name-cell">
            <span class="service-name-icon"><el-icon><Connection /></el-icon></span>
            <el-button link class="service-name-link" @click="page.openServiceDetail(row)">{{ row.name }}</el-button>
            <el-tooltip content="复制服务名称" placement="top">
              <el-button link class="service-copy-button" aria-label="复制服务名称" @click="page.copyServiceName(row)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="namespace" :label="page.t('k8sNamespace')" min-width="130">
        <template #default="{ row }"><el-tag class="service-namespace-tag" effect="plain">{{ row.namespace }}</el-tag></template>
      </el-table-column>
      <el-table-column :label="page.t('k8sType')" width="128">
        <template #default="{ row }"><el-tag :type="serviceTypeTagType(row.type)" :class="serviceTypeTagClass(row.type)" effect="light" round>{{ row.type }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="clusterIP" :label="page.t('k8sClusterIp')" min-width="142">
        <template #default="{ row }"><span class="service-ip-text">{{ row.clusterIP }}</span></template>
      </el-table-column>
      <el-table-column prop="externalIP" :label="page.t('k8sExternalIp')" min-width="142" />
      <el-table-column :label="page.t('k8sPorts')" min-width="245">
        <template #default="{ row }">
          <div class="service-port-list">
            <el-tag v-for="port in servicePorts(row.ports)" :key="port" size="small" effect="plain">{{ port }}</el-tag>
            <span v-if="!servicePorts(row.ports).length" class="service-muted">—</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="page.t('k8sEndpoints')" width="122" align="center">
        <template #default="{ row }"><span class="service-endpoint-count" :class="{ 'is-empty': !row.endpoints }">{{ row.endpoints }}</span></template>
      </el-table-column>
      <el-table-column prop="age" :label="page.t('k8sAge')" width="116" />
      <el-table-column :label="page.t('k8sActions')" width="210" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="page.openServiceDetail(row)">{{ page.t('k8sDetail') }}</el-button>
          <el-button link type="primary" @click="page.openServiceEdit(row)">编辑</el-button>
          <el-button link @click="page.openServiceYAML(row)">{{ page.t('k8sYaml') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-else :description="page.t('k8sNoRealtimeServiceData')" />
  </section>

  <section v-if="page.hasCluster && page.currentTab === 'ingresses'" class="section-body ingress-workspace">
    <el-tabs v-model="page.ingressTab" class="ingress-resource-tabs">
      <el-tab-pane label="Ingress" name="ingresses">
    <el-table v-if="page.hasItems(page.filteredIngresses)" :data="page.filteredIngresses" class="data-table ingress-resource-table">
      <el-table-column prop="name" :label="page.t('k8sName')" min-width="160" />
      <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="120" />
      <el-table-column prop="className" :label="page.t('k8sIngressClass')" min-width="130" />
      <el-table-column prop="host" :label="page.t('k8sHost')" min-width="180" />
      <el-table-column prop="backend" :label="page.t('k8sBackendService')" min-width="180" show-overflow-tooltip />
      <el-table-column prop="address" :label="page.t('k8sAddress')" min-width="140" />
      <el-table-column prop="tls" :label="page.t('k8sTls')" width="120" />
      <el-table-column prop="age" :label="page.t('k8sAge')" width="110" />
      <el-table-column :label="page.t('k8sActions')" width="210" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="page.openIngressDetail(row)">{{ page.t('k8sDetail') }}</el-button>
          <el-button link type="primary" @click="page.openIngressEdit(row)">编辑</el-button>
          <el-button link type="primary" @click="page.openIngressYAML(row)">{{ page.t('k8sYaml') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-else :description="page.t('k8sNoRealtimeIngressData')" />
      </el-tab-pane>
      <el-tab-pane label="IngressClass" name="ingressclasses">
        <el-table v-if="page.hasItems(page.filteredIngressClasses)" :data="page.filteredIngressClasses" class="data-table ingress-resource-table">
          <el-table-column prop="name" :label="page.t('k8sName')" min-width="220" />
          <el-table-column prop="controller" label="Controller" min-width="260" />
          <el-table-column prop="parameters" label="Parameters" min-width="260" />
          <el-table-column prop="age" :label="page.t('k8sAge')" width="120" />
        </el-table>
        <el-empty v-else description="暂无 IngressClass 实时数据" />
      </el-tab-pane>
    </el-tabs>
  </section>

  <section v-if="page.hasCluster && page.currentTab === 'advanced-network'" class="section-body config-grid network-workspace">
    <div class="subsection">
      <div class="subsection-head">
        <strong>{{ page.t('k8sGatewayApiGateways') }}</strong>
        <el-button type="primary" plain @click="page.openIstioCreateDialog('gatewayapi')">{{ page.t('k8sCreate') }}</el-button>
      </div>
      <el-table v-if="page.hasItems(page.filteredGatewayApiGateways)" :data="page.filteredGatewayApiGateways" class="data-table">
        <el-table-column prop="name" :label="page.t('k8sName')" min-width="180" />
        <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="120" />
        <el-table-column prop="hosts" :label="page.t('k8sHost')" min-width="220" />
        <el-table-column prop="address" :label="page.t('k8sAddress')" min-width="180" />
        <el-table-column prop="ports" :label="page.t('k8sPorts')" min-width="160" />
        <el-table-column prop="target" :label="page.t('k8sType')" min-width="160" />
        <el-table-column prop="age" :label="page.t('k8sAge')" width="110" />
        <el-table-column :label="page.t('k8sActions')" width="240" fixed="right">
          <template #default="{ row }">
            <div class="action-row">
              <el-button link type="primary" @click="page.openIstioResourceDetail(row, 'gatewayapi')">{{ page.t('k8sDetail') }}</el-button>
              <el-button link type="primary" @click="page.openIstioResourceYAML(row, 'gatewayapi')">{{ page.t('k8sYaml') }}</el-button>
              <el-button link type="danger" @click="page.handleDeleteIstioResource(row, 'gatewayapi')">{{ page.t('k8sDelete') }}</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else :description="page.t('k8sNoRealtimeAdvancedNetworkData')" />
    </div>

    <div class="subsection">
      <div class="subsection-head">
        <strong>{{ page.t('k8sHttpRoutes') }}</strong>
        <el-button type="primary" plain @click="page.openIstioCreateDialog('httproute')">{{ page.t('k8sCreate') }}</el-button>
      </div>
      <el-table v-if="page.hasItems(page.filteredHTTPRoutes)" :data="page.filteredHTTPRoutes" class="data-table">
        <el-table-column prop="name" :label="page.t('k8sName')" min-width="180" />
        <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="120" />
        <el-table-column prop="hosts" :label="page.t('k8sHost')" min-width="220" />
        <el-table-column prop="gateways" :label="page.t('k8sGateways')" min-width="180" />
        <el-table-column prop="target" :label="page.t('k8sTarget')" min-width="200" />
        <el-table-column prop="age" :label="page.t('k8sAge')" width="110" />
        <el-table-column :label="page.t('k8sActions')" min-width="300" fixed="right">
          <template #default="{ row }">
            <div class="action-row">
              <el-button link type="primary" @click="page.openIstioResourceDetail(row, 'httproute')">{{ page.t('k8sDetail') }}</el-button>
              <el-button link type="primary" @click="page.openIstioResourceYAML(row, 'httproute')">{{ page.t('k8sYaml') }}</el-button>
              <el-button link type="warning" @click="page.openTrafficDialog({ ...row, resourceType: 'httproute' })">{{ page.t('k8sTraffic') }}</el-button>
              <el-button link type="danger" @click="page.handleDeleteIstioResource(row, 'httproute')">{{ page.t('k8sDelete') }}</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else :description="page.t('k8sNoRealtimeAdvancedNetworkData')" />
    </div>
  </section>

  <section v-if="page.hasCluster && page.currentTab === 'monitoring'" class="section-body">
    <div class="k8s-monitoring">
      <header class="monitoring-toolbar">
        <div class="monitoring-source"><span>Prometheus 数据源</span><strong>{{ page.cluster?.monitorDatasourceName || '未绑定' }}</strong><i :class="['monitoring-status-dot', { offline: !page.monitorDatasourceBound }]" /><em>{{ page.monitorDatasourceBound ? '已连接' : '未配置' }}</em><small v-if="page.monitorLastUpdated">更新于 {{ page.monitorLastUpdated }}</small></div>
        <div class="monitoring-toolbar-actions"><el-select v-model="page.monitorRange" size="small" style="width: 116px"><el-option label="最近 1 小时" value="1h" /><el-option label="最近 6 小时" value="6h" /><el-option label="最近 24 小时" value="24h" /><el-option label="最近 3 天" value="3d" /><el-option label="最近 7 天" value="7d" /></el-select><el-button size="small" :loading="page.monitorLoading" @click="page.loadMonitoringCharts(true)">刷新</el-button></div>
      </header>
      <div v-if="!page.monitorDatasourceBound" class="monitoring-unbound"><el-empty description="当前集群未绑定 Prometheus 或 VictoriaMetrics 数据源"><template #default><p>请在集群编辑页关联监控数据源后查看真实的指标趋势。</p></template></el-empty></div>
      <template v-else>
        <aside class="monitoring-nav" aria-label="监控分类"><span class="monitoring-nav-caption">监控视图</span><button :class="{ active: page.monitorView === 'cluster' }" :aria-pressed="page.monitorView === 'cluster'" type="button" @click="page.monitorView = 'cluster'"><i />集群监控概览<small>健康与资源趋势</small></button><span class="monitoring-nav-group">基础监控</span><button :class="{ active: page.monitorView === 'node' }" :aria-pressed="page.monitorView === 'node'" type="button" @click="page.monitorView = 'node'"><i />Node 监控<small>节点负载与磁盘</small></button><button :class="{ active: page.monitorView === 'pod' }" :aria-pressed="page.monitorView === 'pod'" type="button" @click="page.monitorView = 'pod'"><i />Pod 监控<small>容器资源与重启</small></button><span class="monitoring-nav-group">流量观测</span><button :class="{ active: page.monitorView === 'network' }" :aria-pressed="page.monitorView === 'network'" type="button" @click="page.monitorView = 'network'"><i />网络监控<small>流入、流出与吞吐</small></button></aside>
        <main class="monitoring-main">
          <template v-if="page.monitorView === 'cluster'"><div class="monitoring-section-head"><div><h3>集群监控概览</h3><p>节点、Pod 与核心资源的实时运行状态</p></div></div><div class="monitoring-summary-grid"><article v-for="item in page.monitorSummary" :key="item.label" :class="['monitoring-summary-card', item.tone]"><span>{{ item.label }}</span><strong>{{ item.value }}</strong><small>{{ item.hint }}</small></article></div></template>
          <template v-else-if="page.monitorView === 'node'"><div class="monitoring-section-head"><div><h3>Node 监控</h3><p>点击节点可查看该节点的资源与网络趋势</p></div></div><div class="monitoring-node-overview"><strong>节点总览</strong><span>共 {{ page.monitorNodeRows.length }} 个节点</span></div><el-table v-if="page.hasItems(page.monitorNodeRows)" :data="page.monitorNodeRows" size="small" class="monitoring-node-table" row-class-name="monitoring-node-row" @row-click="page.openMonitorNode"><el-table-column prop="name" label="节点" min-width="150" fixed="left"><template #default="{ row }"><button type="button" class="monitoring-node-link" @click.stop="page.openMonitorNode(row)">{{ row.name }}</button></template></el-table-column><el-table-column label="状态" width="95"><template #default="{ row }"><span :class="['monitoring-node-status', { ready: String(row.status).toLowerCase() === 'ready' }]">{{ row.status || '-' }}</span></template></el-table-column><el-table-column prop="pods" label="Pod" width="78" /><el-table-column prop="cpu" label="总 CPU" width="82" /><el-table-column label="CPU 使用率" min-width="130"><template #default="{ row }"><div class="monitoring-meter"><i><b :style="{ width: `${Math.min(100, row.cpuUsageValue || 0)}%` }" /></i><span>{{ row.cpuUsage }}</span></div></template></el-table-column><el-table-column prop="memory" label="总内存" width="95" /><el-table-column label="内存使用率" min-width="130"><template #default="{ row }"><div class="monitoring-meter memory"><i><b :style="{ width: `${Math.min(100, row.memoryUsageValue || 0)}%` }" /></i><span>{{ row.memoryUsage }}</span></div></template></el-table-column><el-table-column prop="diskFree" label="剩余磁盘" width="105" /><el-table-column prop="transmit" label="发送" width="105" /><el-table-column prop="receive" label="接收" width="105" /><el-table-column prop="connections" label="连接数" width="85" /><el-table-column prop="retransmit" label="重传率" width="85" /><el-table-column prop="load" label="Load 5m" width="82" /><el-table-column prop="uptime" label="在线时间" width="105" /></el-table></template>
          <template v-else-if="page.monitorView === 'pod'"><div class="monitoring-section-head"><div><h3>Pod 监控</h3><p>Pod 的 CPU、内存、运行数量和重启情况</p></div></div><div class="monitoring-pod-glance"><span>集群 Pod 总数 <b>{{ page.pods.length }}</b></span><span>运行中 <b>{{ page.monitorSummary[2]?.value || 0 }}</b></span><span>命名空间 <b>{{ page.namespaces.length }}</b></span></div></template>
          <template v-else><div class="monitoring-section-head"><div><h3>网络监控</h3><p>节点与 Pod 的实时流入、流出和吞吐趋势</p></div></div><div class="monitoring-pod-glance"><span>节点数量 <b>{{ page.nodes.length }}</b></span><span>服务数量 <b>{{ page.services.length }}</b></span><span>Ingress 数量 <b>{{ page.ingresses.length }}</b></span></div></template>
          <section v-if="page.monitorView === 'node'" class="monitor-node-insights">
            <div class="monitor-node-insight-stats"><article><span>在线实例</span><strong class="success">{{ page.monitorNodeDashboardStats.online }}</strong></article><article><span>总连接数</span><strong>{{ page.monitorNodeDashboardStats.connections }}</strong></article><article><span>总吞吐速率</span><strong class="primary">{{ page.monitorNodeDashboardStats.throughput }}</strong></article></div>
            <div class="monitor-node-ranking"><header><strong>节点吞吐排名</strong><span>Mean / Max</span></header><div v-for="item in page.monitorNodeThroughputRanking" :key="item.name" class="monitor-node-ranking-row"><b>{{ item.name }}</b><i><em :style="{ width: `${Math.min(100, item.mean / Math.max(...page.monitorNodeThroughputRanking.map(row => row.mean), 0.000001) * 100)}%` }" /></i><span>{{ item.meanText }}</span><span>{{ item.maxText }}</span></div><el-empty v-if="!page.monitorNodeThroughputRanking.length" description="暂无吞吐数据" :image-size="34" /></div>
            <div class="monitor-node-period-total"><article><span>30 天内发送</span><strong class="primary">{{ page.monitorNodeDashboardStats.transmit30d }}</strong></article><article><span>30 天内接收</span><strong>{{ page.monitorNodeDashboardStats.receive30d }}</strong></article><article><span>30 天内总计</span><strong class="warning">{{ page.monitorNodeDashboardStats.total30d }}</strong></article></div>
          </section>
          <div class="monitoring-charts-grid"><article v-for="chart in page.monitorVisibleCharts" :key="chart.title" v-loading="chart.loading" class="monitoring-chart-card"><header><span>{{ chart.title }}</span><b>{{ page.monitorLatestValue(chart) }}</b></header><div v-if="chart.series.length" class="monitoring-chart-canvas"><div class="monitoring-chart-scale"><span>{{ page.monitorFormatValue(chart.maxValue, chart.unit) }}</span><span>{{ page.monitorFormatValue(chart.minValue, chart.unit) }}</span></div><svg viewBox="0 0 1000 220" preserveAspectRatio="none" role="img" :aria-label="chart.title" @mousemove="page.updateMonitorChartHover(chart, $event)" @mouseleave="page.monitorHover = null"><line v-for="grid in 5" :key="grid" x1="42" x2="960" :y1="14 + (grid - 1) * 46" :y2="14 + (grid - 1) * 46" class="monitoring-chart-grid" /><path :d="page.monitorChartArea(chart, chart.series[0].values)" class="monitoring-chart-area" /><path v-for="(series, index) in chart.series" :key="series.label" :d="page.monitorChartPath(chart, series.values)" :stroke="page.monitorColor(index)" class="monitoring-chart-line" /><template v-if="page.monitorHover?.chart === chart.title"><line :x1="42 + page.monitorHover.position * 918" :x2="42 + page.monitorHover.position * 918" y1="14" y2="198" class="monitoring-chart-cursor" /></template></svg><div v-if="page.monitorHover?.chart === chart.title" class="monitoring-chart-tooltip" :style="{ left: `${Math.min(76, Math.max(6, page.monitorHover.position * 100))}%` }"><strong>{{ page.monitorTimeText(page.monitorHover.time) }}</strong><span v-for="point in page.monitorHover.points.slice(0, 6)" :key="point.label"><i :style="{ background: point.color }" />{{ point.label }}<b>{{ page.monitorFormatValue(point.value, page.monitorHover.unit) }}</b></span></div><div class="monitoring-chart-axis"><span>{{ page.monitorTimeText(chart.minTime) }}</span><span>{{ page.monitorTimeText(chart.maxTime) }}</span></div><div class="monitoring-chart-legend"><span v-for="(series, index) in chart.series.slice(0, 3)" :key="series.label"><i :style="{ background: page.monitorColor(index) }" />{{ series.label }}</span><em v-if="chart.series.length > 3">+{{ chart.series.length - 3 }} 条序列</em></div></div><el-empty v-else-if="!chart.loading" :description="chart.failed ? '指标查询失败，可单独刷新重试' : '当前数据源暂无该指标'" :image-size="42" /></article></div>
        </main>
      </template>
    </div>
    <el-drawer v-model="page.monitorNodeDrawerVisible" size="78%" class="monitor-node-drawer" destroy-on-close>
      <template #header><div class="monitor-node-drawer-title"><strong>{{ page.monitorNodeSelected?.name || 'Node 详情' }}</strong><span :class="['monitoring-node-status', { ready: String(page.monitorNodeSelected?.status).toLowerCase() === 'ready' }]">{{ page.monitorNodeSelected?.status || '-' }}</span><small>{{ page.monitorNodeSelected?.internalIP }}</small></div></template>
      <div v-if="page.monitorNodeSelected" class="monitor-node-detail">
        <div class="monitor-node-stat-grid"><article><span>CPU</span><strong>{{ page.monitorNodeSelected.cpuUsage }}</strong></article><article><span>内存</span><strong>{{ page.monitorNodeSelected.memoryUsage }}</strong></article><article><span>剩余磁盘</span><strong class="warning">{{ page.monitorNodeSelected.diskFree }}</strong></article><article><span>Pods</span><strong>{{ page.monitorNodeSelected.pods }}</strong></article><article><span>Load 5m</span><strong>{{ page.monitorNodeSelected.load }}</strong></article></div>
        <div class="monitor-node-detail-section"><strong>资源趋势</strong><span>所选时间范围内的节点指标</span></div>
        <div class="monitoring-charts-grid monitor-node-chart-grid"><article v-for="chart in page.monitorNodeDetailCharts" :key="chart.key" class="monitoring-chart-card"><header><span>{{ chart.title }}</span><b>{{ page.monitorLatestValue(chart) }}</b></header><div v-if="chart.series.length" class="monitoring-chart-canvas"><div class="monitoring-chart-scale"><span>{{ page.monitorFormatValue(chart.maxValue, chart.unit) }}</span><span>{{ page.monitorFormatValue(chart.minValue, chart.unit) }}</span></div><svg viewBox="0 0 1000 220" preserveAspectRatio="none" role="img" :aria-label="chart.title" @mousemove="page.updateMonitorChartHover(chart, $event)" @mouseleave="page.monitorHover = null"><line v-for="grid in 5" :key="grid" x1="42" x2="960" :y1="14 + (grid - 1) * 46" :y2="14 + (grid - 1) * 46" class="monitoring-chart-grid" /><path v-if="chart.series[0]" :d="page.monitorChartArea(chart, chart.series[0].values)" class="monitoring-chart-area" /><path v-for="(series, index) in chart.series" :key="series.label" :d="page.monitorChartPath(chart, series.values)" :stroke="page.monitorColor(index)" class="monitoring-chart-line" /><line v-if="page.monitorHover?.chart === chart.title" :x1="42 + page.monitorHover.position * 918" :x2="42 + page.monitorHover.position * 918" y1="14" y2="198" class="monitoring-chart-cursor" /></svg><div v-if="page.monitorHover?.chart === chart.title" class="monitoring-chart-tooltip" :style="{ left: `${Math.min(76, Math.max(6, page.monitorHover.position * 100))}%` }"><strong>{{ page.monitorTimeText(page.monitorHover.time) }}</strong><span v-for="point in page.monitorHover.points.slice(0, 6)" :key="point.label"><i :style="{ background: point.color }" />{{ point.label }}<b>{{ page.monitorFormatValue(point.value, page.monitorHover.unit) }}</b></span></div><div class="monitoring-chart-axis"><span>{{ page.monitorTimeText(chart.minTime) }}</span><span>{{ page.monitorTimeText(chart.maxTime) }}</span></div></div><el-empty v-else description="暂无该节点指标" :image-size="40" /></article></div>
      </div>
    </el-drawer>
  </section>

  <section v-if="page.hasCluster && page.currentTab === 'config-storage'" class="section-body storage-workspace">
    <div class="config-storage-tabs">
      <div class="config-storage-create-action">
        <el-button v-if="page.configStorageTab === 'storage-classes'" type="primary" @click="page.openStorageClassCreate">
          新增存储类
        </el-button>
        <el-button v-else type="primary" @click="page.openConfigStorageCreate">
          {{ page.configStorageTab === 'configmaps' ? '新建 ConfigMap' : page.configStorageTab === 'secrets' ? '新建 Secret' : '新增存储卷' }}
        </el-button>
      </div>
      <el-tabs v-model="page.configStorageTab">
        <el-tab-pane :label="page.t('k8sConfigMaps')" name="configmaps">
          <el-table v-if="page.hasItems(page.filteredConfigMaps)" :data="page.filteredConfigMaps" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sName')" min-width="180" />
            <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="120" />
            <el-table-column prop="keys" :label="page.t('k8sKeys')" width="100" />
            <el-table-column prop="age" :label="page.t('k8sAge')" width="110" />
            <el-table-column :label="page.t('k8sActions')" width="260">
              <template #default="{ row }">
                <el-button link type="primary" @click="page.openConfigMapDetail(row)">{{ page.t('k8sDetail') }}</el-button>
                <el-button link type="primary" @click="page.openConfigMapEdit(row)">编辑</el-button>
                <el-button link type="primary" @click="page.openConfigMapYAML(row)">{{ page.t('k8sYaml') }}</el-button>
                <el-button link type="danger" @click="page.deleteConfigMap(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else :description="page.t('k8sNoRealtimeConfigMapData')" />
        </el-tab-pane>

        <el-tab-pane :label="page.t('k8sSecrets')" name="secrets">
          <el-table v-if="page.hasItems(page.filteredSecrets)" :data="page.filteredSecrets" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sName')" min-width="180" />
            <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="120" />
            <el-table-column prop="type" :label="page.t('k8sType')" min-width="160" />
            <el-table-column prop="age" :label="page.t('k8sAge')" width="110" />
            <el-table-column :label="page.t('k8sActions')" width="260">
              <template #default="{ row }">
                <el-button link type="primary" @click="page.openSecretDetail(row)">{{ page.t('k8sDetail') }}</el-button>
                <el-button link type="primary" @click="page.openSecretEdit(row)">编辑</el-button>
                <el-button link type="primary" @click="page.openSecretYAML(row)">{{ page.t('k8sYaml') }}</el-button>
                <el-button link type="danger" @click="page.deleteSecret(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else :description="page.t('k8sNoRealtimeSecretData')" />
        </el-tab-pane>

        <el-tab-pane label="存储类" name="storage-classes">
          <el-table v-if="page.hasItems(page.filteredStorageClasses)" :data="page.filteredStorageClasses" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sName')" min-width="200" />
            <el-table-column prop="namespaceScope" label="限定命名空间" min-width="140" />
            <el-table-column prop="status" :label="page.t('k8sStatus')" width="120" />
            <el-table-column prop="capacity" :label="page.t('k8sCapacity')" width="120" />
            <el-table-column prop="sourceType" label="存储源" width="110" />
            <el-table-column prop="path" label="路径" min-width="180" show-overflow-tooltip />
            <el-table-column prop="accessModes" label="读取策略" min-width="160" />
            <el-table-column prop="reclaimPolicy" label="回收策略" width="120" />
            <el-table-column :label="page.t('k8sActions')" width="240">
              <template #default="{ row }">
                <el-button link type="primary" @click="page.openStorageDetail(row)">{{ page.t('k8sDetail') }}</el-button>
                <el-button link type="primary" @click="page.openStorageYAML(row)">{{ page.t('k8sYaml') }}</el-button>
                <el-button link type="danger" @click="page.deleteStorageClass(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else description="暂无存储类，请新增 hostPath 或 NFS 存储类" />
        </el-tab-pane>

        <el-tab-pane label="存储卷" name="storage-volumes">
          <el-table v-if="page.hasItems(page.filteredStorageVolumes)" :data="page.filteredStorageVolumes" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sName')" min-width="200" />
            <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="150" />
            <el-table-column prop="status" :label="page.t('k8sStatus')" width="120" />
            <el-table-column prop="capacity" :label="page.t('k8sCapacity')" width="120" />
            <el-table-column prop="storageClass" :label="page.t('k8sStorageClass')" min-width="160" />
            <el-table-column prop="accessModes" label="读取策略" min-width="160" />
            <el-table-column :label="page.t('k8sActions')" width="240">
              <template #default="{ row }">
                <el-button link type="primary" @click="page.openStorageDetail(row)">{{ page.t('k8sDetail') }}</el-button>
                <el-button link type="primary" @click="page.openStorageYAML(row)">{{ page.t('k8sYaml') }}</el-button>
                <el-button link type="danger" @click="page.deleteStorageVolume(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else description="暂无存储卷，请新增 PersistentVolumeClaim" />
        </el-tab-pane>
      </el-tabs>
    </div>
  </section>

  <section v-if="!page.hasCluster" class="section-body">
    <el-empty :description="page.t('k8sNeedClusterFirst')" />
  </section>
</template>
