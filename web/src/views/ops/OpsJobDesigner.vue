<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Graph, Shape } from '@antv/x6'
import { Aim, Bell, Check, Connection, Delete, Document, FolderOpened, Grid, InfoFilled, Lock, Mouse, Plus, Setting, Share, Upload, User, ZoomIn, ZoomOut } from '@element-plus/icons-vue'
import { queryAssetHostGroupList, queryAssetHostList } from '../../api/asset'
import {
  addOpsJob,
  addOpsJobTemplate,
  opsJobInfo,
  opsJobTemplateInfo,
  queryNotifyRuleOptions,
  queryOpsJobTemplateOptions,
  queryOpsScriptOptions,
  updateOpsJob,
  updateOpsJobTemplate
} from '../../api/ops'
import OpsTargetScope from './components/OpsTargetScope.vue'
import { conditionFields, conditionStatuses, conditionOperators, conditionSummary, validateJobBranches } from './job-conditions'

const router = useRouter()
const route = useRoute()

const graphContainer = ref()
const loading = ref(false)
const saving = ref(false)
const graphReady = ref(false)
const importDialogVisible = ref(false)
const importTemplateId = ref()
const selectedNodeId = ref('')
const selectedEdgeId = ref('')
const selectedCellIds = ref([])
const nodeCount = ref(0)
const edgeCount = ref(0)
const zoomPercent = ref(100)
const graphRevision = ref(0)
const stepLibrary = [
  { type: 'script', label: '脚本执行', description: '在目标主机运行脚本', icon: Document, color: '#356ae6', soft: '#edf3ff' },
  { type: 'file', label: '文件分发', description: '将文件同步到指定主机', icon: FolderOpened, color: '#138a78', soft: '#eaf7f3' },
  { type: 'approval', label: '人工确认', description: '确认操作后继续执行', icon: User, color: '#b77b21', soft: '#fff6e5' },
  { type: 'notify', label: '消息通知', description: '发送执行结果与通知', icon: Bell, color: '#7860bd', soft: '#f3eefb' },
  { type: 'condition', label: '条件分支', description: '根据执行结果选择后续流程', icon: Share, color: '#c07826', soft: '#fff4e4' }
]

const scriptOptions = ref([])
const hostOptions = ref([])
const groupOptions = ref([])
const templateOptions = ref([])
const notifyRuleOptions = ref([])

const form = reactive({
  id: undefined,
  name: '',
  description: '',
  status: 1,
  templateId: undefined,
  notifyEnabled: false,
  notifyRuleId: undefined
})

const selectedNodeForm = reactive({
  id: '',
  type: 'script',
  label: '',
  config: {}
})

let graph
let graphResizeObserver

const isTemplateMode = computed(() => String(route.query.mode || '') === 'template')
const editorTitle = computed(() => (isTemplateMode.value ? '作业模板编排' : '作业编排'))
const saveButtonText = computed(() => (isTemplateMode.value ? '保存模板' : '保存作业'))
const selectedCount = computed(() => selectedCellIds.value.length)
const selectedStepType = computed(() => stepLibrary.find((item) => item.type === selectedNodeForm.type) || stepLibrary[0])
const selectedConditionOperators = computed(() => conditionOperators.filter((item) => selectedNodeForm.config.field !== 'status' || ['eq', 'ne'].includes(item.value)))
const conditionSourceOptions = computed(() => {
  void graphRevision.value
  if (!graph || !selectedNodeId.value) return []
  return (graph.getIncomingEdges(selectedNodeId.value) || []).map((edge) => edge.getSourceNode()?.getData()).filter((node) => ['script', 'file'].includes(node?.type))
})

function handleConditionFieldChange() {
  selectedNodeForm.config.operator = 'eq'
  selectedNodeForm.config.value = selectedNodeForm.config.field === 'status' ? 'success' : 0
}

function selectedBranchLabel() {
  const edge = graph?.getCellById(selectedEdgeId.value)
  return edge?.getSourceNode()?.getData()?.type === 'condition' ? (edge.getSourcePortId() === 'true' ? '满足条件' : '不满足') : ''
}
const selectedScriptTimeout = computed(() => {
  const script = scriptOptions.value.find((item) => Number(item.id) === Number(selectedNodeForm.config?.scriptId))
  return script?.timeoutSeconds || 300
})
const selectedScriptVariables = computed(() => {
  const script = scriptOptions.value.find((item) => Number(item.id) === Number(selectedNodeForm.config?.scriptId))
  return script?.variables || []
})

function syncSelectedScriptVariables(values = {}) {
  const variables = {}
  selectedScriptVariables.value.forEach((variable) => {
    variables[variable.name] = values[variable.name] ?? (variable.secret ? '' : (variable.defaultValue || ''))
  })
  selectedNodeForm.config = { ...selectedNodeForm.config, variables }
}

function handleSelectedScriptChange() {
  syncSelectedScriptVariables(selectedNodeForm.config?.variables || {})
}

watch(
  () => [selectedNodeForm.label, JSON.stringify(selectedNodeForm.config || {})],
  () => {
    if (!graph || !selectedNodeId.value) return
    const node = graph.getCellById(selectedNodeId.value)
    if (!node) return
    const data = {
      id: selectedNodeForm.id,
      type: selectedNodeForm.type,
      label: selectedNodeForm.label,
      config: cloneValue(selectedNodeForm.config || {})
    }
    node.setData(data)
    updateNodePresentation(node)
  },
  { deep: true }
)

watch(
  () => route.fullPath,
  async () => {
    if (!graph) return
    form.id = undefined
    form.name = ''
    form.description = ''
    form.status = 1
    form.templateId = undefined
    form.notifyEnabled = false
    form.notifyRuleId = undefined
    graph.clearCells()
    loadSelectedNode('')
    await loadCurrentRecord()
  }
)

function cloneValue(value) {
  return JSON.parse(JSON.stringify(value))
}

function normalizeTargetIds(value) {
  if (!Array.isArray(value)) return []
  return [...new Set(value.map((item) => Number(item)).filter((item) => item > 0))]
}

function normalizeNodeTargets(config = {}) {
  const normalized = cloneValue(config || {})
  const hostIds = normalizeTargetIds(normalized.hostIds)
  const groupIds = normalizeTargetIds(normalized.groupIds)
  if (hostIds.length) {
    normalized.hostIds = hostIds
    normalized.groupIds = []
  } else {
    normalized.hostIds = []
    normalized.groupIds = groupIds
  }
  return normalized
}

function updateSelectedHostIds(hostIds) {
  const normalizedHostIds = normalizeTargetIds(hostIds)
  selectedNodeForm.config = {
    ...selectedNodeForm.config,
    hostIds: normalizedHostIds,
    groupIds: normalizedHostIds.length ? [] : normalizeTargetIds(selectedNodeForm.config.groupIds)
  }
}

function updateSelectedGroupIds(groupIds) {
  const normalizedGroupIds = normalizeTargetIds(groupIds)
  selectedNodeForm.config = {
    ...selectedNodeForm.config,
    hostIds: normalizedGroupIds.length ? [] : normalizeTargetIds(selectedNodeForm.config.hostIds),
    groupIds: normalizedGroupIds
  }
}

function defaultNodeLabel(type) {
  switch (type) {
    case 'condition':
      return '条件分支'
    case 'file':
      return '\u6587\u4ef6\u5206\u53d1'
    case 'approval':
      return '\u4eba\u5de5\u786e\u8ba4'
    case 'notify':
      return '\u6d88\u606f\u901a\u77e5'
    default:
      return '\u811a\u672c\u6267\u884c'
  }
}

function createDefaultConfig(type) {
  switch (type) {
    case 'condition':
      return { sourceStepId: '', field: 'status', operator: 'eq', value: 'success' }
    case 'file':
      return {
        sourceHostId: undefined,
        sourcePath: '',
        targetPath: '',
        hostIds: [],
        groupIds: [],
        concurrency: 5,
        timeoutSeconds: 30,
        overwrite: true
      }
    case 'approval':
      return {
        message: '\u7b49\u5f85\u4eba\u5de5\u786e\u8ba4',
        content: '\u8bf7\u786e\u8ba4\u540e\u7ee7\u7eed\u6267\u884c\u8be5\u4f5c\u4e1a'
      }
    case 'notify':
      return {
        notifyRuleId: undefined,
        message: '\u4f5c\u4e1a\u901a\u77e5',
        content: '\u4f5c\u4e1a\u6267\u884c\u5230\u6d88\u606f\u901a\u77e5\u6b65\u9aa4'
      }
    default:
      return {
        scriptId: undefined,
        variables: {},
        hostIds: [],
        groupIds: [],
        concurrency: 5
      }
  }
}

function buildNodeData(type) {
  const id = `job_node_${Date.now()}_${Math.random().toString(16).slice(2, 6)}`
  return {
    id,
    type,
    label: defaultNodeLabel(type),
    config: createDefaultConfig(type)
  }
}

function createGraphNode(data, position = {}) {
  const node = graph.addNode({
    id: data.id,
    shape: 'job-step-node',
    x: position.x ?? 80,
    y: position.y ?? 80,
    width: Math.max(position.width || 0, 248),
    height: Math.max(position.height || 0, data.type === 'condition' ? 148 : 120),
    data: cloneValue(data),
    ports: {
      groups: {
        top: {
          position: 'top',
          attrs: {
            circle: {
              r: 5,
              magnet: true,
              stroke: '#4f73ff',
              strokeWidth: 2,
              fill: '#fff'
            }
          }
        },
        bottom: {
          position: 'bottom',
          attrs: {
            circle: {
              r: 5,
              magnet: true,
              stroke: '#4f73ff',
              strokeWidth: 2,
              fill: '#fff'
            }
          }
        },
        true: { position: { name: 'bottom', args: { dx: -60 } }, attrs: { circle: { r: 5, magnet: true, stroke: '#138a78', strokeWidth: 2, fill: '#fff' } } },
        false: { position: { name: 'bottom', args: { dx: 60 } }, attrs: { circle: { r: 5, magnet: true, stroke: '#c07826', strokeWidth: 2, fill: '#fff' } } }
      },
      items: data.type === 'condition' ? [{ id: 'in', group: 'top' }, { id: 'true', group: 'true' }, { id: 'false', group: 'false' }] : [{ id: 'in', group: 'top' }, { id: 'out', group: 'bottom' }]
    }
  })
  updateNodePresentation(node)
  return node
}

function updateNodePresentation(node) {
  const data = node.getData() || {}
  const type = stepLibrary.find((item) => item.type === data.type) || stepLibrary[0]
  const config = data.config || {}
  const script = scriptOptions.value.find((item) => Number(item.id) === Number(config.scriptId))
  const rule = notifyRuleOptions.value.find((item) => Number(item.id) === Number(config.notifyRuleId))
  const detail = data.type === 'script' ? (script?.name || '请选择执行脚本')
    : data.type === 'file' ? (config.targetPath || '请配置源文件与目标路径')
      : data.type === 'approval' ? (config.message || '等待人工确认后继续')
        : data.type === 'condition' ? conditionSummary(config) : (rule?.name || '请选择通知规则')
  const target = config.hostIds?.length ? `${config.hostIds.length} 台主机`
    : config.groupIds?.length ? `${config.groupIds.length} 个主机组` : '未选择目标'
  const footer = ['script', 'file'].includes(data.type)
    ? `${target}   ·   并发 ${config.concurrency || 5}`
    : data.type === 'condition' ? '仅执行匹配的路径' : data.type === 'approval' ? '人工放行' : '通知步骤'
  const active = selectedNodeId.value === node.id
  node.setAttrs({
    body: { fill: '#ffffff', stroke: active ? type.color : '#dce3ee', strokeWidth: active ? 2 : 1, rx: 12, ry: 12 },
    accent: { fill: type.color },
    kind: { text: type.label, fill: type.color },
    label: { text: data.label || type.label },
    detail: { text: detail },
    footer: { text: footer },
    branchTrue: { text: '满足条件', display: data.type === 'condition' ? 'block' : 'none' },
    branchFalse: { text: '不满足', display: data.type === 'condition' ? 'block' : 'none' }
  })
}

function syncGraphSummary() {
  graphRevision.value++
  nodeCount.value = graph?.getNodes().length || 0
  edgeCount.value = graph?.getEdges().length || 0
}

function changeZoom(delta) {
  if (!graph) return
  graph.zoom(Math.min(1.5, Math.max(0.4, graph.zoom() + delta)), { absolute: true })
}

function fitCanvas() {
  if (!graph) return
  if (graph.getNodes().length) graph.zoomToFit({ padding: 56, maxScale: 1 })
  else graph.zoom(1, { absolute: true })
}

function loadSelectedNode(nodeId) {
  selectedNodeId.value = nodeId || ''
  selectedEdgeId.value = ''
  graph?.getNodes().forEach(updateNodePresentation)
  if (!graph || !nodeId) {
    Object.assign(selectedNodeForm, { id: '', type: 'script', label: '', config: {} })
    return
  }
  const node = graph.getCellById(nodeId)
  if (!node) return
  const data = node.getData() || {}
  Object.assign(selectedNodeForm, {
    id: data.id || node.id,
    type: data.type || 'script',
    label: data.label || node.attr('label/text') || '',
    config: cloneValue(data.config || {})
  })
}

function loadSelectedEdge(edgeId) {
  selectedNodeId.value = ''
  selectedEdgeId.value = edgeId || ''
  graph?.getNodes().forEach(updateNodePresentation)
}

function showEdgeTools(edge) {
  if (!edge) return
  edge.addTools([
    { name: 'source-arrowhead' },
    { name: 'target-arrowhead' }
  ])
}

function removeSelectedEdge() {
  if (!graph || !selectedEdgeId.value) return
  const edge = graph.getCellById(selectedEdgeId.value)
  if (!edge?.isEdge?.()) return
  graph.removeCell(edge)
  graph.cleanSelection()
  selectedCellIds.value = []
  loadSelectedEdge('')
  ElMessage.success('连线已删除')
}

function syncSelectionState() {
  if (!graph) return
  const selectedCells = graph.getSelectedCells()
  selectedCellIds.value = selectedCells.map((cell) => cell.id)
  const selectedNodes = selectedCells.filter((cell) => cell.isNode())
  if (selectedNodes.length === 1) {
    loadSelectedNode(selectedNodes[0].id)
    return
  }
  const selectedEdges = selectedCells.filter((cell) => cell.isEdge())
  if (selectedEdges.length === 1) {
    loadSelectedEdge(selectedEdges[0].id)
    return
  }
  loadSelectedNode('')
}

function setActiveNode(node) {
  if (!node) {
    selectedCellIds.value = []
    loadSelectedNode('')
    return
  }
  selectedCellIds.value = [node.id]
  loadSelectedNode(node.id)
}

function initGraph() {
  Graph.registerNode(
    'job-step-node',
    {
      inherit: 'rect',
      markup: [
        { tagName: 'rect', selector: 'body' },
        { tagName: 'rect', selector: 'accent' },
        { tagName: 'text', selector: 'kind' },
        { tagName: 'text', selector: 'label' },
        { tagName: 'text', selector: 'detail' },
        { tagName: 'rect', selector: 'divider' },
        { tagName: 'text', selector: 'footer' },
        { tagName: 'text', selector: 'branchTrue' },
        { tagName: 'text', selector: 'branchFalse' }
      ],
      attrs: {
        accent: { x: 16, y: 17, width: 3, height: 12, rx: 1.5, stroke: 'none' },
        kind: { refX: 26, refY: 23, fontSize: 10, fontWeight: 600, textAnchor: 'start', textVerticalAnchor: 'middle' },
        label: { refX: 16, refY: 46, fontSize: 14, fontWeight: 600, fill: '#1d2d49', textAnchor: 'start', textVerticalAnchor: 'middle', textWrap: { width: -32, height: 22, ellipsis: true } },
        detail: { refX: 16, refY: 69, fontSize: 11, fill: '#748399', textAnchor: 'start', textVerticalAnchor: 'middle', textWrap: { width: -32, height: 18, ellipsis: true } },
        divider: { x: 16, y: 87, width: 216, height: 1, fill: '#edf0f5', stroke: 'none' },
        footer: { refX: 16, refY: 104, fontSize: 10, fill: '#8591a5', textAnchor: 'start', textVerticalAnchor: 'middle' },
        branchTrue: { refX: 64, refY: 130, fontSize: 10, fill: '#138a78', textAnchor: 'middle', textVerticalAnchor: 'middle' },
        branchFalse: { refX: 184, refY: 130, fontSize: 10, fill: '#ad6c20', textAnchor: 'middle', textVerticalAnchor: 'middle' }
      }
    },
    true
  )
  graph = new Graph({
    container: graphContainer.value,
    grid: {
      size: 16,
      visible: true
    },
    background: {
      color: '#f7f9fc'
    },
    panning: true,
    mousewheel: {
      enabled: true,
      modifiers: ['ctrl', 'meta']
    },
    selecting: {
      enabled: true,
      multiple: true,
      rubberband: true,
      filter: ['node', 'edge'],
      showNodeSelectionBox: true,
      showEdgeSelectionBox: true
    },
    connecting: {
      snap: true,
      allowBlank: false,
      allowLoop: false,
      validateConnection({ sourceCell, targetCell, sourcePort, targetPort, edge }) {
        if (!sourceCell || !targetCell || sourceCell.id === targetCell.id || sourcePort === 'in' || targetPort !== 'in') return false
        if (targetCell.getData()?.type === 'condition') {
          if (!['script', 'file'].includes(sourceCell.getData()?.type) || (graph.getIncomingEdges(targetCell) || []).some((item) => item.id !== edge?.id)) return false
        }
        if (sourceCell.getData()?.type === 'condition' && (graph.getOutgoingEdges(sourceCell) || []).some((item) => item.id !== edge?.id && item.getSourcePortId() === sourcePort)) return false
        return true
      },
      highlight: true,
      connector: 'rounded',
      router: {
        name: 'manhattan'
      },
      createEdge() {
        return new Shape.Edge({
          attrs: {
            line: {
              stroke: '#4f73ff',
              strokeWidth: 2,
              cursor: 'pointer',
              targetMarker: {
                name: 'block',
                width: 12,
                height: 8
              }
            }
          }
        })
      }
    }
  })
  for (const eventName of ['node:added', 'node:removed', 'edge:added', 'edge:removed']) graph.on(eventName, syncGraphSummary)
  graph.on('scale', ({ sx }) => { zoomPercent.value = Math.round(sx * 100) })
  graph.on('node:change:data', ({ node }) => updateNodePresentation(node))
  graphResizeObserver = new ResizeObserver(([entry]) => {
    const { width, height } = entry.contentRect
    if (width && height) graph?.resize(width, height)
  })
  graphResizeObserver.observe(graphContainer.value)
  graph.on('cell:click', ({ cell }) => {
    if (cell.isNode()) {
      graph.cleanSelection()
      graph.select(cell)
      setActiveNode(cell)
      return
    }
    graph.cleanSelection()
    graph.select(cell)
    selectedCellIds.value = [cell.id]
    if (cell.isEdge()) {
      loadSelectedEdge(cell.id)
      return
    }
    loadSelectedNode('')
  })
  graph.on('node:selected', ({ node }) => {
    setActiveNode(node)
  })
  graph.on('edge:selected', ({ edge }) => {
    selectedCellIds.value = [edge.id]
    loadSelectedEdge(edge.id)
    showEdgeTools(edge)
  })
  graph.on('edge:click', ({ edge }) => {
    graph.cleanSelection()
    graph.select(edge)
    selectedCellIds.value = [edge.id]
    loadSelectedEdge(edge.id)
    showEdgeTools(edge)
  })
  graph.on('blank:click', () => {
    graph.getEdges().forEach((edge) => edge.removeTools())
    graph.cleanSelection()
    syncSelectionState()
  })
  graph.on('selection:changed', () => syncSelectionState())
  graph.on('edge:connected', ({ edge }) => {
    updateEdgePresentation(edge)
    const target = edge.getTargetNode()
    if (target?.getData()?.type === 'condition') {
      const data = target.getData()
      target.setData({ ...data, config: { ...data.config, sourceStepId: edge.getSourceCellId() } })
    }
    syncGraphSummary()
    graph.cleanSelection()
    graph.select(edge)
    selectedCellIds.value = [edge.id]
    loadSelectedEdge(edge.id)
    showEdgeTools(edge)
    if (selectedEdgeId.value === edge.id) loadSelectedEdge(edge.id)
  })
  graphReady.value = true
}

function updateEdgePresentation(edge) {
  const isCondition = edge.getSourceNode()?.getData()?.type === 'condition'
  const branch = isCondition ? edge.getSourcePortId() : ''
  const color = isCondition ? (branch === 'true' ? '#138a78' : '#c07826') : '#4f73ff'
  edge.setData({ branch })
  edge.setAttrs({ line: { stroke: color, strokeWidth: 2 } })
  edge.setLabels(isCondition ? [{ attrs: { label: { text: branch === 'true' ? '满足条件' : '不满足', fill: color, fontSize: 11 }, body: { fill: '#fff', stroke: '#e4e9f1', rx: 4, ry: 4 } }, position: 0.35 }] : [])
}

function serializeDefinition() {
  const nodes = graph.getNodes().map((node) => {
    const data = node.getData() || {}
    const position = node.position()
    const size = node.size()
    const config = ['script', 'file'].includes(data.type)
      ? normalizeNodeTargets(data.config)
      : cloneValue(data.config || {})
    return {
      id: data.id || node.id,
      type: data.type || 'script',
      label: data.label || node.attr('label/text') || '',
      config,
      meta: {
        x: position.x,
        y: position.y,
        width: size.width,
        height: size.height
      }
    }
  })
  const edges = graph.getEdges()
    .map((edge) => ({
      source: edge.getSourceCellId(),
      target: edge.getTargetCellId(),
      ...(edge.getSourceNode()?.getData()?.type === 'condition' ? { branch: edge.getSourcePortId() } : {})
    }))
    .filter((item) => item.source && item.target)
  return { nodes, edges }
}

function getGraphJson() {
  return JSON.stringify(graph.toJSON())
}

function loadDefinition(definition, graphJson = '') {
  if (!graph) return
  graph.clearCells()
  const positionMap = {}
  try {
    const parsed = JSON.parse(graphJson || '{}')
    for (const cell of parsed.cells || []) {
      if (cell.shape === 'edge') continue
      positionMap[cell.id] = {
        x: cell.x,
        y: cell.y,
        width: cell.width,
        height: cell.height
      }
    }
  } catch (error) {
    console.warn(error)
  }
  let row = 0
  for (const node of definition.nodes || []) {
    const metaPosition = node.meta || {}
    const cell = createGraphNode(node, {
      x: positionMap[node.id]?.x ?? metaPosition.x ?? 80 + (row % 3) * 260,
      y: positionMap[node.id]?.y ?? metaPosition.y ?? 80 + Math.floor(row / 3) * 140,
      width: positionMap[node.id]?.width ?? metaPosition.width ?? 248,
      height: positionMap[node.id]?.height ?? metaPosition.height ?? 120
    })
    cell.setData({
      id: node.id,
      type: node.type,
      label: node.label,
      config: cloneValue(node.config || {})
    })
    row += 1
  }
  for (const edge of definition.edges || []) {
    if (!edge.source || !edge.target) continue
    if (!graph.getCellById(edge.source) || !graph.getCellById(edge.target)) continue
    const cell = graph.addEdge({
      source: { cell: edge.source, port: graph.getCellById(edge.source).getData()?.type === 'condition' ? edge.branch : 'out' },
      target: { cell: edge.target, port: 'in' },
      router: { name: 'manhattan' },
      connector: 'rounded',
      attrs: {
        line: {
          stroke: '#4f73ff',
          strokeWidth: 2,
          cursor: 'pointer',
          targetMarker: {
            name: 'block',
            width: 12,
            height: 8
          }
        }
      }
    })
    updateEdgePresentation(cell)
  }
  loadSelectedNode('')
  fitCanvas()
}

function addStep(type) {
  if (!graph) return
  const nodeData = buildNodeData(type)
  const columns = graphContainer.value.clientWidth >= 650 ? 2 : 1
  createGraphNode(nodeData, {
    x: 64 + (graph.getNodes().length % columns) * 280,
    y: 56 + Math.floor(graph.getNodes().length / columns) * 168
  })
  graph.cleanSelection()
  graph.select(graph.getCellById(nodeData.id))
  setActiveNode(graph.getCellById(nodeData.id))
  fitCanvas()
}

function removeSelectedNode() {
  if (!graph) return
  const selectedCells = graph.getSelectedCells()
  const cellsToRemove = []
  const appended = new Set()

  const appendCell = (cell) => {
    if (!cell || appended.has(cell.id)) return
    appended.add(cell.id)
    cellsToRemove.push(cell)
  }

  for (const cell of selectedCells) {
    appendCell(cell)
    if (cell.isNode?.()) {
      const edges = graph.getConnectedEdges(cell) || []
      edges.forEach(appendCell)
    }
  }

  if (!cellsToRemove.length && selectedNodeId.value) {
    const node = graph.getCellById(selectedNodeId.value)
    if (node) {
      appendCell(node)
      const edges = graph.getConnectedEdges(node) || []
      edges.forEach(appendCell)
    }
  }

  if (!cellsToRemove.length) return
  graph.removeCells(cellsToRemove)
  graph.cleanSelection()
  syncSelectionState()
}

function handleCanvasKeydown(event) {
  if (!graph) return
  const isDelete = event.key === 'Delete' || event.key === 'Backspace'
  if (!isDelete || !selectedCellIds.value.length) return
  const target = event.target
  const tagName = target?.tagName?.toLowerCase?.() || ''
  const editable = target?.isContentEditable || ['input', 'textarea'].includes(tagName)
  if (editable) return
  event.preventDefault()
  removeSelectedNode()
}

async function clearCanvas() {
  await ElMessageBox.confirm('确认清空当前编排画布吗？', '提示', { type: 'warning' })
  graph.clearCells()
  loadSelectedNode('')
}

async function loadBaseOptions() {
  const [scripts, hosts, groups, templates, notifyRules] = await Promise.all([
    queryOpsScriptOptions(),
    queryAssetHostList({ pageNum: 1, pageSize: 1000 }),
    queryAssetHostGroupList(),
    queryOpsJobTemplateOptions(),
    queryNotifyRuleOptions({ scope: 'job' })
  ])
  scriptOptions.value = scripts || []
  hostOptions.value = hosts.list || []
  groupOptions.value = groups.tree || []
  templateOptions.value = templates || []
  notifyRuleOptions.value = notifyRules || []
  graph?.getNodes().forEach(updateNodePresentation)
}

async function loadCurrentRecord() {
  const id = Number(route.query.id || 0)
  if (!id) return
  loading.value = true
  try {
    const data = isTemplateMode.value ? await opsJobTemplateInfo(id) : await opsJobInfo(id)
    form.id = data.id
    form.name = data.name || ''
    form.description = data.description || ''
    form.status = data.status || 1
    form.templateId = data.templateId || undefined
    form.notifyEnabled = !!data.notifyEnabled
    form.notifyRuleId = data.notifyRuleId || undefined
    const definition = JSON.parse(data.definitionJson || '{"nodes":[],"edges":[]}')
    loadDefinition(definition, data.graphJson || '')
  } finally {
    loading.value = false
  }
}

async function importTemplate() {
  if (!importTemplateId.value) {
    ElMessage.warning('请先选择作业模板')
    return
  }
  const data = await opsJobTemplateInfo(importTemplateId.value)
  form.templateId = data.id
  if (!form.name.trim()) {
    form.name = data.name || ''
  }
  if (!form.description.trim()) {
    form.description = data.description || ''
  }
  const definition = JSON.parse(data.definitionJson || '{"nodes":[],"edges":[]}')
  loadDefinition(definition, data.graphJson || '')
  importDialogVisible.value = false
  ElMessage.success('模板已导入到编排画布')
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning(isTemplateMode.value ? '请输入模板名称' : '请输入作业名称')
    return
  }
  const definition = serializeDefinition()
  if (!definition.nodes.length) {
    ElMessage.warning('请至少添加一个步骤')
    return
  }
  const branchError = validateJobBranches(definition)
  if (branchError) return ElMessage.warning(branchError)
  const invalidNotifyNode = definition.nodes.find((node) => node.type === 'notify' && !node.config?.notifyRuleId)
  if (invalidNotifyNode) {
    ElMessage.warning('\u8bf7\u4e3a\u6d88\u606f\u901a\u77e5\u6b65\u9aa4\u9009\u62e9\u901a\u77e5\u89c4\u5219')
    return
  }
  saving.value = true
  try {
    const payload = {
      id: form.id,
      name: form.name,
      description: form.description,
      status: form.status,
      templateId: form.templateId,
      notifyEnabled: false,
      notifyRuleId: undefined,
      graphJson: getGraphJson(),
      definitionJson: JSON.stringify(definition)
    }
    if (isTemplateMode.value) {
      if (form.id) {
        await updateOpsJobTemplate(payload)
      } else {
        await addOpsJobTemplate(payload)
      }
      ElMessage.success('作业模板已保存')
      router.push('/ops/jobs/templates')
      return
    }
    if (form.id) {
      await updateOpsJob(payload)
    } else {
      await addOpsJob(payload)
    }
    ElMessage.success('作业已保存')
    router.push('/ops/jobs/list')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await nextTick()
  initGraph()
  window.addEventListener('keydown', handleCanvasKeydown)
  await loadBaseOptions()
  await loadCurrentRecord()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleCanvasKeydown)
  graphResizeObserver?.disconnect()
  if (graph) {
    graph.dispose()
    graph = null
  }
})
</script>

<template>
  <div class="ops-job-page job-designer">
    <header class="designer-header">
      <div class="designer-heading">
        <span class="designer-heading-icon"><el-icon><Connection /></el-icon></span>
        <div>
        <h2 class="page-title">{{ editorTitle }}</h2>
        <p class="page-desc">连接每个操作，让运维流程有序执行。</p>
        </div>
        <span class="editing-badge">{{ form.id ? '编辑中' : '新建' }}</span>
      </div>
      <div class="head-actions">
        <el-button :icon="Upload" @click="importDialogVisible = true">导入模板</el-button>
        <el-button type="primary" :icon="Check" :loading="saving" @click="save">{{ saveButtonText }}</el-button>
      </div>
    </header>

    <div class="page-card record-form">
      <div class="record-heading"><span><el-icon><Setting /></el-icon>基本信息</span><small>为这次编排定义名称与用途</small></div>
      <el-form label-position="top">
        <div class="record-fields">
          <div class="record-name">
            <el-form-item :label="isTemplateMode ? '模板名称' : '作业名称'" required>
              <el-input v-model="form.name" :placeholder="isTemplateMode ? '例如：标准发布模板' : '例如：生产环境应用发布'" />
            </el-form-item>
          </div>
          <div class="record-description">
            <el-form-item label="描述">
              <el-input v-model="form.description" placeholder="说明作业用途、执行顺序或注意事项" />
            </el-form-item>
          </div>
          <div v-if="!isTemplateMode">
            <el-form-item label="来源模板">
              <el-select v-model="form.templateId" clearable filterable placeholder="不使用模板">
                <el-option v-for="item in templateOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
          </div>
          <div class="record-status">
            <el-form-item label="状态">
              <div class="status-switch"><el-switch v-model="form.status" :active-value="1" :inactive-value="2" /><span>{{ form.status === 1 ? '已启用' : '已禁用' }}</span></div>
            </el-form-item>
          </div>
        </div>
      </el-form>
    </div>

    <div class="designer-layout">
      <aside class="page-card left-palette">
        <div class="panel-header"><div class="panel-title">步骤库</div><span class="count-badge">{{ stepLibrary.length }}</span></div>
        <p class="panel-description">点击添加到画布</p>
        <div class="step-library">
          <button v-for="step in stepLibrary" :key="step.type" type="button" class="step-card" :style="{ '--step-color': step.color, '--step-soft': step.soft }" @click="addStep(step.type)">
            <span class="step-card-icon"><el-icon><component :is="step.icon" /></el-icon></span>
            <span class="step-card-copy"><strong>{{ step.label }}</strong><small>{{ step.description }}</small></span>
            <el-icon class="step-add"><Plus /></el-icon>
          </button>
        </div>
        <div class="palette-guide"><el-icon><InfoFilled /></el-icon><div><strong>连接，即是执行顺序</strong><p>从步骤底部的连接点拖动到下一个步骤顶部，即可创建执行关系。</p></div></div>
        <div class="palette-footer"><el-icon><Mouse /></el-icon><span>拖动节点调整位置<br />Ctrl / ⌘ + 滚轮缩放</span></div>
      </aside>

      <div class="page-card canvas-panel">
        <div class="canvas-header">
          <div class="panel-title"><el-icon><Grid /></el-icon>编排画布<span class="canvas-count">{{ nodeCount }} 步骤<span>·</span>{{ edgeCount }} 连线</span></div>
          <div class="canvas-actions">
            <el-tooltip :content="selectedEdgeId ? '删除选中连线' : '删除选中步骤'" placement="top"><button type="button" class="canvas-icon-button danger-action" aria-label="删除选中项" @click="selectedEdgeId ? removeSelectedEdge() : removeSelectedNode()" :disabled="!selectedCount && !selectedNodeId && !selectedEdgeId"><el-icon><Delete /></el-icon></button></el-tooltip>
            <span class="toolbar-divider" />
            <el-button text size="small" @click="clearCanvas" :disabled="!graphReady || !nodeCount">清空画布</el-button>
          </div>
        </div>
        <div class="canvas-workspace">
          <div ref="graphContainer" v-loading="loading" class="graph-container" />
          <div v-if="!nodeCount && !loading" class="canvas-empty"><span class="canvas-empty-icon"><el-icon><Connection /></el-icon></span><h3>从第一个步骤开始</h3><p>从左侧步骤库添加操作，再连接它们的执行顺序。</p><el-button :icon="Plus" @click="addStep('script')">添加脚本步骤</el-button></div>
          <div class="canvas-legend"><span v-for="step in stepLibrary" :key="step.type"><i :style="{ background: step.color }" />{{ step.label }}</span></div>
          <div class="canvas-zoom"><button type="button" class="canvas-icon-button" aria-label="缩小画布" :disabled="zoomPercent <= 40" @click="changeZoom(-0.1)"><el-icon><ZoomOut /></el-icon></button><span>{{ zoomPercent }}%</span><button type="button" class="canvas-icon-button" aria-label="放大画布" :disabled="zoomPercent >= 150" @click="changeZoom(0.1)"><el-icon><ZoomIn /></el-icon></button><span class="toolbar-divider" /><button type="button" class="canvas-icon-button" aria-label="适应画布" title="适应画布" @click="fitCanvas"><el-icon><Aim /></el-icon></button></div>
        </div>
        <div class="canvas-footer"><span><el-icon><Connection /></el-icon>连接点决定步骤执行顺序</span><span v-if="selectedCount">已选择 {{ selectedCount }} 项 · Delete 删除</span><span v-else>点击步骤配置参数</span></div>
      </div>

      <aside class="page-card config-panel">
        <div class="panel-header inspector-header"><div class="panel-title"><el-icon><Setting /></el-icon>{{ selectedEdgeId ? '连线操作' : '步骤配置' }}</div><span v-if="selectedNodeId" class="type-badge" :style="{ color: selectedStepType.color, background: selectedStepType.soft }">{{ selectedStepType.label }}</span></div>
        <div v-if="!selectedNodeId && !selectedEdgeId" class="inspector-empty"><span><el-icon><Mouse /></el-icon></span><h3>选择一个步骤</h3><p>点击画布中的步骤卡片，<br />在这里配置执行内容与目标。</p></div>
        <div v-else-if="selectedEdgeId" class="inspector-empty"><span><el-icon><Connection /></el-icon></span><h3>{{ selectedBranchLabel() || '调整执行关系' }}</h3><p>拖动连线两端的手柄，<br />即可重新连接步骤。</p><el-button type="danger" plain :icon="Delete" @click="removeSelectedEdge">删除此连线</el-button></div>
        <el-form v-else-if="selectedNodeId" label-position="top" class="inspector-form">
          <div class="config-section-heading">基础信息</div>
          <el-form-item label="步骤名称">
            <el-input v-model="selectedNodeForm.label" placeholder="为步骤命名" />
          </el-form-item>

          <template v-if="selectedNodeForm.type === 'script'">
            <div class="config-section-heading">执行内容</div>
            <el-form-item label="脚本">
              <el-select v-model="selectedNodeForm.config.scriptId" filterable placeholder="选择脚本" @change="handleSelectedScriptChange">
                <el-option v-for="item in scriptOptions" :key="item.id" :label="item.name" :value="item.id" />
              </el-select>
            </el-form-item>
            <div class="job-variable-panel">
              <div class="job-variable-panel__title">步骤变量</div>
              <div class="job-variable-panel__hint">运行时将以 <code>VARIABLE_变量名</code> 注入脚本；敏感值保存后不会回显。</div>
              <div v-if="!selectedScriptVariables.length" class="job-variable-panel__empty">此脚本未声明变量。</div>
              <div v-else class="job-variable-list">
                <div v-for="variable in selectedScriptVariables" :key="variable.name" class="job-variable-field">
                  <div class="job-variable-field__label"><code>VARIABLE_{{ variable.name }}</code><el-tag v-if="variable.required" size="small" type="danger" effect="plain">必填</el-tag></div>
                  <el-input v-model="selectedNodeForm.config.variables[variable.name]" :type="variable.secret ? 'password' : 'text'" :show-password="variable.secret" :placeholder="variable.secret ? '已配置时留空可保留原值' : (variable.defaultValue || '请输入变量值')" />
                  <div v-if="variable.description" class="job-variable-field__desc">{{ variable.description }}</div>
                </div>
              </div>
            </div>
            <div class="config-section-heading">执行参数</div>
            <div class="config-field-pair">
              <el-form-item label="并发数">
                <el-input-number v-model="selectedNodeForm.config.concurrency" :min="1" :max="10" style="width: 100%" />
              </el-form-item>
              <el-form-item label="脚本超时">
                <el-input :model-value="selectedScriptTimeout" disabled>
                  <template #append>s</template>
                </el-input>
              </el-form-item>
            </div>
            <el-form-item label="失败时">
              <el-select v-model="selectedNodeForm.config.failurePolicy" placeholder="停止作业">
                <el-option label="停止作业（默认）" value="stop" />
                <el-option label="进入后续条件分支" value="branch" />
              </el-select>
              <div v-if="selectedNodeForm.config.failurePolicy === 'branch'" class="form-tip">所有后续步骤必须为判断此步骤的条件分支；失败结果会保留在执行历史中。</div>
            </el-form-item>
            <div class="config-section-heading">目标范围</div>
            <OpsTargetScope
              :host-options="hostOptions"
              :group-options="groupOptions"
              :host-ids="selectedNodeForm.config.hostIds || []"
              :group-ids="selectedNodeForm.config.groupIds || []"
              @update:host-ids="updateSelectedHostIds"
              @update:group-ids="updateSelectedGroupIds"
            />
          </template>

          <template v-else-if="selectedNodeForm.type === 'file'">
            <div class="config-section-heading">分发内容</div>
            <el-form-item label="源主机">
              <el-select v-model="selectedNodeForm.config.sourceHostId" filterable placeholder="选择源主机">
                <el-option v-for="item in hostOptions" :key="item.id" :label="item.hostName" :value="item.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="源文件路径">
              <el-input v-model="selectedNodeForm.config.sourcePath" placeholder="/opt/app/config.yml" />
            </el-form-item>
            <el-form-item label="目标路径">
              <el-input v-model="selectedNodeForm.config.targetPath" placeholder="/opt/app/config.yml" />
            </el-form-item>
            <div class="config-section-heading">执行参数</div>
            <div class="config-field-pair">
              <el-form-item label="并发数">
                <el-input-number v-model="selectedNodeForm.config.concurrency" :min="1" :max="10" style="width: 100%" />
              </el-form-item>
              <el-form-item label="超时秒数">
                <el-input-number v-model="selectedNodeForm.config.timeoutSeconds" :min="10" :max="3600" style="width: 100%" />
              </el-form-item>
            </div>
            <el-form-item label="覆盖目标文件">
              <el-switch v-model="selectedNodeForm.config.overwrite" />
            </el-form-item>
            <el-form-item label="失败时">
              <el-select v-model="selectedNodeForm.config.failurePolicy" placeholder="停止作业">
                <el-option label="停止作业（默认）" value="stop" />
                <el-option label="进入后续条件分支" value="branch" />
              </el-select>
              <div v-if="selectedNodeForm.config.failurePolicy === 'branch'" class="form-tip">所有后续步骤必须为判断此步骤的条件分支；失败结果会保留在执行历史中。</div>
            </el-form-item>
            <div class="config-section-heading">目标范围</div>
            <OpsTargetScope
              :host-options="hostOptions"
              :group-options="groupOptions"
              :host-ids="selectedNodeForm.config.hostIds || []"
              :group-ids="selectedNodeForm.config.groupIds || []"
              @update:host-ids="updateSelectedHostIds"
              @update:group-ids="updateSelectedGroupIds"
            />
          </template>
          <template v-else-if="selectedNodeForm.type === 'condition'">
            <div class="config-section-heading">判断条件</div>
            <el-form-item label="判断哪个步骤" required>
              <el-select v-model="selectedNodeForm.config.sourceStepId" placeholder="先连接一个脚本或文件步骤">
                <el-option v-for="item in conditionSourceOptions" :key="item.id" :label="item.label" :value="item.id" />
              </el-select>
              <div class="form-tip">判断直接连接到此分支的前置步骤。</div>
            </el-form-item>
            <el-form-item label="判断什么">
              <el-select v-model="selectedNodeForm.config.field" @change="handleConditionFieldChange"><el-option v-for="item in conditionFields" :key="item.value" :label="item.label" :value="item.value" /></el-select>
            </el-form-item>
            <el-form-item label="比较方式">
              <el-select v-model="selectedNodeForm.config.operator"><el-option v-for="item in selectedConditionOperators" :key="item.value" :label="item.label" :value="item.value" /></el-select>
            </el-form-item>
            <el-form-item label="比较值">
              <el-select v-if="selectedNodeForm.config.field === 'status'" v-model="selectedNodeForm.config.value"><el-option v-for="item in conditionStatuses" :key="item.value" :label="item.label" :value="item.value" /></el-select>
              <el-input-number v-else v-model="selectedNodeForm.config.value" :min="0" :max="2147483647" :precision="0" style="width: 100%" />
            </el-form-item>
            <div class="condition-preview"><strong>{{ conditionSummary(selectedNodeForm.config) }}</strong><p>满足条件时走绿色出口，不满足时走橙色出口。两个出口都需连接后续步骤。</p></div>
            <div class="form-tip">如需处理前置步骤失败的结果，请在该步骤的“失败时”中选择“进入后续条件分支”。</div>
          </template>
          <template v-else-if="selectedNodeForm.type === 'notify'">
            <div class="config-section-heading">通知设置</div>
            <el-form-item label="&#x901a;&#x77e5;&#x89c4;&#x5219;" required>
              <el-select v-model="selectedNodeForm.config.notifyRuleId" filterable placeholder="&#x9009;&#x62e9;&#x901a;&#x77e5;&#x89c4;&#x5219;">
                <el-option v-for="item in notifyRuleOptions" :key="item.id" :label="`${item.name} · 作业编排`" :value="item.id" />
              </el-select>
              <div class="form-tip">&#x4ec5;&#x5c55;&#x793a;&#x9002;&#x7528;&#x4e8e;&#x4f5c;&#x4e1a;&#x7f16;&#x6392;&#x7684;&#x901a;&#x77e5;&#x89c4;&#x5219;&#xff0c;&#x5361;&#x7247;&#x4f1a;&#x81ea;&#x52a8;&#x5e26;&#x4e0a;&#x4f5c;&#x4e1a;&#x4e0e;&#x5f53;&#x524d;&#x6b65;&#x9aa4;&#x4fe1;&#x606f;&#x3002;</div>
            </el-form-item>
            <el-form-item label="&#x901a;&#x77e5;&#x6458;&#x8981;">
              <el-input v-model="selectedNodeForm.config.message" placeholder="&#x4f8b;&#x5982;&#xff1a;&#x53d1;&#x5e03;&#x5b8c;&#x6210;&#x901a;&#x77e5;" />
            </el-form-item>
            <el-form-item label="&#x901a;&#x77e5;&#x5185;&#x5bb9;">
              <el-input v-model="selectedNodeForm.config.content" type="textarea" :rows="6" placeholder="&#x8fd9;&#x91cc;&#x4f1a;&#x4f5c;&#x4e3a; {{detail}} &#x53d8;&#x91cf;&#x4f20;&#x7ed9;&#x6d88;&#x606f;&#x6a21;&#x677f;" />
            </el-form-item>
          </template>

          <template v-else>
            <div class="config-section-heading">确认设置</div>
            <el-form-item label="确认提示">
              <el-input v-model="selectedNodeForm.config.message" placeholder="例如：请确认窗口维护已完成" />
            </el-form-item>
            <el-form-item label="确认说明">
              <el-input v-model="selectedNodeForm.config.content" type="textarea" :rows="6" placeholder="在这里填写人工确认节点的操作说明、注意事项和放行条件" />
            </el-form-item>
          </template>
        </el-form>
        <div v-if="selectedNodeId" class="inspector-footer"><el-icon><Lock /></el-icon>参数随作业一同保存</div>
      </aside>
    </div>

    <el-dialog v-model="importDialogVisible" title="导入作业模板" width="520px">
      <el-form label-width="90px">
        <el-form-item label="作业模板">
          <el-select v-model="importTemplateId" filterable placeholder="选择要导入的模板" style="width: 100%">
            <el-option v-for="item in templateOptions" :key="item.id" :label="item.name" :value="item.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="importTemplate">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped src="./ops-job-designer.css"></style>
