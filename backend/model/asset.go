package model

import "time"

// AssetService is an operational service discovered from one Kubernetes
// namespace. It is intentionally independent from application-center projects
// and their repository, build, release, and pipeline records.
type AssetService struct {
	ID           uint                   `json:"id" gorm:"primaryKey"`
	Name         string                 `json:"name" gorm:"size:128;not null;index"`
	ServiceUID   string                 `json:"serviceUid" gorm:"size:255;not null;uniqueIndex"`
	K8sClusterID uint                   `json:"k8sClusterId" gorm:"not null;index"`
	Namespace    string                 `json:"namespace" gorm:"size:128;not null;index"`
	ServiceType  string                 `json:"serviceType" gorm:"size:64;index"`
	Status       int                    `json:"status" gorm:"default:1;index"`
	Description  string                 `json:"description" gorm:"size:255"`
	Workloads    []AssetServiceWorkload `json:"workloads" gorm:"foreignKey:ServiceID"`
	CreatedAt    time.Time              `json:"createTime"`
	UpdatedAt    time.Time              `json:"updateTime"`
}

func (AssetService) TableName() string { return "asset_service" }

type AssetServiceWorkload struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ServiceID    uint      `json:"serviceId" gorm:"index;not null;uniqueIndex:idx_asset_service_workload"`
	WorkloadType string    `json:"workloadType" gorm:"size:32;not null;uniqueIndex:idx_asset_service_workload"`
	WorkloadName string    `json:"workloadName" gorm:"size:128;not null;uniqueIndex:idx_asset_service_workload"`
	CreatedAt    time.Time `json:"createTime"`
	UpdatedAt    time.Time `json:"updateTime"`
}

func (AssetServiceWorkload) TableName() string { return "asset_service_workload" }

type AssetHostGroup struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ParentID    uint      `json:"parentId" gorm:"index;default:0"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	Code        string    `json:"code" gorm:"size:128"`
	Sort        int       `json:"sort" gorm:"default:0"`
	Status      int       `json:"status" gorm:"default:1;not null"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (AssetHostGroup) TableName() string {
	return "asset_host_group"
}

type AssetHostGroupRelation struct {
	HostID  uint `json:"hostId" gorm:"primaryKey"`
	GroupID uint `json:"groupId" gorm:"primaryKey"`
}

func (AssetHostGroupRelation) TableName() string {
	return "asset_host_group_rel"
}

type AssetCredential struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	AuthType    string    `json:"authType" gorm:"size:32;not null;index"`
	Username    string    `json:"username" gorm:"size:128;not null"`
	Password    string    `json:"password,omitempty" gorm:"size:512"`
	PrivateKey  string    `json:"privateKey,omitempty" gorm:"type:text"`
	Passphrase  string    `json:"passphrase,omitempty" gorm:"size:512"`
	CACert      string    `json:"caCert,omitempty" gorm:"type:text"`
	ClientCert  string    `json:"clientCert,omitempty" gorm:"type:text"`
	Status      int       `json:"status" gorm:"default:1;not null"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (AssetCredential) TableName() string {
	return "asset_credential"
}

type AssetCloudAccount struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"size:128;not null"`
	Provider  string `json:"provider" gorm:"size:64;not null;index"`
	AccessKey string `json:"accessKey" gorm:"size:255"`
	SecretKey string `json:"secretKey,omitempty" gorm:"size:512"`
	// Regions is the preferred multi-region configuration for host synchronization.
	// Region is kept for existing records and older clients.
	Regions     []string  `json:"regions" gorm:"serializer:json;type:text"`
	Region      string    `json:"region" gorm:"size:128"`
	Status      int       `json:"status" gorm:"default:1;not null"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (AssetCloudAccount) TableName() string {
	return "asset_cloud_account"
}

type AssetGateway struct {
	ID            uint            `json:"id" gorm:"primaryKey"`
	Name          string          `json:"name" gorm:"size:128;not null;index"`
	Code          string          `json:"code" gorm:"size:128;index"`
	GatewayType   string          `json:"gatewayType" gorm:"size:32;not null;default:ssh;index"`
	Host          string          `json:"host" gorm:"size:128;not null;index"`
	Port          int             `json:"port" gorm:"default:22"`
	CredentialID  uint            `json:"credentialId" gorm:"index"`
	Credential    AssetCredential `json:"credential" gorm:"foreignKey:CredentialID"`
	NetworkZone   string          `json:"networkZone" gorm:"size:128;index"`
	Status        int             `json:"status" gorm:"default:1;not null"`
	ConnectStatus int             `json:"connectStatus" gorm:"default:0;not null"`
	Description   string          `json:"description" gorm:"size:255"`
	LastCheckTime *time.Time      `json:"lastCheckTime"`
	CreatedAt     time.Time       `json:"createTime"`
	UpdatedAt     time.Time       `json:"updateTime"`
	HostCount     int64           `json:"hostCount" gorm:"-"`
	DatabaseCount int64           `json:"databaseCount" gorm:"-"`
	ClusterCount  int64           `json:"clusterCount" gorm:"-"`
}

func (AssetGateway) TableName() string {
	return "asset_gateway"
}

type AssetHost struct {
	ID         uint             `json:"id" gorm:"primaryKey"`
	HostName   string           `json:"hostName" gorm:"size:128;not null;index"`
	Alias      string           `json:"alias" gorm:"size:128"`
	GroupID    uint             `json:"groupId" gorm:"index"`
	Group      AssetHostGroup   `json:"group" gorm:"foreignKey:GroupID"`
	HostGroups []AssetHostGroup `json:"hostGroups" gorm:"many2many:asset_host_group_rel;joinForeignKey:HostID;joinReferences:GroupID"`
	// CredentialID is optional: cloud discovery can register a host before SSH credentials are configured.
	CredentialID   *uint             `json:"credentialId" gorm:"index"`
	Credential     AssetCredential   `json:"credential" gorm:"foreignKey:CredentialID"`
	ConnectionMode string            `json:"connectionMode" gorm:"size:32;default:direct;index"`
	GatewayID      *uint             `json:"gatewayId" gorm:"index"`
	Gateway        AssetGateway      `json:"gateway" gorm:"foreignKey:GatewayID"`
	CloudAccountID *uint             `json:"cloudAccountId" gorm:"index"`
	CloudAccount   AssetCloudAccount `json:"cloudAccount" gorm:"foreignKey:CloudAccountID"`
	PrivateIP      string            `json:"privateIp" gorm:"size:64;index"`
	PublicIP       string            `json:"publicIp" gorm:"size:64;index"`
	SSHUser        string            `json:"sshUser" gorm:"size:128"`
	SSHIP          string            `json:"sshIp" gorm:"size:64;index"`
	SSHPort        int               `json:"sshPort" gorm:"default:22"`
	OS             string            `json:"os" gorm:"size:128"`
	Arch           string            `json:"arch" gorm:"size:64"`
	CPU            string            `json:"cpu" gorm:"size:64"`
	Memory         string            `json:"memory" gorm:"size:64"`
	Disk           string            `json:"disk" gorm:"size:64"`
	Environment    string            `json:"environment" gorm:"size:64;index"`
	// Kept only for backwards-compatible database reads; host management no longer exposes tags.
	Tags           []string   `json:"-" gorm:"serializer:json;type:text"`
	Provider       string     `json:"provider" gorm:"size:64"`
	Region         string     `json:"region" gorm:"size:128"`
	InstanceID     string     `json:"instanceId" gorm:"size:128;index"`
	SyncSourceID   *uint      `json:"syncSourceId" gorm:"index"`
	SyncSourceName string     `json:"syncSourceName" gorm:"size:128"`
	SyncProvider   string     `json:"syncProvider" gorm:"size:32"`
	SyncMissing    bool       `json:"syncMissing" gorm:"default:false"`
	LastSyncedAt   *time.Time `json:"lastSyncedAt"`
	Status         int        `json:"status" gorm:"default:1;not null"`
	AliveStatus    int        `json:"aliveStatus" gorm:"default:2;not null"`
	AuthStatus     int        `json:"authStatus" gorm:"default:2;not null"`
	Description    string     `json:"description" gorm:"size:255"`
	LastCheckTime  *time.Time `json:"lastCheckTime"`
	CPUUsage       string     `json:"cpuUsage" gorm:"-"`
	MemoryUsage    string     `json:"memoryUsage" gorm:"-"`
	DiskUsage      string     `json:"diskUsage" gorm:"-"`
	MetricsStatus  string     `json:"metricsStatus" gorm:"-"`
	CreatedAt      time.Time  `json:"createTime"`
	UpdatedAt      time.Time  `json:"updateTime"`
}

func (AssetHost) TableName() string {
	return "asset_host"
}

type AssetDatabase struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	Name           string       `json:"name" gorm:"size:128;not null;index"`
	DBType         string       `json:"dbType" gorm:"size:32;not null;index"`
	Host           string       `json:"host" gorm:"size:128;not null;index"`
	Port           int          `json:"port" gorm:"default:3306"`
	Username       string       `json:"username" gorm:"size:128;not null"`
	Password       string       `json:"password,omitempty" gorm:"size:512"`
	ConnectionMode string       `json:"connectionMode" gorm:"size:32;default:direct;index"`
	GatewayID      *uint        `json:"gatewayId" gorm:"index"`
	Gateway        AssetGateway `json:"gateway" gorm:"foreignKey:GatewayID"`
	DBName         string       `json:"dbName" gorm:"size:128"`
	Charset        string       `json:"charset" gorm:"size:64"`
	Version        string       `json:"version" gorm:"size:128"`
	Env            string       `json:"env" gorm:"size:64;index"`
	Tags           []string     `json:"tags" gorm:"serializer:json;type:text"`
	AccessMode     string       `json:"accessMode" gorm:"size:32;default:readwrite;index"`
	MonitorEnabled bool         `json:"monitorEnabled" gorm:"default:false;not null"`
	Status         int          `json:"status" gorm:"default:1;not null"`
	ConnectStatus  int          `json:"connectStatus" gorm:"default:0;not null"`
	Description    string       `json:"description" gorm:"size:255"`
	LastCheckTime  *time.Time   `json:"lastCheckTime"`
	CreatedAt      time.Time    `json:"createTime"`
	UpdatedAt      time.Time    `json:"updateTime"`
}

// AssetDatabaseMetricSnapshot stores samples collected by the database
// dashboard itself. It deliberately does not depend on an external metrics
// system: each enabled dashboard records a lightweight native database sample.
type AssetDatabaseMetricSnapshot struct {
	ID         uint               `json:"id" gorm:"primaryKey"`
	DatabaseID uint               `json:"databaseId" gorm:"not null;index:idx_db_metric_time"`
	Metrics    map[string]float64 `json:"metrics" gorm:"serializer:json;type:text"`
	CreatedAt  time.Time          `json:"createTime" gorm:"index:idx_db_metric_time"`
}

func (AssetDatabaseMetricSnapshot) TableName() string { return "asset_database_metric_snapshot" }

func (AssetDatabase) TableName() string {
	return "asset_database"
}

type AssetChangeLog struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ResourceType string    `json:"resourceType" gorm:"size:32;not null;index"`
	ResourceID   uint      `json:"resourceId" gorm:"not null;index"`
	ResourceName string    `json:"resourceName" gorm:"size:128;index"`
	Action       string    `json:"action" gorm:"size:32;not null;index"`
	Summary      string    `json:"summary" gorm:"size:512"`
	Operator     string    `json:"operator" gorm:"size:128;index"`
	CreatedAt    time.Time `json:"createTime" gorm:"index"`
}

func (AssetChangeLog) TableName() string {
	return "asset_change_log"
}
