package controller

import (
	"net/http"
	"strings"

	"ops-admin/backend/auth"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetK8sClusterList(c *gin.Context) {
	data, err := ctl.service.ListK8sClusters()
	if err != nil {
		httpx.Failed(c, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sClusterInfo(c *gin.Context) {
	var query struct {
		ID uint `form:"id"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ID == 0 {
		httpx.Failed(c, http.StatusBadRequest, "invalid cluster id")
		return
	}
	data, err := ctl.service.GetK8sCluster(query.ID)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateK8sCluster(c *gin.Context) {
	var payload model.K8sClusterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid k8s cluster payload")
		return
	}
	payload.Operator = c.GetString("username")
	data, err := ctl.service.CreateK8sCluster(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sCluster(c *gin.Context) {
	var payload model.K8sClusterPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.ID == 0 {
		httpx.Failed(c, http.StatusBadRequest, "invalid k8s cluster payload")
		return
	}
	payload.Operator = c.GetString("username")
	data, err := ctl.service.UpdateK8sCluster(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteK8sCluster(c *gin.Context) {
	var payload struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || payload.ID == 0 {
		httpx.Failed(c, http.StatusBadRequest, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteK8sCluster(payload.ID); err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetK8sClusterDetail(c *gin.Context) {
	var query struct {
		ClusterID uint `form:"clusterId"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 {
		httpx.Failed(c, http.StatusBadRequest, "invalid cluster id")
		return
	}
	data, err := ctl.service.GetK8sClusterDetail(query.ClusterID)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sNodeDetail(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		NodeName  string `form:"nodeName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.NodeName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid node query")
		return
	}
	data, err := ctl.service.GetK8sNodeDetail(query.ClusterID, query.NodeName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sNodePods(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		NodeName  string `form:"nodeName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.NodeName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid node query")
		return
	}
	data, err := ctl.service.GetK8sNodePods(query.ClusterID, query.NodeName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sNodeLabels(c *gin.Context) {
	var payload model.K8sNodeLabelsPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.ClusterID == 0 || strings.TrimSpace(payload.NodeName) == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid node labels payload")
		return
	}
	if err := ctl.service.UpdateK8sNodeLabels(payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetK8sPodDetail(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod query")
		return
	}
	data, err := ctl.service.GetK8sPodDetail(query.ClusterID, query.Namespace, query.PodName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sPodMetrics(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
		Range     string `form:"range"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod metrics query")
		return
	}
	data, err := ctl.service.GetK8sPodMetrics(query.ClusterID, query.Namespace, query.PodName, query.Range)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sWorkloadMetrics(c *gin.Context) {
	var query struct {
		ClusterID    uint   `form:"clusterId"`
		Namespace    string `form:"namespace"`
		WorkloadType string `form:"workloadType"`
		WorkloadName string `form:"workloadName"`
		Range        string `form:"range"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.WorkloadType == "" || query.WorkloadName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload metrics query")
		return
	}
	data, err := ctl.service.GetK8sWorkloadMetrics(query.ClusterID, query.Namespace, query.WorkloadType, query.WorkloadName, query.Range)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sPodLogs(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
		Container string `form:"container"`
		TailLines int    `form:"tailLines"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod query")
		return
	}
	data, err := ctl.service.GetK8sPodLogs(query.ClusterID, query.Namespace, query.PodName, query.Container, query.TailLines)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sPodEvents(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod query")
		return
	}
	data, err := ctl.service.GetK8sPodEvents(query.ClusterID, query.Namespace, query.PodName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sPodContainers(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod query")
		return
	}
	data, err := ctl.service.GetK8sPodContainers(query.ClusterID, query.Namespace, query.PodName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) K8sPodTerminalWS(c *gin.Context) {
	token := c.Query("token")
	if strings.TrimSpace(token) == "" {
		httpx.Failed(c, http.StatusUnauthorized, "请先登录")
		return
	}
	if _, err := auth.ParseToken(token); err != nil {
		httpx.Failed(c, http.StatusUnauthorized, auth.TokenErrorMessage(err))
		return
	}

	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
		PodName   string `form:"podName"`
		Container string `form:"container"`
		Command   string `form:"command"`
		Rows      int    `form:"rows"`
		Cols      int    `form:"cols"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.PodName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid pod terminal query")
		return
	}
	if query.Rows <= 0 {
		query.Rows = 32
	}
	if query.Cols <= 0 {
		query.Cols = 120
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := ctl.service.OpenK8sPodTerminal(
		query.ClusterID,
		query.Namespace,
		query.PodName,
		query.Container,
		query.Command,
		query.Rows,
		query.Cols,
		conn,
	); err != nil {
		_ = conn.WriteJSON(map[string]any{
			"operation": "stdout",
			"data":      "\r\n" + err.Error() + "\r\n",
		})
	}
}

func (ctl *Controller) GetK8sNamespaceDetail(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid namespace query")
		return
	}
	data, err := ctl.service.GetK8sNamespaceDetail(query.ClusterID, query.Namespace)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sNamespaceEvents(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Namespace string `form:"namespace"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid namespace query")
		return
	}
	data, err := ctl.service.GetK8sNamespaceEvents(query.ClusterID, query.Namespace)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sWorkloadDetail(c *gin.Context) {
	var query struct {
		ClusterID    uint   `form:"clusterId"`
		Namespace    string `form:"namespace"`
		WorkloadType string `form:"workloadType"`
		WorkloadName string `form:"workloadName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.WorkloadType == "" || query.WorkloadName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload query")
		return
	}
	data, err := ctl.service.GetK8sWorkloadDetail(query.ClusterID, query.Namespace, query.WorkloadType, query.WorkloadName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ScaleK8sWorkload(c *gin.Context) {
	var payload model.K8sWorkloadActionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload payload")
		return
	}
	data, err := ctl.service.ScaleK8sWorkload(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) RestartK8sWorkload(c *gin.Context) {
	var payload model.K8sWorkloadActionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload payload")
		return
	}
	data, err := ctl.service.RestartK8sWorkload(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sResourceYAML(c *gin.Context) {
	var payload model.K8sResourceYAMLPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid yaml payload")
		return
	}
	data, err := ctl.service.UpdateK8sResourceYAML(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateK8sResourceYAML(c *gin.Context) {
	var payload model.K8sResourceYAMLPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid yaml payload")
		return
	}
	data, err := ctl.service.CreateK8sResourceYAML(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateK8sWorkloadBundle(c *gin.Context) {
	var payload model.K8sWorkloadBundlePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload payload")
		return
	}
	data, err := ctl.service.CreateK8sWorkloadBundle(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteK8sResource(c *gin.Context) {
	var payload model.K8sResourceDeletePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid delete payload")
		return
	}
	data, err := ctl.service.DeleteK8sResource(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sIstioTraffic(c *gin.Context) {
	var payload model.K8sIstioTrafficPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid istio traffic payload")
		return
	}
	data, err := ctl.service.UpdateK8sIstioTraffic(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sHTTPRouteTraffic(c *gin.Context) {
	var payload model.K8sIstioTrafficPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid http route traffic payload")
		return
	}
	data, err := ctl.service.UpdateK8sHTTPRouteTraffic(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sWorkloadImages(c *gin.Context) {
	var payload model.K8sWorkloadImageBatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload image payload")
		return
	}
	data, err := ctl.service.UpdateK8sWorkloadImages(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sWorkloadResources(c *gin.Context) {
	var payload model.K8sWorkloadResourcesPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid workload resource payload")
		return
	}
	data, err := ctl.service.UpdateK8sWorkloadResources(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sServiceDetail(c *gin.Context) {
	var query struct {
		ClusterID   uint   `form:"clusterId"`
		Namespace   string `form:"namespace"`
		ServiceName string `form:"serviceName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.ServiceName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid service query")
		return
	}
	data, err := ctl.service.GetK8sServiceDetail(query.ClusterID, query.Namespace, query.ServiceName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateK8sService(c *gin.Context) {
	var payload model.K8sServiceUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid service payload")
		return
	}
	data, err := ctl.service.UpdateK8sService(payload)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sIngressDetail(c *gin.Context) {
	var query struct {
		ClusterID   uint   `form:"clusterId"`
		Namespace   string `form:"namespace"`
		IngressName string `form:"ingressName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.IngressName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid ingress query")
		return
	}
	data, err := ctl.service.GetK8sIngressDetail(query.ClusterID, query.Namespace, query.IngressName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sIstioResourceDetail(c *gin.Context) {
	var query struct {
		ClusterID    uint   `form:"clusterId"`
		ResourceType string `form:"resourceType"`
		Namespace    string `form:"namespace"`
		Name         string `form:"name"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.ResourceType == "" || query.Namespace == "" || query.Name == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid istio resource query")
		return
	}
	data, err := ctl.service.GetK8sIstioResourceDetail(query.ClusterID, query.ResourceType, query.Namespace, query.Name)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sConfigMapDetail(c *gin.Context) {
	var query struct {
		ClusterID     uint   `form:"clusterId"`
		Namespace     string `form:"namespace"`
		ConfigMapName string `form:"configMapName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.ConfigMapName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid configmap query")
		return
	}
	data, err := ctl.service.GetK8sConfigMapDetail(query.ClusterID, query.Namespace, query.ConfigMapName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sSecretDetail(c *gin.Context) {
	var query struct {
		ClusterID  uint   `form:"clusterId"`
		Namespace  string `form:"namespace"`
		SecretName string `form:"secretName"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Namespace == "" || query.SecretName == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid secret query")
		return
	}
	data, err := ctl.service.GetK8sSecretDetail(query.ClusterID, query.Namespace, query.SecretName)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetK8sStorageDetail(c *gin.Context) {
	var query struct {
		ClusterID uint   `form:"clusterId"`
		Kind      string `form:"kind"`
		Namespace string `form:"namespace"`
		Name      string `form:"name"`
	}
	if err := c.ShouldBindQuery(&query); err != nil || query.ClusterID == 0 || query.Kind == "" || query.Name == "" {
		httpx.Failed(c, http.StatusBadRequest, "invalid storage query")
		return
	}
	data, err := ctl.service.GetK8sStorageDetail(query.ClusterID, query.Kind, query.Namespace, query.Name)
	if err != nil {
		httpx.Failed(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.Success(c, data)
}
