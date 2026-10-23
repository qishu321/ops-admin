const retiredK8sPanelTitles = new Set(['Pod 累计重启次数 Top', 'Pod 最近 1 小时新增重启 Top'])

const hostAggregateTrendModes = new Map([
  ['CPU 使用趋势', 'average'],
  ['内存使用趋势', 'average'],
  ['磁盘使用率', 'average'],
  ['磁盘 IOPS（次/秒）', 'sum']
])

const k8sPodPanelDefinitions = [
  { title: 'Pod CPU 使用量 Top', chartType: 'line', unit: 'Core', span: 12, promql: 'topk(10, sum by(namespace, pod) (irate(container_cpu_usage_seconds_total{container!="",container!="POD",pod!=""}[5m])))' },
  { title: 'Pod 内存使用量 Top', chartType: 'line', unit: 'B', span: 12, promql: 'topk(10, sum by(namespace, pod) (container_memory_working_set_bytes{container!="",pod!=""}))' },
  { title: 'Pod CPU 使用趋势', chartType: 'line', unit: 'Core', span: 12, promql: 'topk(10, sum by(namespace, pod) (irate(container_cpu_usage_seconds_total{container!="",container!="POD",pod!=""}[5m])))' },
  { title: 'Pod 内存使用趋势', chartType: 'line', unit: 'B', span: 12, promql: 'topk(10, sum by(namespace, pod) (container_memory_working_set_bytes{container!="",pod!=""}))' },
  { title: 'Pod 网络接收速率 Top', chartType: 'bar', unit: 'B/s', span: 12, promql: 'topk(10, sum by(namespace, pod) (rate(container_network_receive_bytes_total{pod!=""}[5m])))' },
  { title: 'Pod 网络发送速率 Top', chartType: 'bar', unit: 'B/s', span: 12, promql: 'topk(10, sum by(namespace, pod) (rate(container_network_transmit_bytes_total{pod!=""}[5m])))' },
  { title: 'Pod 资源明细', chartType: 'table', unit: '', span: 24, promql: 'topk(10, sum by(namespace, pod) (container_memory_working_set_bytes{container!="",pod!=""}))' }
]

const k8sTrendPanelTitles = new Set([
  'Pod CPU 使用量 Top',
  'Pod 内存使用量 Top',
  'Pod CPU 使用趋势',
  'Pod 内存使用趋势'
])

const k8sPanelTitleMap = {
  'Pod CPU 使用量 Top': 'Pod CPU 使用量 Top 10（核）',
  'Pod 内存使用量 Top': 'Pod 内存使用量 Top 10（MiB）',
  'Pod CPU 使用趋势': 'Pod CPU 使用趋势（核）',
  'Pod 内存使用趋势': 'Pod 内存使用趋势（MiB）',
  'Pod 网络接收速率 Top': 'Pod 网络流入 Top 10',
  'Pod 网络发送速率 Top': 'Pod 网络流出 Top 10'
}

const dashboardTemplates = [
  {
    key: 'blank',
    name: '空白大屏',
    description: '创建一个空白大屏，手动添加 PromQL 面板。',
    panels: []
  },
  {
    key: 'host',
    name: '主机资源大屏',
    description: '参考 Node Exporter 大屏，覆盖在线状态、CPU、内存、磁盘、网络和系统负载。',
    panels: [
      { title: '主机总数', chartType: 'stat', unit: '台', span: 6, promql: 'count(up{job=~"node.*|node-exporter"})' },
      { title: '在线主机', chartType: 'stat', unit: '台', span: 6, promql: 'sum(up{job=~"node.*|node-exporter"} == 1)' },
      { title: '离线主机', chartType: 'stat', unit: '台', span: 6, promql: 'sum(up{job=~"node.*|node-exporter"} == 0)' },
      { title: '平均 CPU 使用率', chartType: 'gauge', unit: '%', span: 6, promql: '100 - (avg(irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' },
      { title: '平均内存使用率', chartType: 'gauge', unit: '%', span: 6, promql: '(1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)) * 100' },
      { title: '磁盘使用率', chartType: 'line', unit: '%', span: 12, promql: '100 - (node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint="/"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint="/"} * 100)' },
      { title: 'CPU 使用率 Top', chartType: 'bar', unit: '%', span: 12, promql: 'topk(10, 100 - (avg by (instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100))' },
      { title: '内存使用率 Top', chartType: 'bar', unit: '%', span: 12, promql: 'topk(10, (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100)' },
      { title: '磁盘 IOPS（次/秒）', chartType: 'line', unit: '次/秒', span: 12, promql: 'topk(10, sum by (instance) (rate(node_disk_reads_completed_total[5m]) + rate(node_disk_writes_completed_total[5m])))' },
      { title: '系统负载 Top', chartType: 'bar', unit: '', span: 12, promql: 'topk(10, node_load1)' },
      { title: '网络接收速率', chartType: 'line', unit: 'B/s', span: 12, promql: 'sum(rate(node_network_receive_bytes_total{device!~"lo|veth.*|docker.*|br.*"}[5m]))' },
      { title: '网络发送速率', chartType: 'line', unit: 'B/s', span: 12, promql: 'sum(rate(node_network_transmit_bytes_total{device!~"lo|veth.*|docker.*|br.*"}[5m]))' },
      { title: 'CPU 使用趋势', chartType: 'line', unit: '%', span: 12, promql: '100 - (avg by (instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' },
      { title: '内存使用趋势', chartType: 'line', unit: '%', span: 12, promql: '(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100' },
      { title: '文件句柄使用率', chartType: 'gauge', unit: '%', span: 6, promql: 'sum(node_filefd_allocated) / sum(node_filefd_maximum) * 100' },
      { title: '运行进程数', chartType: 'stat', unit: '个', span: 6, promql: 'sum(node_procs_running)' },
      { title: '主机信息', chartType: 'table', unit: '', span: 12, promql: 'node_uname_info' }
    ]
  },
  {
    key: 'k8s',
    name: 'Kubernetes 大屏',
    description: '参考 K8S Dashboard 大屏风格，覆盖集群资源、容量、工作负载和网络指标。',
    panels: [
      { title: 'Pod 数量', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_pod_info)' },
      { title: 'Running Pod', chartType: 'stat', unit: '个', span: 8, promql: 'sum(kube_pod_status_phase{phase="Running"})' },
      { title: '异常 Pod', chartType: 'stat', unit: '个', span: 8, promql: 'sum(kube_pod_status_phase{phase=~"Failed|Unknown|Pending"})' },
      { title: 'Ready 节点', chartType: 'stat', unit: '个', span: 8, promql: 'sum(kube_node_status_condition{condition="Ready",status="true"})' },
      { title: '命名空间', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_namespace_created)' },
      { title: 'Deployment', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_deployment_created)' },
      { title: 'Service', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_service_info)' },
      { title: 'Ingress', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_ingress_info)' },
      { title: 'PVC', chartType: 'stat', unit: '个', span: 8, promql: 'count(kube_persistentvolumeclaim_info)' },
      { title: 'CPU Request 使用率', chartType: 'line', unit: '%', span: 12, promql: 'sum(kube_pod_container_resource_requests{resource="cpu"}) / sum(kube_node_status_allocatable{resource="cpu"}) * 100' },
      { title: '内存 Request 使用率', chartType: 'line', unit: '%', span: 12, promql: 'sum(kube_pod_container_resource_requests{resource="memory"}) / sum(kube_node_status_allocatable{resource="memory"}) * 100' },
      { title: '命名空间 Pod 分布', chartType: 'bar', unit: '个', span: 12, promql: 'sum by (namespace) (kube_pod_info)' },
      { title: '节点 Pod 分布', chartType: 'bar', unit: '个', span: 12, promql: 'sum by (node) (kube_pod_info)' },
      { title: '工作负载副本可用率', chartType: 'bar', unit: '%', span: 12, promql: 'sum by (deployment) (kube_deployment_status_replicas_available) / sum by (deployment) (kube_deployment_spec_replicas) * 100' },
      { title: '异常原因 Top', chartType: 'bar', unit: '个', span: 12, promql: 'sum by (reason) (kube_pod_container_status_waiting_reason)' },
      ...k8sPodPanelDefinitions
    ]
  }
]

const inspectionTemplates = [
  {
    key: 'blank',
    name: '空白巡检方案',
    description: '创建空白方案，按时间窗口配置 PromQL、统计方式和判定阈值。',
    panels: []
  },
  {
    key: 'host',
    name: '主机资源巡检',
    description: '检查所选时间范围内的可用性、资源高水位、IO、网络错误和进程阻塞。',
    panels: [
      { title: '主机离线检测', category: '可用性', unit: '台', promql: 'sum(up{job=~"node.*|node-exporter"} == 0)', reducer: 'max', warningValue: 0.5, criticalValue: 0.5, description: '时间范围内任一采样点存在离线主机即判定异常。' },
      { title: 'CPU 高负载', category: '计算资源', unit: '%', promql: '100 - (avg by (instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', reducer: 'p95', warningValue: 80, criticalValue: 90, description: '按主机计算 CPU 使用率，使用窗口 P95 避免瞬时毛刺。' },
      { title: '内存高使用率', category: '内存资源', unit: '%', promql: '(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100', reducer: 'p95', warningValue: 85, criticalValue: 95, description: '检查各主机在窗口内的内存使用率 P95。' },
      { title: '根分区磁盘使用率', category: '存储资源', unit: '%', promql: '100 - (node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint="/"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint="/"} * 100)', reducer: 'max', warningValue: 80, criticalValue: 90, description: '取所有主机根分区在时间窗口内的最大使用率。' },
      { title: '磁盘 IO 利用率', category: '存储资源', unit: '%', promql: 'rate(node_disk_io_time_seconds_total[5m]) * 100', reducer: 'p95', warningValue: 70, criticalValue: 90, description: '检查磁盘繁忙程度的窗口 P95。' },
      { title: '文件句柄使用率', category: '系统资源', unit: '%', promql: 'node_filefd_allocated / node_filefd_maximum * 100', reducer: 'max', warningValue: 70, criticalValue: 85, description: '检查文件句柄占用高水位。' },
      { title: '网络接收错误增量', category: '稳定性', unit: '次', promql: 'node_network_receive_errs_total{device!="lo"}', reducer: 'increase', warningValue: 1, criticalValue: 10, description: '统计窗口内网卡接收错误计数的增量。' },
      { title: '网络发送错误增量', category: '稳定性', unit: '次', promql: 'node_network_transmit_errs_total{device!="lo"}', reducer: 'increase', warningValue: 1, criticalValue: 10, description: '统计窗口内网卡发送错误计数的增量。' },
      { title: '阻塞进程', category: '系统资源', unit: '个', promql: 'node_procs_blocked', reducer: 'max', warningValue: 1, criticalValue: 5, description: '检查时间窗口内阻塞进程数峰值。' }
    ]
  },
  {
    key: 'k8s',
    name: 'Kubernetes 巡检',
    description: '检查所选时间范围内的节点、Pod、工作负载、存储和容器资源异常。',
    panels: [
      { title: 'NotReady 节点', category: '可用性', unit: '个', promql: 'sum(kube_node_status_condition{condition="Ready",status="true"} == 0)', reducer: 'max', warningValue: 0.5, criticalValue: 0.5, description: '检查窗口内是否出现过 NotReady 节点。' },
      { title: 'Pending Pod', category: '工作负载', unit: '个', promql: 'sum(kube_pod_status_phase{phase="Pending"} == 1)', reducer: 'max', warningValue: 1, criticalValue: 5, description: '检查 Pending Pod 数量峰值。' },
      { title: '失败或未知 Pod', category: '工作负载', unit: '个', promql: 'sum(kube_pod_status_phase{phase=~"Failed|Unknown"} == 1)', reducer: 'max', warningValue: 1, criticalValue: 3, description: '检查 Failed 或 Unknown Pod 数量峰值。' },
      { title: 'Pod 重启增量', category: '稳定性', unit: '次', promql: 'sum by(namespace, pod) (kube_pod_container_status_restarts_total)', reducer: 'increase', warningValue: 1, criticalValue: 5, description: '统计窗口内各 Pod 容器重启次数增量。' },
      { title: 'CrashLoopBackOff 容器', category: '稳定性', unit: '个', promql: 'sum(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"} == 1)', reducer: 'max', warningValue: 0.5, criticalValue: 0.5, description: '检查窗口内是否出现 CrashLoopBackOff。' },
      { title: 'OOMKilled 容器', category: '稳定性', unit: '个', promql: 'sum(kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1)', reducer: 'max', warningValue: 0.5, criticalValue: 0.5, description: '检查窗口内是否出现 OOMKilled。' },
      { title: 'Deployment 不可用副本', category: '工作负载', unit: '个', promql: 'sum(kube_deployment_status_replicas_unavailable)', reducer: 'max', warningValue: 1, criticalValue: 3, description: '检查 Deployment 不可用副本峰值。' },
      { title: 'PVC Pending', category: '存储资源', unit: '个', promql: 'sum(kube_persistentvolumeclaim_status_phase{phase="Pending"} == 1)', reducer: 'max', warningValue: 0.5, criticalValue: 0.5, description: '检查窗口内是否存在 Pending PVC。' },
      { title: 'CPU 节流率', category: '计算资源', unit: '%', promql: 'sum by(namespace, pod) (rate(container_cpu_cfs_throttled_periods_total{container!="",pod!=""}[5m])) / sum by(namespace, pod) (rate(container_cpu_cfs_periods_total{container!="",pod!=""}[5m])) * 100', reducer: 'p95', warningValue: 10, criticalValue: 25, description: '按 Pod 检查 CPU 节流率窗口 P95。' },
      { title: '容器内存使用率', category: '内存资源', unit: '%', promql: 'sum by(namespace, pod) (container_memory_working_set_bytes{container!="",pod!=""}) / sum by(namespace, pod) (kube_pod_container_resource_limits{resource="memory"} > 0) * 100', reducer: 'p95', warningValue: 80, criticalValue: 95, description: '按 Pod 检查有内存限制容器的窗口 P95。' }
    ]
  }
]

const lineColors = ['#3b82f6', '#14b8a6', '#f59e0b', '#8b5cf6', '#ef4444', '#06b6d4', '#84cc16', '#ec4899']

export { retiredK8sPanelTitles, hostAggregateTrendModes, k8sPodPanelDefinitions, k8sTrendPanelTitles, k8sPanelTitleMap, dashboardTemplates, inspectionTemplates, lineColors }
