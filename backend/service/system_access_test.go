package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ops-admin/backend/internal/access"
)

type systemTestDB struct {
	query  func(string, []driver.NamedValue) ([]string, [][]driver.Value, error)
	writes []string
}
type systemTestConnector struct{ state *systemTestDB }
type systemTestDriver struct{}
type systemTestConn struct{ state *systemTestDB }
type systemTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (c systemTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &systemTestConn{c.state}, nil
}
func (systemTestConnector) Driver() driver.Driver { return systemTestDriver{} }
func (systemTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("connector required")
}
func (c *systemTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *systemTestConn) Close() error              { return nil }
func (c *systemTestConn) Begin() (driver.Tx, error) { return readonlyTx{}, nil }
func (c *systemTestConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	cols, rows, err := c.state.query(q, args)
	return &systemTestRows{columns: cols, rows: rows}, err
}
func (c *systemTestConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.writes = append(c.state.writes, q)
	return driver.RowsAffected(1), nil
}
func (r *systemTestRows) Columns() []string { return r.columns }
func (r *systemTestRows) Close() error      { return nil }
func (r *systemTestRows) Next(v []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(v, r.rows[r.index])
	r.index++
	return nil
}
func systemDB(t *testing.T, state *systemTestDB) *gorm.DB {
	t.Helper()
	conn := sql.OpenDB(systemTestConnector{state})
	t.Cleanup(func() { conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEditingExistingMemberPreservesLegacyRolesAndOmittedGroups(t *testing.T) {
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "sys_admin_role") {
			return nil, nil, errors.New("unexpected query: " + q)
		}
		return []string{"count"}, [][]driver.Value{{int64(0)}}, nil
	}}
	db := systemDB(t, state)
	if err := saveAccountRole(db, 42, AdminPayload{AccountType: "member", RoleID: 99}, false); err != nil {
		t.Fatal(err)
	}
	if err := replaceUserGroups(db, 42, nil); err != nil {
		t.Fatal(err)
	}
	if len(state.writes) != 0 {
		t.Fatalf("profile edit replaced legacy memberships: %v", state.writes)
	}
	empty := []uint{}
	if err := replaceUserGroups(db, 42, &empty); err != nil {
		t.Fatal(err)
	}
	if len(state.writes) != 1 || !strings.Contains(state.writes[0], "DELETE FROM `sys_user_group_member`") {
		t.Fatalf("explicit clearing must clear only group membership: %v", state.writes)
	}
}

func TestGroupValidationFailsBeforeReplacingMembership(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		return []string{"count"}, [][]driver.Value{{int64(1)}}, nil
	}}
	ids := []uint{1, 999}
	if err := replaceUserGroups(systemDB(t, state), 42, &ids); err == nil {
		t.Fatal("missing group accepted")
	}
	if len(state.writes) != 0 {
		t.Fatal("old membership was removed before validating new group IDs")
	}
}

func TestNavigationHasFourSystemEntriesAndInfersResourceAncestors(t *testing.T) {
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "sys_menu") {
			return nil, nil, errors.New("unexpected query: " + q)
		}
		return []string{"id", "parent_id", "menu_name", "menu_type", "url", "value"}, [][]driver.Value{
			{int64(1), int64(0), "系统管理", int64(1), "/system", "system"},
			{int64(2), int64(1), "用户管理", int64(2), "/system/admin", "system:admin:list"},
			{int64(3), int64(1), "用户组", int64(2), "/system/user-groups", "system:usergroup:list"},
			{int64(4), int64(1), "访问策略", int64(2), "/system/access-policies", "system:resourcegrant:list"},
			{int64(5), int64(1), "系统设置", int64(2), "/system/settings", "system:config:view"},
			{int64(6), int64(1), "旧角色", int64(2), "/system/role", "system:role:list"},
			{int64(7), int64(1), "旧菜单", int64(2), "/system/menu", "system:menu:list"},
			{int64(8), int64(1), "已退役页面", int64(2), "/system/dept", "system:dept:list"},
			{int64(9), int64(0), "容器管理", int64(1), "/containers", "containers"},
			{int64(10), int64(9), "Pod 管理", int64(2), "/containers/k8s/pods", "assets:k8s:pod"},
		}, nil
	}}
	s := &Service{db: systemDB(t, state)}
	menus := s.menusForPolicy(&access.Policy{Super: true})
	if len(menus) != 2 || len(menus[0]["menuSvoList"].([]map[string]any)) != 4 {
		t.Fatalf("system sidebar was not simplified: %#v", menus)
	}
	menus = s.menusForPolicy(&access.Policy{Permissions: []string{"assets:k8s:pod", "system:admin:list"}})
	if len(menus) != 1 || menus[0]["url"] != "/containers" {
		t.Fatalf("resource member must see ancestors, not platform administration: %#v", menus)
	}
}

func TestPolicyLoaderWorksWithoutMandatoryRoleAndKeepsLegacyGates(t *testing.T) {
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		switch {
		case strings.Contains(q, "FROM `sys_admin`"):
			return []string{"id", "username", "status"}, [][]driver.Value{{int64(42), "member", int64(1)}}, nil
		case strings.Contains(q, "FROM `sys_role`"):
			return []string{"id", "role_key", "status"}, nil, nil
		case strings.Contains(q, "sys_menu"):
			return []string{"value"}, nil, nil
		case strings.Contains(q, "sys_group_access_binding"):
			return []string{"id"}, nil, nil
		case strings.Contains(q, "sys_resource_grant"):
			if !strings.Contains(q, "enabled") || !strings.Contains(q, "g.status=1") {
				return nil, nil, errors.New("policy query omitted disabled-policy/group filtering")
			}
			return []string{"id", "subject_type", "subject_id", "resource_type", "resource_id", "scope", "actions", "unified", "enabled"}, [][]driver.Value{
				{int64(1), "user", int64(42), "k8s", int64(9), "dev", `["read","terminal"]`, true, true},
				{int64(2), "user", int64(42), "k8s", int64(10), "prod", `["read","manage"]`, false, true},
			}, nil
		case strings.Contains(q, "asset_host_group"):
			return []string{"id", "parent_id", "name"}, nil, nil
		}
		return nil, nil, errors.New("unexpected query: " + q)
	}}
	p, err := access.Load(systemDB(t, state), 42)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Has("assets:k8s:pod:terminal") || !p.Allows("k8s", 9, "dev", "terminal") {
		t.Fatal("a member was forced through a second role assignment")
	}
	if p.Allows("k8s", 10, "prod", "terminal") || len(p.IDs("k8s")) != 1 {
		t.Fatal("unified feature widened an existing compatibility grant")
	}
}
