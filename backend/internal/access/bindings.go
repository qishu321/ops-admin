package access

import (
	"errors"
	"gorm.io/gorm"
	"ops-admin/backend/model"
)

// Resolve on each request so source edits, disablement and deletion take effect.
// Bindings never inherit administrator roles or traverse other bindings.
func BoundGrants(db *gorm.DB, userID uint) ([]model.ResourceGrant, error) {
	var bindings []model.GroupAccessBinding
	groups := db.Table("sys_user_group_member gm").Select("gm.group_id").Joins("JOIN sys_user_group g ON g.id=gm.group_id AND g.status=1").Where("gm.admin_id=?", userID)
	if err := db.Where("group_id IN (?)", groups).Order("id").Find(&bindings).Error; err != nil {
		return nil, err
	}
	result := []model.ResourceGrant{}
	for _, b := range bindings {
		var grant model.ResourceGrant
		if b.Kind == "policy" {
			err := db.Where("id=? AND (enabled IS NULL OR enabled=?)", b.SourceID, true).First(&grant).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else if b.Kind == "template" {
			var role model.Role
			err := db.Where("id=? AND status=1 AND role_key NOT IN ?", b.SourceID, []string{"super-admin", "member"}).First(&role).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var permissions []string
			if err := db.Table("sys_menu m").Select("DISTINCT m.value").Joins("JOIN sys_role_menu rm ON rm.menu_id=m.id").Where("rm.role_id=? AND m.menu_status=1", role.ID).Pluck("m.value", &permissions).Error; err != nil {
				return nil, err
			}
			grant = model.ResourceGrant{Name: role.RoleName, Unified: true, TemplateID: role.ID, ResourceType: b.ResourceType, ResourceID: b.ResourceID, Scope: b.Scope, Actions: TemplateActions(permissions, role.IsReadOnly)[b.ResourceType]}
			if len(grant.Actions) == 0 {
				continue
			}
		} else {
			continue
		}
		grant.Unified = true
		grant.SubjectType, grant.SubjectID = "group", b.GroupID
		grant.BindingID, grant.BindingKind, grant.SourceID = b.ID, b.Kind, b.SourceID
		result = append(result, grant)
	}
	return result, nil
}
