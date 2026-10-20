package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

func assetSyncProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "jumpserver":
		return "jumpserver"
	case "alicloud", "aliyun":
		return "aliyun"
	case "tencentcloud", "tencent":
		return "tencent"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
func syncSourceGroupName(src model.AssetSyncSource) string {
	name := map[string]string{"jumpserver": "JumpServer", "aliyun": "阿里云", "tencent": "腾讯云"}[assetSyncProvider(src.Provider)]
	return syncGroupName(name + " · " + src.Name)
}
func (s *Service) validateCloudSyncSource(src *model.AssetSyncSource) error {
	if src.Provider != "aliyun" && src.Provider != "tencent" {
		return errors.New("当前云资产同步支持阿里云和腾讯云")
	}
	var account model.AssetCloudAccount
	if src.CloudAccountID == 0 || s.db.First(&account, src.CloudAccountID).Error != nil {
		return errors.New("请选择已配置的云账号")
	}
	if account.Status != 1 || assetSyncProvider(account.Provider) != src.Provider || account.AccessKey == "" || account.SecretKey == "" {
		return errors.New("云账号已停用、凭证不完整或云厂商不匹配")
	}
	configured := normalizeCloudRegions(account.Regions, account.Region)
	if len(configured) == 0 {
		return errors.New("请先在云账号管理中配置至少一个地域")
	}
	src.Regions = normalizeCloudRegions(src.Regions, "")
	if len(src.Regions) == 0 {
		src.Regions = configured
	}
	allowed := map[string]bool{}
	for _, r := range configured {
		allowed[r] = true
	}
	for _, r := range src.Regions {
		if !allowed[r] {
			return fmt.Errorf("地域 %s 未在该云账号中配置", r)
		}
	}
	src.Environment = normalizeEnvCode(src.Environment)
	var environment model.OpsEnvironment
	if src.Environment == "" || s.db.Where("code = ? AND status = ?", src.Environment, 1).First(&environment).Error != nil {
		return errors.New("请选择可用的所属环境")
	}
	src.ConnectionMode = normalizeConnectionMode(src.ConnectionMode)
	if src.ConnectionMode == "gateway" {
		var gateway model.AssetGateway
		if src.GatewayID == 0 || s.db.Where("id = ? AND status = ?", src.GatewayID, 1).First(&gateway).Error != nil {
			return errors.New("请选择可用的访问网关")
		}
	} else {
		src.GatewayID = 0
	}
	src.URL = ""
	src.OrgID = ""
	src.APICredentialID = 0
	return nil
}
func (s *Service) fetchAssetSyncAssets(ctx context.Context, src model.AssetSyncSource) ([]SyncNode, []SyncAsset, error) {
	if assetSyncProvider(src.Provider) != "jumpserver" {
		return s.fetchCloudSyncAssets(ctx, src)
	}
	c, err := s.syncCredential(src.APICredentialID, true)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := jumpServerList[SyncNode](ctx, src, c, "/api/v1/assets/nodes/")
	if err == nil {
		err = resolveSyncNodeParents(nodes)
	}
	if err != nil {
		return nil, nil, err
	}
	assets, err := jumpServerList[SyncAsset](ctx, src, c, "/api/v1/assets/hosts/")
	return nodes, assets, err
}
func (s *Service) fetchCloudSyncAssets(ctx context.Context, src model.AssetSyncSource) ([]SyncNode, []SyncAsset, error) {
	if err := s.validateCloudSyncSource(&src); err != nil {
		return nil, nil, err
	}
	var account model.AssetCloudAccount
	if err := s.db.First(&account, src.CloudAccountID).Error; err != nil {
		return nil, nil, err
	}
	var instances []cloudInstance
	var err error
	switch src.Provider {
	case "aliyun":
		instances, err = fetchAliyunCloudInstancesWithContext(ctx, account.AccessKey, account.SecretKey, strings.Join(src.Regions, ","))
	case "tencent":
		var found []util.TencentInstanceInfo
		found, err = util.NewTencentCloudService(account.AccessKey, account.SecretKey).GetInstancesWithContext(ctx, src.Regions)
		for _, i := range found {
			instances = append(instances, cloudInstance{InstanceID: i.InstanceID, HostName: i.HostName, PrivateIP: i.PrivateIP, PublicIP: i.PublicIP, CPU: fmt.Sprintf("%d核", i.CPU), Memory: fmt.Sprintf("%dGB", i.Memory), Disk: fmt.Sprintf("%dGB", i.Disk), OS: i.OS, Region: i.Region, SSHPort: 22})
		}
	}
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, nil, fmt.Errorf("云平台读取失败，请检查账号权限、地域及网络：%w", err)
	}
	return cloudSyncAssets(src.Regions, instances)
}

// Cloud IDs are scoped by region as well as the account stored on the source.
func cloudSyncIdentity(region, instance string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(region+"\x00"+instance)))
}
func cloudSyncAssets(regions []string, instances []cloudInstance) ([]SyncNode, []SyncAsset, error) {
	nodes := make([]SyncNode, 0, len(regions))
	allowed := map[string]bool{}
	for i, r := range regions {
		allowed[r] = true
		nodes = append(nodes, SyncNode{ID: "region:" + r, Key: fmt.Sprint(i + 1), Name: r, FullValue: r})
	}
	assets := make([]SyncAsset, 0, len(instances))
	seen := map[string]bool{}
	for _, item := range instances {
		if item.InstanceID == "" || !allowed[item.Region] {
			return nil, nil, errors.New("云平台返回缺失实例 ID 或未配置地域的数据，已中止预览")
		}
		id := cloudSyncIdentity(item.Region, item.InstanceID)
		if seen[id] {
			return nil, nil, errors.New("云平台返回重复实例，已中止预览")
		}
		seen[id] = true
		a := SyncAsset{ID: id, Name: firstNonEmpty(item.HostName, item.InstanceID), Address: firstNonEmpty(item.PrivateIP, item.PublicIP), Active: true, Nodes: []syncReference{syncReference("region:" + item.Region)}, Cloud: &item}
		a.Protocols = append(a.Protocols, struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}{"ssh", defaultSSHPort(item.SSHPort)})
		assets = append(assets, a)
	}
	return nodes, assets, nil
}

func cloudSyncAddress(host model.AssetHost, c cloudInstance) string {
	if host.ID > 0 && host.SSHIP != "" && host.SSHIP != host.PrivateIP && host.SSHIP != host.PublicIP {
		return host.SSHIP
	}
	return firstNonEmpty(c.PrivateIP, c.PublicIP)
}
func cloudIdentityMatches(host model.AssetHost, src model.AssetSyncSource, c cloudInstance) bool {
	if host.CloudAccountID != nil && *host.CloudAccountID != src.CloudAccountID {
		return false
	}
	return host.InstanceID == "" || host.InstanceID == c.InstanceID && (host.Region == "" || host.Region == c.Region)
}
func (s *Service) classifyCloudSyncItem(db *gorm.DB, src model.AssetSyncSource, item *SyncPreviewItem, link model.AssetSyncHostLink, host model.AssetHost) (model.AssetSyncHostLink, model.AssetHost, error) {
	c := *item.Cloud
	if host.ID == 0 {
		var matches []model.AssetHost
		if err := db.Where("cloud_account_id = ? AND instance_id = ? AND (region = ? OR region = '')", src.CloudAccountID, c.InstanceID, c.Region).Limit(2).Find(&matches).Error; err != nil {
			return link, host, err
		}
		if len(matches) > 1 {
			item.Action = "conflict"
			item.Reason = "同一云实例已有多条本地主机记录，请先整理重复资产"
			return link, host, nil
		}
		if len(matches) == 1 {
			host = matches[0]
		}
	}
	if host.ID > 0 {
		if !cloudIdentityMatches(host, src, c) {
			return link, host, errors.New("所选主机已关联不同的云账号或实例，不能覆盖其云身份")
		}
		var count int64
		if err := db.Model(&model.AssetSyncHostLink{}).Where("source_id = ? AND host_id = ? AND external_id <> ?", src.ID, host.ID, item.ID).Count(&count).Error; err != nil {
			return link, host, err
		}
		if count > 0 {
			return link, host, errors.New("该主机已关联同一同步源的另一云实例")
		}
		item.HostID = host.ID
		item.Action = "update"
		item.Reason = "更新云端信息与分组，保留已有登录凭据、网关及环境"
		if host.CredentialID != nil {
			item.CredentialID = *host.CredentialID
		}
		var groups []uint
		if err := db.Model(&model.AssetHostGroupRelation{}).Where("host_id = ?", host.ID).Pluck("group_id", &groups).Error; err != nil {
			return link, host, err
		}
		present := map[uint]bool{}
		for _, g := range groups {
			present[g] = true
		}
		complete := true
		for _, g := range append(append([]uint{}, link.ManagedGroups...), link.DesiredGroups...) {
			if !present[g] {
				complete = false
			}
		}
		if link.ID > 0 && link.Fingerprint == syncFingerprint(src, item.SyncAsset) && complete && host.CloudAccountID != nil && *host.CloudAccountID == src.CloudAccountID && host.InstanceID == c.InstanceID && host.SSHIP == cloudSyncAddress(host, c) && (host.CredentialID != nil || item.CredentialID == 0) {
			item.Action = "unchanged"
			item.Reason = "该实例已同步，云端信息无变化"
		}
	} else {
		var count int64
		if item.Address != "" {
			if err := db.Model(&model.AssetHost{}).Where("ssh_ip = ? OR private_ip = ? OR public_ip = ?", item.Address, item.Address, item.Address).Count(&count).Error; err != nil {
				return link, host, err
			}
		}
		if count > 0 {
			item.Action = "conflict"
			item.Reason = "地址与本地主机重复，请确认并关联已有主机"
		}
	}
	return link, host, nil
}

func cloudHostChanges(src model.AssetSyncSource, item SyncPreviewItem, host model.AssetHost) map[string]any {
	c := *item.Cloud
	updates := map[string]any{"cloud_account_id": src.CloudAccountID, "instance_id": c.InstanceID, "provider": assetSyncProvider(src.Provider), "region": c.Region, "host_name": item.Name, "private_ip": c.PrivateIP, "public_ip": c.PublicIP, "ssh_ip": cloudSyncAddress(host, c)}
	for key, value := range map[string]string{"cpu": c.CPU, "memory": c.Memory, "disk": c.Disk, "os": c.OS} {
		if value != "" {
			updates[key] = value
		}
	}
	if host.ID == 0 || host.Environment == "" {
		updates["environment"] = src.Environment
	}
	if host.ID == 0 || host.ConnectionMode == "" {
		updates["connection_mode"] = src.ConnectionMode
		updates["gateway_id"] = optionalUint(src.GatewayID)
	}
	return updates
}
