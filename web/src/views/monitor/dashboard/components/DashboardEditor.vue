<script setup>
import { useDashboardDomain } from '../context'
import { CircleCheck } from '@element-plus/icons-vue'

const { dashboardDialogVisible, editingDashboard, dashboardForm, availableTemplates, activeTemplate, selectDashboardTemplate, currentTemplate, submitDashboard } = useDashboardDomain('dashboardEditor')
const { isListLayout, pageTitle, panels } = useDashboardDomain('selection')
const { isDark } = useDashboardDomain('theme')
</script>

<template>
    <el-dialog class="monitor-dialog" :class="{ 'is-dark': isDark }" v-model="dashboardDialogVisible" :title="editingDashboard ? (isListLayout ? '编辑巡检方案' : `编辑${pageTitle}`) : (isListLayout ? '新建巡检方案' : `创建${pageTitle}`)" width="760px">
      <el-form label-width="100px">
        <el-form-item label="名称" required><el-input v-model="dashboardForm.name" :placeholder="`例如：生产环境${pageTitle}`" /></el-form-item>
        <el-form-item v-if="!editingDashboard" :label="isListLayout ? '方案模板' : '大屏模板'">
          <div class="template-picker">
            <div class="template-grid" role="radiogroup" :aria-label="isListLayout ? '巡检方案模板' : '大屏模板'">
            <button v-for="item in availableTemplates" :key="item.key" type="button" class="template-card" :class="{ active: activeTemplate === item.key }" role="radio" :aria-checked="activeTemplate === item.key" @click="selectDashboardTemplate(item.key)">
              <span v-if="activeTemplate === item.key" class="template-selected"><el-icon><CircleCheck /></el-icon>已选择</span>
              <strong>{{ item.name }}</strong>
              <span>{{ item.description }}</span>
            </button>
            </div>
            <div class="template-selection-hint" aria-live="polite">当前选择：<strong>{{ currentTemplate.name }}</strong><span>{{ currentTemplate.panels.length ? `，创建后将生成 ${currentTemplate.panels.length} 个默认${isListLayout ? '巡检项' : '面板'}` : `，创建后可手动新增 PromQL ${isListLayout ? '巡检项' : '面板'}` }}</span></div>
          </div>
        </el-form-item>
        <el-form-item label="类型">
          <el-tag type="primary" effect="light">{{ isListLayout ? '巡检方案 / 列表巡检' : '监控大屏 / 网格展示' }}</el-tag>
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="dashboardForm.status">
            <el-radio :value="1">启用</el-radio>
            <el-radio :value="2">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="dashboardForm.description" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dashboardDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitDashboard">{{ editingDashboard ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>
</template>

<style scoped>
.template-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  width: 100%;
}
.template-picker {
  width: 100%;
}
.template-card {
  position: relative;
  min-height: 118px;
  padding: 14px;
  border: 1px solid var(--dash-border);
  border-radius: 12px;
  background: var(--dash-subtle);
  color: var(--dash-muted);
  text-align: left;
  cursor: pointer;
  transition: border-color .16s ease, background-color .16s ease, box-shadow .16s ease, transform .16s ease;
}
.template-card:hover {
  border-color: #8cb7ff;
  background: var(--dash-subtle);
}
.template-card:focus-visible {
  outline: 3px solid rgba(59, 130, 246, .25);
  outline-offset: 2px;
}
.template-card strong,
.template-card span {
  display: block;
}
.template-card span {
  margin-top: 8px;
  color: var(--dash-muted);
  line-height: 1.5;
}
.template-card.active {
  border: 2px solid #3b82f6;
  background: linear-gradient(145deg, var(--dash-subtle), var(--dash-subtle));
  box-shadow: 0 8px 20px rgba(59, 130, 246, .16), inset 0 0 0 1px rgba(255, 255, 255, .75);
  transform: translateY(-1px);
}
.template-selected {
  position: absolute;
  top: 10px;
  right: 10px;
  display: inline-flex !important;
  align-items: center;
  gap: 3px;
  margin: 0 !important;
  padding: 3px 7px;
  border-radius: 999px;
  background: #3b82f6;
  color: var(--dash-muted) !important;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
}
.template-selected .el-icon {
  font-size: 12px;
}
.template-selection-hint {
  margin-top: 10px;
  padding: 9px 11px;
  border: 1px solid var(--dash-border);
  border-radius: 8px;
  background: var(--dash-subtle);
  color: var(--dash-muted);
  font-size: 13px;
}
.template-selection-hint strong {
  color: var(--dash-accent);
}
</style>
