import assert from 'node:assert/strict'
import test from 'node:test'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'
import { computed, ref, reactive, nextTick, onActivated, onMounted, onBeforeUnmount, createRenderer, defineComponent, h, KeepAlive } from 'vue'

const page = readFileSync(new URL('../src/views/assets/Terminal.vue', import.meta.url), 'utf8')
const setup = page.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm, '')
const renderer = createRenderer({
  createElement: () => ({ children: [] }), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, patchProp() {},
  insert(node, parent) { parent.children ||= []; parent.children.push(node); node.parent = parent },
  remove(node) { if (node.parent) node.parent.children = node.parent.children.filter(item => item !== node) },
  parentNode: node => node.parent, nextSibling: () => null,
})

function mountTerminal(adapters = {}) {
  const events = new EventTarget()
  events.location = { protocol: 'http:', host: 'localhost' }
  let closes = 0, creates = 0, state
  class Socket {
    static OPEN = 1
    readyState = 1
    sent = []
    constructor() { creates++ }
    send(data) { this.sent.push(data) }
    close() { closes++; this.readyState = 3 }
  }
  class Xterm {
    rows = 30; cols = 120
    writes = []
    open() {} focus() {} dispose() { this.disposed = true }
    writeln(data) { this.writes.push(data) } write(data) { this.writes.push(data) }
    clear() { this.writes = [] }
    onData(fn) { this.input = fn; return { dispose: () => { this.input = undefined } } }
  }
  const context = vm.createContext({
    computed, ref, reactive, nextTick, onActivated, onMounted, onBeforeUnmount,
    window: events, WebSocket: Socket, Terminal: Xterm, TextDecoder, ArrayBuffer, Uint8Array, FormData,
    getToken: () => 'test-token', useAccessRefresh() {},
    ElMessage: { warning() {}, error() {}, success() {} },
    requestAnimationFrame: () => 1, cancelAnimationFrame() {},
    ResizeObserver: class { observe() {} disconnect() {} },
    queryAssetHostGroupList: async () => ({list:[]}), queryAssetHostList: async () => ({list:[]}),
    ...adapters,
  })
  const TerminalPage = defineComponent({setup() {
    state = vm.runInContext(`${setup}; ({sessions, activeSessionId, runtimes, openHost, setTerminalElement, activateSession, closeSession, closeOtherSessions, reconnect, disconnect, clearTerminal, currentTerminalDirectory, openUploadDialog, uploadFile, uploadDirectory, submitUpload, openDownloadDialog, downloadDirectory, downloadItems, sessionLabel, openBroadcastDialog, sendBroadcastCommand, broadcastDialogVisible, broadcastTargets})`, context)
    return () => h('div', null, state.sessions.value.map(session => h('div', {key:session.id, ref:el => state.setTerminalElement(session.id, el)})))
  }})
  const visible = ref(true)
  const app = renderer.createApp({setup: () => () => h(KeepAlive, null, { default: () => visible.value ? h(TerminalPage, {key:'/assets/terminal'}) : h('span') })})
  app.mount({children:[]})
  return { state, visible, events, app, closes: () => closes, creates: () => creates }
}

test('switching away and back preserves the original terminal and WebSocket', async () => {
  const view = mountTerminal()
  await view.state.openHost({id:1,hostName:'test',sshIp:'192.0.2.1'})
  const id = view.state.sessions.value[0].id
  const runtime = view.state.runtimes.get(id)
  const socket = runtime.socket
  socket.onopen()
  view.visible.value = false; await nextTick()
  view.visible.value = true; await nextTick()
  assert.equal(view.creates(), 1)
  assert.equal(view.closes(), 0)
  assert.equal(view.state.runtimes.get(id), runtime)
  assert.equal(runtime.socket, socket)
  assert.equal(view.state.sessions.value[0].status, 'connected')
  view.app.unmount()
  assert.equal(view.closes(), 1)
})

test('explicitly closing the terminal tag still releases all session resources', async () => {
  const view = mountTerminal()
  await view.state.openHost({id:1,sshIp:'192.0.2.1'})
  const event = new Event('ops-admin:tab-closed')
  event.detail = {path:'/assets/terminal'}
  view.events.dispatchEvent(event)
  assert.equal(view.closes(), 1)
  assert.equal(view.state.runtimes.size, 0)
  assert.equal(view.state.sessions.value.length, 0)
  view.app.unmount()
})

test('three rapid opens of the same host create independent input, output and directory state', async (t) => {
  const view = mountTerminal()
  t.after(() => view.app.unmount())
  const host = {id:1,hostName:'test-host',sshIp:'192.0.2.1'}
  await Promise.all([view.state.openHost(host), view.state.openHost(host), view.state.openHost(host)])
  const sessions = view.state.sessions.value
  assert.equal(sessions.length, 3)
  assert.equal(new Set(sessions.map(session => session.id)).size, 3)
  assert.deepEqual(Array.from(sessions, session => session.number), [1, 2, 3])
  assert.deepEqual(Array.from(sessions, session => view.state.sessionLabel(session)), ['test-host -1', 'test-host -2', 'test-host -3'])
  assert.equal(view.creates(), 3)
  const runtimes = sessions.map(session => view.state.runtimes.get(session.id))
  runtimes.forEach((runtime, index) => {
    runtime.term.input(`command-${index}`)
    runtime.socket.onmessage({data:`root@host:/data/session-${index}# `})
  })
  runtimes.forEach((runtime, index) => {
    assert.deepEqual(runtime.socket.sent, [`command-${index}`])
    assert.deepEqual(runtime.term.writes, [`root@host:/data/session-${index}# `])
    assert.equal(view.state.currentTerminalDirectory(runtime), `/data/session-${index}`)
  })
  view.state.activateSession(sessions[0].id)
  view.state.clearTerminal()
  assert.equal(runtimes[0].term.writes.length, 0)
  assert.equal(runtimes[1].term.writes.length, 1)
  assert.equal(view.closes(), 0)
})

test('reconnect and close affect only one session and numbers are never reused', async (t) => {
  const view = mountTerminal()
  t.after(() => view.app.unmount())
  const host = {id:1,sshIp:'192.0.2.1'}
  for (let i = 0; i < 3; i++) await view.state.openHost(host)
  assert.equal(view.state.sessions.value.length, 3)
  const [first, second, third] = view.state.sessions.value
  const firstRuntime = view.state.runtimes.get(first.id)
  const secondRuntime = view.state.runtimes.get(second.id)
  const thirdRuntime = view.state.runtimes.get(third.id)
  const firstSocket = firstRuntime.socket, oldSecondSocket = secondRuntime.socket, thirdSocket = thirdRuntime.socket
  view.state.activateSession(second.id)
  view.state.reconnect()
  assert.equal(oldSecondSocket.readyState, 3)
  assert.notEqual(secondRuntime.socket, oldSecondSocket)
  assert.equal(firstRuntime.socket, firstSocket)
  assert.equal(thirdRuntime.socket, thirdSocket)
  view.state.closeSession(second.id)
  assert.equal(secondRuntime.term.disposed, true)
  assert.equal(firstSocket.readyState, 1)
  assert.equal(thirdSocket.readyState, 1)
  assert.equal(view.state.activeSessionId.value, third.id)
  await view.state.openHost(host)
  assert.equal(view.state.sessions.value.at(-1).number, 4)
  assert.notEqual(view.state.sessions.value.at(-1).id, second.id)
  view.state.closeOtherSessions()
  assert.equal(view.state.sessions.value.length, 1)
  assert.equal(firstSocket.readyState, 3)
  assert.equal(thirdSocket.readyState, 3)
  assert.equal(view.state.runtimes.size, 1)
  await view.state.openHost({id:2,hostName:'other',sshIp:'192.0.2.2'})
  assert.equal(view.state.sessions.value.at(-1).number, 1)
  assert.equal(view.state.sessionLabel(first), '192.0.2.1 -1')
})

test('upload completion stays in its originating session after switching same-host tabs', async (t) => {
  let finish, form
  const view = mountTerminal({uploadAssetTerminalFile: data => {
    form = data
    return new Promise(resolve => { finish = resolve })
  }})
  t.after(() => view.app.unmount())
  const host = {id:1,hostName:'test',sshIp:'192.0.2.1'}
  await view.state.openHost(host)
  await view.state.openHost(host)
  const [first, second] = view.state.sessions.value
  const a = view.state.runtimes.get(first.id), b = view.state.runtimes.get(second.id)
  a.socket.onmessage({data:'root@host:/data/first# '})
  b.socket.onmessage({data:'root@host:/data/second# '})
  view.state.activateSession(first.id)
  view.state.openUploadDialog()
  assert.equal(view.state.uploadDirectory.value, '/data/first')
  view.state.uploadFile.value = new File(['test'], 'test.txt')
  const uploading = view.state.submitUpload()
  view.state.activateSession(second.id)
  finish({filename:'test.txt'})
  await uploading
  assert.equal(form.get('directory'), '/data/first')
  assert.match(a.term.writes.at(-1), /上传完成.*\/data\/first/)
  assert.equal(b.term.writes.length, 1)
  view.state.openUploadDialog()
  assert.equal(view.state.uploadDirectory.value, '/data/second')
})

test('late file listings cannot replace another session file browser', async (t) => {
  const pending = []
  const view = mountTerminal({queryAssetTerminalFiles: (hostId, directory) => new Promise(resolve => pending.push({hostId,directory,resolve}))})
  t.after(() => view.app.unmount())
  await view.state.openHost({id:1,hostName:'first'})
  const first = view.state.sessions.value[0]
  view.state.runtimes.get(first.id).currentDirectory = '/data/first'
  const firstListing = view.state.openDownloadDialog()
  await view.state.openHost({id:2,hostName:'second'})
  const second = view.state.sessions.value[1]
  view.state.runtimes.get(second.id).currentDirectory = '/data/second'
  const secondListing = view.state.openDownloadDialog()
  assert.deepEqual(pending.map(({hostId,directory}) => [hostId,directory]), [[1,'/data/first'],[2,'/data/second']])
  pending[1].resolve({path:'/data/second',items:[{name:'second.txt'}]})
  await secondListing
  pending[0].resolve({path:'/data/first',items:[{name:'first.txt'}]})
  await firstListing
  assert.equal(view.state.downloadDirectory.value, '/data/second')
  assert.equal(view.state.downloadItems.value[0].name, 'second.txt')
})

test('broadcast sends a command and Enter once to every connected session, including duplicates of one host', async (t) => {
  const notices = []
  const view = mountTerminal({ElMessage:{success: message => notices.push(message), warning: message => notices.push(message)}})
  t.after(() => view.app.unmount())
  for (const host of [{id:1,hostName:'same'}, {id:1,hostName:'same'}, {id:2,hostName:'other'}, {id:3,hostName:'pending'}, {id:4,hostName:'closed'}]) await view.state.openHost(host)
  const sessions = view.state.sessions.value
  const sockets = sessions.map(session => view.state.runtimes.get(session.id).socket)
  sockets.slice(0,3).forEach(socket => socket.onopen())
  sockets[4].close()
  view.state.openBroadcastDialog()
  assert.equal(view.state.broadcastTargets.value.length, 3)
  assert.ok(sockets.every(socket => socket.sent.length === 0), 'opening the dialog must never send a command')
  view.state.sendBroadcastCommand('  echo "你好"\r\nhostname\n')
  for (const socket of sockets.slice(0,3)) assert.deepEqual(socket.sent, ['  echo "你好"\rhostname\r'])
  assert.equal(sockets[3].sent.length, 0)
  assert.equal(sockets[4].sent.length, 0)
  assert.equal(view.state.broadcastDialogVisible.value, false)
  assert.match(notices.at(-1), /已发送到 3 个会话/)
  view.state.sendBroadcastCommand('duplicate click')
  assert.equal(sockets[0].sent.length, 1, 'the closed dialog cannot submit twice')
})

test('broadcast skips sessions disconnected after opening and continues when one socket throws', async (t) => {
  const notices = []
  const view = mountTerminal({ElMessage:{success: message => notices.push(message), warning: message => notices.push(message)}})
  t.after(() => view.app.unmount())
  for (let i=0; i<3; i++) await view.state.openHost({id:1,hostName:'same'})
  const [a,b,c] = view.state.sessions.value.map(session => view.state.runtimes.get(session.id).socket)
  for (const socket of [a,b,c]) socket.onopen()
  view.state.openBroadcastDialog()
  a.close()
  b.send = () => { throw new Error('socket closed during send') }
  view.state.sendBroadcastCommand('hostname')
  assert.deepEqual(a.sent, [])
  assert.deepEqual(c.sent, ['hostname\r'])
  assert.match(notices.at(-1), /已发送到 1 个会话，1 个会话未发送/)
  assert.equal(view.state.broadcastDialogVisible.value, false)
})

test('empty or cancelled broadcast never sends and unsupported transfers use the normal input guard', async (t) => {
  const view = mountTerminal()
  t.after(() => view.app.unmount())
  await view.state.openHost({id:1,hostName:'same'})
  const runtime = [...view.state.runtimes.values()][0]
  view.state.openBroadcastDialog()
  assert.equal(view.state.broadcastDialogVisible.value, false, 'a connecting terminal is not a logged-in target')
  runtime.socket.onopen()
  view.state.openBroadcastDialog()
  view.state.sendBroadcastCommand('  \n ')
  assert.deepEqual(runtime.socket.sent, [])
  view.state.broadcastDialogVisible.value = false
  view.state.sendBroadcastCommand('cancelled')
  assert.deepEqual(runtime.socket.sent, [])
  view.state.openBroadcastDialog()
  view.state.sendBroadcastCommand('rz')
  assert.deepEqual(runtime.socket.sent, ['\x15'])
  assert.match(runtime.term.writes.at(-1), /Web 终端不支持 rz \/ sz/)
})
