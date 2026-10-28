<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Bell, Connection, DataAnalysis, Monitor, Operation, Refresh, SetUp } from '@element-plus/icons-vue'
import { appDefinitions } from '../utils/apps'
import './BusinessTopology.css'

const router = useRouter()
const selectedKey = ref('')

const capabilityMap = {
  console: { icon: Monitor, tone: 'blue', role: '统一入口', description: '提供统一工作台，管理用户、用户组、访问策略、资源授权、系统设置与操作审计。', capabilities: ['仪表盘与全局视图', '业务拓扑图', '用户与用户组管理', '访问策略与资源授权', '系统设置与菜单配置', '登录与操作审计'], consumers: ['assets', 'containers', 'ops', 'applications', 'notify', 'integration', 'monitor', 'domains'] },
  assets: { icon: SetUp, tone: 'cyan', role: '资源底座', description: '统一管理环境、主机与主机组、凭据、云资产和 SSH 网关；提供数据库工作台、数据导入备份，以及 RabbitMQ、Kafka、Nacos、etcd、ZooKeeper 中间件管理。', capabilities: ['资产概览', '环境模型', '主机与主机组管理', '凭据与命令过滤', '云账号与资产同步', '数据库与 SQL 工作台', '数据导入与备份管理', 'RabbitMQ / Kafka', 'Nacos / etcd / ZooKeeper', '网关管理', '终端登录'], consumers: ['containers', 'ops', 'applications', 'monitor', 'domains'] },
  containers: { icon: Connection, tone: 'teal', role: '容器运行', description: '管理业务服务与资源拓扑，提供服务健康诊断、Kubernetes 集群部署、节点添加、工作负载伸缩、容器终端、网络、配置存储和集群监控。', capabilities: ['服务管理与资源拓扑', '服务健康诊断', 'K8s 集群部署', '集群管理与概览', '节点管理与节点添加', '命名空间管理', '工作负载与弹性伸缩', 'Pod 与容器终端', 'Service / Ingress 网络', '配置与存储', '集群监控'], consumers: ['applications', 'monitor', 'notify'] },
  ops: { icon: Operation, tone: 'violet', role: '运维执行', description: '基于资产与环境执行命令、参数化脚本和文件分发，支持定时任务、HTTP 探针、可视化作业、人工确认、失败重试与任务模板。', capabilities: ['脚本库与参数模板', '命令与脚本执行', '文件分发', '定时任务与 HTTP 探针', '可视化作业编排', '人工确认', '执行历史与失败重试', '任务与作业模板'], consumers: ['applications', 'notify', 'monitor'] },
  applications: { icon: Connection, tone: 'green', role: '应用交付', description: '管理应用项目与 Git/SVN 仓库，配置构建发布任务、镜像仓库和 CI/CD 流水线，并追踪构建阶段与执行日志。', capabilities: ['应用与 Git/SVN 仓库', '构建与部署任务', '构建历史与日志', '镜像仓库管理', 'CI/CD 流水线'], consumers: ['containers', 'monitor', 'notify'] },
  notify: { icon: Bell, tone: 'pink', role: '消息触达', description: '通过通知规则、消息模板与钉钉、企微、飞书或自定义 Webhook 触达相关人员，并查看发送请求、响应与失败原因。', capabilities: ['通知规则', '消息模板', '通知媒介', '发送日志与失败排查'], consumers: ['console'] },
  integration: { icon: Connection, tone: 'indigo', role: '外部集成与智能分析', description: '提供分组导航与公开入口、AI 对话与会话、模型、工具集和知识库管理，以及多云账单同步、费用拆分、资源分析和成本优化建议。', capabilities: ['分组导航与公开入口', 'AI 助手与会话管理', '模型管理', 'AI 工具集', '知识库管理', '云费用看板', '云账单账号与同步', '费用拆分与资源分析', '成本优化建议'], consumers: ['console'] },
  monitor: { icon: DataAnalysis, tone: 'orange', role: '可观测性', description: '统一查看指标、日志、调用链与资源热点，提供智能运维大屏、告警模板、事件处置、屏蔽与聚合收敛，以及监控大屏和巡检报告。', capabilities: ['监控概览', '智能运维大屏', '监控数据源', '指标即时查询', '日志检索与上下文', '链路追踪', '告警模板与规则', '告警事件与处置', '告警屏蔽与聚合收敛', '监控大屏', '巡检大屏与报告'], consumers: ['notify', 'integration'] },
  domains: { icon: Connection, tone: 'purple', role: '域名与解析', description: '统一管理公网域名及解析记录、DNS 账号、SSL 证书、内网 Zone 与解析记录，提供 DNS 设置、解析测试及操作审计。', capabilities: ['公网域名与解析记录', 'DNS 账号管理', 'SSL 证书管理', '内网 Zone 与解析记录', '内网 DNS 设置', '解析测试与诊断', '操作审计'], consumers: ['assets', 'monitor', 'notify'] }
}

const nodes = computed(() => appDefinitions.filter(item => capabilityMap[item.key]).map(item => ({ ...item, ...capabilityMap[item.key] })))
const selectedNode = computed(() => nodes.value.find(item => item.key === selectedKey.value) || null)
const capabilityCount = computed(() => nodes.value.reduce((total, item) => total + item.capabilities.length, 0))
const edges = computed(() => nodes.value.flatMap(item => item.consumers.filter(target => nodes.value.some(node => node.key === target)).map(target => ({ source: item.key, target }))))

function openApp(node) { router.push(node.defaultRoute) }
function selectNode(key) { selectedKey.value = selectedKey.value === key ? '' : key }
</script>

<template>
  <main class="business-topology-page">
    <section class="topology-hero">
      <div><p>OPS ADMIN · BUSINESS TOPOLOGY</p><h1>业务拓扑图</h1><span>以“应用 → 多项业务能力”的方式展示当前平台的业务能力版图。</span></div>
      <div class="hero-meta"><div><strong>{{ nodes.length }}</strong><span>个业务应用</span></div><div><strong>{{ capabilityCount }}</strong><span>项实现能力</span></div></div>
    </section>

    <section class="topology-legend"><div><span class="legend-resource"></span>资源与数据</div><div><span class="legend-execution"></span>运维与交付</div><div><span class="legend-observe"></span>监控与告警</div><div><span class="legend-output"></span>通知与智能分析</div><p>点击应用或能力卡片查看详细职责；双击应用进入对应工作台。</p></section>

    <section class="topology-canvas">
      <header><strong>{{ nodes.length }} 个应用</strong><b>→</b><strong>{{ capabilityCount }} 项业务能力</strong><span>每一列代表一个应用及其实现功能</span></header>
      <div class="capability-lanes">
        <article v-for="node in nodes" :key="node.key" class="capability-lane" :class="[`tone-${node.tone}`, { selected: selectedKey === node.key }]">
          <button class="application-parent" type="button" @click="selectNode(node.key)" @dblclick="openApp(node)">
            <i class="app-icon"><el-icon><component :is="node.icon" /></el-icon></i><span><small>{{ node.role }}</small><b>{{ node.name }}</b></span><em title="进入应用" @click.stop="openApp(node)">↗</em>
          </button>
          <div class="stem"></div>
          <button v-for="capability in node.capabilities" :key="capability" type="button" class="capability-node" @click="selectNode(node.key)">{{ capability }}</button>
        </article>
      </div>
    </section>

    <section class="capability-section">
      <div class="section-heading"><div><p>APPLICATION CAPABILITIES</p><h2>{{ selectedNode ? selectedNode.name : '应用功能说明' }}</h2></div><el-button :icon="Refresh" @click="selectedKey = ''">清除选择</el-button></div>
      <div v-if="selectedNode" class="capability-detail" :class="`tone-${selectedNode.tone}`"><div><span>应用定位</span><strong>{{ selectedNode.role }}</strong><p>{{ selectedNode.description }}</p></div><div><span>实现功能</span><ul><li v-for="item in selectedNode.capabilities" :key="item">{{ item }}</li></ul></div><div><span>业务输出给</span><ul><li v-for="key in selectedNode.consumers" :key="key">{{ nodes.find(item => item.key === key)?.name }}</li></ul></div></div>
      <div v-else class="capability-grid"><article v-for="node in nodes" :key="node.key" :class="`tone-${node.tone}`" @click="selectNode(node.key)"><strong>{{ node.name }}</strong><p>{{ node.description }}</p></article></div>
    </section>

    <section class="topology-note"><el-icon><Connection /></el-icon><div><strong>链路说明</strong><p>这里展示平台的业务依赖与信息输出方向，用于理解能力边界和联动路径，不替代实时调用链或资源依赖图。</p></div><span>{{ edges.length }} 条业务关系</span></section>
  </main>
</template>
