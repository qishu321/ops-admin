package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func TestCloudSyncIdentityAndRegions(t *testing.T) {
	instances := []cloudInstance{{InstanceID: "i-1", Region: "cn-shanghai", PrivateIP: "10.0.0.1", PublicIP: "192.0.2.1", SSHPort: 2222}, {InstanceID: "i-1", Region: "cn-beijing", PublicIP: "192.0.2.2"}}
	nodes, assets, err := cloudSyncAssets([]string{"cn-shanghai", "cn-beijing"}, instances)
	if err != nil || len(nodes) != 2 || len(assets) != 2 {
		t.Fatalf("nodes=%v assets=%v err=%v", nodes, assets, err)
	}
	if assets[0].ID == assets[1].ID || assets[0].Address != "10.0.0.1" || assets[1].Address != "192.0.2.2" || syncAssetPort(assets[0]) != 2222 {
		t.Fatalf("wrong instance identity/address: %#v", assets)
	}
	if string(assets[1].Nodes[0]) != nodes[1].ID || assets[0].Cloud.InstanceID != "i-1" {
		t.Fatal("cloud metadata or group mapping lost")
	}
	for _, bad := range [][]cloudInstance{{instances[0], instances[0]}, {{Region: "cn-shanghai"}}, {{InstanceID: "i-2", Region: "unconfigured"}}} {
		if _, result, err := cloudSyncAssets([]string{"cn-shanghai"}, bad); err == nil || result != nil {
			t.Fatal("incomplete/ambiguous cloud snapshot accepted")
		}
	}
}

func TestCloudSyncPreservesConfiguredHostAccess(t *testing.T) {
	src := model.AssetSyncSource{Provider: "tencent", CloudAccountID: 9, Environment: "prod", ConnectionMode: "gateway", GatewayID: 3}
	cloud := cloudInstance{InstanceID: "i-1", Region: "ap-shanghai", PrivateIP: "10.0.0.2", PublicIP: "192.0.2.2", CPU: "8核", Memory: "16GB"}
	item := SyncPreviewItem{SyncAsset: SyncAsset{Name: "cloud-node", Cloud: &cloud}}
	host := model.AssetHost{ID: 1, SSHIP: "custom.internal", PrivateIP: "10.0.0.1", PublicIP: "192.0.2.1", ConnectionMode: "direct", Environment: "test", CredentialID: optionalUint(8), SSHPort: 2222}
	changes := cloudHostChanges(src, item, host)
	if changes["ssh_ip"] != "custom.internal" {
		t.Fatal("custom SSH address overwritten")
	}
	for _, key := range []string{"credential_id", "ssh_user", "ssh_port", "environment", "gateway_id", "connection_mode", "status"} {
		if _, ok := changes[key]; ok {
			t.Fatalf("user config overwritten: %s", key)
		}
	}
	host.SSHIP = host.PrivateIP
	if cloudSyncAddress(host, cloud) != "10.0.0.2" {
		t.Fatal("managed address did not follow cloud IP")
	}
	changes = cloudHostChanges(src, item, model.AssetHost{})
	if changes["environment"] != "prod" || changes["connection_mode"] != "gateway" || changes["cloud_account_id"] != uint(9) {
		t.Fatal(changes)
	}
	for _, tc := range []struct {
		h    model.AssetHost
		want bool
	}{{model.AssetHost{}, true}, {model.AssetHost{CloudAccountID: optionalUint(9), InstanceID: "i-1", Region: "ap-shanghai"}, true}, {model.AssetHost{CloudAccountID: optionalUint(7)}, false}, {model.AssetHost{InstanceID: "i-other"}, false}, {model.AssetHost{InstanceID: "i-1", Region: "other"}, false}} {
		if cloudIdentityMatches(tc.h, src, cloud) != tc.want {
			t.Fatal(tc)
		}
	}
}

func TestCloudFingerprintIncludesSavedAccessConfiguration(t *testing.T) {
	a := SyncAsset{ID: "one", Cloud: &cloudInstance{InstanceID: "i-1", Region: "cn-shanghai"}}
	source := model.AssetSyncSource{Provider: "aliyun"}
	initial := syncFingerprint(source, a)
	for _, changed := range []model.AssetSyncSource{{Provider: "aliyun", Environment: "prod"}, {Provider: "aliyun", GatewayID: 3, ConnectionMode: "gateway"}, {Provider: "aliyun", DefaultCredentialID: 4}, {Provider: "aliyun", RootGroupID: 5}} {
		if syncFingerprint(changed, a) == initial {
			t.Fatal("configuration change hidden from preview")
		}
	}
	if assetSyncProvider("") != "jumpserver" || syncSourceGroupName(model.AssetSyncSource{Name: "prod", Provider: "tencent"}) != "腾讯云 · prod" {
		t.Fatal("legacy provider/group naming changed")
	}
}

func TestCollectionResultRequiresConnectivityAndAuthentication(t *testing.T) {
	for _, tc := range []struct {
		host *model.AssetHost
		want string
	}{{nil, "failed"}, {&model.AssetHost{AliveStatus: 1, AuthStatus: 2}, "failed"}, {&model.AssetHost{AliveStatus: 2, AuthStatus: 1}, "failed"}, {&model.AssetHost{AliveStatus: 1, AuthStatus: 1}, "success"}} {
		status, _ := syncCollectedHostResult(tc.host)
		if status != tc.want {
			t.Fatal("nil error alone must not mean SSH success", tc)
		}
	}
}

func TestCollectionKeepsInventoryAndRetriesOnlyUnsuccessfulHosts(t *testing.T) {
	snapshot := SyncSnapshot{Items: []SyncPreviewItem{{SyncAsset: SyncAsset{ID: "one"}, HostID: 1, Action: "add", CollectStatus: "pending"}, {SyncAsset: SyncAsset{ID: "two"}, HostID: 2, Action: "update", CollectStatus: "pending"}, {HostID: 3, Action: "conflict"}}}
	err := collectSyncSnapshot(context.Background(), &snapshot, func(_ context.Context, i SyncPreviewItem) (string, string) {
		if i.HostID == 2 {
			return "failed", "auth"
		}
		return "success", "ok"
	}, func(current SyncSnapshot) error { return nil })
	if err != nil || snapshot.Items[1].HostID != 2 || snapshot.Items[1].Action != "update" {
		t.Fatal("failed collection destroyed imported asset", err)
	}
	total, success, failed, skipped := syncCollectionCounts(snapshot)
	if total != 2 || success != 1 || failed != 1 || skipped != 0 {
		t.Fatal(total, success, failed, skipped)
	}
	if prepareSyncCollectionRetry(&snapshot) != 1 || snapshot.Items[0].CollectStatus != "success" || snapshot.Items[1].CollectStatus != "pending" || snapshot.Items[2].CollectStatus != "" {
		t.Fatal(snapshot)
	}
	if err = collectSyncSnapshot(context.Background(), &snapshot, func(_ context.Context, i SyncPreviewItem) (string, string) {
		if i.HostID != 2 {
			t.Error("retried successful/conflicting host")
		}
		return "success", "ok"
	}, func(SyncSnapshot) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if prepareSyncCollectionRetry(&snapshot) != 0 {
		t.Fatal("all successful hosts offered for retry")
	}
}

func TestCollectionConcurrencyPanicAndPersistenceFailure(t *testing.T) {
	snapshot := SyncSnapshot{}
	for i := 0; i < 13; i++ {
		snapshot.Items = append(snapshot.Items, SyncPreviewItem{HostID: uint(i + 1), CollectStatus: "pending"})
	}
	var inFlight, peak atomic.Int32
	err := collectSyncSnapshot(context.Background(), &snapshot, func(_ context.Context, i SyncPreviewItem) (string, string) {
		if i.HostID == 1 {
			panic("network adapter")
		}
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		return "success", fmt.Sprint(i.HostID)
	}, func(SyncSnapshot) error { return nil })
	if err != nil || peak.Load() > 4 || snapshot.Items[0].CollectStatus != "failed" {
		t.Fatalf("peak=%d snapshot=%v err=%v", peak.Load(), snapshot, err)
	}
	_, success, failed, _ := syncCollectionCounts(snapshot)
	if success != 12 || failed != 1 {
		t.Fatal(success, failed)
	}
	before := snapshot
	stop := errors.New("task superseded")
	err = collectSyncSnapshot(context.Background(), &snapshot, func(context.Context, SyncPreviewItem) (string, string) {
		t.Fatal("persistence failed before network")
		return "", ""
	}, func(SyncSnapshot) error { return stop })
	if !errors.Is(err, stop) || !reflect.DeepEqual(before, snapshot) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot.Items[0].CollectStatus = "pending"
	if err = collectSyncSnapshot(ctx, &snapshot, func(ctx context.Context, _ SyncPreviewItem) (string, string) { <-ctx.Done(); return "failed", "cancel" }, func(SyncSnapshot) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
