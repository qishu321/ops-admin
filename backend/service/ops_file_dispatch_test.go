package service

import "testing"

func TestResolveOpsDispatchTargetPathUsesSourceNameInTargetDirectory(t *testing.T) {
	got, err := resolveOpsDispatchTargetPath(OpsFileDispatchPayload{TargetDir: "/home/testvm/tmp/robot"}, "szfc-robot.tar.gz")
	if err != nil {
		t.Fatalf("resolveOpsDispatchTargetPath() error = %v", err)
	}
	if want := "/home/testvm/tmp/robot/szfc-robot.tar.gz"; got != want {
		t.Fatalf("resolveOpsDispatchTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveOpsDispatchTargetPathAllowsExplicitRename(t *testing.T) {
	got, err := resolveOpsDispatchTargetPath(OpsFileDispatchPayload{TargetDir: "/opt/release", TargetFileName: "app-current.tar.gz"}, "szfc-robot.tar.gz")
	if err != nil {
		t.Fatalf("resolveOpsDispatchTargetPath() error = %v", err)
	}
	if want := "/opt/release/app-current.tar.gz"; got != want {
		t.Fatalf("resolveOpsDispatchTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveOpsDispatchTargetPathRetainsLegacyFullPath(t *testing.T) {
	got, err := resolveOpsDispatchTargetPath(OpsFileDispatchPayload{TargetPath: "/opt/release/app.tar.gz"}, "ignored.tar.gz")
	if err != nil {
		t.Fatalf("resolveOpsDispatchTargetPath() error = %v", err)
	}
	if want := "/opt/release/app.tar.gz"; got != want {
		t.Fatalf("resolveOpsDispatchTargetPath() = %q, want %q", got, want)
	}
}

func TestResolveOpsDispatchTargetPathRejectsFilePathAsName(t *testing.T) {
	_, err := resolveOpsDispatchTargetPath(OpsFileDispatchPayload{TargetDir: "/opt/release", TargetFileName: "nested/app.tar.gz"}, "source.tar.gz")
	if err == nil {
		t.Fatal("resolveOpsDispatchTargetPath() error = nil, want invalid filename error")
	}
}
