<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ResourceAccessCenter from './ResourceAccessCenter.vue'
import Role from './Role.vue'
const route = useRoute(), router = useRouter()
const tab = computed({ get: () => route.query.tab === 'templates' ? 'templates' : 'policies', set: value => router.replace({ query: { ...route.query, tab: value } }) })
</script>

<template>
  <div class="policy-workspace">
    <el-tabs v-model="tab" class="policy-tabs">
      <el-tab-pane name="policies" label="访问策略" lazy><ResourceAccessCenter /></el-tab-pane>
      <el-tab-pane name="templates" label="权限模板" lazy><Role /></el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.policy-tabs :deep(> .el-tabs__header) { margin-bottom:20px; }
.policy-tabs :deep(> .el-tabs__header .el-tabs__item) { font-size:14px; height:46px; }
</style>
