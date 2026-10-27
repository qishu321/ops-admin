<script setup>
import { useMonitorDashboard } from './composables/useMonitorDashboard'
import DashboardWorkspace from './components/DashboardWorkspace.vue'
import DashboardToolbar from './components/DashboardToolbar.vue'
import DashboardSummary from './components/DashboardSummary.vue'
import DashboardGrid from './components/DashboardGrid.vue'
import InspectionView from './components/InspectionView.vue'
import InspectionToolbar from './components/inspection/InspectionToolbar.vue'
import DashboardEditor from './components/DashboardEditor.vue'
import PanelFocusDialog from './components/PanelFocusDialog.vue'
import PanelEditor from './components/PanelEditor.vue'

const { loadError, retryLoad, loading, isListLayout, isK8sDashboard, activeDashboard, panels, isDark, isFullscreen } = useMonitorDashboard()
</script>

<template>
  <div class="dashboard-page" :class="{ 'is-dark': isDark, 'is-inspection-page': isListLayout }">
    <DashboardWorkspace v-if="!isListLayout || !isFullscreen" />
    <main class="dashboard-main observability-canvas" :class="{ 'is-k8s-dashboard': isK8sDashboard, 'is-presentation': isFullscreen }" v-loading="loading">
      <InspectionToolbar v-if="isListLayout && !isFullscreen" />
      <DashboardToolbar v-else-if="!isListLayout" />
      <DashboardSummary />
      <el-alert v-if="loadError" :title="isListLayout ? '巡检方案加载失败' : '监控大屏加载失败'" type="error" :closable="false" show-icon>
        <p>{{ loadError }}</p>
        <el-button :loading="loading" @click="retryLoad">重新加载</el-button>
      </el-alert>
      <el-empty v-else-if="!activeDashboard" :description="isListLayout ? '还没有巡检方案，请先新建一个' : '还没有监控大屏，请先创建一个'" />
      <el-empty v-else-if="!panels.length" :description="isListLayout ? '当前方案还没有巡检项，请先新增巡检项' : '当前大屏还没有面板，可以新增面板或使用模板创建'" />
      <InspectionView v-else-if="isListLayout" />
      <DashboardGrid v-else />
    </main>
    <DashboardEditor />
    <PanelEditor />
    <PanelFocusDialog />
  </div>
</template>

<style scoped src="./styles/workspace.css"></style>
<style src="./styles/theme.css"></style>
<style src="./styles/inspection.css"></style>
