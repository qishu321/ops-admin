package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNavigationPublicOriginsNormalizeMultipleEntrances(t *testing.T) {
	got, err := normalizeNavigationPublicOrigins([]string{" HTTPS://Ops.Example.com:443/ ", "http://192.0.2.10:8080/public/navigation/old-token", "https://ops.example.com", "http://[::1]:8080", ""})
	want := []string{"https://ops.example.com", "http://192.0.2.10:8080", "http://[::1]:8080"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestNavigationPublicOriginsRejectUnsupportedAddresses(t *testing.T) {
	for _, value := range []string{"ops.example.com", "javascript:alert(1)", "https://user:pass@example.com", "https://example.com/?secret=x", "https://example.com/#", "https://example.com/other/path", "http://example.com:99999"} {
		if _, err := normalizeNavigationPublicOrigins([]string{value}); err == nil {
			t.Fatalf("invalid entrance accepted: %s", value)
		}
	}
	if _, err := normalizeNavigationPublicOrigins(make([]string, 11)); err == nil {
		t.Fatal("unbounded origins accepted")
	}
}

func TestNavigationOriginPayloadPreservesOmittedSettingsAndAllowsExplicitClear(t *testing.T) {
	var omitted, cleared IntegrationNavigationGroupPayload
	if err := json.Unmarshal([]byte(`{"id":1,"name":"existing group"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"id":1,"publicOrigins":[]}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if omitted.PublicOrigins != nil || cleared.PublicOrigins == nil || len(*cleared.PublicOrigins) != 0 {
		t.Fatal("older clients must preserve origins; explicit empty array must clear them")
	}
}
