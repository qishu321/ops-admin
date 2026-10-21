import test from 'node:test'
import assert from 'node:assert/strict'
import { middlewareRows, middlewareValue, middlewareTypes } from '../src/views/assets/middleware-schema.js'
test('Nacos 2/3, etcd and ZooKeeper result envelopes retain resource identities', () => {
 assert.deepEqual(middlewareRows('nacos','configs',{pageItems:[{dataId:'a.yaml'}]}),[{dataId:'a.yaml'}])
 assert.deepEqual(middlewareRows('nacos','services',{doms:['orders']}),[{name:'orders'}])
 assert.deepEqual(middlewareRows('nacos','instances',{pageItems:[{ip:'127.0.0.1'}]}),[{ip:'127.0.0.1'}])
 assert.deepEqual(middlewareRows('etcd','keys',{kvs:[{key:'/app/a',value:'hello'}]}),[{key:'/app/a',value:'hello'}])
 assert.deepEqual(middlewareRows('zookeeper','children',{children:['app']},{path:'/'}),[{name:'app',path:'/app'}])
 assert.deepEqual(middlewareRows('zookeeper','children',{children:['config']},{path:'/app'}),[{name:'config',path:'/app/config'}])
})
test('nested consumer fields and Kafka partition counts are readable', () => {
 assert.equal(middlewareValue({queue:{name:'orders'}},'queue.name'),'orders')
 assert.equal(middlewareValue({Partitions:[{},{}]},'Partitions'),'2')
 assert.equal(middlewareValue({healthy:false},'healthy'),'否')
})
test('every middleware view starts with a query and mutations require explicit operation classification', () => {
 assert.deepEqual(Object.keys(middlewareTypes).sort(),['etcd','kafka','nacos','rabbitmq','zookeeper'])
 for(const spec of Object.values(middlewareTypes)) {
  assert.ok(spec.tabs.length)
  for(const op of spec.operations) assert.ok(['query','write','delete'].includes(op.action))
 }
})
