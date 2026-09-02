<script setup>
import { ArrowDown, Connection, Grid, Histogram, Monitor, Promotion, Refresh, SetUp } from '@element-plus/icons-vue'

defineProps({
  page: {
    type: Object,
    required: true
  }
})

const iconMap = {
  overview: Histogram,
  nodes: Monitor,
  namespaces: Grid,
  workloads: SetUp,
  pods: Promotion,
  services: Connection,
  ingresses: Connection,
  'advanced-network': Connection,
  'config-storage': Grid
}
</script>

<template>
  <div class="kuboard-shell">
    <section class="kuboard-main">
      <header class="kuboard-main-head">
        <div class="kuboard-head-main">
          <div class="kuboard-head-title">
            <h2>{{ page.currentSection.title }}</h2>
            <p>{{ page.currentSection.description }}</p>
          </div>

          <div class="kuboard-head-tools">
            <el-select
              :model-value="page.cluster?.id"
              :placeholder="page.t('k8sSelectCluster')"
              filterable
              class="kuboard-cluster-select"
              :disabled="!page.clusterOptions.length"
              :loading="page.switching"
              @update:model-value="page.handleClusterChange"
            >
              <el-option v-for="item in page.clusterOptions" :key="item.id" :label="item.name" :value="item.id" />
            </el-select>

            <div v-if="page.cluster" class="kuboard-head-chips">
              <span class="kuboard-chip">
                <em>{{ page.t('k8sApiServer') }}</em>
                <b>{{ page.cluster.apiServer }}</b>
              </span>
              <span class="kuboard-chip">
                <em>{{ page.t('k8sVersion') }}</em>
                <b>{{ page.cluster.version }}</b>
              </span>
              <span class="kuboard-chip">
                <em>{{ page.t('k8sNodeCount') }}</em>
                <b>{{ page.t('k8sNodeCountTotal', { count: page.cluster.nodeCount }) }}</b>
              </span>
            </div>
          </div>
        </div>

        <div
          v-if="page.hasCluster && (page.shouldShowNamespaceFilter(page.currentTab) || page.currentTab === 'namespaces' || page.currentTab === 'workloads')"
          class="kuboard-global-toolbar"
          :class="{ 'is-actions-only': page.currentTab === 'namespaces' }"
        >
          <template v-if="page.shouldShowNamespaceFilter(page.currentTab)">
            <el-select
            :model-value="page.namespaceFilter"
            filterable
            class="namespace-select"
            :placeholder="page.t('k8sAllNamespaces')"
            @update:model-value="page.handleNamespaceFilterChange"
          >
            <el-option
              v-for="item in page.namespaceOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
            </el-select>
            <el-select
              v-if="page.currentTab === 'pods'"
              :model-value="page.podWorkloadFilter"
              filterable
              class="workload-select"
              placeholder="全部工作负载"
              @update:model-value="page.handlePodWorkloadFilterChange"
            >
              <el-option
                v-for="item in page.podWorkloadOptions"
                :key="item.value"
                :label="item.label"
                :value="item.value"
              />
            </el-select>
            <el-input
            :model-value="page.resourceKeyword"
            clearable
            :placeholder="page.t('k8sSearchResources')"
            class="resource-search"
            @update:model-value="page.handleResourceKeywordChange"
            />
            <div class="inline-scope-card">
            <span>{{ page.t('k8sCurrentScope') }}</span>
            <strong>
              {{ page.namespaceFilter === '__all__' ? page.t('k8sAllNamespaces') : page.namespaceFilter }}
            </strong>
            </div>
          </template>
          <el-input
            v-if="page.currentTab === 'namespaces'"
            :model-value="page.namespaceKeyword"
            clearable
            :placeholder="page.t('k8sSearchResources')"
            class="resource-search namespace-resource-search"
            @update:model-value="page.handleNamespaceKeywordChange"
          />
          <el-button
            v-if="['namespaces', 'pods', 'workloads', 'services', 'ingresses', 'advanced-network', 'config-storage'].includes(page.currentTab)"
            class="pod-list-refresh"
            plain
            :icon="Refresh"
            :loading="page.loading"
            @click="page.refreshCurrentClusterData"
          >
            {{ page.t('k8sRefresh') }}
          </el-button>
          <el-dropdown v-if="page.currentTab === 'workloads'" trigger="click" @command="page.handleWorkloadBatchCommand">
            <el-button class="workload-batch-trigger" :disabled="!page.workloadSelectionCount">
              批量操作<span v-if="page.workloadSelectionCount">（{{ page.workloadSelectionCount }}）</span><el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="images">批量更新镜像版本</el-dropdown-item>
                <el-dropdown-item command="scale">批量伸缩</el-dropdown-item>
                <el-dropdown-item command="restart">批量重启</el-dropdown-item>
                <el-dropdown-item command="delete" divided class="workload-batch-delete">批量删除</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button v-if="page.currentTab === 'workloads'" type="primary" @click="page.openWorkloadCreate">
            新增工作负载
          </el-button>
          <template v-if="page.currentTab === 'services'"><el-button @click="page.openServiceCreate">YAML 创建资源</el-button><el-button type="primary" @click="page.openServiceFormCreate">新增服务</el-button></template>
          <template v-if="page.currentTab === 'ingresses'"><el-button @click="page.openIngressCreate">YAML 创建资源</el-button><el-button type="primary" @click="page.openIngressFormCreate">新增 Ingress</el-button></template>
          <el-button v-if="page.currentTab === 'namespaces'" type="primary" @click="page.openNamespaceCreate">
            {{ page.t('k8sCreateNamespace') }}
          </el-button>
        </div>
      </header>

      <main class="kuboard-workspace">
        <slot />
      </main>
    </section>
  </div>
</template>
