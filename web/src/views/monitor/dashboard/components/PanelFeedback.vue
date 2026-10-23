<script setup>
import { DataLine, Warning } from '@element-plus/icons-vue'
defineProps({ loading: Boolean, error: String, disabled: Boolean })
defineEmits(['retry'])
</script>

<template>
  <div v-if="loading" class="panel-skeleton" role="status" aria-label="正在加载面板">
    <span class="skeleton-value"></span>
    <div class="skeleton-plot"><i v-for="height in [42, 65, 48, 80, 62, 90, 74]" :key="height" :style="{ height: `${height}%` }"></i></div>
    <span class="skeleton-caption"></span>
  </div>
  <div v-else class="panel-feedback" :class="{ 'has-error': error }" role="status">
    <el-icon><Warning v-if="error" /><DataLine v-else /></el-icon>
    <strong>{{ error ? '面板查询失败' : disabled ? '面板已停用' : '当前时间范围内暂无数据' }}</strong>
    <p>{{ error || (disabled ? '编辑面板以重新启用查询' : '尝试调整时间范围，或检查数据源与 PromQL') }}</p>
    <el-button v-if="error && !disabled" size="small" @click="$emit('retry')">重试查询</el-button>
  </div>
</template>

<style scoped>
.panel-feedback, .panel-skeleton { min-height: 168px; padding: 24px; box-sizing: border-box; }
.panel-feedback { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; color: var(--dash-muted); text-align: center; }
.panel-feedback .el-icon { font-size: 25px; color: var(--dash-accent); }
.panel-feedback strong { color: var(--dash-text); font-size: 13px; }
.panel-feedback p { margin: 0; max-width: 100%; font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
.panel-feedback.has-error .el-icon, .has-error strong { color: var(--dash-danger); }
.panel-skeleton { display: flex; flex-direction: column; gap: 14px; }
.panel-skeleton span, .skeleton-plot i { border-radius: 4px; background: var(--dash-skeleton); animation: pulse 1.5s ease-in-out infinite alternate; }
.skeleton-value { width: 32%; height: 20px; }
.skeleton-caption { width: 60%; height: 8px; }
.skeleton-plot { display: flex; align-items: flex-end; gap: 7px; height: 75px; border-bottom: 1px solid var(--dash-border); }
.skeleton-plot i { flex: 1; opacity: .7; }
@keyframes pulse { to { opacity: .35; } }
@media (prefers-reduced-motion: reduce) { .panel-skeleton span, .skeleton-plot i { animation: none; } }
</style>
