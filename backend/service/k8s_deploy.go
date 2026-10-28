package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ops-admin/backend/model"
)

const deployActive = "running"

var deployNodeNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?(\.[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)*$`)

type K8sDeployPayload struct {
	ID          uint                  `json:"id"`
	Revision    int                   `json:"revision"`
	Name        string                `json:"name"`
	Env         string                `json:"env"`
	Description string                `json:"description"`
	Config      model.K8sDeployConfig `json:"config"`
	Nodes       []model.K8sDeployNode `json:"nodes"`
}

func (s *Service) requireDeployAdmin() error {
	if p := s.resourcePolicy(); p != nil && !p.Super {
		return errors.New("K8s 部署仅限管理员操作")
	}
	return nil
}

func (s *Service) K8sDeployCapabilities() map[string]any {
	return map[string]any{
		"versions":     []string{"1.34.9", "1.33.5", "1.32.9", "1.31.13"},
		"versionRange": "1.31.x–1.34.x",
		"systems":      "Ubuntu 22.04 / 24.04、Debian 12 / 13，x86_64 / aarch64；部署前自动检测",
		"plugins":      []map[string]any{{"value": "calico", "label": "Calico", "version": "3.31.2"}, {"value": "flannel", "label": "Flannel", "version": "0.27.4"}, {"value": "cilium", "label": "Cilium（待适配）", "disabled": true}},
	}
}

func (s *Service) ListK8sDeployPlans(pageNum, pageSize int, keyword, status string) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	pageNum = max(1, pageNum)
	pageSize = min(100, max(1, pageSize))
	query := s.db.Model(&model.K8sDeployPlan{})
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	items := []model.K8sDeployPlan{}
	if err := query.Omit("kube_config_cipher").Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": items, "total": total}, nil
}

func (s *Service) GetK8sDeployPlan(id uint) (model.K8sDeployPlan, error) {
	var plan model.K8sDeployPlan
	if err := s.requireDeployAdmin(); err != nil {
		return plan, err
	}
	err := s.db.First(&plan, id).Error
	return plan, err
}

func (s *Service) K8sDeployDetail(id uint) (map[string]any, error) {
	plan, err := s.GetK8sDeployPlan(id)
	if err != nil {
		return nil, err
	}
	runs := []model.K8sDeployRun{}
	if err := s.db.Where("plan_id = ?", id).Order("id DESC").Limit(50).Find(&runs).Error; err != nil {
		return nil, err
	}
	steps := []model.K8sDeployStep{}
	if plan.LastRunID > 0 {
		if err := s.db.Where("run_id = ?", plan.LastRunID).Order("id").Find(&steps).Error; err != nil {
			return nil, err
		}
	}
	return map[string]any{"plan": plan, "runs": runs, "steps": steps}, nil
}

func (s *Service) K8sDeployLogs(planID, runID, after uint) (map[string]any, error) {
	if _, err := s.GetK8sDeployPlan(planID); err != nil {
		return nil, err
	}
	var run model.K8sDeployRun
	if err := s.db.Where("id = ? AND plan_id = ?", runID, planID).First(&run).Error; err != nil {
		return nil, err
	}
	logs := []model.K8sDeployLog{}
	if err := s.db.Where("run_id = ? AND id > ?", runID, after).Order("id").Limit(200).Find(&logs).Error; err != nil {
		return nil, err
	}
	steps := []model.K8sDeployStep{}
	if err := s.db.Where("run_id = ?", runID).Order("id").Find(&steps).Error; err != nil {
		return nil, err
	}
	return map[string]any{"logs": logs, "steps": steps, "run": run}, nil
}

// Only project non-secret CMDB fields into this response. Never serialize credentials.
func (s *Service) K8sDeployHostOptions(keyword, environment string, groupID uint, pageNum, pageSize int) (map[string]any, error) {
	if err := s.requireDeployAdmin(); err != nil {
		return nil, err
	}
	query := s.db.Model(&model.AssetHost{}).Preload("Credential").Preload("Gateway")
	if keyword != "" {
		query = query.Where("host_name LIKE ? OR private_ip LIKE ? OR public_ip LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if environment != "" {
		query = query.Where("environment = ?", environment)
	}
	if groupID > 0 {
		query = query.Where("group_id = ? OR id IN (SELECT host_id FROM asset_host_group_rel WHERE group_id = ?)", groupID, groupID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	hosts := []model.AssetHost{}
	if err := query.Order("id DESC").Offset((max(1, pageNum) - 1) * min(100, max(1, pageSize))).Limit(min(100, max(1, pageSize))).Find(&hosts).Error; err != nil {
		return nil, err
	}
	locks := []model.K8sDeployHostLock{}
	if err := s.db.Find(&locks).Error; err != nil {
		return nil, err
	}
	list := []map[string]any{}
	for _, h := range hosts {
		reason := ""
		if h.Status != 1 {
			reason = "主机已停用"
		} else if h.CredentialID == nil || h.Credential.Status != 1 {
			reason = "请先在 CMDB 配置有效凭据"
		}
		var occupied uint
		for _, lock := range locks {
			if lock.HostID == h.ID {
				occupied = lock.PlanID
				if lock.PlanID == 0 {
					reason = "被节点添加任务占用或需要核对安装现场"
				}
				break
			}
		}
		list = append(list, map[string]any{"id": h.ID, "hostName": h.HostName, "privateIp": h.PrivateIP, "publicIp": h.PublicIP, "sshIp": h.SSHIP, "os": h.OS, "arch": h.Arch, "cpu": h.CPU, "memory": h.Memory, "disk": h.Disk, "environment": h.Environment, "groupId": h.GroupID, "connectionMode": h.ConnectionMode, "gatewayName": h.Gateway.Name, "credentialName": h.Credential.Name, "authType": h.Credential.AuthType, "aliveStatus": h.AliveStatus, "authStatus": h.AuthStatus, "disabledReason": reason, "occupiedPlanId": occupied})
	}
	return map[string]any{"list": list, "total": total}, nil
}

func (s *Service) SaveK8sDeployPlan(payload K8sDeployPayload, operator string) (model.K8sDeployPlan, error) {
	var plan model.K8sDeployPlan
	if err := s.requireDeployAdmin(); err != nil {
		return plan, err
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" || utf8.RuneCountInString(payload.Name) > 50 {
		return plan, errors.New("部署名称必填且不超过 50 个字符")
	}
	if utf8.RuneCountInString(payload.Description) > 255 || len(payload.Config.CustomSource) > 32768 || len(payload.Config.KubeletConfig) > 8192 || len(payload.Nodes) > 100 {
		return plan, errors.New("配置内容或节点数量超出限制")
	}
	seen := map[uint]bool{}
	for i := range payload.Nodes {
		n := &payload.Nodes[i]
		if n.HostID == 0 || seen[n.HostID] {
			return plan, errors.New("节点不能为空或重复")
		}
		seen[n.HostID] = true
		var h model.AssetHost
		if err := s.db.First(&h, n.HostID).Error; err != nil {
			return plan, fmt.Errorf("CMDB 主机 %d 不存在或不可访问", n.HostID)
		}
		n.HostName, n.OS, n.Arch, n.CPU, n.Memory = h.HostName, h.OS, h.Arch, h.CPU, h.Memory
		n.MachineID = ""
	}
	if payload.Nodes == nil {
		payload.Nodes = []model.K8sDeployNode{}
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if payload.ID > 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, payload.ID).Error; err != nil {
				return err
			}
			if plan.Revision != payload.Revision {
				return errors.New("计划已被更新，请刷新后重试")
			}
			if !slices.Contains([]string{"draft", "ready", "precheck_failed", "failed", "stopped"}, plan.Status) {
				return errors.New("当前状态不能编辑计划")
			}
			var locks int64
			if err := tx.Model(&model.K8sDeployHostLock{}).Where("plan_id = ?", plan.ID).Count(&locks).Error; err != nil {
				return err
			}
			if locks > 0 {
				return errors.New("节点已有安装变更，请先核对现场，不能修改计划后重新安装")
			}
		}
		plan.Name, plan.Env, plan.Description, plan.Config, plan.Nodes = payload.Name, payload.Env, strings.TrimSpace(payload.Description), payload.Config, payload.Nodes
		plan.Status = "draft"
		plan.Revision++
		if plan.ID == 0 {
			plan.Revision = 1
			plan.CreatedBy = operator
			plan.RegistrationStatus = "pending"
			return tx.Create(&plan).Error
		}
		return tx.Select("Name", "Env", "Description", "Config", "Nodes", "Status", "Revision", "UpdatedAt").Updates(&plan).Error
	})
	return plan, err
}

func validateDeployConfig(c model.K8sDeployConfig, nodes []model.K8sDeployNode) error {
	if !regexp.MustCompile(`^1\.(31|32|33|34)\.[0-9]{1,3}$`).MatchString(c.Version) {
		return errors.New("当前支持 Kubernetes 1.31.x–1.34.x，版本格式不含 v 前缀")
	}
	if c.NetworkPlugin != "calico" && c.NetworkPlugin != "flannel" {
		return errors.New("当前安装器支持 Calico、Flannel")
	}
	pod, err := netip.ParsePrefix(c.PodCIDR)
	if err != nil || !pod.Addr().Is4() || pod != pod.Masked() || pod.Bits() < 8 || pod.Bits() > 24 {
		return errors.New("Pod CIDR 必须为规范 IPv4 网段，掩码范围 /8–/24")
	}
	svc, err := netip.ParsePrefix(c.ServiceCIDR)
	if err != nil || !svc.Addr().Is4() || svc != svc.Masked() || svc.Bits() < 12 || svc.Bits() > 27 || pod.Overlaps(svc) {
		return errors.New("Service CIDR 必须为规范 IPv4 网段（/12–/27），且不能与 Pod 网段重叠")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.:/_-]{0,240}$`).MatchString(c.ImageRepository) || strings.Contains(c.ImageRepository, "://") {
		return errors.New("镜像仓库格式不正确，请勿包含协议或凭据")
	}
	if c.RuntimeDirectory != "" && (!path.IsAbs(c.RuntimeDirectory) || path.Clean(c.RuntimeDirectory) != c.RuntimeDirectory || len(c.RuntimeDirectory) < 5 || !regexp.MustCompile(`^/[a-zA-Z0-9/_.-]+$`).MatchString(c.RuntimeDirectory)) {
		return errors.New("运行时目录必须是规范绝对路径，不能使用根目录")
	}
	if c.ProxyMode != "iptables" && c.ProxyMode != "ipvs" {
		return errors.New("请选择有效的 Kube-proxy 模式")
	}
	if c.APIPort < 1 || c.APIPort > 65535 {
		return errors.New("API 端口必须在 1–65535 之间")
	}
	if !c.HA && c.APIServer == "" && c.APIPort != 6443 {
		return errors.New("直接使用控制平面 IP 时，API 入口端口必须为 6443")
	}
	if c.CustomCertificates && (c.CertificateYears < 1 || c.CertificateYears > 10 || c.CAYears < c.CertificateYears || c.CAYears > 30) {
		return errors.New("证书有效期应为 1–10 年，CA 应不少于证书有效期且不超过 30 年")
	}
	if c.HA && c.APIServer == "" {
		return errors.New("高可用部署必须填写 API Server 地址")
	}
	if c.APIServer != "" && net.ParseIP(c.APIServer) == nil && !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`).MatchString(c.APIServer) {
		return errors.New("API 地址应为 IP 或域名，不含协议和端口")
	}
	if c.BuiltinLB && (!c.HA || net.ParseIP(c.APIServer) == nil || net.ParseIP(c.APIServer).To4() == nil || c.APIPort == 6443 || c.RouterID < 1 || c.RouterID > 255) {
		return errors.New("自建负载均衡需要高可用、IPv4 VIP、1–255 路由 ID，入口端口不能占用 6443")
	}
	if c.CustomSource != "" {
		for _, line := range strings.Split(c.CustomSource, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !regexp.MustCompile(`^deb(-src)? https?://[^\s]+ [a-zA-Z0-9 ./_-]+$`).MatchString(line) {
				return errors.New("自定义软件源仅接受不带额外选项的 apt deb/deb-src 行")
			}
		}
	}
	if _, err := deployKubeletOverrides(c.KubeletConfig); err != nil {
		return err
	}
	masters := 0
	ips := map[string]bool{}
	names := map[string]bool{}
	if len(nodes) == 0 {
		return errors.New("请从 CMDB 关联节点")
	}
	for _, n := range nodes {
		ip, err := netip.ParseAddr(n.IP)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ips[n.IP] || pod.Contains(ip) || svc.Contains(ip) {
			return errors.New("节点 IP 无效、重复或与容器/Service 网段冲突")
		}
		ips[n.IP] = true
		if c.AutoHostname && (len(n.Name) > 63 || !deployNodeNamePattern.MatchString(n.Name) || names[n.Name]) {
			return errors.New("节点名应为唯一的小写 DNS 主机名，最多 63 字符")
		}
		names[n.Name] = true
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$`).MatchString(n.Interface) {
			return errors.New("请为每个节点填写实际网口名称")
		}
		if n.Role == "master" {
			masters++
		} else if n.Role != "worker" {
			return errors.New("节点角色无效")
		}
	}
	if (!c.HA && masters != 1) || (c.HA && masters != 3) {
		return errors.New("普通部署需要 1 个控制平面节点，高可用需要 3 个")
	}
	if len(nodes) == masters && !c.AllowControlPlaneWorkloads {
		return errors.New("没有工作节点时，请开启控制平面运行普通业务")
	}
	if c.BuiltinLB && ips[c.APIServer] {
		return errors.New("VIP 不能与节点 IP 相同")
	}
	return nil
}

func deployKubeletOverrides(raw string) (map[string]any, error) {
	values := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return values, nil
	}
	if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("自定义 kubelet 配置不是有效 YAML: %w", err)
	}
	if values == nil {
		return nil, errors.New("自定义 kubelet 配置应为 YAML 映射，不能为 null")
	}
	for k, v := range values {
		switch k {
		case "maxPods":
			n, ok := v.(int)
			if !ok || n < 10 || n > 250 {
				return nil, errors.New("maxPods 应为 10–250 的整数")
			}
		case "systemReserved", "kubeReserved":
			m, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("%s 应为资源映射", k)
			}
			for key, value := range m {
				text, ok := value.(string)
				if !slices.Contains([]string{"cpu", "memory", "ephemeral-storage"}, key) || !ok || !regexp.MustCompile(`^[0-9]+(m|Ki|Mi|Gi|Ti)?$`).MatchString(text) {
					return nil, errors.New("预留资源仅支持 cpu、memory、ephemeral-storage 及有效数量字符串")
				}
			}
		default:
			return nil, fmt.Errorf("不支持自定义 kubelet 字段 %s，仅允许 maxPods、systemReserved、kubeReserved", k)
		}
	}
	return values, nil
}

func (s *Service) deployHosts(nodes []model.K8sDeployNode) ([]model.AssetHost, string, error) {
	hosts := []model.AssetHost{}
	seen := map[uint]bool{}
	for _, n := range nodes {
		if seen[n.HostID] {
			return nil, "", errors.New("不能重复选择主机")
		}
		seen[n.HostID] = true
		if err := s.requireResource("host", n.HostID, "", "manage"); err != nil {
			return nil, "", err
		}
		var h model.AssetHost
		if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&h, n.HostID).Error; err != nil {
			return nil, "", err
		}
		if h.Status != 1 || h.CredentialID == nil || h.Credential.Status != 1 {
			return nil, "", fmt.Errorf("主机 %s 已停用或缺少有效凭据", h.HostName)
		}
		hosts = append(hosts, h)
	}
	slices.SortFunc(hosts, func(a, b model.AssetHost) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	// Includes credential and gateway revisions; stored only as a digest, never in API/logs.
	data, _ := json.Marshal(hosts)
	sum := sha256.Sum256(data)
	return hosts, hex.EncodeToString(sum[:]), nil
}

func (s *Service) StartK8sDeployRun(id uint, revision int, kind, operator string) (model.K8sDeployRun, error) {
	var run model.K8sDeployRun
	plan, err := s.GetK8sDeployPlan(id)
	if err != nil {
		return run, err
	}
	if kind != "precheck" && kind != "deploy" {
		return run, errors.New("任务类型无效")
	}
	if err = validateDeployConfig(plan.Config, plan.Nodes); err != nil {
		return run, err
	}
	if strings.TrimSpace(plan.Env) == "" {
		return run, errors.New("请选择部署所属环境，以便完成集群管理接入")
	}
	hosts, fingerprint, err := s.deployHosts(plan.Nodes)
	if err != nil {
		return run, err
	}
	created := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, id).Error; err != nil {
			return err
		}
		if plan.Revision != revision {
			return errors.New("计划已修改，请刷新后重新预检")
		}
		if plan.Status == "prechecking" || plan.Status == "deploying" || plan.Status == "stopping" {
			if err := tx.First(&run, plan.LastRunID).Error; err != nil {
				return err
			}
			if run.Kind == kind {
				return nil
			}
			return errors.New("该计划已有任务运行中")
		}
		if !slices.Contains([]string{"draft", "ready", "precheck_failed", "failed", "stopped"}, plan.Status) {
			return errors.New("当前状态不能开始任务，请先查看执行记录")
		}
		if kind == "deploy" {
			var check model.K8sDeployRun
			if err := tx.Where("plan_id = ? AND kind = ? AND status = ? AND revision = ?", id, "precheck", "success", revision).Order("id DESC").First(&check).Error; err != nil {
				return errors.New("请先完成当前配置的预检")
			}
			if check.Fingerprint != fingerprint || check.FinishedAt == nil || time.Since(*check.FinishedAt) > 15*time.Minute {
				return errors.New("CMDB 信息已变化或预检超过 15 分钟，请重新预检")
			}
		}
		// CAS also serializes concurrent submissions on SQLite/DBs without FOR UPDATE.
		status := "prechecking"
		if kind == "deploy" {
			status = "deploying"
		}
		res := tx.Model(&model.K8sDeployPlan{}).Where("id = ? AND revision = ? AND status = ?", id, revision, plan.Status).Update("status", status)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("计划状态已变化，请刷新")
		}
		run = model.K8sDeployRun{PlanID: id, Revision: revision, Kind: kind, Status: deployActive, Operator: operator, Config: plan.Config, Nodes: plan.Nodes, Fingerprint: fingerprint, StartedAt: time.Now(), HeartbeatAt: time.Now()}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		for _, h := range hosts {
			if err := tx.Create(&model.K8sDeployHostLock{HostID: h.ID, PlanID: id, RunID: run.ID}).Error; err != nil {
				return fmt.Errorf("主机 %s 已被其他任务或集群占用，请查看关联部署", h.HostName)
			}
		}
		if err := tx.Model(&plan).Update("last_run_id", run.ID).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err == nil && created {
		go s.processK8sDeploy(run, hosts)
	}
	return run, err
}

func (s *Service) StopK8sDeploy(id uint) error {
	plan, err := s.GetK8sDeployPlan(id)
	if err != nil {
		return err
	}
	if plan.Status != "deploying" && plan.Status != "prechecking" && plan.Status != "stopping" {
		return errors.New("当前没有正在运行的任务")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", plan.LastRunID, deployActive).Update("stop_requested", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("任务已结束，请刷新")
		}
		return tx.Model(&model.K8sDeployPlan{}).Where("id = ? AND last_run_id = ?", id, plan.LastRunID).Update("status", "stopping").Error
	})
}

func (s *Service) DeleteK8sDeploy(id uint) error {
	plan, err := s.GetK8sDeployPlan(id)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.K8sDeployRun{}).Where("plan_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("已有执行记录的计划请保留用于审计，不能删除")
		}
		res := tx.Where("id = ? AND status = ?", id, "draft").Delete(&model.K8sDeployPlan{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("仅可删除未执行的草稿")
		}
		return nil
	})
}

func (s *Service) initK8sDeployRecovery() {
	go func() {
		for {
			_ = s.db.Model(&model.K8sDeployPlan{}).Where("registration_status = ? AND updated_at < ?", "registering", time.Now().Add(-2*time.Minute)).Updates(map[string]any{"registration_status": "failed", "registration_error": "接入过程被中断，可重新接入，无需重新安装"}).Error
			var stale []model.K8sDeployRun
			if s.db.Where("status = ? AND heartbeat_at < ?", deployActive, time.Now().Add(-2*time.Minute)).Find(&stale).Error == nil {
				for _, r := range stale {
					_ = s.db.Transaction(func(tx *gorm.DB) error {
						now := time.Now()
						runStatus, planStatus, message := "needs_review", "needs_review", "执行器心跳中断，请核对远端状态；不会自动重新安装"
						if !r.Mutated {
							runStatus = "failed"
							planStatus = "precheck_failed"
							if r.Kind == "deploy" {
								planStatus = "failed"
							}
							message = "执行器在系统变更前中断，可重新预检"
						}
						res := tx.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ? AND heartbeat_at < ? AND mutated = ?", r.ID, deployActive, now.Add(-2*time.Minute), r.Mutated).Updates(map[string]any{"status": runStatus, "error": message, "finished_at": now})
						if res.Error != nil {
							return res.Error
						}
						if res.RowsAffected == 0 {
							return nil
						}
						if err := tx.Model(&model.K8sDeployPlan{}).Where("id = ? AND last_run_id = ?", r.PlanID, r.ID).Update("status", planStatus).Error; err != nil {
							return err
						}
						if !r.Mutated {
							return tx.Where("run_id = ?", r.ID).Delete(&model.K8sDeployHostLock{}).Error
						}
						return nil
					})
				}
			}
			time.Sleep(time.Minute)
		}
	}()
}
