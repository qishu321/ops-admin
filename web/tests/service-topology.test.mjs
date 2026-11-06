import assert from 'node:assert/strict'
import test from 'node:test'
import { buildServiceTopology, routeServiceEdge, segmentHitsNode, nodeWidth, nodeHeight } from '../src/utils/service-topology.js'
const workload=(name,type='Deployment')=>({name,type,ready:'2/2',available:2})
const sample=()=>['nginx-gm','szfc-mgr','szfc-gate','szfc-notice','szfc-login','szfc-contact','szfc-social','szfc-battle','szfc-home','szfc-world'].map(name=>workload(name,name.includes('home')||name.includes('world')?'StatefulSet':'Deployment'))

test('Contact is a central scheduler receiving exactly the specified service roles',()=>{
  const layout=buildServiceTopology(sample()),contact=layout.positions.contact,zk=layout.positions.zk
  assert.equal(contact.kind,'scheduler');assert.equal(contact.x,zk.x)
  // 注册中心就挂在调度器正下方：同一列、隔一个可走线的通道（>44 才有通道，又不能再拉开两百多像素）。
  const gap=zk.y-(contact.y+nodeHeight(contact))
  assert.ok(gap>44&&gap<120,`zk 与调度器的间距 ${gap}`)
  assert.deepEqual(layout.edges.filter(edge=>edge.to==='contact').map(edge=>edge.from).sort(),['gate','mgr','notice','social','battle-0','home-0','world-0'].sort())
  assert.equal(layout.nodes.filter(node=>node.workload?.name==='szfc-contact').length,1)
})

test('the entry chain reads left to right: player → gate → login → contact → backend services',()=>{
  const layout=buildServiceTopology(sample()),p=layout.positions
  // 玩家客户端是唯一的最左入口，不在「其他内部服务」里下沉。
  assert.equal(p.player.kind,'external')
  assert.equal(Math.min(...layout.nodes.map(node=>node.x)),p.player.x)
  const chain=['player','gate','login','contact']
  for(let i=1;i<chain.length;i++)assert.ok(p[chain[i]].x>p[chain[i-1]].x,`${chain[i]} 应该在 ${chain[i-1]} 右边`)
  // 后段服务（home / world）在调度器右侧。
  assert.ok(p['home-0'].x>p.contact.x&&p['world-0'].x>p.contact.x)
  // 战斗服单独成卡，紧接在 login 后面（同列下行 + 登录后进入战斗的连线），不再归入其他内部服务。
  const battle=p['battle-0']
  assert.equal(battle.x,p.login.x)
  assert.ok(battle.y>p.login.y)
  assert.ok(layout.edges.some(edge=>edge.from==='login'&&edge.to==='battle-0'))
  assert.equal(battle.workload.name,'szfc-battle')
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
