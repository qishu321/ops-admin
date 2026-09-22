package service

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ops-admin/backend/model"
)

func scheduleTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestExecuteScheduledHTTPTaskRetriesOrdinary4xxStatuses(t *testing.T) {
	statuses := []int{400, 401, 403, 404, 200}
	calls := 0
	delays := make([]time.Duration, 0, 4)
	task := model.OpsScheduleTask{
		HTTPMethod: "GET", URL: "https://example.test/healthz", ExpectedStatus: 200,
		RetryEnabled: true, MaxRetries: 4, RetryIntervalSeconds: 1, RetryBackoff: "fixed",
	}

	result := executeScheduledHTTPTask(task, func(*http.Request) (*http.Response, error) {
		status := statuses[calls]
		calls++
		return scheduleTestResponse(status, http.StatusText(status)), nil
	}, func(delay time.Duration) {
		delays = append(delays, delay)
	})

	if result.Status != "success" || result.AttemptCount != 5 || result.HTTPCode != 200 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if calls != 5 || len(delays) != 4 {
		t.Fatalf("expected five requests and four waits, got requests=%d waits=%d", calls, len(delays))
	}
	for _, status := range statuses[:4] {
		if !strings.Contains(result.Detail, "HTTP "+strconv.Itoa(status)) {
			t.Fatalf("attempt detail does not contain retried status %d: %s", status, result.Detail)
		}
	}
}

func TestExecuteScheduledHTTPTaskRetriesRequestError(t *testing.T) {
	calls := 0
	task := model.OpsScheduleTask{
		HTTPMethod: "GET", URL: "https://example.test/healthz", ExpectedStatus: 204,
		RetryEnabled: true, MaxRetries: 1, RetryIntervalSeconds: 2, RetryBackoff: "exponential",
	}
	result := executeScheduledHTTPTask(task, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("connection reset")
		}
		return scheduleTestResponse(204, ""), nil
	}, func(time.Duration) {})

	if result.Status != "success" || result.AttemptCount != 2 || !strings.Contains(result.Detail, "connection reset") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestScheduleHTTPRetryDelaySupportsFixedAndExponentialBackoff(t *testing.T) {
	fixed := model.OpsScheduleTask{RetryIntervalSeconds: 3, RetryBackoff: "fixed"}
	if got := scheduleHTTPRetryDelay(fixed, 3); got != 3*time.Second {
		t.Fatalf("fixed delay = %s, want 3s", got)
	}
	exponential := model.OpsScheduleTask{RetryIntervalSeconds: 3, RetryBackoff: "exponential"}
	if got := scheduleHTTPRetryDelay(exponential, 3); got != 12*time.Second {
		t.Fatalf("exponential delay = %s, want 12s", got)
	}
}

func TestBuildOpsScheduleNotifyPreviewEventRespectsFailureOnlyPolicy(t *testing.T) {
	payload := OpsScheduleTaskPayload{
		Name: "HTTP 健康检查", TaskType: "http", HTTPMethod: "GET", URL: "https://example.test/healthz",
		ExpectedStatus: 200, NotifyOnFailureOnly: true, RetryEnabled: true, MaxRetries: 2,
	}
	if _, err := buildOpsScheduleNotifyPreviewEvent(payload, "success"); err == nil {
		t.Fatal("failure-only policy must reject success preview")
	}
	event, err := buildOpsScheduleNotifyPreviewEvent(payload, "failed")
	if err != nil {
		t.Fatalf("failed preview returned error: %v", err)
	}
	if event.Status != "failed" || event.Extra["attemptCount"] != "3" {
		t.Fatalf("unexpected preview event: %+v", event)
	}
}
