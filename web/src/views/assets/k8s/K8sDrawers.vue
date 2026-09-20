<script setup>
import PodMonitor from './PodMonitor.vue'
import ServicePodMonitor from '../ServicePodMonitor.vue'

defineProps({
  page: {
    type: Object,
    required: true
  }
})
</script>

<template>
  <el-drawer v-model="page.nodeDrawerVisible" :title="page.t('k8sNodeDetailTitle')" size="56%">
    <div v-loading="page.nodeDrawerLoading" class="drawer-content">
      <template v-if="page.nodeDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.nodeDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStatus')">{{ page.nodeDetail.status }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sRoles')">{{ page.nodeDetail.roles }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sVersion')">{{ page.nodeDetail.version }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sInternalIp')">{{ page.nodeDetail.internalIP }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sArchitecture')">{{ page.nodeDetail.architecture }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sOs')">{{ page.nodeDetail.os }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sKernel')">{{ page.nodeDetail.kernel }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sContainerRuntime')">{{ page.nodeDetail.containerRuntime }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sCpu')">{{ page.nodeDetail.allocatableCPU }} / {{ page.nodeDetail.capacityCPU }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sMemory')">{{ page.nodeDetail.allocatableMemory }} / {{ page.nodeDetail.capacityMemory }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sNodeLabels') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.nodeDetail.labels || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sPodsOnNode') }}</strong>
          <el-table :data="page.nodePods" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sResourcePod')" min-width="220" />
            <el-table-column prop="namespace" :label="page.t('k8sNamespace')" width="140" />
            <el-table-column prop="status" :label="page.t('k8sStatus')" width="120" />
            <el-table-column prop="restarts" :label="page.t('k8sRestarts')" width="90" />
            <el-table-column prop="age" :label="page.t('k8sAge')" width="100" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-dialog v-model="page.nodeLabelsVisible" width="720px" destroy-on-close class="node-label-dialog">
    <template #header>
      <div class="node-label-dialog-title">
        <div>
          <strong>节点标签管理</strong>
          <span>{{ page.nodeLabelTarget?.name || '-' }}</span>
        </div>
      </div>
    </template>
    <div class="node-label-intro">
      标签会直接更新到 Kubernetes Node，可用于节点筛选、工作负载调度与环境标识。保存前请确认系统标签是否仍需保留。
    </div>
    <div class="node-label-grid-head">
      <span>标签键</span>
      <span>标签值</span>
      <span>操作</span>
    </div>
    <div v-for="(item, index) in page.nodeLabelItems" :key="`${item.key}-${index}`" class="node-label-row">
      <el-input v-model="item.key" placeholder="例如 workload.example.com/tier" />
      <el-input v-model="item.value" placeholder="可留空" />
      <el-button link type="danger" @click="page.removeNodeLabel(index)">删除</el-button>
    </div>
    <el-button link type="primary" @click="page.addNodeLabel">+ 添加标签</el-button>
    <template #footer>
      <el-button @click="page.nodeLabelsVisible = false">取消</el-button>
      <el-button type="primary" :loading="page.nodeLabelsSaving" @click="page.saveNodeLabels">保存标签</el-button>
    </template>
  </el-dialog>

  <el-drawer v-model="page.namespaceDrawerVisible" :title="page.t('k8sNamespaceDetailTitle')" size="56%">
    <div v-loading="page.namespaceDrawerLoading" class="drawer-content">
      <template v-if="page.namespaceDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.namespaceDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStatus')">{{ page.namespaceDetail.status }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sCreatedAt')">{{ page.namespaceDetail.createdAt }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sPodsCount')">{{ page.namespaceDetail.pods }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sServicesCount')">{{ page.namespaceDetail.services }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sWorkloadsCount')">{{ page.namespaceDetail.workloads }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sConfigMaps')">{{ page.namespaceDetail.configMaps }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sSecrets')">{{ page.namespaceDetail.secrets }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStorage')">{{ page.namespaceDetail.storage }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sLabels') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.namespaceDetail.labels || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <div class="drawer-section" v-if="page.hasItems(page.namespaceEvents)">
          <strong>{{ page.t('k8sEvents') }}</strong>
          <el-table :data="page.namespaceEvents" class="data-table">
            <el-table-column prop="type" :label="page.t('k8sType')" width="100" />
            <el-table-column prop="reason" :label="page.t('k8sReason')" width="140" />
            <el-table-column prop="message" :label="page.t('k8sMessage')" min-width="280" />
            <el-table-column prop="count" :label="page.t('k8sCount')" width="80" />
            <el-table-column prop="lastTime" :label="page.t('k8sLastTime')" width="160" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.workloadDrawerVisible" :title="page.t('k8sWorkloadDetailTitle')" size="62%">
    <div v-loading="page.workloadDrawerLoading" class="drawer-content">
      <template v-if="page.workloadDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.workloadDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sType')">{{ page.workloadDetail.type }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.workloadDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sReady')">{{ page.workloadDetail.ready }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sUpdated')">{{ page.workloadDetail.updated }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAvailable')">{{ page.workloadDetail.available }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.workloadDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sSelector') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.workloadDetail.selector || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sLabels') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.workloadDetail.labels || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sContainers') }}</strong>
          <el-table :data="page.workloadDetail.containers || []" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sContainer')" min-width="180" />
            <el-table-column prop="image" :label="page.t('k8sImage')" min-width="280" />
            <el-table-column label="CPU Request / Limit" min-width="170">
              <template #default="{ row }">{{ row.requestCPU || '-' }} / {{ row.limitCPU || '-' }}</template>
            </el-table-column>
            <el-table-column label="内存 Request / Limit" min-width="190">
              <template #default="{ row }">{{ row.requestMemory || '-' }} / {{ row.limitMemory || '-' }}</template>
            </el-table-column>
          </el-table>
        </div>

        <ServicePodMonitor
          compact
          :cluster-id="page.cluster?.id"
          :namespace="page.workloadDetail.namespace"
          :workload-type="page.workloadDetail.type"
          :workload-name="page.workloadDetail.name"
        />

        <div class="drawer-section">
          <strong>{{ page.t('k8sRelatedPods') }}</strong>
          <el-table :data="page.workloadDetail.pods || []" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sPodName')" min-width="220" />
            <el-table-column prop="status" :label="page.t('k8sStatus')" width="120" />
            <el-table-column prop="node" :label="page.t('k8sNode')" min-width="150" />
            <el-table-column prop="restarts" :label="page.t('k8sRestarts')" width="100" />
            <el-table-column prop="age" :label="page.t('k8sAge')" width="100" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.serviceDrawerVisible" :title="page.t('k8sServiceDetailTitle')" size="58%">
    <div v-loading="page.serviceDrawerLoading" class="drawer-content">
      <template v-if="page.serviceDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.serviceDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sType')">{{ page.serviceDetail.type }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.serviceDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sClusterIp')">{{ page.serviceDetail.clusterIP }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sExternalIp')">{{ page.serviceDetail.externalIP }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sEndpoints')">{{ page.serviceDetail.endpoints }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.serviceDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sSelector') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.serviceDetail.selector || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sPorts') }}</strong>
          <el-table :data="page.serviceDetail.ports || []" class="data-table">
            <el-table-column prop="label" :label="page.t('k8sName')" min-width="160" />
            <el-table-column prop="value" :label="page.t('k8sTarget')" min-width="220" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.ingressDrawerVisible" :title="page.t('k8sIngressDetailTitle')" size="58%">
    <div v-loading="page.ingressDrawerLoading" class="drawer-content">
      <template v-if="page.ingressDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.ingressDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.ingressDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sHost')">{{ page.ingressDetail.host }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAddress')">{{ page.ingressDetail.address }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sTls')">{{ page.ingressDetail.tls }}</el-descriptions-item>
          <el-descriptions-item label="IngressClass">{{ page.ingressDetail.className }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.ingressDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sRules') }}</strong>
          <el-table :data="page.ingressDetail.rules || []" class="data-table">
            <el-table-column prop="label" :label="page.t('k8sRule')" min-width="240" />
            <el-table-column prop="value" :label="page.t('k8sBackend')" min-width="220" />
          </el-table>
        </div>

        <div class="drawer-section ingress-annotation-section">
          <div class="ingress-detail-section-head">
            <strong>注解</strong>
            <span>{{ Object.keys(page.ingressDetail.annotations || {}).length }} 项</span>
          </div>
          <el-table
            v-if="Object.keys(page.ingressDetail.annotations || {}).length"
            :data="Object.entries(page.ingressDetail.annotations || {}).map(([key, value]) => ({ key, value }))"
            class="data-table ingress-annotation-table"
          >
            <el-table-column prop="key" label="注解键" min-width="280">
              <template #default="{ row }"><code class="ingress-annotation-key">{{ row.key }}</code></template>
            </el-table-column>
            <el-table-column label="注解值" min-width="360">
              <template #default="{ row }"><div class="ingress-annotation-value">{{ row.value }}</div></template>
            </el-table-column>
          </el-table>
          <div v-else class="ingress-annotation-empty">该 Ingress 未配置注解。</div>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.istioDrawerVisible" :title="page.t('k8sIstioDetailTitle')" size="58%">
    <div v-loading="page.istioDrawerLoading" class="drawer-content">
      <template v-if="page.istioDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.istioDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sKind')">{{ page.istioDetail.kind }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.istioDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.istioDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sSummary') }}</strong>
          <el-table :data="page.istioDetail.summary || []" class="data-table">
            <el-table-column min-width="180">
              <template #header>{{ page.t('k8sName') }}</template>
              <template #default="{ row }">{{ page.translateIstioDetailLabel(row.label) }}</template>
            </el-table-column>
            <el-table-column prop="value" :label="page.t('k8sTarget')" min-width="260" />
          </el-table>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sItems') }}</strong>
          <el-table :data="page.istioDetail.items || []" class="data-table">
            <el-table-column min-width="180">
              <template #header>{{ page.t('k8sName') }}</template>
              <template #default="{ row }">{{ page.translateIstioDetailLabel(row.label) }}</template>
            </el-table-column>
            <el-table-column prop="value" :label="page.t('k8sTarget')" min-width="260" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.configMapDrawerVisible" :title="page.t('k8sConfigMapDetailTitle')" size="58%">
    <div v-loading="page.configMapDrawerLoading" class="drawer-content">
      <template v-if="page.configMapDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.configMapDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.configMapDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.configMapDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>键值</strong>
          <el-table :data="page.configMapDetail.keys || []" class="data-table">
            <el-table-column prop="label" :label="page.t('k8sKeys')" min-width="220" />
            <el-table-column label="值" min-width="360">
              <template #default="{ row }">
                <pre class="k8s-detail-value">{{ row.value }}</pre>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.secretDrawerVisible" :title="page.t('k8sSecretDetailTitle')" size="58%">
    <div v-loading="page.secretDrawerLoading" class="drawer-content">
      <template v-if="page.secretDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.secretDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.secretDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sType')">{{ page.secretDetail.type }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.secretDetail.age }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>键值</strong>
          <el-table :data="page.secretDetail.keys || []" class="data-table">
            <el-table-column prop="label" :label="page.t('k8sKeys')" min-width="220" />
            <el-table-column label="值" min-width="360">
              <template #default="{ row }">
                <el-input :model-value="row.value" type="password" show-password readonly class="k8s-secret-value" />
              </template>
            </el-table-column>
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.storageDrawerVisible" :title="page.t('k8sStorageDetailTitle')" size="58%">
    <div v-loading="page.storageDrawerLoading" class="drawer-content">
      <template v-if="page.storageDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sName')">{{ page.storageDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sKind')">{{ page.storageDetail.kind }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.storageDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStatus')">{{ page.storageDetail.status }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sCapacity')">{{ page.storageDetail.capacity }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStorageClass')">{{ page.storageDetail.storageClass }}</el-descriptions-item>
          <el-descriptions-item v-if="page.storageDetail.kind === 'PV'" label="存储源">{{ page.storageDetail.sourceType }}</el-descriptions-item>
          <el-descriptions-item v-if="page.storageDetail.kind === 'PV'" label="限定命名空间">{{ page.storageDetail.namespaceScope || '集群级' }}</el-descriptions-item>
          <el-descriptions-item v-if="page.storageDetail.kind === 'PV'" label="路径">{{ page.storageDetail.path }}</el-descriptions-item>
          <el-descriptions-item v-if="page.storageDetail.kind === 'PV' && page.storageDetail.nfsServer !== '-'" label="NFS 服务地址">{{ page.storageDetail.nfsServer }}</el-descriptions-item>
          <el-descriptions-item label="读取策略">{{ page.storageDetail.accessModes || '-' }}</el-descriptions-item>
          <el-descriptions-item v-if="page.storageDetail.kind === 'PV'" label="回收策略">{{ page.storageDetail.reclaimPolicy }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sAge')">{{ page.storageDetail.age }}</el-descriptions-item>
        </el-descriptions>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.podDrawerVisible" :title="page.t('k8sPodDetailTitle')" size="60%">
    <div v-loading="page.podDrawerLoading" class="drawer-content">
      <template v-if="page.podDetail">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="page.t('k8sPodName')">{{ page.podDetail.name }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sStatus')">{{ page.podDetail.status }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNamespace')">{{ page.podDetail.namespace }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sNode')">{{ page.podDetail.node }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sPodIp')">{{ page.podDetail.podIP }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sHostIp')">{{ page.podDetail.hostIP }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sQosClass')">{{ page.podDetail.qosClass }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sServiceAccount')">{{ page.podDetail.serviceAccount }}</el-descriptions-item>
          <el-descriptions-item :label="page.t('k8sCreatedAt')">{{ page.podDetail.createdAt }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-section">
          <strong>{{ page.t('k8sContainers') }}</strong>
          <el-table :data="page.podDetail.containers || []" class="data-table">
            <el-table-column prop="name" :label="page.t('k8sContainer')" min-width="160" />
            <el-table-column prop="image" :label="page.t('k8sImage')" min-width="240" />
            <el-table-column :label="page.t('k8sReady')" width="100">
              <template #default="{ row }">
                <el-tag :type="row.ready ? 'success' : 'warning'" effect="light">
                  {{ row.ready ? page.t('k8sReadyStatus') : page.t('k8sNotReadyStatus') }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="restart" :label="page.t('k8sRestarts')" width="100" />
          </el-table>
        </div>

        <div class="drawer-section">
          <strong>{{ page.t('k8sLabels') }}</strong>
          <div class="tag-group">
            <el-tag v-for="(value, key) in page.podDetail.labels || {}" :key="key" class="info-tag" effect="plain">
              {{ key }}={{ value }}
            </el-tag>
          </div>
        </div>

        <PodMonitor :cluster-id="page.cluster?.id" :pod="page.podDetail" />

        <div class="drawer-section">
          <strong>{{ page.t('k8sEvents') }}</strong>
          <el-table :data="page.podEvents" class="data-table">
            <el-table-column prop="type" :label="page.t('k8sType')" width="100" />
            <el-table-column prop="reason" :label="page.t('k8sReason')" width="140" />
            <el-table-column prop="message" :label="page.t('k8sMessage')" min-width="280" />
            <el-table-column prop="count" :label="page.t('k8sCount')" width="80" />
            <el-table-column prop="lastTime" :label="page.t('k8sLastTime')" width="160" />
          </el-table>
        </div>
      </template>
    </div>
  </el-drawer>

  <el-drawer v-model="page.podLogDrawerVisible" title="Pod 日志" size="70%" class="pod-log-drawer">
    <div v-loading="page.podLogLoading" class="drawer-content pod-log-drawer-content">
      <div class="pod-log-overview">
        <div>
          <span>Pod</span>
          <strong>{{ page.currentPodQuery.podName || '-' }}</strong>
        </div>
        <div>
          <span>命名空间</span>
          <strong>{{ page.currentPodQuery.namespace || '-' }}</strong>
        </div>
      </div>
      <div class="pod-log-toolbar">
        <el-select v-model="page.selectedContainer" class="pod-log-container-select" placeholder="选择容器" @change="page.refreshPodLogs">
          <el-option v-for="item in page.podContainerOptions" :key="item" :label="item" :value="item" />
        </el-select>
        <el-select v-model="page.podLogTailLines" class="pod-log-tail-select" @change="page.refreshPodLogs">
          <el-option :value="100" label="最近 100 条" />
          <el-option :value="200" label="最近 200 条" />
          <el-option :value="500" label="最近 500 条" />
          <el-option :value="1000" label="最近 1000 条" />
        </el-select>
        <el-button :loading="page.podLogLoading" @click="page.refreshPodLogs">刷新日志</el-button>
      </div>
      <div class="pod-log-meta">显示容器标准输出 / 错误输出，默认最近 200 条。</div>
      <div class="pod-log-console">
        <template v-if="page.podLogLines.length">
          <div v-for="(line, index) in page.podLogLines" :key="`${index}-${line}`" class="pod-log-line">
            <span class="pod-log-line-number">{{ String(index + 1).padStart(3, '0') }}</span>
            <span class="pod-log-line-content">{{ line }}</span>
          </div>
        </template>
        <div v-else class="pod-log-empty">{{ page.t('k8sNoLogsAvailable') }}</div>
      </div>
    </div>
  </el-drawer>
</template>
