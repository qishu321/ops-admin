package access

import (
	"slices"
	"testing"

	"ops-admin/backend/model"
)

func TestRevisionIsStableAndDoesNotMutatePolicy(t *testing.T) {
	p := &Policy{UserID: 7, Permissions: []string{"b", "a"}, LegacyPermissions: []string{"x", "y"}, Grants: []model.ResourceGrant{
		{ID: 2, Unified: true, ResourceType: "host", ResourceID: 8, Actions: []string{"terminal", "read"}},
		{ID: 1, Unified: true, ResourceType: "k8s", ResourceID: 9, Scope: "test", Actions: []string{"logs", "read"}},
	}}
	before := p.Revision()
	if !slices.Equal(p.Permissions, []string{"b", "a"}) || p.Grants[0].Actions[0] != "terminal" {
		t.Fatal("revision mutated caller state")
	}
	slices.Reverse(p.Permissions)
	slices.Reverse(p.LegacyPermissions)
	slices.Reverse(p.Grants)
	for i := range p.Grants {
		slices.Reverse(p.Grants[i].Actions)
	}
	if p.Revision() != before {
		t.Fatal("ordering changes must not repeatedly refresh client data")
	}
	p.Grants = append(p.Grants, p.Grants[0])
	if p.Revision() != before {
		t.Fatal("duplicate expansion must not change effective revision")
	}
}

func TestResourceOnlyChangesInvalidateTheRevision(t *testing.T) {
	p := &Policy{UserID: 7, Permissions: []string{"assets:host:list"}, Grants: []model.ResourceGrant{{ID: 1, Unified: true, ResourceType: "host", ResourceID: 8, Actions: []string{"read"}}}}
	before := p.Revision()
	p.Grants = append(p.Grants, model.ResourceGrant{ID: 1, Unified: true, ResourceType: "host", ResourceID: 9, Actions: []string{"read"}})
	if before == p.Revision() {
		t.Fatal("new host group membership did not invalidate access")
	}
	before = p.Revision()
	p.Grants[0].Actions = append(p.Grants[0].Actions, "terminal")
	if before == p.Revision() {
		t.Fatal("operation changes did not invalidate access")
	}
	before = p.Revision()
	p.ReadOnly = true
	if before == p.Revision() {
		t.Fatal("readonly restriction did not invalidate access")
	}
	before = p.Revision()
	p.Grants = nil
	if before == p.Revision() {
		t.Fatal("revocation did not invalidate access")
	}
}
