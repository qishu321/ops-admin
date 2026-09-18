<script setup>
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Back, Monitor, UploadFilled } from '@element-plus/icons-vue'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { buildK8sPodTerminalWSUrl, queryK8sPodContainers, uploadK8sPodFile } from '../../api/k8s'

const route = useRoute()
const router = useRouter()

const clusterId = computed(() => Number(route.params.clusterId || 0))
const namespace = computed(() => String(route.params.namespace || ''))
const podName = computed(() => String(route.params.podName || ''))

const terminalRef = ref()
const terminalBoxRef = ref()
const containers = ref([])
const selectedContainer = ref(String(route.query.container || ''))
const connecting = ref(false)
const connected = ref(false)
const currentDirectory = ref('')
const uploadDialogVisible = ref(false)
const uploadFile = ref()
const uploadInputRef = ref()
const uploading = ref(false)
const uploadProgress = ref(0)

const MAX_UPLOAD_SIZE = 50 * 1024 * 1024

let term
let socket
let inputDisposable
let resizeObserver
let resizeFrame
let connectionHintTimer
let terminalRoutePath = ''
let terminalInitialization
let terminalGeneration = 0
let terminalOutputBuffer = ''

function writeTerminalOutput(data) {
  const markerStart = '\x1eOPS_ADMIN_CWD:'
  const markerEnd = '\x1f'
  terminalOutputBuffer += String(data || '')
  let visible = ''
  while (terminalOutputBuffer) {
    const start = terminalOutputBuffer.indexOf(markerStart)
    if (start < 0) {
      let keep = 0
      const maximum = Math.min(markerStart.length - 1, terminalOutputBuffer.length)
      for (let length = maximum; length > 0; length -= 1) {
        if (terminalOutputBuffer.endsWith(markerStart.slice(0, length))) {
          keep = length
          break
        }
      }
      visible += terminalOutputBuffer.slice(0, terminalOutputBuffer.length - keep)
      terminalOutputBuffer = keep ? terminalOutputBuffer.slice(-keep) : ''
      break
    }
    visible += terminalOutputBuffer.slice(0, start)
    const end = terminalOutputBuffer.indexOf(markerEnd, start + markerStart.length)
    if (end < 0) {
      terminalOutputBuffer = terminalOutputBuffer.slice(start)
      break
    }
    const directory = terminalOutputBuffer.slice(start + markerStart.length, end).trim()
    if (directory.startsWith('/')) currentDirectory.value = directory
    terminalOutputBuffer = terminalOutputBuffer.slice(end + markerEnd.length)
  }
  if (visible) term?.write(visible)
}

function createTerminal() {
  term = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontSize: 13,
    rows: 32,
    cols: 120,
    fontFamily: 'Consolas, "Courier New", monospace',
    theme: {
      background: '#07111f',
      foreground: '#e5edf7',
      cursor: '#67c23a',
      green: '#67c23a',
      brightGreen: '#95d475',
      red: '#f56c6c'
    }
  })
  term.open(terminalRef.value)
  term.focus()
  inputDisposable = term.onData((data) => {
    if (connected.value && socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ operation: 'stdin', data }))
    }
  })
  bindTerminalResize()
}

function bindTerminalResize() {
  if (!terminalBoxRef.value || !terminalRef.value || !term) return
  resizeObserver = new ResizeObserver(() => {
    scheduleTerminalSizeSync()
  })
  resizeObserver.observe(terminalBoxRef.value)
  resizeObserver.observe(terminalRef.value)
  syncTerminalSize()
  scheduleTerminalSizeSync()
}

function scheduleTerminalSizeSync() {
  cancelAnimationFrame(resizeFrame)
  resizeFrame = requestAnimationFrame(syncTerminalSize)
}

function syncTerminalSize() {
  if (!term || !terminalRef.value) return
  const width = terminalRef.value.clientWidth
  const height = terminalRef.value.clientHeight
  if (!width || !height) return

  // xterm renders a cell height based on the active browser font.  Using a
  // fixed 18px estimate leaves a visible unused area when that differs from
  // the real value, so derive the next size from the rendered screen instead.
  const screen = terminalRef.value.querySelector('.xterm-screen')
  const cellWidth = screen?.clientWidth ? screen.clientWidth / term.cols : 8.2
  const cellHeight = screen?.clientHeight ? screen.clientHeight / term.rows : 18
  const cols = Math.max(80, Math.floor(width / cellWidth))
  const rows = Math.max(20, Math.floor(height / cellHeight))
  if (term.cols !== cols || term.rows !== rows) {
    term.resize(cols, rows)
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(
        JSON.stringify({
          operation: 'resize',
          data: { cols, rows }
        })
      )
    }
  }
}

async function loadContainers() {
  const list = await queryK8sPodContainers(clusterId.value, namespace.value, podName.value)
  containers.value = Array.isArray(list) ? list : []
  if (!containers.value.length) {
    ElMessage.warning('当前 Pod 没有可用容器')
    return
  }
  if (!containers.value.includes(selectedContainer.value)) {
    selectedContainer.value = containers.value[0]
  }
}

function connectTerminal() {
  if (!selectedContainer.value) {
    ElMessage.warning('请先选择容器')
    return
  }
  disconnectTerminal(true)
  currentDirectory.value = ''
  terminalOutputBuffer = ''
  connecting.value = true
  const url = buildK8sPodTerminalWSUrl({
    clusterId: clusterId.value,
    namespace: namespace.value,
    podName: podName.value,
    container: selectedContainer.value,
    rows: term?.rows || 32,
    cols: term?.cols || 120
  })
  socket = new WebSocket(url)
  socket.onopen = () => {
    term?.clear()
    term?.writeln('\x1b[36m浏览器通道已建立，正在连接 Kubernetes API Server…\x1b[0m')
    clearConnectionHintTimer()
    connectionHintTimer = window.setTimeout(() => {
      if (connecting.value) {
        term?.writeln('\x1b[33m连接耗时较长，仍在等待访问网关建立 exec 通道…\x1b[0m')
      }
    }, 5000)
    syncTerminalSize()
  }
  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data)
      if (payload?.operation === 'stdout' && payload.data) {
        writeTerminalOutput(payload.data)
        return
      }
      if (payload?.operation === 'status') {
        const state = payload.data?.state
        const message = payload.data?.message
        if (state === 'connected') {
          clearConnectionHintTimer()
          connecting.value = false
          connected.value = true
          term?.writeln(`\x1b[32m已连接到 ${namespace.value}/${podName.value}\x1b[0m`)
          term?.writeln(`\x1b[36m容器: ${selectedContainer.value}\x1b[0m`)
          term?.writeln('')
          term?.focus()
        } else if (message) {
          term?.writeln(`\x1b[36m${message}\x1b[0m`)
        }
        return
      }
      if (payload?.operation === 'error') {
        const message = payload.data?.message || 'Pod 终端连接失败'
        clearConnectionHintTimer()
        connecting.value = false
        connected.value = false
        term?.writeln(`\r\n\x1b[31m${message}\x1b[0m`)
        ElMessage.error(message)
        return
      }
    } catch (error) {
      // ignore non-json payload
    }
    writeTerminalOutput(event.data)
  }
  socket.onerror = () => {
    clearConnectionHintTimer()
    connecting.value = false
    connected.value = false
    ElMessage.error('Pod 终端连接失败')
  }
  socket.onclose = () => {
    clearConnectionHintTimer()
    connecting.value = false
    connected.value = false
    term?.writeln('\r\n\x1b[33m连接已关闭。\x1b[0m')
  }
}

function disconnectTerminal(silent = false) {
  const wasConnected = Boolean(socket || connected.value || connecting.value)
  clearConnectionHintTimer()
  // Write before closing the socket. xterm writes asynchronously, so doing it
  // after teardown can be swallowed when a closing stream repaints the prompt.
  if (!silent && wasConnected) {
    term?.writeln('\r\n\x1b[33m已断开 Pod 终端连接。\x1b[0m')
    term?.scrollToBottom()
  }
  if (socket) {
    socket.onclose = null
    socket.close()
    socket = undefined
  }
  connected.value = false
  connecting.value = false
  currentDirectory.value = ''
  terminalOutputBuffer = ''
}

function handleManualDisconnect() {
  // A remote exec stream may already have closed while the terminal remains
  // visible. The explicit user action must still leave an unambiguous record.
  term?.write('\r\n\x1b[33m已断开 Pod 终端连接。\x1b[0m\r\n')
  term?.scrollToBottom()
  disconnectTerminal(true)
}

function clearConnectionHintTimer() {
  if (connectionHintTimer) {
    window.clearTimeout(connectionHintTimer)
    connectionHintTimer = undefined
  }
}

function clearTerminal() {
  term?.clear()
}

function handleContainerChange() {
  connectTerminal()
}

function openUploadDialog() {
  if (!connected.value) {
    ElMessage.warning('请先连接 Pod 终端')
    return
  }
  if (!currentDirectory.value) {
    ElMessage.warning('正在识别当前终端目录，请稍后重试')
    return
  }
  uploadFile.value = undefined
  uploadProgress.value = 0
  uploadDialogVisible.value = true
}

function chooseUploadFile() {
  uploadInputRef.value?.click()
}

function handleUploadFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  if (file.size > MAX_UPLOAD_SIZE) {
    ElMessage.error('单个文件不能超过 50 MB')
    return
  }
  uploadFile.value = file
}

function formatFileSize(size = 0) {
  return size >= 1024 * 1024 ? `${(size / (1024 * 1024)).toFixed(2)} MB` : `${Math.max(1, Math.ceil(size / 1024))} KB`
}

async function submitUpload() {
  if (!uploadFile.value) {
    ElMessage.warning('请选择一个文件')
    return
  }
  const form = new FormData()
  form.append('clusterId', String(clusterId.value))
  form.append('namespace', namespace.value)
  form.append('podName', podName.value)
  form.append('container', selectedContainer.value)
  form.append('directory', currentDirectory.value)
  form.append('file', uploadFile.value)
  uploading.value = true
  uploadProgress.value = 0
  try {
    const result = await uploadK8sPodFile(form, (event) => {
      if (event.total) uploadProgress.value = Math.min(100, Math.round(event.loaded * 100 / event.total))
    })
    const destination = `${result.directory}/${result.filename}`.replace(/\/+/g, '/')
    term?.writeln(`\r\n\x1b[32m上传完成：${result.filename} → ${destination}\x1b[0m`)
    ElMessage.success('文件已上传到当前目录')
    uploadDialogVisible.value = false
  } catch (error) {
    const message = error.message || '文件上传失败'
    term?.writeln(`\r\n\x1b[31m文件上传失败：${message}\x1b[0m`)
  } finally {
    uploading.value = false
  }
}

function goBack() {
  window.dispatchEvent(new CustomEvent('ops-admin:close-tab-request', {
    detail: { path: route.path, nextPath: '/containers/k8s/pods' }
  }))
}

function disposeTerminal() {
  terminalGeneration += 1
  cancelAnimationFrame(resizeFrame)
  resizeFrame = undefined
  resizeObserver?.disconnect()
  resizeObserver = undefined
  inputDisposable?.dispose()
  inputDisposable = undefined
  disconnectTerminal(true)
  term?.dispose()
  term = undefined
}

async function initializeTerminal() {
  if (terminalInitialization) return terminalInitialization
  const generation = terminalGeneration
  terminalInitialization = (async () => {
  await nextTick()
    if (generation !== terminalGeneration) return
    if (!term) {
      createTerminal()
    }
  try {
      await loadContainers()
      if (generation !== terminalGeneration) return
      if (selectedContainer.value) {
      connectTerminal()
    }
  } catch (error) {
    ElMessage.error(error.message || '获取 Pod 容器失败')
  }
  })()
  try {
    await terminalInitialization
  } finally {
    terminalInitialization = undefined
  }
}

function handleTabClosed(event) {
  if (event.detail?.path !== terminalRoutePath) return
  disposeTerminal()
}

onMounted(() => {
  terminalRoutePath = route.path
  window.addEventListener('ops-admin:tab-closed', handleTabClosed)
  initializeTerminal()
})

onActivated(() => {
  if (term) {
    scheduleTerminalSizeSync()
    term.focus()
    return
  }
  initializeTerminal()
})

onBeforeUnmount(() => {
  window.removeEventListener('ops-admin:tab-closed', handleTabClosed)
  disposeTerminal()
})
</script>

<template>
  <div class="pod-terminal-page">
    <section class="terminal-shell">
      <header class="terminal-head">
        <div class="title-row">
          <el-button text @click="goBack">
            <el-icon><Back /></el-icon>
            返回 Pod 管理
          </el-button>
          <div class="title-block">
            <span class="title-label">
              <el-icon><Monitor /></el-icon>
              Pod 终端
            </span>
            <strong>{{ namespace }}/{{ podName }}</strong>
            <span class="terminal-shortcut-hint">Tab 补全命令或路径</span>
          </div>
          <el-button class="upload-current-button" :disabled="!connected" @click="openUploadDialog">
            <el-icon><UploadFilled /></el-icon>
            上传文件
          </el-button>
        </div>

        <div class="toolbar">
          <el-select
            v-model="selectedContainer"
            class="container-select"
            placeholder="选择容器"
            filterable
            :disabled="!containers.length"
            @change="handleContainerChange"
          >
            <el-option v-for="item in containers" :key="item" :label="item" :value="item" />
          </el-select>
          <el-button :loading="connecting" type="primary" @click="connectTerminal">连接</el-button>
          <el-button @click="handleManualDisconnect">断开</el-button>
          <el-button @click="clearTerminal">清屏</el-button>
        </div>
      </header>

      <div ref="terminalBoxRef" class="terminal-stage">
        <div ref="terminalRef" class="terminal-body" />
      </div>

      <el-dialog v-model="uploadDialogVisible" title="上传到当前目录" width="480px" :close-on-click-modal="false" append-to-body>
        <div class="upload-target">
          <span>目标目录</span>
          <code>{{ currentDirectory }}</code>
        </div>
        <p class="upload-description">容器：{{ selectedContainer }}。仅支持单个文件，最大 50 MB。</p>
        <input ref="uploadInputRef" class="upload-file-input" type="file" @change="handleUploadFileChange" />
        <button class="upload-picker" type="button" @click="chooseUploadFile">
          <el-icon><UploadFilled /></el-icon>
          <span>{{ uploadFile ? '重新选择文件' : '选择文件' }}</span>
          <small>文件会直接上传到当前终端目录</small>
        </button>
        <div v-if="uploadFile" class="upload-file-summary"><span>{{ uploadFile.name }}</span><small>{{ formatFileSize(uploadFile.size) }}</small></div>
        <el-progress v-if="uploading" :percentage="uploadProgress" :stroke-width="7" />
        <template #footer><el-button :disabled="uploading" @click="uploadDialogVisible = false">取消</el-button><el-button type="primary" :loading="uploading" @click="submitUpload">上传文件</el-button></template>
      </el-dialog>
    </section>
  </div>
</template>

<style scoped>
.pod-terminal-page {
  display: flex;
  height: calc(100vh - 190px);
  min-height: 680px;
}

.terminal-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  border: 1px solid #dbe5f0;
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
}

.terminal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
  border-bottom: 1px solid #e6edf5;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}

.title-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.terminal-shortcut-hint {
  color: #8492a7;
  font-size: 12px;
}

.title-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #6b7280;
  font-size: 13px;
}

.title-block strong {
  color: #111827;
  font-size: 18px;
  line-height: 1.3;
}

.upload-current-button {
  margin-left: 14px;
  border-color: #b8cdf4;
  color: #356fd0;
  background: #f6f9ff;
}

.upload-current-button:not(:disabled):hover {
  border-color: #6598ec;
  color: #1f5dc4;
  background: #edf4ff;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.container-select {
  width: 220px;
}

.terminal-stage {
  display: flex;
  flex: 1;
  min-height: 0;
  padding: 12px;
  background: #07111f;
}

.terminal-body {
  display: flex;
  flex: 1;
  min-height: 0;
  width: 100%;
  height: 100%;
}

.terminal-body :deep(.xterm) {
  width: 100%;
  height: 100%;
}

.upload-target {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 13px;
  border: 1px solid #dce8f8;
  border-radius: 6px;
  background: #f7faff;
}

.upload-target span,
.upload-file-summary small,
.upload-description {
  color: #7d8fa8;
  font-size: 13px;
}

.upload-target code {
  overflow: hidden;
  color: #2f68c5;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload-description { margin: 13px 0; }
.upload-file-input { display: none; }

.upload-picker {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  min-height: 84px;
  border: 1px dashed #a9c4ed;
  border-radius: 7px;
  color: #3975d1;
  background: #fbfdff;
  cursor: pointer;
}

.upload-picker:hover { border-color: #5c92e6; background: #f4f8ff; }
.upload-picker small { color: #8b9cb3; }

.upload-file-summary {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  background: #f6f8fc;
  color: #425672;
  font-size: 13px;
}

.upload-file-summary span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.upload-file-summary small { flex: 0 0 auto; }

@media (max-width: 960px) {
  .terminal-head {
    flex-direction: column;
    align-items: stretch;
  }

  .title-row,
  .toolbar {
    width: 100%;
  }

  .toolbar {
    flex-wrap: wrap;
  }

  .container-select {
    width: 100%;
  }

  .upload-current-button { margin-left: 0; }
}
</style>
