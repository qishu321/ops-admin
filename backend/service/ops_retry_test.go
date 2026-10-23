package service

import (
	"database/sql/driver"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func TestOpsRetryServiceNeverResolvesSuccessfulOrUnselectedHosts(t *testing.T) {
	var targetArgs []driver.NamedValue
	state := &systemTestDB{query: func(q string, args []driver.NamedValue) ([]string, [][]driver.Value, error) {
		switch {
		case strings.Contains(q, "FROM `ops_exec_task`"):
			return []string{"id", "task_type", "status"}, [][]driver.Value{{int64(9), "command", "partial"}}, nil
		case strings.Contains(q, "FROM `ops_exec_target_result`"):
			return []string{"id", "task_id", "host_id", "status"}, [][]driver.Value{{int64(1), int64(9), int64(11), "success"}, {int64(2), int64(9), int64(12), "failed"}, {int64(3), int64(9), int64(13), "failed"}}, nil
		case strings.Contains(q, "JOIN ops_exec_task t"):
			return []string{"id"}, nil, nil
		case strings.Contains(q, "FROM `asset_host`"):
			targetArgs = args
			return []string{"id"}, nil, nil
		default:
			t.Fatalf("unexpected query: %s", q)
			return nil, nil, nil
		}
	}}
	s := &Service{db: systemDB(t, state)}
	if _, err := s.RetryFailedOpsExecTask(OpsExecRetryPayload{TaskID: 9, ResultID: 2}); err == nil {
		t.Fatal("expected unavailable target error")
	}
	if len(targetArgs) != 1 || targetArgs[0].Value != int64(12) {
		t.Fatalf("retried other hosts: %v", targetArgs)
	}
	targetArgs = nil
	if _, err := s.RetryFailedOpsExecTask(OpsExecRetryPayload{TaskID: 9, ResultID: 1}); err == nil {
		t.Fatal("accepted successful result")
	}
	if len(targetArgs) != 0 || len(state.writes) != 0 {
		t.Fatal("successful target caused a host lookup or execution write")
	}
}

func TestOpsRetrySelectsOnlyRequestedFailure(t *testing.T) {
	rows := []model.OpsExecTargetResult{{ID: 1, HostID: 11, Status: "success"}, {ID: 2, HostID: 12, Status: "failed"}, {ID: 3, HostID: 13, Status: "timeout"}, {ID: 4, HostID: 14, Status: "running"}}
	ids, err := selectOpsRetryTargets(rows, 2)
	if err != nil || !reflect.DeepEqual(ids, []uint{12}) {
		t.Fatalf("individual retry expanded targets: %v, %v", ids, err)
	}
	ids, err = selectOpsRetryTargets(rows, 0)
	if err != nil || !reflect.DeepEqual(ids, []uint{12, 13}) {
		t.Fatalf("bulk retry included a successful or running host: %v, %v", ids, err)
	}
	for _, id := range []uint{1, 4, 99} {
		if _, err := selectOpsRetryTargets(rows, id); err == nil {
			t.Fatalf("accepted successful/running/foreign result %d", id)
		}
	}
}

func TestOpsBulkRetrySkipsAlreadyRetriedTargetsWithoutBlockingOtherFailures(t *testing.T) {
	ids := excludeOpsRetriedTargets([]uint{11, 12, 13}, []model.OpsExecTargetResult{{HostID: 11, Status: "success"}, {HostID: 12, Status: "running"}})
	if !reflect.DeepEqual(ids, []uint{13}) {
		t.Fatalf("retried successful/running hosts or dropped remaining failures: %v", ids)
	}
}

func TestOpsRetrySnapshotPreservesScriptAndSecretVariables(t *testing.T) {
	original := opsRetryContext{Script: &model.OpsScript{Content: "echo original", Interpreter: "bash", CurrentVersion: 7}, Variables: map[string]string{"TOKEN": "private-token"}, Overwrite: true}
	cipher, err := encodeOpsRetryContext(original)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cipher, "private-token") {
		t.Fatal("persisted plaintext secret")
	}
	copy, err := decodeOpsRetryContext(cipher)
	if err != nil || !reflect.DeepEqual(copy, original) {
		t.Fatalf("lost original execution inputs: %+v %v", copy, err)
	}
	response, _ := json.Marshal(model.OpsExecTask{RetryContext: cipher})
	if strings.Contains(string(response), cipher) || strings.Contains(string(response), "retryContext") {
		t.Fatal("exposed internal snapshot in API response")
	}
	if _, err := decodeOpsRetryContext(""); err == nil {
		t.Fatal("legacy task silently reused missing inputs")
	}
}

func TestOpsRetryFileSurvivesOriginalCleanupAndExpiresIndependently(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.WriteFile(source, []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "retry")
	now := time.Now()
	first, expiry, err := retainOpsRetryFileIn(dir, source, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := retainOpsRetryFileIn(dir, first, now)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(source)
	os.Remove(first)
	bytes, err := os.ReadFile(second)
	if err != nil || string(bytes) != "original bytes" {
		t.Fatalf("retry depended on cleaned source: %s %v", bytes, err)
	}
	if expiry.Sub(now) != 24*time.Hour {
		t.Fatal("unexpected retry retention")
	}
	old := now.Add(-25 * time.Hour)
	os.Chtimes(second, old, old)
	cleanupOpsRetryFiles(dir, now)
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("expired copy retained")
	}
}
