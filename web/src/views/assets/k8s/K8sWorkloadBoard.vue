<script setup>
function workloadReady(row) {
  const [ready, desired] = String(row.ready || '').split('/').map((item) => Number(item))
  return Number.isFinite(ready) && Number.isFinite(desired) && desired > 0 && ready >= desired
}

function primaryImage(images) {
  return String(images || '').split('\n').map((item) => item.trim()).filter(Boolean)[0] || ''
}

defineProps({
  page: {
    type: Object,
    required: true
  }
})
</script>

<template>
  <section class="kuboard-section workload-workspace">
    <div class="workload-type-panel">
      <div class="workload-type-row">
        <span>工作负载类型</span>
        <div class="kuboard-type-tabs">
          <button
            v-for="item in page.workloadTypeOptions"
            :key="item.value"
            type="button"
            class="kuboard-type-tab"
            :class="{ active: page.workloadTypeFilter === item.value }"
            @click="page.handleWorkloadTypeChange(item.value)"
          >
            <span>{{ item.label }}</span>
            <em>{{ item.count }}</em>
          </button>
        </div>
      </div>
    </div>

    <div class="kuboard-table-card">
      <el-table
        v-if="page.hasItems(page.kuboardWorkloadRows)"
        :data="page.kuboardWorkloadRows"
        class="kuboard-table"
        @selection-change="page.handleWorkloadSelectionChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column :label="page.t('k8sName')" min-width="220">
          <template #default="{ row }">
            <div class="kuboard-name-cell">
              <button type="button" class="kuboard-link-button" @click="page.openWorkloadPods(row)">
                {{ row.name }}
              </button>
              <div class="workload-name-meta"><el-tag size="small" effect="plain" class="workload-kind-tag">{{ row.type }}</el-tag><small>{{ row.namespace }}</small></div>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="page.t('k8sReady')" width="108" align="center">
          <template #default="{ row }"><el-tag :type="workloadReady(row) ? 'success' : 'warning'" effect="light" round>{{ row.ready || '—' }}</el-tag></template>
        </el-table-column>
        <el-table-column label="副本" width="130">
          <template #default="{ row }"><div class="workload-replica-cell"><span>已更新 <b>{{ row.updated ?? '—' }}</b></span><span>可用 <b>{{ row.available ?? '—' }}</b></span></div></template>
        </el-table-column>
        <el-table-column label="资源规格" min-width="180">
          <template #default="{ row }"><div class="workload-resource-cell"><span>Req <b>{{ row.requests || '—' }}</b></span><span>Lim <b>{{ row.limits || '—' }}</b></span></div></template>
        </el-table-column>
        <el-table-column prop="age" :label="page.t('k8sAge')" width="100" />
        <el-table-column :label="page.t('k8sImages')" min-width="270">
          <template #default="{ row }">
            <el-tooltip :content="row.images || page.t('k8sNoImageData')" placement="top" :show-after="300">
              <div class="workload-image-cell"><code>{{ primaryImage(row.images) || page.t('k8sNoImageData') }}</code><small v-if="String(row.images || '').split('\n').filter(Boolean).length > 1">+{{ String(row.images || '').split('\n').filter(Boolean).length - 1 }} 个容器镜像</small></div>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="page.t('k8sActions')" min-width="350" fixed="right">
          <template #default="{ row }">
            <div class="kuboard-actions workload-actions">
              <el-button link type="primary" class="workload-settings-action" @click="page.openWorkloadResourceSettings(row)">编辑工作负载</el-button>
              <el-button link type="primary" @click="page.openWorkloadDetail(row)">{{ page.t('k8sDetail') }}</el-button>
              <el-button link type="primary" @click="page.openWorkloadYAML(row)">{{ page.t('k8sYaml') }}</el-button>
              <el-button v-if="page.supportsScale(row)" link type="primary" @click="page.openScaleDialog(row)">{{ page.t('k8sScale') }}</el-button>
              <el-button v-if="page.supportsRestart(row)" link type="warning" @click="page.handleRestartWorkload(row)">{{ page.t('k8sRestart') }}</el-button>
              <el-button link type="danger" @click="page.handleDeleteWorkload(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else :description="page.t('k8sNoRealtimeWorkloadData')" />
    </div>
  </section>
</template>
