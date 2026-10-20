package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ops-admin/backend/model"
)

func TestJumpServerSignedPaginationAndOrganization(t *testing.T) {
	credential := model.AssetCredential{Username: "11111111-1111-1111-1111-111111111111", Password: "test-only-secret"}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/assets/nodes/" || r.Header.Get("X-JMS-ORG") != "organization-under-test" {
			t.Errorf("wrong endpoint or organization")
		}
		if _, err := http.ParseTime(r.Header.Get("Date")); err != nil {
			t.Error(err)
		}
		message := "(request-target): get " + r.URL.RequestURI() + "\naccept: application/json\ndate: " + r.Header.Get("Date") + "\nhost: " + r.Host
		mac := hmac.New(sha256.New, []byte(credential.Password))
		mac.Write([]byte(message))
		want := fmt.Sprintf(`Signature keyId="%s",algorithm="hmac-sha256",headers="(request-target) accept date host",signature="%s"`, credential.Username, base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		if r.Header.Get("Authorization") != want {
			t.Error("signature did not match request target, host, date and key")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"count":2,"next":"https://untrusted.example/page2","results":[{"id":"n1","key":"1","name":"root"}]}`)
		} else {
			if r.URL.Query().Get("offset") != "1" {
				t.Error("pagination offset did not advance by returned records")
			}
			fmt.Fprint(w, `{"count":2,"next":null,"results":[{"id":"n2","key":"1:1","name":"child"}]}`)
		}
	}))
	defer server.Close()
	nodes, err := jumpServerList[SyncNode](context.Background(), model.AssetSyncSource{URL: server.URL, OrgID: "organization-under-test"}, credential, "/api/v1/assets/nodes/")
	if err != nil || len(nodes) != 2 || calls != 2 {
		t.Fatalf("nodes=%v calls=%d err=%v", nodes, calls, err)
	}
	if err = resolveSyncNodeParents(nodes); err != nil || nodes[1].ParentID != "n1" {
		t.Fatalf("tree=%v err=%v", nodes, err)
	}
}

func TestJumpServerRejectsIncompleteAndRepeatedPages(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"empty early page", `{"count":2,"next":"next","results":[]}`},
		{"missing pagination metadata", `{"results":[]}`},
		{"count smaller than result", `{"count":0,"next":null,"results":[{"id":"a"}]}`},
		{"repeated page", `{"count":5,"next":"next","results":[{"id":"a"}]}`},
		{"invalid json", `<html>login</html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, tc.body) }))
			defer server.Close()
			_, err := jumpServerList[SyncNode](context.Background(), model.AssetSyncSource{URL: server.URL}, model.AssetCredential{}, "/api/v1/assets/nodes/")
			if err == nil || calls > 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestJumpServerNeverFollowsCredentialRedirects(t *testing.T) {
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	_, err := jumpServerList[SyncNode](context.Background(), model.AssetSyncSource{URL: server.URL}, model.AssetCredential{}, "/api/v1/assets/nodes/")
	if err == nil || forwarded != 0 {
		t.Fatalf("redirect followed=%d err=%v", forwarded, err)
	}
}
func TestJumpServerAbortsOnLaterPageFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "0" {
			fmt.Fprint(w, `{"count":2,"next":"next","results":[{"id":"a"}]}`)
		} else {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "sensitive upstream detail")
		}
	}))
	defer server.Close()
	nodes, err := jumpServerList[SyncNode](context.Background(), model.AssetSyncSource{URL: server.URL}, model.AssetCredential{}, "/api/v1/assets/nodes/")
	if err == nil || nodes != nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("partial data must not escape: %v %v", nodes, err)
	}
}
func TestJumpServerRespectsOverallCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := jumpServerList[SyncNode](ctx, model.AssetSyncSource{URL: "http://127.0.0.1:1"}, model.AssetCredential{}, "/api/v1/assets/nodes/")
	if err == nil {
		t.Fatal("canceled snapshot succeeded")
	}
}
func TestSyncNodeScopeAndIncompleteTrees(t *testing.T) {
	nodes := []SyncNode{{ID: "root", Key: "1", Name: "DEFAULT"}, {ID: "bj", Key: "1:1", Name: "北京"}, {ID: "dev", Key: "1:1:1", Name: "开发"}, {ID: "nj", Key: "1:2", Name: "南京"}}
	if err := resolveSyncNodeParents(nodes); err != nil {
		t.Fatal(err)
	}
	scope, err := syncSelectedNodes(model.AssetSyncSource{NodeIDs: []string{"bj"}}, nodes)
	if err != nil || !scope["dev"] || !scope["bj"] || scope["nj"] || scope["root"] {
		t.Fatalf("scope=%v err=%v", scope, err)
	}
	if _, err = syncSelectedNodes(model.AssetSyncSource{NodeIDs: []string{"deleted"}}, nodes); err == nil {
		t.Fatal("missing selected node must fail")
	}
	if err = resolveSyncNodeParents([]SyncNode{{ID: "dev", Key: "1:1", Name: "开发"}}); err == nil {
		t.Fatal("invisible parent must fail")
	}
}
func TestSyncCredentialsInheritDeepestAndRejectAmbiguousPeers(t *testing.T) {
	nodes := []SyncNode{{ID: "root", Key: "1", Name: "root"}, {ID: "a", Key: "1:1", Name: "a"}, {ID: "b", Key: "1:2", Name: "b"}, {ID: "deep", Key: "1:2:1", Name: "deep"}}
	if err := resolveSyncNodeParents(nodes); err != nil {
		t.Fatal(err)
	}
	source := model.AssetSyncSource{DefaultCredentialID: 1, Rules: []model.AssetSyncRule{{NodeID: "a", CredentialID: 2}, {NodeID: "b", CredentialID: 3}, {NodeID: "deep", CredentialID: 4}}}
	for _, refs := range [][]syncReference{{"a", "b", "deep"}, {"deep", "b", "a"}} {
		id, err := syncAssignedCredential(source, SyncAsset{Nodes: refs}, syncNodeIndex(nodes))
		if err != nil || id != 4 {
			t.Fatalf("deepest should win regardless of order: id=%d err=%v", id, err)
		}
	}
	if _, err := syncAssignedCredential(source, SyncAsset{Nodes: []syncReference{"a", "b"}}, syncNodeIndex(nodes)); err == nil {
		t.Fatal("peer credentials require explicit conflict")
	}
	if id, err := syncAssignedCredential(source, SyncAsset{Nodes: []syncReference{"root"}}, syncNodeIndex(nodes)); err != nil || id != 1 {
		t.Fatal("default credential was not inherited")
	}
}
func TestSyncMembershipMovePreservesManualAndPreexistingMappedGroups(t *testing.T) {
	// Groups 1 and 3 are manual, 2 was created by sync; remote moves 2 -> 3/4.
	existing := map[uint]bool{1: true, 2: true, 3: true}
	added, removed, owned := planSyncMemberships(existing, []uint{2}, []uint{3, 4, 4})
	if !reflect.DeepEqual(added, []uint{4}) || !reflect.DeepEqual(removed, []uint{2}) || !reflect.DeepEqual(owned, []uint{4}) {
		t.Fatalf("add=%v remove=%v owned=%v", added, removed, owned)
	}
	// Next move must not remove manually held mapped group 3.
	added, removed, owned = planSyncMemberships(map[uint]bool{1: true, 3: true, 4: true}, owned, []uint{5})
	if !reflect.DeepEqual(added, []uint{5}) || !reflect.DeepEqual(removed, []uint{4}) || !reflect.DeepEqual(owned, []uint{5}) {
		t.Fatalf("add=%v remove=%v owned=%v", added, removed, owned)
	}
}
func TestSyncMembershipRepairsRemovedManagedRelation(t *testing.T) {
	added, removed, owned := planSyncMemberships(map[uint]bool{8: true}, []uint{2}, []uint{2})
	if !reflect.DeepEqual(added, []uint{2}) || len(removed) != 0 || !reflect.DeepEqual(owned, []uint{2}) {
		t.Fatal("managed relation was not restored")
	}
}
func TestSyncExternalReferencesAndFingerprint(t *testing.T) {
	var asset SyncAsset
	if err := json.Unmarshal([]byte(`{"id":"asset1","nodes":["a",{"id":"b"},{"pk":"c"}],"protocols":[{"name":"ssh","port":2222}]}`), &asset); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asset.Nodes, []syncReference{"a", "b", "c"}) || syncAssetPort(asset) != 2222 {
		t.Fatalf("asset=%v", asset)
	}
	before := syncFingerprint(model.AssetSyncSource{}, asset)
	asset.Nodes = []syncReference{"c", "b", "a"}
	if before != syncFingerprint(model.AssetSyncSource{}, asset) {
		t.Fatal("node order changed identity")
	}
	asset.Address = "192.0.2.15"
	if before == syncFingerprint(model.AssetSyncSource{}, asset) {
		t.Fatal("address change was not detected")
	}
}

func TestSyncNodeRenameAndMoveRemainPendingUpdates(t *testing.T) {
	nodes := []SyncNode{{ID: "root", Name: "DEFAULT", Key: "1"}, {ID: "dev", Name: "开发", Key: "1:1", ParentID: "root"}}
	asset := SyncAsset{ID: "host", Nodes: []syncReference{"dev"}}
	source := model.AssetSyncSource{Name: "生产"}
	links := []model.AssetSyncNodeLink{{ExternalID: "__root__", GroupID: 1, Owned: true}, {ExternalID: "root", GroupID: 2, Owned: true}, {ExternalID: "dev", GroupID: 3, Owned: true}}
	groups := []model.AssetHostGroup{{ID: 1, Name: "JumpServer · 生产"}, {ID: 2, Name: "DEFAULT", ParentID: 1}, {ID: 3, Name: "开发", ParentID: 2}}
	changed := syncChangedGroups(source, nodes, links, groups)
	if syncAssetGroupChanged(asset, syncNodeIndex(nodes), changed) {
		t.Fatal("already synchronized groups must not appear as updates")
	}
	nodes[1].Name = "研发"
	changed = syncChangedGroups(source, nodes, links, groups)
	if !syncAssetGroupChanged(asset, syncNodeIndex(nodes), changed) {
		t.Fatal("node rename must remain visible as an update")
	}
	nodes[1].Name = "开发"
	nodes[1].ParentID = ""
	nodes[1].Key = "2"
	changed = syncChangedGroups(source, nodes, links, groups)
	if !syncAssetGroupChanged(asset, syncNodeIndex(nodes), changed) {
		t.Fatal("node move must remain visible as an update")
	}
	source.Rules = []model.AssetSyncRule{{NodeID: "dev", GroupID: 3}}
	links[2].Owned = false
	changed = syncChangedGroups(source, nodes, links, groups)
	if changed["dev"] {
		t.Fatal("mapped manual groups must retain their name and position")
	}
}
func TestSyncAPIKeyCannotBeUsedAsSSHPassword(t *testing.T) {
	if _, err := credentialAuthMethod(model.AssetCredential{AuthType: "jumpserver_key", Password: "test-only-secret"}); err == nil {
		t.Fatal("API key was accepted by SSH")
	}
	for _, raw := range []string{"https://user:pass@example.com", "https://example.com?key=x", "file:///tmp/data", "https://example.com/#fragment"} {
		if _, err := normalizeSyncURL(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	if _, err := normalizeSyncURL("https://jumpserver.example.com/"); err != nil {
		t.Fatal(err)
	}
	if validateSyncCredentialPayload(AssetCredentialPayload{Name: "test", AuthType: "jumpserver_key", Username: "not-a-uuid", Password: "test"}, false) == nil {
		t.Fatal("invalid key id accepted")
	}
}
func TestSyncRecordCountsExcludeMissingAssetsFromCredentialWarnings(t *testing.T) {
	counts := syncCounts([]SyncPreviewItem{{Action: "missing"}, {Action: "skip"}, {Action: "add"}, {Action: "update", CredentialID: 2}, {Action: "conflict"}})
	if counts["pending_credential"] != 2 || counts["conflicts"] != 1 || counts["total"] != 5 {
		t.Fatalf("counts=%v", counts)
	}
}
