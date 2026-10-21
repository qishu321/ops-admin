package access

import (
	"slices"
	"testing"

	"ops-admin/backend/model"
)

func TestUnifiedPolicyDoesNotExpandCompatibilityGrants(t *testing.T) {
	p := &Policy{LegacyPermissions: []string{"assets:database:list", "assets:database:workbench"}, Actions: map[string]string{}, Grants: []model.ResourceGrant{
		{ID: 1, ResourceType: "database", ResourceID: 1, Actions: []string{"read", "manage"}},
		{ID: 2, Unified: true, ResourceType: "database", ResourceID: 2, Actions: []string{"read", "write"}},
	}}
	p.Permissions = append(p.Permissions, FeaturePermissions(p.Grants[1])...)
	if !p.Has("assets:database:sql:execute") || !p.Allows("database", 2, "", "write") {
		t.Fatal("new policy must open its own SQL operation")
	}
	if p.Allows("database", 1, "", "write") || p.Allows("database", 1, "", "export") {
		t.Fatal("new policy widened a legacy instance through global feature permissions")
	}
	if !p.Allows("database", 1, "", "query") {
		t.Fatal("legacy read queries must remain available")
	}
	if got := p.OperationIDs("database", "write"); !slices.Equal(got, []uint{2}) {
		t.Fatalf("write SQL filter leaked compatibility resources: %v", got)
	}
}

func TestUnifiedPoliciesKeepActionsOnTheirOwnNamespaces(t *testing.T) {
	p := &Policy{LegacyPermissions: []string{}, Grants: []model.ResourceGrant{
		{ID: 1, Unified: true, ResourceType: "k8s", ResourceID: 5, Scope: "production", Actions: []string{"read", "logs"}},
		{ID: 2, Unified: true, ResourceType: "k8s", ResourceID: 5, Scope: "test", Actions: []string{"read", "terminal", "write"}},
	}}
	for _, g := range p.Grants {
		p.Permissions = append(p.Permissions, FeaturePermissions(g)...)
	}
	if !p.Has("assets:k8s:pod:terminal") || !p.Allows("k8s", 5, "test", "terminal") {
		t.Fatal("terminal policy did not open its feature")
	}
	if p.Allows("k8s", 5, "production", "terminal") || p.Allows("k8s", 5, "production", "write") {
		t.Fatal("actions crossed namespace boundaries")
	}
	p.ReadOnly = true
	if p.Allows("k8s", 5, "test", "terminal") || !p.Allows("k8s", 5, "production", "logs") {
		t.Fatal("global readonly cap was not preserved")
	}
	enabled := false
	p.Grants[0].Enabled = &enabled
	if p.Allows("k8s", 5, "production", "logs") {
		t.Fatal("disabled policy remained effective")
	}
}

func TestTemplatesDoNotInferWriteFromPageOrQueryPermission(t *testing.T) {
	values := []string{"assets:database:workbench", "assets:database:sql:execute", "assets:k8s:pod", "assets:host:terminal"}
	actions := TemplateActions(values, false)
	if !slices.Contains(actions["database"], "query") || slices.Contains(actions["database"], "write") {
		t.Fatalf("query template inferred SQL writes: %v", actions)
	}
	if !slices.Contains(actions["host"], "terminal") || !slices.Contains(actions["k8s"], "logs") {
		t.Fatal("template failed to copy explicit operations")
	}
	actions = TemplateActions(values, true)
	if slices.Contains(actions["host"], "terminal") {
		t.Fatal("readonly template copied terminal access")
	}
	for _, g := range []model.ResourceGrant{{ResourceType: "host", Actions: []string{"read", "manage"}}, {ResourceType: "k8s", Actions: []string{"read", "manage"}}, {ResourceType: "database", Actions: []string{"read", "manage"}}} {
		for _, key := range FeaturePermissions(g) {
			if len(key) >= 7 && key[:7] == "system:" {
				t.Fatalf("resource policy exposed platform administration: %s", key)
			}
		}
	}
}
