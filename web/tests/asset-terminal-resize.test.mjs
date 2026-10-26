import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const page = readFileSync(new URL('../src/views/assets/Terminal.vue', import.meta.url), 'utf8')
const resize = page.slice(page.indexOf('function bindTerminalResize('), page.indexOf('function handleTabClosed('))

function resizeFixture() {
  const sent = []
  let width = 1024, height = 800, active = 'first'
  const makeRuntime = () => ({
    protocol:'ops-admin-terminal-v2',
    socket: {readyState:1,send: data => sent.push(data)},
    term: {cols:120,rows:30,resize(cols,rows) { this.cols=cols; this.rows=rows }}
  })
  const first = makeRuntime(), second = makeRuntime()
  const element = {
    get clientWidth() { return width }, get clientHeight() { return height },
    querySelector(selector) {
      if (selector === '.xterm-screen') return {clientWidth:first.term.cols*8,clientHeight:first.term.rows*16}
      if (selector === '.xterm-viewport') return {offsetWidth:width-24,clientWidth:width-36}
      return null
    }
  }
  const context = vm.createContext({
    activeSessionId:{get value() { return active }},
    runtimes:new Map([['first',first],['second',second]]),
    terminalElements:new Map([['first',element],['second',{clientWidth:0,clientHeight:0}]]),
    window:{getComputedStyle:()=>({paddingLeft:'12px',paddingRight:'12px',paddingTop:'10px',paddingBottom:'10px'})},
    WebSocket:{OPEN:1}, TERMINAL_PROTOCOL:'ops-admin-terminal-v2',resizeFrame:undefined,
    cancelAnimationFrame(){},requestAnimationFrame(callback){callback(1234.5);return 1}
  })
  vm.runInContext(resize, context)
  return {first,second,sent,fit:()=>vm.runInContext('syncTerminalSize()',context),scheduleFit:()=>vm.runInContext('scheduleTerminalSizeSync()',context),size:(w,h)=>{width=w;height=h},activate:id=>{active=id}}
}

test('fitting the visible terminal updates its owned SSH window and ignores unchanged sizes', () => {
  const view = resizeFixture()
  view.fit()
  assert.equal(view.first.term.rows,48)
  assert.equal(view.sent.length,1, 'local resizing must also send the new size to SSH')
  assert.equal(view.first.term.cols,123, 'leave room for the terminal scrollbar')
  assert.deepEqual(JSON.parse(view.sent[0]),{type:'resize',rows:48,cols:123})
  view.fit()
  assert.equal(view.sent.length,1)
  view.size(664,500)
  view.fit()
  assert.equal(view.first.term.rows,30)
  assert.deepEqual(JSON.parse(view.sent[1]),{type:'resize',rows:30,cols:78})
  assert.equal(view.second.term.cols,120)
  view.activate('second')
  view.fit()
  assert.equal(view.sent.length,2, 'hidden terminal must not get a zero-sized SSH window')
})

test('animation frame timestamp cannot be interpreted as a terminal session ID',()=>{
  const view=resizeFixture()
  view.scheduleFit()
  assert.equal(view.sent.length,1,'a real animation frame must resize the active session')
  assert.equal(view.first.term.rows,48)
})

test('legacy backend never receives control messages and disconnected sockets are ignored', () => {
  const view = resizeFixture()
  view.first.protocol = ''
  view.fit()
  assert.equal(view.sent.length,0)
  view.first.protocol = 'ops-admin-terminal-v2'
  view.first.socket.readyState = 3
  view.size(800,600)
  view.fit()
  assert.equal(view.sent.length,0)
})
