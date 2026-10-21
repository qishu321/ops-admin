import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

const page = readFileSync(new URL('../src/views/integration/IntegrationNavigation.vue', import.meta.url), 'utf8')
const copyPublicURL = page.slice(page.indexOf('async function copyPublicURL()'), page.indexOf('async function regenerateToken()'))
const url = 'http://192.0.2.10:8080/public/navigation/test-public-token'
const clipboardSource = readFileSync(new URL('../src/utils/clipboard.js', import.meta.url), 'utf8').replace('export async function', 'async function')

function copyContext(copied) {
  const messages = []
  const context = vm.createContext({
    publicURL: { value: url }, copying: { value: false },
    manualCopyURL: { value: '' }, copyDialogVisible: { value: false },
    navigator: {},
    copyTextToClipboard: async value => { assert.equal(value, url); return copied },
    ElMessage: { success: text => messages.push(text), warning: () => {} }
  })
  return { context, messages }
}

test('navigation share link copies on HTTP even when navigator.clipboard is unavailable', async () => {
  const { context, messages } = copyContext(true)
  await vm.runInContext(`${copyPublicURL}\ncopyPublicURL()`, context)
  assert.deepEqual(messages, ['公开链接已复制'])
  assert.equal(context.copyDialogVisible.value, false)
})

test('navigation exposes the exact link for manual copying when automatic copying is blocked', async () => {
  const { context, messages } = copyContext(false)
  await vm.runInContext(`${copyPublicURL}\ncopyPublicURL()`, context)
  assert.equal(context.copyDialogVisible.value, true)
  assert.equal(context.manualCopyURL.value, url)
  assert.equal(messages.length, 0, 'blocked copy must never report success')
})

function legacyClipboardContext({ clipboard, succeeds = true, throws = false } = {}) {
  const state = { value: '', focused: false, selected: false, removed: false, restored: false, commands: [] }
  const textarea = {
    style: {}, value: '', setAttribute() {},
    focus() { state.focused = true }, select() { state.selected = true },
    setSelectionRange(start, end) { assert.equal(start, 0); assert.equal(end, url.length) },
    remove() { state.removed = true }
  }
  const context = vm.createContext({ navigator: { clipboard }, document: {
    activeElement: { isConnected: true, focus() { state.restored = true } },
    body: { appendChild(element) { assert.equal(element, textarea) } },
    createElement(tag) { assert.equal(tag, 'textarea'); return textarea },
    execCommand(command) {
      state.commands.push(command)
      assert.equal(state.focused && state.selected, true)
      state.value = textarea.value
      if (throws) throw new Error('clipboard denied')
      return succeeds
    }
  } })
  return { state, run: () => vm.runInContext(`${clipboardSource}\ncopyTextToClipboard(${JSON.stringify(url)})`, context) }
}

test('clipboard helper prefers the modern API and copies the full URL', async () => {
  let copied
  const { state, run } = legacyClipboardContext({ clipboard: { writeText: async value => { copied = value } } })
  assert.equal(await run(), true)
  assert.equal(copied, url)
  assert.deepEqual(state.commands, [])
})

test('clipboard helper falls back on HTTP and restores focus after selecting the exact URL', async () => {
  const { state, run } = legacyClipboardContext()
  assert.equal(await run(), true)
  assert.equal(state.value, url)
  assert.deepEqual(state.commands, ['copy'])
  assert.equal(state.removed && state.restored, true)
})

test('clipboard helper also falls back when modern clipboard permission is denied', async () => {
  const { state, run } = legacyClipboardContext({ clipboard: { writeText: async () => { throw new Error('NotAllowedError') } } })
  assert.equal(await run(), true)
  assert.equal(state.value, url)
  assert.equal(state.removed && state.restored, true)
})

test('clipboard helper reports denial truthfully and cleans up temporary input on every failure', async () => {
  for (const options of [{ succeeds: false }, { throws: true }]) {
    const { state, run } = legacyClipboardContext(options)
    assert.equal(await run(), false)
    assert.equal(state.removed && state.restored, true)
  }
})
