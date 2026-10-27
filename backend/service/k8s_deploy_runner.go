package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

var errDeployStopped = errors.New("已在阶段边界停止；已执行的系统变更保留")

type deployWarning struct{ message string }

func (e *deployWarning) Error() string { return e.message }

type deployProbe struct {
	OS        string   `json:"os"`
	Version   string   `json:"version"`
	Arch      string   `json:"arch"`
	Hostname  string   `json:"hostname"`
	MachineID string   `json:"machineId"`
	CPU       int      `json:"cpu"`
	MemoryMB  int64    `json:"memoryMB"`
	DiskGB    int64    `json:"diskGB"`
	Addresses []string `json:"addresses"`
	Networks  []string `json:"networks"`
	Errors    []string `json:"errors"`
}

func deployRootCommand(script string) string {
	// The script never travels through a user-controlled shell expansion.
	return "set -e; if [ \"$(id -u)\" = 0 ]; then bash -se; else sudo -n bash -se; fi <<'OPS_ADMIN_SCRIPT'\nset -euo pipefail\n" + script + "\nOPS_ADMIN_SCRIPT"
}

func (s *Service) processK8sDeploy(run model.K8sDeployRun, hosts []model.AssetHost) {
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
	defer func() {
		if recovered := recover(); recovered != nil {
			s.finishK8sDeploy(run.ID, errors.New("部署执行异常，请查看阶段记录并核对节点状态"))
		}
	}()
	probes := map[uint]deployProbe{}
	identities := map[string]bool{}
	var err error
	for _, host := range hosts {
		n := deployNodeForHost(run.Nodes, host.ID)
		var probe deployProbe
		err = s.deployStep(run, host, "节点预检", false, func(log func(string)) error {
			output, err := s.deployCommand(host, deployProbeScript(run.Config, n), 180, nil)
			if err != nil {
				return err
			}
			if err = json.Unmarshal([]byte(strings.TrimSpace(output)), &probe); err != nil {
				return errors.New("无法解析节点探测结果，请确认 python3 与 iproute2 可用")
			}
			log(fmt.Sprintf("识别系统 %s %s / %s，CPU %d，内存 %d MiB，可用磁盘 %d GiB\n", probe.OS, probe.Version, probe.Arch, probe.CPU, probe.MemoryMB, probe.DiskGB))
			if probe.MachineID == "" {
				return errors.New("无法获取主机身份 machine-id")
			}
			if identities[probe.MachineID] {
				return errors.New("所选 CMDB 记录指向同一台真实主机")
			}
			identities[probe.MachineID] = true
			if len(probe.Errors) > 0 {
				return errors.New(strings.Join(probe.Errors, "；"))
			}
			for _, network := range probe.Networks {
				prefix, e := netip.ParsePrefix(network)
				if e != nil {
					continue
				}
				for _, cidr := range []string{run.Config.PodCIDR, run.Config.ServiceCIDR} {
					wanted, _ := netip.ParsePrefix(cidr)
					if prefix.Bits() > 0 && prefix.Overlaps(wanted) {
						return fmt.Errorf("主机网络 %s 与集群网段 %s 冲突", network, cidr)
					}
				}
			}
			log("SSH、提权、系统、资源、网口、现有安装痕迹、软件源及镜像仓库探测通过。未知 VPC 网段仍需人工核对。\n")
			return nil
		})
		if err != nil {
			break
		}
		probes[host.ID] = probe
	}
	if err == nil && len(probes) > 1 {
		var first deployProbe
		for i, h := range hosts {
			p := probes[h.ID]
			if i == 0 {
				first = p
			} else if p.OS != first.OS || p.Version != first.Version || p.Arch != first.Arch {
				err = errors.New("当前安装器要求所有节点的发行版、系统版本及架构一致")
			}
		}
	}
	if err == nil {
		// Resolve real names only after probing; subsequent init/join use this immutable run snapshot.
		run.Nodes, err = resolveDeployNodes(run, probes)
	}
	if err == nil {
		err = s.deployConnectivity(run, hosts)
	}
	if err == nil {
		err = s.db.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", run.ID, deployActive).Select("Nodes").Updates(&model.K8sDeployRun{Nodes: run.Nodes}).Error
	}
	if err == nil && run.Kind == "precheck" {
		err = s.db.Model(&model.K8sDeployPlan{}).Where("id = ? AND revision = ? AND last_run_id = ?", run.PlanID, run.Revision, run.ID).Select("Nodes").Updates(&model.K8sDeployPlan{Nodes: run.Nodes}).Error
	}
	if err == nil && run.Kind == "deploy" {
		err = s.installK8sDeploy(run, hosts)
	}
	s.finishK8sDeploy(run.ID, err)
}

func deployNodeForHost(nodes []model.K8sDeployNode, id uint) model.K8sDeployNode {
	for _, n := range nodes {
		if n.HostID == id {
			return n
		}
	}
	return model.K8sDeployNode{}
}

func resolveDeployNodes(run model.K8sDeployRun, probes map[uint]deployProbe) ([]model.K8sDeployNode, error) {
	nodes := append([]model.K8sDeployNode{}, run.Nodes...)
	names := map[string]bool{}
	for i := range nodes {
		n := &nodes[i]
		p, ok := probes[n.HostID]
		if !ok {
			return nil, errors.New("节点探测结果缺失")
		}
		name := n.Name
		if !run.Config.AutoHostname {
			name = p.Hostname
		}
		if run.Kind == "deploy" && (n.MachineID == "" || n.MachineID != p.MachineID || n.OS != p.OS+" "+p.Version || n.Arch != p.Arch || (!run.Config.AutoHostname && n.Name != name)) {
			return nil, fmt.Errorf("主机 %s 的身份、系统或主机名与预检不一致，请重新预检", n.HostName)
		}
		if len(name) > 63 || !deployNodeNamePattern.MatchString(name) || names[name] {
			return nil, fmt.Errorf("实际节点名 %s 无效或重复，请开启自动命名并设置唯一名称", name)
		}
		names[name] = true
		n.Name, n.OS, n.Arch, n.MachineID = name, p.OS+" "+p.Version, p.Arch, p.MachineID
	}
	return nodes, nil
}

func (s *Service) deployCommand(h model.AssetHost, script string, timeout int, output func(string)) (string, error) {
	r := s.execCommandOnHostStreaming(h, deployRootCommand(script), timeout, output)
	if r.Status != "success" {
		message := strings.TrimSpace(r.Stderr)
		if len(message) > 2000 {
			message = message[len(message)-2000:]
		}
		return "", fmt.Errorf("主机 %s 执行失败（退出码 %d）：%s %s", h.HostName, r.ExitCode, r.ErrorText, message)
	}
	return r.Stdout, nil
}

func (s *Service) deployStep(run model.K8sDeployRun, host model.AssetHost, title string, mutates bool, fn func(func(string)) error) error {
	var state model.K8sDeployRun
	if err := s.db.First(&state, run.ID).Error; err != nil {
		return err
	}
	if state.Status != deployActive {
		return errors.New("任务执行权已失效，请核对现场")
	}
	if state.StopRequested {
		return errDeployStopped
	}
	if mutates {
		res := s.db.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ? AND stop_requested = ?", run.ID, deployActive, false).Update("mutated", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errDeployStopped
		}
	}
	step := model.K8sDeployStep{RunID: run.ID, HostID: host.ID, HostName: host.HostName, Title: title, Status: deployActive, StartedAt: time.Now()}
	if err := s.db.Create(&step).Error; err != nil {
		return err
	}
	// Persist bounded chunks throughout long-running SSH commands. Never stream secret-producing commands.
	var mu sync.Mutex
	var buffer strings.Builder
	total := 0
	var logErr error
	flush := func() {
		mu.Lock()
		defer mu.Unlock()
		if buffer.Len() == 0 || logErr != nil {
			return
		}
		chunk := buffer.String()
		buffer.Reset()
		logErr = s.db.Create(&model.K8sDeployLog{RunID: run.ID, StepID: step.ID, HostID: host.ID, Content: chunk}).Error
	}
	logger := func(chunk string) {
		mu.Lock()
		defer mu.Unlock()
		if total >= 256*1024 {
			return
		}
		if len(chunk) > 256*1024-total {
			chunk = chunk[:256*1024-total]
		}
		buffer.WriteString(chunk)
		total += len(chunk)
	}
	stopFlush := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopFlush:
				flush()
				return
			case <-ticker.C:
				flush()
			}
		}
	}()
	err := fn(logger)
	close(stopFlush)
	<-flushed
	if err == nil && logErr != nil {
		err = fmt.Errorf("阶段日志保存失败: %w", logErr)
	}
	status, summary := "success", "完成"
	var warning *deployWarning
	if errors.As(err, &warning) {
		status, summary = "warning", warning.Error()
		err = nil
	}
	if err != nil {
		status = "failed"
		summary = err.Error()
	}
	finished := time.Now()
	saveErr := s.db.Model(&step).Updates(map[string]any{"status": status, "summary": summary, "finished_at": finished}).Error
	if err != nil {
		return err
	}
	return saveErr
}

func (s *Service) finishK8sDeploy(runID uint, runErr error) {
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		var r model.K8sDeployRun
		if err := tx.First(&r, runID).Error; err != nil {
			return err
		}
		if r.Status != deployActive {
			return nil
		}
		status, planStatus, message := "success", "ready", ""
		if r.Kind == "deploy" {
			planStatus = "success"
		}
		if runErr != nil {
			status = "failed"
			planStatus = "precheck_failed"
			message = runErr.Error()
			if r.Kind == "deploy" {
				planStatus = "failed"
			}
			if errors.Is(runErr, errDeployStopped) {
				status = "stopped"
				planStatus = "stopped"
			}
		}
		if r.Mutated && runErr != nil {
			planStatus = "needs_review"
		}
		now := time.Now()
		res := tx.Model(&model.K8sDeployRun{}).Where("id = ? AND status = ?", runID, deployActive).Updates(map[string]any{"status": status, "error": message, "finished_at": now})
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
			return tx.Where("run_id = ?", runID).Delete(&model.K8sDeployHostLock{}).Error
		}
		return nil
	})
}

func (s *Service) deployConnectivity(run model.K8sDeployRun, hosts []model.AssetHost) error {
	for _, h := range hosts {
		if err := s.deployStep(run, h, "节点网络检查", false, func(log func(string)) error {
			var commands []string
			for _, n := range run.Nodes {
				if n.HostID != h.ID {
					commands = append(commands, "ping -c 1 -W 3 "+shellQuote(n.IP)+" >/dev/null || { echo '节点间 ICMP 不通，请确认节点互通后重试' >&2; exit 1; }")
				}
			}
			if run.Config.HA && !run.Config.BuiltinLB {
				// Before kubeadm init, a correctly configured LB may refuse connections
				// because no backend is ready yet. Timeouts/routing errors still block.
				commands = append(commands, "python3 - "+shellQuote(run.Config.APIServer)+" "+shellQuote(fmt.Sprint(run.Config.APIPort))+" <<'PY'\nimport socket, sys\ntry:\n    socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=5).close()\nexcept ConnectionRefusedError:\n    print('LB 地址可达，后端尚未就绪；集群初始化后将验证 API 入口')\nexcept OSError as e:\n    raise SystemExit('无法访问 LB 地址：'+str(e))\nPY")
			}
			if len(commands) > 0 {
				if _, err := s.deployCommand(h, strings.Join(commands, "\n"), 60, nil); err != nil {
					return err
				}
			}
			log("节点间 ICMP 检查通过；安装后的组件端口和网络连通性将再次验证。\n")
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func deployProbeScript(c model.K8sDeployConfig, n model.K8sDeployNode) string {
	params, _ := json.Marshal(map[string]any{"interface": n.Interface, "ip": n.IP, "version": c.Version, "repository": c.ImageRepository, "master": n.Role == "master", "builtinLB": c.BuiltinLB, "apiPort": c.APIPort, "runtimeDirectory": c.RuntimeDirectory, "vip": c.APIServer, "plugin": c.NetworkPlugin, "metrics": c.MetricsServer})
	return "set -euo pipefail\ncommand -v python3 >/dev/null || { echo '目标主机需要预装 python3' >&2; exit 1; }\npython3 - " + shellQuote(base64.StdEncoding.EncodeToString(params)) + " <<'PY'\n" + deployProbePython + "\nPY"
}

const deployProbePython = `import os, sys, json, platform, subprocess, shutil, urllib.request, base64, gzip
p=json.loads(base64.b64decode(sys.argv[1])); errors=[]
release={}
for line in open('/etc/os-release'):
    if '=' in line:
        k,v=line.strip().split('=',1); release[k]=v.strip('"')
system=release.get('ID',''); version=release.get('VERSION_ID',''); arch=platform.machine()
if not ((system=='ubuntu' and version in ('22.04','24.04')) or (system=='debian' and version in ('12','13'))): errors.append('不支持的系统：'+system+' '+version)
if arch not in ('x86_64','aarch64'): errors.append('不支持的架构：'+arch)
if not shutil.which('apt-get') or not shutil.which('ip') or not shutil.which('systemctl'): errors.append('需要 apt-get、iproute2 和 systemd')
cpu=os.cpu_count() or 0
memory=int(next((l.split()[1] for l in open('/proc/meminfo') if l.startswith('MemTotal:')),0))//1024
disk=shutil.disk_usage('/').free//(1024**3)
if cpu<2 or memory<1800 or disk<15: errors.append('至少需要 2 CPU、约 2 GiB 内存和 15 GiB 可用磁盘')
for target in ('/etc/kubernetes/admin.conf','/etc/kubernetes/kubelet.conf','/etc/kubernetes/manifests/kube-apiserver.yaml','/var/lib/etcd/member'):
    if os.path.exists(target): errors.append('发现现有 Kubernetes/etcd 数据，禁止自动覆盖'); break
for service in ('docker','containerd','kubelet'):
    if subprocess.run(['systemctl','is-active','--quiet',service],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0: errors.append('已有运行中的 '+service+'，请使用干净的目标主机')
for target in set(['/var/lib/containerd',p['runtimeDirectory']]):
    if target and os.path.isdir(target) and os.listdir(target): errors.append('运行时目录非空：'+target)
addresses=[]; networks=[]
try:
    interfaces=json.loads(subprocess.check_output(['ip','-j','-4','address'],text=True))
    import ipaddress
    for interface in interfaces:
        for addr in interface.get('addr_info',[]):
            if interface['ifname']==p['interface']: addresses.append(addr['local'])
            if addr.get('scope')=='global': networks.append(str(ipaddress.ip_network(addr['local']+'/'+str(addr['prefixlen']),strict=False)))
    if p['ip'] not in addresses: errors.append('部署 IP 不属于指定网口 '+p['interface'])
    routes=json.loads(subprocess.check_output(['ip','-j','-4','route'],text=True))
    networks+= [r['dst'] for r in routes if r.get('dst') and r['dst']!='default']
    if p['master'] and p['builtinLB']:
        own=[ipaddress.ip_network(a['local']+'/'+str(a['prefixlen']),strict=False) for interface in interfaces if interface['ifname']==p['interface'] for a in interface.get('addr_info',[])]
        if not any(ipaddress.ip_address(p['vip']) in subnet for subnet in own): errors.append('VIP 必须属于控制平面网口所在子网')
        if subprocess.run(['ping','-c','1','-W','2',p['vip']],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0: errors.append('VIP 已有 ICMP 响应，请确认未被占用')
except Exception: errors.append('无法读取网口及路由信息')
if os.path.exists('/sys/fs/cgroup/cgroup.controllers') is False: errors.append('当前模板要求 cgroup v2')
try:
    import socket
    ports=[10250]+([6443,2379,2380] if p['master'] else [])+([p['apiPort']] if p['master'] and p['builtinLB'] else [])
    for port in ports:
        sock=socket.socket()
        try: sock.bind(('0.0.0.0',port))
        except OSError: errors.append('端口已被占用：'+str(port))
        finally: sock.close()
except Exception: errors.append('端口检查失败')
minor='.'.join(p['version'].split('.')[:2])
try:
    with urllib.request.urlopen('https://pkgs.k8s.io/core:/stable:/v'+minor+'/deb/Packages.gz',timeout=20) as r: packages=gzip.decompress(r.read(16*1024*1024)).decode()
    if not any(line.startswith('Version: '+p['version']+'-') for line in packages.splitlines()): errors.append('官方软件源未找到 Kubernetes '+p['version'])
except Exception: errors.append('无法访问 Kubernetes 软件源，请检查出网及代理')
urls=['https://raw.githubusercontent.com/projectcalico/calico/v3.31.2/manifests/calico.yaml' if p['plugin']=='calico' else 'https://raw.githubusercontent.com/flannel-io/flannel/v0.27.4/Documentation/kube-flannel.yml']
if p['metrics']: urls.append('https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.0/components.yaml')
for url in urls:
    try:
        with urllib.request.urlopen(url,timeout=15) as response: response.read(128)
    except Exception: errors.append('无法访问组件安装文件：'+url)
try:
    req=urllib.request.Request('https://'+p['repository'].split('/')[0]+'/v2/',method='GET')
    try: urllib.request.urlopen(req,timeout=15).close()
    except urllib.error.HTTPError as e:
        if e.code!=401: raise
except Exception: errors.append('无法访问镜像仓库；组件镜像是否存在将在拉取阶段验证')
if not os.path.exists('/run/systemd/timesync/synchronized'):
    try:
        synchronized=subprocess.check_output(['timedatectl','show','--property=NTPSynchronized','--value'],text=True).strip()
        if synchronized!='yes': errors.append('系统时间尚未同步，请先启用 NTP')
    except Exception: errors.append('无法确认系统时间同步')
machine=open('/etc/machine-id').read().strip() if os.path.exists('/etc/machine-id') else ''
print(json.dumps(dict(os=system,version=version,arch=arch,hostname=platform.node().split('.')[0],machineId=machine,cpu=cpu,memoryMB=memory,diskGB=disk,addresses=addresses,networks=networks,errors=errors)))`

func (s *Service) installK8sDeploy(run model.K8sDeployRun, hosts []model.AssetHost) error {
	c := run.Config
	for _, h := range hosts {
		n := deployNodeForHost(run.Nodes, h.ID)
		if err := s.deployStep(run, h, "系统准备与组件安装", true, func(log func(string)) error {
			_, err := s.deployCommand(h, deployPrepareScript(c, n), 1800, log)
			return err
		}); err != nil {
			return err
		}
	}
	masters := []model.AssetHost{}
	workers := []model.AssetHost{}
	for _, n := range run.Nodes {
		for _, h := range hosts {
			if h.ID == n.HostID {
				if n.Role == "master" {
					masters = append(masters, h)
				} else {
					workers = append(workers, h)
				}
				break
			}
		}
	}
	if c.BuiltinLB {
		for i, h := range masters {
			if err := s.deployStep(run, h, "配置高可用入口", true, func(log func(string)) error {
				_, err := s.deployCommand(h, deployLBScript(c, run.Nodes, i), 300, log)
				return err
			}); err != nil {
				return err
			}
		}
	}
	first := masters[0]
	firstNode := deployNodeForHost(run.Nodes, first.ID)
	endpoint := c.APIServer
	if endpoint == "" {
		endpoint = firstNode.IP
	}
	endpoint = net.JoinHostPort(endpoint, fmt.Sprint(c.APIPort))
	if err := s.deployStep(run, first, "初始化控制平面", true, func(log func(string)) error {
		config, err := deployInitYAML(c, firstNode, endpoint)
		if err != nil {
			return err
		}
		// init output can contain bootstrap tokens; deliberately do not stream it.
		_, err = s.deployCommand(first, deployWriteFile("/etc/kubernetes/ops-admin-init.yaml", config)+"\nkubeadm init --config /etc/kubernetes/ops-admin-init.yaml --skip-token-print\n", 900, nil)
		if err == nil {
			log("控制平面初始化完成，敏感输出已隐藏。\n")
		}
		if err != nil {
			return errors.New("控制平面初始化失败，请核对节点 kubelet/containerd 服务；敏感输出已隐藏")
		}
		return nil
	}); err != nil {
		return err
	}
	var token, certificateKey, caHash string
	if err := s.deployStep(run, first, "准备节点加入凭据", true, func(log func(string)) error {
		out, err := s.deployCommand(first, "kubeadm token create --ttl 1h\n", 30, nil)
		if err != nil {
			return errors.New("创建加入 token 失败，敏感输出已隐藏")
		}
		token = strings.TrimSpace(out)
		out, err = s.deployCommand(first, "openssl x509 -pubkey -in /etc/kubernetes/pki/ca.crt | openssl pkey -pubin -outform DER 2>/dev/null | openssl dgst -sha256 -hex | awk '{print $NF}'", 30, nil)
		if err != nil {
			return errors.New("获取 CA 指纹失败")
		}
		caHash = strings.TrimSpace(out)
		if c.HA {
			out, err = s.deployCommand(first, "kubeadm init phase upload-certs --upload-certs\n", 60, nil)
			if err != nil {
				return errors.New("上传控制平面证书失败，敏感输出已隐藏")
			}
			lines := strings.Fields(out)
			if len(lines) == 0 {
				return errors.New("未获得证书加入密钥")
			}
			certificateKey = lines[len(lines)-1]
		}
		log("临时加入凭据已生成，有效期一小时。\n")
		return nil
	}); err != nil {
		return err
	}
	if err := s.deployStep(run, first, "安装容器网络", true, func(log func(string)) error {
		_, err := s.deployCommand(first, deployCNIScript(c), 600, log)
		return err
	}); err != nil {
		return err
	}
	joining := append(append([]model.AssetHost{}, masters[1:]...), workers...)
	for _, h := range joining {
		n := deployNodeForHost(run.Nodes, h.ID)
		if err := s.deployStep(run, h, "加入集群", true, func(log func(string)) error {
			config, err := deployJoinYAML(c, n, endpoint, token, caHash, certificateKey)
			if err != nil {
				return err
			}
			_, err = s.deployCommand(h, deployWriteFile("/etc/kubernetes/ops-admin-join.yaml", config)+"\ntrap 'rm -f /etc/kubernetes/ops-admin-join.yaml' EXIT\nkubeadm join --config /etc/kubernetes/ops-admin-join.yaml\n", 900, nil)
			if err != nil {
				return errors.New("节点加入失败，请核对节点服务与集群事件；加入凭据和敏感输出已隐藏")
			}
			log("节点加入完成。\n")
			return nil
		}); err != nil {
			return err
		}
	}
	if c.AllowControlPlaneWorkloads {
		if err := s.deployStep(run, first, "配置控制平面调度", true, func(log func(string)) error {
			_, err := s.deployCommand(first, "export KUBECONFIG=/etc/kubernetes/admin.conf\nkubectl taint nodes --all node-role.kubernetes.io/control-plane- --overwrite\n", 60, log)
			return err
		}); err != nil {
			return err
		}
	}
	if c.MetricsServer {
		if err := s.deployStep(run, first, "安装 Metrics Server", true, func(log func(string)) error {
			_, err := s.deployCommand(first, "export KUBECONFIG=/etc/kubernetes/admin.conf\nkubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.0/components.yaml\n", 180, log)
			return err
		}); err != nil {
			return err
		}
	}
	if err := s.deployStep(run, first, "验证集群就绪", false, func(log func(string)) error {
		commands := "export KUBECONFIG=/etc/kubernetes/admin.conf\nkubectl wait --for=condition=Ready nodes --all --timeout=600s\nkubectl rollout status deployment/coredns -n kube-system --timeout=300s\n"
		if c.NetworkPlugin == "calico" {
			commands += "kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s\n"
		} else {
			commands += "kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s\n"
		}
		if c.MetricsServer {
			commands += "kubectl rollout status deployment/metrics-server -n kube-system --timeout=180s\n"
		}
		_, err := s.deployCommand(first, commands, 1500, log)
		return err
	}); err != nil {
		return err
	}
	return s.deployStep(run, first, "接入集群管理", false, func(log func(string)) error {
		out, err := s.deployCommand(first, "cat /etc/kubernetes/admin.conf", 30, nil)
		if err != nil {
			return errors.New("读取 kubeconfig 失败，敏感输出已隐藏")
		}
		// Use the stable endpoint, including for the initial control plane.
		var cfg map[string]any
		if err = yaml.Unmarshal([]byte(out), &cfg); err != nil {
			return errors.New("kubeconfig 格式无效")
		}
		if clusters, ok := cfg["clusters"].([]any); ok {
			for _, v := range clusters {
				if item, ok := v.(map[string]any); ok {
					if cluster, ok := item["cluster"].(map[string]any); ok {
						cluster["server"] = "https://" + endpoint
					}
				}
			}
		}
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		cipher, err := util.EncryptSecret(string(data))
		if err != nil {
			return err
		}
		if err = s.db.Model(&model.K8sDeployPlan{}).Where("id = ?", run.PlanID).Updates(map[string]any{"kube_config_cipher": cipher, "registration_status": "pending"}).Error; err != nil {
			return err
		}
		if _, err = s.RegisterK8sDeployCluster(run.PlanID); err != nil {
			log("Kubernetes 已安装成功，平台接入失败。可在详情页单独重试接入。\n")
			return &deployWarning{message: "集群已就绪，平台接入失败，请单独重试接入"}
		}
		log("已接入集群管理。\n")
		return nil
	})
}

func (s *Service) RegisterK8sDeployCluster(id uint) (uint, error) {
	plan, err := s.GetK8sDeployPlan(id)
	if err != nil {
		return 0, err
	}
	if plan.ClusterID > 0 {
		return plan.ClusterID, nil
	}
	if plan.KubeConfigCipher == "" {
		return 0, errors.New("尚未获得已安装集群的 kubeconfig")
	}
	// Serialize retries without holding a database transaction across a network probe.
	res := s.db.Model(&model.K8sDeployPlan{}).Where("id = ? AND cluster_id = 0 AND registration_status IN ?", id, []string{"pending", "failed"}).Updates(map[string]any{"registration_status": "registering", "registration_error": ""})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, errors.New("集群接入正在处理中，请刷新")
	}
	defer func() {
		if err != nil {
			_ = s.db.Model(&model.K8sDeployPlan{}).Where("id = ?", id).Updates(map[string]any{"registration_status": "failed", "registration_error": err.Error()}).Error
		}
	}()
	plain, err := util.DecryptSecret(plan.KubeConfigCipher)
	if err != nil {
		return 0, err
	}
	payload := model.K8sClusterPayload{Name: plan.Name, Env: plan.Env, Description: plan.Description, KubeConfig: "encrypted:v1:" + plan.KubeConfigCipher, ConnectionMode: "direct", Operator: plan.CreatedBy}
	// Reuse the first master's CMDB SSH gateway for API access when applicable.
	for _, n := range plan.Nodes {
		if n.Role == "master" {
			var h model.AssetHost
			if err = s.db.First(&h, n.HostID).Error; err != nil {
				return 0, err
			}
			if h.ConnectionMode == "gateway" && h.GatewayID != nil {
				payload.ConnectionMode = "gateway"
				payload.GatewayID = *h.GatewayID
			}
			break
		}
	}
	// Recover the narrow crash window between creation and linking, only for the same credential.
	var existing model.K8sCluster
	lookup := s.db.Where("name = ?", plan.Name).First(&existing).Error
	if lookup == nil {
		existingConfig := existing.KubeConfig
		if strings.HasPrefix(existingConfig, "encrypted:v1:") {
			existingConfig, err = util.DecryptSecret(strings.TrimPrefix(existingConfig, "encrypted:v1:"))
			if err != nil {
				return 0, err
			}
		}
		if existingConfig != plain {
			err = errors.New("同名集群已存在且凭据不同，请核对后处理")
			return 0, err
		}
	} else if !errors.Is(lookup, gorm.ErrRecordNotFound) {
		err = lookup
		return 0, err
	} else {
		existing, err = s.CreateK8sCluster(payload)
		if err != nil {
			return 0, err
		}
	}
	err = s.db.Model(&model.K8sDeployPlan{}).Where("id = ? AND cluster_id = 0", id).Updates(map[string]any{"cluster_id": existing.ID, "registration_status": "registered", "registration_error": ""}).Error
	return existing.ID, err
}

func deployWriteFile(filename, content string) string {
	return "mkdir -p " + shellQuote(pathDirectory(filename)) + "\n(umask 077; printf %s " + shellQuote(base64.StdEncoding.EncodeToString([]byte(content))) + " | base64 -d > " + shellQuote(filename) + ")\nchmod 600 " + shellQuote(filename) + "\n"
}
func pathDirectory(filename string) string {
	pos := strings.LastIndex(filename, "/")
	return filename[:pos]
}
