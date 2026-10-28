package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"ops-admin/backend/model"
)

type K8sNodeAddPayload struct {
	model.K8sNodeAddConfig
	PrecheckID     uint   `json:"precheckId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func nodeAddMetadata(text string, taints bool) (map[string]string, []corev1.Taint, error) {
	labels := map[string]string{}
	result := []corev1.Taint{}
	seen := map[string]bool{}
	if len(text) > 8192 {
		return nil, nil, errors.New("节点标签或污点内容过大")
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		effect := ""
		if taints {
			i := strings.LastIndex(line, ":")
			if i < 1 {
				return nil, nil, errors.New("污点格式应为 key=value:NoSchedule")
			}
			effect, line = line[i+1:], line[:i]
			if effect != "NoSchedule" && effect != "PreferNoSchedule" && effect != "NoExecute" {
				return nil, nil, errors.New("节点污点 effect 无效")
			}
		}
		key, value, _ := strings.Cut(line, "=")
		if len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(value)) > 0 || strings.HasPrefix(key, "node-role.kubernetes.io/") || strings.HasPrefix(key, "node-restriction.kubernetes.io/") || strings.HasPrefix(key, "kubernetes.io/") || strings.HasPrefix(key, "k8s.io/") {
			return nil, nil, errors.New("标签或污点格式无效，不能设置系统保留标签")
		}
		identity := key + ":" + effect
		if seen[identity] {
			return nil, nil, errors.New("标签或污点重复")
		}
		seen[identity] = true
		if taints {
			result = append(result, corev1.Taint{Key: key, Value: value, Effect: corev1.TaintEffect(effect)})
		} else {
			labels[key] = value
		}
	}
	return labels, result, nil
}

func validateNodeAdd(c model.K8sNodeAddConfig) error {
	if c.ClusterID == 0 || len(c.Nodes) < 1 || len(c.Nodes) > 50 {
		return errors.New("请选择集群及 1–50 台 CMDB 服务器")
	}
	names, hosts := map[string]bool{}, map[uint]bool{}
	for _, n := range c.Nodes {
		if n.HostID == 0 || hosts[n.HostID] || names[n.NodeName] || len(n.NodeName) > 63 || len(validation.IsDNS1123Subdomain(n.NodeName)) > 0 {
			return errors.New("节点名称应为唯一的小写 DNS 名称（不超过 63 字符），服务器不能重复")
		}
		hosts[n.HostID] = true
		names[n.NodeName] = true
	}
	if _, _, e := nodeAddMetadata(c.Labels, false); e != nil {
		return e
	}
	_, _, e := nodeAddMetadata(c.Taints, true)
	return e
}

func nodeAddInputHash(c model.K8sNodeAddConfig) string {
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) K8sNodeAddContext(id uint) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	var cluster model.K8sCluster
	if err := s.db.Select("id", "name", "version", "node_count").First(&cluster, id).Error; err != nil {
		return nil, err
	}
	envs, err := s.ListOpsEnvironments("", "")
	if err != nil {
		return nil, err
	}
	groups, err := s.ListAssetHostGroups("")
	if err != nil {
		return nil, err
	}
	result := map[string]any{"mode": "real", "name": cluster.Name, "version": cluster.Version, "nodeCount": cluster.NodeCount, "environments": envs, "groups": groups.List, "canAdd": false, "nodeListIsSimulated": false, "runtime": "containerd", "network": "待核验", "podCIDR": "待核验", "serviceCIDR": "待核验"}
	snap, err := s.nodeAddClusterSnapshot(id)
	if err != nil {
		result["blockedReason"] = err.Error()
		return result, nil
	}
	defer snap.Cleanup()
	result["canAdd"], result["version"], result["nodeCount"] = true, "v"+snap.Config.Version, len(snap.Nodes)
	result["network"], result["podCIDR"], result["serviceCIDR"] = snap.Network, snap.Config.PodCIDR, snap.Config.ServiceCIDR
	if snap.Config.PodCIDR == "" {
		result["podCIDR"] = "由现有 CNI 管理（未记录 CIDR）"
	}
	result["endpoint"] = snap.Endpoint
	return result, nil
}

func nodeAddHostUnavailable(h map[string]any, nodes []corev1.Node) string {
	if v, _ := h["disabledReason"].(string); v != "" {
		return v
	}
	if id, _ := h["occupiedPlanId"].(uint); id > 0 {
		return fmt.Sprintf("被部署 #%d 占用", id)
	}
	for _, n := range nodes {
		for _, addr := range n.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP && (addr.Address == h["privateIp"] || addr.Address == h["publicIp"] || addr.Address == h["sshIp"]) {
				return "已属于目标集群"
			}
		}
	}
	return ""
}

func (s *Service) K8sNodeAddHosts(id uint, keyword, environment string, group uint, page, size int) (map[string]any, error) {
	snap, err := s.nodeAddClusterSnapshot(id)
	if err != nil {
		return nil, err
	}
	defer snap.Cleanup()
	result, err := s.K8sDeployHostOptions(keyword, environment, group, page, size)
	if err != nil {
		return nil, err
	}
	for _, h := range result["list"].([]map[string]any) {
		h["disabledReason"] = nodeAddHostUnavailable(h, snap.Nodes)
	}
	return result, nil
}

func nodeAddPublicHost(h model.AssetHost) map[string]any {
	return map[string]any{"id": h.ID, "hostName": h.HostName, "privateIp": h.PrivateIP, "publicIp": h.PublicIP, "sshIp": h.SSHIP, "os": h.OS, "arch": h.Arch, "cpu": h.CPU, "memory": h.Memory, "connectionMode": h.ConnectionMode, "gatewayName": h.Gateway.Name, "credentialName": h.Credential.Name}
}

func (s *Service) createK8sNodeAdd(p K8sNodeAddPayload, kind, operator string) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	if err := validateNodeAdd(p.K8sNodeAddConfig); err != nil {
		return nil, err
	}
	inputHash := nodeAddInputHash(p.K8sNodeAddConfig)
	// Replay before probing the already-joined hosts. The key is global and bound to the exact input.
	if kind == "add" {
		if len(p.IdempotencyKey) < 16 || len(p.IdempotencyKey) > 128 {
			return nil, errors.New("缺少有效的请求幂等键")
		}
		var old model.K8sNodeAddTask
		e := s.db.Where("idempotency_key = ?", p.IdempotencyKey).First(&old).Error
		if e == nil {
			if old.InputHash != inputHash {
				return nil, errors.New("此请求键已用于不同配置")
			}
			return s.K8sNodeAddTask(old.ClusterID, old.ID)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, e
		}
	}
	snap, err := s.nodeAddClusterSnapshot(p.ClusterID)
	if err != nil {
		return nil, err
	}
	defer snap.Cleanup()
	nodes := make([]model.K8sDeployNode, 0, len(p.Nodes))
	for _, selection := range p.Nodes {
		nodes = append(nodes, model.K8sDeployNode{HostID: selection.HostID, Name: selection.NodeName, Role: "worker"})
	}
	hosts, fingerprint, err := s.deployHosts(nodes)
	if err != nil {
		return nil, err
	}
	hostDTOs := []map[string]any{}
	for i := range nodes {
		for _, h := range hosts {
			if h.ID == nodes[i].HostID {
				ip := h.PrivateIP
				if ip == "" {
					ip = h.SSHIP
				}
				if ip == "" {
					ip = h.PublicIP
				}
				addr, e := netip.ParseAddr(ip)
				if e != nil || !addr.Is4() || !addr.IsGlobalUnicast() {
					return nil, fmt.Errorf("主机 %s 缺少有效 IPv4 部署地址", h.HostName)
				}
				pod, _ := netip.ParsePrefix(snap.Config.PodCIDR)
				svc, _ := netip.ParsePrefix(snap.Config.ServiceCIDR)
				if pod.Contains(addr) || svc.Contains(addr) {
					return nil, errors.New("服务器 IP 与集群 Pod/Service 网段冲突")
				}
				nodes[i].IP, nodes[i].HostName = ip, h.HostName
				public := nodeAddPublicHost(h)
				hostDTOs = append(hostDTOs, public)
				if reason := nodeAddHostUnavailable(public, snap.Nodes); reason != "" {
					return nil, fmt.Errorf("%s：%s", h.HostName, reason)
				}
			}
		}
		for _, existing := range snap.Nodes {
			if existing.Name == nodes[i].Name {
				return nil, fmt.Errorf("节点名 %s 已存在", existing.Name)
			}
		}
	}
	var run model.K8sDeployRun
	var task model.K8sNodeAddTask
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var cluster model.K8sCluster
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&cluster, p.ClusterID).Error; e != nil {
			return e
		}
		var active int64
		if e := tx.Table("k8s_node_add_task t").Joins("JOIN k8s_deploy_run r ON r.id=t.run_id").Where("t.cluster_id = ? AND r.status = ?", p.ClusterID, deployActive).Count(&active).Error; e != nil {
			return e
		}
		if active > 0 {
			return errors.New("目标集群已有节点添加或预检任务运行中，请从添加任务查看")
		}
		if kind == "add" {
			var check model.K8sNodeAddTask
			var checked model.K8sDeployRun
			if e := tx.Where("id = ? AND cluster_id = ? AND kind = ?", p.PrecheckID, p.ClusterID, "precheck").First(&check).Error; e != nil {
				return errors.New("请先完成当前配置的预检")
			}
			if e := tx.First(&checked, check.RunID).Error; e != nil {
				return e
			}
			if checked.Status != "success" || checked.FinishedAt == nil || time.Since(*checked.FinishedAt) > 5*time.Minute || check.InputHash != inputHash || check.ClusterHash != snap.Hash || checked.Fingerprint != fingerprint {
				return errors.New("预检未通过、已过期或集群/CMDB/配置已变化，请重新预检")
			}
			nodes = checked.Nodes
		}
		run = model.K8sDeployRun{Kind: "node_" + kind, Status: deployActive, Operator: operator, Config: snap.Config, Nodes: nodes, Fingerprint: fingerprint, StartedAt: time.Now(), HeartbeatAt: time.Now()}
		if e := tx.Create(&run).Error; e != nil {
			return e
		}
		task = model.K8sNodeAddTask{ClusterID: p.ClusterID, RunID: run.ID, Kind: kind, Config: p.K8sNodeAddConfig, Hosts: hostDTOs, InputHash: inputHash, ClusterHash: snap.Hash}
		if kind == "add" {
			task.PrecheckID = &p.PrecheckID
			task.IdempotencyKey = &p.IdempotencyKey
		}
		if e := tx.Create(&task).Error; e != nil {
			return errors.New("重复请求或此预检已用于添加，请刷新任务记录")
		}
		for _, h := range hosts {
			if e := tx.Create(&model.K8sDeployHostLock{HostID: h.ID, RunID: run.ID}).Error; e != nil {
				return fmt.Errorf("主机 %s 已被其他任务占用或存在待核对的安装现场", h.HostName)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	go s.processK8sNodeAdd(task, run, hosts)
	return s.K8sNodeAddTask(p.ClusterID, task.ID)
}

func (s *Service) StartK8sNodeAddPrecheck(p K8sNodeAddPayload, operator string) (map[string]any, error) {
	return s.createK8sNodeAdd(p, "precheck", operator)
}
func (s *Service) StartK8sNodeAdd(p K8sNodeAddPayload, operator string) (map[string]any, error) {
	return s.createK8sNodeAdd(p, "add", operator)
}

func (s *Service) K8sNodeAddTasks(id uint) ([]map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	var tasks []model.K8sNodeAddTask
	if e := s.db.Where("cluster_id = ?", id).Order("id DESC").Limit(50).Find(&tasks).Error; e != nil {
		return nil, e
	}
	result := []map[string]any{}
	for _, t := range tasks {
		detail, e := s.nodeAddTaskDetail(id, t.ID, false)
		if e != nil {
			return nil, e
		}
		result = append(result, detail)
	}
	return result, nil
}

func (s *Service) K8sNodeAddTask(clusterID, id uint) (map[string]any, error) {
	return s.nodeAddTaskDetail(clusterID, id, true)
}

func (s *Service) nodeAddTaskDetail(clusterID, id uint, includeLogs bool) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	var task model.K8sNodeAddTask
	var run model.K8sDeployRun
	if e := s.db.Where("id = ? AND cluster_id = ?", id, clusterID).First(&task).Error; e != nil {
		return nil, e
	}
	if e := s.db.First(&run, task.RunID).Error; e != nil {
		return nil, e
	}
	var cluster model.K8sCluster
	if e := s.db.Select("id", "name").First(&cluster, clusterID).Error; e != nil {
		return nil, e
	}
	steps := []model.K8sDeployStep{}
	if e := s.db.Where("run_id = ?", run.ID).Order("id").Find(&steps).Error; e != nil {
		return nil, e
	}
	logs := []model.K8sDeployLog{}
	if includeLogs {
		if e := s.db.Where("run_id = ?", run.ID).Order("id DESC").Limit(200).Find(&logs).Error; e != nil {
			return nil, e
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })
	return nodeAddTaskView(task, run, cluster.Name, steps, logs), nil
}

func (s *Service) StopK8sNodeAdd(clusterID, id uint) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	var task model.K8sNodeAddTask
	if e := s.db.Where("id = ? AND cluster_id = ? AND kind = ?", id, clusterID, "add").First(&task).Error; e != nil {
		return nil, e
	}
	if e := s.db.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", task.RunID, deployActive).Update("stop_requested", true).Error; e != nil {
		return nil, e
	}
	return s.K8sNodeAddTask(clusterID, id)
}

func nodeAddTaskView(task model.K8sNodeAddTask, run model.K8sDeployRun, name string, steps []model.K8sDeployStep, records []model.K8sDeployLog) map[string]any {
	nodes := []map[string]any{}
	stage := 0
	for _, n := range run.Nodes {
		nodeStage, status, message, mutated, ready := 0, "pending", "等待执行", false, false
		checks := []map[string]any{}
		for _, step := range steps {
			if step.HostID != n.HostID {
				continue
			}
			current := nodeAddStepStage(step.Title)
			if current > nodeStage {
				nodeStage = current
			}
			if current >= 2 {
				mutated = true
			}
			status, message = step.Status, step.Summary
			if status == deployActive {
				message = step.Title
			}
			checks = append(checks, map[string]any{"title": step.Title, "status": step.Status, "message": message})
			if step.Title == "等待 Ready" && step.Status == "success" {
				ready = true
			}
		}
		if task.Kind == "precheck" {
			if status == "warning" {
				status = "success"
			}
			if status == "success" && run.Status == deployActive {
				status = "running"
			}
		} else if nodeStage < 5 && status == "success" {
			status = "running"
		}
		if run.Status != deployActive && (status == "running" || status == "pending") {
			status = "failed"
			message = run.Error
			if run.Status == "stopped" {
				status = "cancelled"
			}
		}
		if run.Status == "needs_review" && mutated && nodeStage < 5 {
			status = "needs_review"
			message = "执行中断或状态不确定，请核对主机现场"
		}
		if task.Kind == "add" && ready {
			nodeStage, status, message = 5, "success", "节点身份、版本和 Ready 验证通过"
		}
		if nodeStage > stage {
			stage = nodeStage
		}
		var host map[string]any
		for _, h := range task.Hosts {
			if fmt.Sprint(h["id"]) == fmt.Sprint(n.HostID) {
				host = h
				break
			}
		}
		nodes = append(nodes, map[string]any{"hostId": n.HostID, "nodeName": n.Name, "host": host, "os": n.OS, "arch": n.Arch, "interface": n.Interface, "stage": nodeStage, "status": status, "message": message, "checks": checks, "passed": status == "success", "retryable": !mutated && (status == "failed" || status == "cancelled")})
	}
	lines := []string{}
	logBytes := 0
	for _, l := range records {
		for _, line := range strings.Split(strings.TrimSpace(l.Content), "\n") {
			if line != "" {
				if len(line) > 4096 {
					line = line[:4096] + "…"
				}
				lines = append(lines, line)
				logBytes += len(line)
				for logBytes > 128*1024 && len(lines) > 1 {
					logBytes -= len(lines[0])
					lines = lines[1:]
				}
			}
		}
	}
	status := run.Status
	if status == "stopped" {
		status = "cancelled"
	}
	result := map[string]any{"id": task.ID, "clusterId": task.ClusterID, "clusterName": name, "mode": "real", "kind": task.Kind, "status": status, "stage": stage, "nodes": nodes, "config": task.Config, "logs": lines, "createdAt": task.CreatedAt, "stopRequested": run.StopRequested, "error": run.Error, "passed": task.Kind == "precheck" && run.Status == "success"}
	if run.FinishedAt != nil {
		result["expiresAt"] = run.FinishedAt.Add(5 * time.Minute)
	}
	return result
}

func nodeAddStepStage(title string) int {
	return map[string]int{"节点预检": 1, "网络检查": 1, "执行前核验": 1, "安装组件": 2, "加入集群": 3, "等待 Ready": 4, "完成": 5}[title]
}
