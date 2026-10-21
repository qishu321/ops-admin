package service

import (
	"database/sql/driver"
	"errors"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"strings"
	"testing"
)

func TestGroupBindingsResolveMultipleSourcesAndFollowChanges(t *testing.T) {
	enabled, bound, terminal := true, true, true
	state := &systemTestDB{query: func(q string, args []driver.NamedValue) ([]string, [][]driver.Value, error) {
		switch {
		case strings.Contains(q, "sys_group_access_binding"):
			if !strings.Contains(q, "g.status=1") || !strings.Contains(q, "gm.admin_id") {
				return nil, nil, errors.New("inactive group or membership check omitted")
			}
			if !bound {
				return []string{"id"}, nil, nil
			}
			return []string{"id", "group_id", "kind", "source_id", "resource_type", "resource_id", "scope"}, [][]driver.Value{{int64(1), int64(8), "policy", int64(11), "", int64(0), ""}, {int64(2), int64(8), "template", int64(22), "k8s", int64(9), "dev"}}, nil
		case strings.Contains(q, "FROM `sys_admin`"):
			return []string{"id", "username", "status"}, [][]driver.Value{{int64(42), "member", int64(1)}}, nil
		case strings.Contains(q, "sys_admin_role"):
			return []string{"id"}, nil, nil
		case strings.Contains(q, "FROM `sys_role`"):
			if !strings.Contains(q, "status=1") || !strings.Contains(q, "role_key NOT IN") {
				return nil, nil, errors.New("unsafe template lookup")
			}
			return []string{"id", "role_name", "role_key", "status", "is_read_only"}, [][]driver.Value{{int64(22), "开发模板", "developer", int64(1), false}}, nil
		case strings.Contains(q, "sys_menu"):
			if strings.Contains(q, "rm.role_id=?") && terminal {
				return []string{"value"}, [][]driver.Value{{"assets:k8s:pod:terminal"}}, nil
			}
			return []string{"value"}, nil, nil
		case strings.Contains(q, "sys_resource_grant"):
			if strings.Contains(q, "subject_type") {
				return []string{"id"}, nil, nil
			}
			if !enabled {
				return []string{"id"}, nil, nil
			}
			return []string{"id", "name", "subject_type", "subject_id", "resource_type", "resource_id", "scope", "actions", "unified"}, [][]driver.Value{{int64(11), "数据库只读", "user", int64(77), "database", int64(4), "", `["read","query"]`, true}}, nil
		case strings.Contains(q, "asset_host_group"):
			return []string{"id"}, nil, nil
		}
		return nil, nil, errors.New("unexpected query: " + q)
	}}
	db := systemDB(t, state)
	load := func() *access.Policy {
		t.Helper()
		p, err := access.Load(db, 42)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := load()
	if !p.Allows("database", 4, "", "query") || !p.Allows("k8s", 9, "dev", "terminal") || p.Allows("k8s", 9, "prod", "terminal") || p.Super {
		t.Fatal("binding did not preserve operations and explicit resource scopes")
	}
	revision := p.Revision()
	terminal = false
	p = load()
	if p.Allows("k8s", 9, "dev", "terminal") || revision == p.Revision() {
		t.Fatal("template edits must update effective permissions and revision")
	}
	enabled = false
	p = load()
	if p.Allows("database", 4, "", "query") {
		t.Fatal("disabled/deleted source policy remained effective")
	}
	bound = false
	p = load()
	if len(p.Grants) != 0 || p.Allows("k8s", 9, "dev", "read") {
		t.Fatal("unbinding left resource access")
	}
}

func TestInvalidGroupBindingsDoNotReplaceExistingBindings(t *testing.T) {
	for _, kind := range []string{"unknown", "template", "policy"} {
		t.Run(kind, func(t *testing.T) {
			state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
				switch {
				case strings.Contains(q, "sys_user_group"):
					return []string{"id", "status"}, [][]driver.Value{{int64(8), int64(1)}}, nil
				case strings.Contains(q, "sys_role"):
					return []string{"id", "role_key", "status"}, [][]driver.Value{{int64(22), "super-admin", int64(1)}}, nil
				case strings.Contains(q, "sys_resource_grant"):
					return []string{"id"}, nil, nil
				}
				return nil, nil, errors.New("unexpected query: " + q)
			}}
			s := &Service{db: systemDB(t, state)}
			err := s.SaveGroupBindings(GroupBindingsPayload{GroupID: 8, Bindings: []model.GroupAccessBinding{{Kind: kind, SourceID: 22, ResourceType: "k8s", ResourceID: 9}}})
			if err == nil || len(state.writes) != 0 {
				t.Fatalf("invalid source changed bindings: %v %v", err, state.writes)
			}
		})
	}
}
