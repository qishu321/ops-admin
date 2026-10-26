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

function mountTerminal() {
  const events = new EventTarget()
  events.location = { protocol: 'http:', host: 'localhost' }
  let closes = 0, creates = 0, state
  class Socket {
    static OPEN = 1
    readyState = 1
    constructor() { creates++ }
    close() { closes++ }
  }
  class Xterm {
    rows = 30; cols = 120
    open() {} focus() {} dispose() {} writeln() {} write() {}
    onData() { return { dispose() {} } }
  }
  const context = vm.createContext({
    computed, ref, reactive, nextTick, onActivated, onMounted, onBeforeUnmount,
    window: events, WebSocket: Socket, Terminal: Xterm, TextDecoder, ArrayBuffer, Uint8Array,
    getToken: () => 'test-token', useAccessRefresh() {},
    requestAnimationFrame: () => 1, cancelAnimationFrame() {},
    ResizeObserver: class { observe() {} disconnect() {} },
    queryAssetHostGroupList: async () => ({list:[]}), queryAssetHostList: async () => ({list:[]}),
  })
  const TerminalPage = defineComponent({setup() {
    state = vm.runInContext(`${setup}; ({sessions, runtimes, openHost, setTerminalElement})`, context)
    return () => h('div')
  }})
  const visible = ref(true)
  const app = renderer.createApp({setup: () => () => h(KeepAlive, null, { default: () => visible.value ? h(TerminalPage, {key:'/assets/terminal'}) : h('span') })})
  app.mount({children:[]})
  return { state, visible, events, app, closes: () => closes, creates: () => creates }
}

test('switching away and back preserves the original terminal and WebSocket', async () => {
  const view = mountTerminal()
  view.state.setTerminalElement('host-1', {})
  await view.state.openHost({id:1,hostName:'test',sshIp:'192.0.2.1'})
  const runtime = view.state.runtimes.get('host-1')
  const socket = runtime.socket
  socket.onopen()
  view.visible.value = false; await nextTick()
  view.visible.value = true; await nextTick()
  assert.equal(view.creates(), 1)
  assert.equal(view.closes(), 0)
  assert.equal(view.state.runtimes.get('host-1'), runtime)
  assert.equal(runtime.socket, socket)
  assert.equal(view.state.sessions.value[0].status, 'connected')
  view.app.unmount()
  assert.equal(view.closes(), 1)
})

test('explicitly closing the terminal tag still releases all session resources', async () => {
  const view = mountTerminal()
  view.state.setTerminalElement('host-1', {})
  await view.state.openHost({id:1,sshIp:'192.0.2.1'})
  const event = new Event('ops-admin:tab-closed')
  event.detail = {path:'/assets/terminal'}
  view.events.dispatchEvent(event)
  assert.equal(view.closes(), 1)
  assert.equal(view.state.runtimes.size, 0)
  assert.equal(view.state.sessions.value.length, 0)
  view.app.unmount()
})
