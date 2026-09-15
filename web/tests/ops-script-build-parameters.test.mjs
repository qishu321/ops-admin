import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const librarySource = readFileSync(new URL('../src/views/ops/OpsScriptLibrary.vue', import.meta.url), 'utf8')
const quickExecuteSource = readFileSync(new URL('../src/views/ops/OpsQuickExecute.vue', import.meta.url), 'utf8')

test('script library renders structured variables as build parameters', () => {
  assert.doesNotMatch(librarySource, /defaultParams/)
  assert.match(librarySource, /row\.variables\?\.length/)
  assert.match(librarySource, /variableDisplay\(variable\)/)
})

test('quick execution does not claim command-line arguments fall back to build parameters', () => {
  assert.doesNotMatch(quickExecuteSource, /留空则使用脚本库中的构建参数/)
  assert.match(quickExecuteSource, /作为命令行参数传入/)
})
