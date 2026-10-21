package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func (s *Service) WithAccess(ctx context.Context, p *access.Policy) *Service {
	return &Service{db: s.db.WithContext(access.Context(context.WithoutCancel(ctx), p)), serviceRuntime: s.serviceRuntime}
}
func (s *Service) resourcePolicy() *access.Policy {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return nil
	}
	return access.From(s.db.Statement.Context)
}
func (s *Service) requireResource(kind string, id uint, scope, action string) error {
	return s.resourcePolicy().Require(kind, id, scope, action)
}
func (s *Service) resourceAction(kind, fallback string) string {
	p := s.resourcePolicy()
	if p != nil && p.Actions[kind] != "" {
		return p.Actions[kind]
	}
	return fallback
}

func (s *Service) withResourceOperation(kind, action string) *Service {
	p := s.resourcePolicy()
	if p == nil {
		return s
	}
	child := *p
	child.Actions = make(map[string]string, len(p.Actions)+1)
	for key, value := range p.Actions {
		child.Actions[key] = value
	}
	child.Actions[kind] = action
	return s.WithAccess(s.db.Statement.Context, &child)
}

func (s *Service) authorizeOpsTargets(hostIDs, groupIDs []uint) error {
	p := s.resourcePolicy()
	if p == nil || p.Super {
		return nil
	}
	ids := uniqueIDs(hostIDs)
	if len(groupIDs) > 0 {
		// Resolve the complete group before applying visibility. A batch must
		// not silently execute on only its authorized subset.
		var grouped []uint
		raw := s.db.WithContext(access.Context(context.Background(), nil))
		if err := raw.Table("asset_host h").Select("DISTINCT h.id").Joins("LEFT JOIN asset_host_group_rel rel ON rel.host_id=h.id").Where("h.group_id IN ? OR rel.group_id IN ?", groupIDs, groupIDs).Pluck("h.id", &grouped).Error; err != nil {
			return err
		}
		ids = append(ids, grouped...)
	}
	for _, id := range ids {
		if err := p.Require("host", id, "", s.resourceAction("host", "execute")); err != nil {
			return err
		}
	}
	return nil
}

type UserGroupPayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	MemberIDs   []uint `json:"memberIds"`
}

func (s *Service) ResourceAccessData() (map[string]any, error) {
	var groups []model.UserGroup
	var members []model.UserGroupMember
	var grants []model.ResourceGrant
	var bindings []model.GroupAccessBinding
	if err := s.db.Order("id DESC").Find(&groups).Error; err != nil {
		return nil, err
	}
	if err := s.db.Find(&members).Error; err != nil {
		return nil, err
	}
	if err := s.db.Order("id DESC").Find(&grants).Error; err != nil {
		return nil, err
	}
	if err := s.db.Order("id").Find(&bindings).Error; err != nil {
		return nil, err
	}
	var users []struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Status   int    `json:"status"`
	}
	if err := s.db.Model(&model.Admin{}).Select("id, username, nickname, status").Order("id").Scan(&users).Error; err != nil {
		return nil, err
	}
	var templates []model.Role
	if err := s.db.Where("role_key NOT IN ?", []string{"super-admin", "member"}).Order("id").Find(&templates).Error; err != nil {
		return nil, err
	}
	presets := []map[string]any{}
	for _, t := range templates {
		presets = append(presets, map[string]any{"id": t.ID, "name": t.RoleName, "status": t.Status, "actions": access.TemplateActions(s.CurrentPermissions(t.ID), t.IsReadOnly)})
	}
	return map[string]any{"groups": groups, "members": members, "grants": grants, "users": users, "actions": access.Actions, "templates": presets, "bindings": bindings}, nil
}
func (s *Service) SaveUserGroup(payload UserGroupPayload) error {
	name := strings.TrimSpace(payload.Name)
	if name == "" || len(name) > 128 || len(payload.Description) > 500 {
		return errors.New("请填写用户组名称，名称最多 128 字节，描述最多 500 字节")
	}
	if payload.Status != 1 && payload.Status != 2 {
		return errors.New("用户组状态不正确")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		ids := uniqueIDs(payload.MemberIDs)
		if len(ids) > 0 {
			if err := tx.Model(&model.Admin{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return errors.New("所选用户已不存在，请刷新后重试")
			}
		}
		g := model.UserGroup{}
		if payload.ID > 0 {
			if err := tx.First(&g, payload.ID).Error; err != nil {
				return err
			}
		}
		g.Name = name
		g.Description = strings.TrimSpace(payload.Description)
		g.Status = payload.Status
		if err := tx.Save(&g).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id=?", g.ID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.Create(&model.UserGroupMember{GroupID: g.ID, AdminID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func uniqueIDs(values []uint) []uint {
	result := []uint{}
	seen := map[uint]bool{}
	for _, id := range values {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}
func (s *Service) DeleteUserGroup(id uint) error {
	if id == 0 {
		return errors.New("请选择用户组")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subject_type='group' AND subject_id=?", id).Delete(&model.ResourceGrant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id=?", id).Delete(&model.GroupAccessBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id=?", id).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.UserGroup{}, id).Error
	})
}
func (s *Service) SaveAccessPolicy(g model.ResourceGrant) error {
	g.Unified = true
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len(g.Name) > 128 {
		return errors.New("请填写策略名称，最多 128 字节")
	}
	return s.SaveResourceGrant(g)
}
func (s *Service) SaveResourceGrant(g model.ResourceGrant) error {
	if err := access.ValidateGrant(&g); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if g.TemplateID > 0 {
			var t model.Role
			if err := tx.First(&t, g.TemplateID).Error; err != nil {
				return err
			}
			if t.RoleKey == "super-admin" || t.RoleKey == "member" || t.Status != 1 {
				return errors.New("所选权限模板不可用")
			}
		}
		var count int64
		subject := any(&model.Admin{})
		if g.SubjectType == "group" {
			subject = &model.UserGroup{}
		}
		if err := tx.Model(subject).Where("id=?", g.SubjectID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("授权对象不存在")
		}
		tables := map[string]any{"host": &model.AssetHost{}, "host_group": &model.AssetHostGroup{}, "k8s": &model.K8sCluster{}, "database": &model.AssetDatabase{}, "datasource": &model.MonitorDatasource{}, "credential": &model.AssetCredential{}}
		if err := tx.Model(tables[g.ResourceType]).Where("id=?", g.ResourceID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("所选资源不存在")
		}
		if g.ID > 0 {
			var old model.ResourceGrant
			if err := tx.First(&old, g.ID).Error; err != nil {
				return err
			}
			g.CreatedAt = old.CreatedAt
			if g.Enabled == nil {
				g.Enabled = old.Enabled
			}
			g.Unified = g.Unified || old.Unified
		}
		if g.ResourceType == "datasource" && g.Scope != "" {
			var ds model.MonitorDatasource
			if err := tx.First(&ds, g.ResourceID).Error; err != nil {
				return err
			}
			if normalizeMonitorDatasourceType(ds.Type) != "elasticsearch" {
				return errors.New("指定索引仅适用于 Elasticsearch，其他数据源请按整个实例授权")
			}
		}
		return tx.Save(&g).Error
	})
}
func (s *Service) DeleteResourceGrant(id uint) error {
	if id == 0 {
		return errors.New("请选择授权记录")
	}
	return s.db.Delete(&model.ResourceGrant{}, id).Error
}
func (s *Service) ResourceAccessOptions() (map[string]any, error) {
	result := map[string]any{}
	for _, entry := range []struct{ kind, table, name string }{{"host", "asset_host", "host_name"}, {"host_group", "asset_host_group", "name"}, {"k8s", "k8s_cluster", "name"}, {"database", "asset_database", "name"}, {"datasource", "monitor_datasource", "name"}, {"credential", "asset_credential", "name"}} {
		var list []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		}
		if err := s.db.Table(entry.table).Select("id, " + entry.name + " AS name").Order("id").Scan(&list).Error; err != nil {
			return nil, err
		}
		result[entry.kind] = list
	}
	return result, nil
}

type GroupBindingsPayload struct {
	GroupID  uint                       `json:"groupId"`
	Bindings []model.GroupAccessBinding `json:"bindings"`
}

func (s *Service) SaveGroupBindings(payload GroupBindingsPayload) error {
	if payload.GroupID == 0 || len(payload.Bindings) > 500 {
		return errors.New("请选择用户组，最多绑定 500 项授权")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var group model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, payload.GroupID).Error; err != nil {
			return err
		}
		bindings := []model.GroupAccessBinding{}
		seen := map[string]bool{}
		for _, b := range payload.Bindings {
			b.ID = 0
			b.GroupID = payload.GroupID
			b.Scope = strings.TrimSpace(b.Scope)
			if b.SourceID == 0 {
				return errors.New("请选择绑定来源")
			}
			if b.Kind == "policy" {
				var grant model.ResourceGrant
				if err := tx.First(&grant, b.SourceID).Error; err != nil {
					return errors.New("访问策略已不存在，请刷新后重试")
				}
				if grant.SubjectType == "group" && grant.SubjectID == payload.GroupID {
					return errors.New("该策略已直接授权给当前用户组，无需重复绑定")
				}
				b.ResourceType = ""
				b.ResourceID = 0
				b.Scope = ""
			} else if b.Kind == "template" {
				var role model.Role
				if err := tx.First(&role, b.SourceID).Error; err != nil {
					return errors.New("权限模板已不存在，请刷新后重试")
				}
				if role.RoleKey == "super-admin" || role.RoleKey == "member" {
					return errors.New("该角色不可作为资源权限模板绑定")
				}
				grant := model.ResourceGrant{SubjectType: "group", SubjectID: payload.GroupID, ResourceType: b.ResourceType, ResourceID: b.ResourceID, Scope: b.Scope, Actions: []string{"read"}}
				if err := access.ValidateGrant(&grant); err != nil {
					return err
				}
				b.Scope = grant.Scope
				tables := map[string]any{"host": &model.AssetHost{}, "host_group": &model.AssetHostGroup{}, "k8s": &model.K8sCluster{}, "database": &model.AssetDatabase{}, "datasource": &model.MonitorDatasource{}, "credential": &model.AssetCredential{}}
				var count int64
				if err := tx.Model(tables[b.ResourceType]).Where("id=?", b.ResourceID).Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return errors.New("所选资源不存在")
				}
				if b.ResourceType == "datasource" && b.Scope != "" {
					var ds model.MonitorDatasource
					if err := tx.First(&ds, b.ResourceID).Error; err != nil {
						return err
					}
					if normalizeMonitorDatasourceType(ds.Type) != "elasticsearch" {
						return errors.New("指定索引仅适用于 Elasticsearch")
					}
				}
			} else {
				return errors.New("绑定类型不正确")
			}
			key := fmt.Sprintf("%s:%d:%s:%d:%s", b.Kind, b.SourceID, b.ResourceType, b.ResourceID, b.Scope)
			if !seen[key] {
				bindings = append(bindings, b)
				seen[key] = true
			}
		}
		if err := tx.Where("group_id=?", payload.GroupID).Delete(&model.GroupAccessBinding{}).Error; err != nil {
			return err
		}
		if len(bindings) > 0 {
			return tx.Create(&bindings).Error
		}
		return nil
	})
}
