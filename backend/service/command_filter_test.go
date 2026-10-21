package service

import (
	"database/sql/driver"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"strings"
	"testing"
	"time"
)

func TestCommandFiltersEnabledScopeAndMatching(t *testing.T) {
	rules := []model.CommandFilter{{Name: "磁盘保护", MatchType: "keyword", Pattern: "MKFS", Scopes: []string{"terminal", "job"}, Enabled: true}, {Name: "删除保护", MatchType: "keyword", Pattern: "rm -rf", Scopes: []string{"job"}, Enabled: true}, {Name: "停止服务", MatchType: "regex", Pattern: `(?i)systemctl\s+stop`, Scopes: []string{"terminal"}, Enabled: true}, {Name: "停用示例", MatchType: "keyword", Pattern: "echo", Scopes: []string{"job"}, Enabled: false}}
	for _, tc := range []struct {
		scope, text string
		blocked     bool
	}{{"terminal", "mkfs /dev/example", true}, {"job", "mkfs", true}, {"terminal", "rm -rf /tmp/example", false}, {"job", "rm   -rf /tmp/example", true}, {"job", "rm \\\n-rf /tmp/example", true}, {"terminal", "SYSTEMCTL stop example", true}, {"job", "systemctl stop example", false}, {"job", "echo ok", false}} {
		if err := matchCommandFilters(rules, tc.scope, tc.text); (err != nil) != tc.blocked {
			t.Errorf("%s %q: %v", tc.scope, tc.text, err)
		}
	}
}

func TestMultipleCommandKeywordsMatchAnyEntry(t *testing.T) {
	for _, pattern := range []string{
		"rm -rf、mkfs、shutdown、reboot、dd if=、iptables -f、systemctl stop、userdel",
		"rm -rf, mkfs, shutdown, reboot, dd if=, iptables -f, systemctl stop, userdel",
		"rm -rf，mkfs，shutdown，reboot，dd if=，iptables -f，systemctl stop，userdel",
		"rm -rf\r\nmkfs\nshutdown\nreboot\ndd if=\niptables -f\nsystemctl stop\nuserdel",
	} {
		rule := model.CommandFilter{Name: "高危命令", MatchType: "keyword", Pattern: pattern, Scopes: []string{"terminal", "job"}, Enabled: true}
		for _, scope := range rule.Scopes {
			for _, command := range []string{"rm -rf sdk_sensitive_word.txt", "MKFS /dev/example", "shutdown now", "reboot", "dd if=/dev/zero", "iptables -f", "systemctl   stop example", "userdel example"} {
				if err := matchCommandFilters([]model.CommandFilter{rule}, scope, command); err == nil {
					t.Errorf("%s: %q did not match list %q", scope, command, pattern)
				}
			}
			if err := matchCommandFilters([]model.CommandFilter{rule}, scope, "history"); err != nil {
				t.Fatal(err)
			}
		}
		f := &TerminalCommandFilter{check: func(command string) error {
			return matchCommandFilters([]model.CommandFilter{rule}, "terminal", command)
		}}
		out, notices := f.Feed([]byte("rm -rf sdk_sensitive_word.txt\r"))
		if len(notices) != 1 || strings.Contains(string(out), "\r") {
			t.Errorf("SSH submission not blocked: %q %v", out, notices)
		}
	}
	regex := model.CommandFilter{Name: "正则逗号", MatchType: "regex", Pattern: `^a{1,3},b$`, Scopes: []string{"job"}, Enabled: true}
	if matchCommandFilters([]model.CommandFilter{regex}, "job", "aa,b") == nil || matchCommandFilters([]model.CommandFilter{regex}, "job", "aa") != nil {
		t.Fatal("regular expression separators were changed")
	}
}
func TestCommandFilterValidationAndAdministratorBoundary(t *testing.T) {
	for _, r := range []model.CommandFilter{{Name: "x", Pattern: "[", MatchType: "regex", Scopes: []string{"job"}}, {Name: "x", Pattern: "rm", MatchType: "keyword"}, {Name: "x", Pattern: "rm", MatchType: "keyword", Scopes: []string{"pod"}}} {
		if validateCommandFilter(&r) == nil {
			t.Fatal("invalid rule accepted")
		}
	}
	member := policyTestService(t, &access.Policy{Permissions: []string{"assets:commandfilter:admin", "system:config:save"}})
	if _, err := member.ListCommandFilters(); !errors.Is(err, access.ErrDenied) {
		t.Fatal(err)
	}
	if err := member.SaveCommandFilter(model.CommandFilter{}); !errors.Is(err, access.ErrDenied) {
		t.Fatal(err)
	}
	if err := member.DeleteCommandFilter(1); !errors.Is(err, access.ErrDenied) {
		t.Fatal(err)
	}
}
func TestLiveTerminalFilterEditingSplitInputAndDisable(t *testing.T) {
	active := true
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "asset_command_filter") || !strings.Contains(q, "enabled") {
			return nil, nil, errors.New("unexpected query " + q)
		}
		if !active {
			return []string{"id"}, nil, nil
		}
		return []string{"id", "name", "match_type", "pattern", "scopes", "enabled"}, [][]driver.Value{{int64(1), "危险删除", "keyword", "rm -rf", `["terminal"]`, true}}, nil
	}}
	s := &Service{db: systemDB(t, state)}
	f := s.NewTerminalCommandFilter()
	_, notes := f.Feed([]byte("rm -"))
	if len(notes) != 0 {
		t.Fatal(notes)
	}
	out, notes := f.Feed([]byte("rf /tmp/example\r"))
	if len(notes) != 1 || strings.Contains(string(out), "\r") || !strings.HasSuffix(string(out), "\x03") {
		t.Fatalf("dangerous submit escaped: %q %v", out, notes)
	}
	out, notes = f.Feed([]byte("echo ok\r"))
	if len(notes) != 0 || string(out) != "echo ok\r" {
		t.Fatalf("safe command altered: %q %v", out, notes)
	}
	active = false
	out, notes = f.Feed([]byte("rm -rf /tmp/example\r"))
	if len(notes) != 0 || !strings.HasSuffix(string(out), "\r") {
		t.Fatal("disabled rule still applies")
	}
	active = true
	f.Feed([]byte("echo -rf /tmp/example\x01\x1b[3~\x1b[3~\x1b[3~\x1b[3~rm"))
	out, notes = f.Feed([]byte("\r"))
	if len(notes) == 0 || strings.Contains(string(out), "\r") {
		t.Fatal("cursor editing bypassed filter")
	}
	_, notes = f.Feed([]byte("\x1b[A\r"))
	if len(notes) != 0 {
		t.Fatal("unconfigured history navigation was blocked")
	}
	_, notes = f.Feed([]byte("\x03echo fresh\r"))
	if len(notes) != 0 {
		t.Fatal("ctrl-c did not reset input tracking")
	}
}
func TestTerminalFilterPastedLinesAndScriptInputs(t *testing.T) {
	f := &TerminalCommandFilter{check: func(command string) error {
		return matchCommandFilters([]model.CommandFilter{{Name: "删除", MatchType: "keyword", Pattern: "rm -rf", Scopes: []string{"terminal"}, Enabled: true}}, "terminal", command)
	}}
	out, notes := f.Feed([]byte("\x1b[200~echo ok\nrm -rf /tmp/example\n\x1b[201~"))
	if len(notes) != 1 || strings.Contains(string(out), "example\n") {
		t.Fatalf("paste not checked per command: %q %v", out, notes)
	}
	text := jobFilterContent("echo $VARIABLE_CMD", "--example", map[string]string{"CMD": "mkfs"})
	if !strings.Contains(text, "mkfs") || !strings.Contains(text, "--example") {
		t.Fatal("script parameters/variables missing from filter input")
	}
}
func TestFiltersDoNotReplaceBuiltinRiskConfirmation(t *testing.T) {
	if _, err := requireOpsRiskConfirmation("reboot", false); err == nil {
		t.Fatal("builtin high-risk confirmation removed")
	}
	if _, err := requireOpsRiskConfirmation("reboot", true); err != nil {
		t.Fatal(err)
	}
}

func TestSSHHistoryAndCompletionAreNotImplicitlyForbidden(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		return []string{"id", "name", "match_type", "pattern", "scopes", "enabled"}, [][]driver.Value{{int64(1), "格式化保护", "keyword", "mkfs", `["terminal"]`, true}}, nil
	}}
	s := &Service{db: systemDB(t, state)}
	for _, input := range []string{"history\r", "cd tm\t\r", "\x1b[A\r", "\x12history\r", "echo hello!\r"} {
		out, notices := s.NewTerminalCommandFilter().Feed([]byte(input))
		if len(notices) != 0 || string(out) != input {
			t.Errorf("unconfigured operation blocked or changed: %q -> %q, %v", input, out, notices)
		}
	}
	_, notices := s.NewTerminalCommandFilter().Feed([]byte("mkfs /dev/example\r"))
	if len(notices) != 1 || !strings.Contains(notices[0], "格式化保护") {
		t.Fatal("configured filter stopped matching")
	}
}

func TestConfirmedJobsAndScriptVariablesCannotBypassEnabledFilter(t *testing.T) {
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "asset_command_filter") {
			return nil, nil, errors.New("execution reached another dependency before filtering")
		}
		return []string{"id", "name", "match_type", "pattern", "scopes", "enabled"}, [][]driver.Value{{int64(1), "磁盘保护", "keyword", "mkfs", `["job"]`, true}}, nil
	}}
	s := &Service{db: systemDB(t, state)}
	if _, err := s.ExecuteOpsCommand(OpsExecCommandPayload{CommandText: "mkfs", RiskConfirmed: true}); err == nil || !strings.Contains(err.Error(), "磁盘保护") {
		t.Fatalf("risk confirmation bypassed filter: %v", err)
	}
	result := s.execScriptOnHost(model.AssetHost{ID: 1}, model.OpsScript{Content: "echo $VARIABLE_COMMAND"}, "", map[string]string{"COMMAND": "mkfs"}, 30)
	if result.Status != "failed" || !strings.Contains(result.ErrorText, "磁盘保护") {
		t.Fatalf("script variable was not filtered before SSH: %+v", result)
	}
}

func TestCommandKeywordListIgnoresEmptyEntries(t *testing.T) {
	rule := model.CommandFilter{Name: "关键词", MatchType: "keyword", Pattern: ", 、，\n", Scopes: []string{"terminal"}, Enabled: true}
	if validateCommandFilter(&rule) == nil {
		t.Fatal("separator-only rule accepted")
	}
	if matchCommandFilters([]model.CommandFilter{rule}, "terminal", "history") != nil {
		t.Fatal("empty entries matched an unrelated command")
	}
	rule.Pattern = "rm -rf, ,，、\nmkfs\n"
	if validateCommandFilter(&rule) != nil {
		t.Fatal("valid list rejected")
	}
	if matchCommandFilters([]model.CommandFilter{rule}, "terminal", "history") != nil {
		t.Fatal("trailing separator matched history")
	}
}

func TestLiveSSHRuleEditBlocksEveryRmVariantAfterFirstDenial(t *testing.T) {
	pattern := "rm -rf"
	reads := 0
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		reads++
		return []string{"id", "name", "match_type", "pattern", "scopes", "enabled"}, [][]driver.Value{{int64(1), "删除保护", "keyword", pattern, `["terminal","job"]`, true}}, nil
	}}
	f := (&Service{db: systemDB(t, state)}).NewTerminalCommandFilter()
	typeCommand := func(command string, blocked bool) {
		t.Helper()
		var forwarded []byte
		var notices []string
		// Use one WebSocket-sized payload per key, as xterm does when typing.
		for _, key := range []byte(command + "\r") {
			out, messages := f.Feed([]byte{key})
			forwarded = append(forwarded, out...)
			notices = append(notices, messages...)
		}
		if (len(notices) > 0) != blocked {
			t.Fatalf("%q with live rule %q: %v", command, pattern, notices)
		}
		if blocked && (strings.Contains(string(forwarded), "\r") || !strings.HasSuffix(string(forwarded), "\x03")) {
			t.Fatalf("blocked command was submitted to SSH: %q", forwarded)
		}
	}
	typeCommand("rm -rf 111.txt/", true)
	f.Feed([]byte{3})
	typeCommand("history", false)
	pattern = "rm、mkfs、shutdown、reboot、dd if=、iptables -f、systemctl stop、userdel"
	for _, command := range []string{"rm 111.txt/", "rm -r 111.txt/", "rm -f 111.txt", "rm -rf 111.txt/", "rm -r -f 111.txt/"} {
		typeCommand(command, true)
		typeCommand("ls", false)
	}
	if reads != 12 {
		t.Fatalf("rules were cached instead of reloaded per submit: %d reads", reads)
	}
}

func TestSSHFilterAcrossRealWebSocketFrames(t *testing.T) {
	state := &systemTestDB{query: func(string, []driver.NamedValue) ([]string, [][]driver.Value, error) {
		return []string{"id", "name", "match_type", "pattern", "scopes", "enabled"}, [][]driver.Value{{int64(1), "删除保护", "keyword", "rm、mkfs、shutdown", `["terminal","job"]`, true}}, nil
	}}
	filter := (&Service{db: systemDB(t, state)}).NewTerminalCommandFilter()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var shellLine []byte
		for {
			_, input, err := conn.ReadMessage()
			if err != nil {
				return
			}
			out, notices := filter.Feed(input)
			executed := []string{}
			// Simulate the remote input stream without executing anything on a host.
			for _, b := range out {
				switch b {
				case 3:
					shellLine = nil
				case '\r', '\n':
					executed = append(executed, string(shellLine))
					shellLine = nil
				default:
					shellLine = append(shellLine, b)
				}
			}
			if err := conn.WriteJSON(map[string]any{"executed": executed, "blocked": len(notices)}); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for _, command := range []string{"rm -rf 111.txt/", "history", "rm 111.txt/", "rm -r 111.txt/", "ls", "rm -f 111.txt", "rm -r -f 111.txt/"} {
		blocked := 0
		executed := []string{}
		for _, key := range []byte(command + "\r") {
			if err := conn.WriteMessage(websocket.TextMessage, []byte{key}); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Executed []string `json:"executed"`
				Blocked  int      `json:"blocked"`
			}
			if err := conn.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			blocked += result.Blocked
			executed = append(executed, result.Executed...)
		}
		if strings.HasPrefix(command, "rm") {
			if blocked != 1 || len(executed) != 0 {
				t.Fatalf("SSH received executable command %q: blocked=%d, executed=%v", command, blocked, executed)
			}
		} else if blocked != 0 || len(executed) != 1 || executed[0] != command {
			t.Fatalf("safe command changed: %q", command)
		}
	}
}
