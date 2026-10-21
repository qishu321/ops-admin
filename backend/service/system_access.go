package service

import (
	"errors"
	"slices"

	"gorm.io/gorm"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func accountType(db *gorm.DB, id uint) string {
	var count int64
	db.Table("sys_admin_role ar").Joins("JOIN sys_role r ON r.id=ar.role_id").Where("ar.admin_id=? AND r.role_key=?", id, "super-admin").Count(&count)
	if count > 0 {
		return "administrator"
	}
	return "member"
}

func memberRole(db *gorm.DB) (uint, error) {
	var role model.Role
	err := db.Where("role_key=?", "member").First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		role = model.Role{RoleName: "普通成员", RoleKey: "member", Status: 1, Description: "通过用户组和访问策略获得资源权限"}
		err = db.Create(&role).Error
	}
	return role.ID, err
}

// An omitted account type from an older client preserves its existing role
// selection. Editing a member's profile never replaces custom legacy roles.
func saveAccountRole(tx *gorm.DB, id uint, p AdminPayload, create bool) error {
	roleID := p.RoleID
	if p.AccountType != "" {
		if p.AccountType != "member" && p.AccountType != "administrator" {
			return errors.New("账号类型不正确")
		}
		if !create && accountType(tx, id) == p.AccountType {
			return nil
		}
		if p.AccountType == "administrator" {
			var r model.Role
			if err := tx.Where("role_key=? AND status=1", "super-admin").First(&r).Error; err != nil {
				return err
			}
			roleID = r.ID
		} else {
			var err error
			roleID, err = memberRole(tx)
			if err != nil {
				return err
			}
		}
	}
	if roleID == 0 {
		if !create {
			return nil
		}
		var err error
		roleID, err = memberRole(tx)
		if err != nil {
			return err
		}
	}
	var role model.Role
	if err := tx.Where("id=? AND status=1", roleID).First(&role).Error; err != nil {
		return errors.New("所选账号角色不可用")
	}
	if err := tx.Where("admin_id=?", id).Delete(&model.AdminRole{}).Error; err != nil {
		return err
	}
	return tx.Create(&model.AdminRole{AdminID: id, RoleID: roleID}).Error
}

func replaceUserGroups(tx *gorm.DB, id uint, ids *[]uint) error {
	if ids == nil {
		return nil
	}
	values := uniqueIDs(*ids)
	var count int64
	if len(values) > 0 {
		if err := tx.Model(&model.UserGroup{}).Where("id IN ?", values).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(values)) {
			return errors.New("用户组已不存在，请刷新后重试")
		}
	}
	if err := tx.Where("admin_id=?", id).Delete(&model.UserGroupMember{}).Error; err != nil {
		return err
	}
	for _, gid := range values {
		if err := tx.Create(&model.UserGroupMember{GroupID: gid, AdminID: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) effectiveNavigation(userID uint) ([]map[string]any, []string, string, error) {
	p, err := access.Load(s.db, userID)
	if err != nil {
		return nil, nil, "", err
	}
	if p.Super {
		if err = s.db.Model(&model.Menu{}).Where("menu_status=1 AND value<>''").Pluck("value", &p.Permissions).Error; err != nil {
			return nil, nil, "", err
		}
	}
	return s.menusForPolicy(p), p.Permissions, p.Revision(), nil
}

// Preview uses the same loader and Allows checks as live requests. Inherited
// hosts are kept separate by policy ID, so actions never cross resource scopes.
func (s *Service) EffectiveAccess(userID uint) (map[string]any, error) {
	var user model.Admin
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	result := map[string]any{"user": user, "enabled": user.Status == 1, "super": false, "readOnly": false, "grants": []any{}, "legacyRoles": []model.Role{}}
	var roles []model.Role
	if err := s.db.Table("sys_role r").Select("r.*").Joins("JOIN sys_admin_role ar ON ar.role_id=r.id").Where("ar.admin_id=?", userID).Find(&roles).Error; err != nil {
		return nil, err
	}
	result["legacyRoles"] = roles
	if user.Status != 1 {
		return result, nil
	}
	p, err := access.Load(s.db, userID)
	if err != nil {
		return nil, err
	}
	result["super"], result["readOnly"] = p.Super, p.ReadOnly
	var raw []model.ResourceGrant
	groups := s.db.Table("sys_user_group_member gm").Select("gm.group_id").Joins("JOIN sys_user_group g ON g.id=gm.group_id AND g.status=1").Where("gm.admin_id=?", userID)
	if err := s.db.Where("(enabled IS NULL OR enabled=?)", true).Where("(subject_type='user' AND subject_id=?) OR (subject_type='group' AND subject_id IN (?))", userID, groups).Find(&raw).Error; err != nil {
		return nil, err
	}
	bound, err := access.BoundGrants(s.db, userID)
	if err != nil {
		return nil, err
	}
	raw = append(raw, bound...)
	entries := []map[string]any{}
	for _, g := range raw {
		allowed, blocked := []string{}, []string{}
		for _, a := range access.Actions[g.ResourceType] {
			if a == "manage" || (!slices.Contains(g.Actions, a) && !slices.Contains(g.Actions, "manage")) {
				continue
			}
			if p.Allows(g.ResourceType, g.ResourceID, g.Scope, a) {
				allowed = append(allowed, a)
			} else {
				blocked = append(blocked, a)
			}
		}
		entries = append(entries, map[string]any{"grant": g, "allowed": allowed, "blocked": blocked})
	}
	result["grants"] = entries
	return result, nil
}

func (s *Service) userGroupIDs(id uint) []uint {
	ids := []uint{}
	s.db.Model(&model.UserGroupMember{}).Where("admin_id=?", id).Pluck("group_id", &ids)
	return ids
}
