package access

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ops-admin/backend/model"
)

func TestResourceAuthorizationMatrix(t *testing.T) {
	p := &Policy{Grants: []model.ResourceGrant{
		{ResourceType: "host", ResourceID: 10, Actions: []string{"read", "terminal"}},
		{ResourceType: "k8s", ResourceID: 20, Scope: "dev", Actions: []string{"read", "logs", "restart"}},
		{ResourceType: "database", ResourceID: 30, Actions: []string{"read", "query"}},
		{ResourceType: "datasource", ResourceID: 40, Scope: "dev-logs", Actions: []string{"read", "query"}},
	}}
	for _, tc := range []struct {
		kind          string
		id            uint
		scope, action string
		want          bool
	}{
		{"host", 10, "", "read", true}, {"host", 10, "", "terminal", true}, {"host", 10, "", "upload", false}, {"host", 11, "", "terminal", false},
		{"k8s", 20, "dev", "logs", true}, {"k8s", 20, "prod", "logs", false}, {"k8s", 20, "", "restart", false}, {"k8s", 20, "dev", "restart", true},
		{"database", 30, "", "query", true}, {"database", 30, "", "write", false}, {"database", 30, "", "export", false},
		{"datasource", 40, "dev-logs", "query", true}, {"datasource", 40, "*", "query", false}, {"datasource", 40, "_all", "query", false}, {"datasource", 40, "prod-logs", "query", false},
	} {
		if got := p.Allows(tc.kind, tc.id, tc.scope, tc.action); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
	p.Grants = append(p.Grants, model.ResourceGrant{ResourceType: "host", ResourceID: 10, Actions: []string{"upload"}})
	if !p.Allows("host", 10, "", "upload") || !p.Allows("host", 10, "", "terminal") {
		t.Fatal("independent direct/group grants must add actions")
	}
	p.ReadOnly = true
	if p.Allows("host", 10, "", "terminal") || !p.Allows("database", 30, "", "query") {
		t.Fatal("global read-only must cap resource grants")
	}
	p.Super = true
	if !p.Allows("host", 999, "", "read") || p.Allows("host", 999, "", "execute") {
		t.Fatal("read-only remains a cap even for a combined super role")
	}
}

func TestGrantValidationRejectsScopeAndActionEscapes(t *testing.T) {
	valid := model.ResourceGrant{SubjectType: "group", SubjectID: 1, ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"terminal"}}
	if err := ValidateGrant(&valid); err != nil {
		t.Fatal(err)
	}
	if len(valid.Actions) != 2 || valid.Actions[0] != "read" {
		t.Fatal("all grants must retain inventory visibility")
	}
	for _, edit := range []func(*model.ResourceGrant){
		func(g *model.ResourceGrant) { g.SubjectType = "role" }, func(g *model.ResourceGrant) { g.SubjectID = 0 }, func(g *model.ResourceGrant) { g.ResourceID = 0 },
		func(g *model.ResourceGrant) { g.ResourceType = "unknown" }, func(g *model.ResourceGrant) { g.Scope = "dev/*" }, func(g *model.ResourceGrant) { g.Scope = "dev,prod" },
		func(g *model.ResourceGrant) { g.ResourceType = "host" }, func(g *model.ResourceGrant) { g.Actions = []string{"*"} }, func(g *model.ResourceGrant) { g.Actions = nil },
	} {
		g := valid
		edit(&g)
		if ValidateGrant(&g) == nil {
			t.Fatalf("invalid grant accepted: %+v", g)
		}
	}
}

func TestSQLResourceScopesApplyBeforePaginationCountAndIndirectFiles(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "unused:unused@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = Install(db); err != nil {
		t.Fatal(err)
	}
	p := &Policy{Actions: map[string]string{"database": "export"}, Grants: []model.ResourceGrant{{ResourceType: "host", ResourceID: 7, Actions: []string{"read"}}, {ResourceType: "database", ResourceID: 8, Actions: []string{"read", "export"}}, {ResourceType: "database", ResourceID: 9, Actions: []string{"read"}}}}
	scoped := db.WithContext(Context(context.Background(), p))
	var hosts []model.AssetHost
	stmt := scoped.Where("host_name LIKE ? OR public_ip LIKE ?", "%x%", "%x%").Offset(10).Limit(5).Find(&hosts).Statement
	sql := stmt.SQL.String()
	if !strings.Contains(sql, "`asset_host`.`id` = ?") || !strings.Contains(sql, "(host_name LIKE ? OR public_ip LIKE ?)") {
		t.Fatalf("scope must AND the entire search predicate: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT ? OFFSET ?") {
		t.Fatalf("scope must be part of paginated SQL: %s", sql)
	}
	for _, target := range []any{&model.DatabaseSQLHistory{}, &model.DatabaseTransferTask{}, &model.DatabaseBackupPlan{}, &model.DatabaseBackupRecord{}} {
		var count int64
		stmt := scoped.Model(target).Count(&count).Statement
		if !strings.Contains(stmt.SQL.String(), "database_id` = ?") {
			t.Fatalf("missing auxiliary resource scope: %s", stmt.SQL.String())
		}
		if len(stmt.Vars) != 1 || stmt.Vars[0] != uint(8) {
			t.Fatalf("export must not expose read-only connection 9: %#v", stmt.Vars)
		}
	}
	var clusters []model.K8sCluster
	stmt = scoped.Find(&clusters).Statement
	if !strings.Contains(stmt.SQL.String(), "IN (NULL)") {
		t.Fatalf("an empty grant set must be an empty list: %s", stmt.SQL.String())
	}
	clusterPolicy := &Policy{Actions: map[string]string{"k8s": "read"}, Grants: []model.ResourceGrant{{ID: 1, Unified: true, ResourceType: "k8s", ResourceID: 3, Scope: "dev", Actions: []string{"read"}}}}
	stmt = db.WithContext(Context(context.Background(), clusterPolicy)).Find(&clusters).Statement
	if !strings.Contains(stmt.SQL.String(), "`k8s_cluster`.`id` = ?") || len(stmt.Vars) != 1 || stmt.Vars[0] != uint(3) {
		t.Fatalf("namespace grant must expose only its own cluster in inventory: %s %#v", stmt.SQL.String(), stmt.Vars)
	}
	stmt = db.WithContext(Context(context.Background(), &Policy{Super: true})).Find(&hosts).Statement
	if strings.Contains(stmt.SQL.String(), " IN ") {
		t.Fatal("super administrators should retain full inventory")
	}
}
