package service

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

func validDeployFixture() (model.K8sDeployConfig, []model.K8sDeployNode) {
	return model.K8sDeployConfig{Version: "1.34.9", NetworkPlugin: "calico", PodCIDR: "172.30.0.0/16", ServiceCIDR: "10.254.0.0/16", Interface: "eth0", ImageRepository: "registry.k8s.io", APIPort: 6443, ProxyMode: "iptables", AllowControlPlaneWorkloads: true}, []model.K8sDeployNode{{HostID: 1, Role: "master", Name: "cp-1", IP: "192.0.2.10", Interface: "eth0", HostName: "asset-alias"}}
}

func TestDeployRejectsConflictingAndUnsafeConfigurations(t *testing.T) {
	cases := []struct {
		name   string
		change func(*model.K8sDeployConfig, *[]model.K8sDeployNode)
	}{
		{"overlapping CIDRs", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.ServiceCIDR = c.PodCIDR }},
		{"host bits", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.PodCIDR = "172.30.1.0/16" }},
		{"node in pod network", func(_ *model.K8sDeployConfig, n *[]model.K8sDeployNode) { (*n)[0].IP = "172.30.0.2" }},
		{"unsupported plugin", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.NetworkPlugin = "cilium" }},
		{"v prefix", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.Version = "v1.34.9" }},
		{"unsupported version", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.Version = "1.28.6" }},
		{"no control plane", func(_ *model.K8sDeployConfig, n *[]model.K8sDeployNode) { (*n)[0].Role = "worker" }},
		{"not enough control planes", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.HA = true; c.APIServer = "192.0.2.100" }},
		{"no scheduling capacity", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.AllowControlPlaneWorkloads = false }},
		{"sed metacharacter", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.RuntimeDirectory = "/data/dir|bad" }},
		{"relative runtime path", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.RuntimeDirectory = "../../data" }},
		{"shell interpolation", func(_ *model.K8sDeployConfig, n *[]model.K8sDeployNode) { (*n)[0].Interface = "$(id)" }},
		{"unsafe config override", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) {
			c.KubeletConfig = "authentication:\n  anonymous:\n    enabled: true"
		}},
		{"invalid maxPods", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.KubeletConfig = "maxPods: 1000" }},
		{"null kubelet map", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.KubeletConfig = "null" }},
		{"software source commands", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) {
			c.CustomSource = "deb https://example.test/ stable main; id"
		}},
		{"no entry forwarding", func(c *model.K8sDeployConfig, _ *[]model.K8sDeployNode) { c.APIPort = 8443 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, n := validDeployFixture()
			tc.change(&c, &n)
			if validateDeployConfig(c, n) == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
	c, n := validDeployFixture()
	if err := validateDeployConfig(c, n); err != nil {
		t.Fatal(err)
	}
	// CMDB aliases are not actual hostnames. Preserving hostnames resolves them by SSH.
	n[0].Name = "中文资产名称"
	if err := validateDeployConfig(c, n); err != nil {
		t.Fatal(err)
	}
	c.AutoHostname = true
	if validateDeployConfig(c, n) == nil {
		t.Fatal("automatic names must be valid DNS names")
	}
}

func TestDeployResolvesRealSystemAndBindsPrecheckToMachine(t *testing.T) {
	c, n := validDeployFixture()
	r := model.K8sDeployRun{Kind: "precheck", Config: c, Nodes: n}
	probes := map[uint]deployProbe{1: {OS: "ubuntu", Version: "24.04", Arch: "x86_64", Hostname: "actual-cp", MachineID: "machine-1"}}
	resolved, err := resolveDeployNodes(r, probes)
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].Name != "actual-cp" || resolved[0].OS != "ubuntu 24.04" || resolved[0].MachineID != "machine-1" {
		t.Fatalf("missing automatic detection: %+v", resolved)
	}
	r.Kind = "deploy"
	r.Nodes = resolved
	if _, err := resolveDeployNodes(r, probes); err != nil {
		t.Fatal(err)
	}
	p := probes[1]
	p.MachineID = "replaced-host"
	probes[1] = p
	if _, err := resolveDeployNodes(r, probes); err == nil {
		t.Fatal("a replaced machine must not inherit an old precheck")
	}
	p.MachineID = "machine-1"
	p.Version = "22.04"
	probes[1] = p
	if _, err := resolveDeployNodes(r, probes); err == nil {
		t.Fatal("changed OS must invalidate the precheck")
	}
}

func TestDeployInitAndJoinYAMLPreserveNetworkingAndRoles(t *testing.T) {
	c, n := validDeployFixture()
	c.CustomCertificates = true
	c.CertificateYears = 2
	c.CAYears = 10
	c.KubeletConfig = "maxPods: 120\nsystemReserved:\n  memory: '256Mi'"
	content, err := deployInitYAML(c, n[0], "192.0.2.100:8443")
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(content))
	docs := map[string]map[string]any{}
	for {
		var doc map[string]any
		err = decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		docs[doc["kind"].(string)] = doc
	}
	cluster := docs["ClusterConfiguration"]
	if cluster["controlPlaneEndpoint"] != "192.0.2.100:8443" || cluster["certificateValidityPeriod"] != "17520h" {
		t.Fatalf("bad cluster configuration: %+v", cluster)
	}
	if docs["KubeletConfiguration"]["maxPods"] != 120 {
		t.Fatal("custom kubelet configuration missing")
	}
	for _, role := range []string{"master", "worker"} {
		node := n[0]
		node.Role = role
		joined, err := deployJoinYAML(c, node, "192.0.2.100:8443", "secret-token", "ca-hash", "secret-cert-key")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if yaml.Unmarshal([]byte(joined), &doc) != nil {
			t.Fatal("invalid join YAML")
		}
		_, controlPlane := doc["controlPlane"]
		if controlPlane != (role == "master") {
			t.Fatal("incorrect join role")
		}
		if strings.Contains(joined, "unsafeSkipCAVerification") {
			t.Fatal("discovery must verify CA")
		}
	}
}

func TestDeployEncryptedClusterConfigIsUsableAndHidden(t *testing.T) {
	plain := "apiVersion: v1\nkind: Config\ncurrent-context: test\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\nclusters:\n- name: test\n  cluster:\n    server: https://192.0.2.10:6443\nusers:\n- name: test\n  user:\n    token: a-test-token\n"
	cipher, err := util.EncryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parseKubeConfig("encrypted:v1:" + cipher); err != nil {
		t.Fatal(err)
	}
	clientConfig, cleanup, err := (&Service{}).k8sRESTConfigForCluster(model.K8sCluster{KubeConfig: "encrypted:v1:" + cipher})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if clientConfig.Host != "https://192.0.2.10:6443" || clientConfig.BearerToken != "a-test-token" {
		t.Fatal("official Kubernetes client lost decrypted connection credentials")
	}
	data, _ := json.Marshal(model.K8sDeployPlan{KubeConfigCipher: cipher})
	if strings.Contains(string(data), cipher) || strings.Contains(string(data), "test-token") {
		t.Fatal("deployment responses must not expose cluster credentials")
	}
}

func TestDeployAdminBoundaryBeforeDatabaseAccess(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		t.Fatal("unauthorized deployment accessed database")
		return nil, nil, errors.New("denied")
	}}
	s := (&Service{db: systemDB(t, state)}).WithAccess(context.Background(), &access.Policy{UserID: 2})
	if _, err := s.ListK8sDeployPlans(1, 10, "", ""); err == nil {
		t.Fatal("member listed all deployment plans")
	}
	if _, err := s.SaveK8sDeployPlan(K8sDeployPayload{Name: "test"}, "member"); err == nil {
		t.Fatal("member saved deployment")
	}
	if _, err := s.StartK8sDeployRun(1, 1, "deploy", "member"); err == nil {
		t.Fatal("member started deployment")
	}
}

func TestDeployFinishRetainsLocksAfterMutation(t *testing.T) {
	for _, mutated := range []bool{false, true} {
		t.Run(fmtBool(mutated), func(t *testing.T) {
			state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
				if !strings.Contains(q, "k8s_deploy_run") {
					return nil, nil, errors.New("unexpected query")
				}
				return []string{"id", "plan_id", "status", "kind", "mutated"}, [][]driver.Value{{int64(4), int64(2), "running", "deploy", mutated}}, nil
			}}
			s := &Service{db: systemDB(t, state)}
			s.finishK8sDeploy(4, errors.New("connection lost"))
			deleted := false
			for _, q := range state.writes {
				if strings.Contains(q, "DELETE FROM `k8s_deploy_host_lock`") {
					deleted = true
				}
			}
			if deleted == mutated {
				t.Fatalf("mutated=%v, deleted ownership locks=%v", mutated, deleted)
			}
		})
	}
}

func TestDeployFinishCannotReviveAnInterruptedRun(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		return []string{"id", "plan_id", "status", "mutated"}, [][]driver.Value{{int64(4), int64(2), "needs_review", true}}, nil
	}}
	s := &Service{db: systemDB(t, state)}
	s.finishK8sDeploy(4, nil)
	if len(state.writes) > 0 {
		t.Fatal("a worker without execution ownership changed a recovered task")
	}
}

func TestDeployProbePythonParsesWithoutExecution(t *testing.T) {
	python, err := exec.LookPath("python3")
	if runtime.GOOS == "windows" || err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Skip("python unavailable for syntax-only validation")
	}
	command := exec.Command(python, "-I", "-c", "import ast,sys; ast.parse(sys.stdin.buffer.read())")
	command.Stdin = strings.NewReader(deployProbePython)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("probe syntax invalid: %s: %v", out, err)
	}
}
func fmtBool(b bool) string {
	if b {
		return "mutated"
	}
	return "read-only"
}

func TestDeployGeneratedShellParsesWithoutExecuting(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		candidate := "C:/Program Files/Git/bin/bash.exe"
		if _, e := os.Stat(candidate); e == nil {
			bash = candidate
			err = nil
		}
	}
	if err != nil {
		t.Skip("bash not available for syntax-only validation")
	}
	c, n := validDeployFixture()
	c.RuntimeDirectory = "/data/containerd"
	c.CustomSource = "deb https://example.test/ubuntu noble main"
	c.AutoHostname = true
	c.ProxyMode = "ipvs"
	c.DisableSELinux = true
	init, _ := deployInitYAML(c, n[0], "192.0.2.10:6443")
	scripts := []string{deployPrepareScript(c, n[0]), deployCNIScript(c), deployProbeScript(c, n[0]), deployWriteFile("/etc/kubernetes/ops-admin-init.yaml", init)}
	c.NetworkPlugin = "flannel"
	scripts = append(scripts, deployCNIScript(c))
	for i := 1; i < 3; i++ {
		node := n[0]
		node.HostID = uint(i + 1)
		node.IP = "192.0.2.1" + string(rune('0'+i))
		n = append(n, node)
	}
	c.BuiltinLB = true
	c.APIServer = "192.0.2.100"
	c.APIPort = 8443
	c.RouterID = 68
	scripts = append(scripts, deployLBScript(c, n, 0))
	for _, script := range scripts {
		command := exec.Command(bash, "-n")
		command.Stdin = strings.NewReader(deployRootCommand(script))
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("invalid shell syntax: %s\n%s", out, script)
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("sensitive-token"))
	file := deployWriteFile("/etc/kubernetes/test.yaml", "sensitive-token")
	if strings.Contains(file, "sensitive-token") || !strings.Contains(file, encoded) {
		t.Fatal("sensitive file content should not be interpolated into shell")
	}
	if strings.Contains(file, "install -d -m 700 '/etc/kubernetes'") {
		t.Fatal("writing a file must not change an existing parent directory's permissions")
	}
}
