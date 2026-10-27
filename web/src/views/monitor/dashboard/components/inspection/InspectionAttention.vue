<script setup>
import { computed, ref } from 'vue'
import { CircleCheck, Warning, ArrowRight } from '@element-plus/icons-vue'
import { useDashboardDomain } from '../../context'

const props = defineProps({ rows: { type: Array, required: true } })
defineEmits(['inspect'])
const { inspectionStateKey, inspectionDisplayValue } = useDashboardDomain('inspection')
const expanded = ref(false)
const visible = computed(() => expanded.value ? props.rows : props.rows.slice(0, 4))
</script>

<template>
  <section class="attention-section" aria-label="需要关注的巡检结果">
    <div v-if="!rows.length" class="attention-clear"><el-icon><CircleCheck /></el-icon>当前范围没有异常或待核查项</div>
    <template v-else>
      <div class="attention-heading"><span><el-icon><Warning /></el-icon>需要关注 <b>{{ rows.length }}</b></span><small>点击查看判定依据</small></div>
      <div class="attention-items">
        <button v-for="row in visible" :key="row.rowKey" class="attention-item" :class="`attention-${inspectionStateKey(row)}`"
          :aria-label="`查看关注项 ${row.title} · ${row.datasourceName}`" @click="$emit('inspect', row)">
          <i class="attention-mark"></i><div><strong>{{ row.title }}</strong><span>{{ row.datasourceName }}</span></div>
          <b>{{ inspectionDisplayValue(row) }}</b><el-icon><ArrowRight /></el-icon>
        </button>
      </div>
      <button v-if="rows.length > 4" class="attention-expand" @click="expanded = !expanded">{{ expanded ? '收起关注项' : `查看全部 ${rows.length} 项` }}</button>
    </template>
  </section>
</template>

<style scoped>
.attention-section { margin: 0 24px 18px; }
.attention-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 9px; }
.attention-heading > span { display: flex; align-items: center; gap: 7px; font-size: 12px; font-weight: 600; }
.attention-heading .el-icon { color: var(--inspect-warning); }
.attention-heading b { font-weight: 500; color: var(--dash-muted); }
.attention-heading small { color: var(--dash-muted); font-size: 11px; }
.attention-items { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 9px; }
.attention-item { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 11px 13px; border: 1px solid var(--dash-border); border-radius: 8px; background: var(--inspect-cell-warning); color: var(--dash-text); text-align: left; cursor: pointer; }
.attention-item:hover { border-color: var(--state-color); }
.attention-warning { --state-color: var(--inspect-warning); }
.attention-danger { --state-color: var(--inspect-danger); background: var(--inspect-cell-danger); }
.attention-mark { width: 4px; height: 26px; flex-shrink: 0; border-radius: 2px; background: var(--state-color); }
.attention-item > div { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 4px; }
.attention-item strong, .attention-item span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attention-item strong { font-size: 12px; font-weight: 600; }
.attention-item span { color: var(--dash-muted); font-size: 11px; }
.attention-item > b { max-width: 35%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--state-color); font-size: 17px; font-weight: 600; }
.attention-item > .el-icon { color: var(--dash-muted); font-size: 11px; }
.attention-clear { display: flex; align-items: center; gap: 8px; padding: 11px 13px; border-radius: 8px; background: var(--dash-subtle); color: var(--dash-muted); font-size: 12px; }
.attention-clear .el-icon { color: var(--inspect-healthy); }
.attention-expand { display: block; margin: 9px auto 0; padding: 5px 10px; border: 0; border-radius: 5px; background: var(--dash-subtle); color: var(--dash-accent); font-size: 11px; cursor: pointer; }
@media (max-width: 760px) { .attention-section { margin: 0 16px 16px; } .attention-items { grid-template-columns: minmax(0, 1fr); } }
</style>
