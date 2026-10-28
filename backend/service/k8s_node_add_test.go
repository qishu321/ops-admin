package service

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func nodeAddTestConfig() model.K8sNodeAddConfig {
	return model.K8sNodeAddConfig{ClusterID: 1, Nodes: []model.K8sNodeAddSelection{{HostID: 2, NodeName: "worker-2"}}, Labels: "workload=batch", Taints: "dedicated=batch:NoSchedule"}
}

func TestNodeAddRejectsUnsafeInputAndSystemLabels(t *testing.T) {
	good := nodeAddTestConfig()
	if e := validateNodeAdd(good); e != nil {
		t.Fatal(e)
	}
	for _, labels := range []string{"node-role.kubernetes.io/control-plane=", "kubernetes.io/hostname=other", "node-restriction.kubernetes.io/secure=true", "a=b\na=c", "bad key=x"} {
		bad := good
		bad.Labels = labels
		if validateNodeAdd(bad) == nil {
			t.Fatalf("accepted labels %q", labels)
		}
	}
	bad := good
	bad.Nodes = append(bad.Nodes, bad.Nodes[0])
	if validateNodeAdd(bad) == nil {
		t.Fatal("accepted duplicate host")
	}
	bad = good
	bad.Taints = "dedicated=x:Unexpected"
	if validateNodeAdd(bad) == nil {
		t.Fatal("accepted taint")
	}
	for _, v := range []string{"1.34.9", "1.31.0"} {
		if !nodeAddVersionSupported(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"1.30.9", "1.36.0", "1.34.9;id", "1.34.9-vendor", "v1.34.9"} {
		if nodeAddVersionSupported(v) {
			t.Fatal(v)
		}
	}
}

func TestNodeAddPinsPublicKeyHashAndRealMachineIdentity(t *testing.T) {
	ca := newTestCA(t)
	hash, e := nodeAddCAHash(ca.pem)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(ca.cert.RawSubjectPublicKeyInfo)
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatal("CA hash must pin SPKI")
	}
	if _, e = nodeAddCAHash([]byte("not a CA")); e == nil {
		t.Fatal("invalid CA accepted")
	}
	n := model.K8sDeployNode{HostID: 2, Name: "worker-2", IP: "192.0.2.2", Role: "worker"}
	p := deployProbe{OS: "ubuntu", Version: "24.04", Arch: "x86_64", MachineID: "new-host", Interface: "ens33"}
	n, e = nodeAddProbeIdentity(n, p, nil, false)
	if e != nil {
		t.Fatal(e)
	}
	changed := p
	changed.MachineID = "different"
	if _, e = nodeAddProbeIdentity(n, changed, nil, true); e == nil {
		t.Fatal("changed machine accepted")
	}
	if _, e = nodeAddProbeIdentity(n, p, []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: n.Name}}}, false); e == nil {
		t.Fatal("existing node name accepted")
	}
	node := corev1.Node{Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{MachineID: p.MachineID, KubeletVersion: "v1.34.9"}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: n.IP}}}}
	if e = nodeAddVerifyRegistered(node, n, "1.34.9"); e != nil {
		t.Fatal(e)
	}
	node.Status.NodeInfo.MachineID = ""
	if nodeAddVerifyRegistered(node, n, "1.34.9") == nil {
		t.Fatal("unreported machine treated as registered")
	}
}

func TestNodeAddCreatesExpiringTokenAndRevokesItEvenWhenJoinFails(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmtBool(fail), func(t *testing.T) {
			var secret corev1.Secret
			created, deleted := false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case "POST":
					if !strings.HasSuffix(r.URL.Path, "/kube-system/secrets") {
						t.Error(r.URL.Path)
					}
					if e := json.NewDecoder(r.Body).Decode(&secret); e != nil {
						t.Error(e)
					}
					secret.UID = "test-token-uid"
					created = true
					w.WriteHeader(201)
					_ = json.NewEncoder(w).Encode(secret)
				case "DELETE":
					var options metav1.DeleteOptions
					_ = json.NewDecoder(r.Body).Decode(&options)
					if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != secret.UID {
						t.Error("token revoke must bind UID")
					}
					deleted = true
					_ = json.NewEncoder(w).Encode(metav1.Status{Status: "Success"})
				default:
					t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()
			client, e := kubernetes.NewForConfig(&rest.Config{Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json", AcceptContentTypes: "application/json"}})
			if e != nil {
				t.Fatal(e)
			}
			c := model.K8sDeployConfig{Version: "1.34.9"}
			n := model.K8sDeployNode{Role: "worker", Name: "worker-2", IP: "192.0.2.2"}
			snap := &nodeAddCluster{Client: client, Endpoint: "192.0.2.1:6443", CAHash: strings.Repeat("a", 64)}
			executed := false
			e = executeNodeAddJoin(snap, model.K8sDeployRun{ID: 7, Config: c}, nodeAddTestConfig(), n, func(script string) error {
				executed = true
				if !created {
					t.Error("join before creating token")
				}
				if strings.Contains(script, "kubeadm init") || strings.Contains(script, "kubeadm reset") {
					t.Error("worker expansion must not initialize/reset")
				}
				matches := regexp.MustCompile(`printf %s '([^']+)'`).FindStringSubmatch(script)
				if len(matches) != 2 {
					t.Fatal("encoded join configuration missing")
				}
				body, _ := base64.StdEncoding.DecodeString(matches[1])
				var config map[string]any
				if yaml.Unmarshal(body, &config) != nil {
					t.Fatal("invalid YAML")
				}
				if _, ok := config["controlPlane"]; ok {
					t.Error("worker config expanded control plane")
				}
				reg := config["nodeRegistration"].(map[string]any)
				if reg["name"] != "worker-2" || len(reg["taints"].([]any)) != 1 {
					t.Error("node configuration not applied")
				}
				token := string(secret.Data["token-id"]) + "." + string(secret.Data["token-secret"])
				bootstrap := config["discovery"].(map[string]any)["bootstrapToken"].(map[string]any)
				if bootstrap["token"] != token {
					t.Error("bootstrap token mismatch")
				}
				if strings.Contains(script, token) {
					t.Error("literal token interpolated into shell")
				}
				if fail {
					return errors.New("sensitive failure " + token)
				}
				return nil
			}, func(message string) { t.Error(message) })
			if !executed || !deleted {
				t.Fatal("join and token revocation required")
			}
			if (e != nil) != fail {
				t.Fatal(e)
			}
			if e != nil && strings.Contains(e.Error(), string(secret.Data["token-secret"])) {
				t.Fatal("token leaked in error")
			}
			expires, e := time.Parse(time.RFC3339, string(secret.Data["expiration"]))
			if e != nil || time.Until(expires) > 21*time.Minute || time.Until(expires) < 18*time.Minute {
				t.Fatal("unbounded token lifetime")
			}
			if len(secret.Data["token-id"]) != 6 || len(secret.Data["token-secret"]) != 16 || secret.Type != corev1.SecretTypeBootstrapToken {
				t.Fatal("invalid bootstrap credential")
			}
		})
	}
}

func TestNodeAddAdminBoundaryBeforeDatabaseAccess(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		t.Fatal("unauthorized database access")
		return nil, nil, errors.New("denied")
	}}
	s := (&Service{db: systemDB(t, state)}).WithAccess(context.Background(), &access.Policy{UserID: 2})
	if _, e := s.K8sNodeAddContext(1); e == nil {
		t.Fatal("member accessed context")
	}
	if _, e := s.StartK8sNodeAdd(K8sNodeAddPayload{K8sNodeAddConfig: nodeAddTestConfig()}, "member"); e == nil {
		t.Fatal("member started execution")
	}
	if _, e := s.StopK8sNodeAdd(1, 1); e == nil {
		t.Fatal("member stopped execution")
	}
}

func TestNodeAddFinishPreservesUncertainHostOwnership(t *testing.T) {
	for _, mutated := range []bool{false, true} {
		t.Run(fmtBool(mutated), func(t *testing.T) {
			nodes, _ := json.Marshal([]model.K8sDeployNode{{HostID: 2, Name: "worker-2"}})
			state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
				if strings.Contains(q, "k8s_deploy_run") {
					return []string{"id", "status", "nodes"}, [][]driver.Value{{int64(4), "running", string(nodes)}}, nil
				}
				title := "节点预检"
				if mutated {
					title = "安装组件"
				}
				return []string{"host_id", "title", "status"}, [][]driver.Value{{int64(2), title, "failed"}}, nil
			}}
			s := &Service{db: systemDB(t, state)}
			s.finishK8sNodeAdd(model.K8sNodeAddTask{}, model.K8sDeployRun{ID: 4}, errors.New("connection lost"))
			deleted := false
			for _, q := range state.writes {
				if strings.Contains(q, "DELETE FROM `k8s_deploy_host_lock`") {
					deleted = true
				}
			}
			if deleted == mutated {
				t.Fatalf("mutated=%v deleted=%v", mutated, deleted)
			}
		})
	}
}

func TestNodeAddTaskProjectionDoesNotOfferBlindRetry(t *testing.T) {
	task := model.K8sNodeAddTask{Kind: "add"}
	run := model.K8sDeployRun{Status: "needs_review", Nodes: []model.K8sDeployNode{{HostID: 2}}}
	view := nodeAddTaskView(task, run, "test", []model.K8sDeployStep{{HostID: 2, Title: "安装组件", Status: "failed"}}, nil)
	node := view["nodes"].([]map[string]any)[0]
	if node["retryable"] != false || node["status"] != "needs_review" {
		t.Fatal("uncertain installation offered blind retry")
	}
	// A stop at the final audit boundary must preserve a Node whose identity and Ready were verified.
	run.Status = "stopped"
	view = nodeAddTaskView(task, run, "test", []model.K8sDeployStep{{HostID: 2, Title: "等待 Ready", Status: "success"}}, nil)
	node = view["nodes"].([]map[string]any)[0]
	if node["status"] != "success" || node["retryable"] != false {
		t.Fatal("verified node lost its successful result after stop")
	}
}
