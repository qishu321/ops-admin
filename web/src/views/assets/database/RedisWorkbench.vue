<script setup>
import { tr } from '../../../utils/i18n'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Coin, Refresh, Search, VideoPlay, Key, View, Delete, Edit } from '@element-plus/icons-vue'
import DatabaseConnectionTree from './DatabaseConnectionTree.vue'
import { analyzeRedisCommand, executeRedisCommand, queryDBMSSchemaTree, queryDBMSResourceData, queryDBMSSQLHistory, queryRedisKeys } from '../../../api/dbms'
import { confirmRiskOperation } from '../../../composables/useRiskConfirm'
import { buildRedisKeyCommands, normalizeRedisSchema, quoteRedisArgument } from '../../../utils/redis-workbench'
const props = defineProps({ databaseId: { type: Number, required: true }, connection: { type: Object, required: true } })
const emit = defineEmits(['switch-database'])
const paneRef = ref(null), paneWidth = ref(0), browserShare = ref(0.5), resizing = ref(false)
const paneLimits = computed(() => {
  const width = Math.max(1, paneWidth.value - 18)
  return { min: Math.min(0.5, 280 / width), max: Math.max(0.5, 1 - 320 / width) }
})
const renderedShare = computed(() => Math.max(paneLimits.value.min, Math.min(paneLimits.value.max, browserShare.value)))
const paneStyle = computed(() => ({ '--rw-key-share': `${renderedShare.value}fr`, '--rw-console-share': `${1 - renderedShare.value}fr` }))
let paneObserver
function setBrowserShare(value) { browserShare.value = Math.max(paneLimits.value.min, Math.min(paneLimits.value.max, value)) }
function startResize(event) {
  if (event.button !== 0 || event.isPrimary === false) return
  event.preventDefault()
  event.currentTarget.setPointerCapture(event.pointerId)
  resizing.value = true
}
function moveResize(event) {
  if (!resizing.value || !paneRef.value) return
  const bounds = paneRef.value.getBoundingClientRect()
  setBrowserShare((event.clientX - bounds.left - 9) / Math.max(1, bounds.width - 18))
}
function stopResize(event) {
  resizing.value = false
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
}
function resizeWithKeyboard(event) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End', 'Enter'].includes(event.key)) return
  event.preventDefault()
  if (event.key === 'Home') setBrowserShare(paneLimits.value.min)
  else if (event.key === 'End') setBrowserShare(paneLimits.value.max)
  else if (event.key === 'Enter') setBrowserShare(0.5)
  else setBrowserShare(renderedShare.value + (event.key === 'ArrowRight' ? 1 : -1) * 20 / Math.max(1, paneWidth.value - 18))
}
const databases = ref([]), schema = ref(''), databaseSearch = ref(''), discovery = ref({ warnings: [] }), discoveryLoading = ref(false), discoveryError = ref('')
const keys = ref([]), cursor = ref('0'), scanComplete = ref(false), scanned = ref(false), keysLoading = ref(false), keysError = ref(''), keyTotal = ref(null), keyPattern = ref(''), scanPattern = ref('*')
const running = ref(false), command = ref('DBSIZE'), activeTab = ref('result'), result = ref(null), commandError = ref('')
const history = ref([]), historyTotal = ref(0), historyPage = ref(1), historyLoading = ref(false), historyError = ref('')
const detailVisible = ref(false), detailLoading = ref(false), detail = ref(null), detailError = ref(''), selectedKey = ref('')
const formVisible = ref(false), editing = ref(false), form = reactive({ key: '', type: 'string', value: '', ttl: undefined })
const locked = computed(() => running.value || formVisible.value)
const readOnly = computed(() => props.connection.accessMode === 'readonly')
const shownDatabases = computed(() => databases.value.filter(db => db.name.toLowerCase().includes(databaseSearch.value.trim().toLowerCase())))
const currentDatabase = computed(() => databases.value.find(db => db.name === schema.value))
const keyEmptyText = computed(() => keysError.value ? tr("扫描失败，请重试") : scanned.value && !scanComplete.value ? tr("本批次暂无匹配项，可继续扫描") : keyPattern.value ? tr("没有匹配的 Key") : tr("当前逻辑库没有 Key"))
const snippets = [{ label: 'GET', text: 'GET key' }, { label: 'MGET', text: 'MGET key1 key2' }, { label: 'TTL', text: 'TTL key' }, { label: 'TYPE', text: 'TYPE key' }, { label: 'SCAN', text: 'SCAN 0 MATCH * COUNT 100' }, { label: 'HGETALL', text: 'HGETALL hash_key' }, { label: 'LRANGE', text: 'LRANGE list_key 0 199' }, { label: 'SET', text: 'SET key value', write: true }, { label: 'DEL', text: 'DEL key', write: true }, { label: 'EXPIRE', text: 'EXPIRE key 3600', write: true }]
let alive = true, keyGeneration = 0, detailGeneration = 0, historyGeneration = 0
const message = err => err?.response?.data?.message || err?.message || tr("请求失败，请重试")
const target = () => ({ databaseId: props.databaseId, schema: schema.value })
function assertTarget(context) { if (!alive || context.databaseId !== props.databaseId || context.schema !== schema.value) throw new Error(tr("连接或逻辑库已切换，本次操作已停止")) }
async function loadDatabases() {
  discoveryLoading.value = true; discoveryError.value = ''
  try {
    const data = await queryDBMSSchemaTree(props.databaseId)
    if (!alive) return
    databases.value = data.schemas || []; discovery.value = data
    if (!schema.value) schema.value = normalizeRedisSchema(data.defaultSchema || props.connection.dbName || 'db0')
  } catch (err) { if (alive) { discoveryError.value = message(err); if (!schema.value) { try { schema.value = normalizeRedisSchema(props.connection.dbName || 'db0') } catch { schema.value = 'db0' } } } }
  finally { if (alive) discoveryLoading.value = false }
}
async function selectDatabase(value) {
  if (locked.value) return
  let next
  try { next = normalizeRedisSchema(value); if (discovery.value.clusterMode && next !== 'db0') throw new Error(tr("Redis Cluster 只支持 db0")) } catch (err) { ElMessage.warning(message(err)); return }
  schema.value = next; detailVisible.value = false; detail.value = null; result.value = null; commandError.value = ''; selectedKey.value = ''; activeTab.value = 'result'; keyPattern.value = ''
  if (!databases.value.some(db => db.name === next)) databases.value.push({ name: next, keyCount: null })
  await loadKeys()
}
async function loadKeys(append = false) {
  if (!schema.value || running.value) return
  const version = ++keyGeneration, context = target()
  keysLoading.value = true; keysError.value = ''
  if (!append) { detailGeneration++; detailVisible.value = false; detail.value = null; selectedKey.value = ''; keys.value = []; keyTotal.value = null; cursor.value = '0'; scanComplete.value = false; scanned.value = false; scanPattern.value = keyPattern.value || '*' }
  try {
    const data = await queryRedisKeys({ ...context, cursor: append ? cursor.value : '0', pattern: scanPattern.value })
    if (!alive || version !== keyGeneration || context.schema !== schema.value) return
    const combined = append ? [...keys.value, ...(data.keys || [])] : data.keys || []
    keys.value = [...new Map(combined.map(row => [row.key, row])).values()]
    cursor.value = String(data.nextCursor || '0'); scanComplete.value = Boolean(data.complete); scanned.value = true; keyTotal.value = data.total ?? null
    if (currentDatabase.value) currentDatabase.value.keyCount = data.total ?? null
    if (data.warnings?.length) keysError.value = data.warnings.map(warning => tr(warning)).join(' ')
  } catch (err) { if (alive && version === keyGeneration) keysError.value = message(err) }
  finally { if (alive && version === keyGeneration) keysLoading.value = false }
}
function toggleKey(row) {
  if (detailVisible.value && selectedKey.value === row.key) { detailGeneration++; detailVisible.value = false; detailLoading.value = false; return }
  showKey(row)
}
async function showKey(row) {
  const version = ++detailGeneration, context = target()
  detailVisible.value = true; detailLoading.value = true; detail.value = null; detailError.value = ''; selectedKey.value = row.key
  try { const data = await queryDBMSResourceData({ ...context, table: row.key }); if (!alive || version !== detailGeneration || context.schema !== schema.value) return; detail.value = data.rows?.[0] || null; if (!detail.value) detailError.value = tr("该 Key 已不存在，请刷新列表") }
  catch (err) { if (alive && version === detailGeneration) detailError.value = message(err) }
  finally { if (alive && version === detailGeneration) detailLoading.value = false }
}
async function loadHistory() {
  const version = ++historyGeneration; historyLoading.value = true; historyError.value = ''
  try { const data = await queryDBMSSQLHistory({ databaseId: props.databaseId, pageNum: historyPage.value, pageSize: 20 }); if (alive && version === historyGeneration) { history.value = data.list || []; historyTotal.value = data.total || 0 } }
  catch (err) { if (alive && version === historyGeneration) historyError.value = message(err) }
  finally { if (alive && version === historyGeneration) historyLoading.value = false }
}
async function confirmWrite(context, operation, key = '') {
  await confirmRiskOperation({ operation, targetSummary: `${props.connection.name} / ${context.schema}${key ? ` / ${key}` : ''}`, production: ['prod', 'production', '生产', '线上'].includes(props.connection.environment), destructive: true })
  assertTarget(context)
}
async function runCommand() {
  if (running.value || !schema.value || !command.value.trim()) return
  const context = target(), text = command.value.trim()
  running.value = true; commandError.value = ''
  try {
    const analysis = await analyzeRedisCommand({ ...context, commandText: text })
    assertTarget(context)
    if (analysis.writeOperation) { if (readOnly.value) throw new Error(tr("当前连接为只读模式，不能执行写入命令")); await confirmWrite(context, `Redis ${analysis.command}`) }
    const data = await executeRedisCommand({ ...context, commandText: text, confirmed: analysis.writeOperation })
    assertTarget(context); result.value = data; activeTab.value = 'result'; await loadHistory()
    if (analysis.writeOperation) { running.value = false; await Promise.all([loadKeys(), loadDatabases()]) }
  } catch (err) { if (alive && err !== 'cancel' && err !== 'close') commandError.value = message(err) }
  finally { if (alive) running.value = false }
}
function openForm(row = null) {
  if (readOnly.value || running.value) return
  if (row?.truncated || row?.type === 'stream') return ElMessage.warning(tr("当前为部分预览或暂不支持的类型，请使用命令控制台按需修改"))
  editing.value = Boolean(row); Object.assign(form, { key: row?.key || '', type: row?.type || 'string', value: row?.value || '', ttl: row ? row.ttlSeconds >= 0 ? row.ttlSeconds : -1 : undefined }); formVisible.value = true
}
async function submitForm() {
  if (running.value || readOnly.value) return
  let commands
  try { commands = buildRedisKeyCommands(form, editing.value) } catch (err) { return ElMessage.warning(message(err)) }
  const context = target(), key = form.key
  running.value = true
  try {
    await confirmWrite(context, editing.value ? tr("更新 Redis Key") : tr("新增 Redis Key"), key)
    for (const commandText of commands) { assertTarget(context); await executeRedisCommand({ ...context, commandText, confirmed: true }) }
    if (!alive) return
    formVisible.value = false; detailVisible.value = false; running.value = false
    await Promise.all([loadKeys(), loadDatabases(), loadHistory()]); ElMessage.success(tr("{p0} / {p1} 已保存", { p0: (context.schema), p1: (key) }))
  } catch (err) { if (alive && err !== 'cancel' && err !== 'close') ElMessage.error(message(err)) }
  finally { if (alive) running.value = false }
}
async function deleteKey(row) {
  if (running.value || readOnly.value) return
  const context = target(); running.value = true
  try { await confirmWrite(context, tr("删除 Redis Key"), row.key); await executeRedisCommand({ ...context, commandText: `DEL ${quoteRedisArgument(row.key)}`, confirmed: true }); assertTarget(context); detailVisible.value = false; running.value = false; await Promise.all([loadKeys(), loadDatabases(), loadHistory()]); ElMessage.success(tr("Key 已删除")) }
  catch (err) { if (alive && err !== 'cancel' && err !== 'close') ElMessage.error(message(err)) }
  finally { if (alive) running.value = false }
}
async function reuseHistory(row) { if (running.value) return; await selectDatabase(row.schemaName); command.value = row.sqlText; activeTab.value = 'result' }
function switchConnection(item) { if (!locked.value) emit('switch-database', item) }
onMounted(async () => {
  paneObserver = new ResizeObserver(([entry]) => { if (entry.contentRect.width > 0) paneWidth.value = entry.contentRect.width })
  if (paneRef.value) paneObserver.observe(paneRef.value)
  await loadDatabases(); if (alive) await Promise.all([loadKeys(), loadHistory()])
})
onBeforeUnmount(() => { paneObserver?.disconnect(); alive = false; keyGeneration++; detailGeneration++; historyGeneration++ })
</script>

<template>
  <div class="rw-layout">
    <aside class="rw-sidebar page-card">
      <section class="rw-connections"><header><strong>{{ tr("数据库连接") }}</strong><span>{{ tr("点击连接即可切换") }}</span></header><DatabaseConnectionTree :active-id="databaseId" @select="switchConnection" /></section>
      <section class="rw-databases" v-loading="discoveryLoading"><header><strong>{{ tr("逻辑数据库") }}</strong><el-button text :icon="Refresh" :disabled="locked" :aria-label="tr('刷新逻辑库')" @click="loadDatabases" /></header><el-input v-model="databaseSearch" :prefix-icon="Search" clearable :placeholder="tr('查找 DB，例如 db10')" :aria-label="tr('搜索逻辑库')" /><el-alert v-if="discoveryError" :title="discoveryError" type="error" :closable="false" /><div class="rw-db-list"><button v-for="db in shownDatabases" :key="db.name" type="button" :class="{ active: schema === db.name, populated: Number(db.keyCount) > 0 }" :disabled="locked" :aria-pressed="schema === db.name" @click="selectDatabase(db.name)"><el-icon><Coin /></el-icon><strong>{{ db.name }}</strong><span>{{ db.keyCount ?? '—' }}</span></button></div><p v-if="!shownDatabases.length" class="rw-muted">{{ tr("没有匹配的逻辑库，可在右侧输入 DB 编号。") }}</p></section>
    </aside>
    <main ref="paneRef" class="rw-main" :class="{ 'is-resizing': resizing }" :style="paneStyle">
      <section :id="`redis-key-browser-${databaseId}`" class="rw-browser page-card">
        <header class="rw-browser-head"><div><h3>{{ schema }} <span>{{ tr("Key 浏览器") }}</span></h3><p>{{ tr("点击 Key，在下方查看值。") }}</p></div><el-button type="primary" plain :disabled="readOnly || !schema || running" @click="openForm()">{{ tr("新增 Key") }}</el-button></header>
        <div class="rw-browser-content">
          <div class="rw-search">
            <el-input v-model="keyPattern" clearable :prefix-icon="Search" :placeholder="tr('匹配 Key，例如 user:*')" :aria-label="tr('Redis Key 匹配模式')" :disabled="keysLoading" @keyup.enter="loadKeys()" />
            <el-button :loading="keysLoading" :disabled="running" @click="loadKeys()">{{ tr("扫描 Key") }}</el-button>
          </div>
          <el-alert v-if="keysError" :title="keysError" type="warning" :closable="false" show-icon />
          <div v-loading="keysLoading" class="rw-key-list" :aria-label="tr('Redis Key 列表')">
            <article v-for="row in keys" :key="row.key" class="rw-key-item" :class="{ expanded: detailVisible && selectedKey === row.key }">
              <header class="rw-key-row">
                <button type="button" class="rw-key-name" :title="row.key" :aria-expanded="detailVisible && selectedKey === row.key" @click="toggleKey(row)">
                  <el-icon><Key /></el-icon><span>{{ row.key }}</span>
                </button>
                <span class="rw-key-type">{{ row.type }}</span>
                <span class="rw-key-ttl" :title="`TTL: ${row.ttlSeconds}`">{{ row.ttlSeconds >= 0 ? `${row.ttlSeconds} ${tr('秒')}` : tr(row.ttl) }}</span>
                <div class="rw-key-actions">
                  <el-button text :icon="View" :aria-label="tr('查看 Key {p0}', { p0: (row.key) })" :title="tr('查看值')" @click="showKey(row)" />
                  <el-button text :icon="Delete" :disabled="readOnly || running" :aria-label="tr('删除 Key {p0}', { p0: (row.key) })" :title="tr('删除 Key')" @click="deleteKey(row)" />
                </div>
              </header>
              <div v-if="detailVisible && selectedKey === row.key" v-loading="detailLoading" class="rw-key-value">
                <el-alert v-if="detailError" :title="detailError" type="warning" :closable="false" />
                <template v-if="detail">
                  <div class="rw-value-toolbar"><span>{{ tr("值") }}</span><el-button text :icon="Edit" :disabled="readOnly || running || detail.truncated || detail.type === 'stream'" @click="openForm(detail)">{{ tr("编辑") }}</el-button></div>
                  <el-alert v-if="detail.truncated" :title="tr('仅展示部分值，已停用可视化编辑。')" type="info" :closable="false" />
                  <pre class="rw-value-content" :aria-label="tr('Redis Key 值')">{{ detail.value }}</pre>
                </template>
              </div>
            </article>
            <p v-if="!keys.length && !keysLoading" class="rw-key-empty">{{ keyEmptyText }}</p>
          </div>
          <footer class="rw-scan-footer"><span>{{ tr("已载入 {p0}{p1} 个 · {p2}", { p0: (keys.length), p1: (keyTotal != null ? ` / ${keyTotal}` : ''), p2: (scanned ? scanComplete ? tr("扫描完成") : tr("扫描未结束") : tr("等待扫描")) }) }}</span><el-button v-if="scanned && !scanComplete" :loading="keysLoading" :disabled="running" @click="loadKeys(true)">{{ tr("继续扫描") }}</el-button></footer>
        </div>
      </section>
      <div class="rw-splitter" role="separator" tabindex="0" :aria-label="tr('调整 Key 浏览器宽度')" aria-orientation="vertical" :aria-controls="`redis-key-browser-${databaseId}`" :aria-valuemin="Math.round(paneLimits.min * 100)" :aria-valuemax="Math.round(paneLimits.max * 100)" :aria-valuenow="Math.round(renderedShare * 100)" :aria-valuetext="tr('Key 浏览器宽度 {p0}%', { p0: (Math.round(renderedShare * 100)) })" :title="tr('左右拖动调整宽度，双击恢复均分')" @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="stopResize" @keydown="resizeWithKeyboard" @dblclick="setBrowserShare(0.5)" />
      <section class="rw-console page-card">
        <header class="rw-console-head"><div><span class="rw-redis-mark">&gt;_</span><div><h3>{{ tr("Redis 命令控制台") }}</h3><p>{{ tr("命令始终在当前选中的逻辑库执行。") }}</p></div></div><el-tag :type="readOnly ? 'info' : 'success'" effect="plain">{{ readOnly ? tr("只读连接") : tr("读写连接") }}</el-tag></header>
        <div class="rw-db-context"><span>{{ tr("当前逻辑库") }}</span><el-select :model-value="schema" filterable allow-create default-first-option :disabled="locked" :aria-label="tr('当前 Redis 逻辑库')" @change="selectDatabase"><el-option v-for="db in databases" :key="db.name" :value="db.name" :label="`${db.name}${db.keyCount != null ? ` (${db.keyCount})` : ''}`" /></el-select><span>{{ tr("{p0} 个 Key", { p0: (keyTotal ?? currentDatabase?.keyCount ?? '—') }) }}</span><el-button type="primary" :icon="VideoPlay" :loading="running" :disabled="!schema" @click="runCommand">{{ tr("执行命令") }}</el-button></div>
        <el-alert v-if="discovery.warnings?.length" :title="discovery.warnings.map(warning => tr(warning)).join(' ')" type="info" :closable="false" class="rw-metadata-note" />
        <div class="rw-snippets"><span>{{ tr("常用命令") }}</span><button v-for="item in snippets" :key="item.label" type="button" :class="{ write: item.write }" :disabled="running || item.write && readOnly" @click="command = item.text">{{ item.label }}</button></div>
        <div class="rw-editor"><div><i />{{ connection.host }}:{{ connection.port || 6379 }} <b>[{{ schema }}]</b></div><el-input v-model="command" type="textarea" :rows="3" resize="vertical" :disabled="running" spellcheck="false" :aria-label="tr('Redis 命令')" :placeholder="tr('例如：GET key 或 SCAN 0 MATCH * COUNT 100')" @keydown.ctrl.enter.prevent="runCommand" @keydown.meta.enter.prevent="runCommand" /></div>
        <footer class="rw-console-foot"><span>{{ tr("支持引号参数 · 写入操作需确认并记录历史") }}</span><kbd>Ctrl / ⌘ + Enter</kbd></footer><el-alert v-if="commandError" :title="commandError" type="error" show-icon :closable="false" />
        <el-tabs v-model="activeTab" class="rw-command-tabs">
          <el-tab-pane :label="tr('命令结果')" name="result"><template v-if="result"><div class="rw-result-meta"><el-tag effect="plain">{{ result.schema }}</el-tag><span>{{ result.durationMs }} ms</span><span>{{ result.executionId }}</span></div><pre class="rw-command-result">{{ result.rows?.map(row => `${row.command}\n${row.result}`).join('\n\n') }}</pre></template><el-empty v-else :description="tr('执行一条命令后，在这里查看结果')" :image-size="64" /></el-tab-pane>
          <el-tab-pane :label="tr('执行记录')" name="history"><div class="rw-history-head"><span>{{ tr("此连接的执行记录，包含逻辑库与执行人") }}</span><el-button text :icon="Refresh" @click="loadHistory">{{ tr("刷新记录") }}</el-button></div><el-alert v-if="historyError" :title="historyError" type="error" :closable="false" /><el-table v-loading="historyLoading" :data="history"><el-table-column prop="schemaName" :label="tr('逻辑库')" width="90" /><el-table-column prop="sqlText" :label="tr('命令')" min-width="250" show-overflow-tooltip /><el-table-column :label="tr('状态')" width="90"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">{{ row.status === 1 ? tr("成功") : tr("失败") }}</el-tag></template></el-table-column><el-table-column prop="durationMs" :label="tr('耗时(ms)')" width="100" /><el-table-column prop="operator" :label="tr('执行人')" width="100" /><el-table-column prop="createTime" :label="tr('时间')" min-width="160" /><el-table-column :label="tr('操作')" width="90"><template #default="{ row }"><el-button link type="primary" :disabled="running" @click="reuseHistory(row)">{{ tr("填入命令") }}</el-button></template></el-table-column></el-table><el-pagination v-model:current-page="historyPage" :total="historyTotal" :page-size="20" layout="total, prev, pager, next" @current-change="loadHistory" /></el-tab-pane>
        </el-tabs>
      </section>
    </main>
    <el-dialog v-model="formVisible" :title="`${editing ? tr('编辑') : tr('新增')} Key · ${schema}`" width="min(640px, 94vw)" :close-on-click-modal="!running" :close-on-press-escape="!running" :show-close="!running"><el-form label-position="top"><el-form-item :label="tr('Key 名称')"><el-input v-model="form.key" :disabled="editing || running" /></el-form-item><el-form-item :label="tr('类型')"><el-select v-model="form.type" :disabled="editing || running"><el-option v-for="kind in ['string', 'hash', 'list', 'set', 'zset']" :key="kind" :label="kind" :value="kind" /></el-select></el-form-item><el-form-item :label="form.type === 'string' ? tr('字符串值') : tr('值（JSON）')"><el-input v-model="form.value" type="textarea" :rows="7" :disabled="running" spellcheck="false" /></el-form-item><el-form-item :label="tr('TTL（秒）')"><el-input-number v-model="form.ttl" :min="-1" :disabled="running" :placeholder="tr('不设置')" /><span class="rw-ttl-hint">{{ tr("-1 永不过期；字符串编辑留空保留 TTL") }}</span></el-form-item></el-form><template #footer><el-button :disabled="running" @click="formVisible = false">{{ tr("取消") }}</el-button><el-button type="primary" :loading="running" @click="submitForm">{{ tr("确认保存") }}</el-button></template></el-dialog>
  </div>
</template>

<style scoped>

.rw-layout {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 18px;
  height: max(580px, calc(100dvh - 240px));
  color: #233a59;
}
.rw-sidebar { display: flex; flex-direction: column; min-height: 0; padding: 0; overflow: hidden; }
.rw-sidebar section { padding: 16px; }
.rw-sidebar header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 13px; font-size: 13px; }
.rw-sidebar header > span { color: #99a6b7; font-size: 11px; }
.rw-connections { flex-shrink: 0; max-height: 38%; overflow: auto; border-bottom: 1px solid #e9eef5; }
.rw-databases { display: flex; flex: 1; flex-direction: column; min-height: 0; }
.rw-databases > .el-input { flex-shrink: 0; }
.rw-databases .el-alert { margin-top: 12px; }
.rw-db-list { display: flex; flex: 1; flex-direction: column; gap: 4px; min-height: 0; overflow: auto; margin-top: 12px; }
.rw-db-list button { display: flex; align-items: center; flex-shrink: 0; gap: 10px; min-height: 35px; border: 1px solid transparent; border-radius: 6px; background: transparent; color: #72849a; padding: 7px 10px; cursor: pointer; text-align: left; }
.rw-db-list button > strong { font: 500 12px Consolas, monospace; }
.rw-db-list button > span { margin-left: auto; min-width: 23px; padding: 1px 5px; border-radius: 4px; background: #f3f6fa; color: #97a5b5; font-size: 11px; text-align: center; }
.rw-db-list button.populated > span { color: #258b76; background: #e8f6f1; }
.rw-db-list button.active { border-color: #dce8ff; background: #edf3ff; color: #3567cc; }
.rw-db-list button:hover { background: #f3f7fd; }
.rw-db-list button:disabled { cursor: default; opacity: .6; }
.rw-main { display: grid; grid-template-columns: minmax(0, var(--rw-key-share)) 18px minmax(0, var(--rw-console-share)); min-width: 0; min-height: 0; }
.rw-splitter { position: relative; cursor: col-resize; touch-action: none; user-select: none; outline: none; }
.rw-splitter::before { content: ''; position: absolute; top: 0; bottom: 0; left: 8px; width: 2px; border-radius: 2px; background: transparent; }
.rw-splitter::after { content: ''; position: absolute; top: 50%; left: 7px; width: 4px; height: 36px; margin-top: -18px; border-radius: 4px; background: #c9d4e3; }
.rw-splitter:hover::before, .rw-splitter:focus-visible::before, .is-resizing > .rw-splitter::before { background: #c5d9fb; }
.rw-splitter:hover::after, .rw-splitter:focus-visible::after, .is-resizing > .rw-splitter::after { background: #6899e8; }
.rw-splitter:focus-visible { outline: 2px solid #8eb1f3; outline-offset: -4px; border-radius: 6px; }
.rw-main.is-resizing { cursor: col-resize; user-select: none; }
.rw-main.is-resizing > section { pointer-events: none; }
.rw-console, .rw-browser { display: flex; flex-direction: column; padding: 19px 20px; min-width: 0; min-height: 0; overflow: hidden; }
.rw-console-head, .rw-browser-head { display: flex; justify-content: space-between; align-items: center; gap: 14px; flex-shrink: 0; }
.rw-console-head > div { display: flex; gap: 11px; align-items: center; }
.rw-console h3, .rw-browser h3 { margin: 0; font-size: 16px; line-height: 1.5; }
.rw-console-head p, .rw-browser-head p { margin: 4px 0 0; font-size: 12px; color: #8c9cb0; }
.rw-redis-mark { display: grid; place-items: center; flex-shrink: 0; width: 36px; height: 36px; border-radius: 9px; color: #d66070; background: #fff0f2; font: 600 17px Consolas, monospace; }
.rw-db-context { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 19px 0; padding-top: 15px; border-top: 1px solid #edf1f6; flex-shrink: 0; }
.rw-db-context > span { font-size: 12px; color: #8394a9; }
.rw-db-context .el-select { width: 160px; }
.rw-db-context .el-button { margin-left: auto; }
.rw-metadata-note { margin-bottom: 14px; }
.rw-snippets { display: flex; align-items: center; flex-wrap: wrap; gap: 7px; margin-bottom: 12px; flex-shrink: 0; }
.rw-snippets > span { font-size: 11px; color: #9ba8b9; margin-right: 3px; }
.rw-snippets > button { border: 1px solid #e2e8f1; border-radius: 5px; background: #fafcff; padding: 4px 9px; color: #5c7290; font: 11px Consolas, monospace; cursor: pointer; }
.rw-snippets > button.write { color: #b47c59; background: #fffaf5; border-color: #f0e3d6; }
.rw-snippets > button:disabled { opacity: .45; cursor: default; }
.rw-snippets > button:hover:not(:disabled) { border-color: #9fb9ea; color: #3567cc; }
.rw-editor { border: 1px solid #dfe7f1; border-radius: 8px; overflow: hidden; background: #f8fafd; flex-shrink: 0; }
.rw-editor > div:first-child { display: flex; align-items: center; gap: 7px; padding: 10px 13px; border-bottom: 1px solid #e8eef5; color: #8799b0; font: 11px Consolas, monospace; }
.rw-editor > div > i { height: 6px; width: 6px; border-radius: 50%; background: #53b99a; }
.rw-editor b { font-weight: 500; color: #5678ae; }
.rw-editor :deep(.el-textarea__inner) { box-shadow: none !important; border: 0; background: #f8fafd; color: #26476b; padding: 12px 15px; min-height: 94px; max-height: 240px; font: 13px/1.85 Consolas, 'Cascadia Code', monospace; }
.rw-editor:focus-within { border-color: #92b5ef; box-shadow: 0 0 0 2px #edf3fd; }
.rw-console-foot { display: flex; justify-content: space-between; flex-wrap: wrap; flex-shrink: 0; gap: 8px; margin-top: 10px; font-size: 11px; color: #95a4b7; }
.rw-console-foot kbd { font: 10px Consolas, monospace; color: #8c9db3; white-space: nowrap; }
.rw-console > .el-alert { margin-top: 12px; flex-shrink: 0; }
.rw-browser-head { margin-bottom: 17px; }
.rw-browser h3 { font-family: Consolas, 'Microsoft YaHei', sans-serif; color: #365c94; }
.rw-browser h3 > span { margin-left: 7px; color: #304866; font-size: 14px; }
.rw-browser-content { display: flex; flex: 1; flex-direction: column; min-height: 0; border-top: 1px solid #edf1f6; padding-top: 17px; }
.rw-search { display: flex; flex-shrink: 0; gap: 10px; align-items: center; margin: 0 0 14px; }
.rw-search > .el-input { flex: 1; min-width: 0; }
.rw-key-list { flex: 1; min-height: 0; overflow: auto; border: 1px solid #e6eaf0; border-radius: 6px; background: #fff; }
.rw-key-item + .rw-key-item { border-top: 1px solid #e6eaf0; }
.rw-key-row { display: flex; align-items: center; gap: 7px; min-height: 38px; padding: 0 9px; background: #f7f8fa; }
.rw-key-name { display: flex; flex: 1; align-items: center; gap: 7px; min-width: 0; border: 0; padding: 8px 0; background: transparent; color: #34465d; cursor: pointer; text-align: left; font: 12px/1.5 Consolas, 'Microsoft YaHei', monospace; }
.rw-key-name .el-icon { flex-shrink: 0; color: #9aa4af; font-size: 15px; }
.rw-key-name span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rw-key-type, .rw-key-ttl { flex-shrink: 0; padding: 1px 7px; border: 1px solid; border-radius: 10px; font: 11px/1.5 Consolas, 'Microsoft YaHei', sans-serif; white-space: nowrap; }
.rw-key-type { border-color: #d8eadf; color: #239454; background: #f1faf4; }
.rw-key-ttl { border-color: #f4e1c3; color: #ce8b34; background: #fffbf2; }
.rw-key-actions { display: flex; flex-shrink: 0; align-items: center; gap: 0; white-space: nowrap; }
.rw-key-actions .el-button { width: 25px; height: 25px; padding: 0; margin: 0; color: #93a0b0; }
.rw-key-actions .el-button:hover:not(:disabled) { color: #3567cc; background: #eaf1fc; }
.rw-key-actions .el-button:last-child:hover:not(:disabled) { color: #d85d68; background: #fff0f2; }
.rw-key-item.expanded .rw-key-row { border-bottom: 1px solid #e6eaf0; }
.rw-key-item.expanded .rw-key-name { color: #3567cc; }
.rw-key-value { position: relative; min-height: 160px; padding: 0 12px 12px; }
.rw-value-toolbar { display: flex; justify-content: space-between; align-items: center; min-height: 35px; color: #97a3b2; font-size: 11px; }
.rw-value-toolbar .el-button { padding: 4px 7px; font-size: 11px; }
.rw-value-content { margin: 0; min-height: 130px; color: #34465d; font: 12px/1.9 Consolas, 'Cascadia Code', monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.rw-key-empty { margin: 38px 12px; text-align: center; color: #96a5b8; font-size: 12px; }
.rw-scan-footer, .rw-history-head { display: flex; justify-content: space-between; align-items: center; flex-shrink: 0; gap: 14px; margin: 12px 0 0; color: #8fa0b3; font-size: 11px; }
.rw-command-tabs { display: flex; flex: 1; flex-direction: column; min-height: 100px; margin-top: 18px; }
.rw-command-tabs :deep(.el-tabs__header) { flex-shrink: 0; margin-bottom: 12px; }
.rw-command-tabs :deep(.el-tabs__content) { flex: 1; min-height: 0; overflow: auto; }
.rw-command-tabs :deep(.el-tab-pane) { display: flex; flex-direction: column; min-height: 100%; }
.rw-command-tabs :deep(.el-empty) { flex: 1; padding: 20px 0; }
.rw-result-meta { display: flex; flex-shrink: 0; gap: 14px; align-items: center; margin: 0 0 12px; color: #8c9db1; font-size: 11px; }
.rw-command-result { flex: 1; margin: 0; min-height: 100px; padding: 14px; border: 1px solid #e6edf5; border-radius: 7px; background: #f8fafd; color: #2f506f; font: 12px/1.85 Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.rw-ttl-hint, .rw-muted { margin-left: 10px; color: #96a5b8; font-size: 11px; }
.rw-layout :deep(.el-pagination) { justify-content: flex-end; margin-top: 15px; }
.rw-layout button:focus-visible { outline: 2px solid #8eb1f3; outline-offset: 2px; }
@media (max-width: 1440px) {
  .rw-layout { grid-template-columns: 220px minmax(0, 1fr); }
  .rw-console, .rw-browser { padding: 17px 16px; }
  .rw-key-row { gap: 5px; padding: 0 7px; }
  .rw-key-type, .rw-key-ttl { padding: 1px 5px; font-size: 10px; }
}
@media (max-width: 1150px) {
  .rw-layout { height: auto; align-items: stretch; }
  .rw-main { grid-template-columns: 1fr; }
  .rw-splitter { display: none; }
  .rw-sidebar { max-height: 1100px; }
  .rw-browser { height: 520px; }
  .rw-console { min-height: 620px; }
}
@media (max-width: 850px) {
  .rw-layout { grid-template-columns: 1fr; }
  .rw-sidebar { display: grid; grid-template-columns: 1fr 1fr; max-height: 300px; }
  .rw-connections { max-height: none; border-bottom: 0; border-right: 1px solid #e9eef5; }
}
@media (max-width: 560px) {
  .rw-sidebar { display: flex; max-height: 430px; }
  .rw-connections { max-height: 180px; border-right: 0; border-bottom: 1px solid #e9eef5; }
  .rw-console, .rw-browser { padding: 15px; }
  .rw-db-context .el-select { width: 140px; }
  .rw-db-context .el-button { margin-left: 0; }
}
</style>
