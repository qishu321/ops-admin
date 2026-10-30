const byName = (a, b) => String(a.name).localeCompare(String(b.name), undefined, { numeric: true, sensitivity: 'base' })
const matches = (item, role) => String(item.name).toLowerCase().includes(role)
export const nodeWidth = node => node.width || 220
export const nodeHeight = node => node.height || 116
export function healthyWorkload(item) {
  if (!item) return true
  const [ready, expected] = String(item.ready || '').split('/').map(Number)
  return expected > 0 && ready === expected && Number(item.available) >= expected
}

export function buildServiceTopology(workloads = []) {
  const find = role => workloads.find(item => matches(item, role))
  const roles = Object.fromEntries(['nginx-gm', 'mgr', 'gate', 'login', 'notice', 'social', 'contact'].map(role => [role, find(role)]))
  const zk = workloads.find(item => matches(item, 'zookeeper') || item.name.toLowerCase() === 'zk')
  const home = workloads.filter(item => matches(item, 'home')).sort(byName)
  const world = workloads.filter(item => matches(item, 'world')).sort(byName)
  const columns = Math.min(2, Math.max(1, Math.ceil(Math.sqrt(Math.max(home.length, world.length, 1)))))
  const groupX = 1120, groupWidth = columns * 270 + 24
  const groupHeight = items => Math.ceil(items.length / columns) * 180 + 40
  const groups = [], nodes = [], edges = []
  const add = (id, title, x, y, kind, note, workload = null, more = {}) => nodes.push({ id, title, x, y, kind, note, workload, healthy: healthyWorkload(workload), width:220, height:116, ...more })
  add('player', '玩家客户端', 36, 360, 'external', '仅通过 Gate / Notice 通信')
  const placements = [
    ['nginx-gm', 'nginx', 36, 80, 'management', '独立 GM 入口'],
    ['mgr', 'mgr', 366, 80, 'management', 'GM 后台'],
    ['gate', 'gate', 366, 360, 'public', 'WebSocket · 对外入口'],
    ['login', 'login', 366, 600, 'service', '登录校验'],
    ['notice', 'notice', 366, 840, 'public', '日志上报 · 对外入口'],
    ['contact', 'contact', 756, 360, 'scheduler', '统一调度 · 内部服务协调'],
    ['social', 'social', 1148, 80, 'service', '社交服务 · 通过 ZooKeeper 发现']
  ]
  placements.forEach(([role,id,x,y,kind,note]) => { if(roles[role]) add(id,roles[role].name,x,y,kind,note,roles[role]) })
  add('zk', zk?.name || 'ZooKeeper', 756, 720, 'dependency', '服务注册 / 启用状态', zk || null)
  let groupY = 320
  for (const [key, items] of [['home',home],['world',world]]) {
    if(!items.length) continue
    const height = groupHeight(items)
    groups.push({key,x:groupX,y:groupY,width:groupWidth,height,count:items.length})
    items.forEach((item,index) => add(`${key}-${index}`,item.name,groupX+28+(index%columns)*270,groupY+36+Math.floor(index/columns)*180,'service','有状态游戏服务',item,{statefulGroup:key,instance:index+1,width:226,height:132}))
    groupY += height + 88
  }
  const known = new Set([...Object.values(roles),zk,...home,...world].filter(Boolean))
  const extras = workloads.filter(item => !known.has(item)).sort(byName)
  const extrasY = Math.max(1060, groupY + 30)
  extras.forEach((item,index) => add(`extra-${index}`,item.name,366+(index%3)*390,extrasY+Math.floor(index/3)*180,'service','内部业务服务',item))
  const positions = Object.fromEntries(nodes.map(node => [node.id,node]))
  const edge = (from,to,label,tone='default',secondary=false,ports={}) => {
    if(positions[from] && positions[to]) edges.push({id:`${from}-${to}`,from,to,label,tone,secondary,...ports})
  }
  edge('player','gate','WebSocket 登录','public')
  edge('player','notice','日志上报','public')
  edge('gate','login','登录请求','public')
  edge('login','zk','读取启用服务')
  edge('mgr','zk','启停管理','management',true)
  edge('zk','social','服务注册','default',true)
  const callers = ['mgr','gate','notice','social',...home.map((_,i)=>`home-${i}`),...world.map((_,i)=>`world-${i}`)].filter(id=>positions[id])
  const left = callers.filter(id => positions[id].x < 756)
  const right = callers.filter(id => positions[id].x >= 756)
  for (const [side,items] of [['left',left],['right',right]]) items.forEach((id,index) => edge(id,'contact','调度请求','scheduler',false,{toPort:{side,offset:(index+1)/(items.length+1)}}))
  for(const key of ['home','world']) {
    const items = key==='home' ? home : world
    items.forEach((_,i) => {
      const id=`${key}-${i}`
      edge('zk',id,'服务注册','default',true)
      edge('gate',id,'游戏会话','default',true)
      edge(id,'social','社交通信','default',true)
      edge('mgr',id,'管理游戏服','management',true)
    })
  }
  // Keep existing GM access relationships, but reveal their labels on selection.
  nodes.filter(n=>!['player','nginx','zk'].includes(n.id)).forEach(n=>edge(n.id,'nginx','访问 GM','management',n.id!=='mgr'))
  const width = Math.max(1500,...nodes.map(n=>n.x+nodeWidth(n)+92),...groups.map(g=>g.x+g.width+60))
  const height = Math.max(1040,...nodes.map(n=>n.y+nodeHeight(n)+76),...groups.map(g=>g.y+g.height+60))
  return {nodes,edges,groups,width,height,positions,coordination:{x:724,y:288,width:284,height:600},extrasY,extrasCount:extras.length}
}

function socket(node, side, offset=.5) {
  const w=nodeWidth(node),h=nodeHeight(node)
  if(side==='left') return {x:node.x,y:node.y+h*offset,dx:-1,dy:0}
  if(side==='right') return {x:node.x+w,y:node.y+h*offset,dx:1,dy:0}
  if(side==='top') return {x:node.x+w*offset,y:node.y,dx:0,dy:-1}
  return {x:node.x+w*offset,y:node.y+h,dx:0,dy:1}
}

export function segmentHitsNode(a,b,node,padding=0) {
  const left=node.x-padding,right=node.x+nodeWidth(node)+padding,top=node.y-padding,bottom=node.y+nodeHeight(node)+padding
  if(a.x===b.x) return a.x>left+.01 && a.x<right-.01 && Math.max(a.y,b.y)>top+.01 && Math.min(a.y,b.y)<bottom-.01
  return a.y>top+.01 && a.y<bottom-.01 && Math.max(a.x,b.x)>left+.01 && Math.min(a.x,b.x)<right-.01
}

// Route SVG edges through the free lanes around cards. Unlike midpoint elbows,
// these paths cannot run through the login, scheduler, or a neighbouring instance.
export function routeServiceEdge(edge,layout) {
  const from=layout.positions[edge.from],to=layout.positions[edge.to],clearance=22
  const dx=to.x+nodeWidth(to)/2-from.x-nodeWidth(from)/2,dy=to.y+nodeHeight(to)/2-from.y-nodeHeight(from)/2
  const vertical=Math.abs(dy)>Math.abs(dx)*1.45
  const fromSide=edge.fromPort?.side || (vertical ? (dy>0?'bottom':'top') : (dx>0?'right':'left'))
  const toSide=edge.toPort?.side || (vertical ? (dy>0?'top':'bottom') : (dx>0?'left':'right'))
  const start=socket(from,fromSide,edge.fromPort?.offset),end=socket(to,toSide,edge.toPort?.offset)
  const first={x:Math.round(start.x+start.dx*clearance),y:Math.round(start.y+start.dy*clearance)}
  const last={x:Math.round(end.x+end.dx*clearance),y:Math.round(end.y+end.dy*clearance)}
  const xs=[first.x,last.x,12,layout.width-12],ys=[first.y,last.y,12,layout.height-12]
  for(const node of layout.nodes){xs.push(node.x-clearance,node.x+nodeWidth(node)+clearance);ys.push(node.y-clearance,node.y+nodeHeight(node)+clearance)}
  const x=[...new Set(xs)].sort((a,b)=>a-b),y=[...new Set(ys)].sort((a,b)=>a-b)
  const key=(i,j)=>j*x.length+i,goal=key(x.indexOf(last.x),y.indexOf(last.y)),origin=key(x.indexOf(first.x),y.indexOf(first.y))
  const distance=(i,j)=>Math.abs(x[i]-last.x)+Math.abs(y[j]-last.y)
  const open=[{i:x.indexOf(first.x),j:y.indexOf(first.y),score:distance(x.indexOf(first.x),y.indexOf(first.y))}],cost=new Map([[origin,0]]),previous=new Map(),closed=new Set()
  while(open.length){
    open.sort((a,b)=>b.score-a.score)
    const current=open.pop(),id=key(current.i,current.j)
    if(closed.has(id))continue
    if(id===goal)break
    closed.add(id)
    for(const [i,j] of [[current.i-1,current.j],[current.i+1,current.j],[current.i,current.j-1],[current.i,current.j+1]]){
      if(i<0||j<0||i>=x.length||j>=y.length)continue
      const next=key(i,j),a={x:x[current.i],y:y[current.j]},b={x:x[i],y:y[j]}
      if(layout.nodes.some(node=>segmentHitsNode(a,b,node,clearance-1)))continue
      const nextCost=cost.get(id)+Math.abs(a.x-b.x)+Math.abs(a.y-b.y)
      if(nextCost>=(cost.get(next)??Infinity))continue
      cost.set(next,nextCost);previous.set(next,id);open.push({i,j,score:nextCost+distance(i,j)})
    }
  }
  if(origin!==goal && !previous.has(goal)) return {points:[],path:'',labelPosition:{x:0,y:0}}
  const middle=[],coords=id=>({x:x[id%x.length],y:y[Math.floor(id/x.length)]})
  for(let cursor=goal;cursor!==undefined;cursor=previous.get(cursor))middle.push(coords(cursor))
  const points=[start,...middle.reverse(),end].map(p=>({x:Math.round(p.x),y:Math.round(p.y)}))
  const simplified=points.filter((p,i)=>i===0||i===points.length-1||!((points[i-1].x===p.x&&p.x===points[i+1].x)||(points[i-1].y===p.y&&p.y===points[i+1].y)))
  let best={length:0,x:0,y:0}
  for(let i=1;i<simplified.length;i++){const a=simplified[i-1],b=simplified[i],length=Math.abs(a.x-b.x)+Math.abs(a.y-b.y);if(a.y===b.y && length>best.length)best={length,x:(a.x+b.x)/2,y:a.y-10}}
  if(!best.length){const a=simplified[0],b=simplified[1] || a;best={x:a.x+10,y:(a.y+b.y)/2}}
  return {points:simplified,path:simplified.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' '),labelPosition:best}
}
