import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const page = readFileSync(new URL('../src/views/assets/Terminal.vue', import.meta.url), 'utf8')
const connectSocket = page.slice(page.indexOf('function connectSocket(id)'), page.indexOf('function disconnectSession('))

function connect() {
  const writes = []
  const prompts = []
  const notices = []
  const runtime = { term: { rows: 30, cols: 120, write: (data) => writes.push(data), writeln: (text) => notices.push(text) } }
  class Socket {
    static OPEN = 1
    constructor() { this.readyState = Socket.OPEN }
  }
  const context = vm.createContext({
    WebSocket: Socket, ArrayBuffer, Uint8Array, TextDecoder,
    sessions: { value: [{ id: 'host-1', host: { id: 1, sshIp: 'test-host' } }] },
    runtimes: new Map([['host-1', runtime]]),
    activeSessionId: { value: null },
    window: { location: { protocol: 'http:', host: 'localhost' } },
    getToken: () => 'test-token', disconnectSession: () => {}, updateSession: () => {},
    TERMINAL_PROTOCOL:'ops-admin-terminal-v2', syncTerminalSize() {}, sendTerminalResize() {},
    trackPromptDirectory: (_, output) => prompts.push(output)
  })
  vm.runInContext(`${connectSocket}\nconnectSocket('host-1')`, context)
  return { runtime, writes, prompts, notices, reconnect: () => vm.runInContext("connectSocket('host-1')", context) }
}

test('SSH terminal receives binary output and decodes split UTF-8 across messages', () => {
  const { runtime, writes, prompts } = connect()
  assert.equal(runtime.socket.binaryType, 'arraybuffer')
  const expected = '当前集群启动中 🚀\r\nroot@host:~/日志# '
  const bytes = new TextEncoder().encode(expected)
  for (const byte of bytes) {
    runtime.socket.onmessage({ data: new Uint8Array([byte]).buffer })
  }
  assert.ok(writes.every((chunk) => chunk instanceof Uint8Array), 'xterm must receive raw byte chunks')
  assert.deepEqual(Buffer.concat(writes.map((chunk) => Buffer.from(chunk))), Buffer.from(bytes))
  assert.equal(prompts.join(''), expected, 'prompt decoding must preserve split characters')
})

test('SSH terminal still accepts text output and ignores stale sockets after reconnect', () => {
  const { runtime, writes, prompts, reconnect } = connect()
  runtime.socket.onmessage({ data: 'root@host:~# ' })
  assert.equal(writes[0], 'root@host:~# ')
  assert.equal(prompts[0], 'root@host:~# ')
  const oldSocket = runtime.socket
  oldSocket.onmessage({ data: new Uint8Array([0xe5]).buffer })
  reconnect()
  oldSocket.onmessage({ data: 'stale output' })
  runtime.socket.onmessage({ data: new TextEncoder().encode('新会话').buffer })
  assert.equal(prompts.at(-1), '新会话', 'new connection must not retain an incomplete previous character')
  assert.ok(!writes.includes('stale output'))
})

test('SSH terminal does not report credential failure for an established connection', () => {
  const { runtime, notices } = connect()
  runtime.socket.onopen()
  runtime.socket.onerror()
  assert.doesNotMatch(notices.at(-1), /认证凭据/)
  assert.match(notices.at(-1), /网络|传输|服务端/)
})

test('60-minute idle expiry has a specific reason and is distinct from network disconnection', () => {
  const idle = connect()
  idle.runtime.socket.onclose({ code: 4001 })
  assert.match(idle.notices.at(-1), /60 分钟没有输入或输出/)
  const network = connect()
  network.runtime.socket.onclose({ code: 1006 })
  assert.doesNotMatch(network.notices.at(-1), /60 分钟/)
})

test('opening more than ten pages never automatically evicts the terminal tag', () => {
  const layout = readFileSync(new URL('../src/layouts/MainLayout.vue', import.meta.url), 'utf8')
  const functions = layout.slice(layout.indexOf('function isPinnedTag('), layout.indexOf('function releaseTagResources('))
  const trim = vm.runInNewContext(`${functions}; trimTagViews`, { MAX_TAG_VIEWS: 10 })
  const tags = [{path:'/dashboard'},{path:'/assets/terminal'},...Array.from({length:14},(_,i)=>({path:`/page-${i}`}))]
  const result=trim(tags,'/page-13')
  assert.equal(result.length,10)
  assert.ok(result.some(tag=>tag.path==='/assets/terminal'))
  assert.ok(result.some(tag=>tag.path==='/dashboard'))
  assert.ok(result.some(tag=>tag.path==='/page-13'))
})
