<script setup>
import { tr } from '../../../utils/i18n'
import { computed, onMounted, ref } from 'vue'
import { queryAssetDatabaseList } from '../../../api/asset'

const props = defineProps({
  activeId: { type: Number, default: 0 }
})

const emit = defineEmits(['select'])
const loading = ref(false)
const keyword = ref('')
const databases = ref([])

const treeData = computed(() => {
  const groups = new Map()
  const search = keyword.value.trim().toLowerCase()
  for (const item of databases.value) {
    const content = `${item.name} ${item.host} ${item.dbName || ''} ${item.env || ''}`.toLowerCase()
    if (search && !content.includes(search)) continue
    const env = item.env || tr("未分配环境")
    if (!groups.has(env)) groups.set(env, [])
    groups.get(env).push({
      id: `database:${item.id}`,
      label: item.name,
      database: item,
      isDatabase: true
    })
  }
  return Array.from(groups.entries()).map(([env, children]) => ({
    id: `environment:${env}`,
    label: env,
    children
  }))
})

async function loadData() {
  loading.value = true
  try {
    const data = await queryAssetDatabaseList({ pageNum: 1, pageSize: 500, status: '1' })
    databases.value = data.list || []
  } finally {
    loading.value = false
  }
}

function selectNode(data) {
  if (data.isDatabase) emit('select', data.database)
}

onMounted(loadData)
defineExpose({ refresh: loadData })
</script>

<template>
  <div class="connection-tree">
    <div class="connection-tree-toolbar">
      <el-input v-model="keyword" clearable :placeholder="tr('搜索数据库连接')" />
      <el-button :title="tr('刷新连接')" @click="loadData">{{ tr("刷新") }}</el-button>
    </div>
    <el-tree
      v-loading="loading"
      :data="treeData"
      node-key="id"
      default-expand-all
      :expand-on-click-node="false"
      @node-click="selectNode"
    >
      <template #default="{ data }">
        <div class="connection-node" :class="{ active: data.database?.id === activeId, disabled: data.database?.connectStatus !== 1 }">
          <template v-if="data.isDatabase">
            <span class="status-dot" :class="{ online: data.database.connectStatus === 1 }" />
            <div class="connection-label">
              <strong>{{ data.label }}</strong>
              <small>{{ data.database.dbType?.toUpperCase() || 'MYSQL' }} · {{ data.database.dbName || data.database.host }} · {{ data.database.accessMode === 'readonly' ? tr("只读") : tr("读写") }}</small>
            </div>
          </template>
          <template v-else>
            <strong class="environment-label">{{ data.label }}</strong>
            <small>{{ data.children?.length || 0 }}</small>
          </template>
        </div>
      </template>
    </el-tree>
    <el-empty v-if="!loading && !treeData.length" :description="tr('暂无数据库连接')" :image-size="58" />
  </div>
</template>

<style scoped>
.connection-tree { display: flex; flex-direction: column; gap: 10px; min-height: 0; }
.connection-tree-toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; }
.connection-node { display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0; padding: 5px 8px; border-radius: 5px; }
.connection-node.active { background: #e8f0ff; color: #255fc4; }
.connection-node.disabled { opacity: 0.55; }
.connection-label { display: flex; flex: 1; flex-direction: column; gap: 2px; min-width: 0; }
.connection-label strong, .connection-label small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.connection-label small { color: #8290a7; font-size: 11px; }
.status-dot { width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; background: #ef4444; }
.status-dot.online { background: #22c55e; }
.environment-label { flex: 1; color: #42526d; }
.connection-node > small { color: #8b98ad; }
:deep(.el-tree-node__content) { min-height: 42px; height: auto; border-radius: 5px; }
:deep(.el-tree-node__content:hover) { background: #f3f6fb; }
</style>
