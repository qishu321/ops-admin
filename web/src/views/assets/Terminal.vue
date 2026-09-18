<script setup>
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { ElMessage } from 'element-plus'
import { Download, Folder, Monitor, Refresh, UploadFilled } from '@element-plus/icons-vue'
import { queryAssetHostGroupList, queryAssetHostList, uploadAssetTerminalFile, downloadAssetTerminalFile, queryAssetTerminalFiles } from '../../api/asset'
import { getToken } from '../../utils/auth'

const loading = ref(false)
const treeRef = ref()
const terminalBoxRef = ref()
const groupTree = ref([])
const sessions = ref([])
const activeSessionId = ref()
const query = reactive({ keyword: '' })
const uploadDialogVisible = ref(false)
const uploadDirectory = ref('~')
const uploadFile = ref()
const uploadInputRef = ref()
const uploading = ref(false)
const uploadProgress = ref(0)
const downloadDialogVisible = ref(false)
const downloadDirectory = ref('~')
const downloadParent = ref('')
const downloadItems = ref([])
const downloadLoading = ref(false)
const downloading = ref(false)
const MAX_UPLOAD_SIZE = 500 * 1024 * 1024

// xterm and WebSocket instances must stay outside Vue reactivity. Each opened
// host has an independent runtime, so switching a tab never disconnects it.
const runtimes = new Map()
const terminalElements = new Map()
let resizeObserver
let resizeFrame

const activeSession = computed(() => sessions.value.find((item) => item.id === activeSessionId.value))

async function loadTree() {
  loading.value = true
  try {
    const [groupsRes, hostsRes] = await Promise.all([
      queryAssetHostGroupList({ keyword: query.keyword }),
      queryAssetHostList({ pageNum: 1, pageSize: 1000, keyword: query.keyword })
    ])
    groupTree.value = buildTree(groupsRes.list || [], hostsRes.list || [])
  } finally {
    loading.value = false
  }
}

function buildTree(groups, hosts) {
  const nodes = new Map()
  const roots = []
  groups.forEach((group) => {
    nodes.set(group.id, { id: `group-${group.id}`, rawId: group.id, parentId: group.parentId, type: 'group', label: group.name, children: [] })
  })
  nodes.forEach((node) => {
    if (node.parentId && nodes.has(node.parentId)) nodes.get(node.parentId).children.push(node)
    else roots.push(node)
  })
  hosts.forEach((host) => {
    const hostNode = { id: `host-${host.id}`, rawId: host.id, type: 'host', label: host.hostName || host.sshIp, host, children: [] }
    const hostGroups = Array.isArray(host.hostGroups) && host.hostGroups.length ? host.hostGroups : host.groupId ? [{ id: host.groupId }] : []
    if (hostGroups.length) {
      hostGroups.forEach((item) => {
        const group = nodes.get(item.id)
        if (group) group.children.push({ ...hostNode, id: `${hostNode.id}-g-${item.id}` })
      })
    } else roots.push(hostNode)
  })
  return roots
}

function handleNodeClick(node) {
  if (node.type === 'host') openHost(node.host)
}

function getSessionId(host) { return `host-${host.id}` }

async function openHost(host) {
  const id = getSessionId(host)
  if (sessions.value.some((item) => item.id === id)) return activateSession(id)
  sessions.value.push({ id, host, status: 'connecting' })
  activeSessionId.value = id
  await nextTick()
  bindTerminalResize()
  createTerminal(id)
  connectSocket(id)
}

function setTerminalElement(id, element) {
  if (element) terminalElements.set(id, element)
  else terminalElements.delete(id)
}

function createTerminal(id) {
  const element = terminalElements.get(id)
  if (!element || runtimes.has(id)) return
  const term = new Terminal({
    cursorBlink: true,
    convertEol: true,
    rows: 34,
    cols: 150,
    fontSize: 13,
    fontFamily: 'Consolas, "Courier New", monospace',
    theme: { background: '#050000', foreground: '#e6edf3', cursor: '#00ff88', green: '#00ff88', brightGreen: '#23ff9a', red: '#ff4d4f' }
  })
  const runtime = { term, socket: undefined, inputDisposable: undefined, commandLine: '', promptBuffer: '', currentDirectory: '~' }
  runtimes.set(id, runtime)
  term.open(element)
  runtime.inputDisposable = term.onData((data) => {
    if (runtime.socket?.readyState !== WebSocket.OPEN) return
    if (shouldBlockZmodemCommand(runtime, data)) {
      runtime.socket.send('\x15')
      runtime.term.writeln('\r\n\x1b[33mWeb 终端不支持 rz / sz 命令，请使用上传文件或下载文件功能。\x1b[0m')
      return
    }
    runtime.socket.send(data)
  })
  term.focus()
  scheduleTerminalSizeSync()
}

function shouldBlockZmodemCommand(runtime, data) {
  const text = String(data || '')
  if (!/[\r\n]/.test(text)) {
    runtime.commandLine = `${runtime.commandLine}${text}`.slice(-1024)
    return false
  }
  const lines = `${runtime.commandLine}${text}`.split(/[\r\n]/)
  runtime.commandLine = lines.at(-1) || ''
  return lines.slice(0, -1).some((line) => /^\s*(?:rz|sz)(?:\s|$)/i.test(line))
}

function trackPromptDirectory(runtime, output) {
  // A terminal prompt is not a protocol. It can include carriage returns,
  // OSC titles and BEL (\x07). Never let those control bytes become part of a
  // filesystem path sent to the transfer API.
  runtime.promptBuffer = `${runtime.promptBuffer}${String(output || '')}`
    .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')
    .replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '')
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, '')
    .replace(/\r/g, '\n')
    .slice(-2048)
  const matches = [...runtime.promptBuffer.matchAll(/(?:^|\n)[A-Za-z0-9._-]+@[A-Za-z0-9._-]+:([~/][^\r\n$#]*)[$#]\s*/g)]
  const directory = matches.at(-1)?.[1]?.trim()
  if (isSafeTerminalDirectory(directory)) runtime.currentDirectory = directory
}

function isSafeTerminalDirectory(directory) {
  return typeof directory === 'string' && /^(?:~(?:\/[^\x00-\x1F:@]*)?|\/[^\x00-\x1F:@]*)$/.test(directory)
}

function currentTerminalDirectory(runtime) {
  return isSafeTerminalDirectory(runtime?.currentDirectory) ? runtime.currentDirectory : '~'
}

function updateSession(id, patch) {
  const session = sessions.value.find((item) => item.id === id)
  if (session) Object.assign(session, patch)
}

function connectSocket(id) {
  const session = sessions.value.find((item) => item.id === id)
  const runtime = runtimes.get(id)
  if (!session || !runtime) return
  disconnectSession(id, false)
  updateSession(id, { status: 'connecting' })
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const token = encodeURIComponent(getToken())
  const url = `${protocol}://${window.location.host}/api/v1/asset/terminal/ws?hostId=${session.host.id}&rows=${runtime.term.rows || 34}&cols=${runtime.term.cols || 150}&token=${token}`
  const currentSocket = new WebSocket(url)
  runtime.socket = currentSocket
  currentSocket.onopen = () => {
    if (runtime.socket !== currentSocket) return
    updateSession(id, { status: 'connected' })
    runtime.term.writeln(`\x1b[32m欢迎使用 SSH 终端，正在连接 ${session.host.sshIp || session.host.hostName} ...\x1b[0m`)
    if (activeSessionId.value === id) {
      scheduleTerminalSizeSync()
      runtime.term.focus()
    }
  }
  currentSocket.onmessage = (event) => {
    if (runtime.socket === currentSocket) {
      trackPromptDirectory(runtime, event.data)
      runtime.term.write(event.data)
    }
  }
  currentSocket.onerror = () => {
    if (runtime.socket !== currentSocket) return
    updateSession(id, { status: 'error' })
    runtime.term.writeln('\r\n\x1b[31mSSH 连接异常，请检查主机、端口和认证凭据。\x1b[0m')
  }
  currentSocket.onclose = () => {
    if (runtime.socket !== currentSocket) return
    runtime.socket = undefined
    updateSession(id, { status: 'disconnected' })
    runtime.term.writeln('\r\n\x1b[33m连接已断开。\x1b[0m')
  }
}

function disconnectSession(id, showMessage = true) {
  const runtime = runtimes.get(id)
  if (!runtime?.socket) return
  const currentSocket = runtime.socket
  runtime.socket = undefined
  currentSocket.onopen = null
  currentSocket.onmessage = null
  currentSocket.onerror = null
  currentSocket.onclose = null
  currentSocket.close()
  updateSession(id, { status: 'disconnected' })
  if (showMessage) runtime.term.writeln('\r\n\x1b[33m连接已断开。\x1b[0m')
}

function activateSession(id) {
  activeSessionId.value = id
  nextTick(() => {
    scheduleTerminalSizeSync()
    runtimes.get(id)?.term.focus()
  })
}

function reconnect() {
  if (!activeSession.value) return ElMessage.warning('请先选择一台主机')
  connectSocket(activeSession.value.id)
}
function disconnect() { if (activeSession.value) disconnectSession(activeSession.value.id) }
function clearTerminal() { if (activeSession.value) runtimes.get(activeSession.value.id)?.term.clear() }

function openUploadDialog() {
  const runtime = activeSessionId.value && runtimes.get(activeSessionId.value)
  if (!activeSession.value || !runtime?.socket || runtime.socket.readyState !== WebSocket.OPEN) return ElMessage.warning('请先连接 SSH 终端')
  uploadDirectory.value = currentTerminalDirectory(runtime)
  uploadFile.value = undefined
  uploadProgress.value = 0
  uploadDialogVisible.value = true
}
async function openDownloadDialog() {
  const runtime = activeSessionId.value && runtimes.get(activeSessionId.value)
  if (!activeSession.value || !runtime?.socket || runtime.socket.readyState !== WebSocket.OPEN) return ElMessage.warning('请先连接 SSH 终端')
  downloadDirectory.value = currentTerminalDirectory(runtime)
  downloadParent.value = ''
  downloadItems.value = []
  downloadDialogVisible.value = true
  await loadDownloadDirectory(downloadDirectory.value)
}
function chooseUploadFile() { uploadInputRef.value?.click() }
function handleUploadFileChange(event) {
  const file = event.target.files?.[0]; event.target.value = ''
  if (!file) return
  if (file.size > MAX_UPLOAD_SIZE) return ElMessage.error('单个文件不能超过 500 MB')
  uploadFile.value = file
}
function formatFileSize(size = 0) { if (!size) return '0 B'; return size >= 1024 * 1024 ? `${(size / 1024 / 1024).toFixed(2)} MB` : `${Math.max(1, Math.ceil(size / 1024))} KB` }
async function submitUpload() {
  if (!uploadFile.value) return ElMessage.warning('请选择一个文件')
  if (!isSafeTerminalDirectory(uploadDirectory.value)) return ElMessage.error('目标目录格式异常，请填写绝对路径或以 ~/ 开头的路径')
  const form = new FormData(); form.append('hostId', String(activeSession.value.host.id)); form.append('directory', uploadDirectory.value); form.append('file', uploadFile.value)
  uploading.value = true; uploadProgress.value = 0
  try {
    const result = await uploadAssetTerminalFile(form, (event) => { if (event.total) uploadProgress.value = Math.round(event.loaded * 100 / event.total) })
    const runtime = runtimes.get(activeSessionId.value)
    runtime?.term.writeln(`\r\n\x1b[32m上传完成：${result.filename} → ${result.destination || uploadDirectory.value}\x1b[0m`)
    uploadDialogVisible.value = false; ElMessage.success('文件已上传')
  } finally { uploading.value = false }
}
async function loadDownloadDirectory(directory = downloadDirectory.value) {
  if (!activeSession.value || !directory) return
  downloadLoading.value = true
  try {
    const result = await queryAssetTerminalFiles(activeSession.value.host.id, directory)
    downloadDirectory.value = result.path
    downloadParent.value = result.parent || ''
    downloadItems.value = result.items || []
  } finally { downloadLoading.value = false }
}
function openDownloadEntry(entry) {
  if (entry.directory) loadDownloadDirectory(entry.path)
}
async function submitDownload(entry) {
  if (!entry || entry.directory) return
  downloading.value = true
  try {
    const response = await downloadAssetTerminalFile(activeSession.value.host.id, entry.path)
    // Empty files are valid downloads. The backend validates the remote target
    // before it writes attachment headers, so an error is returned as HTTP 400
    // instead of being saved by the browser as a fake 0 B download.
    if (!response.data) throw new Error('下载响应为空')
    const url = URL.createObjectURL(response.data)
    const link = document.createElement('a')
    link.href = url
    link.download = entry.name || 'download'
    document.body.appendChild(link); link.click(); link.remove(); URL.revokeObjectURL(url)
    const runtime = runtimes.get(activeSessionId.value)
    runtime?.term.writeln(`\r\n\x1b[32m已开始下载：${entry.path}\x1b[0m`)
    ElMessage.success(`已开始下载 ${entry.name}`)
  } finally { downloading.value = false }
}

function closeSession(id = activeSessionId.value) {
  const index = sessions.value.findIndex((item) => item.id === id)
  if (index < 0) return
  disposeRuntime(id)
  sessions.value.splice(index, 1)
  if (activeSessionId.value === id) {
    activeSessionId.value = sessions.value[index]?.id || sessions.value[index - 1]?.id
    nextTick(() => {
      scheduleTerminalSizeSync()
      activeSessionId.value && runtimes.get(activeSessionId.value)?.term.focus()
    })
  }
}

function closeOtherSessions() {
  if (!activeSessionId.value) return
  sessions.value.filter((item) => item.id !== activeSessionId.value).forEach((item) => disposeRuntime(item.id))
  sessions.value = sessions.value.filter((item) => item.id === activeSessionId.value)
  ElMessage.success('已关闭其他终端会话')
}

function disposeRuntime(id) {
  const runtime = runtimes.get(id)
  if (!runtime) return
  disconnectSession(id, false)
  runtime.inputDisposable?.dispose()
  runtime.term?.dispose()
  runtimes.delete(id)
  terminalElements.delete(id)
}

function closeAllSessions() {
  sessions.value.forEach((item) => disposeRuntime(item.id))
  sessions.value = []
  activeSessionId.value = undefined
}

function bindTerminalResize() {
  if (!terminalBoxRef.value || resizeObserver) return
  resizeObserver = new ResizeObserver(scheduleTerminalSizeSync)
  resizeObserver.observe(terminalBoxRef.value)
}
function scheduleTerminalSizeSync() {
  cancelAnimationFrame(resizeFrame)
  resizeFrame = requestAnimationFrame(syncTerminalSize)
}
function syncTerminalSize() {
  const id = activeSessionId.value
  const runtime = id && runtimes.get(id)
  const element = id && terminalElements.get(id)
  if (!runtime || !element) return
  const style = window.getComputedStyle(element)
  const width = element.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
  const height = element.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom)
  if (!width || !height) return
  const screen = element.querySelector('.xterm-screen')
  const cellWidth = screen?.clientWidth ? screen.clientWidth / runtime.term.cols : 8.2
  const cellHeight = screen?.clientHeight ? screen.clientHeight / runtime.term.rows : 18
  const cols = Math.max(80, Math.floor(width / cellWidth))
  const rows = Math.max(20, Math.floor(height / cellHeight))
  if (runtime.term.cols !== cols || runtime.term.rows !== rows) runtime.term.resize(cols, rows)
}

function handleTabClosed(event) { if (event.detail?.path === '/assets/terminal') closeAllSessions() }
onMounted(() => {
  window.addEventListener('ops-admin:tab-closed', handleTabClosed)
  loadTree()
})
onActivated(() => {
  bindTerminalResize()
  scheduleTerminalSizeSync()
  activeSessionId.value && runtimes.get(activeSessionId.value)?.term.focus()
})
onBeforeUnmount(() => {
  window.removeEventListener('ops-admin:tab-closed', handleTabClosed)
  resizeObserver?.disconnect()
  cancelAnimationFrame(resizeFrame)
  closeAllSessions()
})
</script>

<template>
  <div class="terminal-page">
    <aside class="asset-tree-card">
      <h3>资产分组</h3>
      <el-input v-model="query.keyword" clearable placeholder="搜索分组 / 主机" class="tree-search" @keyup.enter="loadTree" @clear="loadTree" />
      <el-tree ref="treeRef" v-loading="loading" :data="groupTree" node-key="id" default-expand-all :expand-on-click-node="false" class="asset-tree" @node-click="handleNodeClick">
        <template #default="{ data }">
          <span :class="['tree-node', data.type, { active: data.type === 'host' && activeSession?.host.id === data.host?.id }]">
            <el-icon v-if="data.type === 'group'"><Folder /></el-icon>
            <el-icon v-else><Monitor /></el-icon>
            <span>{{ data.label }}</span>
          </span>
        </template>
      </el-tree>
    </aside>

    <section v-if="sessions.length" ref="terminalBoxRef" class="terminal-window">
      <header class="terminal-titlebar">
        <div><strong>SSH 终端</strong><span class="session-count">已打开 {{ sessions.length }} 个会话</span></div>
        <el-dropdown trigger="click" @command="(command) => command === 'closeOthers' && closeOtherSessions()">
          <button class="terminal-menu" aria-label="终端会话操作">•••</button>
          <template #dropdown><el-dropdown-menu><el-dropdown-item command="closeOthers" :disabled="sessions.length < 2">关闭其他会话</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
      </header>
      <div class="terminal-tabs" role="tablist" aria-label="终端会话">
        <button v-for="session in sessions" :key="session.id" :class="['terminal-tab', { active: session.id === activeSessionId }]" role="tab" :aria-selected="session.id === activeSessionId" @click="activateSession(session.id)">
          <span :class="['connection-dot', session.status]" />
          <span class="terminal-tab-label">{{ session.host.sshIp || session.host.hostName }}</span>
          <span class="terminal-tab-close" title="关闭终端" @click.stop="closeSession(session.id)">×</span>
        </button>
      </div>
      <div class="terminal-toolbar">
        <span class="active-host">{{ activeSession?.host.hostName || activeSession?.host.sshIp }}</span>
        <div class="terminal-transfer-actions"><button type="button" class="terminal-transfer-button" @click="openUploadDialog"><el-icon><UploadFilled /></el-icon>上传文件</button><button type="button" class="terminal-transfer-button" @click="openDownloadDialog"><el-icon><Download /></el-icon>下载文件</button></div>
        <div class="terminal-actions">
          <el-button size="small" color="#00d084" plain @click="reconnect">重新连接</el-button>
          <el-button size="small" color="#00d084" plain @click="disconnect">断开</el-button>
          <el-button size="small" color="#00d084" plain @click="clearTerminal">清屏</el-button>
          <el-button size="small" type="danger" plain @click="closeSession()">关闭</el-button>
        </div>
      </div>
      <el-dialog v-model="uploadDialogVisible" title="上传文件到主机" width="480px" :close-on-click-modal="false" append-to-body>
        <el-form label-width="88px"><el-form-item label="目标目录"><el-input v-model="uploadDirectory" placeholder="例如：/opt/app 或 ~" /></el-form-item></el-form>
        <p class="terminal-upload-hint">单次仅支持一个文件，最大 500 MB。</p>
        <input ref="uploadInputRef" class="terminal-upload-input" type="file" @change="handleUploadFileChange" />
        <button class="terminal-upload-picker" type="button" @click="chooseUploadFile"><el-icon><UploadFilled /></el-icon>{{ uploadFile ? '重新选择文件' : '选择文件' }}</button>
        <div v-if="uploadFile" class="terminal-upload-file"><span>{{ uploadFile.name }}</span><small>{{ formatFileSize(uploadFile.size) }}</small></div>
        <el-progress v-if="uploading" :percentage="uploadProgress" :stroke-width="7" />
        <template #footer><el-button :disabled="uploading" @click="uploadDialogVisible = false">取消</el-button><el-button type="primary" :loading="uploading" @click="submitUpload">上传文件</el-button></template>
      </el-dialog>
      <el-dialog v-model="downloadDialogVisible" title="选择要下载的文件" width="680px" :close-on-click-modal="false" append-to-body>
        <div class="terminal-file-browser-toolbar"><span class="terminal-file-path" :title="downloadDirectory">{{ downloadDirectory }}</span><div><el-button size="small" :disabled="!downloadParent || downloadLoading" @click="loadDownloadDirectory(downloadParent)">上一级</el-button><el-button size="small" :icon="Refresh" :loading="downloadLoading" @click="loadDownloadDirectory()">刷新</el-button></div></div>
        <p class="terminal-upload-hint">双击目录进入；选择文件后下载到本地。</p>
        <el-table v-loading="downloadLoading" :data="downloadItems" max-height="350" class="terminal-file-table" empty-text="当前目录没有可下载的普通文件">
          <el-table-column label="名称" min-width="270"><template #default="{ row }"><button class="terminal-file-name" :class="{ directory: row.directory }" @dblclick="openDownloadEntry(row)" @click="row.directory && openDownloadEntry(row)"><el-icon><Folder v-if="row.directory" /><Download v-else /></el-icon>{{ row.name }}</button></template></el-table-column>
          <el-table-column label="大小" width="110"><template #default="{ row }">{{ row.directory ? '—' : formatFileSize(row.size) }}</template></el-table-column>
          <el-table-column prop="updatedAt" label="修改时间" width="150" />
          <el-table-column label="操作" width="88"><template #default="{ row }"><el-button v-if="!row.directory" link type="primary" :loading="downloading" @click="submitDownload(row)">下载</el-button><el-button v-else link type="primary" @click="openDownloadEntry(row)">进入</el-button></template></el-table-column>
        </el-table>
        <template #footer><el-button :disabled="downloading" @click="downloadDialogVisible = false">关闭</el-button></template>
      </el-dialog>
      <div v-for="session in sessions" :key="`screen-${session.id}`" v-show="session.id === activeSessionId" class="terminal-stage">
        <div :ref="(element) => setTerminalElement(session.id, element)" class="terminal-body" />
      </div>
    </section>

    <section v-else class="terminal-empty"><div><h2>终端登录</h2><p>在左侧资产分组中选择主机即可新开终端会话；已打开的会话会保留在上方标签中，方便同时查看多台服务器。</p></div></section>
  </div>
</template>

<style scoped>
.terminal-page { display: grid; grid-template-columns: 300px minmax(0, 1fr); gap: 12px; height: calc(100vh - 190px); min-height: 680px; }
.asset-tree-card { padding: 28px 20px; border-radius: 4px; background: #142230; color: #00b96b; box-shadow: 0 18px 40px rgba(18, 33, 49, .18); }
.asset-tree-card h3 { margin: 0 0 18px; color: #00c875; font-size: 20px; }.tree-search { margin-bottom: 18px; }
.asset-tree { --el-tree-node-hover-bg-color: rgba(0, 185, 107, .1); background: transparent; color: #00b96b; }
.tree-node { display: inline-flex; align-items: center; gap: 8px; border-radius: 4px; font-weight: 700; }.tree-node.host { color: #a3b600; }.tree-node.host.active { color: #16d995; }
.terminal-window { display: flex; flex-direction: column; min-height: 0; overflow: hidden; border-radius: 12px 12px 0 0; background: #20384c; box-shadow: 0 22px 48px rgba(7, 20, 35, .28); }
.terminal-titlebar { display: flex; align-items: center; justify-content: space-between; min-height: 58px; padding: 0 18px 0 22px; border-bottom: 1px solid rgba(0, 208, 132, .6); color: #00ff88; font-size: 18px; }.session-count { margin-left: 12px; color: #9db0c1; font-size: 12px; font-weight: 500; }
.terminal-menu { width: 32px; height: 30px; border: 1px solid rgba(157, 176, 193, .45); border-radius: 6px; background: transparent; color: #c7d4dc; cursor: pointer; font-weight: 700; letter-spacing: 1px; }.terminal-menu:hover { border-color: #00d084; color: #00ff88; }
.terminal-tabs { display: flex; min-height: 44px; gap: 4px; overflow-x: auto; padding: 6px 10px 0; border-bottom: 1px solid rgba(111, 140, 164, .35); background: #172b3a; }
.terminal-tab { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 8px; max-width: 240px; padding: 0 10px; border: 1px solid transparent; border-bottom: 0; border-radius: 7px 7px 0 0; background: transparent; color: #aabac7; cursor: pointer; font-size: 13px; }.terminal-tab:hover { background: rgba(0, 208, 132, .08); color: #ecf6f1; }.terminal-tab.active { border-color: rgba(0, 208, 132, .55); background: #20384c; color: #f4fffa; }.terminal-tab-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.connection-dot { width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; background: #8393a5; }.connection-dot.connecting { background: #f2c94c; box-shadow: 0 0 0 3px rgba(242, 201, 76, .14); }.connection-dot.connected { background: #00d084; box-shadow: 0 0 0 3px rgba(0, 208, 132, .14); }.connection-dot.error { background: #ff6b6b; }
.terminal-tab-close { display: inline-grid; width: 18px; height: 18px; place-items: center; border-radius: 4px; color: #91a2b1; font-size: 18px; line-height: 1; }.terminal-tab-close:hover { background: rgba(255, 77, 79, .18); color: #ff8a8c; }
.terminal-toolbar { display: flex; align-items: center; gap: 12px; min-height: 52px; padding: 0 12px; border-bottom: 1px solid rgba(0, 208, 132, .6); }.active-host { overflow: hidden; color: #d6e4ec; font-size: 13px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }.terminal-actions { display: flex; flex: 0 0 auto; gap: 10px; margin-left: auto; }
.terminal-transfer-actions { display: flex; flex: 0 0 auto; gap: 8px; margin-left: 8px; }.terminal-transfer-button { display: inline-flex; align-items: center; gap: 5px; height: 29px; padding: 0 10px; border: 1px solid rgba(0, 208, 132, .72); border-radius: 5px; color: #00b978; background: rgba(0, 208, 132, .08); cursor: pointer; font-size: 12px; }.terminal-transfer-button:hover { color: #d9fff0; background: rgba(0, 208, 132, .25); }.terminal-upload-hint { margin: 0 0 14px; color: #72849a; font-size: 13px; }.terminal-upload-input { display: none; }.terminal-upload-picker { display: flex; align-items: center; justify-content: center; gap: 8px; width: 100%; min-height: 76px; border: 1px dashed #7dbca8; border-radius: 7px; color: #159669; background: #f3fcf8; cursor: pointer; font-size: 14px; }.terminal-upload-picker:hover { border-color: #00a878; background: #ebfaf3; }.terminal-upload-file { display: flex; justify-content: space-between; gap: 12px; margin-top: 12px; padding: 10px 12px; border-radius: 6px; background: #f3f7f6; color: #42566a; font-size: 13px; }.terminal-upload-file span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.terminal-upload-file small { flex: 0 0 auto; color: #8393a5; }.terminal-file-browser-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; padding: 9px 10px; border: 1px solid #e0e8f2; border-radius: 6px; background: #f7faff; }.terminal-file-path { overflow: hidden; color: #36506b; font-family: Consolas, monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }.terminal-file-table { border: 1px solid #e5edf5; border-radius: 6px; }.terminal-file-name { display: inline-flex; align-items: center; gap: 7px; max-width: 100%; border: 0; background: transparent; color: #3f5267; cursor: pointer; font: inherit; }.terminal-file-name.directory { color: #2472d7; font-weight: 600; }.terminal-file-name:hover { color: #409eff; }
.terminal-stage { display: flex; flex: 1; min-height: 0; }.terminal-body { display: flex; flex: 1; min-height: 0; padding: 10px 12px; box-sizing: border-box; background: #050000; overflow: hidden; }.terminal-body :deep(.xterm) { width: 100%; height: 100%; }
.terminal-empty { display: grid; place-items: center; border-radius: 12px; background: linear-gradient(135deg, #243d70, #466df4); color: #fff; }.terminal-empty h2 { margin: 0 0 10px; font-size: 34px; }.terminal-empty p { max-width: 580px; margin: 0; color: rgba(255, 255, 255, .8); line-height: 1.7; }
@media (max-width: 960px) { .terminal-page { grid-template-columns: 240px minmax(0, 1fr); }.terminal-toolbar { align-items: flex-start; flex-direction: column; padding: 10px 12px; } }
</style>
