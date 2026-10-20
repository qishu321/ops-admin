package model

import "time"

// One database row serializes local import transactions across server instances.
type AssetSyncMutex struct {
	ID uint `gorm:"primaryKey"`
}

// AssetSyncSource owns only the external identities and memberships it creates.
// Secrets remain in AssetCredential; a source never stores API or SSH secrets.
type AssetSyncSource struct {
	ID                  uint            `json:"id" gorm:"primaryKey"`
	Name                string          `json:"name" gorm:"size:128;not null"`
	Provider            string          `json:"provider" gorm:"size:32;default:jumpserver;index"`
	CloudAccountID      uint            `json:"cloudAccountId" gorm:"index"`
	Regions             []string        `json:"regions" gorm:"serializer:json;type:text"`
	Environment         string          `json:"environment" gorm:"size:64"`
	ConnectionMode      string          `json:"connectionMode" gorm:"size:32"`
	GatewayID           uint            `json:"gatewayId"`
	AutoCollect         bool            `json:"autoCollect"`
	URL                 string          `json:"url" gorm:"size:512;not null"`
	OrgID               string          `json:"orgId" gorm:"size:64;not null"`
	APICredentialID     uint            `json:"apiCredentialId"`
	DefaultCredentialID uint            `json:"defaultCredentialId"`
	RootGroupID         uint            `json:"rootGroupId"`
	NodeIDs             []string        `json:"nodeIds" gorm:"serializer:json;type:text"`
	Rules               []AssetSyncRule `json:"rules" gorm:"serializer:json;type:text"`
	Version             uint            `json:"version"`
	ActiveRunID         uint            `json:"activeRunId"`
	BusyUntil           *time.Time      `json:"-"`
	CreatedAt           time.Time       `json:"createTime"`
	UpdatedAt           time.Time       `json:"updateTime"`
}

type AssetSyncRule struct {
	NodeID       string `json:"nodeId"`
	GroupID      uint   `json:"groupId"`
	CredentialID uint   `json:"credentialId"`
}

type AssetSyncNodeLink struct {
	ID         uint   `gorm:"primaryKey"`
	SourceID   uint   `gorm:"uniqueIndex:idx_sync_node"`
	ExternalID string `gorm:"size:64;uniqueIndex:idx_sync_node"`
	GroupID    uint
	Owned      bool
}

type AssetSyncHostLink struct {
	ID            uint   `gorm:"primaryKey"`
	SourceID      uint   `gorm:"uniqueIndex:idx_sync_host;uniqueIndex:idx_sync_source_host"`
	ExternalID    string `gorm:"size:64;uniqueIndex:idx_sync_host"`
	HostID        uint   `gorm:"uniqueIndex:idx_sync_source_host"`
	ManagedGroups []uint `gorm:"serializer:json;type:text"`
	DesiredGroups []uint `gorm:"serializer:json;type:text"`
	Fingerprint   string `gorm:"size:64"`
	LastSeenAt    time.Time
	NodeIDs       []string `gorm:"serializer:json;type:text"`
}

type AssetSyncRun struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	SourceID          uint       `json:"sourceId" gorm:"index"`
	SourceVersion     uint       `json:"sourceVersion"`
	Status            string     `json:"status" gorm:"size:32;index"`
	Message           string     `json:"message" gorm:"type:text"`
	Snapshot          string     `json:"-" gorm:"type:longtext"`
	Total             int        `json:"total"`
	Added             int        `json:"added"`
	Updated           int        `json:"updated"`
	Unchanged         int        `json:"unchanged"`
	Conflicts         int        `json:"conflicts"`
	PendingCredential int        `json:"pendingCredential"`
	InventoryStatus   string     `json:"inventoryStatus" gorm:"size:32"`
	CollectStatus     string     `json:"collectStatus" gorm:"size:32"`
	CollectTotal      int        `json:"collectTotal"`
	CollectSuccess    int        `json:"collectSuccess"`
	CollectFailed     int        `json:"collectFailed"`
	CollectSkipped    int        `json:"collectSkipped"`
	CollectAttempt    uint       `json:"collectAttempt"`
	CreatedAt         time.Time  `json:"createTime"`
	FinishedAt        *time.Time `json:"finishTime"`
}
