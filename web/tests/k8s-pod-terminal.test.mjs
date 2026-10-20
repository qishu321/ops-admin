import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const page = readFileSync(new URL('../src/views/assets/K8sPodTerminal.vue', import.meta.url), 'utf8')

test('Pod 终端在后端 exec 就绪前保持连接中状态', () => {
  const onOpen = page.slice(page.indexOf('socket.onopen ='), page.indexOf('socket.onmessage ='))
  assert.doesNotMatch(onOpen, /connected\.value\s*=\s*true/)
  assert.match(page, /payload\?\.operation === 'status'/)
  assert.match(page, /state === 'connected'/)
  assert.match(page, /连接耗时较长/)
})

test('Pod 终端以结构化错误展示网关连接失败', () => {
  assert.match(page, /payload\?\.operation === 'error'/)
  assert.match(page, /ElMessage\.error\(message\)/)
  assert.match(page, /connected\.value && socket\?\.readyState === WebSocket\.OPEN/)
})

const syncTerminalSize = page.slice(page.indexOf('function syncTerminalSize()'), page.indexOf('async function loadContainers()'))

test('Pod terminal reserves the native scrollbar gutter instead of drawing text over the draggable thumb', () => {
  const sent = []
  const term = { cols: 120, rows: 32, resize(cols, rows) { this.cols = cols; this.rows = rows } }
  const viewport = { clientWidth: 1000, offsetWidth: 1017 }
  const screen = { clientWidth: 1200, clientHeight: 640 }
  const context = vm.createContext({
    term,
    terminalRef: { value: { clientWidth: 1017, clientHeight: 640, querySelector: selector => selector === '.xterm-screen' ? screen : viewport } },
    WebSocket: { OPEN: 1 },
    socket: { readyState: 1, send: data => sent.push(JSON.parse(data)) }
  })
  vm.runInContext(`${syncTerminalSize}\nsyncTerminalSize()`, context)
  assert.equal(term.cols, 100, 'rendered columns must fit inside the 1000px content area')
  assert.ok(term.cols * 10 <= viewport.clientWidth, 'the text layer must not cover any part of the scrollbar')
  assert.deepEqual(sent, [{ operation: 'resize', data: { cols: 100, rows: 32 } }])
})

test('Pod terminal keeps the scrollbar usable with overlay scrollbars and narrow terminal panes', () => {
  for (const { width, gutter, expectedCols } of [
    { width: 1000, gutter: 0, expectedCols: 98 },
    { width: 300, gutter: 17, expectedCols: 28 }
  ]) {
    const term = { cols: 120, rows: 32, resize(cols, rows) { this.cols = cols; this.rows = rows } }
    const context = vm.createContext({
      term,
      terminalRef: { value: {
        clientWidth: width, clientHeight: 640,
        querySelector: selector => selector === '.xterm-screen'
          ? { clientWidth: 1200, clientHeight: 640 }
          : { offsetWidth: width, clientWidth: width - gutter }
      } },
      socket: undefined,
      WebSocket: { OPEN: 1 }
    })
    vm.runInContext(`${syncTerminalSize}\nsyncTerminalSize()`, context)
    assert.equal(term.cols, expectedCols)
    assert.ok(term.cols * 10 <= width - Math.max(14, gutter))
  }
})
