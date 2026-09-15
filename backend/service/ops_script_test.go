package service

import (
	"encoding/json"
	"strings"
	"testing"

	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

func TestOpsScriptJSONDoesNotExposeLegacyDefaultParams(t *testing.T) {
	for name, value := range map[string]any{"script": model.OpsScript{}, "version": model.OpsScriptVersion{}, "payload": OpsScriptPayload{}} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "defaultParams") {
			t.Fatalf("%s still exposes retired defaultParams: %s", name, data)
		}
	}
}

func TestNormalizeOpsScriptInterpreter(t *testing.T) {
	tests := []struct {
		scriptType  string
		interpreter string
		want        string
	}{
		{scriptType: "python", interpreter: "", want: "python"},
		{scriptType: "python", interpreter: "bash", want: "python"},
		{scriptType: "python", interpreter: "python3", want: "python3"},
		{scriptType: "shell", interpreter: "", want: "bash"},
		{scriptType: "shell", interpreter: "python", want: "bash"},
		{scriptType: "shell", interpreter: "sh", want: "sh"},
	}
	for _, item := range tests {
		if got := normalizeOpsInterpreter(item.interpreter, item.scriptType); got != item.want {
			t.Errorf("normalizeOpsInterpreter(%q, %q) = %q, want %q", item.interpreter, item.scriptType, got, item.want)
		}
	}
}

func TestResolveScheduleScriptVariablesKeepsSecretsEncrypted(t *testing.T) {
	util.ConfigureCredentialKey("schedule-variable-test-key")
	t.Cleanup(func() { util.ConfigureCredentialKey("") })
	script := &model.OpsScript{Variables: model.OpsScriptVariables{
		{Name: "ENV", DefaultValue: "test"},
		{Name: "TOKEN", Required: true, Secret: true},
	}}
	stored, runtime, err := resolveScheduleScriptVariables(script, map[string]string{"ENV": "prod", "TOKEN": "top-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stored["TOKEN"] == "top-secret" || runtime["TOKEN"] != "top-secret" || stored["ENV"] != "prod" {
		t.Fatalf("unexpected variable storage/runtime values: stored=%#v runtime=%#v", stored, runtime)
	}
	_, retained, err := resolveScheduleScriptVariables(script, nil, stored)
	if err != nil || retained["TOKEN"] != "top-secret" || retained["ENV"] != "prod" {
		t.Fatalf("scheduled variables were not restored: %#v, %v", retained, err)
	}
}

func TestNormalizeOpsScriptTimeout(t *testing.T) {
	tests := []struct{ input, want int }{{0, 300}, {10, 30}, {30, 30}, {300, 300}, {7200, 3600}}
	for _, item := range tests {
		if got := normalizeOpsScriptTimeout(item.input); got != item.want {
			t.Errorf("normalizeOpsScriptTimeout(%d) = %d, want %d", item.input, got, item.want)
		}
	}
}

func TestResolveOpsScriptVariables(t *testing.T) {
	definitions := []model.OpsScriptVariable{{Name: "ENV", DefaultValue: "test"}, {Name: "TOKEN", Required: true, Secret: true}}
	if _, err := resolveOpsScriptVariables(definitions, map[string]string{"ENV": "prod"}); err == nil {
		t.Fatal("required variable should be rejected when omitted")
	}
	values, err := resolveOpsScriptVariables(definitions, map[string]string{"ENV": "prod", "TOKEN": "secret"})
	if err != nil || values["ENV"] != "prod" || values["TOKEN"] != "secret" {
		t.Fatalf("unexpected resolved variables: %#v, %v", values, err)
	}
	exports := opsScriptVariableExports(values)
	if !strings.Contains(exports, "VARIABLE_ENV") || strings.Contains(exports, "secret") {
		t.Fatalf("variables must be exported with a namespaced, encoded value: %s", exports)
	}
}
