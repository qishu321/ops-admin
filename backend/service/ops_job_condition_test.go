package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"ops-admin/backend/model"
)

func branchTestDefinition() OpsJobDefinition {
	return OpsJobDefinition{
		Nodes: []OpsJobNode{
			{ID: "check", Type: "script", Label: "检查", Config: map[string]any{}},
			{ID: "condition", Type: "condition", Label: "条件", Config: map[string]any{"sourceStepId": "check", "field": "status", "operator": "eq", "value": "success"}},
			{ID: "yes", Type: "approval", Label: "通过", Config: map[string]any{}},
			{ID: "no", Type: "notify", Label: "处理失败", Config: map[string]any{}},
			{ID: "join", Type: "notify", Label: "汇合", Config: map[string]any{}},
		},
		Edges: []OpsJobEdge{{Source: "check", Target: "condition"}, {Source: "condition", Target: "yes", Branch: "true"}, {Source: "condition", Target: "no", Branch: "false"}, {Source: "yes", Target: "join"}, {Source: "no", Target: "join"}},
	}
}

func TestOpsJobBranchSelectsOnlyOnePathAndJoinsOnce(t *testing.T) {
	for _, decision := range []bool{true, false} {
		t.Run(map[bool]string{true: "true path", false: "false path"}[decision], func(t *testing.T) {
			definition := branchTestDefinition()
			states := map[string]model.OpsJobHistoryStep{}
			var executed, skipped []string
			paused, handled, err := walkOpsJobDefinition(definition, states, func(node OpsJobNode, skip bool) (model.OpsJobHistoryStep, error) {
				step := model.OpsJobHistoryStep{StepID: node.ID, Status: "success"}
				if skip {
					skipped = append(skipped, node.ID)
					step.Status = "skipped"
				} else {
					executed = append(executed, node.ID)
					if node.Type == "condition" {
						step.BranchResult = &decision
					}
				}
				return step, nil
			})
			wantPath, wantSkip := "yes", "no"
			if !decision {
				wantPath, wantSkip = "no", "yes"
			}
			if err != nil || paused || handled || !reflect.DeepEqual(executed, []string{"check", "condition", wantPath, "join"}) || !reflect.DeepEqual(skipped, []string{wantSkip}) {
				t.Fatalf("executed=%v skipped=%v paused=%v handled=%v err=%v", executed, skipped, paused, handled, err)
			}
		})
	}
}

func TestOpsJobBranchResumeKeepsDecisionAndNeverRepeatsSteps(t *testing.T) {
	definition := branchTestDefinition()
	states := map[string]model.OpsJobHistoryStep{}
	calls := map[string]int{}
	decision := true
	execute := func(node OpsJobNode, skip bool) (model.OpsJobHistoryStep, error) {
		calls[node.ID]++
		step := model.OpsJobHistoryStep{StepID: node.ID, Status: "success"}
		if skip {
			step.Status = "skipped"
		} else if node.Type == "condition" {
			choice := decision
			step.BranchResult = &choice
		} else if node.Type == "approval" {
			step.Status = "waiting_approval"
		}
		return step, nil
	}
	paused, _, err := walkOpsJobDefinition(definition, states, execute)
	if err != nil || !paused || calls["join"] != 0 {
		t.Fatalf("expected pause before join, calls=%v err=%v", calls, err)
	}
	// Reconstruct exactly the persisted snapshot used after approval/restart.
	data, _ := json.Marshal(states)
	restored := map[string]model.OpsJobHistoryStep{}
	_ = json.Unmarshal(data, &restored)
	approved := restored["yes"]
	approved.Status = "success"
	restored["yes"] = approved
	decision = false // A re-evaluation would now choose the wrong path.
	paused, _, err = walkOpsJobDefinition(definition, restored, execute)
	if err != nil || paused || restored["no"].Status != "skipped" {
		t.Fatalf("resume lost selected path: states=%v err=%v", restored, err)
	}
	for _, id := range []string{"check", "condition", "yes", "no", "join"} {
		if calls[id] != 1 {
			t.Fatalf("%s executed %d times", id, calls[id])
		}
	}
}

func TestOpsJobFailureOnlyContinuesWhenExplicitlyConfigured(t *testing.T) {
	for _, policy := range []string{"", "branch"} {
		definition := branchTestDefinition()
		definition.Nodes[0].Config["failurePolicy"] = policy
		decision := false
		var ran []string
		_, handled, err := walkOpsJobDefinition(definition, map[string]model.OpsJobHistoryStep{}, func(node OpsJobNode, skip bool) (model.OpsJobHistoryStep, error) {
			step := model.OpsJobHistoryStep{Status: "success"}
			if skip {
				step.Status = "skipped"
				return step, nil
			}
			ran = append(ran, node.ID)
			if node.ID == "check" {
				step.Status = "failed"
			}
			if node.Type == "condition" {
				step.BranchResult = &decision
			}
			return step, nil
		})
		if policy == "" {
			if err == nil || !reflect.DeepEqual(ran, []string{"check"}) {
				t.Fatalf("legacy failure did not stop: ran=%v err=%v", ran, err)
			}
		} else if err != nil || !handled || !reflect.DeepEqual(ran, []string{"check", "condition", "no", "join"}) {
			t.Fatalf("failure handler did not run: ran=%v handled=%v err=%v", ran, handled, err)
		}
	}
}

func TestOpsJobConditionComparisons(t *testing.T) {
	tests := []struct {
		field, op, status string
		value             any
		want              bool
	}{
		{"status", "eq", "success", "success", true},
		{"status", "eq", "partial", "success", false},
		{"status", "eq", "partial", "partial", true},
		{"status", "ne", "failed", "success", true},
		{"failedCount", "eq", "partial", 2, true},
		{"failedCount", "ne", "partial", 0, true},
		{"failedCount", "gt", "partial", 2, false},
		{"failedCount", "gte", "partial", 2, true},
		{"successCount", "lt", "partial", 4, true},
		{"successCount", "lte", "partial", 3, true},
	}
	for _, tt := range tests {
		got, detail, err := compareOpsJobCondition(map[string]any{"field": tt.field, "operator": tt.op, "value": tt.value}, tt.status, 3, 2)
		if err != nil || got != tt.want || detail == "" {
			t.Errorf("%+v: got %v, detail=%q err=%v", tt, got, detail, err)
		}
	}
	for _, config := range []map[string]any{
		{"field": "status", "operator": "gt", "value": "success"},
		{"field": "status", "operator": "eq", "value": "running"},
		{"field": "stdout", "operator": "eq", "value": "secret"},
		{"field": "failedCount", "operator": "eq", "value": -1},
		{"field": "failedCount", "operator": "eq", "value": 1.2},
		{"field": "failedCount", "operator": "eq", "value": "0"},
	} {
		if err := validateOpsJobCondition(config); err == nil {
			t.Errorf("accepted invalid condition: %v", config)
		}
	}
}

func TestOpsJobBranchesRejectInvalidGraphsBeforeExecution(t *testing.T) {
	mutations := map[string]func(*OpsJobDefinition){
		"missing false path": func(d *OpsJobDefinition) { d.Edges = append(d.Edges[:2], d.Edges[3:]...) },
		"unknown branch":     func(d *OpsJobDefinition) { d.Edges[1].Branch = "maybe" },
		"wrong source":       func(d *OpsJobDefinition) { d.Nodes[1].Config["sourceStepId"] = "no" },
		"duplicate id":       func(d *OpsJobDefinition) { d.Nodes[3].ID = "yes" },
		"cycle":              func(d *OpsJobDefinition) { d.Edges = append(d.Edges, OpsJobEdge{Source: "join", Target: "check"}) },
		"dangling edge":      func(d *OpsJobDefinition) { d.Edges[0].Source = "missing" },
		"unsafe failure continuation": func(d *OpsJobDefinition) {
			d.Nodes[0].Config["failurePolicy"] = "branch"
			d.Edges = append(d.Edges, OpsJobEdge{Source: "check", Target: "yes"})
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			d := branchTestDefinition()
			mutate(&d)
			called := false
			_, _, err := walkOpsJobDefinition(d, map[string]model.OpsJobHistoryStep{}, func(OpsJobNode, bool) (model.OpsJobHistoryStep, error) {
				called = true
				return model.OpsJobHistoryStep{}, nil
			})
			if err == nil || called {
				t.Fatalf("invalid graph was executed, err=%v called=%v", err, called)
			}
		})
	}
}

func TestOpsJobBranchStopsOnPersistenceFailure(t *testing.T) {
	_, _, err := walkOpsJobDefinition(branchTestDefinition(), map[string]model.OpsJobHistoryStep{}, func(OpsJobNode, bool) (model.OpsJobHistoryStep, error) {
		return model.OpsJobHistoryStep{}, errors.New("storage unavailable")
	})
	if err == nil {
		t.Fatal("continued without a persisted step result")
	}
}

func TestOpsJobLegacyFanoutStillRunsAllSteps(t *testing.T) {
	d := OpsJobDefinition{Nodes: []OpsJobNode{{ID: "a", Type: "script"}, {ID: "b", Type: "notify"}, {ID: "c", Type: "approval"}}, Edges: []OpsJobEdge{{Source: "a", Target: "b"}, {Source: "a", Target: "c"}}}
	var ran []string
	_, _, err := walkOpsJobDefinition(d, map[string]model.OpsJobHistoryStep{}, func(node OpsJobNode, skip bool) (model.OpsJobHistoryStep, error) {
		if skip {
			t.Fatalf("legacy step %s skipped", node.ID)
		}
		ran = append(ran, node.ID)
		return model.OpsJobHistoryStep{Status: "success"}, nil
	})
	if err != nil || !reflect.DeepEqual(ran, []string{"a", "b", "c"}) {
		t.Fatalf("legacy fanout changed: %v %v", ran, err)
	}
}

func TestOpsJobNestedBranchesPropagateSkipsWithoutSkippingSharedJoin(t *testing.T) {
	d := branchTestDefinition()
	d.Nodes[2].Type = "script"
	d.Nodes = append(d.Nodes,
		OpsJobNode{ID: "inner", Type: "condition", Config: map[string]any{"sourceStepId": "yes", "field": "failedCount", "operator": "eq", "value": 0}},
		OpsJobNode{ID: "inner-yes", Type: "notify"}, OpsJobNode{ID: "inner-no", Type: "approval"},
	)
	d.Edges = append(d.Edges[:3],
		OpsJobEdge{Source: "yes", Target: "inner"},
		OpsJobEdge{Source: "inner", Target: "inner-yes", Branch: "true"},
		OpsJobEdge{Source: "inner", Target: "inner-no", Branch: "false"},
		OpsJobEdge{Source: "inner-yes", Target: "join"},
		OpsJobEdge{Source: "inner-no", Target: "join"},
		OpsJobEdge{Source: "no", Target: "join"},
	)
	for _, outer := range []bool{true, false} {
		states := map[string]model.OpsJobHistoryStep{}
		calls := map[string]int{}
		_, _, err := walkOpsJobDefinition(d, states, func(node OpsJobNode, skip bool) (model.OpsJobHistoryStep, error) {
			step := model.OpsJobHistoryStep{Status: "success"}
			if skip {
				step.Status = "skipped"
				return step, nil
			}
			calls[node.ID]++
			choice := outer && node.ID == "condition" // Inner condition always chooses false.
			if node.Type == "condition" {
				step.BranchResult = &choice
			}
			return step, nil
		})
		if err != nil || calls["join"] != 1 || calls["inner-yes"] != 0 {
			t.Fatalf("nested branch failed: outer=%v calls=%v err=%v", outer, calls, err)
		}
		if !outer && (calls["inner"] != 0 || states["inner-no"].Status != "skipped") {
			t.Fatalf("inactive subtree was executed: calls=%v", calls)
		}
	}
}

func TestOpsJobFalseChoiceSurvivesJSONAndMissingChoiceStops(t *testing.T) {
	choice := false
	data, _ := json.Marshal(model.OpsJobHistoryStep{StepID: "condition", Status: "success", BranchResult: &choice})
	var persisted model.OpsJobHistoryStep
	_ = json.Unmarshal(data, &persisted)
	if persisted.BranchResult == nil || *persisted.BranchResult {
		t.Fatal("false was confused with missing branch result")
	}
	states := map[string]model.OpsJobHistoryStep{"check": {Status: "success"}, "condition": {Status: "success"}}
	called := false
	_, _, err := walkOpsJobDefinition(branchTestDefinition(), states, func(OpsJobNode, bool) (model.OpsJobHistoryStep, error) {
		called = true
		return model.OpsJobHistoryStep{}, nil
	})
	if err == nil || called {
		t.Fatal("executed an unknown path without a persisted branch choice")
	}
}
