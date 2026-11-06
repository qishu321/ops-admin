<script setup>
import { tr, getLocale } from '../../utils/i18n'
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowUp, CopyDocument } from '@element-plus/icons-vue'
import { kafkaByteSize, kafkaEncodingLabel, kafkaPayload } from './kafka-topic'

const props = defineProps({ message: { type: Object, required: true } })
const emit = defineEmits(['collapse'])

const tab = ref('value'), pretty = ref(true)
const headers = computed(() => props.message?.headers || [])
const tabs = computed(() => [
  { key: 'key', label: 'Key', meta: `${kafkaEncodingLabel(props.message?.key?.encoding)} · ${kafkaByteSize(props.message?.key?.size)}` },
  { key: 'value', label: 'Value', meta: `${kafkaEncodingLabel(props.message?.value?.encoding)} · ${kafkaByteSize(props.message?.value?.size)}` },
  { key: 'headers', label: 'Headers', meta: String(headers.value.length) }
])
const currentText = computed(() => kafkaPayload(tab.value === 'key' ? props.message?.key : props.message?.value, pretty.value))
const currentEncoding = computed(() => (tab.value === 'key' ? props.message?.key : props.message?.value)?.encoding || 'utf8')
const isNull = computed(() => Boolean((tab.value === 'key' ? props.message?.key : props.message?.value)?.null))
const time = computed(() => {
  const raw = props.message?.timestamp
  if (!raw || String(raw).startsWith('0001-')) return '—'
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString(getLocale(), { hour12: false })
})

async function copy() {
  const text = tab.value === 'headers'
    ? headers.value.map(h => `${h.key}: ${kafkaPayload(h.value)}`).join('\n')
    : currentText.value
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(tr("已复制到剪贴板"))
  } catch {
    ElMessage.warning(tr("浏览器不允许访问剪贴板，请手动选择内容复制"))
  }
}
</script>

<template>
  <section class="msg-panel">
    <header class="msg-head">
      <span class="msg-head-dot"/>
      <b>{{ tr("消息详情") }}</b>
      <span class="msg-head-path">{{ tr("分区 {p0} · Offset {p1}", { p0: (message.partition), p1: (message.offset) }) }}</span>
      <span class="msg-head-hint">{{ tr("点击上方消息行任意位置可收起") }}</span>
      <el-button link type="primary" :icon="ArrowUp" @click="emit('collapse')">{{ tr("收起") }}</el-button>
    </header>

    <div class="msg-body">
      <div class="msg-main">
        <div class="msg-bar">
          <div class="msg-tabs" role="tablist">
            <button v-for="item in tabs" :key="item.key" type="button" role="tab" :aria-selected="tab === item.key" class="msg-tab" :class="{ active: tab === item.key }" @click="tab = item.key">
              {{ item.label }}<small>{{ item.meta }}</small>
            </button>
          </div>
          <div class="msg-tools">
            <el-checkbox v-if="tab !== 'headers'" v-model="pretty" :disabled="currentEncoding === 'base64'">{{ tr("格式化 JSON") }}</el-checkbox>
            <el-button link type="primary" :icon="CopyDocument" @click="copy">{{ tr("复制") }}</el-button>
          </div>
        </div>

        <template v-if="tab === 'headers'">
          <p v-if="!headers.length" class="msg-empty">{{ tr("该消息没有 Headers。") }}</p>
          <ul v-else class="msg-headers">
            <li v-for="(header, index) in headers" :key="`${header.key}-${index}`"><code>{{ header.key }}</code><span>{{ kafkaPayload(header.value) || tr("（空值）") }}</span></li>
          </ul>
        </template>
        <pre v-else-if="isNull" class="msg-code null">null</pre>
        <pre v-else class="msg-code">{{ currentText || tr("（空字符串）") }}</pre>
      </div>

      <aside class="msg-meta">
        <div><span>{{ tr("时间") }}</span><b>{{ time }}</b></div>
        <div><span>{{ tr("时间类型") }}</span><b>CreateTime</b></div>
        <div><span>{{ tr("分区") }}</span><b>{{ message.partition }}</b></div>
        <div><span>Offset</span><b>{{ message.offset }}</b></div>
        <div><span>{{ tr("Key 编码") }}</span><b>{{ kafkaEncodingLabel(message.key?.encoding) }}</b></div>
        <div><span>{{ tr("Key 大小") }}</span><b>{{ kafkaByteSize(message.key?.size) }}</b></div>
        <div><span>{{ tr("Value 编码") }}</span><b>{{ kafkaEncodingLabel(message.value?.encoding) }}</b></div>
        <div><span>{{ tr("Value 大小") }}</span><b>{{ kafkaByteSize(message.value?.size) }}</b></div>
        <div><span>Headers</span><b>{{ tr("{p0} 个", { p0: (headers.length) }) }}</b></div>
        <p v-if="message.value?.null" class="msg-flag">{{ tr("Tombstone：Value 为 null 的删除标记") }}</p>
        <p v-else-if="message.key?.null" class="msg-flag">{{ tr("该消息没有 Key") }}</p>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.msg-panel{margin:2px 14px 16px;border:1px solid #d8e5f8;border-radius:10px;background:var(--el-bg-color,#fff);box-shadow:0 3px 10px rgba(37,72,128,.08);overflow:hidden}
.msg-head{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:10px 14px;background:#f4f8ff;border-bottom:1px solid #e2ecfb}
.msg-head-dot{width:7px;height:7px;border-radius:50%;background:var(--el-color-primary,#3b68b0);box-shadow:0 0 0 3px rgba(59,104,176,.14);flex-shrink:0}
.msg-head b{font-size:14px;color:#26456f}
.msg-head-path{font-size:13px;color:#5b7395;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.msg-head-hint{margin-left:auto;font-size:12.5px;color:var(--el-text-color-secondary,#8a97ab)}
.msg-body{display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:20px;padding:14px 16px 16px;align-items:start}
.msg-main{min-width:0}
.msg-bar{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:10px}
.msg-tabs{display:flex;align-items:center;gap:4px;margin-right:auto}
.msg-tab{display:flex;align-items:center;gap:6px;border:0;background:none;padding:6px 11px;border-radius:6px;cursor:pointer;font:inherit;font-size:14px;color:var(--el-text-color-regular,#4a5b74)}
.msg-tab small{font-size:12px;color:var(--el-text-color-secondary,#8a97ab);font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.msg-tab:hover{background:var(--el-fill-color-light,#f3f6fb)}
.msg-tab.active{background:var(--el-color-primary-light-9,#e8f1fb);color:var(--el-color-primary,#3b68b0);font-weight:600}
.msg-tools{display:flex;align-items:center;gap:10px;flex-shrink:0}
.msg-tools :deep(.el-checkbox__label){font-size:13.5px}
.msg-code{margin:0;padding:14px 16px;max-height:320px;overflow:auto;border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:8px;background:#f8fafd;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14px;line-height:1.75;color:#2d3f5c;white-space:pre-wrap;overflow-wrap:anywhere}
.msg-code.null{color:var(--el-text-color-secondary,#8a97ab)}
.msg-headers{list-style:none;margin:0;padding:10px 14px;border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:8px;background:#f8fafd;display:grid;gap:8px;max-height:320px;overflow:auto}
.msg-headers li{display:grid;grid-template-columns:minmax(120px,220px) minmax(0,1fr);gap:14px;font-size:13.5px;line-height:1.7}
.msg-headers code{color:#2f6bb5;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.msg-headers span{color:#2d3f5c;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.msg-empty{margin:0;padding:18px 16px;border:1px dashed var(--el-border-color,#dfe6f0);border-radius:8px;font-size:13.5px;color:var(--el-text-color-secondary,#8a97ab)}
.msg-meta{display:grid;gap:9px;padding:12px 14px;border:1px solid var(--el-border-color-light,#e4eaf4);border-radius:8px;background:#f8fafd}
.msg-meta>div{display:flex;align-items:baseline;gap:10px;font-size:13.5px}
.msg-meta span{color:var(--el-text-color-secondary,#8a97ab);min-width:76px}
.msg-meta b{color:var(--el-text-color-primary,#23364f);font-weight:600;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.msg-flag{margin:2px 0 0;font-size:12.5px;line-height:1.7;color:#b8722a}
@media (max-width:1100px){.msg-body{grid-template-columns:1fr}.msg-meta{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
