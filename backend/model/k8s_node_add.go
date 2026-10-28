package model

import "time"

type K8sNodeAddSelection struct {
	HostID   uint   `json:"hostId"`
	NodeName string `json:"nodeName"`
}

type K8sNodeAddConfig struct {
	ClusterID        uint                  `json:"clusterId"`
	Nodes            []K8sNodeAddSelection `json:"nodes"`
	Labels           string                `json:"labels"`
	Taints           string                `json:"taints"`
	DisableSELinux   bool                  `json:"disableSELinux"`
	DisableFirewalld bool                  `json:"disableFirewalld"`
	RetryOf          uint                  `json:"retryOf,omitempty"`
}

// Execution, host ownership and logs use the existing deployment run infrastructure.
// Bootstrap tokens are never persisted in this record or in run/log snapshots.
type K8sNodeAddTask struct {
	ID             uint             `gorm:"primaryKey"`
	ClusterID      uint             `gorm:"index;not null"`
	RunID          uint             `gorm:"uniqueIndex;not null"`
	Kind           string           `gorm:"size:24;index"`
	Config         K8sNodeAddConfig `gorm:"serializer:json;type:text"`
	Hosts          []map[string]any `gorm:"serializer:json;type:text"`
	InputHash      string           `gorm:"size:64"`
	ClusterHash    string           `gorm:"size:64"`
	PrecheckID     *uint            `gorm:"uniqueIndex"`
	IdempotencyKey *string          `gorm:"size:128;uniqueIndex"`
	CreatedAt      time.Time
}

func (K8sNodeAddTask) TableName() string { return "k8s_node_add_task" }
