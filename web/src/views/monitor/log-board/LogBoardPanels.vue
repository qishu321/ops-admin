<script setup>
import { computed, ref } from 'vue'
import {
  LOG_BOARD_LEVEL_TONES,
  formatLogBoardNumber,
  formatLogBoardStep,
  logBoardBarHeight,
  logBoardBarWidth,
  logBoardClockLabel,
  logBoardDonutGradient,
  logBoardFieldLabel,
  logBoardLevelClass,
  logBoardLevelCount,
  logBoardLevels,
  logBoardShortMessage
} from './logBoardShared'

// 面板本身只负责展示：内部大屏和公开分享页共用同一套布局，
// 差异只有「能不能下钻」和「能不能看详情」。
const props = defineProps({
  data: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  rangeLabel: { type: String, default: '' },
  interactive: { type: Boolean, default: true },
  allowDetail: { type: Boolean, default: true },
  emptyHint: { type: String, default: '当前过滤条件下没有日志' }
})

const emit = defineEmits(['drill'])

const hoverIndex = ref(-1)
const detail = ref(null)
const detailVisible = ref(false)

const buckets = computed(() => props.data?.buckets || [])
const maxTotal = computed(() => Math.max(1, ...buckets.value.map(item => item.total || 0)))
const levels = computed(() => logBoardLevels(props.data))
const errorCount = computed(() => logBoardLevelCount(props.data, ['ERROR', 'FATAL']))
const warnCount = computed(() => logBoardLevelCount(props.data, ['WARN']))
const facets = computed(() => props.data?.facets || [])
const errorTypes = computed(() => props.data?.errorTypes || [])
const patterns = computed(() => props.data?.patterns || [])
const recent = computed(() => props.data?.recent || [])
const recentLimit = computed(() => Number(props.data?.recentLimit || 50))
const recentTotal = computed(() => Number(props.data?.recentTotal || recent.value.length))
// 最近日志只展示 ERROR/FATAL：值班排查时 INFO 会稀释信号，
// 条数上限由后端 recentLimit 决定（默认 50）。
const recentHint = computed(() => {
  if (!recent.value.length) return '所选时间范围内没有 ERROR 日志'
  const base = `最新 ${recent.value.length} 条 ERROR`
  if (recentTotal.value > recent.value.length) {
    return `${base} · 采样内共 ${formatLogBoardNumber(recentTotal.value)} 条，仅展示前 ${recentLimit.value} 条`
  }
  return base
})
const donutGradient = computed(() => logBoardDonutGradient(levels.value))
const stepLabel = computed(() => formatLogBoardStep(props.data?.step))
const truncated = computed(() => Boolean(props.data?.truncated))
const rangeSeconds = computed(() => Math.max(1, (Number(props.data?.endAt || 0) - Number(props.data?.startAt || 0)) / 1000))
const ratePerMinute = computed(() => ((props.data?.total || 0) / Math.max(1, rangeSeconds.value / 60)).toFixed(1))

function barHeight(value) {
  return logBoardBarHeight(value, maxTotal.value)
}

function drill(key, value) {
  if (!props.interactive) return
  emit('drill', { key, value })
}

function openDetail(item) {
  if (!props.allowDetail) return
  detail.value = item
  detailVisible.value = true
}
</script>

<template>
  <div class="log-board-panels">
    <template v-if="data">
      <section class="kpi-row">
        <article class="kpi-card">
          <span class="kpi-label">日志总量</span>
          <strong class="kpi-value">{{ formatLogBoardNumber(data.total) }}</strong>
          <small class="kpi-hint">{{ rangeLabel }} · 耗时 {{ data.tookMs }} ms</small>
        </article>
        <article class="kpi-card is-error">
          <span class="kpi-label">错误日志</span>
          <strong class="kpi-value">{{ formatLogBoardNumber(errorCount) }}</strong>
          <small class="kpi-hint">占比 {{ data.total ? ((errorCount / data.total) * 100).toFixed(1) : '0.0' }}%</small>
        </article>
        <article class="kpi-card is-warn">
          <span class="kpi-label">告警日志</span>
          <strong class="kpi-value">{{ formatLogBoardNumber(warnCount) }}</strong>
          <small class="kpi-hint">WARN 及以上共 {{ formatLogBoardNumber(errorCount + warnCount) }} 条</small>
        </article>
        <article class="kpi-card">
          <span class="kpi-label">涉及 Pod</span>
          <strong class="kpi-value">{{ data.podCount }}</strong>
          <small class="kpi-hint">{{ data.nsCount }} 个命名空间</small>
        </article>
        <article class="kpi-card">
          <span class="kpi-label">分析样本</span>
          <strong class="kpi-value">{{ formatLogBoardNumber(data.sampled) }}</strong>
          <small class="kpi-hint">{{ truncated ? `总量较大，仅分析最近 ${formatLogBoardNumber(data.limit)} 条` : '已覆盖全部日志' }}</small>
        </article>
        <article class="kpi-card">
          <span class="kpi-label">日志速率</span>
          <strong class="kpi-value">{{ ratePerMinute }}</strong>
          <small class="kpi-hint">条 / 分钟</small>
        </article>
      </section>

      <section class="board-row is-trend">
        <article class="board-card trend-card" v-loading="loading">
          <header class="card-head">
            <div>
              <h3>日志速率</h3>
              <p>按 {{ stepLabel }} 聚合 · 红色为错误日志</p>
            </div>
            <span class="card-meta">{{ formatLogBoardNumber(data.total) }} 条 / {{ formatLogBoardNumber(data.sampled) }} 样本</span>
          </header>
          <div class="trend-body" @mouseleave="hoverIndex = -1">
            <div class="trend-bars">
              <div
                v-for="(item, index) in buckets"
                :key="item.ts"
                class="trend-col"
                @mouseenter="hoverIndex = index"
              >
                <div class="trend-stack">
                  <i class="seg is-error" :style="{ height: barHeight(item.error) }" />
                  <i class="seg is-other" :style="{ height: barHeight(Math.max(0, item.total - item.error)) }" />
                </div>
              </div>
              <div v-if="hoverIndex >= 0 && buckets[hoverIndex]" class="trend-tip" :style="{ left: `${((hoverIndex + 0.5) / buckets.length) * 100}%` }">
                <b>{{ logBoardClockLabel(buckets[hoverIndex].ts) }}</b>
                <span>总计 {{ buckets[hoverIndex].total }}</span>
                <span>错误 {{ buckets[hoverIndex].error }}</span>
              </div>
            </div>
            <p v-if="!buckets.length" class="trend-empty">当前时间范围内没有日志</p>
            <div class="trend-axis">
              <span v-for="tick in [0, Math.floor(buckets.length / 4), Math.floor(buckets.length / 2), Math.floor((buckets.length * 3) / 4), buckets.length - 1]" :key="tick">
                {{ logBoardClockLabel(buckets[tick]?.ts) }}
              </span>
            </div>
          </div>
        </article>

        <article class="board-card level-card">
          <header class="card-head">
            <div>
              <h3>级别分布</h3>
              <p>按日志行首的级别标记</p>
            </div>
          </header>
          <div class="level-body">
            <div class="donut" :style="{ background: donutGradient }">
              <div class="donut-hole">
                <strong>{{ formatLogBoardNumber(data.total) }}</strong>
                <small>条日志</small>
              </div>
            </div>
            <ul class="legend-list">
              <li v-for="item in levels" :key="item.value">
                <span class="legend-dot" :style="{ background: LOG_BOARD_LEVEL_TONES[item.value] || LOG_BOARD_LEVEL_TONES.OTHER }" />
                <span class="legend-name">{{ item.value }}</span>
                <b>{{ formatLogBoardNumber(item.count) }}</b>
                <small>{{ item.ratio }}%</small>
              </li>
              <li v-if="!levels.length" class="legend-empty">当前范围内没有日志</li>
            </ul>
          </div>
        </article>
      </section>

      <section class="board-row is-insight">
        <article class="board-card">
          <header class="card-head">
            <div>
              <h3>错误类型 Top</h3>
              <p>{{ interactive ? '点击可追加过滤条件继续下钻' : '按出现次数排序' }}</p>
            </div>
          </header>
          <ul class="rank-list">
            <li
              v-for="item in errorTypes"
              :key="item.name"
              :class="['rank-item', { 'is-static': !interactive }]"
              :title="interactive ? `按 ${item.name} 过滤` : item.name"
              @click="drill(item.field, item.name)"
            >
              <div class="rank-line">
                <span class="rank-name" :title="item.name">{{ item.name }}</span>
                <b>{{ formatLogBoardNumber(item.count) }}</b>
              </div>
              <div class="rank-bar"><i :style="{ width: logBoardBarWidth(item.count, errorTypes[0]?.count) }" /></div>
            </li>
            <li v-if="!errorTypes.length" class="rank-empty">该范围内没有错误日志</li>
          </ul>
        </article>

        <article class="board-card">
          <header class="card-head">
            <div>
              <h3>日志模式 Top</h3>
              <p>数字 / ID / IP 已折叠为占位符</p>
            </div>
          </header>
          <ul class="rank-list">
            <li v-for="item in patterns" :key="item.pattern" class="rank-item is-compact">
              <div class="rank-line">
                <span class="rank-name is-mono" :title="item.pattern">{{ item.pattern }}</span>
                <b>{{ formatLogBoardNumber(item.count) }}</b>
              </div>
              <div class="rank-bar is-muted"><i :style="{ width: logBoardBarWidth(item.count, patterns[0]?.count) }" /></div>
            </li>
            <li v-if="!patterns.length" class="rank-empty">没有可统计的日志模式</li>
          </ul>
        </article>
      </section>

      <section v-if="facets.length" class="facet-grid">
        <article v-for="facet in facets" :key="facet.key" class="board-card facet-card">
          <header class="card-head">
            <div>
              <h3>{{ logBoardFieldLabel(facet.key) }}</h3>
              <p>{{ facet.values.length }} 个取值 · 覆盖 {{ formatLogBoardNumber(facet.coverage) }} 条</p>
            </div>
          </header>
          <ul class="facet-list">
            <li
              v-for="value in facet.values"
              :key="value.value"
              :class="['facet-item', { 'is-static': !interactive }]"
              :title="interactive ? `按 ${logBoardFieldLabel(facet.key)}=${value.value} 过滤` : value.value"
              @click="drill(facet.key, value.value)"
            >
              <span class="facet-name" :title="value.value">{{ value.value }}</span>
              <span class="facet-bar"><i :style="{ width: logBoardBarWidth(value.count, facet.values[0]?.count) }" /></span>
              <b>{{ formatLogBoardNumber(value.count) }}</b>
            </li>
          </ul>
        </article>
      </section>

      <section class="board-card recent-card">
        <header class="card-head">
          <div>
            <h3>最近错误日志</h3>
            <p>{{ recentHint }} {{ allowDetail ? '· 点击查看完整正文与字段' : '' }}</p>
          </div>
          <span class="card-meta">{{ data.datasource?.name }}</span>
        </header>
        <div class="log-lines">
          <button v-for="(item, index) in recent" :key="index" type="button" :class="['log-line', { 'is-static': !allowDetail }]" @click="openDetail(item)">
            <span class="log-time">{{ String(item.time || '').slice(11) }}</span>
            <span :class="['log-level', logBoardLevelClass(item.level)]">{{ item.level || '-' }}</span>
            <span class="log-ns" :title="item.pod">{{ item.namespace || item.container || '-' }}</span>
            <code class="log-msg">{{ logBoardShortMessage(item) }}</code>
          </button>
          <p v-if="!recent.length" class="log-empty">{{ loading ? '正在加载错误日志…' : '所选时间范围内没有 ERROR 日志' }}</p>
        </div>
      </section>
    </template>

    <section v-else-if="!loading" class="board-empty">
      <p>{{ emptyHint }}</p>
    </section>
    <section v-if="loading && !data" class="board-empty" v-loading="true" />

    <el-dialog v-model="detailVisible" title="日志详情" width="720px" class="log-detail-dialog" append-to-body>
      <template v-if="detail">
        <div class="detail-meta">
          <span>{{ detail.time }}</span>
          <span :class="['log-level', logBoardLevelClass(detail.level)]">{{ detail.level }}</span>
          <span>{{ detail.namespace }}</span>
          <span>{{ detail.pod }}</span>
        </div>
        <div class="detail-tags">
          <span v-if="detail.exceptionType">异常类型：{{ detail.exceptionType }}</span>
          <span v-if="detail.platform">平台：{{ detail.platform }}</span>
          <span v-if="detail.appVersion">版本：{{ detail.appVersion }}</span>
          <span v-if="detail.deviceModel">设备：{{ detail.deviceModel }}</span>
        </div>
        <pre class="detail-message">{{ detail.message || detail.summary }}</pre>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.log-board-panels {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.kpi-row {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}
.kpi-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border: 1px solid #e0e8f4;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 6px 18px rgba(49, 78, 125, 0.05);
}
.kpi-label {
  color: #7c8da5;
  font-size: 12px;
}
.kpi-value {
  color: #14294a;
  font-size: 26px;
  line-height: 1.15;
}
.kpi-hint {
  overflow: hidden;
  color: #93a2b7;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.kpi-card.is-error .kpi-value {
  color: #d9485a;
}
.kpi-card.is-warn .kpi-value {
  color: #c8850d;
}
.board-row {
  display: grid;
  gap: 12px;
}
.board-row.is-trend {
  grid-template-columns: minmax(0, 1.62fr) minmax(0, 1fr);
}
.board-row.is-insight {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.board-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 14px 16px;
  border: 1px solid #e0e8f4;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 6px 18px rgba(49, 78, 125, 0.05);
}
.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.card-head h3 {
  margin: 0;
  color: #14294a;
  font-size: 15px;
}
.card-head p {
  margin: 4px 0 0;
  color: #8b9ab0;
  font-size: 12px;
}
.card-meta {
  flex: 0 0 auto;
  color: #8b9ab0;
  font-size: 12px;
}
.trend-body {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  height: 210px;
}
.trend-bars {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 2px;
  flex: 1 1 auto;
  min-height: 0;
  padding-top: 6px;
  border-bottom: 1px solid #eef2f8;
  background-image: linear-gradient(#f4f7fc 1px, transparent 1px);
  background-size: 100% 25%;
}
.trend-col {
  display: flex;
  flex: 1 1 0;
  align-items: flex-end;
  justify-content: center;
  height: 100%;
  min-width: 2px;
  cursor: crosshair;
}
.trend-stack {
  display: flex;
  flex-direction: column-reverse;
  justify-content: flex-start;
  width: 100%;
  max-width: 16px;
  height: 100%;
}
.seg {
  display: block;
  width: 100%;
  min-height: 0;
  border-radius: 2px 2px 0 0;
  transition: height 0.2s ease;
}
.seg.is-other {
  background: #9dbef0;
}
.seg.is-error {
  background: #ef5d65;
}
.trend-col:hover .seg.is-other {
  background: #6f9fe8;
}
.trend-tip {
  position: absolute;
  top: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 9px;
  border-radius: 8px;
  background: rgba(21, 42, 74, 0.92);
  color: #fff;
  font-size: 11px;
  pointer-events: none;
  transform: translateX(-50%);
  white-space: nowrap;
}
.trend-tip b {
  font-size: 12px;
}
.trend-empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  margin: 0;
  color: #9aa8bd;
  font-size: 12px;
}
.trend-axis {
  display: flex;
  justify-content: space-between;
  color: #93a2b7;
  font-size: 11px;
}
.level-body {
  display: flex;
  align-items: center;
  gap: 18px;
  flex: 1 1 auto;
}
.donut {
  position: relative;
  flex: 0 0 auto;
  width: 128px;
  height: 128px;
  border-radius: 50%;
}
.donut-hole {
  position: absolute;
  inset: 26px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #fff;
}
.donut-hole strong {
  color: #14294a;
  font-size: 16px;
}
.donut-hole small {
  color: #93a2b7;
  font-size: 11px;
}
.legend-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1 1 auto;
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}
.legend-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #4f6482;
  font-size: 12px;
}
.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.legend-name {
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.legend-list b {
  color: #22375a;
}
.legend-list small {
  width: 46px;
  color: #93a2b7;
  text-align: right;
}
.legend-empty,
.rank-empty,
.log-empty {
  color: #9aa8bd;
  font-size: 12px;
}
.rank-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
  max-height: 300px;
  overflow: auto;
}
.rank-item {
  cursor: pointer;
}
.rank-item.is-compact,
.rank-item.is-static {
  cursor: default;
}
.rank-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 4px;
}
.rank-name {
  overflow: hidden;
  color: #2b4162;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rank-name.is-mono {
  font-family: Consolas, Monaco, monospace;
}
.rank-line b {
  flex: 0 0 auto;
  color: #d9485a;
  font-size: 12px;
}
.rank-bar,
.facet-bar {
  display: block;
  height: 6px;
  border-radius: 999px;
  background: #eef2f9;
  overflow: hidden;
}
.rank-bar i {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #f2777f, #ef5d65);
}
.rank-bar.is-muted i {
  background: linear-gradient(90deg, #9fb6d8, #7f9cc4);
}
.facet-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(270px, 1fr));
  align-items: start;
  gap: 12px;
}
.facet-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.facet-item {
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(72px, 1fr) 44px;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.facet-item.is-static {
  cursor: default;
}
.facet-item:hover .facet-name {
  color: #1f5fd0;
}
.facet-item.is-static:hover .facet-name {
  color: #3d5474;
}
.facet-name {
  overflow: hidden;
  color: #3d5474;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.facet-bar {
  height: 5px;
}
.facet-bar i {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #8fb6ee, #4f8ee6);
}
.facet-item b {
  color: #22375a;
  font-size: 12px;
  text-align: right;
}
.log-lines {
  display: flex;
  flex-direction: column;
  gap: 2px;
  /* 50 条错误日志一屏放不下，固定高度 + 滚动，避免把整块大屏撑长。 */
  max-height: 460px;
  overflow: auto;
}
.log-line {
  display: grid;
  grid-template-columns: 78px 58px 118px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 7px 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}
.log-line.is-static {
  cursor: default;
}
.log-line:hover {
  background: #f3f7fd;
}
.log-time {
  color: #8b9ab0;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
}
.log-level {
  justify-self: start;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
}
.log-level.is-error {
  color: #c93b4b;
  background: #fdeff0;
}
.log-level.is-warn {
  color: #b57c0c;
  background: #fff6e6;
}
.log-level.is-info {
  color: #2f6cbf;
  background: #edf4ff;
}
.log-level.is-other {
  color: #6b7c93;
  background: #f1f4f9;
}
.log-ns {
  overflow: hidden;
  color: #5c7291;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.log-msg {
  overflow: hidden;
  color: #2b4162;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.board-empty {
  display: grid;
  place-items: center;
  min-height: 260px;
  border: 1px dashed #d6e2f3;
  border-radius: 14px;
  background: #fff;
  color: #93a2b7;
}
.detail-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: #5c7291;
  font-size: 12px;
}
.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 10px 0;
}
.detail-tags span {
  padding: 3px 8px;
  border-radius: 6px;
  background: #f2f6fc;
  color: #45608a;
  font-size: 12px;
}
.detail-message {
  max-height: 420px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: #101b2d;
  color: #d6e2f3;
  font: 12px/1.65 Consolas, Monaco, monospace;
  white-space: pre-wrap;
  word-break: break-word;
}
@media (max-width: 1400px) {
  .kpi-row {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .board-row.is-trend {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 900px) {
  .kpi-row,
  .board-row.is-insight {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
