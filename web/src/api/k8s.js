import http from './http'
import { getToken } from '../utils/auth'

export const queryK8sClusterList = () => http.get('/api/v1/k8s/cluster/list')

export const queryK8sClusterInfo = (id) => http.get('/api/v1/k8s/cluster/info', { params: { id } })

export const addK8sCluster = (data) => http.post('/api/v1/k8s/cluster/add', data)

export const updateK8sCluster = (data) => http.put('/api/v1/k8s/cluster/update', data)

export const deleteK8sCluster = (id) => http.delete('/api/v1/k8s/cluster/delete', { data: { id } })

export const queryK8sClusterOverview = (clusterId) =>
  http.get('/api/v1/k8s/cluster/detail', { params: { clusterId } })

export const queryK8sNodeDetail = (clusterId, nodeName) =>
  http.get('/api/v1/k8s/node/detail', { params: { clusterId, nodeName } })

export const queryK8sNodePods = (clusterId, nodeName) =>
  http.get('/api/v1/k8s/node/pods', { params: { clusterId, nodeName } })

export const updateK8sNodeLabels = (data) => http.put('/api/v1/k8s/node/labels', data)

export const queryK8sNamespaceDetail = (clusterId, namespace) =>
  http.get('/api/v1/k8s/namespace/detail', { params: { clusterId, namespace } })

export const queryK8sNamespaceEvents = (clusterId, namespace) =>
  http.get('/api/v1/k8s/namespace/events', { params: { clusterId, namespace } })

export const queryK8sServiceDetail = (clusterId, namespace, serviceName) =>
  http.get('/api/v1/k8s/service/detail', { params: { clusterId, namespace, serviceName } })

export const updateK8sService = (data) => http.put('/api/v1/k8s/service/update', data)

export const queryK8sIngressDetail = (clusterId, namespace, ingressName) =>
  http.get('/api/v1/k8s/ingress/detail', { params: { clusterId, namespace, ingressName } })

export const queryK8sIstioResourceDetail = (clusterId, resourceType, namespace, name) =>
  http.get('/api/v1/k8s/istio/detail', { params: { clusterId, resourceType, namespace, name } })

export const queryK8sConfigMapDetail = (clusterId, namespace, configMapName) =>
  http.get('/api/v1/k8s/configmap/detail', { params: { clusterId, namespace, configMapName } })

export const queryK8sSecretDetail = (clusterId, namespace, secretName) =>
  http.get('/api/v1/k8s/secret/detail', { params: { clusterId, namespace, secretName } })

export const queryK8sStorageDetail = (clusterId, kind, namespace, name) =>
  http.get('/api/v1/k8s/storage/detail', { params: { clusterId, kind, namespace, name } })

export const queryK8sPodDetail = (clusterId, namespace, podName) =>
  http.get('/api/v1/k8s/pod/detail', { params: { clusterId, namespace, podName } })

export const queryK8sPodMetrics = (clusterId, namespace, podName, range = '1h') =>
  http.get('/api/v1/k8s/pod/metrics', { params: { clusterId, namespace, podName, range } })

export const queryK8sWorkloadMetrics = (clusterId, namespace, workloadType, workloadName, range = '1h') =>
  http.get('/api/v1/k8s/workload/metrics', { params: { clusterId, namespace, workloadType, workloadName, range } })

export const queryK8sPodContainers = (clusterId, namespace, podName) =>
  http.get('/api/v1/k8s/pod/containers', { params: { clusterId, namespace, podName } })

export const queryK8sPodLogs = (clusterId, namespace, podName, container = '', tailLines = 200) =>
  http.get('/api/v1/k8s/pod/logs', { params: { clusterId, namespace, podName, container, tailLines } })

export const queryK8sPodEvents = (clusterId, namespace, podName) =>
  http.get('/api/v1/k8s/pod/events', { params: { clusterId, namespace, podName } })

export const queryK8sWorkloadDetail = (clusterId, namespace, workloadType, workloadName) =>
  http.get('/api/v1/k8s/workload/detail', { params: { clusterId, namespace, workloadType, workloadName } })

export const scaleK8sWorkload = (data) => http.post('/api/v1/k8s/workload/scale', data)

export const restartK8sWorkload = (data) => http.post('/api/v1/k8s/workload/restart', data)

export const updateK8sWorkloadImages = (data) => http.post('/api/v1/k8s/workload/images', data)

export const updateK8sWorkloadResources = (data) => http.put('/api/v1/k8s/workload/resources', data)

export const createK8sWorkloadBundle = (data) => http.post('/api/v1/k8s/workload/create', data)

export const updateK8sIstioTraffic = (data) => http.post('/api/v1/k8s/istio/traffic', data)

export const updateK8sHTTPRouteTraffic = (data) => http.post('/api/v1/k8s/httproute/traffic', data)

export const createK8sResourceYAML = (data) => http.post('/api/v1/k8s/resource/yaml/create', data)

export const updateK8sResourceYAML = (data) => http.put('/api/v1/k8s/resource/yaml', data)

export const deleteK8sResource = (data) => http.delete('/api/v1/k8s/resource/delete', { data })

export const buildK8sPodTerminalWSUrl = ({
  clusterId,
  namespace,
  podName,
  container = '',
  command = '/bin/sh',
  rows = 32,
  cols = 120
}) => {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const params = new URLSearchParams({
    clusterId: String(clusterId),
    namespace,
    podName,
    token: getToken() || '',
    command,
    rows: String(rows),
    cols: String(cols)
  })
  if (container) {
    params.set('container', container)
  }
  return `${protocol}://${window.location.host}/api/v1/k8s/pod/terminal/ws?${params.toString()}`
}
