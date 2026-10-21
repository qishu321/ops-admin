// Package access defines the platform's resource boundary. Background services
// have no policy; every authenticated HTTP request receives a fresh policy.
package access

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ops-admin/backend/model"
)

var ErrDenied = errors.New("没有访问该资源或执行该操作的权限")

type contextKey struct{}
type Policy struct {
	UserID            uint
	Username          string
	Super             bool
	ReadOnly          bool
	Permissions       []string
	LegacyPermissions []string
	Grants            []model.ResourceGrant
	Actions           map[string]string
}

func From(ctx context.Context) *Policy {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(contextKey{}).(*Policy)
	return p
}
func Context(ctx context.Context, p *Policy) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func (p *Policy) Action(kind string) string {
	if p != nil && p.Actions[kind] != "" {
		return p.Actions[kind]
	}
	return "read"
}
func (p *Policy) Has(permission string) bool {
	return p == nil || p.Super || slices.Contains(p.Permissions, permission)
}
func (p *Policy) Allows(kind string, id uint, scope, action string) bool {
	if p == nil {
		return true
	}
	if p.ReadOnly && action != "read" && action != "query" && action != "logs" {
		return false
	}
	if p.Super {
		return true
	}
	for _, g := range p.Grants {
		if g.ResourceType != kind || g.ResourceID != id {
			continue
		}
		if !p.grantFeatureAllowed(g, action) {
			continue
		}
		if g.Scope != "" && g.Scope != scope {
			continue
		}
		if slices.Contains(g.Actions, action) || slices.Contains(g.Actions, "manage") {
			return true
		}
	}
	return false
}
func (p *Policy) Visible(kind string, id uint) bool {
	if p == nil || p.Super {
		return true
	}
	return slices.Contains(p.IDs(kind), id)
}
func (p *Policy) IDs(kind string) []uint {
	ids := []uint{}
	if p == nil {
		return ids
	}
	for _, g := range p.Grants {
		if g.ResourceType == kind && p.grantFeatureAllowed(g, "read") && !slices.Contains(ids, g.ResourceID) {
			ids = append(ids, g.ResourceID)
		}
	}
	return ids
}

func (p *Policy) OperationIDs(kind, action string) []uint {
	ids := []uint{}
	if p.ReadOnly && action != "read" && action != "query" && action != "logs" {
		return ids
	}
	for _, g := range p.Grants {
		if g.ResourceType == kind && p.grantFeatureAllowed(g, action) && (slices.Contains(g.Actions, action) || slices.Contains(g.Actions, "manage")) && !slices.Contains(ids, g.ResourceID) {
			ids = append(ids, g.ResourceID)
		}
	}
	return ids
}
func (p *Policy) Require(kind string, id uint, scope, action string) error {
	if !p.Allows(kind, id, scope, action) {
		return fmt.Errorf("%w (%s:%s)", ErrDenied, kind, action)
	}
	return nil
}

func (p *Policy) grantFeatureAllowed(g model.ResourceGrant, action string) bool {
	if g.Enabled != nil && !*g.Enabled {
		return false
	}
	return g.Unified || g.ID == 0 || p.LegacyPermissions == nil || FeatureAllows(p.LegacyPermissions, g.ResourceType, action)
}
func Load(db *gorm.DB, id uint) (*Policy, error) {
	var user model.Admin
	if err := db.First(&user, id).Error; err != nil || user.Status != 1 {
		return nil, ErrDenied
	}
	var roles []model.Role
	if err := db.Table("sys_role").Select("sys_role.*").Joins("JOIN sys_admin_role ar ON ar.role_id=sys_role.id").Where("ar.admin_id=? AND sys_role.status=1", id).Find(&roles).Error; err != nil {
		return nil, err
	}
	p := &Policy{UserID: id, Username: user.Username, Actions: map[string]string{}}
	roleIDs := []uint{}
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
		p.Super = p.Super || r.RoleKey == "super-admin"
		p.ReadOnly = p.ReadOnly || r.IsReadOnly
	}
	if err := db.Table("sys_menu m").Select("DISTINCT m.value").Joins("JOIN sys_role_menu rm ON rm.menu_id=m.id").Where("rm.role_id IN ? AND m.menu_status=1", roleIDs).Pluck("m.value", &p.Permissions).Error; err != nil {
		return nil, err
	}
	if p.Super {
		return p, nil
	}
	p.LegacyPermissions = append([]string{}, p.Permissions...)
	groups := db.Table("sys_user_group_member gm").Select("gm.group_id").Joins("JOIN sys_user_group g ON g.id=gm.group_id AND g.status=1").Where("gm.admin_id=?", id)
	if err := db.Where("(enabled IS NULL OR enabled=?)", true).Where("(subject_type='user' AND subject_id=?) OR (subject_type='group' AND subject_id IN (?))", id, groups).Find(&p.Grants).Error; err != nil {
		return nil, err
	}
	bound, err := BoundGrants(db, id)
	if err != nil {
		return nil, err
	}
	p.Grants = append(p.Grants, bound...)
	for _, g := range p.Grants {
		if p.ReadOnly {
			g.Actions = slices.DeleteFunc(slices.Clone(g.Actions), func(a string) bool { return a != "read" && a != "query" && a != "logs" })
		}
		if !g.Unified {
			continue
		}
		for _, key := range FeaturePermissions(g) {
			if !slices.Contains(p.Permissions, key) {
				p.Permissions = append(p.Permissions, key)
			}
		}
	}
	// Resolve host groups once, including descendants. Direct and group grants
	// are combined without widening the actions on either source.
	var hostGroups []model.AssetHostGroup
	if err := db.Find(&hostGroups).Error; err != nil {
		return nil, err
	}
	for _, g := range slices.Clone(p.Grants) {
		if g.ResourceType != "host_group" {
			continue
		}
		groupIDs := []uint{g.ResourceID}
		for changed := true; changed; {
			changed = false
			for _, child := range hostGroups {
				if slices.Contains(groupIDs, child.ParentID) && !slices.Contains(groupIDs, child.ID) {
					groupIDs = append(groupIDs, child.ID)
					changed = true
				}
			}
		}
		var ids []uint
		if err := db.Table("asset_host h").Select("DISTINCT h.id").Joins("LEFT JOIN asset_host_group_rel rel ON rel.host_id=h.id").Where("h.group_id IN ? OR rel.group_id IN ?", groupIDs, groupIDs).Pluck("h.id", &ids).Error; err != nil {
			return nil, err
		}
		for _, hostID := range ids {
			expanded := g
			expanded.ResourceType = "host"
			expanded.ResourceID = hostID
			p.Grants = append(p.Grants, expanded)
		}
	}
	// Expose only the tree paths needed to navigate visible hosts. These
	// derived read grants are added after host expansion and cannot grant SSH.
	var groupIDs []uint
	if ids := p.IDs("host"); len(ids) > 0 {
		if err := db.Model(&model.AssetHostGroupRelation{}).Where("host_id IN ?", ids).Pluck("group_id", &groupIDs).Error; err != nil {
			return nil, err
		}
		var legacy []uint
		if err := db.Model(&model.AssetHost{}).Where("id IN ?", ids).Pluck("group_id", &legacy).Error; err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, legacy...)
	}
	groupIDs = append(groupIDs, p.IDs("host_group")...)
	for changed := true; changed; {
		changed = false
		for _, g := range hostGroups {
			if slices.Contains(groupIDs, g.ID) && g.ParentID > 0 && !slices.Contains(groupIDs, g.ParentID) {
				groupIDs = append(groupIDs, g.ParentID)
				changed = true
			}
		}
	}
	for _, id := range groupIDs {
		if id > 0 {
			p.Grants = append(p.Grants, model.ResourceGrant{ResourceType: "host_group", ResourceID: id, Actions: []string{"read"}})
		}
	}
	return p, nil
}

// Scope is applied before pagination/counts, including preloads and indirect
// asset lookups. It never modifies the shared GORM connection's policy.
func Scope(db *gorm.DB, kind, column string) *gorm.DB {
	p := From(db.Statement.Context)
	if p == nil || p.Super {
		return db
	}
	return db.Where(clause.IN{Column: clause.Column{Name: column}, Values: uintValues(p.IDs(kind))})
}
func uintValues(ids []uint) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
func Install(db *gorm.DB) error {
	apply := func(tx *gorm.DB) {
		p := From(tx.Statement.Context)
		if p == nil || p.Super {
			return
		}
		table := tx.Statement.Table
		kinds := map[string]string{"asset_host": "host", "asset_database": "database", "k8s_cluster": "k8s", "monitor_datasource": "datasource", "asset_host_group": "host_group"}
		if p.Actions["credential"] != "" {
			kinds["asset_credential"] = "credential"
		}
		if kind := kinds[table]; kind != "" {
			ids := p.IDs(kind)
			if action := p.Action(kind); action != "read" {
				ids = p.OperationIDs(kind, action)
			}
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: clause.Column{Table: table, Name: "id"}, Values: uintValues(ids)}}})
		}
		aux := map[string]string{"dbms_sql_history": "database_id", "dbms_transfer_task": "database_id", "dbms_backup_plan": "database_id", "dbms_backup_record": "database_id"}
		if column := aux[table]; column != "" {
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: clause.Column{Table: table, Name: column}, Values: uintValues(p.OperationIDs("database", p.Action("database")))}}})
		}
		if table == "k8s_scaling_policy" {
			conditions := []clause.Expression{}
			for _, g := range p.Grants {
				if g.ResourceType != "k8s" || !(slices.Contains(g.Actions, p.Action("k8s")) || slices.Contains(g.Actions, "manage")) {
					continue
				}
				expr := clause.Expr{SQL: "cluster_id = ?", Vars: []any{g.ResourceID}}
				if g.Scope != "" {
					expr.SQL += " AND namespace = ?"
					expr.Vars = append(expr.Vars, g.Scope)
				}
				conditions = append(conditions, expr)
			}
			if len(conditions) == 0 {
				conditions = append(conditions, clause.Expr{SQL: "1=0"})
			}
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Or(conditions...)}})
		}
		if table == "asset_host_group_rel" {
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: clause.Column{Table: table, Name: "host_id"}, Values: uintValues(p.IDs("host"))}}})
		}
		if table == "ops_exec_task" {
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{Column: clause.Column{Table: table, Name: "operator"}, Value: p.Username}}})
		}
		if table == "ops_exec_target_result" {
			tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "task_id IN (SELECT id FROM ops_exec_task WHERE operator = ?)", Vars: []any{p.Username}}}})
		}
		// Credential/gateway preloads remain available to the connection layer;
		// they are stripped from HTTP responses for ordinary resource users.
	}
	if err := db.Callback().Query().Before("gorm:query").Register("access:query", apply); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("access:row", apply); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("access:update", apply); err != nil {
		return err
	}
	return db.Callback().Delete().Before("gorm:delete").Register("access:delete", apply)
}

var Actions = map[string][]string{
	"host":       {"read", "terminal", "upload", "download", "execute", "manage"},
	"host_group": {"read", "terminal", "upload", "download", "execute", "manage"},
	"k8s":        {"read", "logs", "terminal", "upload", "restart", "scale", "write", "delete", "manage"},
	"database":   {"read", "query", "write", "export", "backup", "restore", "manage"},
	"datasource": {"read", "query", "manage"},
	"credential": {"read", "use", "manage"},
}

func ValidateGrant(g *model.ResourceGrant) error {
	if (g.SubjectType != "user" && g.SubjectType != "group") || g.SubjectID == 0 || g.ResourceID == 0 {
		return errors.New("请选择授权对象与资源")
	}
	allowed, ok := Actions[g.ResourceType]
	if !ok {
		return errors.New("不支持的资源类型")
	}
	g.Scope = strings.TrimSpace(g.Scope)
	if g.ResourceType == "k8s" && g.Scope != "" && (len(g.Scope) > 63 || !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(g.Scope)) {
		return errors.New("Namespace 名称不正确")
	}
	if len(g.Scope) > 255 || strings.ContainsAny(g.Scope, "*?[],/\\\x00\r\n") {
		return errors.New("范围必须是具体的命名空间或索引名称，不能包含通配符或路径")
	}
	if g.ResourceType == "datasource" && g.Scope != "" && (g.Scope == "_all" || g.Scope == "." || g.Scope == ".." || !regexp.MustCompile(`^[\pL\pN._-]+$`).MatchString(g.Scope)) {
		return errors.New("请填写具体的 Elasticsearch 索引名称")
	}
	if g.Scope != "" && g.ResourceType != "k8s" && g.ResourceType != "datasource" {
		return errors.New("仅 K8s 和日志数据源支持子范围")
	}
	if len(g.Actions) == 0 {
		return errors.New("至少选择一项操作")
	}
	for _, a := range g.Actions {
		if !slices.Contains(allowed, a) {
			return errors.New("不支持的操作权限")
		}
	}
	if !slices.Contains(g.Actions, "read") {
		g.Actions = append([]string{"read"}, g.Actions...)
	}
	return nil
}
