package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/model"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ksyaml "sigs.k8s.io/yaml"
)

const k8sClusterConnectError = "集群连接失败，请检查 kubeconfig"

type kubeConfig struct {
	APIVersion     string `yaml:"apiVersion"`
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token                 string `yaml:"token"`
			Username              string `yaml:"username"`
			Password              string `yaml:"password"`
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

type kubeClusterRuntime struct {
	Server                string
	InsecureSkipTLSVerify bool
	CertificateAuthority  string
	Token                 string
	Username              string
	Password              string
	ClientCertificateData string
	ClientKeyData         string
}

type kubeVersionResponse struct {
	GitVersion string `json:"gitVersion"`
}

type kubeNodeListResponse struct {
	Items []kubeNode `json:"items"`
}

type kubeNamespaceListResponse struct {
	Items []kubeNamespace `json:"items"`
}

type kubePodListResponse struct {
	Items []kubePod `json:"items"`
}

type kubeServiceListResponse struct {
	Items []kubeService `json:"items"`
}

type kubeIngressListResponse struct {
	Items []kubeIngress `json:"items"`
}

type kubeConfigMapListResponse struct {
	Items []kubeConfigMap `json:"items"`
}

type kubeSecretListResponse struct {
	Items []kubeSecret `json:"items"`
}

type kubePVCListResponse struct {
	Items []kubePersistentVolumeClaim `json:"items"`
}

type kubePVListResponse struct {
	Items []kubePersistentVolume `json:"items"`
}

type kubeDeploymentListResponse struct {
	Items []kubeDeployment `json:"items"`
}

type kubeReplicaSetListResponse struct {
	Items []kubeReplicaSet `json:"items"`
}

type kubeStatefulSetListResponse struct {
	Items []kubeStatefulSet `json:"items"`
}

type kubeDaemonSetListResponse struct {
	Items []kubeDaemonSet `json:"items"`
}

type kubeJobListResponse struct {
	Items []kubeJob `json:"items"`
}

type kubeCronJobListResponse struct {
	Items []kubeCronJob `json:"items"`
}

type kubeEndpointListResponse struct {
	Items []kubeEndpoints `json:"items"`
}

type kubeMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp string            `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	OwnerReferences   []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"ownerReferences"`
}

type kubeNode struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Unschedulable bool     `json:"unschedulable"`
		PodCIDR       string   `json:"podCIDR"`
		PodCIDRs      []string `json:"podCIDRs"`
	} `json:"spec"`
	Status struct {
		NodeInfo struct {
			KubeletVersion          string `json:"kubeletVersion"`
			OSImage                 string `json:"osImage"`
			KernelVersion           string `json:"kernelVersion"`
			ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
			Architecture            string `json:"architecture"`
		} `json:"nodeInfo"`
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		Capacity    map[string]string `json:"capacity"`
		Allocatable map[string]string `json:"allocatable"`
	} `json:"status"`
}

type kubeEnvVar struct {
	Name      string         `json:"name"`
	Value     string         `json:"value"`
	ValueFrom map[string]any `json:"valueFrom"`
}

type kubeContainer struct {
	Name            string       `json:"name"`
	Image           string       `json:"image"`
	ImagePullPolicy string       `json:"imagePullPolicy"`
	Env             []kubeEnvVar `json:"env"`
	Resources       struct {
		Requests map[string]string `json:"requests"`
		Limits   map[string]string `json:"limits"`
	} `json:"resources"`
}

type kubePod struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		NodeName           string          `json:"nodeName"`
		ServiceAccountName string          `json:"serviceAccountName"`
		Containers         []kubeContainer `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		PodIP             string `json:"podIP"`
		HostIP            string `json:"hostIP"`
		QoSClass          string `json:"qosClass"`
		ContainerStatuses []struct {
			Name         string `json:"name"`
			RestartCount int    `json:"restartCount"`
			Ready        bool   `json:"ready"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type kubeReplicaSet struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Replicas *int           `json:"replicas"`
		Template map[string]any `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

type kubeNamespace struct {
	Metadata kubeMetadata `json:"metadata"`
	Status   struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

type kubeService struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Type         string            `json:"type"`
		ClusterIP    string            `json:"clusterIP"`
		ExternalName string            `json:"externalName"`
		ExternalIPs  []string          `json:"externalIPs"`
		Selector     map[string]string `json:"selector"`
		Ports        []struct {
			Name       string      `json:"name"`
			Port       int         `json:"port"`
			NodePort   int         `json:"nodePort"`
			Protocol   string      `json:"protocol"`
			TargetPort interface{} `json:"targetPort"`
		} `json:"ports"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

type kubeEndpoints struct {
	Metadata kubeMetadata `json:"metadata"`
	Subsets  []struct {
		Addresses         []struct{} `json:"addresses"`
		NotReadyAddresses []struct{} `json:"notReadyAddresses"`
	} `json:"subsets"`
}

type kubeIngress struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		IngressClassName string `json:"ingressClassName"`
		Rules            []struct {
			Host string `json:"host"`
			HTTP struct {
				Paths []struct {
					Path    string `json:"path"`
					Backend struct {
						Service struct {
							Name string `json:"name"`
							Port struct {
								Number int    `json:"number"`
								Name   string `json:"name"`
							} `json:"port"`
						} `json:"service"`
					} `json:"backend"`
				} `json:"paths"`
			} `json:"http"`
		} `json:"rules"`
		TLS []struct{} `json:"tls"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

type kubeConfigMap struct {
	Metadata kubeMetadata      `json:"metadata"`
	Data     map[string]string `json:"data"`
	Binary   map[string]string `json:"binaryData"`
}

type kubeSecret struct {
	Metadata kubeMetadata      `json:"metadata"`
	Type     string            `json:"type"`
	Data     map[string]string `json:"data"`
}

type kubePersistentVolumeClaim struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		StorageClassName string   `json:"storageClassName"`
		AccessModes      []string `json:"accessModes"`
		Resources        struct {
			Requests map[string]string `json:"requests"`
		} `json:"resources"`
	} `json:"spec"`
	Status struct {
		Phase    string            `json:"phase"`
		Capacity map[string]string `json:"capacity"`
	} `json:"status"`
}

type kubePersistentVolume struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		StorageClassName              string            `json:"storageClassName"`
		Capacity                      map[string]string `json:"capacity"`
		AccessModes                   []string          `json:"accessModes"`
		PersistentVolumeReclaimPolicy string            `json:"persistentVolumeReclaimPolicy"`
		HostPath                      *struct {
			Path string `json:"path"`
		} `json:"hostPath"`
		NFS *struct {
			Server string `json:"server"`
			Path   string `json:"path"`
		} `json:"nfs"`
	} `json:"spec"`
	Status struct {
		Phase    string            `json:"phase"`
		Capacity map[string]string `json:"capacity"`
	} `json:"status"`
}

type kubeDeployment struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []kubeContainer `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		UpdatedReplicas   int `json:"updatedReplicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

type kubeStatefulSet struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []kubeContainer `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		UpdatedReplicas   int `json:"updatedReplicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

type kubeDaemonSet struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []kubeContainer `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		UpdatedNumberScheduled int `json:"updatedNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		NumberAvailable        int `json:"numberAvailable"`
	} `json:"status"`
}

type kubeJob struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Completions *int `json:"completions"`
		Selector    *struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []kubeContainer `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		Active    int `json:"active"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		Ready     int `json:"ready"`
	} `json:"status"`
}

type kubeCronJob struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Schedule    string `json:"schedule"`
		Suspend     *bool  `json:"suspend"`
		JobTemplate struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []kubeContainer `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		} `json:"jobTemplate"`
	} `json:"spec"`
	Status struct {
		Active []struct{} `json:"active"`
	} `json:"status"`
}

type kubeIstioGatewayListResponse struct {
	Items []kubeIstioGateway `json:"items"`
}

type kubeIstioVirtualServiceListResponse struct {
	Items []kubeIstioVirtualService `json:"items"`
}

type kubeIstioDestinationRuleListResponse struct {
	Items []kubeIstioDestinationRule `json:"items"`
}

type kubeIstioServiceEntryListResponse struct {
	Items []kubeIstioServiceEntry `json:"items"`
}

type kubeGatewayAPIListResponse struct {
	Items []kubeGatewayAPI `json:"items"`
}

type kubeHTTPRouteListResponse struct {
	Items []kubeHTTPRoute `json:"items"`
}

type kubeIstioGateway struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Selector map[string]string `json:"selector"`
		Servers  []struct {
			Port struct {
				Name     string `json:"name"`
				Number   int    `json:"number"`
				Protocol string `json:"protocol"`
			} `json:"port"`
			Hosts []string `json:"hosts"`
		} `json:"servers"`
	} `json:"spec"`
}

type kubeIstioVirtualService struct {
	APIVersion string       `json:"apiVersion"`
	Metadata   kubeMetadata `json:"metadata"`
	Spec       struct {
		Hosts    []string `json:"hosts"`
		Gateways []string `json:"gateways"`
		HTTP     []struct {
			Match []struct {
				URI struct {
					Exact  string `json:"exact"`
					Prefix string `json:"prefix"`
				} `json:"uri"`
			} `json:"match"`
			Route []struct {
				Destination struct {
					Host   string `json:"host"`
					Subset string `json:"subset"`
					Port   struct {
						Number int `json:"number"`
					} `json:"port"`
				} `json:"destination"`
				Weight int `json:"weight"`
			} `json:"route"`
		} `json:"http"`
		TCP []struct {
			Route []struct {
				Destination struct {
					Host string `json:"host"`
					Port struct {
						Number int `json:"number"`
					} `json:"port"`
				} `json:"destination"`
			} `json:"route"`
		} `json:"tcp"`
	} `json:"spec"`
}

type kubeIstioDestinationRule struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Host    string `json:"host"`
		Subsets []struct {
			Name string `json:"name"`
		} `json:"subsets"`
	} `json:"spec"`
}

type kubeIstioServiceEntry struct {
	Metadata kubeMetadata `json:"metadata"`
	Spec     struct {
		Hosts      []string `json:"hosts"`
		Addresses  []string `json:"addresses"`
		Location   string   `json:"location"`
		Resolution string   `json:"resolution"`
		Ports      []struct {
			Name     string `json:"name"`
			Number   int    `json:"number"`
			Protocol string `json:"protocol"`
		} `json:"ports"`
	} `json:"spec"`
}

type kubeGatewayAPI struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   kubeMetadata `json:"metadata"`
	Spec       struct {
		GatewayClassName string `json:"gatewayClassName"`
		Listeners        []struct {
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
		} `json:"listeners"`
	} `json:"spec"`
	Status struct {
		Addresses []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"addresses"`
	} `json:"status"`
}

type kubeHTTPRoute struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   kubeMetadata `json:"metadata"`
	Spec       struct {
		Hostnames  []string `json:"hostnames"`
		ParentRefs []struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"parentRefs"`
		Rules []struct {
			Matches []struct {
				Path struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"path"`
			} `json:"matches"`
			BackendRefs []struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Port      int    `json:"port"`
				Weight    int    `json:"weight"`
			} `json:"backendRefs"`
		} `json:"rules"`
	} `json:"spec"`
}

type k8sClusterProbe struct {
	APIServer string
	Version   string
	NodeCount int
	Status    string
}

type k8sFetchedData struct {
	Nodes                 []kubeNode
	Namespaces            []kubeNamespace
	Pods                  []kubePod
	Services              []kubeService
	Endpoints             []kubeEndpoints
	Ingresses             []kubeIngress
	ConfigMaps            []kubeConfigMap
	Secrets               []kubeSecret
	PVCs                  []kubePersistentVolumeClaim
	PVs                   []kubePersistentVolume
	Deployments           []kubeDeployment
	ReplicaSets           []kubeReplicaSet
	StatefulSet           []kubeStatefulSet
	DaemonSets            []kubeDaemonSet
	Jobs                  []kubeJob
	CronJobs              []kubeCronJob
	GatewayAPIGateways    []kubeGatewayAPI
	HTTPRoutes            []kubeHTTPRoute
	IstioGateways         []kubeIstioGateway
	IstioVirtualServices  []kubeIstioVirtualService
	IstioDestinationRules []kubeIstioDestinationRule
	IstioServiceEntries   []kubeIstioServiceEntry
}

type k8sAggregateMetrics struct {
	TotalAllocCPUMilli    int64
	TotalAllocMemoryBytes int64
	TotalReqCPUMilli      int64
	TotalReqMemoryBytes   int64
	AlertCount            int
}

type k8sManifestIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

func (s *Service) ListK8sClusters() ([]model.K8sClusterView, error) {
	var list []model.K8sCluster
	if err := s.db.Preload("Gateway").Preload("MonitorDatasource").Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}

	result := make([]model.K8sClusterView, 0, len(list))
	for _, item := range list {
		result = append(result, toK8sClusterView(item))
	}
	return result, nil
}

func (s *Service) GetK8sCluster(id uint) (model.K8sCluster, error) {
	var cluster model.K8sCluster
	if err := s.db.Preload("Gateway").Preload("MonitorDatasource").First(&cluster, id).Error; err != nil {
		return cluster, err
	}
	return cluster, nil
}

func (s *Service) CreateK8sCluster(payload model.K8sClusterPayload) (model.K8sCluster, error) {
	monitorDatasourceID, err := s.resolveK8sMonitorDatasource(payload.MonitorDatasourceID)
	if err != nil {
		return model.K8sCluster{}, err
	}
	cluster := model.K8sCluster{
		Name:                strings.TrimSpace(payload.Name),
		Description:         strings.TrimSpace(payload.Description),
		KubeConfig:          strings.TrimSpace(payload.KubeConfig),
		Env:                 normalizeEnvCode(payload.Env),
		Tags:                normalizeAssetTags(payload.Tags),
		ConnectionMode:      normalizeConnectionMode(payload.ConnectionMode),
		GatewayID:           optionalGatewayID(payload.ConnectionMode, payload.GatewayID),
		MonitorDatasourceID: monitorDatasourceID,
	}
	if err := validateK8sClusterPayload(cluster); err != nil {
		return cluster, err
	}

	var count int64
	if err := s.db.Model(&model.K8sCluster{}).Where("name = ?", cluster.Name).Count(&count).Error; err != nil {
		return cluster, err
	}
	if count > 0 {
		return cluster, errors.New("K8s 集群名称已存在")
	}

	probe, err := s.probeK8sCluster(cluster)
	if err != nil {
		return cluster, err
	}

	now := time.Now()
	cluster.APIServer = probe.APIServer
	cluster.Version = probe.Version
	cluster.NodeCount = probe.NodeCount
	cluster.Status = probe.Status
	cluster.LastSyncAt = &now

	if err := s.db.Create(&cluster).Error; err != nil {
		return cluster, err
	}
	s.recordAssetChange("k8s", cluster.ID, cluster.Name, "create", "新增 K8s 集群", payload.Operator)
	return cluster, nil
}

func (s *Service) UpdateK8sCluster(payload model.K8sClusterPayload) (model.K8sCluster, error) {
	cluster, err := s.GetK8sCluster(payload.ID)
	if err != nil {
		return cluster, err
	}

	cluster.Name = strings.TrimSpace(payload.Name)
	cluster.Description = strings.TrimSpace(payload.Description)
	cluster.KubeConfig = strings.TrimSpace(payload.KubeConfig)
	cluster.Env = normalizeEnvCode(payload.Env)
	cluster.Tags = normalizeAssetTags(payload.Tags)
	cluster.ConnectionMode = normalizeConnectionMode(payload.ConnectionMode)
	cluster.GatewayID = optionalGatewayID(payload.ConnectionMode, payload.GatewayID)
	monitorDatasourceID, err := s.resolveK8sMonitorDatasource(payload.MonitorDatasourceID)
	if err != nil {
		return cluster, err
	}
	cluster.MonitorDatasourceID = monitorDatasourceID

	if err := validateK8sClusterPayload(cluster); err != nil {
		return cluster, err
	}

	var count int64
	if err := s.db.Model(&model.K8sCluster{}).Where("name = ? AND id <> ?", cluster.Name, cluster.ID).Count(&count).Error; err != nil {
		return cluster, err
	}
	if count > 0 {
		return cluster, errors.New("K8s 集群名称已存在")
	}

	probe, err := s.probeK8sCluster(cluster)
	if err != nil {
		return cluster, err
	}

	now := time.Now()
	cluster.APIServer = probe.APIServer
	cluster.Version = probe.Version
	cluster.NodeCount = probe.NodeCount
	cluster.Status = probe.Status
	cluster.LastSyncAt = &now

	if err := s.db.Save(&cluster).Error; err != nil {
		return cluster, err
	}
	s.recordAssetChange("k8s", cluster.ID, cluster.Name, "update", "更新 K8s 集群配置并校验连接", payload.Operator)
	return cluster, nil
}

func (s *Service) DeleteK8sCluster(id uint) error {
	cluster, _ := s.GetK8sCluster(id)
	if err := s.db.Delete(&model.K8sCluster{}, id).Error; err != nil {
		return err
	}
	s.recordAssetChange("k8s", id, cluster.Name, "delete", "删除 K8s 集群", "system")
	return nil
}

const k8sOverviewCacheTTL = 15 * time.Second

func (s *Service) GetK8sClusterDetail(clusterID uint) (model.K8sClusterDetail, error) {
	if detail, ok := s.cachedK8sClusterDetail(clusterID); ok {
		return detail, nil
	}
	result, err, _ := s.k8sOverviewGroup.Do(fmt.Sprintf("cluster-overview:%d", clusterID), func() (any, error) {
		if detail, ok := s.cachedK8sClusterDetail(clusterID); ok {
			return detail, nil
		}
		detail, err := s.getK8sClusterDetailUncached(clusterID)
		if err != nil {
			return model.K8sClusterDetail{}, err
		}
		s.k8sOverviewMu.Lock()
		s.k8sOverviewCache[clusterID] = k8sOverviewCacheEntry{detail: detail, expiresAt: time.Now().Add(k8sOverviewCacheTTL)}
		s.k8sOverviewMu.Unlock()
		return detail, nil
	})
	if err != nil {
		return model.K8sClusterDetail{}, err
	}
	return result.(model.K8sClusterDetail), nil
}

func (s *Service) cachedK8sClusterDetail(clusterID uint) (model.K8sClusterDetail, bool) {
	s.k8sOverviewMu.Lock()
	defer s.k8sOverviewMu.Unlock()
	entry, ok := s.k8sOverviewCache[clusterID]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(s.k8sOverviewCache, clusterID)
		}
		return model.K8sClusterDetail{}, false
	}
	return entry.detail, true
}

func (s *Service) invalidateK8sClusterDetailCache(clusterID uint) {
	s.k8sOverviewMu.Lock()
	delete(s.k8sOverviewCache, clusterID)
	s.k8sOverviewMu.Unlock()
}

func (s *Service) getK8sClusterDetailUncached(clusterID uint) (model.K8sClusterDetail, error) {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.K8sClusterDetail{}, errors.New("k8s cluster not found")
		}
		return model.K8sClusterDetail{}, err
	}

	runtime, err := parseKubeConfig(cluster.KubeConfig)
	if err != nil {
		return model.K8sClusterDetail{}, fmt.Errorf("解析 kubeconfig 失败: %w", err)
	}
	// 集群详情必须复用与集群连接方式一致的客户端。对于 gateway 模式，
	// 该客户端会先连接网关，再由网关访问私网 Kubernetes API；不能在
	// Ops Admin 所在机器上直接拨号 kubeconfig 中的私网地址。
	client, cleanup, err := s.newK8sHTTPClientForCluster(cluster, runtime)
	if err != nil {
		return model.K8sClusterDetail{}, fmt.Errorf("创建 Kubernetes API 客户端失败: %w", err)
	}
	defer cleanup()

	data, err := fetchK8sData(client, runtime)
	if err != nil {
		route := "直连"
		if normalizeConnectionMode(cluster.ConnectionMode) == "gateway" {
			route = "网关转发"
		}
		return model.K8sClusterDetail{}, fmt.Errorf("通过%s获取 Kubernetes 集群详情失败: %w", route, err)
	}

	metrics := calculateK8sAggregateMetrics(data.Nodes, data.Pods)
	detailCluster := toK8sClusterView(cluster)
	if metrics.AlertCount > 0 {
		detailCluster.Status = "warning"
		detailCluster.StatusText = k8sStatusText("warning")
	}

	namespaceCounts := buildNamespaceCounts(data)
	endpointCounts := buildEndpointCounts(data.Endpoints)
	workloads := buildWorkloadItems(data)
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].Namespace == workloads[j].Namespace {
			return workloads[i].Name < workloads[j].Name
		}
		return workloads[i].Namespace < workloads[j].Namespace
	})

	return model.K8sClusterDetail{
		Cluster: detailCluster,
		Overview: model.K8sOverview{
			HealthScore:  calculateHealthScore(metrics.AlertCount),
			CPUUsage:     formatUsagePercent(metrics.TotalReqCPUMilli, metrics.TotalAllocCPUMilli),
			MemoryUsage:  formatUsagePercent(metrics.TotalReqMemoryBytes, metrics.TotalAllocMemoryBytes),
			PodUsage:     fmt.Sprintf("%d Pods", len(data.Pods)),
			RequestRate:  fmt.Sprintf("%d Workloads", len(workloads)),
			AlertCount:   metrics.AlertCount,
			Distribution: buildOverviewDistribution(detailCluster, data.Nodes, data.ConfigMaps),
			Certificates: buildOverviewCertificates(runtime),
		},
		Nodes:      buildNodeItems(data.Nodes, data.Pods),
		Namespaces: buildNamespaceItems(data.Namespaces, namespaceCounts),
		Pods:       buildPodItemsWithWorkloads(data),
		Workloads:  workloads,
		Network:    buildNetworkSection(data.Services, data.Ingresses, endpointCounts),
		AdvancedNetwork: buildAdvancedNetworkSection(
			data.GatewayAPIGateways,
			data.HTTPRoutes,
			data.Services,
		),
		ConfigStorage: buildConfigStorageSection(data.ConfigMaps, data.Secrets, data.PVCs, data.PVs),
	}, nil
}

func (s *Service) GetK8sNodeDetail(clusterID uint, nodeName string) (model.K8sNodeDetail, error) {
	cluster, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sNodeDetail{}, err
	}

	var node kubeNode
	if err := k8sGetJSON(client, runtime, "/api/v1/nodes/"+nodeName, &node); err != nil {
		return model.K8sNodeDetail{}, errors.New(k8sClusterConnectError)
	}

	pods, err := fetchPodsForNode(client, runtime, nodeName)
	if err != nil {
		return model.K8sNodeDetail{}, errors.New(k8sClusterConnectError)
	}

	_ = cluster
	return model.K8sNodeDetail{
		Name:           node.Metadata.Name,
		Status:         nodeReadyStatus(node),
		Roles:          joinNodeRoles(node.Metadata.Labels),
		Version:        fallbackText(node.Status.NodeInfo.KubeletVersion),
		InternalIP:     firstNodeInternalIP(node),
		OS:             fallbackText(node.Status.NodeInfo.OSImage),
		Kernel:         fallbackText(node.Status.NodeInfo.KernelVersion),
		ContainerRT:    fallbackText(node.Status.NodeInfo.ContainerRuntimeVersion),
		Architecture:   fallbackText(node.Status.NodeInfo.Architecture),
		Labels:         node.Metadata.Labels,
		CapacityCPU:    fallbackText(node.Status.Capacity["cpu"]),
		CapacityMem:    fallbackText(node.Status.Capacity["memory"]),
		AllocatableCPU: fallbackText(node.Status.Allocatable["cpu"]),
		AllocatableMem: fallbackText(node.Status.Allocatable["memory"]),
		Pods:           buildPodItems(pods),
	}, nil
}

func (s *Service) GetK8sNodePods(clusterID uint, nodeName string) ([]model.K8sPodItem, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return nil, err
	}
	pods, err := fetchPodsForNode(client, runtime, nodeName)
	if err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	return buildPodItems(pods), nil
}

// UpdateK8sNodeLabels applies the submitted label set to a Kubernetes node.
// Existing labels not included in the set are explicitly removed via a merge patch.
func (s *Service) UpdateK8sNodeLabels(payload model.K8sNodeLabelsPayload) error {
	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return err
	}
	var node kubeNode
	if err := k8sGetJSON(client, runtime, "/api/v1/nodes/"+url.PathEscape(payload.NodeName), &node); err != nil {
		return errors.New(k8sClusterConnectError)
	}
	labelsPatch := make(map[string]any, len(node.Metadata.Labels)+len(payload.Labels))
	for key := range node.Metadata.Labels {
		if _, keep := payload.Labels[key]; !keep {
			labelsPatch[key] = nil
		}
	}
	for key, value := range payload.Labels {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("节点标签键不能为空")
		}
		labelsPatch[key] = strings.TrimSpace(value)
	}
	if err := k8sPatchJSON(client, runtime, "/api/v1/nodes/"+url.PathEscape(payload.NodeName), map[string]any{
		"metadata": map[string]any{"labels": labelsPatch},
	}, "application/merge-patch+json", nil); err != nil {
		return errors.New(k8sClusterConnectError)
	}
	return nil
}

func (s *Service) GetK8sPodDetail(clusterID uint, namespace string, podName string) (model.K8sPodDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sPodDetail{}, err
	}

	var pod kubePod
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/pods/"+podName, &pod); err != nil {
		return model.K8sPodDetail{}, errors.New(k8sClusterConnectError)
	}

	containers := make([]model.K8sContainerItem, 0, len(pod.Spec.Containers))
	statusMap := make(map[string]struct {
		ready   bool
		restart int
	}, len(pod.Status.ContainerStatuses))
	for _, status := range pod.Status.ContainerStatuses {
		statusMap[status.Name] = struct {
			ready   bool
			restart int
		}{ready: status.Ready, restart: status.RestartCount}
	}
	for _, container := range pod.Spec.Containers {
		containerStatus := statusMap[container.Name]
		containers = append(containers, model.K8sContainerItem{
			Name:            container.Name,
			Image:           container.Image,
			Ready:           containerStatus.ready,
			Restart:         containerStatus.restart,
			RequestCPU:      container.Resources.Requests["cpu"],
			LimitCPU:        container.Resources.Limits["cpu"],
			RequestMemory:   container.Resources.Requests["memory"],
			LimitMemory:     container.Resources.Limits["memory"],
			ImagePullPolicy: container.ImagePullPolicy,
			Env:             buildContainerEnvItems(container.Env),
		})
	}

	return model.K8sPodDetail{
		Name:           pod.Metadata.Name,
		Namespace:      pod.Metadata.Namespace,
		Status:         fallbackText(pod.Status.Phase),
		Node:           fallbackText(pod.Spec.NodeName),
		PodIP:          fallbackText(pod.Status.PodIP),
		HostIP:         fallbackText(pod.Status.HostIP),
		QoSClass:       fallbackText(pod.Status.QoSClass),
		ServiceAccount: fallbackText(pod.Spec.ServiceAccountName),
		Labels:         pod.Metadata.Labels,
		Containers:     containers,
		CreatedAt:      formatTimestamp(pod.Metadata.CreationTimestamp),
		YAML:           marshalK8sYAML(pod),
	}, nil
}

// GetK8sPodMetrics returns the basic CPU and memory history for one Pod. The
// datasource stays server-side so kube pages never expose monitoring endpoint
// credentials or need to choose a datasource themselves.
func (s *Service) GetK8sPodMetrics(clusterID uint, namespace string, podName string, rangeKey string) (map[string]any, error) {
	if clusterID == 0 || strings.TrimSpace(namespace) == "" || strings.TrimSpace(podName) == "" {
		return nil, errors.New("invalid pod metrics query")
	}
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return nil, err
	}
	startAt, endAt, stepSeconds, normalizedRange := resolveK8sPodMetricRange(rangeKey)
	response := map[string]any{
		"status": "unavailable",
		"range":  map[string]any{"key": normalizedRange, "startAt": startAt.Unix(), "endAt": endAt.Unix(), "stepSeconds": stepSeconds},
		"metrics": map[string]any{
			"cpu":    map[string]any{"latest": nil, "points": []map[string]any{}},
			"memory": map[string]any{"latest": nil, "points": []map[string]any{}},
		},
	}

	if cluster.MonitorDatasourceID == nil || *cluster.MonitorDatasourceID == 0 {
		response["status"] = "not_configured"
		return response, nil
	}
	var datasource model.MonitorDatasource
	if err := s.db.First(&datasource, *cluster.MonitorDatasourceID).Error; err != nil {
		response["status"] = "not_configured"
		return response, nil
	}
	if datasource.Status != 1 || !isMonitorMetricDatasource(datasource.Type) {
		response["status"] = "not_configured"
		return response, nil
	}
	selector := fmt.Sprintf(`namespace=%q,pod=%q,container!="",container!="POD"`, namespace, podName)
	queries := map[string]string{
		"cpu":    fmt.Sprintf("sum(rate(container_cpu_usage_seconds_total{%s}[5m]))", selector),
		"memory": fmt.Sprintf("sum(container_memory_working_set_bytes{%s})", selector),
	}
	metrics := response["metrics"].(map[string]any)
	queryErrors := make(map[string]string)
	for name, query := range queries {
		result, err := s.prometheusRangeQuery(datasource, query, startAt, endAt, stepSeconds)
		if err != nil {
			queryErrors[name] = err.Error()
			continue
		}
		points := k8sPodMetricPoints(result)
		metrics[name] = map[string]any{"latest": k8sPodMetricLatest(points), "points": points}
	}
	if len(queryErrors) > 0 {
		response["errors"] = queryErrors
	}
	if len(queryErrors) == len(queries) {
		response["status"] = "query_failed"
	} else {
		response["status"] = "available"
	}
	return response, nil
}

// GetK8sWorkloadMetrics compares the runtime Pods that currently belong to a workload.
func (s *Service) GetK8sWorkloadMetrics(clusterID uint, namespace, workloadType, workloadName, rangeKey string) (map[string]any, error) {
	if clusterID == 0 || strings.TrimSpace(namespace) == "" || strings.TrimSpace(workloadType) == "" || strings.TrimSpace(workloadName) == "" {
		return nil, errors.New("invalid workload metrics query")
	}
	detail, err := s.GetK8sWorkloadDetail(clusterID, namespace, workloadType, workloadName)
	if err != nil {
		return nil, err
	}
	podNames := make([]string, 0, len(detail.Pods))
	for _, pod := range detail.Pods {
		podNames = append(podNames, pod.Name)
	}
	return s.getK8sPodMetricComparison(clusterID, namespace, podNames, rangeKey)
}

func (s *Service) getK8sPodMetricComparison(clusterID uint, namespace string, podNames []string, rangeKey string) (map[string]any, error) {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return nil, err
	}
	startAt, endAt, stepSeconds, normalizedRange := resolveK8sPodMetricRange(rangeKey)
	response := map[string]any{
		"status": "unavailable",
		"range":  map[string]any{"key": normalizedRange, "startAt": startAt.Unix(), "endAt": endAt.Unix(), "stepSeconds": stepSeconds},
		"metrics": map[string]any{
			"cpu":        map[string]any{"series": []map[string]any{}},
			"memory":     map[string]any{"series": []map[string]any{}},
			"wss":        map[string]any{"series": []map[string]any{}},
			"networkIn":  map[string]any{"series": []map[string]any{}},
			"networkOut": map[string]any{"series": []map[string]any{}},
		},
	}
	if cluster.MonitorDatasourceID == nil || *cluster.MonitorDatasourceID == 0 || len(podNames) == 0 {
		response["status"] = "not_configured"
		return response, nil
	}
	var datasource model.MonitorDatasource
	if err := s.db.First(&datasource, *cluster.MonitorDatasourceID).Error; err != nil || datasource.Status != 1 || !isMonitorMetricDatasource(datasource.Type) {
		response["status"] = "not_configured"
		return response, nil
	}
	quotedNames := make([]string, 0, len(podNames))
	for _, name := range podNames {
		if name = strings.TrimSpace(name); name != "" {
			quotedNames = append(quotedNames, regexp.QuoteMeta(name))
		}
	}
	if len(quotedNames) == 0 {
		return response, nil
	}
	selector := fmt.Sprintf(`namespace=%q,pod=~%q,container!="",container!="POD"`, namespace, "^("+strings.Join(quotedNames, "|")+")$")
	queries := map[string]string{
		"cpu":    fmt.Sprintf("sum by (pod) (rate(container_cpu_usage_seconds_total{%s}[5m]))", selector),
		"memory": fmt.Sprintf("sum by (pod) (container_memory_rss{%s})", selector),
		"wss": fmt.Sprintf(
			"100 * (sum by (pod) (container_memory_working_set_bytes{%s}) / on (pod) sum by (pod) (kube_pod_container_resource_limits{namespace=%q,pod=~%q,resource=\"memory\"}))",
			selector, namespace, "^("+strings.Join(quotedNames, "|")+")$",
		),
		"networkIn":  fmt.Sprintf("sum by (pod) (rate(container_network_receive_bytes_total{namespace=%q,pod=~%q}[5m]))", namespace, "^("+strings.Join(quotedNames, "|")+")$"),
		"networkOut": fmt.Sprintf("sum by (pod) (rate(container_network_transmit_bytes_total{namespace=%q,pod=~%q}[5m]))", namespace, "^("+strings.Join(quotedNames, "|")+")$"),
	}
	metrics := response["metrics"].(map[string]any)
	queryErrors := make(map[string]string)
	for name, query := range queries {
		result, queryErr := s.prometheusRangeQuery(datasource, query, startAt, endAt, stepSeconds)
		if queryErr != nil {
			queryErrors[name] = queryErr.Error()
			continue
		}
		metrics[name] = map[string]any{"series": k8sPodMetricSeries(result)}
	}
	if len(queryErrors) > 0 {
		response["errors"] = queryErrors
	}
	if len(queryErrors) == len(queries) {
		response["status"] = "query_failed"
	} else {
		response["status"] = "available"
	}
	return response, nil
}

func k8sPodMetricSeries(result *PromQueryResult) []map[string]any {
	series := make([]map[string]any, 0)
	if result == nil {
		return series
	}
	for _, sample := range result.Data.Result {
		points := make([]map[string]any, 0, len(sample.Values))
		for _, pair := range sample.Values {
			if len(pair) < 2 {
				continue
			}
			timestamp, timestampErr := strconv.ParseFloat(fmt.Sprint(pair[0]), 64)
			value, valueErr := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
			if timestampErr == nil && valueErr == nil {
				points = append(points, map[string]any{"timestamp": int64(timestamp * 1000), "value": value})
			}
		}
		if len(points) > 0 {
			series = append(series, map[string]any{"name": sample.Metric["pod"], "latest": points[len(points)-1]["value"], "points": points})
		}
	}
	sort.Slice(series, func(left, right int) bool {
		return fmt.Sprint(series[left]["name"]) < fmt.Sprint(series[right]["name"])
	})
	return series
}

func resolveK8sPodMetricRange(rangeKey string) (time.Time, time.Time, int, string) {
	definitions := map[string]struct {
		duration time.Duration
		step     int
	}{
		"1h":  {duration: time.Hour, step: 30},
		"6h":  {duration: 6 * time.Hour, step: 120},
		"24h": {duration: 24 * time.Hour, step: 300},
	}
	definition, ok := definitions[rangeKey]
	if !ok {
		rangeKey = "1h"
		definition = definitions[rangeKey]
	}
	endAt := time.Now()
	return endAt.Add(-definition.duration), endAt, definition.step, rangeKey
}

func k8sPodMetricPoints(result *PromQueryResult) []map[string]any {
	values := make(map[int64]float64)
	if result != nil {
		for _, sample := range result.Data.Result {
			for _, pair := range sample.Values {
				if len(pair) < 2 {
					continue
				}
				timestamp, timestampErr := strconv.ParseFloat(fmt.Sprint(pair[0]), 64)
				value, valueErr := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
				if timestampErr == nil && valueErr == nil {
					values[int64(timestamp*1000)] += value
				}
			}
		}
	}
	points := make([]map[string]any, 0, len(values))
	for timestamp, value := range values {
		points = append(points, map[string]any{"timestamp": timestamp, "value": value})
	}
	sort.Slice(points, func(left, right int) bool {
		return points[left]["timestamp"].(int64) < points[right]["timestamp"].(int64)
	})
	return points
}

func k8sPodMetricLatest(points []map[string]any) any {
	if len(points) == 0 {
		return nil
	}
	return points[len(points)-1]["value"]
}

func (s *Service) resolveK8sMonitorDatasource(datasourceID uint) (*uint, error) {
	if datasourceID == 0 {
		return nil, nil
	}
	var datasource model.MonitorDatasource
	if err := s.db.First(&datasource, datasourceID).Error; err != nil {
		return nil, errors.New("监控数据源不存在")
	}
	if datasource.Status != 1 || !isMonitorMetricDatasource(datasource.Type) {
		return nil, errors.New("请选择已启用的 Prometheus 或 VictoriaMetrics 数据源")
	}
	id := datasource.ID
	return &id, nil
}

func (s *Service) GetK8sPodLogs(clusterID uint, namespace string, podName string, container string, tailLines int) (map[string]any, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", namespace, podName)
	query := map[string]string{}
	if strings.TrimSpace(container) != "" {
		query["container"] = strings.TrimSpace(container)
	}
	if tailLines <= 0 {
		tailLines = 200
	}
	if tailLines > 1000 {
		tailLines = 1000
	}
	query["tailLines"] = strconv.Itoa(tailLines)
	body, err := k8sGetText(client, runtime, path, query)
	if err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	return map[string]any{
		"namespace": namespace,
		"podName":   podName,
		"container": container,
		"tailLines": tailLines,
		"content":   body,
	}, nil
}

func (s *Service) GetK8sPodEvents(clusterID uint, namespace string, podName string) ([]model.K8sEventItem, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return nil, err
	}

	return fetchNamespacedEvents(client, runtime, namespace, fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", podName))
}

func (s *Service) GetK8sNamespaceDetail(clusterID uint, namespace string) (model.K8sNamespaceDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sNamespaceDetail{}, err
	}

	var ns kubeNamespace
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace, &ns); err != nil {
		return model.K8sNamespaceDetail{}, errors.New(k8sClusterConnectError)
	}

	var pods kubePodListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/pods", &pods); err != nil {
		return model.K8sNamespaceDetail{}, errors.New(k8sClusterConnectError)
	}
	var services kubeServiceListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/services", &services); err != nil {
		return model.K8sNamespaceDetail{}, errors.New(k8sClusterConnectError)
	}
	var configMaps kubeConfigMapListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/configmaps", &configMaps); err != nil {
		return model.K8sNamespaceDetail{}, errors.New(k8sClusterConnectError)
	}
	var secrets kubeSecretListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/secrets", &secrets); err != nil {
		return model.K8sNamespaceDetail{}, errors.New(k8sClusterConnectError)
	}

	storageCount := 0
	var pvcs kubePVCListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/persistentvolumeclaims", &pvcs); err == nil {
		storageCount = len(pvcs.Items)
	}

	workloadCount := 0
	if count, err := fetchNamespaceWorkloadCount(client, runtime, namespace); err == nil {
		workloadCount = count
	}

	return model.K8sNamespaceDetail{
		Name:        ns.Metadata.Name,
		Status:      fallbackText(ns.Status.Phase),
		CreatedAt:   formatTimestamp(ns.Metadata.CreationTimestamp),
		Labels:      ns.Metadata.Labels,
		Annotations: ns.Metadata.Annotations,
		Pods:        len(pods.Items),
		Services:    len(services.Items),
		Workloads:   workloadCount,
		ConfigMaps:  len(configMaps.Items),
		Secrets:     len(secrets.Items),
		Storage:     storageCount,
		YAML:        marshalK8sYAML(ns),
	}, nil
}

func (s *Service) UpdateK8sResourceYAML(payload model.K8sResourceYAMLPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.ResourceType) == "" || Trimmed(payload.YAML) == "" {
		return nil, errors.New("invalid yaml payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	body, err := ksyaml.YAMLToJSON([]byte(payload.YAML))
	if err != nil {
		return nil, errors.New("invalid yaml content")
	}

	paths, err := buildK8sYAMLResourcePaths(payload)
	if err != nil {
		return nil, err
	}
	// Kubernetes PUT requires metadata.resourceVersion. Form editors submit a
	// concise manifest, so retrieve the current version server-side.
	var submitted map[string]any
	if err := json.Unmarshal(body, &submitted); err != nil {
		return nil, errors.New("invalid yaml content")
	}
	var updateErr error
	for _, path := range paths {
		var current map[string]any
		if err := k8sGetJSONWithQuery(client, runtime, path, nil, &current); err != nil {
			updateErr = err
			if isK8sNotFoundError(updateErr) {
				continue
			}
			break
		}
		if metadata, ok := current["metadata"].(map[string]any); ok {
			if version, ok := metadata["resourceVersion"].(string); ok && version != "" {
				if submittedMetadata, ok := submitted["metadata"].(map[string]any); ok {
					submittedMetadata["resourceVersion"] = version
				}
			}
		}
		requestBody, marshalErr := json.Marshal(submitted)
		if marshalErr != nil {
			return nil, errors.New("invalid yaml content")
		}
		updateErr = k8sDoJSON(client, runtime, http.MethodPut, path, nil, requestBody, "application/json", nil)
		if updateErr == nil {
			break
		}
		if !isK8sNotFoundError(updateErr) {
			break
		}
	}
	if updateErr != nil {
		return nil, friendlyK8sYAMLError(payload, updateErr)
	}
	return map[string]any{
		"resourceType": payload.ResourceType,
		"namespace":    payload.Namespace,
		"name":         payload.Name,
		"workloadType": payload.WorkloadType,
		"updated":      true,
	}, nil
}

func (s *Service) CreateK8sResourceYAML(payload model.K8sResourceYAMLPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.ResourceType) == "" || Trimmed(payload.YAML) == "" {
		return nil, errors.New("invalid yaml payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	body, err := ksyaml.YAMLToJSON([]byte(payload.YAML))
	if err != nil {
		return nil, errors.New("invalid yaml content")
	}

	manifest, err := parseK8sManifestIdentity(body)
	if err != nil {
		return nil, err
	}
	if Trimmed(payload.Name) == "" {
		payload.Name = manifest.Metadata.Name
	}
	if Trimmed(payload.Namespace) == "" {
		payload.Namespace = manifest.Metadata.Namespace
	}

	paths, err := buildK8sCreateResourcePaths(payload, manifest)
	if err != nil {
		return nil, err
	}
	if err := k8sDoJSONAnyPath(client, runtime, http.MethodPost, paths, nil, body, "application/json", nil); err != nil {
		return nil, friendlyK8sYAMLError(payload, err)
	}
	return map[string]any{
		"resourceType": payload.ResourceType,
		"namespace":    payload.Namespace,
		"name":         firstNonEmpty(payload.Name, manifest.Metadata.Name),
		"created":      true,
	}, nil
}

// CreateK8sWorkloadBundle creates the workload first, then its optional Service and
// Ingress. Kubernetes has no transaction across these APIs, so later failures roll
// back only the resources created by this request.
func (s *Service) CreateK8sWorkloadBundle(payload model.K8sWorkloadBundlePayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.Namespace) == "" || Trimmed(payload.WorkloadType) == "" || Trimmed(payload.WorkloadYAML) == "" {
		return nil, errors.New("invalid workload payload")
	}

	workload, err := validateBundleManifest(payload.WorkloadYAML, payload.Namespace, strings.ToLower(Trimmed(payload.WorkloadType)))
	if err != nil {
		return nil, fmt.Errorf("invalid workload: %w", err)
	}
	var service k8sManifestIdentity
	if Trimmed(payload.ServiceYAML) != "" {
		service, err = validateBundleManifest(payload.ServiceYAML, payload.Namespace, "service")
		if err != nil {
			return nil, fmt.Errorf("invalid service: %w", err)
		}
	}
	var ingress k8sManifestIdentity
	if Trimmed(payload.IngressYAML) != "" {
		if Trimmed(payload.ServiceYAML) == "" {
			return nil, errors.New("ingress requires a service in the same request")
		}
		ingress, err = validateBundleManifest(payload.IngressYAML, payload.Namespace, "ingress")
		if err != nil {
			return nil, fmt.Errorf("invalid ingress: %w", err)
		}
	}

	workloadResult, err := s.CreateK8sResourceYAML(model.K8sResourceYAMLPayload{
		ClusterID: payload.ClusterID, ResourceType: "workload", Namespace: payload.Namespace,
		Name: workload.Metadata.Name, WorkloadType: payload.WorkloadType, YAML: payload.WorkloadYAML,
	})
	if err != nil {
		return nil, err
	}
	rollbackWorkload := func() {
		_, _ = s.DeleteK8sResource(model.K8sResourceDeletePayload{ClusterID: payload.ClusterID, ResourceType: "workload", Namespace: payload.Namespace, Name: workload.Metadata.Name, WorkloadType: payload.WorkloadType})
	}
	result := map[string]any{"workload": workloadResult}

	if Trimmed(payload.ServiceYAML) != "" {
		serviceResult, createErr := s.CreateK8sResourceYAML(model.K8sResourceYAMLPayload{ClusterID: payload.ClusterID, ResourceType: "service", Namespace: payload.Namespace, Name: service.Metadata.Name, YAML: payload.ServiceYAML})
		if createErr != nil {
			rollbackWorkload()
			return nil, fmt.Errorf("service creation failed; workload was rolled back: %w", createErr)
		}
		result["service"] = serviceResult
		if Trimmed(payload.IngressYAML) != "" {
			ingressResult, createErr := s.CreateK8sResourceYAML(model.K8sResourceYAMLPayload{ClusterID: payload.ClusterID, ResourceType: "ingress", Namespace: payload.Namespace, Name: ingress.Metadata.Name, YAML: payload.IngressYAML})
			if createErr != nil {
				_, _ = s.DeleteK8sResource(model.K8sResourceDeletePayload{ClusterID: payload.ClusterID, ResourceType: "service", Namespace: payload.Namespace, Name: service.Metadata.Name})
				rollbackWorkload()
				return nil, fmt.Errorf("ingress creation failed; service and workload were rolled back: %w", createErr)
			}
			result["ingress"] = ingressResult
		}
	}
	return result, nil
}

func (s *Service) DeleteK8sResource(payload model.K8sResourceDeletePayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.ResourceType) == "" || Trimmed(payload.Name) == "" {
		return nil, errors.New("invalid delete payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	paths, err := buildK8sDeleteResourcePaths(payload)
	if err != nil {
		return nil, err
	}
	if err := k8sDoJSONAnyPath(client, runtime, http.MethodDelete, paths, nil, nil, "application/json", nil); err != nil {
		if isK8sNotFoundError(err) {
			return nil, errors.New("resource not found")
		}
		return nil, err
	}

	return map[string]any{
		"resourceType": payload.ResourceType,
		"namespace":    payload.Namespace,
		"name":         payload.Name,
		"deleted":      true,
	}, nil
}

func (s *Service) UpdateK8sIstioTraffic(payload model.K8sIstioTrafficPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.Namespace) == "" || Trimmed(payload.Name) == "" || len(payload.Routes) == 0 {
		return nil, errors.New("invalid istio traffic payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	var item kubeIstioVirtualService
	if err := k8sGetIstioJSON(client, runtime, "virtualservices", payload.Namespace, payload.Name, &item); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	httpIndex := firstVirtualServiceHTTPRouteIndex(item)
	if httpIndex < 0 {
		return nil, errors.New("virtualservice has no adjustable HTTP routes")
	}
	if len(item.Spec.HTTP[httpIndex].Route) != len(payload.Routes) {
		return nil, errors.New("virtualservice route count changed, please refresh and try again")
	}

	totalWeight := 0
	for index, route := range payload.Routes {
		if route.Weight < 0 {
			return nil, errors.New("traffic weight must be greater than or equal to 0")
		}
		totalWeight += route.Weight
		item.Spec.HTTP[httpIndex].Route[index].Weight = route.Weight
	}
	if totalWeight != 100 {
		return nil, errors.New("traffic weights must total 100")
	}

	body, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	paths := buildIstioResourcePathsWithPreferred("virtualservices", payload.Namespace, payload.Name, item.APIVersion)
	if err := k8sDoJSONAnyPath(client, runtime, http.MethodPut, paths, nil, body, "application/json", nil); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	return map[string]any{
		"namespace": payload.Namespace,
		"name":      payload.Name,
		"updated":   true,
	}, nil
}

func (s *Service) UpdateK8sHTTPRouteTraffic(payload model.K8sIstioTrafficPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || Trimmed(payload.Namespace) == "" || Trimmed(payload.Name) == "" || len(payload.Routes) == 0 {
		return nil, errors.New("invalid http route traffic payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	var item kubeHTTPRoute
	if err := k8sGetGatewayAPIJSON(client, runtime, "httproutes", payload.Namespace, payload.Name, &item); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	ruleIndex := firstHTTPRouteRuleIndex(item)
	if ruleIndex < 0 {
		return nil, errors.New("httproute has no adjustable backend refs")
	}
	if len(item.Spec.Rules[ruleIndex].BackendRefs) != len(payload.Routes) {
		return nil, errors.New("httproute backend refs changed, please refresh and try again")
	}

	totalWeight := 0
	for index, route := range payload.Routes {
		if route.Weight < 0 {
			return nil, errors.New("traffic weight must be greater than or equal to 0")
		}
		totalWeight += route.Weight
		item.Spec.Rules[ruleIndex].BackendRefs[index].Weight = route.Weight
	}
	if totalWeight != 100 {
		return nil, errors.New("traffic weights must total 100")
	}

	body, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	paths := buildGatewayAPIResourcePathsWithPreferred("httproutes", payload.Namespace, payload.Name, item.APIVersion)
	if err := k8sDoJSONAnyPath(client, runtime, http.MethodPut, paths, nil, body, "application/json", nil); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	return map[string]any{
		"namespace": payload.Namespace,
		"name":      payload.Name,
		"updated":   true,
	}, nil
}

func (s *Service) GetK8sNamespaceEvents(clusterID uint, namespace string) ([]model.K8sEventItem, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return nil, err
	}
	return fetchNamespacedEvents(client, runtime, namespace, fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Namespace", namespace))
}

func (s *Service) GetK8sWorkloadDetail(clusterID uint, namespace string, workloadType string, workloadName string) (model.K8sWorkloadDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sWorkloadDetail{}, err
	}

	typeName := strings.ToLower(strings.TrimSpace(workloadType))
	switch typeName {
	case "deployment":
		var item kubeDeployment
		path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, workloadName)
		if err := k8sGetJSON(client, runtime, path, &item); err != nil {
			return model.K8sWorkloadDetail{}, errors.New(k8sClusterConnectError)
		}
		return buildDeploymentDetail(client, runtime, item), nil
	case "statefulset":
		var item kubeStatefulSet
		path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", namespace, workloadName)
		if err := k8sGetJSON(client, runtime, path, &item); err != nil {
			return model.K8sWorkloadDetail{}, errors.New(k8sClusterConnectError)
		}
		return buildStatefulSetDetail(client, runtime, item), nil
	case "daemonset":
		var item kubeDaemonSet
		path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", namespace, workloadName)
		if err := k8sGetJSON(client, runtime, path, &item); err != nil {
			return model.K8sWorkloadDetail{}, errors.New(k8sClusterConnectError)
		}
		return buildDaemonSetDetail(client, runtime, item), nil
	case "job":
		var item kubeJob
		path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", namespace, workloadName)
		if err := k8sGetJSON(client, runtime, path, &item); err != nil {
			return model.K8sWorkloadDetail{}, errors.New(k8sClusterConnectError)
		}
		return buildJobDetail(client, runtime, item), nil
	case "cronjob":
		var item kubeCronJob
		path := fmt.Sprintf("/apis/batch/v1/namespaces/%s/cronjobs/%s", namespace, workloadName)
		if err := k8sGetJSON(client, runtime, path, &item); err != nil {
			return model.K8sWorkloadDetail{}, errors.New(k8sClusterConnectError)
		}
		return buildCronJobDetail(client, runtime, item), nil
	default:
		return model.K8sWorkloadDetail{}, errors.New("unsupported workload type")
	}
}

func (s *Service) ScaleK8sWorkload(payload model.K8sWorkloadActionPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || strings.TrimSpace(payload.Namespace) == "" || strings.TrimSpace(payload.WorkloadType) == "" || strings.TrimSpace(payload.WorkloadName) == "" {
		return nil, errors.New("invalid workload payload")
	}
	if payload.Replicas < 0 {
		return nil, errors.New("replicas must be greater than or equal to 0")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	workloadType := strings.ToLower(strings.TrimSpace(payload.WorkloadType))
	var path string
	switch workloadType {
	case "deployment":
		path = fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s/scale", payload.Namespace, payload.WorkloadName)
	case "statefulset":
		path = fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s/scale", payload.Namespace, payload.WorkloadName)
	default:
		return nil, errors.New("only deployment and statefulset support scaling")
	}

	patchBody := map[string]any{
		"spec": map[string]any{
			"replicas": payload.Replicas,
		},
	}
	if err := k8sPatchJSON(client, runtime, path, patchBody, "application/merge-patch+json", nil); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	s.invalidateK8sClusterDetailCache(payload.ClusterID)

	return map[string]any{
		"namespace":    payload.Namespace,
		"workloadType": payload.WorkloadType,
		"workloadName": payload.WorkloadName,
		"replicas":     payload.Replicas,
	}, nil
}

func (s *Service) RestartK8sWorkload(payload model.K8sWorkloadActionPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || strings.TrimSpace(payload.Namespace) == "" || strings.TrimSpace(payload.WorkloadType) == "" || strings.TrimSpace(payload.WorkloadName) == "" {
		return nil, errors.New("invalid workload payload")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	workloadType := strings.ToLower(strings.TrimSpace(payload.WorkloadType))
	var path string
	switch workloadType {
	case "deployment":
		path = fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", payload.Namespace, payload.WorkloadName)
	case "statefulset":
		path = fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", payload.Namespace, payload.WorkloadName)
	case "daemonset":
		path = fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", payload.Namespace, payload.WorkloadName)
	default:
		return nil, errors.New("only deployment, statefulset and daemonset support restart")
	}

	patchBody := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
	if err := k8sPatchJSON(client, runtime, path, patchBody, "application/strategic-merge-patch+json", nil); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	s.invalidateK8sClusterDetailCache(payload.ClusterID)
	return map[string]any{
		"namespace":    payload.Namespace,
		"workloadType": payload.WorkloadType,
		"workloadName": payload.WorkloadName,
		"restarted":    true,
	}, nil
}

func (s *Service) UpdateK8sWorkloadImages(payload model.K8sWorkloadImageBatchPayload) (map[string]any, error) {
	if payload.ClusterID == 0 {
		return nil, errors.New("invalid cluster payload")
	}
	version := strings.TrimSpace(payload.Version)
	if version == "" {
		return nil, errors.New("image version is required")
	}
	if len(payload.Items) == 0 {
		return nil, errors.New("please select workloads first")
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}

	updated := make([]map[string]any, 0, len(payload.Items))
	for _, item := range payload.Items {
		namespace := strings.TrimSpace(item.Namespace)
		workloadType := strings.ToLower(strings.TrimSpace(item.WorkloadType))
		workloadName := strings.TrimSpace(item.WorkloadName)
		if namespace == "" || workloadType == "" || workloadName == "" {
			return nil, errors.New("invalid workload item")
		}

		path, err := k8sWorkloadResourcePath(namespace, workloadType, workloadName)
		if err != nil {
			return nil, err
		}

		resource := map[string]any{}
		if err := k8sGetJSON(client, runtime, path, &resource); err != nil {
			return nil, errors.New(k8sClusterConnectError)
		}

		containers, err := extractWorkloadContainers(resource)
		if err != nil {
			return nil, err
		}

		patchedContainers := make([]map[string]any, 0, len(containers))
		images := make([]string, 0, len(containers))
		for _, container := range containers {
			name := strings.TrimSpace(anyToString(container["name"]))
			image := strings.TrimSpace(anyToString(container["image"]))
			if name == "" || image == "" {
				continue
			}
			nextImage := replaceImageVersion(image, version)
			patchedContainers = append(patchedContainers, map[string]any{
				"name":  name,
				"image": nextImage,
			})
			images = append(images, nextImage)
		}
		if len(patchedContainers) == 0 {
			return nil, errors.New("no containers found in selected workload")
		}

		patchBody := buildWorkloadImagePatchBody(patchedContainers)
		if err := k8sPatchJSON(client, runtime, path, patchBody, "application/strategic-merge-patch+json", nil); err != nil {
			return nil, errors.New(k8sClusterConnectError)
		}

		updated = append(updated, map[string]any{
			"namespace":    namespace,
			"workloadType": item.WorkloadType,
			"workloadName": workloadName,
			"images":       images,
		})
	}
	s.invalidateK8sClusterDetailCache(payload.ClusterID)
	return map[string]any{
		"version": payload.Version,
		"count":   len(updated),
		"items":   updated,
	}, nil
}

// UpdateK8sWorkloadResources updates the editable pod-template settings while preserving
// container image and command configuration: CPU/memory resources, environment variables
// and image pull policy.
func (s *Service) UpdateK8sWorkloadResources(payload model.K8sWorkloadResourcesPayload) (map[string]any, error) {
	if payload.ClusterID == 0 || strings.TrimSpace(payload.Namespace) == "" || strings.TrimSpace(payload.WorkloadType) == "" || strings.TrimSpace(payload.WorkloadName) == "" || len(payload.Containers) == 0 {
		return nil, errors.New("invalid workload resource payload")
	}
	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}
	path, err := k8sWorkloadResourcePath(payload.Namespace, payload.WorkloadType, payload.WorkloadName)
	if err != nil {
		return nil, err
	}
	resource := map[string]any{}
	if err := k8sGetJSON(client, runtime, path, &resource); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	existingContainers, err := extractWorkloadContainers(resource)
	if err != nil {
		return nil, err
	}
	existingByName := make(map[string]map[string]any, len(existingContainers))
	for _, container := range existingContainers {
		if name, ok := container["name"].(string); ok && strings.TrimSpace(name) != "" {
			existingByName[name] = container
		}
	}
	containers := make([]map[string]any, 0, len(payload.Containers))
	for _, item := range payload.Containers {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, errors.New("container name is required")
		}
		existing, ok := existingByName[name]
		if !ok {
			return nil, fmt.Errorf("container %s was not found in workload", name)
		}
		requests := map[string]any{}
		limits := map[string]any{}
		if value := strings.TrimSpace(item.RequestCPU); value != "" {
			requests["cpu"] = value
		}
		if value := strings.TrimSpace(item.RequestMemory); value != "" {
			requests["memory"] = value
		}
		if value := strings.TrimSpace(item.LimitCPU); value != "" {
			limits["cpu"] = value
		}
		if value := strings.TrimSpace(item.LimitMemory); value != "" {
			limits["memory"] = value
		}
		resources := map[string]any{}
		if len(requests) > 0 {
			resources["requests"] = requests
		}
		if len(limits) > 0 {
			resources["limits"] = limits
		}
		containerPatch := map[string]any{"name": name, "resources": resources}
		if policy := strings.TrimSpace(item.ImagePullPolicy); policy != "" {
			if policy != "Always" && policy != "IfNotPresent" && policy != "Never" {
				return nil, errors.New("invalid image pull policy")
			}
			containerPatch["imagePullPolicy"] = policy
		}
		envPatch, err := buildWorkloadEnvPatch(existing, item.Env)
		if err != nil {
			return nil, err
		}
		containerPatch["env"] = envPatch
		containers = append(containers, containerPatch)
	}
	patchBody := buildWorkloadContainerPatchBody(payload.WorkloadType, containers)
	if err := k8sPatchJSON(client, runtime, path, patchBody, "application/strategic-merge-patch+json", nil); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	s.invalidateK8sClusterDetailCache(payload.ClusterID)
	return map[string]any{
		"namespace": payload.Namespace, "workloadType": payload.WorkloadType,
		"workloadName": payload.WorkloadName, "containers": len(containers),
	}, nil
}

func (s *Service) GetK8sServiceDetail(clusterID uint, namespace string, serviceName string) (model.K8sServiceDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sServiceDetail{}, err
	}

	var service kubeService
	if err := k8sGetJSON(client, runtime, fmt.Sprintf("/api/v1/namespaces/%s/services/%s", namespace, serviceName), &service); err != nil {
		return model.K8sServiceDetail{}, errors.New(k8sClusterConnectError)
	}

	endpointCount := 0
	var endpoints kubeEndpoints
	if err := k8sGetJSON(client, runtime, fmt.Sprintf("/api/v1/namespaces/%s/endpoints/%s", namespace, serviceName), &endpoints); err == nil {
		for _, subset := range endpoints.Subsets {
			endpointCount += len(subset.Addresses)
		}
	}

	ports := make([]model.K8sKVTextItem, 0, len(service.Spec.Ports))
	portSpecs := make([]model.K8sServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		label := strconv.Itoa(port.Port)
		if strings.TrimSpace(port.Name) != "" {
			label = port.Name
		}
		target := stringifyTargetPort(port.TargetPort)
		if target == "" {
			target = strconv.Itoa(port.Port)
		}
		ports = append(ports, model.K8sKVTextItem{
			Label: label,
			Value: formatServiceDetailPort(port.Port, port.NodePort, port.Protocol, target),
		})
		protocol := strings.TrimSpace(port.Protocol)
		if protocol == "" {
			protocol = "TCP"
		}
		portSpecs = append(portSpecs, model.K8sServicePort{Name: port.Name, Protocol: protocol, Port: port.Port, TargetPort: target, NodePort: port.NodePort})
	}

	return model.K8sServiceDetail{
		Name:         service.Metadata.Name,
		Namespace:    service.Metadata.Namespace,
		Type:         serviceDisplayType(service),
		ClusterIP:    fallbackText(service.Spec.ClusterIP),
		ExternalIP:   serviceExternalIP(service),
		ExternalName: service.Spec.ExternalName,
		Ports:        ports,
		PortSpecs:    portSpecs,
		Selector:     service.Spec.Selector,
		Labels:       service.Metadata.Labels,
		Annotations:  service.Metadata.Annotations,
		Endpoints:    endpointCount,
		Age:          humanizeAge(service.Metadata.CreationTimestamp),
		YAML:         marshalK8sYAML(service),
	}, nil
}

func (s *Service) UpdateK8sService(payload model.K8sServiceUpdatePayload) (map[string]any, error) {
	payload.Namespace = strings.TrimSpace(payload.Namespace)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = strings.TrimSpace(payload.Type)
	if payload.ClusterID == 0 || payload.Namespace == "" || payload.Name == "" {
		return nil, errors.New("invalid service payload")
	}
	if payload.Type == "" {
		payload.Type = "ClusterIP"
	}
	allowedTypes := map[string]bool{"ClusterIP": true, "NodePort": true, "LoadBalancer": true, "ExternalName": true}
	if !allowedTypes[payload.Type] {
		return nil, errors.New("unsupported service type")
	}
	if payload.Headless && payload.Type != "ClusterIP" {
		return nil, errors.New("headless service must use ClusterIP")
	}
	if payload.Type == "ExternalName" && strings.TrimSpace(payload.ExternalName) == "" {
		return nil, errors.New("external name is required")
	}
	if payload.Type != "ExternalName" && len(payload.Ports) == 0 {
		return nil, errors.New("at least one service port is required")
	}
	for _, port := range payload.Ports {
		if port.Port < 1 || port.Port > 65535 {
			return nil, errors.New("service port must be between 1 and 65535")
		}
	}

	_, runtime, client, err := s.k8sClientForCluster(payload.ClusterID)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/services/%s", payload.Namespace, payload.Name)
	var current map[string]any
	if err := k8sGetJSON(client, runtime, path, &current); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}
	spec, ok := current["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid service resource")
	}
	spec["type"] = payload.Type
	if payload.Type == "ExternalName" {
		spec["externalName"] = strings.TrimSpace(payload.ExternalName)
		delete(spec, "selector")
		delete(spec, "clusterIP")
		delete(spec, "clusterIPs")
	} else {
		delete(spec, "externalName")
		selectors := make(map[string]any, len(payload.Selector))
		for key, value := range payload.Selector {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if key != "" && value != "" {
				selectors[key] = value
			}
		}
		spec["selector"] = selectors
		if payload.Headless {
			spec["clusterIP"] = "None"
			spec["clusterIPs"] = []string{"None"}
		}
	}
	ports := make([]map[string]any, 0, len(payload.Ports))
	for _, port := range payload.Ports {
		protocol := strings.TrimSpace(port.Protocol)
		if protocol == "" {
			protocol = "TCP"
		}
		item := map[string]any{"port": port.Port, "protocol": protocol, "targetPort": serviceTargetPort(port.TargetPort, port.Port)}
		if name := strings.TrimSpace(port.Name); name != "" {
			item["name"] = name
		}
		if payload.Type == "NodePort" || payload.Type == "LoadBalancer" {
			if port.NodePort > 0 {
				item["nodePort"] = port.NodePort
			}
		}
		ports = append(ports, item)
	}
	if payload.Type != "ExternalName" {
		spec["ports"] = ports
	}
	metadata, _ := current["metadata"].(map[string]any)
	labels := make(map[string]any, len(payload.Labels))
	for key, value := range payload.Labels {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key != "" && value != "" {
			labels[key] = value
		}
	}
	annotations := make(map[string]any, len(payload.Annotations))
	for key, value := range payload.Annotations {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key != "" && value != "" {
			annotations[key] = value
		}
	}
	request := map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": payload.Name, "namespace": payload.Namespace, "resourceVersion": metadata["resourceVersion"]}, "spec": spec}
	requestMetadata := request["metadata"].(map[string]any)
	requestMetadata["labels"] = labels
	requestMetadata["annotations"] = annotations
	if err := k8sPatchJSON(client, runtime, path, request, "application/merge-patch+json", nil); err != nil {
		return nil, err
	}
	return map[string]any{"name": payload.Name, "namespace": payload.Namespace}, nil
}

func serviceTargetPort(value string, fallback int) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if port, err := strconv.Atoi(value); err == nil {
		return port
	}
	return value
}

func (s *Service) GetK8sIstioResourceDetail(clusterID uint, resourceType string, namespace string, name string) (model.K8sIstioResourceDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sIstioResourceDetail{}, err
	}

	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	switch resourceType {
	case "gatewayapi":
		var item kubeGatewayAPI
		if err := k8sGetGatewayAPIJSON(client, runtime, "gateways", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "GatewayClass", Value: fallbackText(item.Spec.GatewayClassName)},
			{Label: "Hosts", Value: joinAndLimit(collectGatewayAPIHosts(item), 0)},
			{Label: "Address", Value: joinAndLimit(collectGatewayAPIAddresses(item), 0)},
			{Label: "Ports", Value: joinAndLimit(collectGatewayAPIPorts(item), 0)},
		}
		items := make([]model.K8sKVTextItem, 0, len(item.Spec.Listeners))
		for _, listener := range item.Spec.Listeners {
			label := firstNonEmpty(listener.Name, formatIstioPort(listener.Port, listener.Protocol))
			items = append(items, model.K8sKVTextItem{
				Label: label,
				Value: firstNonEmpty(listener.Hostname, "*"),
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "Gateway",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	case "httproute":
		var item kubeHTTPRoute
		if err := k8sGetGatewayAPIJSON(client, runtime, "httproutes", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "Hosts", Value: joinAndLimit(item.Spec.Hostnames, 0)},
			{Label: "Gateways", Value: joinAndLimit(collectHTTPRouteParents(item), 0)},
			{Label: "Targets", Value: joinAndLimit(collectHTTPRouteTargets(item), 0)},
		}
		items := make([]model.K8sKVTextItem, 0, len(item.Spec.Rules))
		for _, rule := range item.Spec.Rules {
			matchText := "/"
			if len(rule.Matches) > 0 {
				matchParts := make([]string, 0, len(rule.Matches))
				for _, match := range rule.Matches {
					if strings.TrimSpace(match.Path.Value) != "" {
						matchParts = append(matchParts, match.Path.Value)
					}
				}
				if len(matchParts) > 0 {
					matchText = strings.Join(matchParts, ", ")
				}
			}
			items = append(items, model.K8sKVTextItem{
				Label: matchText,
				Value: joinAndLimit(collectHTTPRouteTargetsFromRule(rule), 0),
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "HTTPRoute",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Traffic:     buildHTTPRouteTrafficItems(item),
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	case "gateway":
		var item kubeIstioGateway
		if err := k8sGetIstioJSON(client, runtime, "gateways", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "Selector", Value: joinSelector(item.Spec.Selector)},
			{Label: "Hosts", Value: joinAndLimit(flattenGatewayHosts(item), 0)},
			{Label: "Ports", Value: joinAndLimit(flattenGatewayPorts(item), 0)},
		}
		items := make([]model.K8sKVTextItem, 0, len(item.Spec.Servers))
		for _, server := range item.Spec.Servers {
			items = append(items, model.K8sKVTextItem{
				Label: formatIstioPort(server.Port.Number, server.Port.Protocol),
				Value: joinAndLimit(server.Hosts, 0),
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "Gateway",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	case "virtualservice":
		var item kubeIstioVirtualService
		if err := k8sGetIstioJSON(client, runtime, "virtualservices", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "Hosts", Value: joinAndLimit(item.Spec.Hosts, 0)},
			{Label: "Gateways", Value: joinAndLimit(item.Spec.Gateways, 0)},
			{Label: "Targets", Value: joinAndLimit(collectVirtualServiceTargets(item), 0)},
		}
		items := make([]model.K8sKVTextItem, 0)
		for _, httpRoute := range item.Spec.HTTP {
			routeTarget := make([]string, 0, len(httpRoute.Route))
			for _, route := range httpRoute.Route {
				target := route.Destination.Host
				if route.Destination.Subset != "" {
					target += ":" + route.Destination.Subset
				}
				if route.Destination.Port.Number > 0 {
					target += ":" + strconv.Itoa(route.Destination.Port.Number)
				}
				routeTarget = append(routeTarget, target)
			}
			matchText := "/"
			if len(httpRoute.Match) > 0 {
				matchParts := make([]string, 0, len(httpRoute.Match))
				for _, match := range httpRoute.Match {
					value := firstNonEmpty(match.URI.Exact, match.URI.Prefix)
					if value != "" {
						matchParts = append(matchParts, value)
					}
				}
				if len(matchParts) > 0 {
					matchText = strings.Join(matchParts, ", ")
				}
			}
			items = append(items, model.K8sKVTextItem{
				Label: matchText,
				Value: joinAndLimit(routeTarget, 0),
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "VirtualService",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Traffic:     buildVirtualServiceTrafficItems(item),
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	case "destinationrule":
		var item kubeIstioDestinationRule
		if err := k8sGetIstioJSON(client, runtime, "destinationrules", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "Host", Value: fallbackText(item.Spec.Host)},
			{Label: "Subsets", Value: strconv.Itoa(len(item.Spec.Subsets))},
		}
		items := make([]model.K8sKVTextItem, 0, len(item.Spec.Subsets))
		for _, subset := range item.Spec.Subsets {
			items = append(items, model.K8sKVTextItem{
				Label: subset.Name,
				Value: "subset",
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "DestinationRule",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	case "serviceentry":
		var item kubeIstioServiceEntry
		if err := k8sGetIstioJSON(client, runtime, "serviceentries", namespace, name, &item); err != nil {
			return model.K8sIstioResourceDetail{}, errors.New(k8sClusterConnectError)
		}
		summary := []model.K8sKVTextItem{
			{Label: "Hosts", Value: joinAndLimit(item.Spec.Hosts, 0)},
			{Label: "Addresses", Value: joinAndLimit(item.Spec.Addresses, 0)},
			{Label: "Resolution", Value: fallbackText(item.Spec.Resolution)},
		}
		items := make([]model.K8sKVTextItem, 0, len(item.Spec.Ports))
		for _, port := range item.Spec.Ports {
			label := firstNonEmpty(port.Name, strconv.Itoa(port.Number))
			items = append(items, model.K8sKVTextItem{
				Label: label,
				Value: formatIstioPort(port.Number, port.Protocol),
			})
		}
		return model.K8sIstioResourceDetail{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Kind:        "ServiceEntry",
			Labels:      item.Metadata.Labels,
			Annotations: item.Metadata.Annotations,
			Summary:     summary,
			Items:       items,
			Age:         humanizeAge(item.Metadata.CreationTimestamp),
			YAML:        marshalK8sYAML(item),
		}, nil
	default:
		return model.K8sIstioResourceDetail{}, errors.New("unsupported istio resource type")
	}
}

func (s *Service) GetK8sIngressDetail(clusterID uint, namespace string, ingressName string) (model.K8sIngressDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sIngressDetail{}, err
	}

	var ingress kubeIngress
	if err := k8sGetJSON(client, runtime, fmt.Sprintf("/apis/networking.k8s.io/v1/namespaces/%s/ingresses/%s", namespace, ingressName), &ingress); err != nil {
		return model.K8sIngressDetail{}, errors.New(k8sClusterConnectError)
	}

	address := "-"
	if len(ingress.Status.LoadBalancer.Ingress) > 0 {
		address = firstNonEmpty(ingress.Status.LoadBalancer.Ingress[0].IP, ingress.Status.LoadBalancer.Ingress[0].Hostname)
	}

	tls := "未启用"
	if len(ingress.Spec.TLS) > 0 {
		tls = "已启用"
	}

	hosts := make([]string, 0, len(ingress.Spec.Rules))
	rules := make([]model.K8sKVTextItem, 0)
	for _, rule := range ingress.Spec.Rules {
		if strings.TrimSpace(rule.Host) != "" {
			hosts = append(hosts, rule.Host)
		}
		if len(rule.HTTP.Paths) == 0 {
			rules = append(rules, model.K8sKVTextItem{
				Label: fallbackText(rule.Host),
				Value: "/",
			})
			continue
		}
		for _, path := range rule.HTTP.Paths {
			backendTarget := path.Backend.Service.Name
			portText := firstNonEmpty(path.Backend.Service.Port.Name, strconv.Itoa(path.Backend.Service.Port.Number))
			if backendTarget != "" && portText != "" && portText != "0" {
				backendTarget = backendTarget + ":" + portText
			}
			rules = append(rules, model.K8sKVTextItem{
				Label: fmt.Sprintf("%s %s", fallbackText(rule.Host), fallbackText(path.Path)),
				Value: fallbackText(backendTarget),
			})
		}
	}

	return model.K8sIngressDetail{
		Name:        ingress.Metadata.Name,
		Namespace:   ingress.Metadata.Namespace,
		Host:        fallbackText(strings.Join(hosts, ", ")),
		Address:     fallbackText(address),
		TLS:         tls,
		ClassName:   fallbackText(ingress.Spec.IngressClassName),
		Labels:      ingress.Metadata.Labels,
		Annotations: ingress.Metadata.Annotations,
		Rules:       rules,
		Age:         humanizeAge(ingress.Metadata.CreationTimestamp),
		YAML:        marshalK8sYAML(ingress),
	}, nil
}

func (s *Service) GetK8sConfigMapDetail(clusterID uint, namespace string, configMapName string) (model.K8sConfigMapDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sConfigMapDetail{}, err
	}

	var item kubeConfigMap
	if err := k8sGetJSON(client, runtime, fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", namespace, configMapName), &item); err != nil {
		return model.K8sConfigMapDetail{}, errors.New(k8sClusterConnectError)
	}

	keys := make([]model.K8sKVTextItem, 0, len(item.Data)+len(item.Binary))
	for key, value := range item.Data {
		keys = append(keys, model.K8sKVTextItem{Label: key, Value: value})
	}
	for key, value := range item.Binary {
		keys = append(keys, model.K8sKVTextItem{Label: key, Value: value})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Label < keys[j].Label })

	return model.K8sConfigMapDetail{
		Name:        item.Metadata.Name,
		Namespace:   item.Metadata.Namespace,
		Keys:        keys,
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		YAML:        marshalK8sYAML(item),
	}, nil
}

func (s *Service) GetK8sSecretDetail(clusterID uint, namespace string, secretName string) (model.K8sSecretDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sSecretDetail{}, err
	}

	var item kubeSecret
	if err := k8sGetJSON(client, runtime, fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, secretName), &item); err != nil {
		return model.K8sSecretDetail{}, errors.New(k8sClusterConnectError)
	}

	keys := make([]model.K8sKVTextItem, 0, len(item.Data))
	for key, value := range item.Data {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			decoded = []byte(value)
		}
		keys = append(keys, model.K8sKVTextItem{Label: key, Value: string(decoded), Sensitive: true})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Label < keys[j].Label })

	return model.K8sSecretDetail{
		Name:        item.Metadata.Name,
		Namespace:   item.Metadata.Namespace,
		Type:        fallbackText(item.Type),
		Keys:        keys,
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		YAML:        marshalK8sYAML(item),
	}, nil
}

func (s *Service) GetK8sStorageDetail(clusterID uint, kind string, namespace string, name string) (model.K8sStorageDetail, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return model.K8sStorageDetail{}, err
	}

	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "PVC":
		var item kubePersistentVolumeClaim
		if strings.TrimSpace(namespace) == "" {
			return model.K8sStorageDetail{}, errors.New("namespace is required for pvc")
		}
		if err := k8sGetJSON(client, runtime, fmt.Sprintf("/api/v1/namespaces/%s/persistentvolumeclaims/%s", namespace, name), &item); err != nil {
			return model.K8sStorageDetail{}, errors.New(k8sClusterConnectError)
		}
		return model.K8sStorageDetail{
			Name:      item.Metadata.Name,
			Kind:      "PVC",
			Namespace: fallbackText(item.Metadata.Namespace),
			Status:    fallbackText(item.Status.Phase),
			// PVC 列表与详情应展示用户声明的申请容量；绑定后 status.capacity
			// 表示实际绑定 PV 的容量，可能大于 PVC 请求容量。
			Capacity:     fallbackText(firstNonEmpty(item.Spec.Resources.Requests["storage"], item.Status.Capacity["storage"])),
			StorageClass: fallbackText(item.Spec.StorageClassName),
			AccessModes:  strings.Join(item.Spec.AccessModes, ", "),
			Labels:       item.Metadata.Labels,
			Annotations:  item.Metadata.Annotations,
			Age:          humanizeAge(item.Metadata.CreationTimestamp),
			YAML:         marshalK8sYAML(item),
		}, nil
	case "PV":
		var item kubePersistentVolume
		if err := k8sGetJSON(client, runtime, "/api/v1/persistentvolumes/"+name, &item); err != nil {
			return model.K8sStorageDetail{}, errors.New(k8sClusterConnectError)
		}
		sourceType, path, nfsServer := persistentVolumeSource(item)
		return model.K8sStorageDetail{
			Name:           item.Metadata.Name,
			Kind:           "PV",
			Namespace:      "集群级",
			NamespaceScope: storageNamespaceScope(item.Metadata.Annotations),
			Status:         fallbackText(item.Status.Phase),
			Capacity:       fallbackText(firstNonEmpty(item.Status.Capacity["storage"], item.Spec.Capacity["storage"])),
			StorageClass:   fallbackText(item.Spec.StorageClassName),
			SourceType:     sourceType,
			Path:           path,
			NFSServer:      nfsServer,
			AccessModes:    strings.Join(item.Spec.AccessModes, ", "),
			ReclaimPolicy:  fallbackText(item.Spec.PersistentVolumeReclaimPolicy),
			Labels:         item.Metadata.Labels,
			Annotations:    item.Metadata.Annotations,
			Age:            humanizeAge(item.Metadata.CreationTimestamp),
			YAML:           marshalK8sYAML(item),
		}, nil
	default:
		return model.K8sStorageDetail{}, errors.New("unsupported storage kind")
	}
}

func fetchK8sData(client *http.Client, runtime kubeClusterRuntime) (k8sFetchedData, error) {
	var data k8sFetchedData

	var nodeResp kubeNodeListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/nodes", &nodeResp); err != nil {
		return data, err
	}
	data.Nodes = nodeResp.Items

	var namespaceResp kubeNamespaceListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces", &namespaceResp); err != nil {
		return data, err
	}
	data.Namespaces = namespaceResp.Items

	var podResp kubePodListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/pods", &podResp); err != nil {
		return data, err
	}
	data.Pods = podResp.Items

	var serviceResp kubeServiceListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/services", &serviceResp); err != nil {
		return data, err
	}
	data.Services = serviceResp.Items

	var endpointResp kubeEndpointListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/endpoints", &endpointResp); err != nil {
		return data, err
	}
	data.Endpoints = endpointResp.Items

	var ingressResp kubeIngressListResponse
	if err := k8sGetJSON(client, runtime, "/apis/networking.k8s.io/v1/ingresses", &ingressResp); err == nil {
		data.Ingresses = ingressResp.Items
	}

	var gatewayAPIResp kubeGatewayAPIListResponse
	if err := k8sGetGatewayAPIJSON(client, runtime, "gateways", "", "", &gatewayAPIResp); err == nil {
		data.GatewayAPIGateways = gatewayAPIResp.Items
	}

	var httpRouteResp kubeHTTPRouteListResponse
	if err := k8sGetGatewayAPIJSON(client, runtime, "httproutes", "", "", &httpRouteResp); err == nil {
		data.HTTPRoutes = httpRouteResp.Items
	}

	var configMapResp kubeConfigMapListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/configmaps", &configMapResp); err != nil {
		return data, err
	}
	data.ConfigMaps = configMapResp.Items

	var secretResp kubeSecretListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/secrets", &secretResp); err != nil {
		return data, err
	}
	data.Secrets = secretResp.Items

	var pvcResp kubePVCListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/persistentvolumeclaims", &pvcResp); err == nil {
		data.PVCs = pvcResp.Items
	}

	var pvResp kubePVListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/persistentvolumes", &pvResp); err == nil {
		data.PVs = pvResp.Items
	}

	var deploymentResp kubeDeploymentListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/deployments", &deploymentResp); err == nil {
		data.Deployments = deploymentResp.Items
	}

	var replicaSetResp kubeReplicaSetListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/replicasets", &replicaSetResp); err == nil {
		data.ReplicaSets = replicaSetResp.Items
	}

	var statefulSetResp kubeStatefulSetListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/statefulsets", &statefulSetResp); err == nil {
		data.StatefulSet = statefulSetResp.Items
	}

	var daemonSetResp kubeDaemonSetListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/daemonsets", &daemonSetResp); err == nil {
		data.DaemonSets = daemonSetResp.Items
	}

	var jobResp kubeJobListResponse
	if err := k8sGetJSON(client, runtime, "/apis/batch/v1/jobs", &jobResp); err == nil {
		data.Jobs = jobResp.Items
	}

	var cronJobResp kubeCronJobListResponse
	if err := k8sGetJSON(client, runtime, "/apis/batch/v1/cronjobs", &cronJobResp); err == nil {
		data.CronJobs = cronJobResp.Items
	}

	return data, nil
}

func (s *Service) k8sClientForCluster(clusterID uint) (model.K8sCluster, kubeClusterRuntime, *http.Client, error) {
	cluster, err := s.GetK8sCluster(clusterID)
	if err != nil {
		return model.K8sCluster{}, kubeClusterRuntime{}, nil, err
	}
	runtime, err := parseKubeConfig(cluster.KubeConfig)
	if err != nil {
		return cluster, kubeClusterRuntime{}, nil, errors.New(k8sClusterConnectError)
	}
	client, cleanup, err := s.newK8sHTTPClientForCluster(cluster, runtime)
	if err != nil {
		return cluster, kubeClusterRuntime{}, nil, errors.New(k8sClusterConnectError)
	}
	_ = cleanup
	return cluster, runtime, client, nil
}

func fetchPodsForNode(client *http.Client, runtime kubeClusterRuntime, nodeName string) ([]kubePod, error) {
	var payload kubePodListResponse
	if err := k8sGetJSONWithQuery(client, runtime, "/api/v1/pods", map[string]string{
		"fieldSelector": "spec.nodeName=" + nodeName,
	}, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func fetchPodsByNamespace(client *http.Client, runtime kubeClusterRuntime, namespace string) ([]kubePod, error) {
	var payload kubePodListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/namespaces/"+namespace+"/pods", &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func fetchNamespacedEvents(client *http.Client, runtime kubeClusterRuntime, namespace string, fieldSelector string) ([]model.K8sEventItem, error) {
	var payload struct {
		Items []struct {
			Type      string `json:"type"`
			Reason    string `json:"reason"`
			Message   string `json:"message"`
			Count     int    `json:"count"`
			FirstTime string `json:"firstTimestamp"`
			LastTime  string `json:"lastTimestamp"`
		} `json:"items"`
	}
	if err := k8sGetJSONWithQuery(client, runtime, "/api/v1/namespaces/"+namespace+"/events", map[string]string{"fieldSelector": fieldSelector}, &payload); err != nil {
		return nil, errors.New(k8sClusterConnectError)
	}

	events := make([]model.K8sEventItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		events = append(events, model.K8sEventItem{
			Type:      fallbackText(item.Type),
			Reason:    fallbackText(item.Reason),
			Message:   fallbackText(item.Message),
			Count:     item.Count,
			FirstTime: formatTimestamp(item.FirstTime),
			LastTime:  formatTimestamp(item.LastTime),
		})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].LastTime > events[j].LastTime })
	return events, nil
}

func fetchNamespaceWorkloadCount(client *http.Client, runtime kubeClusterRuntime, namespace string) (int, error) {
	total := 0

	var deployments kubeDeploymentListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/namespaces/"+namespace+"/deployments", &deployments); err != nil {
		return 0, err
	}
	total += len(deployments.Items)

	var statefulsets kubeStatefulSetListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/namespaces/"+namespace+"/statefulsets", &statefulsets); err == nil {
		total += len(statefulsets.Items)
	}

	var daemonsets kubeDaemonSetListResponse
	if err := k8sGetJSON(client, runtime, "/apis/apps/v1/namespaces/"+namespace+"/daemonsets", &daemonsets); err == nil {
		total += len(daemonsets.Items)
	}

	var jobs kubeJobListResponse
	if err := k8sGetJSON(client, runtime, "/apis/batch/v1/namespaces/"+namespace+"/jobs", &jobs); err == nil {
		total += len(jobs.Items)
	}

	var cronjobs kubeCronJobListResponse
	if err := k8sGetJSON(client, runtime, "/apis/batch/v1/namespaces/"+namespace+"/cronjobs", &cronjobs); err == nil {
		total += len(cronjobs.Items)
	}

	return total, nil
}

func matchLabels(labels map[string]string, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func filterPodsBySelector(pods []kubePod, namespace string, selector map[string]string) []kubePod {
	result := make([]kubePod, 0)
	for _, pod := range pods {
		if pod.Metadata.Namespace != namespace {
			continue
		}
		if matchLabels(pod.Metadata.Labels, selector) {
			result = append(result, pod)
		}
	}
	return result
}

func filterPodsByOwnerOrSelector(pods []kubePod, namespace string, selector map[string]string, ownerKind string, ownerName string) []kubePod {
	result := make([]kubePod, 0)
	for _, pod := range pods {
		if pod.Metadata.Namespace != namespace {
			continue
		}
		matched := false
		for _, owner := range pod.Metadata.OwnerReferences {
			if strings.EqualFold(owner.Kind, ownerKind) && owner.Name == ownerName {
				matched = true
				break
			}
		}
		if matched || matchLabels(pod.Metadata.Labels, selector) {
			result = append(result, pod)
		}
	}
	return result
}

func buildContainerItems(containers []kubeContainer) []model.K8sContainerItem {
	items := make([]model.K8sContainerItem, 0, len(containers))
	for _, container := range containers {
		items = append(items, model.K8sContainerItem{
			Name:            container.Name,
			Image:           container.Image,
			RequestCPU:      container.Resources.Requests["cpu"],
			LimitCPU:        container.Resources.Limits["cpu"],
			RequestMemory:   container.Resources.Requests["memory"],
			LimitMemory:     container.Resources.Limits["memory"],
			ImagePullPolicy: container.ImagePullPolicy,
			Env:             buildContainerEnvItems(container.Env),
		})
	}
	return items
}

func marshalK8sYAML(v any) string {
	body, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(body)
}

func buildDeploymentDetail(client *http.Client, runtime kubeClusterRuntime, item kubeDeployment) model.K8sWorkloadDetail {
	pods, _ := fetchPodsByNamespace(client, runtime, item.Metadata.Namespace)
	relatedPods := filterPodsBySelector(pods, item.Metadata.Namespace, item.Spec.Selector.MatchLabels)
	replicas := intValue(item.Spec.Replicas)
	return model.K8sWorkloadDetail{
		Name:        item.Metadata.Name,
		Type:        "Deployment",
		Namespace:   item.Metadata.Namespace,
		Ready:       fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, replicas),
		Updated:     item.Status.UpdatedReplicas,
		Available:   item.Status.AvailableReplicas,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Selector:    item.Spec.Selector.MatchLabels,
		Pods:        buildPodItems(relatedPods),
		Containers:  buildContainerItems(item.Spec.Template.Spec.Containers),
		YAML:        marshalK8sYAML(item),
	}
}

func buildStatefulSetDetail(client *http.Client, runtime kubeClusterRuntime, item kubeStatefulSet) model.K8sWorkloadDetail {
	pods, _ := fetchPodsByNamespace(client, runtime, item.Metadata.Namespace)
	relatedPods := filterPodsBySelector(pods, item.Metadata.Namespace, item.Spec.Selector.MatchLabels)
	replicas := intValue(item.Spec.Replicas)
	return model.K8sWorkloadDetail{
		Name:        item.Metadata.Name,
		Type:        "StatefulSet",
		Namespace:   item.Metadata.Namespace,
		Ready:       fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, replicas),
		Updated:     item.Status.UpdatedReplicas,
		Available:   item.Status.AvailableReplicas,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Selector:    item.Spec.Selector.MatchLabels,
		Pods:        buildPodItems(relatedPods),
		Containers:  buildContainerItems(item.Spec.Template.Spec.Containers),
		YAML:        marshalK8sYAML(item),
	}
}

func buildDaemonSetDetail(client *http.Client, runtime kubeClusterRuntime, item kubeDaemonSet) model.K8sWorkloadDetail {
	pods, _ := fetchPodsByNamespace(client, runtime, item.Metadata.Namespace)
	relatedPods := filterPodsBySelector(pods, item.Metadata.Namespace, item.Spec.Selector.MatchLabels)
	return model.K8sWorkloadDetail{
		Name:        item.Metadata.Name,
		Type:        "DaemonSet",
		Namespace:   item.Metadata.Namespace,
		Ready:       fmt.Sprintf("%d/%d", item.Status.NumberReady, item.Status.DesiredNumberScheduled),
		Updated:     item.Status.UpdatedNumberScheduled,
		Available:   item.Status.NumberAvailable,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Selector:    item.Spec.Selector.MatchLabels,
		Pods:        buildPodItems(relatedPods),
		Containers:  buildContainerItems(item.Spec.Template.Spec.Containers),
		YAML:        marshalK8sYAML(item),
	}
}

func buildJobDetail(client *http.Client, runtime kubeClusterRuntime, item kubeJob) model.K8sWorkloadDetail {
	pods, _ := fetchPodsByNamespace(client, runtime, item.Metadata.Namespace)
	selector := map[string]string{"job-name": item.Metadata.Name}
	if item.Spec.Selector != nil && len(item.Spec.Selector.MatchLabels) > 0 {
		selector = item.Spec.Selector.MatchLabels
	}
	relatedPods := filterPodsByOwnerOrSelector(pods, item.Metadata.Namespace, selector, "Job", item.Metadata.Name)
	total := intValue(item.Spec.Completions)
	if total == 0 {
		total = item.Status.Active + item.Status.Succeeded + item.Status.Failed
	}
	return model.K8sWorkloadDetail{
		Name:        item.Metadata.Name,
		Type:        "Job",
		Namespace:   item.Metadata.Namespace,
		Ready:       fmt.Sprintf("%d/%d", item.Status.Succeeded, total),
		Updated:     item.Status.Active,
		Available:   item.Status.Succeeded,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Selector:    selector,
		Pods:        buildPodItems(relatedPods),
		Containers:  buildContainerItems(item.Spec.Template.Spec.Containers),
		YAML:        marshalK8sYAML(item),
	}
}

func buildCronJobDetail(client *http.Client, runtime kubeClusterRuntime, item kubeCronJob) model.K8sWorkloadDetail {
	pods, _ := fetchPodsByNamespace(client, runtime, item.Metadata.Namespace)
	relatedPods := make([]kubePod, 0)
	for _, pod := range pods {
		if pod.Metadata.Namespace != item.Metadata.Namespace {
			continue
		}
		if strings.HasPrefix(pod.Metadata.Name, item.Metadata.Name+"-") {
			relatedPods = append(relatedPods, pod)
			continue
		}
		for _, owner := range pod.Metadata.OwnerReferences {
			if strings.EqualFold(owner.Kind, "Job") && strings.HasPrefix(owner.Name, item.Metadata.Name+"-") {
				relatedPods = append(relatedPods, pod)
				break
			}
		}
	}
	active := len(item.Status.Active)
	schedule := item.Spec.Schedule
	if strings.TrimSpace(schedule) == "" {
		schedule = "-"
	}
	return model.K8sWorkloadDetail{
		Name:        item.Metadata.Name,
		Type:        "CronJob",
		Namespace:   item.Metadata.Namespace,
		Ready:       cronJobReadyText(item.Spec.Suspend, active),
		Updated:     active,
		Available:   active,
		Age:         humanizeAge(item.Metadata.CreationTimestamp),
		Labels:      item.Metadata.Labels,
		Annotations: item.Metadata.Annotations,
		Selector:    map[string]string{"schedule": schedule},
		Pods:        buildPodItems(relatedPods),
		Containers:  buildContainerItems(item.Spec.JobTemplate.Spec.Template.Spec.Containers),
		YAML:        marshalK8sYAML(item),
	}
}

func buildOverviewDistribution(cluster model.K8sClusterView, nodes []kubeNode, configMaps []kubeConfigMap) []model.K8sKVTextItem {
	serviceCIDR, podCIDR := resolveK8sNetworkCIDRs(nodes, configMaps)
	return []model.K8sKVTextItem{
		{Label: "集群状态", Value: cluster.StatusText},
		{Label: "集群版本", Value: fallbackText(cluster.Version)},
		{Label: "节点数量", Value: intLabel(cluster.NodeCount, " 个")},
		{Label: "Service IP 段", Value: serviceCIDR},
		{Label: "容器网络", Value: podCIDR},
	}
}

// resolveK8sNetworkCIDRs reads the cluster-level CIDRs from kubeadm's ConfigMap
// when it is available, then falls back to the Pod CIDRs assigned to nodes.
// Kubernetes does not expose the Service CIDR from a stable core API, so an
// unavailable value is intentionally reported as unavailable instead of guessed.
func resolveK8sNetworkCIDRs(nodes []kubeNode, configMaps []kubeConfigMap) (string, string) {
	serviceCIDR, podCIDR := "未识别", "未识别"
	for _, configMap := range configMaps {
		if configMap.Metadata.Namespace != "kube-system" || configMap.Metadata.Name != "kubeadm-config" {
			continue
		}
		for _, content := range configMap.Data {
			for _, line := range strings.Split(content, "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
				if !ok {
					continue
				}
				value = strings.Trim(strings.TrimSpace(strings.Split(value, "#")[0]), "\"'")
				switch strings.TrimSpace(key) {
				case "serviceSubnet":
					if value != "" {
						serviceCIDR = value
					}
				case "podSubnet":
					if value != "" {
						podCIDR = value
					}
				}
			}
		}
	}
	if podCIDR == "未识别" {
		cidrs := make([]string, 0)
		seen := map[string]struct{}{}
		for _, node := range nodes {
			values := append([]string{}, node.Spec.PodCIDRs...)
			if node.Spec.PodCIDR != "" {
				values = append(values, node.Spec.PodCIDR)
			}
			for _, cidr := range values {
				cidr = strings.TrimSpace(cidr)
				if cidr == "" {
					continue
				}
				if _, exists := seen[cidr]; !exists {
					seen[cidr] = struct{}{}
					cidrs = append(cidrs, cidr)
				}
			}
		}
		if len(cidrs) > 0 {
			sort.Strings(cidrs)
			podCIDR = strings.Join(cidrs, "、")
		}
	}
	return serviceCIDR, podCIDR
}

func buildOverviewCertificates(runtime kubeClusterRuntime) []model.K8sCertificate {
	certificates := make([]model.K8sCertificate, 0, 2)
	if certificate, ok := parseOverviewCertificate("CA 证书", "certificate-authority", runtime.CertificateAuthority); ok {
		certificates = append(certificates, certificate)
	}
	if certificate, ok := parseOverviewCertificate("客户端证书", "client-certificate", runtime.ClientCertificateData); ok {
		certificates = append(certificates, certificate)
	}
	return certificates
}

func parseOverviewCertificate(name string, certType string, encoded string) (model.K8sCertificate, bool) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return model.K8sCertificate{}, false
	}

	certBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return model.K8sCertificate{}, false
	}

	block, _ := pem.Decode(certBytes)
	if block == nil {
		return model.K8sCertificate{}, false
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return model.K8sCertificate{}, false
	}

	daysRemaining := int(time.Until(certificate.NotAfter).Hours() / 24)
	status, statusText := k8sCertificateStatus(certificate.NotAfter)

	return model.K8sCertificate{
		Name:          name,
		Type:          certType,
		Subject:       certificateCommonName(certificate.Subject.CommonName, certificate.Subject.String()),
		Issuer:        certificateCommonName(certificate.Issuer.CommonName, certificate.Issuer.String()),
		NotBefore:     certificate.NotBefore.Local().Format("2006-01-02 15:04:05"),
		NotAfter:      certificate.NotAfter.Local().Format("2006-01-02 15:04:05"),
		DaysRemaining: daysRemaining,
		Status:        status,
		StatusText:    statusText,
	}, true
}

func certificateCommonName(commonName string, fallback string) string {
	if strings.TrimSpace(commonName) != "" {
		return strings.TrimSpace(commonName)
	}
	return fallbackText(fallback)
}

func k8sCertificateStatus(notAfter time.Time) (string, string) {
	remaining := time.Until(notAfter)
	switch {
	case remaining <= 0:
		return "expired", "已过期"
	case remaining <= 30*24*time.Hour:
		return "warning", "即将到期"
	default:
		return "valid", "有效"
	}
}

func buildNodeItems(nodes []kubeNode, pods []kubePod) []model.K8sNodeItem {
	podCountByNode := map[string]int{}
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			podCountByNode[pod.Spec.NodeName]++
		}
	}

	items := make([]model.K8sNodeItem, 0, len(nodes))
	for _, node := range nodes {
		internalIP := "-"
		for _, address := range node.Status.Addresses {
			if address.Type == "InternalIP" {
				internalIP = fallbackText(address.Address)
				break
			}
		}

		items = append(items, model.K8sNodeItem{
			Name:       node.Metadata.Name,
			Role:       joinNodeRoles(node.Metadata.Labels),
			Status:     nodeReadyStatus(node),
			Version:    fallbackText(node.Status.NodeInfo.KubeletVersion),
			InternalIP: internalIP,
			OS:         fallbackText(node.Status.NodeInfo.OSImage),
			CPU:        fallbackText(node.Status.Allocatable["cpu"]),
			Memory:     formatMemoryMB(node.Status.Allocatable["memory"]),
			Pods:       fmt.Sprintf("%d/%s", podCountByNode[node.Metadata.Name], fallbackText(node.Status.Capacity["pods"])),
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func buildNamespaceCounts(data k8sFetchedData) map[string]struct {
	pods      int
	services  int
	workloads int
} {
	counts := map[string]struct {
		pods      int
		services  int
		workloads int
	}{}

	for _, pod := range data.Pods {
		item := counts[pod.Metadata.Namespace]
		item.pods++
		counts[pod.Metadata.Namespace] = item
	}
	for _, service := range data.Services {
		item := counts[service.Metadata.Namespace]
		item.services++
		counts[service.Metadata.Namespace] = item
	}

	addWorkload := func(namespace string) {
		item := counts[namespace]
		item.workloads++
		counts[namespace] = item
	}
	for _, item := range data.Deployments {
		addWorkload(item.Metadata.Namespace)
	}
	for _, item := range data.StatefulSet {
		addWorkload(item.Metadata.Namespace)
	}
	for _, item := range data.DaemonSets {
		addWorkload(item.Metadata.Namespace)
	}
	for _, item := range data.Jobs {
		addWorkload(item.Metadata.Namespace)
	}
	for _, item := range data.CronJobs {
		addWorkload(item.Metadata.Namespace)
	}

	return counts
}

func buildNamespaceItems(namespaces []kubeNamespace, counts map[string]struct {
	pods      int
	services  int
	workloads int
}) []model.K8sNamespaceItem {
	items := make([]model.K8sNamespaceItem, 0, len(namespaces))
	for _, namespace := range namespaces {
		stat := counts[namespace.Metadata.Name]
		items = append(items, model.K8sNamespaceItem{
			Name:      namespace.Metadata.Name,
			Status:    fallbackText(namespace.Status.Phase),
			Pods:      stat.pods,
			Services:  stat.services,
			Workloads: stat.workloads,
			CreatedAt: formatTimestamp(namespace.Metadata.CreationTimestamp),
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

type podWorkloadRef struct {
	Name string
	Type string
}

func podWorkloadKey(namespace, podName string) string {
	return namespace + "/" + podName
}

func buildPodItemsWithWorkloads(data k8sFetchedData) []model.K8sPodItem {
	refs := make(map[string]podWorkloadRef)
	workloadsByNamespace := make(map[string][]podWorkloadRef)
	addWorkload := func(namespace, name, workloadType string) {
		if name == "" {
			return
		}
		workloadsByNamespace[namespace] = append(workloadsByNamespace[namespace], podWorkloadRef{Name: name, Type: workloadType})
	}
	for _, item := range data.Deployments {
		addWorkload(item.Metadata.Namespace, item.Metadata.Name, "Deployment")
	}
	for _, item := range data.StatefulSet {
		addWorkload(item.Metadata.Namespace, item.Metadata.Name, "StatefulSet")
	}
	for _, item := range data.DaemonSets {
		addWorkload(item.Metadata.Namespace, item.Metadata.Name, "DaemonSet")
	}
	for _, item := range data.Jobs {
		addWorkload(item.Metadata.Namespace, item.Metadata.Name, "Job")
	}
	for _, item := range data.CronJobs {
		addWorkload(item.Metadata.Namespace, item.Metadata.Name, "CronJob")
	}

	replicaSetRefs := make(map[string]podWorkloadRef)
	for _, item := range data.ReplicaSets {
		for _, owner := range item.Metadata.OwnerReferences {
			if strings.EqualFold(owner.Kind, "Deployment") && owner.Name != "" {
				replicaSetRefs[podWorkloadKey(item.Metadata.Namespace, item.Metadata.Name)] = podWorkloadRef{Name: owner.Name, Type: "Deployment"}
				break
			}
		}
	}
	jobRefs := make(map[string]podWorkloadRef)
	for _, item := range data.Jobs {
		for _, owner := range item.Metadata.OwnerReferences {
			if strings.EqualFold(owner.Kind, "CronJob") && owner.Name != "" {
				jobRefs[podWorkloadKey(item.Metadata.Namespace, item.Metadata.Name)] = podWorkloadRef{Name: owner.Name, Type: "CronJob"}
				break
			}
		}
	}
	for _, pod := range data.Pods {
		for _, owner := range pod.Metadata.OwnerReferences {
			var ref podWorkloadRef
			switch {
			case strings.EqualFold(owner.Kind, "ReplicaSet"):
				ref = replicaSetRefs[podWorkloadKey(pod.Metadata.Namespace, owner.Name)]
			case strings.EqualFold(owner.Kind, "Job"):
				ref = jobRefs[podWorkloadKey(pod.Metadata.Namespace, owner.Name)]
				if ref.Name == "" {
					ref = podWorkloadRef{Name: owner.Name, Type: "Job"}
				}
			case strings.EqualFold(owner.Kind, "StatefulSet"), strings.EqualFold(owner.Kind, "DaemonSet"):
				ref = podWorkloadRef{Name: owner.Name, Type: owner.Kind}
			}
			if ref.Name != "" {
				refs[podWorkloadKey(pod.Metadata.Namespace, pod.Metadata.Name)] = ref
				break
			}
		}
	}
	assignBySelector := func(namespace, name, workloadType string, selector map[string]string) {
		for _, pod := range data.Pods {
			key := podWorkloadKey(namespace, pod.Metadata.Name)
			if pod.Metadata.Namespace == namespace && refs[key].Name == "" && len(selector) > 0 && matchLabels(pod.Metadata.Labels, selector) {
				refs[key] = podWorkloadRef{Name: name, Type: workloadType}
			}
		}
	}
	for _, item := range data.Deployments {
		assignBySelector(item.Metadata.Namespace, item.Metadata.Name, "Deployment", item.Spec.Selector.MatchLabels)
	}
	for _, item := range data.StatefulSet {
		assignBySelector(item.Metadata.Namespace, item.Metadata.Name, "StatefulSet", item.Spec.Selector.MatchLabels)
	}
	for _, item := range data.DaemonSets {
		assignBySelector(item.Metadata.Namespace, item.Metadata.Name, "DaemonSet", item.Spec.Selector.MatchLabels)
	}
	for _, item := range data.Jobs {
		selector := map[string]string{"job-name": item.Metadata.Name}
		if item.Spec.Selector != nil && len(item.Spec.Selector.MatchLabels) > 0 {
			selector = item.Spec.Selector.MatchLabels
		}
		assignBySelector(item.Metadata.Namespace, item.Metadata.Name, "Job", selector)
	}
	for _, item := range data.CronJobs {
		for _, pod := range data.Pods {
			if pod.Metadata.Namespace != item.Metadata.Namespace {
				continue
			}
			for _, owner := range pod.Metadata.OwnerReferences {
				if refs[podWorkloadKey(pod.Metadata.Namespace, pod.Metadata.Name)].Name == "" && strings.EqualFold(owner.Kind, "Job") && strings.HasPrefix(owner.Name, item.Metadata.Name+"-") {
					refs[podWorkloadKey(pod.Metadata.Namespace, pod.Metadata.Name)] = podWorkloadRef{Name: item.Metadata.Name, Type: "CronJob"}
				}
			}
		}
	}
	for _, pod := range data.Pods {
		key := podWorkloadKey(pod.Metadata.Namespace, pod.Metadata.Name)
		if refs[key].Name != "" {
			continue
		}
		// 部分受限集群不会返回 ownerReferences；在这种情况下按 Kubernetes
		// 控制器生成的 Pod 名称前缀兜底，并优先选择最长的工作负载名称。
		for _, candidate := range workloadsByNamespace[pod.Metadata.Namespace] {
			if strings.HasPrefix(pod.Metadata.Name, candidate.Name+"-") && len(candidate.Name) > len(refs[key].Name) {
				refs[key] = candidate
			}
		}
	}
	return buildPodItemsWithRefs(data.Pods, refs)
}

func buildPodItems(pods []kubePod) []model.K8sPodItem {
	return buildPodItemsWithRefs(pods, nil)
}

func buildPodItemsWithRefs(pods []kubePod, refs map[string]podWorkloadRef) []model.K8sPodItem {
	items := make([]model.K8sPodItem, 0, len(pods))
	for _, pod := range pods {
		restarts := 0
		for _, status := range pod.Status.ContainerStatuses {
			restarts += status.RestartCount
		}

		workload := refs[podWorkloadKey(pod.Metadata.Namespace, pod.Metadata.Name)]
		if workload.Name == "" {
			for _, owner := range pod.Metadata.OwnerReferences {
				if owner.Name != "" && (owner.Kind == "StatefulSet" || owner.Kind == "DaemonSet" || owner.Kind == "Job") {
					workload = podWorkloadRef{Name: owner.Name, Type: owner.Kind}
					break
				}
			}
		}
		items = append(items, model.K8sPodItem{
			Name: pod.Metadata.Name, Namespace: pod.Metadata.Namespace, WorkloadName: workload.Name, WorkloadType: workload.Type,
			Status: fallbackText(pod.Status.Phase), Node: fallbackText(pod.Spec.NodeName), NodeIP: fallbackText(pod.Status.HostIP),
			Restarts: restarts, Age: humanizeAge(pod.Metadata.CreationTimestamp), IP: fallbackText(pod.Status.PodIP),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace == items[j].Namespace {
			return items[i].Name < items[j].Name
		}
		return items[i].Namespace < items[j].Namespace
	})
	return items
}

func buildWorkloadItems(data k8sFetchedData) []model.K8sWorkloadItem {
	items := make([]model.K8sWorkloadItem, 0, len(data.Deployments)+len(data.StatefulSet)+len(data.DaemonSets)+len(data.Jobs)+len(data.CronJobs))

	for _, item := range data.Deployments {
		replicas := intValue(item.Spec.Replicas)
		items = append(items, model.K8sWorkloadItem{
			Name:      item.Metadata.Name,
			Type:      "Deployment",
			Namespace: item.Metadata.Namespace,
			Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, replicas),
			Updated:   item.Status.UpdatedReplicas,
			Available: item.Status.AvailableReplicas,
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
			Requests:  formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, true),
			Limits:    formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, false),
		})
	}

	for _, item := range data.StatefulSet {
		replicas := intValue(item.Spec.Replicas)
		items = append(items, model.K8sWorkloadItem{
			Name:      item.Metadata.Name,
			Type:      "StatefulSet",
			Namespace: item.Metadata.Namespace,
			Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, replicas),
			Updated:   item.Status.UpdatedReplicas,
			Available: item.Status.AvailableReplicas,
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
			Requests:  formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, true),
			Limits:    formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, false),
		})
	}

	for _, item := range data.DaemonSets {
		items = append(items, model.K8sWorkloadItem{
			Name:      item.Metadata.Name,
			Type:      "DaemonSet",
			Namespace: item.Metadata.Namespace,
			Ready:     fmt.Sprintf("%d/%d", item.Status.NumberReady, item.Status.DesiredNumberScheduled),
			Updated:   item.Status.UpdatedNumberScheduled,
			Available: item.Status.NumberAvailable,
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
			Requests:  formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, true),
			Limits:    formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, false),
		})
	}

	for _, item := range data.Jobs {
		total := intValue(item.Spec.Completions)
		if total == 0 {
			total = item.Status.Active + item.Status.Succeeded + item.Status.Failed
		}
		items = append(items, model.K8sWorkloadItem{
			Name:      item.Metadata.Name,
			Type:      "Job",
			Namespace: item.Metadata.Namespace,
			Ready:     fmt.Sprintf("%d/%d", item.Status.Succeeded, total),
			Updated:   item.Status.Active,
			Available: item.Status.Succeeded,
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
			Requests:  formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, true),
			Limits:    formatWorkloadResourceSummary(item.Spec.Template.Spec.Containers, false),
		})
	}

	for _, item := range data.CronJobs {
		active := len(item.Status.Active)
		items = append(items, model.K8sWorkloadItem{
			Name:      item.Metadata.Name,
			Type:      "CronJob",
			Namespace: item.Metadata.Namespace,
			Ready:     cronJobReadyText(item.Spec.Suspend, active),
			Updated:   active,
			Available: active,
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
			Requests:  formatWorkloadResourceSummary(item.Spec.JobTemplate.Spec.Template.Spec.Containers, true),
			Limits:    formatWorkloadResourceSummary(item.Spec.JobTemplate.Spec.Template.Spec.Containers, false),
		})
	}

	return items
}

func buildEndpointCounts(endpoints []kubeEndpoints) map[string]int {
	result := make(map[string]int, len(endpoints))
	for _, endpoint := range endpoints {
		key := endpoint.Metadata.Namespace + "/" + endpoint.Metadata.Name
		count := 0
		for _, subset := range endpoint.Subsets {
			count += len(subset.Addresses)
		}
		result[key] = count
	}
	return result
}

func formatServiceListPort(port int, nodePort int, protocol string) string {
	proto := strings.TrimSpace(protocol)
	if proto == "" {
		proto = "TCP"
	}
	if nodePort > 0 {
		return fmt.Sprintf("%d:%d/%s", port, nodePort, proto)
	}
	return fmt.Sprintf("%d/%s", port, proto)
}

func formatServiceDetailPort(port int, nodePort int, protocol string, targetPort string) string {
	base := formatServiceListPort(port, nodePort, protocol)
	targetPort = strings.TrimSpace(targetPort)
	if targetPort == "" || targetPort == strconv.Itoa(port) {
		return base
	}
	return fmt.Sprintf("%s -> %s", base, targetPort)
}

func serviceExternalIP(service kubeService) string {
	values := make([]string, 0, len(service.Spec.ExternalIPs)+len(service.Status.LoadBalancer.Ingress))
	for _, value := range service.Spec.ExternalIPs {
		value = strings.TrimSpace(value)
		if value != "" && value != "-" {
			values = append(values, value)
		}
	}
	for _, item := range service.Status.LoadBalancer.Ingress {
		value := firstNonEmpty(strings.TrimSpace(item.IP), strings.TrimSpace(item.Hostname))
		if value != "" && value != "-" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return "<none>"
	}
	return strings.Join(values, ", ")
}

func k8sWorkloadResourcePath(namespace string, workloadType string, workloadName string) (string, error) {
	// 页面列表展示的是 Deployment / StatefulSet 等 Kubernetes Kind，
	// 而 API 调用也可能传入小写形式；路径选择统一按规范化的小写值处理。
	switch strings.ToLower(strings.TrimSpace(workloadType)) {
	case "deployment":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, workloadName), nil
	case "statefulset":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", namespace, workloadName), nil
	case "daemonset":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", namespace, workloadName), nil
	case "job":
		return fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", namespace, workloadName), nil
	case "cronjob":
		return fmt.Sprintf("/apis/batch/v1/namespaces/%s/cronjobs/%s", namespace, workloadName), nil
	default:
		return "", errors.New("unsupported workload type")
	}
}

func extractWorkloadContainers(resource map[string]any) ([]map[string]any, error) {
	spec, ok := resource["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid workload spec")
	}
	template, ok := spec["template"].(map[string]any)
	if !ok {
		jobTemplate, jobTemplateOK := spec["jobTemplate"].(map[string]any)
		if !jobTemplateOK {
			return nil, errors.New("invalid workload template")
		}
		jobSpec, jobSpecOK := jobTemplate["spec"].(map[string]any)
		if !jobSpecOK {
			return nil, errors.New("invalid cronjob template")
		}
		template, ok = jobSpec["template"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid cronjob pod template")
		}
	}
	templateSpec, ok := template["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid pod template spec")
	}
	rawContainers, ok := templateSpec["containers"].([]any)
	if !ok {
		return nil, errors.New("no containers found in workload")
	}
	containers := make([]map[string]any, 0, len(rawContainers))
	for _, item := range rawContainers {
		container, ok := item.(map[string]any)
		if ok {
			containers = append(containers, container)
		}
	}
	return containers, nil
}

func buildWorkloadEnvPatch(existing map[string]any, envItems []model.K8sEnvVarItem) ([]map[string]any, error) {
	desired := make(map[string]struct{}, len(envItems))
	patch := make([]map[string]any, 0, len(envItems))
	for _, item := range envItems {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, errors.New("environment variable name is required")
		}
		if _, exists := desired[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable: %s", name)
		}
		desired[name] = struct{}{}
		entry := map[string]any{"name": name}
		if len(item.ValueFrom) > 0 {
			entry["valueFrom"] = item.ValueFrom
		} else {
			entry["value"] = item.Value
		}
		patch = append(patch, entry)
	}
	if existingEnv, ok := existing["env"].([]any); ok {
		for _, raw := range existingEnv {
			env, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := env["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, keep := desired[name]; !keep {
				patch = append(patch, map[string]any{"name": name, "$patch": "delete"})
			}
		}
	}
	return patch, nil
}

func buildContainerEnvItems(envs []kubeEnvVar) []model.K8sEnvVarItem {
	items := make([]model.K8sEnvVarItem, 0, len(envs))
	for _, env := range envs {
		item := model.K8sEnvVarItem{Name: env.Name, Value: env.Value, ValueFrom: env.ValueFrom}
		item.Source = formatK8sEnvSource(env.ValueFrom)
		items = append(items, item)
	}
	return items
}

func formatK8sEnvSource(valueFrom map[string]any) string {
	if len(valueFrom) == 0 {
		return ""
	}
	for sourceType, raw := range valueFrom {
		source, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := source["name"].(string)
		key, _ := source["key"].(string)
		if name != "" && key != "" {
			return fmt.Sprintf("%s: %s/%s", sourceType, name, key)
		}
		if name != "" {
			return fmt.Sprintf("%s: %s", sourceType, name)
		}
		return sourceType
	}
	return "由 Kubernetes 引用提供"
}

func buildWorkloadImagePatchBody(containers []map[string]any) map[string]any {
	return map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": containers,
				},
			},
		},
	}
}

func buildWorkloadContainerPatchBody(workloadType string, containers []map[string]any) map[string]any {
	template := map[string]any{"spec": map[string]any{"containers": containers}}
	if strings.EqualFold(strings.TrimSpace(workloadType), "cronjob") {
		return map[string]any{"spec": map[string]any{"jobTemplate": map[string]any{"spec": template}}}
	}
	return map[string]any{"spec": template}
}

func formatWorkloadResourceSummary(containers []kubeContainer, requests bool) string {
	var cpuMilli, memoryBytes int64
	hasCPU, hasMemory := false, false
	for _, container := range containers {
		values := container.Resources.Limits
		if requests {
			values = container.Resources.Requests
		}
		if value := strings.TrimSpace(values["cpu"]); value != "" {
			cpuMilli += parseCPUToMilli(value)
			hasCPU = true
		}
		if value := strings.TrimSpace(values["memory"]); value != "" {
			memoryBytes += parseBytesQuantity(value)
			hasMemory = true
		}
	}
	parts := make([]string, 0, 2)
	if hasCPU {
		parts = append(parts, formatCPUMilli(cpuMilli))
	}
	if hasMemory {
		parts = append(parts, formatMemoryBytes(memoryBytes))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

func formatCPUMilli(value int64) string {
	if value >= 1000 && value%1000 == 0 {
		return fmt.Sprintf("%d核", value/1000)
	}
	return fmt.Sprintf("%dm", value)
}

func formatMemoryBytes(value int64) string {
	if value >= 1024*1024*1024 {
		return fmt.Sprintf("%.1fGi", float64(value)/(1024*1024*1024))
	}
	return fmt.Sprintf("%.0fMi", float64(value)/(1024*1024))
}

func replaceImageVersion(image string, version string) string {
	trimmed := strings.TrimSpace(image)
	if trimmed == "" {
		return trimmed
	}

	base := trimmed
	if at := strings.Index(base, "@"); at >= 0 {
		base = base[:at]
	}

	lastSlash := strings.LastIndex(base, "/")
	lastColon := strings.LastIndex(base, ":")
	if lastColon > lastSlash {
		base = base[:lastColon]
	}

	return base + ":" + version
}

func anyToString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	default:
		return fmt.Sprintf("%v", value)
	}
}

func buildAdvancedNetworkSection(
	gatewayAPIGateways []kubeGatewayAPI,
	httpRoutes []kubeHTTPRoute,
	services []kubeService,
) model.K8sAdvancedNetworkSection {
	result := model.K8sAdvancedNetworkSection{
		GatewayAPIGateways: make([]model.K8sIstioResourceItem, 0, len(gatewayAPIGateways)),
		HTTPRoutes:         make([]model.K8sIstioResourceItem, 0, len(httpRoutes)),
	}

	for _, item := range gatewayAPIGateways {
		result.GatewayAPIGateways = append(result.GatewayAPIGateways, model.K8sIstioResourceItem{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Kind:      "Gateway",
			Hosts:     joinAndLimit(collectGatewayAPIHosts(item), 3),
			Address:   resolveGatewayAPIAddress(item, services),
			Ports:     joinAndLimit(collectGatewayAPIPorts(item), 4),
			Target:    fallbackText(item.Spec.GatewayClassName),
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
		})
	}

	for _, item := range httpRoutes {
		result.HTTPRoutes = append(result.HTTPRoutes, model.K8sIstioResourceItem{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Kind:      "HTTPRoute",
			Hosts:     joinAndLimit(uniqueNonEmptyStrings(item.Spec.Hostnames), 3),
			Gateways:  joinAndLimit(collectHTTPRouteParents(item), 3),
			Target:    joinAndLimit(collectHTTPRouteTargets(item), 3),
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
		})
	}

	sort.Slice(result.GatewayAPIGateways, func(i, j int) bool {
		return compareIstioItems(result.GatewayAPIGateways[i], result.GatewayAPIGateways[j])
	})
	sort.Slice(result.HTTPRoutes, func(i, j int) bool { return compareIstioItems(result.HTTPRoutes[i], result.HTTPRoutes[j]) })

	return result
}

func compareIstioItems(left, right model.K8sIstioResourceItem) bool {
	if left.Namespace == right.Namespace {
		return left.Name < right.Name
	}
	return left.Namespace < right.Namespace
}

func joinAndLimit(values []string, limit int) string {
	values = uniqueNonEmptyStrings(values)
	if len(values) == 0 {
		return "-"
	}
	if limit > 0 && len(values) > limit {
		return strings.Join(values[:limit], ", ") + fmt.Sprintf(" +%d", len(values)-limit)
	}
	return strings.Join(values, ", ")
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == "-" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func formatIstioPort(number int, protocol string) string {
	if number <= 0 {
		return fallbackText(protocol)
	}
	proto := strings.TrimSpace(protocol)
	if proto == "" {
		proto = "TCP"
	}
	return fmt.Sprintf("%d/%s", number, proto)
}

func joinSelector(selector map[string]string) string {
	if len(selector) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(selector))
	for key := range selector {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+selector[key])
	}
	return strings.Join(parts, ", ")
}

func collectVirtualServiceTargets(item kubeIstioVirtualService) []string {
	result := make([]string, 0)
	for _, httpRoute := range item.Spec.HTTP {
		for _, route := range httpRoute.Route {
			target := route.Destination.Host
			if route.Destination.Subset != "" {
				target += ":" + route.Destination.Subset
			}
			if route.Destination.Port.Number > 0 {
				target += ":" + strconv.Itoa(route.Destination.Port.Number)
			}
			result = append(result, target)
		}
	}
	for _, tcpRoute := range item.Spec.TCP {
		for _, route := range tcpRoute.Route {
			target := route.Destination.Host
			if route.Destination.Port.Number > 0 {
				target += ":" + strconv.Itoa(route.Destination.Port.Number)
			}
			result = append(result, target)
		}
	}
	return result
}

func buildVirtualServiceTrafficItems(item kubeIstioVirtualService) []model.K8sIstioTrafficRoute {
	httpIndex := firstVirtualServiceHTTPRouteIndex(item)
	if httpIndex < 0 {
		return nil
	}

	result := make([]model.K8sIstioTrafficRoute, 0, len(item.Spec.HTTP[httpIndex].Route))
	for index, route := range item.Spec.HTTP[httpIndex].Route {
		label := route.Destination.Host
		if route.Destination.Subset != "" {
			label += " / " + route.Destination.Subset
		}
		if route.Destination.Port.Number > 0 {
			label += ":" + strconv.Itoa(route.Destination.Port.Number)
		}
		result = append(result, model.K8sIstioTrafficRoute{
			Index:  index,
			Host:   route.Destination.Host,
			Subset: route.Destination.Subset,
			Port:   route.Destination.Port.Number,
			Weight: route.Weight,
			Label:  label,
		})
	}
	return result
}

func firstVirtualServiceHTTPRouteIndex(item kubeIstioVirtualService) int {
	for index, route := range item.Spec.HTTP {
		if len(route.Route) > 0 {
			return index
		}
	}
	return -1
}

func collectGatewayAPIHosts(item kubeGatewayAPI) []string {
	hosts := make([]string, 0, len(item.Spec.Listeners))
	for _, listener := range item.Spec.Listeners {
		hosts = append(hosts, firstNonEmpty(listener.Hostname, "*"))
	}
	return uniqueNonEmptyStrings(hosts)
}

func collectGatewayAPIPorts(item kubeGatewayAPI) []string {
	ports := make([]string, 0, len(item.Spec.Listeners))
	for _, listener := range item.Spec.Listeners {
		ports = append(ports, formatIstioPort(listener.Port, listener.Protocol))
	}
	return uniqueNonEmptyStrings(ports)
}

func collectGatewayAPIAddresses(item kubeGatewayAPI) []string {
	values := make([]string, 0, len(item.Status.Addresses))
	for _, address := range item.Status.Addresses {
		values = append(values, address.Value)
	}
	return uniqueNonEmptyStrings(values)
}

func resolveGatewayAPIAddress(item kubeGatewayAPI, services []kubeService) string {
	addresses := collectGatewayAPIAddresses(item)
	if len(addresses) > 0 {
		return joinAndLimit(addresses, 3)
	}

	candidates := []string{
		item.Metadata.Name,
		item.Metadata.Name + "-istio",
	}
	for _, service := range services {
		if service.Metadata.Namespace != item.Metadata.Namespace {
			continue
		}
		name := service.Metadata.Name
		matched := false
		for _, candidate := range candidates {
			if name == candidate || strings.HasPrefix(name, item.Metadata.Name+"-") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		address := serviceExternalIP(service)
		if strings.TrimSpace(address) != "" && address != "<none>" && address != "-" {
			return address
		}
	}

	return "-"
}

func collectHTTPRouteParents(item kubeHTTPRoute) []string {
	values := make([]string, 0, len(item.Spec.ParentRefs))
	for _, ref := range item.Spec.ParentRefs {
		if strings.TrimSpace(ref.Namespace) != "" {
			values = append(values, ref.Namespace+"/"+ref.Name)
			continue
		}
		values = append(values, ref.Name)
	}
	return uniqueNonEmptyStrings(values)
}

func collectHTTPRouteTargets(item kubeHTTPRoute) []string {
	values := make([]string, 0)
	for _, rule := range item.Spec.Rules {
		values = append(values, collectHTTPRouteTargetsFromRule(rule)...)
	}
	return uniqueNonEmptyStrings(values)
}

func collectHTTPRouteTargetsFromRule(rule struct {
	Matches []struct {
		Path struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"path"`
	} `json:"matches"`
	BackendRefs []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Port      int    `json:"port"`
		Weight    int    `json:"weight"`
	} `json:"backendRefs"`
}) []string {
	values := make([]string, 0, len(rule.BackendRefs))
	for _, backend := range rule.BackendRefs {
		target := backend.Name
		if strings.TrimSpace(backend.Namespace) != "" {
			target = backend.Namespace + "/" + target
		}
		if backend.Port > 0 {
			target += ":" + strconv.Itoa(backend.Port)
		}
		if backend.Weight > 0 {
			target += fmt.Sprintf(" (%d%%)", backend.Weight)
		}
		values = append(values, target)
	}
	return values
}

func buildHTTPRouteTrafficItems(item kubeHTTPRoute) []model.K8sIstioTrafficRoute {
	ruleIndex := firstHTTPRouteRuleIndex(item)
	if ruleIndex < 0 {
		return nil
	}
	result := make([]model.K8sIstioTrafficRoute, 0, len(item.Spec.Rules[ruleIndex].BackendRefs))
	for index, backend := range item.Spec.Rules[ruleIndex].BackendRefs {
		label := backend.Name
		if strings.TrimSpace(backend.Namespace) != "" {
			label = backend.Namespace + "/" + label
		}
		if backend.Port > 0 {
			label += ":" + strconv.Itoa(backend.Port)
		}
		result = append(result, model.K8sIstioTrafficRoute{
			Index:  index,
			Host:   backend.Name,
			Port:   backend.Port,
			Weight: backend.Weight,
			Label:  label,
		})
	}
	return result
}

func firstHTTPRouteRuleIndex(item kubeHTTPRoute) int {
	for index, rule := range item.Spec.Rules {
		if len(rule.BackendRefs) > 0 {
			return index
		}
	}
	return -1
}

func buildNetworkSection(services []kubeService, ingresses []kubeIngress, endpointCounts map[string]int) model.K8sNetworkSection {
	serviceItems := make([]model.K8sServiceItem, 0, len(services))
	for _, service := range services {
		ports := make([]string, 0, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			ports = append(ports, formatServiceListPort(port.Port, port.NodePort, port.Protocol))
		}

		key := service.Metadata.Namespace + "/" + service.Metadata.Name
		serviceItems = append(serviceItems, model.K8sServiceItem{
			Name:       service.Metadata.Name,
			Namespace:  service.Metadata.Namespace,
			Type:       serviceDisplayType(service),
			ClusterIP:  fallbackText(service.Spec.ClusterIP),
			ExternalIP: serviceExternalIP(service),
			Ports:      strings.Join(ports, ", "),
			Endpoints:  endpointCounts[key],
			Age:        humanizeAge(service.Metadata.CreationTimestamp),
		})
	}
	sort.Slice(serviceItems, func(i, j int) bool {
		if serviceItems[i].Namespace == serviceItems[j].Namespace {
			return serviceItems[i].Name < serviceItems[j].Name
		}
		return serviceItems[i].Namespace < serviceItems[j].Namespace
	})

	ingressItems := make([]model.K8sIngressItem, 0, len(ingresses))
	for _, ingress := range ingresses {
		hosts := make([]string, 0, len(ingress.Spec.Rules))
		for _, rule := range ingress.Spec.Rules {
			if strings.TrimSpace(rule.Host) != "" {
				hosts = append(hosts, rule.Host)
			}
		}

		address := "-"
		if len(ingress.Status.LoadBalancer.Ingress) > 0 {
			address = firstNonEmpty(ingress.Status.LoadBalancer.Ingress[0].IP, ingress.Status.LoadBalancer.Ingress[0].Hostname)
		}

		tls := "未启用"
		if len(ingress.Spec.TLS) > 0 {
			tls = "已启用"
		}

		ingressItems = append(ingressItems, model.K8sIngressItem{
			Name:      ingress.Metadata.Name,
			Namespace: ingress.Metadata.Namespace,
			Host:      fallbackText(strings.Join(hosts, ", ")),
			Address:   fallbackText(address),
			TLS:       tls,
			Age:       humanizeAge(ingress.Metadata.CreationTimestamp),
		})
	}
	sort.Slice(ingressItems, func(i, j int) bool {
		if ingressItems[i].Namespace == ingressItems[j].Namespace {
			return ingressItems[i].Name < ingressItems[j].Name
		}
		return ingressItems[i].Namespace < ingressItems[j].Namespace
	})

	return model.K8sNetworkSection{
		Services:  serviceItems,
		Ingresses: ingressItems,
	}
}

func serviceDisplayType(service kubeService) string {
	if strings.EqualFold(strings.TrimSpace(service.Spec.ClusterIP), "None") {
		return "Headless"
	}
	return fallbackText(service.Spec.Type)
}

func persistentVolumeSource(item kubePersistentVolume) (sourceType, path, nfsServer string) {
	if item.Spec.HostPath != nil {
		return "hostPath", fallbackText(item.Spec.HostPath.Path), "-"
	}
	if item.Spec.NFS != nil {
		return "NFS", fallbackText(item.Spec.NFS.Path), fallbackText(item.Spec.NFS.Server)
	}
	return "-", "-", "-"
}

const storageNamespaceScopeAnnotation = "ops-admin.io/namespace-scope"

// storageNamespaceScope records the platform-level PVC scope for a static PV.
// PersistentVolumes are cluster-scoped Kubernetes resources, so this annotation
// keeps an Ops Admin namespace restriction explicit.
func storageNamespaceScope(annotations map[string]string) string {
	if annotations != nil {
		if scope := strings.TrimSpace(annotations[storageNamespaceScopeAnnotation]); scope != "" {
			return scope
		}
	}
	return "集群级"
}

func buildConfigStorageSection(configMaps []kubeConfigMap, secrets []kubeSecret, pvcs []kubePersistentVolumeClaim, pvs []kubePersistentVolume) model.K8sConfigStorageSection {
	configMapItems := make([]model.K8sConfigMapItem, 0, len(configMaps))
	for _, item := range configMaps {
		configMapItems = append(configMapItems, model.K8sConfigMapItem{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Keys:      len(item.Data) + len(item.Binary),
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
		})
	}
	sort.Slice(configMapItems, func(i, j int) bool {
		if configMapItems[i].Namespace == configMapItems[j].Namespace {
			return configMapItems[i].Name < configMapItems[j].Name
		}
		return configMapItems[i].Namespace < configMapItems[j].Namespace
	})

	secretItems := make([]model.K8sSecretItem, 0, len(secrets))
	for _, item := range secrets {
		secretItems = append(secretItems, model.K8sSecretItem{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Type:      fallbackText(item.Type),
			Age:       humanizeAge(item.Metadata.CreationTimestamp),
		})
	}
	sort.Slice(secretItems, func(i, j int) bool {
		if secretItems[i].Namespace == secretItems[j].Namespace {
			return secretItems[i].Name < secretItems[j].Name
		}
		return secretItems[i].Namespace < secretItems[j].Namespace
	})

	storageItems := make([]model.K8sStorageItem, 0, len(pvcs)+len(pvs))
	for _, item := range pvcs {
		storageItems = append(storageItems, model.K8sStorageItem{
			Name:      item.Metadata.Name,
			Kind:      "PVC",
			Namespace: fallbackText(item.Metadata.Namespace),
			Status:    fallbackText(item.Status.Phase),
			// 申请容量优先于绑定 PV 的实际容量，避免把 PV 容量误展示为 PVC 容量。
			Capacity:     fallbackText(firstNonEmpty(item.Spec.Resources.Requests["storage"], item.Status.Capacity["storage"])),
			StorageClass: fallbackText(item.Spec.StorageClassName),
			AccessModes:  strings.Join(item.Spec.AccessModes, ", "),
		})
	}
	for _, item := range pvs {
		sourceType, path, nfsServer := persistentVolumeSource(item)
		storageItems = append(storageItems, model.K8sStorageItem{
			Name:           item.Metadata.Name,
			Kind:           "PV",
			Namespace:      "集群级",
			NamespaceScope: storageNamespaceScope(item.Metadata.Annotations),
			Status:         fallbackText(item.Status.Phase),
			Capacity:       fallbackText(firstNonEmpty(item.Status.Capacity["storage"], item.Spec.Capacity["storage"])),
			StorageClass:   fallbackText(item.Spec.StorageClassName),
			SourceType:     sourceType,
			Path:           path,
			NFSServer:      nfsServer,
			AccessModes:    strings.Join(item.Spec.AccessModes, ", "),
			ReclaimPolicy:  fallbackText(item.Spec.PersistentVolumeReclaimPolicy),
		})
	}
	sort.Slice(storageItems, func(i, j int) bool {
		if storageItems[i].Kind == storageItems[j].Kind {
			if storageItems[i].Namespace == storageItems[j].Namespace {
				return storageItems[i].Name < storageItems[j].Name
			}
			return storageItems[i].Namespace < storageItems[j].Namespace
		}
		return storageItems[i].Kind < storageItems[j].Kind
	})

	return model.K8sConfigStorageSection{
		ConfigMaps: configMapItems,
		Secrets:    secretItems,
		Storage:    storageItems,
	}
}

func calculateK8sAggregateMetrics(nodes []kubeNode, pods []kubePod) k8sAggregateMetrics {
	var metrics k8sAggregateMetrics

	for _, node := range nodes {
		metrics.TotalAllocCPUMilli += parseCPUToMilli(node.Status.Allocatable["cpu"])
		metrics.TotalAllocMemoryBytes += parseBytesQuantity(node.Status.Allocatable["memory"])
		if nodeReadyStatus(node) != "Ready" || node.Spec.Unschedulable {
			metrics.AlertCount++
		}
	}

	for _, pod := range pods {
		switch strings.ToLower(pod.Status.Phase) {
		case "failed", "pending", "unknown":
			metrics.AlertCount++
		}
		for _, container := range pod.Spec.Containers {
			metrics.TotalReqCPUMilli += parseCPUToMilli(container.Resources.Requests["cpu"])
			metrics.TotalReqMemoryBytes += parseBytesQuantity(container.Resources.Requests["memory"])
		}
	}

	return metrics
}

func toK8sClusterView(cluster model.K8sCluster) model.K8sClusterView {
	return model.K8sClusterView{
		ID:                    cluster.ID,
		Name:                  cluster.Name,
		Status:                cluster.Status,
		StatusText:            k8sStatusText(cluster.Status),
		APIServer:             cluster.APIServer,
		Version:               cluster.Version,
		NodeCount:             cluster.NodeCount,
		Env:                   cluster.Env,
		Tags:                  cluster.Tags,
		ConnectionMode:        normalizeConnectionMode(cluster.ConnectionMode),
		GatewayID:             cluster.GatewayID,
		GatewayName:           cluster.Gateway.Name,
		MonitorDatasourceID:   cluster.MonitorDatasourceID,
		MonitorDatasourceName: cluster.MonitorDatasource.Name,
		Description:           cluster.Description,
		LastSyncAt:            cluster.LastSyncAt,
		CreatedAt:             cluster.CreatedAt,
		UpdatedAt:             cluster.UpdatedAt,
	}
}

func validateK8sClusterPayload(cluster model.K8sCluster) error {
	if cluster.Name == "" {
		return errors.New("闆嗙兢鍚嶇О涓嶈兘涓虹┖")
	}
	if cluster.KubeConfig == "" {
		return errors.New("kubeconfig 涓嶈兘涓虹┖")
	}
	if cluster.Env == "" {
		return errors.New("请选择所属环境")
	}
	if err := validateGatewaySelection(cluster.ConnectionMode, cluster.GatewayID); err != nil {
		return err
	}
	return nil
}

func (s *Service) probeK8sCluster(cluster model.K8sCluster) (k8sClusterProbe, error) {
	config, cleanup, err := s.k8sRESTConfigForCluster(cluster)
	if err != nil {
		return k8sClusterProbe{}, fmt.Errorf("集群配置解析失败: %w", err)
	}
	defer cleanup()
	config.Timeout = 8 * time.Second

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return k8sClusterProbe{}, fmt.Errorf("Kubernetes 客户端初始化失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		if normalizeConnectionMode(cluster.ConnectionMode) == "gateway" {
			return k8sClusterProbe{}, fmt.Errorf("通过网关连接 API Server 失败（%s）: %w", config.Host, err)
		}
		return k8sClusterProbe{}, fmt.Errorf("连接 API Server 失败（%s）: %w", config.Host, err)
	}
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return k8sClusterProbe{}, fmt.Errorf("连接成功，但读取节点列表失败（请检查 nodes/list 权限）: %w", err)
	}

	return k8sClusterProbe{
		APIServer: config.Host,
		Version:   version.GitVersion,
		NodeCount: len(nodes.Items),
		Status:    "running",
	}, nil
}

func parseKubeConfig(content string) (kubeClusterRuntime, error) {
	var cfg kubeConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return kubeClusterRuntime{}, err
	}

	contextName := strings.TrimSpace(cfg.CurrentContext)
	if contextName == "" && len(cfg.Contexts) > 0 {
		contextName = cfg.Contexts[0].Name
	}
	if contextName == "" {
		return kubeClusterRuntime{}, errors.New("missing context")
	}

	var clusterName string
	var userName string
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == contextName {
			clusterName = strings.TrimSpace(cfg.Contexts[i].Context.Cluster)
			userName = strings.TrimSpace(cfg.Contexts[i].Context.User)
			break
		}
	}
	if clusterName == "" {
		return kubeClusterRuntime{}, errors.New("cluster not found")
	}

	runtime := kubeClusterRuntime{}
	for i := range cfg.Clusters {
		if cfg.Clusters[i].Name == clusterName {
			runtime.Server = strings.TrimSpace(cfg.Clusters[i].Cluster.Server)
			runtime.InsecureSkipTLSVerify = cfg.Clusters[i].Cluster.InsecureSkipTLSVerify
			runtime.CertificateAuthority = strings.TrimSpace(cfg.Clusters[i].Cluster.CertificateAuthorityData)
			break
		}
	}
	if runtime.Server == "" {
		return kubeClusterRuntime{}, errors.New("server not found")
	}

	for i := range cfg.Users {
		if cfg.Users[i].Name == userName {
			runtime.Token = strings.TrimSpace(cfg.Users[i].User.Token)
			runtime.Username = strings.TrimSpace(cfg.Users[i].User.Username)
			runtime.Password = strings.TrimSpace(cfg.Users[i].User.Password)
			runtime.ClientCertificateData = strings.TrimSpace(cfg.Users[i].User.ClientCertificateData)
			runtime.ClientKeyData = strings.TrimSpace(cfg.Users[i].User.ClientKeyData)
			break
		}
	}

	return runtime, nil
}

func newK8sHTTPClient(runtime kubeClusterRuntime) (*http.Client, error) {
	client, err := newK8sHTTPClientWithDial(runtime, nil)
	return client, err
}

func newK8sHTTPClientWithDial(runtime kubeClusterRuntime, dialContext func(context.Context, string, string) (net.Conn, error)) (*http.Client, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: runtime.InsecureSkipTLSVerify,
	}

	if runtime.CertificateAuthority != "" {
		caBytes, err := base64.StdEncoding.DecodeString(runtime.CertificateAuthority)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, errors.New("invalid certificate authority")
		}
		tlsConfig.RootCAs = pool
	}

	if runtime.ClientCertificateData != "" && runtime.ClientKeyData != "" {
		certBytes, err := base64.StdEncoding.DecodeString(runtime.ClientCertificateData)
		if err != nil {
			return nil, err
		}
		keyBytes, err := base64.StdEncoding.DecodeString(runtime.ClientKeyData)
		if err != nil {
			return nil, err
		}
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	if dialContext != nil {
		transport.DialContext = dialContext
	}
	return &http.Client{Timeout: 8 * time.Second, Transport: transport}, nil
}

func (s *Service) newK8sHTTPClientForCluster(cluster model.K8sCluster, runtime kubeClusterRuntime) (*http.Client, func(), error) {
	if normalizeConnectionMode(cluster.ConnectionMode) != "gateway" || cluster.GatewayID == nil || *cluster.GatewayID == 0 {
		client, err := newK8sHTTPClient(runtime)
		return client, func() {}, err
	}
	gatewayID := *cluster.GatewayID
	dialContext := func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, cleanup, err := s.dialThroughGateway(ctx, gatewayID, network, address)
		if err != nil {
			return nil, err
		}
		return cleanupConn{Conn: conn, cleanup: cleanup}, nil
	}
	client, err := newK8sHTTPClientWithDial(runtime, dialContext)
	if err != nil {
		return nil, func() {}, err
	}
	return client, func() {}, nil
}

func fetchK8sVersion(client *http.Client, runtime kubeClusterRuntime) (string, error) {
	var payload kubeVersionResponse
	if err := k8sGetJSON(client, runtime, "/version", &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.GitVersion) == "" {
		return "", errors.New("empty version")
	}
	return payload.GitVersion, nil
}

func fetchK8sNodeCount(client *http.Client, runtime kubeClusterRuntime) (int, error) {
	var payload kubeNodeListResponse
	if err := k8sGetJSON(client, runtime, "/api/v1/nodes", &payload); err != nil {
		return 0, err
	}
	return len(payload.Items), nil
}

func flattenGatewayHosts(item kubeIstioGateway) []string {
	hosts := make([]string, 0)
	for _, server := range item.Spec.Servers {
		hosts = append(hosts, server.Hosts...)
	}
	return uniqueNonEmptyStrings(hosts)
}

func flattenGatewayPorts(item kubeIstioGateway) []string {
	ports := make([]string, 0, len(item.Spec.Servers))
	for _, server := range item.Spec.Servers {
		ports = append(ports, formatIstioPort(server.Port.Number, server.Port.Protocol))
	}
	return uniqueNonEmptyStrings(ports)
}

func k8sGetJSON(client *http.Client, runtime kubeClusterRuntime, path string, target any) error {
	return k8sGetJSONWithQuery(client, runtime, path, nil, target)
}

func k8sGetJSONAnyPath(client *http.Client, runtime kubeClusterRuntime, paths []string, target any) error {
	var lastErr error
	for _, path := range paths {
		if err := k8sGetJSON(client, runtime, path, target); err != nil {
			lastErr = err
			if isK8sNotFoundError(err) {
				continue
			}
			return err
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("resource path not found")
	}
	return lastErr
}

func k8sGetIstioJSON(client *http.Client, runtime kubeClusterRuntime, resource string, namespace string, name string, target any) error {
	paths := buildIstioResourcePaths(resource, namespace, name)
	return k8sGetJSONAnyPath(client, runtime, paths, target)
}

func k8sGetGatewayAPIJSON(client *http.Client, runtime kubeClusterRuntime, resource string, namespace string, name string, target any) error {
	paths := buildGatewayAPIResourcePaths(resource, namespace, name)
	return k8sGetJSONAnyPath(client, runtime, paths, target)
}

func buildIstioResourcePaths(resource string, namespace string, name string) []string {
	return buildIstioResourcePathsWithPreferred(resource, namespace, name, "")
}

func buildIstioResourcePathsWithPreferred(resource string, namespace string, name string, preferredVersion string) []string {
	versions := []string{"v1", "v1beta1"}
	preferredVersion = strings.TrimSpace(strings.TrimPrefix(preferredVersion, "networking.istio.io/"))
	if preferredVersion == "v1beta1" {
		versions = []string{"v1beta1", "v1"}
	}
	paths := make([]string, 0, len(versions))
	for _, version := range versions {
		base := fmt.Sprintf("/apis/networking.istio.io/%s", version)
		if strings.TrimSpace(namespace) != "" {
			base += "/namespaces/" + strings.TrimSpace(namespace)
		}
		base += "/" + resource
		if strings.TrimSpace(name) != "" {
			base += "/" + strings.TrimSpace(name)
		}
		paths = append(paths, base)
	}
	return paths
}

func buildGatewayAPIResourcePaths(resource string, namespace string, name string) []string {
	return buildGatewayAPIResourcePathsWithPreferred(resource, namespace, name, "")
}

func buildGatewayAPIResourcePathsWithPreferred(resource string, namespace string, name string, preferredVersion string) []string {
	versions := []string{"v1", "v1beta1"}
	preferredVersion = strings.TrimSpace(strings.TrimPrefix(preferredVersion, "gateway.networking.k8s.io/"))
	if preferredVersion == "v1beta1" {
		versions = []string{"v1beta1", "v1"}
	}
	paths := make([]string, 0, len(versions))
	for _, version := range versions {
		base := fmt.Sprintf("/apis/gateway.networking.k8s.io/%s", version)
		if strings.TrimSpace(namespace) != "" {
			base += "/namespaces/" + strings.TrimSpace(namespace)
		}
		base += "/" + resource
		if strings.TrimSpace(name) != "" {
			base += "/" + strings.TrimSpace(name)
		}
		paths = append(paths, base)
	}
	return paths
}

func k8sPatchJSON(client *http.Client, runtime kubeClusterRuntime, path string, body any, contentType string, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return k8sDoJSON(client, runtime, http.MethodPatch, path, nil, payload, contentType, target)
}

func k8sGetJSONWithQuery(client *http.Client, runtime kubeClusterRuntime, path string, query map[string]string, target any) error {
	return k8sDoJSON(client, runtime, http.MethodGet, path, query, nil, "application/json", target)
}

func k8sDoJSONAnyPath(
	client *http.Client,
	runtime kubeClusterRuntime,
	method string,
	paths []string,
	query map[string]string,
	body []byte,
	contentType string,
	target any,
) error {
	var lastErr error
	for _, path := range paths {
		err := k8sDoJSON(client, runtime, method, path, query, body, contentType, target)
		if err == nil {
			return nil
		}
		lastErr = err
		if isK8sNotFoundError(err) {
			continue
		}
		return err
	}
	if lastErr == nil {
		lastErr = errors.New("resource path not found")
	}
	return lastErr
}

func k8sDoJSON(client *http.Client, runtime kubeClusterRuntime, method string, path string, query map[string]string, body []byte, contentType string, target any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	endpointURL := strings.TrimRight(runtime.Server, "/") + path
	if len(query) > 0 {
		values := url.Values{}
		for key, value := range query {
			values.Set(key, value)
		}
		endpointURL += "?" + values.Encode()
	}

	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpointURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 && strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if runtime.Token != "" {
		req.Header.Set("Authorization", "Bearer "+runtime.Token)
	}
	if runtime.Username != "" || runtime.Password != "" {
		req.SetBasicAuth(runtime.Username, runtime.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		if len(message) > 0 {
			return fmt.Errorf("unexpected status: %d, %s", resp.StatusCode, strings.TrimSpace(string(message)))
		}
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func buildK8sYAMLResourcePath(payload model.K8sResourceYAMLPayload) (string, error) {
	resourceType := strings.ToLower(Trimmed(payload.ResourceType))
	namespace := Trimmed(payload.Namespace)
	name := Trimmed(payload.Name)

	switch resourceType {
	case "namespace":
		if name == "" {
			return "", errors.New("namespace name is required")
		}
		return "/api/v1/namespaces/" + name, nil
	case "pod":
		if namespace == "" || name == "" {
			return "", errors.New("pod namespace and name are required")
		}
		return fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", namespace, name), nil
	case "service":
		if namespace == "" || name == "" {
			return "", errors.New("service namespace and name are required")
		}
		return fmt.Sprintf("/api/v1/namespaces/%s/services/%s", namespace, name), nil
	case "ingress":
		if namespace == "" || name == "" {
			return "", errors.New("ingress namespace and name are required")
		}
		return fmt.Sprintf("/apis/networking.k8s.io/v1/namespaces/%s/ingresses/%s", namespace, name), nil
	case "configmap":
		if namespace == "" || name == "" {
			return "", errors.New("configmap namespace and name are required")
		}
		return fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", namespace, name), nil
	case "secret":
		if namespace == "" || name == "" {
			return "", errors.New("secret namespace and name are required")
		}
		return fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, name), nil
	case "pvc":
		if namespace == "" || name == "" {
			return "", errors.New("pvc namespace and name are required")
		}
		return fmt.Sprintf("/api/v1/namespaces/%s/persistentvolumeclaims/%s", namespace, name), nil
	case "pv":
		if name == "" {
			return "", errors.New("pv name is required")
		}
		return "/api/v1/persistentvolumes/" + name, nil
	case "workload":
		if namespace == "" || name == "" {
			return "", errors.New("workload namespace and name are required")
		}
		switch strings.ToLower(Trimmed(payload.WorkloadType)) {
		case "deployment":
			return fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, name), nil
		case "statefulset":
			return fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", namespace, name), nil
		case "daemonset":
			return fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", namespace, name), nil
		case "job":
			return fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", namespace, name), nil
		case "cronjob":
			return fmt.Sprintf("/apis/batch/v1/namespaces/%s/cronjobs/%s", namespace, name), nil
		default:
			return "", errors.New("unsupported workload type")
		}
	default:
		return "", errors.New("unsupported resource type")
	}
}

func buildK8sCreateResourcePaths(payload model.K8sResourceYAMLPayload, manifest k8sManifestIdentity) ([]string, error) {
	resourceType := strings.ToLower(Trimmed(payload.ResourceType))
	namespace := firstNonEmpty(Trimmed(payload.Namespace), Trimmed(manifest.Metadata.Namespace))

	switch resourceType {
	case "namespace":
		return []string{"/api/v1/namespaces"}, nil
	case "pod":
		if namespace == "" {
			return nil, errors.New("pod namespace is required")
		}
		return []string{fmt.Sprintf("/api/v1/namespaces/%s/pods", namespace)}, nil
	case "service":
		if namespace == "" {
			return nil, errors.New("service namespace is required")
		}
		return []string{fmt.Sprintf("/api/v1/namespaces/%s/services", namespace)}, nil
	case "ingress":
		if namespace == "" {
			return nil, errors.New("ingress namespace is required")
		}
		return []string{fmt.Sprintf("/apis/networking.k8s.io/v1/namespaces/%s/ingresses", namespace)}, nil
	case "configmap":
		if namespace == "" {
			return nil, errors.New("configmap namespace is required")
		}
		return []string{fmt.Sprintf("/api/v1/namespaces/%s/configmaps", namespace)}, nil
	case "secret":
		if namespace == "" {
			return nil, errors.New("secret namespace is required")
		}
		return []string{fmt.Sprintf("/api/v1/namespaces/%s/secrets", namespace)}, nil
	case "pvc":
		if namespace == "" {
			return nil, errors.New("pvc namespace is required")
		}
		return []string{fmt.Sprintf("/api/v1/namespaces/%s/persistentvolumeclaims", namespace)}, nil
	case "pv":
		return []string{"/api/v1/persistentvolumes"}, nil
	case "workload":
		if namespace == "" {
			return nil, errors.New("workload namespace is required")
		}
		switch strings.ToLower(Trimmed(payload.WorkloadType)) {
		case "deployment":
			return []string{fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments", namespace)}, nil
		case "statefulset":
			return []string{fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets", namespace)}, nil
		case "daemonset":
			return []string{fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets", namespace)}, nil
		case "job":
			return []string{fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs", namespace)}, nil
		case "cronjob":
			return []string{fmt.Sprintf("/apis/batch/v1/namespaces/%s/cronjobs", namespace)}, nil
		default:
			return nil, errors.New("unsupported workload type")
		}
	case "gateway", "virtualservice", "destinationrule", "serviceentry":
		if namespace == "" {
			return nil, fmt.Errorf("%s namespace is required", resourceType)
		}
		resourceMap := map[string]string{
			"gateway":         "gateways",
			"virtualservice":  "virtualservices",
			"destinationrule": "destinationrules",
			"serviceentry":    "serviceentries",
		}
		return buildIstioResourcePathsWithPreferred(
			resourceMap[resourceType],
			namespace,
			"",
			manifest.APIVersion,
		), nil
	case "gatewayapi":
		if namespace == "" {
			return nil, errors.New("gateway namespace is required")
		}
		return buildGatewayAPIResourcePathsWithPreferred("gateways", namespace, "", manifest.APIVersion), nil
	case "httproute":
		if namespace == "" {
			return nil, errors.New("httproute namespace is required")
		}
		return buildGatewayAPIResourcePathsWithPreferred("httproutes", namespace, "", manifest.APIVersion), nil
	default:
		return nil, errors.New("unsupported resource type")
	}
}

func buildK8sYAMLResourcePaths(payload model.K8sResourceYAMLPayload) ([]string, error) {
	resourceType := strings.ToLower(Trimmed(payload.ResourceType))
	namespace := Trimmed(payload.Namespace)
	name := Trimmed(payload.Name)

	switch resourceType {
	case "gateway", "virtualservice", "destinationrule", "serviceentry":
		if namespace == "" || name == "" {
			return nil, fmt.Errorf("%s namespace and name are required", resourceType)
		}
		resourceMap := map[string]string{
			"gateway":         "gateways",
			"virtualservice":  "virtualservices",
			"destinationrule": "destinationrules",
			"serviceentry":    "serviceentries",
		}
		return buildIstioResourcePaths(resourceMap[resourceType], namespace, name), nil
	case "gatewayapi":
		if namespace == "" || name == "" {
			return nil, errors.New("gateway namespace and name are required")
		}
		return buildGatewayAPIResourcePaths("gateways", namespace, name), nil
	case "httproute":
		if namespace == "" || name == "" {
			return nil, errors.New("httproute namespace and name are required")
		}
		return buildGatewayAPIResourcePaths("httproutes", namespace, name), nil
	default:
		path, err := buildK8sYAMLResourcePath(payload)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
}

func buildK8sDeleteResourcePaths(payload model.K8sResourceDeletePayload) ([]string, error) {
	return buildK8sYAMLResourcePaths(model.K8sResourceYAMLPayload{
		ResourceType: payload.ResourceType,
		Namespace:    payload.Namespace,
		Name:         payload.Name,
		WorkloadType: payload.WorkloadType,
	})
}

func parseK8sManifestIdentity(body []byte) (k8sManifestIdentity, error) {
	var manifest k8sManifestIdentity
	if err := json.Unmarshal(body, &manifest); err != nil {
		return manifest, errors.New("invalid yaml content")
	}
	if Trimmed(manifest.Kind) == "" {
		return manifest, errors.New("resource kind is required")
	}
	return manifest, nil
}

func validateBundleManifest(raw, namespace, expectedKind string) (k8sManifestIdentity, error) {
	body, err := ksyaml.YAMLToJSON([]byte(raw))
	if err != nil {
		return k8sManifestIdentity{}, errors.New("invalid yaml content")
	}
	manifest, err := parseK8sManifestIdentity(body)
	if err != nil {
		return manifest, err
	}
	if !strings.EqualFold(Trimmed(manifest.Kind), expectedKind) {
		return manifest, fmt.Errorf("resource kind must be %s", expectedKind)
	}
	if Trimmed(manifest.Metadata.Name) == "" {
		return manifest, errors.New("metadata.name is required")
	}
	if manifest.Metadata.Namespace != "" && Trimmed(manifest.Metadata.Namespace) != Trimmed(namespace) {
		return manifest, errors.New("metadata.namespace must match the selected namespace")
	}
	return manifest, nil
}

func isK8sNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected status: 404") || strings.Contains(message, "\"code\":404")
}

func friendlyK8sYAMLError(payload model.K8sResourceYAMLPayload, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	lower := strings.ToLower(message)

	if strings.Contains(lower, "field is immutable") || strings.Contains(lower, "immutable") {
		switch strings.ToLower(Trimmed(payload.ResourceType)) {
		case "pod":
			return errors.New("Pod 瀛樺湪涓嶅彲鍙樺瓧娈碉紝Kubernetes 涓嶅厑璁哥洿鎺ユ洿鏂拌繖閮ㄥ垎鍐呭銆傚缓璁慨鏀瑰彲鍙樺瓧娈碉紝鎴栧垹闄ゅ悗閲嶆柊鍒涘缓 Pod")
		case "pv", "pvc":
			return errors.New("褰撳墠瀛樺偍璧勬簮鍖呭惈涓嶅彲鍙樺瓧娈碉紝Kubernetes 涓嶅厑璁哥洿鎺ヨ鐩栦繚瀛樸€傝浠呬慨鏀瑰彲鍙樺瓧娈碉紝鎴栨寜瀛樺偍鍙樻洿娴佺▼澶勭悊")
		default:
			return errors.New("褰撳墠璧勬簮鍖呭惈涓嶅彲鍙樺瓧娈碉紝Kubernetes 涓嶅厑璁哥洿鎺ヨ鐩栦繚瀛樸€傝妫€鏌?metadata銆乻elector銆乿olume 绛夊瓧娈垫槸鍚﹁淇敼")
		}
	}

	if strings.Contains(lower, "already exists") {
		return errors.New("YAML 涓殑璧勬簮鏍囪瘑涓庡綋鍓嶉泦缇ょ幇鏈夎祫婧愬啿绐侊紝璇锋鏌ュ悕绉般€佸懡鍚嶇┖闂存垨鍏宠仈瀵硅薄")
	}
	if strings.Contains(lower, "not found") {
		return errors.New("目标资源不存在，可能已被删除或命名空间已变化，请刷新后重试")
	}
	if strings.Contains(lower, "invalid") || strings.Contains(lower, "unprocessable entity") {
		return errors.New("YAML 鏍￠獙鏈€氳繃锛岃妫€鏌ュ瓧娈垫牸寮忋€乤piVersion銆乲ind 浠ュ強 spec 鍐呭鏄惁姝ｇ‘")
	}
	return err
}

func k8sGetText(client *http.Client, runtime kubeClusterRuntime, path string, query map[string]string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	endpointURL := strings.TrimRight(runtime.Server, "/") + path
	if len(query) > 0 {
		values := url.Values{}
		for key, value := range query {
			values.Set(key, value)
		}
		endpointURL += "?" + values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return "", err
	}

	if runtime.Token != "" {
		req.Header.Set("Authorization", "Bearer "+runtime.Token)
	}
	if runtime.Username != "" || runtime.Password != "" {
		req.SetBasicAuth(runtime.Username, runtime.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func k8sStatusText(status string) string {
	switch normalizedK8sStatus(status) {
	case "warning":
		return "部分告警"
	case "offline":
		return "离线"
	default:
		return "运行中"
	}
}

func normalizedK8sStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "warning":
		return "warning"
	case "offline":
		return "offline"
	default:
		return "running"
	}
}

func calculateHealthScore(alertCount int) int {
	if alertCount <= 0 {
		return 100
	}
	score := 100 - alertCount*8
	if score < 40 {
		return 40
	}
	return score
}

func formatUsagePercent(used int64, total int64) string {
	if used <= 0 || total <= 0 {
		return "-"
	}
	value := float64(used) / float64(total) * 100
	return fmt.Sprintf("%.1f%%", value)
}

func nodeReadyStatus(node kubeNode) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			if condition.Status == "True" {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func firstNodeInternalIP(node kubeNode) string {
	for _, address := range node.Status.Addresses {
		if address.Type == "InternalIP" && strings.TrimSpace(address.Address) != "" {
			return address.Address
		}
	}
	return "-"
}

func joinNodeRoles(labels map[string]string) string {
	roles := make([]string, 0, 3)
	for key := range labels {
		if strings.HasPrefix(key, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(key, "node-role.kubernetes.io/")
			if role == "" {
				role = "worker"
			}
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return "worker"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

func parseCPUToMilli(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.HasSuffix(value, "m") {
		number := strings.TrimSuffix(value, "m")
		parsed, _ := strconv.ParseFloat(number, 64)
		return int64(parsed)
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return int64(parsed * 1000)
}

func parseBytesQuantity(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	units := map[string]float64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"Pi": 1024 * 1024 * 1024 * 1024 * 1024,
		"K":  1000,
		"M":  1000 * 1000,
		"G":  1000 * 1000 * 1000,
		"T":  1000 * 1000 * 1000 * 1000,
	}

	for suffix, multiplier := range units {
		if strings.HasSuffix(value, suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(value, suffix))
			parsed, _ := strconv.ParseFloat(number, 64)
			return int64(parsed * multiplier)
		}
	}

	parsed, _ := strconv.ParseFloat(value, 64)
	return int64(parsed)
}

func formatMemoryMB(value string) string {
	bytes := parseBytesQuantity(value)
	if bytes <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d MB", int64(math.Round(float64(bytes)/1000/1000)))
}

func humanizeAge(timestamp string) string {
	createdAt, err := time.Parse(time.RFC3339, strings.TrimSpace(timestamp))
	if err != nil {
		return "-"
	}

	duration := time.Since(createdAt)
	if duration < time.Minute {
		return "刚刚"
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	if duration < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
	return createdAt.Format("2006-01-02")
}

func formatTimestamp(value string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func stringifyTargetPort(value interface{}) string {
	switch current := value.(type) {
	case string:
		return current
	case float64:
		return strconv.Itoa(int(current))
	default:
		return fmt.Sprint(current)
	}
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func cronJobReadyText(suspend *bool, active int) string {
	if suspend != nil && *suspend {
		return "Suspended"
	}
	if active > 0 {
		return fmt.Sprintf("%d Active", active)
	}
	return "Scheduled"
}

func fallbackText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func intLabel(value int, suffix string) string {
	return strconv.Itoa(value) + suffix
}
