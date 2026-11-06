package service

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePipelineTestArchive(t *testing.T, name string, kind byte) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "release.tar.gz")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: name, Mode: 0755, Typeflag: kind}
	if kind == tar.TypeReg {
		h.Size = 3
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := tw.Write([]byte("app")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestPipelineArchiveBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  byte
		valid bool
	}{
		{"./bin/app", tar.TypeReg, true}, {"../escape", tar.TypeReg, false}, {"/etc/passwd", tar.TypeReg, false}, {"a/../../escape", tar.TypeReg, false}, {"a\\..\\escape", tar.TypeReg, false}, {"symlink", tar.TypeSymlink, false}, {"hardlink", tar.TypeLink, false}, {"empty", tar.TypeDir, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			digest, err := validatePipelineArchive(writePipelineTestArchive(t, tc.name, tc.kind))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && len(digest) != 64 {
				t.Fatalf("missing SHA256: %s", digest)
			}
		})
	}
}

func TestSSHPipelineConfigAndFailurePolicy(t *testing.T) {
	config := map[string]any{"hostIds": []uint{1, 2}, "deployRoot": "/opt/apps/test", "artifactPath": "dist/release.tar.gz", "activateScript": "true", "healthScript": "true"}
	if err := validateSSHPipelineConfig(config); err != nil {
		t.Fatal(err)
	}
	stage := OpsAppPipelineStageDefinition{ID: "deploy", Name: "deploy", Type: "sshDeploy", FailurePolicy: "stop", Config: config}
	if err := validateOpsPipelineStages([]OpsAppPipelineStageDefinition{stage}, true); err != nil {
		t.Fatal(err)
	}
	stage.FailurePolicy = "ignore"
	if err := validateOpsPipelineStages([]OpsAppPipelineStageDefinition{stage}, true); err == nil {
		t.Fatal("unsafe failure policy accepted")
	}
	stage.FailurePolicy = "stop"
	other := stage
	other.ID = "deploy2"
	if err := validateOpsPipelineStages([]OpsAppPipelineStageDefinition{stage, other}, true); err == nil {
		t.Fatal("multiple deployment stages accepted")
	}
	for _, root := range []string{"/", "relative", "/opt/../etc", "/opt/apps/", "/opt\napps"} {
		config["deployRoot"] = root
		if err := validateSSHPipelineConfig(config); err == nil {
			t.Fatalf("invalid root accepted: %q", root)
		}
	}
}

func TestPipelineActivateCommandQuotesPathsAndRunsHealth(t *testing.T) {
	command := pipelineActivateCommand("/opt/my app", "/opt/my app/releases/123", map[string]any{"activateScript": "systemctl restart app", "healthScript": "curl --fail http://127.0.0.1/health"})
	for _, want := range []string{"set -e", "mv -Tf", "current must be a symlink", "systemctl restart app", "curl --fail", "RELEASE_DIR='/opt/my app/releases/123'"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q", want)
		}
	}
}

func TestPipelineRollbackDecision(t *testing.T) {
	for _, tc := range []struct {
		actual, previous, released string
		restore, fail              bool
	}{
		{"/releases/new", "/releases/old", "/releases/new", true, false},
		{"/releases/old", "/releases/old", "/releases/new", false, false},
		{"/releases/later", "/releases/old", "/releases/new", false, true},
		{"/releases/new", "", "/releases/new", false, true},
		{"", "", "/releases/new", false, false},
	} {
		restore, err := pipelineRollbackDecision(tc.actual, tc.previous, tc.released)
		if restore != tc.restore || (err != nil) != tc.fail {
			t.Fatalf("%+v: restore=%v err=%v", tc, restore, err)
		}
	}
}
