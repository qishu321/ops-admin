import assert from 'node:assert/strict'
import test from 'node:test'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'
import { computed, ref } from 'vue'

function fixture(request) {
  const source = readFileSync(new URL('../src/composables/useOpsTaskRetry.js', import.meta.url), 'utf8').replace(/^import .*$/gm, '').replace('export function', 'function')
  const task = ref({ id: 10, taskType: 'file', status: 'partial', retrySupported:true })
  const created = []
  const useRetry = vm.runInNewContext(`${source}; useOpsTaskRetry`, {computed, ref, retryOpsExecTask:request, ElMessage:{success(){}}})
  return {task, created, ...useRetry(task, data => created.push(data))}
}

test('individual retry sends original task and one failed result, never the complete host list', async () => {
  const calls = []
  const retry = fixture(async (...args) => {calls.push(args);return {task:{id:11}}})
  await retry.retry({id:3,hostName:'failed-host',status:'failed'})
  assert.deepEqual(calls, [[10,3]])
  assert.equal(retry.created[0].task.id,11)
  await retry.retry({id:4,status:'success'})
  await retry.retry({id:5,status:'running'})
  assert.equal(calls.length,1)
})

test('retry blocks repeat clicks and releases loading after rejection', async () => {
  let reject, calls=0
  const retry = fixture(() => {calls++;return new Promise((_,r)=>reject=r)})
  const first=retry.retry({id:3,status:'failed'})
  await retry.retry({id:3,status:'failed'})
  assert.equal(calls,1)
  assert.equal(retry.retrying.value,true)
  reject(Error('offline'))
  assert.equal(await first,null)
  assert.equal(retry.retrying.value,false)
})

test('switching viewed tasks during retry does not overwrite the new task', async () => {
  let resolve
  const retry=fixture(()=>new Promise(r=>resolve=r))
  const pending=retry.retry({id:3,status:'failed'})
  retry.task.value={id:99,taskType:'command',status:'success'}
  resolve({task:{id:11}})
  await pending
  assert.equal(retry.created.length,0)
})

test('all three execution types support failure-only retries but running tasks do not', () => {
  const retry=fixture(async()=>({}))
  for (const taskType of ['command','script','file']) {
    retry.task.value={id:10,taskType,status:'failed',retrySupported:true}
    assert.equal(retry.canRetryRow({id:3,status:'timeout'}),true)
    retry.task.value.status='running'
    assert.equal(retry.canRetryRow({id:3,status:'failed'}),false)
  }
})

test('old backend cannot turn an individual retry into a bulk retry by ignoring resultId', async () => {
  let calls=0
  const retry=fixture(async()=>{calls++})
  delete retry.task.value.retrySupported
  assert.equal(retry.canRetryRow({id:3,status:'failed'}),false)
  await retry.retry({id:3,status:'failed'})
  assert.equal(calls,0)
})
