import assert from 'node:assert/strict'
import test from 'node:test'
import { buildServiceTopology, routeServiceEdge, segmentHitsNode, nodeWidth, nodeHeight } from '../src/utils/service-topology.js'
const workload=(name,type='Deployment')=>({name,type,ready:'2/2',available:2})
const sample=()=>['nginx-gm','szfc-mgr','szfc-gate','szfc-notice','szfc-login','szfc-contact','szfc-social','szfc-battle','szfc-home','szfc-world'].map(name=>workload(name,name.includes('home')||name.includes('world')?'StatefulSet':'Deployment'))

test('Contact is a central scheduler receiving exactly the six specified service roles',()=>{
  const layout=buildServiceTopology(sample()),contact=layout.positions.contact,zk=layout.positions.zk
  assert.equal(contact.kind,'scheduler');assert.equal(contact.x,zk.x)
  assert.ok(zk.y-contact.y>=300)
  assert.deepEqual(layout.edges.filter(edge=>edge.to==='contact').map(edge=>edge.from).sort(),['gate','mgr','notice','social','home-0','world-0'].sort())
  assert.equal(layout.nodes.filter(node=>node.workload?.name==='szfc-contact').length,1)
})

test('cards remain separated and inside the canvas for both ordinary and many-instance layouts',()=>{
  for(const items of [sample(),[...sample(),...Array.from({length:10},(_,i)=>workload(`szfc-home-${i}`,'StatefulSet')),...Array.from({length:7},(_,i)=>workload(`szfc-world-${i}`,'StatefulSet')),...Array.from({length:6},(_,i)=>workload(`extra-${i}`))]]) {
    const layout=buildServiceTopology(items)
    for(const node of layout.nodes){
      assert.ok(node.x+nodeWidth(node)<=layout.width);assert.ok(node.y+nodeHeight(node)<=layout.height)
      for(const other of layout.nodes.filter(other=>other!==node))assert.ok(node.x+nodeWidth(node)+24<=other.x || other.x+nodeWidth(other)+24<=node.x || node.y+nodeHeight(node)+24<=other.y || other.y+nodeHeight(other)+24<=node.y,`${node.id} overlaps ${other.id}`)
    }
  }
})

test('every directed edge reaches its destination without crossing another card',()=>{
  for(const items of [sample(),[...sample(),workload('szfc-home-2','StatefulSet'),workload('szfc-world-2','StatefulSet'),workload('szfc-home-3','StatefulSet')]]){
    const layout=buildServiceTopology(items)
    for(const edge of layout.edges){
      const routed=routeServiceEdge(edge,layout)
      assert.ok(routed.path,`missing route ${edge.id}`)
      assert.equal(typeof edge.label,'string')
      for(let i=1;i<routed.points.length;i++)for(const node of layout.nodes.filter(node=>![edge.from,edge.to].includes(node.id)))assert.equal(segmentHitsNode(routed.points[i-1],routed.points[i],node),false,`${edge.id} crosses ${node.id}`)
    }
  }
})

test('missing optional services do not create phantom Contact instances or dangling edges',()=>{
  const layout=buildServiceTopology([workload('szfc-gate'),workload('szfc-home','StatefulSet')])
  assert.equal(layout.positions.contact,undefined)
  assert.equal(layout.edges.some(edge=>edge.to==='contact'),false)
  assert.ok(layout.edges.every(edge=>layout.positions[edge.from]&&layout.positions[edge.to]))
})
