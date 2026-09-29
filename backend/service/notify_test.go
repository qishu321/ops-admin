package service

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeNotifyEvents(t *testing.T) {
	if got := normalizeNotifyEvents([]string{"failed", "failed", "all"}, "job"); !reflect.DeepEqual(got, []string{"all"}) {
		t.Fatalf("expected all event to be exclusive, got %#v", got)
	}
	if got := normalizeNotifyEvents(nil, "monitor"); !reflect.DeepEqual(got, []string{"firing", "recovered"}) {
		t.Fatalf("unexpected monitor defaults: %#v", got)
	}
	if got := normalizeNotifyEvents(nil, "schedule"); !reflect.DeepEqual(got, []string{"success", "failed", "recovered"}) {
		t.Fatalf("unexpected schedule defaults: %#v", got)
	}
}

func TestScheduleRecoveryMatchesLegacySuccessSubscription(t *testing.T) {
	event := NotifyEvent{Scope: "schedule", Event: "recovered", Status: "recovered"}
	if !notifyRuleAcceptsEvent([]string{"success", "failed"}, event) {
		t.Fatal("legacy success subscription should receive probe recovery")
	}
	if notifyRuleAcceptsEvent([]string{"failed"}, event) {
		t.Fatal("failure-only rule must not receive recovery")
	}
}

func TestParseNotifyBusinessResponse(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		body        string
		code        string
		message     string
		exists      bool
	}{
		{name: "dingtalk success", channelType: "dingtalk", body: `{"errcode":0,"errmsg":"ok"}`, code: "0", message: "ok", exists: true},
		{name: "wecom failure", channelType: "wecom", body: `{"errcode":40001,"errmsg":"invalid credential"}`, code: "40001", message: "invalid credential", exists: true},
		{name: "feishu failure", channelType: "feishu", body: `{"code":19001,"msg":"invalid webhook"}`, code: "19001", message: "invalid webhook", exists: true},
		{name: "custom webhook", channelType: "webhook", body: `{"code":500}`, exists: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, message, exists := parseNotifyBusinessResponse(test.channelType, []byte(test.body))
			if code != test.code || message != test.message || exists != test.exists {
				t.Fatalf("got code=%q message=%q exists=%v", code, message, exists)
			}
		})
	}
}

func TestNotifyRetryDelay(t *testing.T) {
	if notifyRetryDelay(1) != 10*time.Second || notifyRetryDelay(2) != 30*time.Second || notifyRetryDelay(3) != 2*time.Minute {
		t.Fatal("unexpected retry schedule")
	}
}

func TestRenderNotifyTemplateUsesBusinessStatusLabels(t *testing.T) {
	job := renderNotifyTemplate("{{status}}/{{jobName}}/{{stepName}}", NotifyEvent{
		Scope: "job", Status: "notice", TargetName: "发布作业",
		Extra: map[string]string{"stepName": "消息通知"},
	})
	if job != "通知/发布作业/消息通知" {
		t.Fatalf("unexpected job rendering: %q", job)
	}
	monitor := renderNotifyTemplate("{{status}}", NotifyEvent{Scope: "monitor", Status: "recovered"})
	if monitor != "已恢复" {
		t.Fatalf("unexpected monitor status: %q", monitor)
	}
}

func TestNormalizeNotifyTemplateForJobRejectsMonitorLayout(t *testing.T) {
	title, content := normalizeNotifyTemplateForEvent("【{{severity}}】{{alertName}} -- {{status}}", "实例：{{instance}}", NotifyEvent{Scope: "job"})
	if !strings.Contains(title, "【作业通知】") || !strings.Contains(content, "{{jobHistoryId}}") {
		t.Fatalf("monitor template was not replaced for job event: %q %q", title, content)
	}
}

func TestNotifyTemplateScopeCompatible(t *testing.T) {
	if !notifyTemplateScopeCompatible("all", "job") {
		t.Fatal("通用模板应兼容作业场景")
	}
	if !notifyTemplateScopeCompatible("job", "job") {
		t.Fatal("作业模板应兼容作业场景")
	}
	if notifyTemplateScopeCompatible("monitor", "job") {
		t.Fatal("监控模板不应兼容作业场景")
	}
	if notifyTemplateScopeCompatible("job", "all") {
		t.Fatal("具体场景模板不应绑定全部场景规则")
	}
}
