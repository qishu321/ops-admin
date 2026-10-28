package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"ops-admin/backend/model"
)

func nodeAddProbeIdentity(n model.K8sDeployNode, probe deployProbe, existing []corev1.Node, installing bool) (model.K8sDeployNode, error) {
	if len(probe.Errors) > 0 {
		return n, errors.New(strings.Join(probe.Errors, "；"))
	}
	if probe.MachineID == "" || probe.Interface == "" {
		return n, errors.New("无法确认主机身份或节点网卡")
	}
	for _, old := range existing {
		if old.Name == n.Name || old.Status.NodeInfo.MachineID == probe.MachineID {
			return n, errors.New("主机身份或节点名已经存在于目标集群")
		}
		for _, a := range old.Status.Addresses {
			if a.Type == corev1.NodeInternalIP && a.Address == n.IP {
				return n, errors.New("节点 IP 已属于目标集群")
			}
		}
	}
	if installing && (n.MachineID != probe.MachineID || n.OS != probe.OS+" "+probe.Version || n.Arch != probe.Arch || n.Interface != probe.Interface) {
		return n, errors.New("主机身份、系统或网卡与预检不一致，请重新预检")
	}
	n.MachineID, n.OS, n.Arch, n.Interface = probe.MachineID, probe.OS+" "+probe.Version, probe.Arch, probe.Interface
	return n, nil
}

func (s *Service) processK8sNodeAdd(task model.K8sNodeAddTask, run model.K8sDeployRun, hosts []model.AssetHost) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_ = s.db.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", run.ID, deployActive).Update("heartbeat_at", time.Now()).Error
			}
		}
	}()
	defer close(done)
	var runErr error
	defer func() {
		if recover() != nil {
			runErr = errors.New("执行器异常，请核对节点现场")
		}
		s.finishK8sNodeAdd(task, run, runErr)
	}()
	snap, err := s.nodeAddClusterSnapshot(task.ClusterID)
	if err != nil {
		runErr = err
		return
	}
	defer snap.Cleanup()
	if snap.Hash != task.ClusterHash {
		runErr = errors.New("目标集群身份或配置已变化，请重新预检")
		return
	}
	identities := map[string]bool{}
	for i := range run.Nodes {
		n := run.Nodes[i]
		var host model.AssetHost
		for _, h := range hosts {
			if h.ID == n.HostID {
				host = h
				break
			}
		}
		err = s.deployStep(run, host, "节点预检", false, func(log func(string)) error {
			probeNode := n
			probeNode.Interface = "auto"
			output, e := s.deployCommand(host, deployProbeScriptOptions(run.Config, probeNode, true), 180, nil)
			if e != nil {
				return e
			}
			var probe deployProbe
			if json.Unmarshal([]byte(strings.TrimSpace(output)), &probe) != nil {
				return errors.New("主机探测结果无法解析，请确认 python3 与 iproute2 可用")
			}
			updated, e := nodeAddProbeIdentity(n, probe, snap.Nodes, task.Kind == "add")
			if e != nil {
				return e
			}
			if identities[probe.MachineID] {
				return errors.New("所选资产指向同一台真实服务器")
			}
			identities[probe.MachineID] = true
			for _, network := range probe.Networks {
				prefix, e := netip.ParsePrefix(network)
				if e != nil || prefix.Bits() == 0 {
					continue
				}
				for _, cidr := range []string{run.Config.PodCIDR, run.Config.ServiceCIDR} {
					wanted, _ := netip.ParsePrefix(cidr)
					if prefix.Overlaps(wanted) {
						return fmt.Errorf("主机网络 %s 与集群网段 %s 冲突", network, cidr)
					}
				}
			}
			run.Nodes[i] = updated
			log(fmt.Sprintf("%s：%s / %s，网卡 %s，CPU %d，内存 %d MiB，磁盘 %d GiB；主机身份及干净状态检查通过。\n", host.HostName, updated.OS, updated.Arch, updated.Interface, probe.CPU, probe.MemoryMB, probe.DiskGB))
			return nil
		})
		if err != nil {
			runErr = err
			return
		}
		err = s.deployStep(run, host, "网络检查", false, func(log func(string)) error {
			script := "python3 - " + shellQuote(run.Config.APIServer) + " " + shellQuote(fmt.Sprint(run.Config.APIPort)) + " <<'PY'\nimport socket,sys\nsocket.create_connection((sys.argv[1],int(sys.argv[2])),timeout=8).close()\nPY\n"
			_, e := s.deployCommand(host, script, 30, nil)
			if e == nil {
				log("目标服务器到集群 API 入口连接通过；网络插件沿用现有集群。\n")
				if run.Config.PodCIDR == "" {
					return &deployWarning{message: "集群未记录 Pod CIDR，未执行 Pod 网段冲突检查，请核对现有 CNI 地址池与新服务器网段"}
				}
			}
			return e
		})
		if err != nil {
			runErr = err
			return
		}
	}
	res := s.db.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", run.ID, deployActive).Select("Nodes").Updates(&model.K8sDeployRun{Nodes: run.Nodes})
	if res.Error != nil || res.RowsAffected != 1 {
		runErr = errors.New("执行权已失效，未开始安装")
		return
	}
	if task.Kind == "precheck" {
		return
	}
	// Recheck the same pinned target immediately before the first system mutation.
	current, e := s.nodeAddClusterSnapshot(task.ClusterID)
	if e != nil {
		runErr = e
		return
	}
	current.Cleanup()
	if current.Hash != task.ClusterHash {
		runErr = errors.New("目标集群已变化，请重新预检")
		return
	}
	success := 0
	for _, n := range run.Nodes {
		var host model.AssetHost
		for _, h := range hosts {
			if h.ID == n.HostID {
				host = h
				break
			}
		}
		// A batch can take a long time: bind each machine again immediately before its own installation.
		err = s.deployStep(run, host, "执行前核验", false, func(log func(string)) error {
			probeNode := n
			probeNode.Interface = "auto"
			out, e := s.deployCommand(host, deployProbeScriptOptions(run.Config, probeNode, true), 180, nil)
			if e != nil {
				return e
			}
			var probe deployProbe
			if json.Unmarshal([]byte(strings.TrimSpace(out)), &probe) != nil {
				return errors.New("执行前主机探测结果无法解析")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			nodes, e := snap.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if e != nil {
				return errors.New("执行前无法核对当前节点列表")
			}
			if _, e = nodeAddProbeIdentity(n, probe, nodes.Items, true); e != nil {
				return e
			}
			log("主机身份、干净状态和当前节点冲突再次核验通过。\n")
			return nil
		})
		if err == nil {
			err = s.deployStep(run, host, "安装组件", true, func(log func(string)) error {
				_, e := s.deployCommand(host, deployPrepareScript(run.Config, n), 1800, log)
				return e
			})
		}
		if err == nil {
			err = s.deployStep(run, host, "加入集群", true, func(log func(string)) error {
				if e := s.nodeAddJoinWorker(snap, run, task.Config, n, host); e != nil {
					return e
				}
				log("kubeadm join 已完成，敏感输出已隐藏。\n")
				return nil
			})
		}
		if err == nil {
			err = s.deployStep(run, host, "等待 Ready", false, func(log func(string)) error {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				ticker := time.NewTicker(4 * time.Second)
				defer ticker.Stop()
				for {
					node, e := snap.Client.CoreV1().Nodes().Get(ctx, n.Name, metav1.GetOptions{})
					if e == nil {
						if node.Status.NodeInfo.MachineID != "" {
							if e = nodeAddVerifyRegistered(*node, n, run.Config.Version); e != nil {
								return e
							}
							for _, condition := range node.Status.Conditions {
								if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
									log("节点身份、InternalIP、版本及 Ready 状态验证通过。\n")
									return nil
								}
							}
						}
					} else if !apierrors.IsNotFound(e) {
						return errors.New("读取节点状态失败，请核对集群连接和节点实际状态")
					}
					select {
					case <-ctx.Done():
						return errors.New("等待节点 Ready 超时，请核对 CNI、kubelet 和集群事件")
					case <-ticker.C:
					}
				}
			})
		}
		if err == nil {
			err = s.deployStep(run, host, "完成", false, func(log func(string)) error { log("Worker 节点添加成功。\n"); return nil })
		}
		if err != nil {
			runErr = err
			if errors.Is(err, errDeployStopped) {
				break
			}
		} else {
			success++
		}
	}
	if success > 0 {
		s.invalidateK8sClusterDetailCache(task.ClusterID)
	}
}

func nodeAddVerifyRegistered(node corev1.Node, expected model.K8sDeployNode, version string) error {
	if node.Status.NodeInfo.MachineID == "" {
		return errors.New("节点尚未上报主机身份")
	}
	if node.Status.NodeInfo.MachineID != expected.MachineID {
		return errors.New("同名节点主机身份与预检不符，停止后续操作")
	}
	found := false
	for _, a := range node.Status.Addresses {
		if a.Type == corev1.NodeInternalIP && a.Address == expected.IP {
			found = true
		}
	}
	if !found || strings.TrimPrefix(node.Status.NodeInfo.KubeletVersion, "v") != version {
		return errors.New("注册节点 IP 或 kubelet 版本与任务不一致")
	}
	return nil
}

func nodeAddJoinConfig(c model.K8sDeployConfig, config model.K8sNodeAddConfig, n model.K8sDeployNode, endpoint, token, hash string) (string, error) {
	if n.Role != "worker" {
		return "", errors.New("本入口只允许添加 Worker")
	}
	labels, taints, err := nodeAddMetadata(config.Labels, false)
	if err != nil {
		return "", err
	}
	_, taints, err = nodeAddMetadata(config.Taints, true)
	if err != nil {
		return "", err
	}
	registration := deployNodeRegistration(n)
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := []string{}
		for _, k := range keys {
			pairs = append(pairs, k+"="+labels[k])
		}
		registration["kubeletExtraArgs"] = []map[string]string{{"name": "node-ip", "value": n.IP}, {"name": "node-labels", "value": strings.Join(pairs, ",")}}
	}
	if len(taints) > 0 {
		values := []map[string]string{}
		for _, t := range taints {
			values = append(values, map[string]string{"key": t.Key, "value": t.Value, "effect": string(t.Effect)})
		}
		registration["taints"] = values
	}
	return deployYAMLDocuments(map[string]any{"apiVersion": "kubeadm.k8s.io/v1beta4", "kind": "JoinConfiguration", "nodeRegistration": registration, "discovery": map[string]any{"bootstrapToken": map[string]any{"apiServerEndpoint": endpoint, "token": token, "caCertHashes": []string{"sha256:" + hash}}}})
}

func (s *Service) nodeAddJoinWorker(snap *nodeAddCluster, run model.K8sDeployRun, config model.K8sNodeAddConfig, n model.K8sDeployNode, host model.AssetHost) error {
	return executeNodeAddJoin(snap, run, config, n, func(script string) error { _, err := s.deployCommand(host, script, 900, nil); return err }, func(message string) {
		_ = s.db.Create(&model.K8sDeployLog{RunID: run.ID, HostID: host.ID, Content: message}).Error
	})
}

func executeNodeAddJoin(snap *nodeAddCluster, run model.K8sDeployRun, config model.K8sNodeAddConfig, n model.K8sDeployNode, execute func(string) error, warn func(string)) error {
	bytes := make([]byte, 11)
	if _, err := rand.Read(bytes); err != nil {
		return errors.New("生成 bootstrap token 失败")
	}
	id, secret := hex.EncodeToString(bytes[:3]), hex.EncodeToString(bytes[3:])
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	token := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-token-" + id, Namespace: "kube-system"}, Type: corev1.SecretTypeBootstrapToken, Data: map[string][]byte{"token-id": []byte(id), "token-secret": []byte(secret), "expiration": []byte(time.Now().UTC().Add(20 * time.Minute).Format(time.RFC3339)), "usage-bootstrap-authentication": []byte("true"), "usage-bootstrap-signing": []byte("true"), "auth-extra-groups": []byte("system:bootstrappers:kubeadm:default-node-token")}}
	created, err := snap.Client.CoreV1().Secrets("kube-system").Create(ctx, token, metav1.CreateOptions{})
	if err != nil {
		return errors.New("无法创建短期 bootstrap token，请核对集群权限")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if e := snap.Client.CoreV1().Secrets("kube-system").Delete(ctx, created.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &created.UID}}); e != nil && !apierrors.IsNotFound(e) {
			warn("临时 bootstrap token 撤销失败，将在 20 分钟有效期后过期，请管理员核对。\n")
		}
	}()
	yml, err := nodeAddJoinConfig(run.Config, config, n, snap.Endpoint, id+"."+secret, snap.CAHash)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("/etc/kubernetes/ops-admin-node-add-%d.yaml", run.ID)
	script := "trap 'rm -f " + filename + "' EXIT\n" + deployWriteFile(filename, yml) + "\nkubeadm join --config " + shellQuote(filename) + "\n"
	if err = execute(script); err != nil {
		return errors.New("kubeadm join 失败，请核对 kubelet/containerd 和集群事件；敏感输出已隐藏")
	}
	return nil
}

func (s *Service) finishK8sNodeAdd(task model.K8sNodeAddTask, run model.K8sDeployRun, runErr error) {
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		var state model.K8sDeployRun
		if e := tx.First(&state, run.ID).Error; e != nil {
			return e
		}
		if state.Status != deployActive {
			return nil
		}
		steps := []model.K8sDeployStep{}
		if e := tx.Where("run_id = ?", run.ID).Find(&steps).Error; e != nil {
			return e
		}
		success, unsafe := 0, false
		for _, n := range state.Nodes {
			mutated, done := false, false
			for _, step := range steps {
				if step.HostID == n.HostID {
					if nodeAddStepStage(step.Title) >= 2 {
						mutated = true
					}
					if (step.Title == "完成" || step.Title == "等待 Ready") && step.Status == "success" {
						done = true
					}
				}
			}
			if done {
				success++
			}
			if mutated && !done {
				unsafe = true
			}
			if !mutated || done {
				if e := tx.Where("host_id = ? AND run_id = ?", n.HostID, run.ID).Delete(&model.K8sDeployHostLock{}).Error; e != nil {
					return e
				}
			}
		}
		status, message := "success", ""
		if runErr != nil {
			status, message = "failed", runErr.Error()
			if errors.Is(runErr, errDeployStopped) {
				status = "stopped"
			}
			if unsafe {
				status = "needs_review"
			}
			if success > 0 {
				status = "partial"
			}
			if task.Kind == "add" && success == len(state.Nodes) {
				status, message = "success", ""
			}
		}
		now := time.Now()
		res := tx.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", run.ID, deployActive).Updates(map[string]any{"status": status, "error": message, "finished_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("执行权已变化，保留主机占用")
		}
		return nil
	})
}
