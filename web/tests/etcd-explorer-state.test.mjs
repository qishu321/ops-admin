import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { computed, ref } from 'vue'
const setup = readFileSync(new URL('../src/views/assets/EtcdExplorer.vue', import.meta.url), 'utf8').match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*$/gm, '')
function harness() {
 const requests = []
 const context = vm.createContext({ computed, ref, defineProps: () => ({ source: { id: 7 }, can: () => true }), defineEmits: () => {}, onMounted: () => {}, onUnmounted: () => {}, ElMessage: { success() {} }, ElMessageBox: { confirm: async () => {} }, operateMiddleware: (...args) => new Promise(resolve => requests.push({ args, resolve })) })
 return { requests, page: vm.runInContext(`${setup};({load,select,tree,keys,more,prefix,selected,reading})`,context) }
}
test('etcd directory preserves keys that are also folders and paginates using the loaded prefix', async () => {
 const { page, requests } = harness()
 page.prefix.value='/app'
 const first=page.load(); requests[0].resolve({kvs:[{key:'/app'},{key:'/app/a'}],more:true}); await first
 assert.equal(page.tree.value[0].key,'/app'); assert.equal(page.tree.value[0].children[0].key,'/app/a')
 page.prefix.value='/different'
 const next=page.load(true)
 assert.equal(requests[1].args[2].key,'/app'); assert.equal(requests[1].args[2].after,'/app/a')
 requests[1].resolve({kvs:[{key:'/app/b'}],more:false}); await next
 assert.equal(page.keys.value.length,3); assert.equal(page.more.value,false)
})
test('etcd selecting a new key ignores late content from the previous key',async()=>{
 const {page,requests}=harness()
 const first=page.select({key:'/one'}), second=page.select({key:'/two'})
 requests[1].resolve({kvs:[{key:'/two',value:'current'}]}); await second
 requests[0].resolve({kvs:[{key:'/one',value:'stale'}]}); await first
 assert.equal(page.selected.value.key,'/two'); assert.equal(page.reading.value,false)
})
