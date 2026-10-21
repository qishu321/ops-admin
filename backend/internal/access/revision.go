package access

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
)

// Revision describes the effective access snapshot, including expanded host
// groups. Resource-only changes must invalidate clients even when the visible
// page/button permissions are unchanged. No credentials are included.
func (p *Policy) Revision() string {
	if p == nil {
		return ""
	}
	canonical := func(values []string) []string {
		values = slices.Clone(values)
		slices.Sort(values)
		return slices.Compact(values)
	}
	grants := []string{}
	for _, g := range p.Grants {
		value, _ := json.Marshal([]any{g.ID, g.SubjectType, g.SubjectID, g.ResourceType, g.ResourceID, g.Scope, canonical(g.Actions), g.Unified, g.Enabled})
		grants = append(grants, string(value))
	}
	value, _ := json.Marshal([]any{strconv.FormatUint(uint64(p.UserID), 10), p.Super, p.ReadOnly, canonical(p.Permissions), canonical(p.LegacyPermissions), canonical(grants)})
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
