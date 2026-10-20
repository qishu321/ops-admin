package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ops-admin/backend/model"
)

var syncUUID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

type SyncPreviewItem struct {
	SyncAsset
	Action         string   `json:"action"`
	Reason         string   `json:"reason"`
	HostID         uint     `json:"hostId"`
	CredentialID   uint     `json:"credentialId"`
	GroupPaths     []string `json:"groupPaths"`
	Port           int      `json:"port"`
	CollectStatus  string   `json:"collectStatus,omitempty"`
	CollectMessage string   `json:"collectMessage,omitempty"`
}
type SyncSnapshot struct {
	Nodes []SyncNode        `json:"nodes"`
	Items []SyncPreviewItem `json:"items"`
}
type SyncApplyPayload struct {
	RunID uint            `json:"runId"`
	Links map[string]uint `json:"links"`
}

type AssetSyncHostOption struct {
	ID           uint   `json:"id"`
	HostName     string `json:"hostName"`
	SSHIP        string `json:"sshIp"`
	PrivateIP    string `json:"privateIp"`
	CredentialID *uint  `json:"credentialId"`
}

func (s *Service) AssetSyncHostOptions(keyword string) ([]AssetSyncHostOption, error) {
	var hosts []AssetSyncHostOption
	query := s.db.Select("id", "host_name", "ssh_ip", "private_ip", "credential_id").Model(&model.AssetHost{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("host_name LIKE ? OR ssh_ip LIKE ? OR private_ip LIKE ?", like, like, like)
	}
	err := query.Order("id desc").Limit(50).Find(&hosts).Error
	return hosts, err
}

func validateSyncCredentialPayload(p AssetCredentialPayload, editing bool) error {
	if normalizedAuthType(p.AuthType) != "jumpserver_key" {
		return nil
	}
	if strings.TrimSpace(p.Name) == "" || !syncUUID.MatchString(strings.TrimSpace(p.Username)) {
		return errors.New("JumpServer 凭据需要名称和有效的 Access Key ID（UUID）")
	}
	if !editing && strings.TrimSpace(p.Password) == "" {
		return errors.New("请输入 JumpServer Access Key Secret")
	}
	return nil
}
func (s *Service) checkSyncCredentialReferences(id uint) error {
	var sources []model.AssetSyncSource
	if err := s.db.Find(&sources).Error; err != nil {
		return err
	}
	for _, src := range sources {
		if src.APICredentialID == id || src.DefaultCredentialID == id {
			return errors.New("凭据被资产同步源引用，请先修改同步配置")
		}
		for _, r := range src.Rules {
			if r.CredentialID == id {
				return errors.New("凭据被资产同步分组规则引用，请先修改规则")
			}
		}
	}
	return nil
}
func (s *Service) validateCredentialTypeChange(p AssetCredentialPayload) error {
	if err := validateSyncCredentialPayload(p, true); err != nil {
		return err
	}
	var old model.AssetCredential
	if err := s.db.First(&old, p.ID).Error; err != nil {
		return err
	}
	if normalizedAuthType(old.AuthType) != normalizedAuthType(p.AuthType) {
		if err := s.checkSyncCredentialReferences(p.ID); err != nil {
			return err
		}
		// Changing SSH types also changes the meaning of stored secret fields.
		var count int64
		for _, table := range []any{&model.AssetHost{}, &model.AssetGateway{}} {
			if err := s.db.Model(table).Where("credential_id = ?", p.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("凭据已被资产引用，不能修改认证类型；请创建新凭据")
			}
		}
		if normalizedAuthType(p.AuthType) == "jumpserver_key" && p.Password == "" {
			return errors.New("切换为 JumpServer API 凭据时必须填写 Secret")
		}
		if normalizedAuthType(p.AuthType) == "password" && p.Password == "" {
			return errors.New("切换认证类型时必须填写新密码")
		}
		if normalizedAuthType(p.AuthType) == "key" && p.PrivateKey == "" {
			return errors.New("切换认证类型时必须填写新私钥")
		}
	}
	return nil
}

func (s *Service) syncCredential(id uint, api bool) (model.AssetCredential, error) {
	var c model.AssetCredential
	if id == 0 && !api {
		return c, nil
	}
	if err := s.db.First(&c, id).Error; err != nil {
		return c, errors.New("所选凭据不存在")
	}
	if c.Status != 1 {
		return c, errors.New("所选凭据已停用")
	}
	if api {
		if c.AuthType != "jumpserver_key" || !syncUUID.MatchString(c.Username) || c.Password == "" {
			return c, errors.New("请选择有效的 JumpServer API Key 凭据")
		}
	} else if c.AuthType != "password" && c.AuthType != "key" {
		return c, errors.New("主机登录必须选择 SSH 密码或密钥凭据")
	}
	return c, nil
}

func (s *Service) validateSyncSource(src *model.AssetSyncSource) error {
	src.Provider = assetSyncProvider(src.Provider)
	src.Name = strings.TrimSpace(src.Name)
	src.OrgID = strings.TrimSpace(src.OrgID)
	if src.Name == "" || len(src.Name) > 128 {
		return errors.New("请填写同步源名称（最多 128 字节）")
	}
	var err error
	if src.Provider != "jumpserver" {
		if err = s.validateCloudSyncSource(src); err != nil {
			return err
		}
	} else {
		src.URL, err = normalizeSyncURL(src.URL)
		if err != nil {
			return err
		}
		if !syncUUID.MatchString(src.OrgID) {
			return errors.New("请填写组织 UUID，可在 JumpServer 组织设置中获取")
		}
		if _, err = s.syncCredential(src.APICredentialID, true); err != nil {
			return err
		}
		src.CloudAccountID = 0
		src.Regions = nil
		src.ConnectionMode = "direct"
		src.GatewayID = 0
	}
	if _, err = s.syncCredential(src.DefaultCredentialID, false); err != nil {
		return err
	}
	if src.RootGroupID > 0 {
		var g model.AssetHostGroup
		if err = s.db.First(&g, src.RootGroupID).Error; err != nil {
			return errors.New("目标根主机组不存在")
		}
	}
	seen := map[string]bool{}
	for _, r := range src.Rules {
		if r.NodeID == "" || seen[r.NodeID] {
			return errors.New("分组规则节点为空或重复")
		}
		seen[r.NodeID] = true
		if _, err = s.syncCredential(r.CredentialID, false); err != nil {
			return err
		}
		if r.GroupID > 0 {
			var g model.AssetHostGroup
			if err = s.db.First(&g, r.GroupID).Error; err != nil {
				return errors.New("映射的本地主机组不存在")
			}
		}
	}
	return nil
}

func (s *Service) expireSyncLeases() {
	var sources []model.AssetSyncSource
	if s.db.Where("active_run_id <> 0 AND busy_until < ?", time.Now()).Find(&sources).Error != nil {
		return
	}
	for _, src := range sources {
		_ = s.db.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = ? AND busy_until < ?", src.ID, src.ActiveRunID, time.Now()).Updates(map[string]any{"active_run_id": 0, "busy_until": nil})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			var run model.AssetSyncRun
			if err := tx.First(&run, src.ActiveRunID).Error; err != nil {
				return err
			}
			if run.Status == "collecting" {
				return tx.Model(&run).Updates(map[string]any{"status": "partial", "collect_status": "partial", "message": "资产已入库，信息采集中断或超时；可直接重试采集", "finished_at": time.Now()}).Error
			}
			return tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status IN ?", src.ActiveRunID, []string{"fetching", "running"}).Updates(map[string]any{"status": "failed", "message": "同步任务超时或服务重启，请重新生成预览", "finished_at": time.Now()}).Error
		})
	}
}
func (s *Service) AssetSyncData() (map[string]any, error) {
	s.expireSyncLeases()
	var sources []model.AssetSyncSource
	if err := s.db.Order("id desc").Find(&sources).Error; err != nil {
		return nil, err
	}
	var credentials []model.AssetCredential
	if err := s.db.Where("status = ?", 1).Find(&credentials).Error; err != nil {
		return nil, err
	}
	for i := range credentials {
		maskCredential(&credentials[i])
	}
	accounts, err := s.ListAssetCloudAccountOptions()
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		accounts[i].AccessKey = ""
		accounts[i].SecretKey = ""
	}
	return map[string]any{"sources": sources, "credentials": credentials, "cloudAccounts": accounts}, nil
}
func (s *Service) SaveAssetSyncSource(src model.AssetSyncSource) (*model.AssetSyncSource, error) {
	s.expireSyncLeases()
	if err := s.validateSyncSource(&src); err != nil {
		return nil, err
	}
	if src.ID == 0 {
		src.Version = 1
		src.ActiveRunID = 0
		src.BusyUntil = nil
		src.CreatedAt = time.Time{}
		src.UpdatedAt = time.Time{}
		return &src, s.db.Create(&src).Error
	}
	var old model.AssetSyncSource
	if err := s.db.First(&old, src.ID).Error; err != nil {
		return nil, err
	}
	// Source identity is immutable once assets have been associated.
	var count int64
	if err := s.db.Model(&model.AssetSyncHostLink{}).Where("source_id = ?", src.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 && (old.URL != src.URL || old.OrgID != src.OrgID || assetSyncProvider(old.Provider) != src.Provider || old.CloudAccountID != src.CloudAccountID) {
		return nil, errors.New("同步源已关联资产，来源、云账号、地址或组织变更请创建新的同步源")
	}
	if src.Version != old.Version {
		return nil, errors.New("同步配置已更新，请刷新后再编辑")
	}
	src.Version++
	res := s.db.Model(&old).Where("version = ? AND active_run_id = 0", old.Version).Select("Name", "URL", "OrgID", "Provider", "CloudAccountID", "Regions", "Environment", "ConnectionMode", "GatewayID", "AutoCollect", "APICredentialID", "DefaultCredentialID", "RootGroupID", "NodeIDs", "Rules", "Version").Updates(&src)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("同步源正在执行任务或配置已变化，请稍后重试")
	}
	return &src, s.db.First(&src, src.ID).Error
}
func (s *Service) DeleteAssetSyncSource(id uint) error {
	s.expireSyncLeases()
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND active_run_id = 0", id).Delete(&model.AssetSyncSource{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("同步源不存在或正在同步")
		}
		// Keep imported hosts, groups, and their memberships as ordinary local assets.
		for _, m := range []any{&model.AssetSyncHostLink{}, &model.AssetSyncNodeLink{}, &model.AssetSyncRun{}} {
			if err := tx.Where("source_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.AssetHost{}).Where("sync_source_id = ?", id).Updates(map[string]any{"sync_source_id": nil, "sync_source_name": "", "sync_provider": "", "sync_missing": false}).Error
	})
}

func (s *Service) TestAssetSyncSource(ctx context.Context, id uint) ([]SyncNode, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var src model.AssetSyncSource
	if err := s.db.First(&src, id).Error; err != nil {
		return nil, err
	}
	if assetSyncProvider(src.Provider) != "jumpserver" {
		nodes, _, err := s.fetchCloudSyncAssets(ctx, src)
		return nodes, err
	}
	c, err := s.syncCredential(src.APICredentialID, true)
	if err != nil {
		return nil, err
	}
	nodes, err := jumpServerList[SyncNode](ctx, src, c, "/api/v1/assets/nodes/")
	if err != nil {
		return nil, err
	}
	return nodes, resolveSyncNodeParents(nodes)
}

func (s *Service) claimSyncRun(sourceID uint, run *model.AssetSyncRun, fromPreview bool) (model.AssetSyncSource, error) {
	s.expireSyncLeases()
	var src model.AssetSyncSource
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&src, sourceID).Error; err != nil {
			return err
		}
		if fromPreview {
			if err := tx.First(run, run.ID).Error; err != nil {
				return err
			}
			if run.SourceID != sourceID || run.SourceVersion != src.Version || time.Since(run.CreatedAt) > 30*time.Minute {
				return errors.New("预览已过期或同步配置已变化，请重新生成预览")
			}
			res := tx.Model(run).Where("status = ?", "ready").Update("status", "running")
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errors.New("该预览已执行或尚未就绪")
			}
			run.Status = "running"
		} else {
			run.SourceID = sourceID
			run.SourceVersion = src.Version
			run.Status = "fetching"
			if err := tx.Create(run).Error; err != nil {
				return err
			}
		}
		until := time.Now().Add(5 * time.Minute)
		res := tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = 0 AND version = ?", sourceID, src.Version).Updates(map[string]any{"active_run_id": run.ID, "busy_until": until})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("该同步源已有任务正在执行，请稍后重试")
		}
		return nil
	})
	return src, err
}
func (s *Service) finishSyncRun(runID, sourceID uint, updates map[string]any) {
	attempt, guarded := updates["_collect_attempt"]
	delete(updates, "_collect_attempt")
	updates["finished_at"] = time.Now()
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status IN ?", runID, []string{"fetching", "running", "collecting"})
		if guarded {
			query = query.Where("collect_attempt = ?", attempt)
		}
		res := query.Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if guarded && res.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = ?", sourceID, runID).Updates(map[string]any{"active_run_id": 0, "busy_until": nil}).Error
	})
}
func (s *Service) StartAssetSyncPreview(id uint) (*model.AssetSyncRun, error) {
	run := model.AssetSyncRun{}
	src, err := s.claimSyncRun(id, &run, false)
	if err != nil {
		return nil, err
	}
	go func() {
		updates := map[string]any{"status": "failed", "message": "预览生成失败"}
		defer func() {
			if recover() != nil {
				updates = map[string]any{"status": "failed", "message": "预览生成异常，请检查服务日志"}
			}
			s.finishSyncRun(run.ID, id, updates)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		nodes, assets, e := s.fetchAssetSyncAssets(ctx, src)
		if e != nil {
			updates["message"] = e.Error()
			return
		}
		snapshot, e := s.buildSyncSnapshot(src, nodes, assets)
		if e != nil {
			updates["message"] = e.Error()
			return
		}
		data, e := json.Marshal(snapshot)
		if e != nil {
			updates["message"] = e.Error()
			return
		}
		updates = syncCounts(snapshot.Items)
		updates["status"] = "ready"
		updates["message"] = "预览已就绪，请核对分组、凭据与冲突资产后确认同步"
		updates["snapshot"] = string(data)
	}()
	return &run, nil
}

func syncNodeIndex(nodes []SyncNode) map[string]SyncNode {
	result := map[string]SyncNode{}
	for _, n := range nodes {
		result[n.ID] = n
	}
	return result
}
func syncSelectedNodes(src model.AssetSyncSource, nodes []SyncNode) (map[string]bool, error) {
	index := syncNodeIndex(nodes)
	selected := map[string]bool{}
	for _, id := range src.NodeIDs {
		if _, ok := index[id]; !ok {
			return nil, fmt.Errorf("所选节点 %s 不存在或无权限，请重新选择同步范围", id)
		}
	}
	for _, n := range nodes {
		if len(src.NodeIDs) == 0 {
			selected[n.ID] = true
			continue
		}
		for id := n.ID; id != ""; id = index[id].ParentID {
			for _, root := range src.NodeIDs {
				if id == root {
					selected[n.ID] = true
				}
			}
		}
	}
	return selected, nil
}
func syncAssetPort(a SyncAsset) int {
	for _, p := range a.Protocols {
		if p.Name == "ssh" && p.Port > 0 && p.Port <= 65535 {
			return p.Port
		}
	}
	return 0
}
func syncFingerprint(src model.AssetSyncSource, a SyncAsset) string {
	if assetSyncProvider(src.Provider) != "jumpserver" {
		data, _ := json.Marshal(struct {
			Asset                     SyncAsset
			Rules                     []model.AssetSyncRule
			Root, Credential, Gateway uint
			Environment, Mode         string
		}{a, src.Rules, src.RootGroupID, src.DefaultCredentialID, src.GatewayID, src.Environment, src.ConnectionMode})
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	ids := append([]syncReference{}, a.Nodes...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	a.Nodes = ids
	data, _ := json.Marshal(struct {
		Asset            SyncAsset
		Rules            []model.AssetSyncRule
		Root, Credential uint
	}{a, src.Rules, src.RootGroupID, src.DefaultCredentialID})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Inspect local group metadata independently of the existing host fingerprint.
// This preserves compatibility with already imported hosts while surfacing node
// renames/moves even when the host's node UUIDs have not changed.
func syncChangedGroups(src model.AssetSyncSource, nodes []SyncNode, links []model.AssetSyncNodeLink, groups []model.AssetHostGroup) map[string]bool {
	byNode := map[string]model.AssetSyncNodeLink{}
	byGroup := map[uint]model.AssetHostGroup{}
	rules := map[string]uint{}
	changed := map[string]bool{}
	for _, l := range links {
		byNode[l.ExternalID] = l
	}
	for _, g := range groups {
		byGroup[g.ID] = g
	}
	for _, r := range src.Rules {
		rules[r.NodeID] = r.GroupID
	}
	root := byNode["__root__"]
	rootGroup := byGroup[root.GroupID]
	changed["__root__"] = rootGroup.ID == 0 || rootGroup.Name != syncSourceGroupName(src) || rootGroup.ParentID != src.RootGroupID
	for _, n := range nodes {
		link := byNode[n.ID]
		group := byGroup[link.GroupID]
		if override := rules[n.ID]; override > 0 {
			changed[n.ID] = link.GroupID != override || group.ID == 0
			continue
		}
		parent := root.GroupID
		if n.ParentID != "" {
			parent = byNode[n.ParentID].GroupID
			if rules[n.ParentID] > 0 {
				parent = rules[n.ParentID]
			}
		}
		changed[n.ID] = !link.Owned || group.ID == 0 || group.Name != syncGroupName(n.Name) || group.ParentID != parent
	}
	return changed
}
func syncAssetGroupChanged(a SyncAsset, nodes map[string]SyncNode, changed map[string]bool) bool {
	if changed["__root__"] {
		return true
	}
	for _, ref := range a.Nodes {
		for id := string(ref); id != ""; id = nodes[id].ParentID {
			if changed[id] {
				return true
			}
		}
	}
	return false
}
func syncAssignedCredential(src model.AssetSyncSource, a SyncAsset, nodes map[string]SyncNode) (uint, error) {
	var chosen uint
	depth := -1
	ambiguous := false
	for _, ref := range a.Nodes {
		for id := string(ref); id != ""; id = nodes[id].ParentID {
			for _, r := range src.Rules {
				if r.NodeID != id || r.CredentialID == 0 {
					continue
				}
				d := strings.Count(nodes[id].Key, ":")
				if d > depth {
					depth = d
					chosen = r.CredentialID
					ambiguous = false
				} else if d == depth && chosen != r.CredentialID {
					ambiguous = true
				}
			}
		}
	}
	if ambiguous {
		return 0, errors.New("多个节点配置了不同的同级登录凭据，请调整节点规则")
	}
	if chosen == 0 {
		chosen = src.DefaultCredentialID
	}
	return chosen, nil
}
func (s *Service) classifySyncItem(db *gorm.DB, src model.AssetSyncSource, item *SyncPreviewItem, explicitHost uint) (model.AssetSyncHostLink, model.AssetHost, error) {
	var link model.AssetSyncHostLink
	var host model.AssetHost
	err := db.Where("source_id = ? AND external_id = ?", src.ID, item.ID).First(&link).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return link, host, err
	}
	item.Action = "add"
	item.HostID = 0
	item.Reason = "新资产"
	if link.ID > 0 {
		err = db.First(&host, link.HostID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return link, host, err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			link.HostID = 0
			link.ManagedGroups = nil
		}
		if host.ID > 0 && explicitHost > 0 && explicitHost != host.ID {
			return link, host, errors.New("已有同步关联的资产不能重新绑定其他主机")
		}
	}
	if host.ID == 0 && explicitHost > 0 {
		if err = db.First(&host, explicitHost).Error; err != nil {
			return link, host, errors.New("选择关联的本地主机不存在")
		}
		var count int64
		if err = db.Model(&model.AssetSyncHostLink{}).Where("source_id = ? AND host_id = ? AND id <> ?", src.ID, host.ID, link.ID).Count(&count).Error; err != nil {
			return link, host, err
		}
		if count > 0 {
			return link, host, errors.New("该本地主机已关联同一同步源的其他资产")
		}
	}
	if item.Cloud != nil {
		return s.classifyCloudSyncItem(db, src, item, link, host)
	}
	if host.ID > 0 {
		item.HostID = host.ID
		item.Action = "update"
		item.Reason = "更新资产信息与同步分组，保留已有登录凭据"
		if host.CredentialID != nil {
			item.CredentialID = *host.CredentialID
		}
		status := 2
		if item.Active {
			status = 1
		}
		var relations []model.AssetHostGroupRelation
		if err = db.Where("host_id = ?", host.ID).Find(&relations).Error; err != nil {
			return link, host, err
		}
		groupsPresent := true
		for _, gid := range append(append([]uint{}, link.ManagedGroups...), link.DesiredGroups...) {
			found := false
			for _, r := range relations {
				if r.GroupID == gid {
					found = true
				}
			}
			if !found {
				groupsPresent = false
			}
		}
		if link.Fingerprint == syncFingerprint(src, item.SyncAsset) && host.HostName == item.Name && host.SSHIP == item.Address && host.SSHPort == item.Port && host.Status == status && groupsPresent && (host.CredentialID != nil || item.CredentialID == 0) {
			item.Action = "unchanged"
			item.Reason = "资产信息与分组无变化"
		}
	} else {
		var count int64
		if err = db.Model(&model.AssetHost{}).Where("ssh_ip = ? OR private_ip = ? OR public_ip = ?", item.Address, item.Address, item.Address).Count(&count).Error; err != nil {
			return link, host, err
		}
		if count > 0 {
			item.Action = "conflict"
			item.Reason = "地址与本地已有主机重复，请选择关联主机后同步"
		}
	}
	return link, host, nil
}
func (s *Service) buildSyncSnapshot(src model.AssetSyncSource, nodes []SyncNode, assets []SyncAsset) (SyncSnapshot, error) {
	snapshot := SyncSnapshot{Nodes: nodes, Items: []SyncPreviewItem{}}
	index := syncNodeIndex(nodes)
	selected, err := syncSelectedNodes(src, nodes)
	if err != nil {
		return snapshot, err
	}
	for _, rule := range src.Rules {
		if _, ok := index[rule.NodeID]; !ok {
			return snapshot, errors.New("分组规则中的节点已不存在或不可见，请更新映射规则")
		}
	}
	seen := map[string]bool{}
	addresses := map[string]int{}
	var nodeLinks []model.AssetSyncNodeLink
	if err = s.db.Where("source_id = ?", src.ID).Find(&nodeLinks).Error; err != nil {
		return snapshot, err
	}
	groupIDs := []uint{}
	for _, l := range nodeLinks {
		groupIDs = append(groupIDs, l.GroupID)
	}
	var localGroups []model.AssetHostGroup
	if len(groupIDs) > 0 {
		if err = s.db.Where("id IN ?", groupIDs).Find(&localGroups).Error; err != nil {
			return snapshot, err
		}
	}
	changedGroups := syncChangedGroups(src, nodes, nodeLinks, localGroups)
	for _, a := range assets {
		if a.ID == "" || seen[a.ID] {
			return snapshot, errors.New("JumpServer 返回缺失或重复的资产 ID")
		}
		seen[a.ID] = true
		include := false
		for _, ref := range a.Nodes {
			if _, ok := index[string(ref)]; !ok {
				return snapshot, errors.New("资产引用了不可见节点，请检查节点读取权限")
			}
			if selected[string(ref)] {
				include = true
			}
		}
		if len(src.NodeIDs) == 0 && len(a.Nodes) == 0 {
			include = true
		}
		if !include {
			continue
		}
		item := SyncPreviewItem{SyncAsset: a, Port: syncAssetPort(a), GroupPaths: []string{}}
		for _, ref := range a.Nodes {
			n := index[string(ref)]
			path := n.FullValue
			if path == "" {
				path = n.Name
			}
			item.GroupPaths = append(item.GroupPaths, path)
		}
		item.CredentialID, err = syncAssignedCredential(src, a, index)
		if err != nil {
			item.Action = "conflict"
			item.Reason = err.Error()
		} else {
			_, _, err = s.classifySyncItem(s.db, src, &item, 0)
			if err != nil {
				return snapshot, err
			}
		}
		if item.Port == 0 || strings.TrimSpace(a.Address) == "" {
			item.Action = "skip"
			item.Reason = "缺少 SSH 协议或地址，当前仅同步可配置 SSH 的主机"
		}
		if item.Action == "unchanged" && syncAssetGroupChanged(a, index, changedGroups) {
			item.Action = "update"
			item.Reason = "主机组名称或层级有变化，需要同步"
		}
		snapshot.Items = append(snapshot.Items, item)
		if item.Action != "skip" {
			addresses[a.Address]++
		}
	}
	for i := range snapshot.Items {
		item := &snapshot.Items[i]
		if addresses[item.Address] > 1 && item.HostID == 0 && item.Action != "skip" {
			item.Action = "conflict"
			item.Reason = "源端有多台未关联资产使用同一地址，请选择不同本地主机进行关联"
		}
	}
	// Only a complete, successful fetch can mark previously associated assets as
	// unseen. Scope reductions do not turn excluded assets into missing assets.
	var previous []model.AssetSyncHostLink
	if err = s.db.Where("source_id = ?", src.ID).Find(&previous).Error; err != nil {
		return snapshot, err
	}
	for _, link := range previous {
		if seen[link.ExternalID] {
			continue
		}
		inScope := len(src.NodeIDs) == 0 && assetSyncProvider(src.Provider) == "jumpserver"
		for _, id := range link.NodeIDs {
			if selected[id] {
				inScope = true
			}
		}
		if !inScope {
			continue
		}
		var host model.AssetHost
		if err = s.db.First(&host, link.HostID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		} else if err != nil {
			return snapshot, err
		}
		a := SyncAsset{ID: link.ExternalID, Name: host.HostName, Address: host.SSHIP}
		for _, id := range link.NodeIDs {
			a.Nodes = append(a.Nodes, syncReference(id))
		}
		var credentialID uint
		if host.CredentialID != nil {
			credentialID = *host.CredentialID
		}
		snapshot.Items = append(snapshot.Items, SyncPreviewItem{SyncAsset: a, Action: "missing", Reason: "本次未发现：可能已删除、移动或权限变化；保留本地主机和分组", HostID: host.ID, Port: host.SSHPort, CredentialID: credentialID})
	}
	return snapshot, nil
}
func syncCounts(items []SyncPreviewItem) map[string]any {
	counts := map[string]any{"total": len(items), "added": 0, "updated": 0, "unchanged": 0, "conflicts": 0, "pending_credential": 0}
	for _, item := range items {
		key := ""
		switch item.Action {
		case "add":
			key = "added"
		case "update":
			key = "updated"
		case "unchanged":
			key = "unchanged"
		case "conflict":
			key = "conflicts"
		}
		if key != "" {
			counts[key] = counts[key].(int) + 1
		}
		if item.Action != "skip" && item.Action != "missing" && item.CredentialID == 0 {
			counts["pending_credential"] = counts["pending_credential"].(int) + 1
		}
	}
	return counts
}

func (s *Service) ListAssetSyncRuns(id uint) ([]model.AssetSyncRun, error) {
	s.expireSyncLeases()
	var runs []model.AssetSyncRun
	err := s.db.Omit("snapshot").Where("source_id = ?", id).Order("id desc").Limit(50).Find(&runs).Error
	return runs, err
}
func (s *Service) GetAssetSyncRun(id uint) (map[string]any, error) {
	s.expireSyncLeases()
	var run model.AssetSyncRun
	if err := s.db.First(&run, id).Error; err != nil {
		return nil, err
	}
	snapshot := SyncSnapshot{Nodes: []SyncNode{}, Items: []SyncPreviewItem{}}
	if run.Snapshot != "" {
		if err := json.Unmarshal([]byte(run.Snapshot), &snapshot); err != nil {
			return nil, err
		}
	}
	return map[string]any{"run": run, "nodes": snapshot.Nodes, "items": snapshot.Items}, nil
}

func syncGroupName(name string) string {
	for len(name) > 128 {
		runes := []rune(name)
		name = string(runes[:len(runes)-1])
	}
	return name
}
func ensureSyncGroup(tx *gorm.DB, src model.AssetSyncSource, id, name string, parent uint, override uint) (uint, error) {
	var link model.AssetSyncNodeLink
	err := tx.Where("source_id = ? AND external_id = ?", src.ID, id).First(&link).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if override > 0 {
		var g model.AssetHostGroup
		if err = tx.First(&g, override).Error; err != nil {
			return 0, errors.New("映射的主机组不存在，请重新配置")
		}
		if link.ID == 0 {
			link = model.AssetSyncNodeLink{SourceID: src.ID, ExternalID: id}
		}
		link.GroupID = override
		link.Owned = false
		return override, tx.Save(&link).Error
	}
	var group model.AssetHostGroup
	if link.ID > 0 && link.Owned {
		err = tx.First(&group, link.GroupID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
	}
	if group.ID == 0 {
		group = model.AssetHostGroup{Name: syncGroupName(name), ParentID: parent, Status: 1, Description: "由资产同步管理"}
		if err = tx.Create(&group).Error; err != nil {
			return 0, err
		}
	} else {
		// Mapping beneath an owned descendant would make the local group tree cyclic.
		seen := map[uint]bool{group.ID: true}
		for pid := parent; pid > 0; {
			if seen[pid] {
				return 0, errors.New("分组映射会产生循环，请选择同步组之外的目标根组")
			}
			seen[pid] = true
			var p model.AssetHostGroup
			if err = tx.First(&p, pid).Error; err != nil {
				return 0, err
			}
			pid = p.ParentID
		}
		if err = tx.Model(&group).Updates(map[string]any{"name": syncGroupName(name), "parent_id": parent}).Error; err != nil {
			return 0, err
		}
	}
	if link.ID == 0 {
		link = model.AssetSyncNodeLink{SourceID: src.ID, ExternalID: id}
	}
	link.GroupID = group.ID
	link.Owned = true
	return group.ID, tx.Save(&link).Error
}

func reconcileSyncMemberships(tx *gorm.DB, host *model.AssetHost, link *model.AssetSyncHostLink, desired []uint) error {
	desired = normalizeGroupIDs(desired, 0)
	var relations []model.AssetHostGroupRelation
	if err := tx.Where("host_id = ?", host.ID).Find(&relations).Error; err != nil {
		return err
	}
	existing := map[uint]bool{}
	for _, r := range relations {
		existing[r.GroupID] = true
	}
	// Include the legacy primary group so adoption never discards a manual group.
	if host.GroupID > 0 && !existing[host.GroupID] {
		if err := tx.Create(&model.AssetHostGroupRelation{HostID: host.ID, GroupID: host.GroupID}).Error; err != nil {
			return err
		}
		existing[host.GroupID] = true
	}
	added, removed, nextManaged := planSyncMemberships(existing, link.ManagedGroups, desired)
	// A membership needed by another source must survive this source moving it.
	var otherLinks []model.AssetSyncHostLink
	if err := tx.Where("host_id = ? AND source_id <> ?", host.ID, link.SourceID).Find(&otherLinks).Error; err != nil {
		return err
	}
	protected := map[uint]bool{}
	for _, other := range otherLinks {
		for _, id := range append(append([]uint{}, other.DesiredGroups...), other.ManagedGroups...) {
			protected[id] = true
		}
	}
	for _, id := range removed {
		if protected[id] {
			for i := range otherLinks {
				other := &otherLinks[i]
				needed := false
				for _, gid := range append(append([]uint{}, other.DesiredGroups...), other.ManagedGroups...) {
					if gid == id {
						needed = true
						break
					}
				}
				if needed {
					other.ManagedGroups = normalizeGroupIDs(append(other.ManagedGroups, id), 0)
					if err := tx.Save(other).Error; err != nil {
						return err
					}
					break
				}
			}
			continue
		}
		if err := tx.Where("host_id = ? AND group_id = ?", host.ID, id).Delete(&model.AssetHostGroupRelation{}).Error; err != nil {
			return err
		}
		delete(existing, id)
	}
	for _, id := range added {
		if err := tx.Create(&model.AssetHostGroupRelation{HostID: host.ID, GroupID: id}).Error; err != nil {
			return err
		}
		existing[id] = true
	}
	if !existing[host.GroupID] {
		ids := make([]uint, 0, len(existing))
		for id := range existing {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		host.GroupID = firstGroupID(ids)
	}
	if err := tx.Model(host).Update("group_id", host.GroupID).Error; err != nil {
		return err
	}
	link.ManagedGroups = nextManaged
	link.DesiredGroups = desired
	return nil
}

// A pre-existing manual membership never becomes owned just because a node is
// mapped to the same group. Only this source's old memberships may be removed.
func planSyncMemberships(existing map[uint]bool, managed, desired []uint) (added, removed, nextManaged []uint) {
	wanted := map[uint]bool{}
	owned := map[uint]bool{}
	for _, id := range normalizeGroupIDs(desired, 0) {
		wanted[id] = true
	}
	for _, id := range managed {
		owned[id] = true
		if !wanted[id] && existing[id] {
			removed = append(removed, id)
		}
	}
	for _, id := range normalizeGroupIDs(desired, 0) {
		if !existing[id] {
			added = append(added, id)
			owned[id] = true
		}
		if owned[id] {
			nextManaged = append(nextManaged, id)
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i] < removed[j] })
	return
}

func (s *Service) ApplyAssetSyncPreview(p SyncApplyPayload) (*model.AssetSyncRun, error) {
	var run model.AssetSyncRun
	if err := s.db.First(&run, p.RunID).Error; err != nil {
		return nil, err
	}
	src, err := s.claimSyncRun(run.SourceID, &run, true)
	if err != nil {
		return nil, err
	}
	go func() {
		updates := map[string]any{"status": "failed", "message": "同步执行失败"}
		inventoryCommitted := false
		defer func() {
			if recover() != nil {
				if inventoryCommitted {
					updates = map[string]any{"status": "partial", "inventory_status": "success", "collect_status": "partial", "message": "资产已入库，信息采集中断，可直接重试采集", "_collect_attempt": uint(1)}
				} else {
					updates = map[string]any{"status": "failed", "message": "同步执行异常，请重新预览"}
				}
			}
			s.finishSyncRun(run.ID, src.ID, updates)
		}()
		if e := s.validateSyncSource(&src); e != nil {
			updates["message"] = e.Error()
			return
		}
		var snapshot SyncSnapshot
		if e := json.Unmarshal([]byte(run.Snapshot), &snapshot); e != nil {
			updates["message"] = "预览数据无效，请重新生成"
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Serialize import transactions across sources and server processes.
			// The network fetch is outside this lock; only local reconciliation waits.
			mutex := model.AssetSyncMutex{ID: 1}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&mutex).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&mutex, 1).Error; err != nil {
				return err
			}
			index := syncNodeIndex(snapshot.Nodes)
			selected, e := syncSelectedNodes(src, snapshot.Nodes)
			if e != nil {
				return e
			}
			root, e := ensureSyncGroup(tx, src, "__root__", syncSourceGroupName(src), src.RootGroupID, 0)
			if e != nil {
				return e
			}
			groups := map[string]uint{}
			visiting := map[string]bool{}
			var resolve func(string) (uint, error)
			resolve = func(id string) (uint, error) {
				if id == "" {
					return root, nil
				}
				if gid := groups[id]; gid > 0 {
					return gid, nil
				}
				n, ok := index[id]
				if !ok || visiting[id] {
					return 0, errors.New("节点层级无效")
				}
				visiting[id] = true
				parent, err := resolve(n.ParentID)
				if err != nil {
					return 0, err
				}
				var override uint
				for _, r := range src.Rules {
					if r.NodeID == id {
						override = r.GroupID
					}
				}
				gid, err := ensureSyncGroup(tx, src, id, n.Name, parent, override)
				if err == nil {
					groups[id] = gid
				}
				visiting[id] = false
				return gid, err
			}
			for _, n := range snapshot.Nodes {
				if selected[n.ID] {
					if _, e = resolve(n.ID); e != nil {
						return e
					}
				}
			}
			usedTargets := map[uint]bool{}
			for i := range snapshot.Items {
				item := &snapshot.Items[i]
				if item.Action == "missing" {
					if e = tx.Model(&model.AssetHost{}).Where("id = ? AND sync_source_id = ?", item.HostID, src.ID).Update("sync_missing", true).Error; e != nil {
						return e
					}
					continue
				}
				if item.Action == "skip" {
					continue
				}
				if item.Action == "conflict" && p.Links[item.ID] == 0 {
					continue
				}
				credential, e := syncAssignedCredential(src, item.SyncAsset, index)
				if e != nil {
					item.Action = "conflict"
					item.Reason = e.Error()
					continue
				}
				item.CredentialID = credential
				previewAction := item.Action
				link, host, e := s.classifySyncItem(tx, src, item, p.Links[item.ID])
				if previewAction == "update" && item.Action == "unchanged" {
					item.Action = "update"
				}
				if e != nil {
					return e
				}
				if item.Action == "conflict" {
					continue
				}
				if host.ID > 0 && usedTargets[host.ID] {
					return errors.New("多台资产选择了同一本地主机，请分别关联")
				}
				if item.Port != syncAssetPort(item.SyncAsset) || item.Port == 0 {
					return errors.New("预览协议无效，请重新生成")
				}
				if item.Action == "unchanged" && host.ID > 0 {
					// Acknowledge unchanged inventory without overwriting SSH-collected data.
					if e = tx.Model(&link).Update("last_seen_at", time.Now()).Error; e != nil {
						return e
					}
					provenance := map[string]any{"last_synced_at": time.Now()}
					if host.SyncSourceID == nil || *host.SyncSourceID == src.ID {
						provenance["sync_source_id"] = src.ID
						provenance["sync_source_name"] = src.Name
						provenance["sync_provider"] = assetSyncProvider(src.Provider)
						provenance["sync_missing"] = false
					}
					if e = tx.Model(&host).Updates(provenance).Error; e != nil {
						return e
					}
					usedTargets[host.ID] = true
					continue
				}
				status := 2
				if item.Active {
					status = 1
				}
				wasNew := host.ID == 0
				if host.ID == 0 {
					c, e := s.syncCredential(item.CredentialID, false)
					if e != nil {
						return e
					}
					host = model.AssetHost{HostName: item.Name, SSHIP: item.Address, SSHPort: item.Port, PrivateIP: item.Address, Status: status, AliveStatus: 2, AuthStatus: 2, ConnectionMode: firstNonEmpty(src.ConnectionMode, "direct"), GatewayID: optionalUint(src.GatewayID), Environment: src.Environment, CredentialID: optionalUint(c.ID), SSHUser: c.Username}
					if e = tx.Create(&host).Error; e != nil {
						return e
					}
				} else {
					changes := map[string]any{"host_name": item.Name, "ssh_ip": item.Address, "ssh_port": item.Port, "status": status}
					if item.Cloud != nil {
						changes = cloudHostChanges(src, *item, host)
						if host.SSHPort == 0 {
							changes["ssh_port"] = item.Port
						}
					}
					if item.Cloud == nil && host.PrivateIP == host.SSHIP {
						changes["private_ip"] = item.Address
					}
					if host.CredentialID == nil && item.CredentialID > 0 {
						c, e := s.syncCredential(item.CredentialID, false)
						if e != nil {
							return e
						}
						changes["credential_id"] = c.ID
						changes["ssh_user"] = c.Username
					}
					if changes["ssh_ip"] != host.SSHIP || (item.Cloud == nil && host.SSHPort != item.Port) {
						changes["alive_status"] = 2
						changes["auth_status"] = 2
						changes["last_check_time"] = nil
					}
					if e = tx.Model(&host).Updates(changes).Error; e != nil {
						return e
					}
				}
				item.HostID = host.ID
				if item.Cloud != nil && wasNew {
					if e = tx.Model(&host).Updates(cloudHostChanges(src, *item, host)).Error; e != nil {
						return e
					}
				}
				now := time.Now()
				provenance := map[string]any{"last_synced_at": now}
				if host.SyncSourceID == nil || *host.SyncSourceID == src.ID {
					provenance["sync_source_id"] = src.ID
					provenance["sync_source_name"] = src.Name
					provenance["sync_provider"] = assetSyncProvider(src.Provider)
					provenance["sync_missing"] = false
				}
				if e = tx.Model(&host).Updates(provenance).Error; e != nil {
					return e
				}
				usedTargets[host.ID] = true
				if link.ID == 0 {
					link = model.AssetSyncHostLink{SourceID: src.ID, ExternalID: item.ID}
				}
				desired := []uint{}
				for _, ref := range item.Nodes {
					gid, e := resolve(string(ref))
					if e != nil {
						return e
					}
					desired = append(desired, gid)
				}
				if len(desired) == 0 {
					desired = []uint{root}
				}
				oldGroups := append([]uint{}, link.ManagedGroups...)
				if e = reconcileSyncMemberships(tx, &host, &link, desired); e != nil {
					return e
				}
				if item.Action == "unchanged" && !reflect.DeepEqual(oldGroups, link.ManagedGroups) {
					item.Action = "update"
				}
				link.HostID = host.ID
				link.Fingerprint = syncFingerprint(src, item.SyncAsset)
				link.LastSeenAt = time.Now()
				link.NodeIDs = nil
				for _, id := range item.Nodes {
					link.NodeIDs = append(link.NodeIDs, string(id))
				}
				if e = tx.Save(&link).Error; e != nil {
					return e
				}
				if src.AutoCollect && (item.Action == "add" || item.Action == "update") {
					item.CollectStatus = "pending"
					item.CollectMessage = "等待自动检测并采集"
				}
			}
			updates = syncApplyResult(snapshot)
			updates["inventory_status"] = "success"
			if src.AutoCollect && syncCollectionTotal(snapshot) > 0 {
				updates["status"] = "collecting"
				updates["collect_status"] = "running"
				updates["collect_attempt"] = 1
				updates["collect_total"] = syncCollectionTotal(snapshot)
				updates["message"] = "资产已入库，正在自动检测并采集主机信息"
			} else {
				updates["collect_status"] = "disabled"
			}
			updates["finished_at"] = time.Now()
			if updates["status"] == "collecting" {
				updates["finished_at"] = nil
			}
			res := tx.Model(&model.AssetSyncRun{}).Where("id = ? AND status = ?", run.ID, "running").Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errors.New("同步任务状态已变化，取消入库")
			}
			if updates["status"] == "collecting" {
				return nil
			}
			return tx.Model(&model.AssetSyncSource{}).Where("id = ? AND active_run_id = ?", src.ID, run.ID).Updates(map[string]any{"active_run_id": 0, "busy_until": nil}).Error
		})
		if e != nil {
			updates = map[string]any{"status": "failed", "message": "同步已回滚：" + e.Error()}
			return
		}
		if updates["status"] == "collecting" {
			inventoryCommitted = true
			updates = s.runAssetSyncCollection(src, run.ID, 1, snapshot)
		}
	}()
	return &run, nil
}

func syncApplyResult(snapshot SyncSnapshot) map[string]any {
	updates := syncCounts(snapshot.Items)
	updates["status"] = "success"
	updates["message"] = "资产与主机组同步完成；已有凭据和手工分组已保留"
	if updates["conflicts"].(int) > 0 {
		updates["status"] = "partial"
		updates["message"] = "同步完成，部分冲突资产已跳过；请处理冲突后重新预览"
	}
	data, _ := json.Marshal(snapshot)
	updates["snapshot"] = string(data)
	return updates
}
