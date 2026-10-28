package model

import "time"

// Deploy plans own installation state; K8sCluster continues to own API access.
type K8sDeployConfig struct {
	Version                    string `json:"version"`
	RuntimeDirectory           string `json:"runtimeDirectory"`
	NetworkPlugin              string `json:"networkPlugin"`
	PodCIDR                    string `json:"podCIDR"`
	ServiceCIDR                string `json:"serviceCIDR"`
	Interface                  string `json:"interface"`
	AutoHostname               bool   `json:"autoHostname"`
	DisableSELinux             bool   `json:"disableSELinux"`
	DisableFirewalld           bool   `json:"disableFirewalld"`
	RequireCgroupV2            bool   `json:"requireCgroupV2"`
	ImageRepository            string `json:"imageRepository"`
	CustomSource               string `json:"customSource"`
	KubeletConfig              string `json:"kubeletConfig"`
	HA                         bool   `json:"ha"`
	BuiltinLB                  bool   `json:"builtinLB"`
	APIServer                  string `json:"apiServer"`
	APIPort                    int    `json:"apiPort"`
	RouterID                   int    `json:"routerId"`
	ProxyMode                  string `json:"proxyMode"`
	CustomCertificates         bool   `json:"customCertificates"`
	CertificateYears           int    `json:"certificateYears"`
	CAYears                    int    `json:"caYears"`
	MetricsServer              bool   `json:"metricsServer"`
	AllowControlPlaneWorkloads bool   `json:"allowControlPlaneWorkloads"`
}

type K8sDeployNode struct {
	HostID    uint   `json:"hostId"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	HostName  string `json:"hostName"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	MachineID string `json:"machineId"`
	Kernel    string `json:"kernel"`
	Cgroup    string `json:"cgroup"`
}

type K8sDeployPlan struct {
	ID                 uint            `json:"id" gorm:"primaryKey"`
	Name               string          `json:"name" gorm:"size:200;uniqueIndex;not null"`
	Env                string          `json:"env" gorm:"size:64"`
	Description        string          `json:"description" gorm:"size:500"`
	Revision           int             `json:"revision" gorm:"not null;default:1"`
	Config             K8sDeployConfig `json:"config" gorm:"serializer:json;type:text"`
	Nodes              []K8sDeployNode `json:"nodes" gorm:"serializer:json;type:text"`
	Status             string          `json:"status" gorm:"size:32;index"`
	CreatedBy          string          `json:"createdBy" gorm:"size:128"`
	LastRunID          uint            `json:"lastRunId"`
	ClusterID          uint            `json:"clusterId"`
	RegistrationStatus string          `json:"registrationStatus" gorm:"size:32"`
	RegistrationError  string          `json:"registrationError" gorm:"type:text"`
	KubeConfigCipher   string          `json:"-" gorm:"type:text"`
	CreatedAt          time.Time       `json:"createTime"`
	UpdatedAt          time.Time       `json:"updateTime"`
}

func (K8sDeployPlan) TableName() string { return "k8s_deploy_plan" }

type K8sDeployRun struct {
	ID                   uint            `json:"id" gorm:"primaryKey"`
	PlanID               uint            `json:"planId" gorm:"index;not null"`
	Revision             int             `json:"revision"`
	Kind                 string          `json:"kind" gorm:"size:32"`
	Status               string          `json:"status" gorm:"size:32;index"`
	Operator             string          `json:"operator" gorm:"size:128"`
	Config               K8sDeployConfig `json:"config" gorm:"serializer:json;type:text"`
	Nodes                []K8sDeployNode `json:"nodes" gorm:"serializer:json;type:text"`
	Fingerprint          string          `json:"-" gorm:"size:64"`
	Error                string          `json:"error" gorm:"type:text"`
	StopRequested        bool            `json:"stopRequested"`
	Mutated              bool            `json:"mutated"`
	DependenciesPrepared bool            `json:"dependenciesPrepared"`
	PhaseSequence        int             `json:"phaseSequence" gorm:"not null;default:0"`
	StartedAt            time.Time       `json:"startedAt"`
	HeartbeatAt          time.Time       `json:"heartbeatAt" gorm:"index"`
	FinishedAt           *time.Time      `json:"finishedAt"`
}

func (K8sDeployRun) TableName() string { return "k8s_deploy_run" }

type K8sDeployStep struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	RunID      uint       `json:"runId" gorm:"index"`
	HostID     uint       `json:"hostId"`
	HostName   string     `json:"hostName" gorm:"size:128"`
	Title      string     `json:"title" gorm:"size:128"`
	Status     string     `json:"status" gorm:"size:32"`
	Summary    string     `json:"summary" gorm:"type:text"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

func (K8sDeployStep) TableName() string { return "k8s_deploy_step" }

type K8sDeployLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	RunID     uint      `json:"runId" gorm:"index"`
	StepID    uint      `json:"stepId"`
	HostID    uint      `json:"hostId"`
	Content   string    `json:"content" gorm:"type:text"`
	CreatedAt time.Time `json:"createTime"`
}

func (K8sDeployLog) TableName() string { return "k8s_deploy_log" }

// Keep installation ownership even after an uncertain/failed deployment.
type K8sDeployHostLock struct {
	HostID uint `json:"hostId" gorm:"primaryKey"`
	PlanID uint `json:"planId" gorm:"index"`
	RunID  uint `json:"runId"`
}

func (K8sDeployHostLock) TableName() string { return "k8s_deploy_host_lock" }
