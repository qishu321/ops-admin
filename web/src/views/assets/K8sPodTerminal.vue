<script setup>
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Back, Monitor } from '@element-plus/icons-vue'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { buildK8sPodTerminalWSUrl, queryK8sPodContainers } from '../../api/k8s'

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

let term
let socket
let inputDisposable
let resizeObserver
let resizeFrame
let connectionHintTimer
let terminalRoutePath = ''
let terminalInitialization
let terminalGeneration = 0

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
  disconnectTerminal()
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
        term?.write(payload.data)
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
    term?.write(event.data)
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

function disconnectTerminal() {
  clearConnectionHintTimer()
  if (socket) {
    socket.onclose = null
    socket.close()
    socket = undefined
  }
  connected.value = false
  connecting.value = false
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

function goBack() {
  router.push('/containers/k8s/pods')
}

function disposeTerminal() {
  terminalGeneration += 1
  cancelAnimationFrame(resizeFrame)
  resizeFrame = undefined
  resizeObserver?.disconnect()
  resizeObserver = undefined
  inputDisposable?.dispose()
  inputDisposable = undefined
  disconnectTerminal()
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
          <el-button @click="disconnectTerminal">断开</el-button>
          <el-button @click="clearTerminal">清屏</el-button>
        </div>
      </header>

      <div ref="terminalBoxRef" class="terminal-stage">
        <div ref="terminalRef" class="terminal-body" />
      </div>
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
}
</style>
