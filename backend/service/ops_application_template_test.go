package service

import "testing"

func TestNormalizeOpsPipelineStagesAcceptsYAML(t *testing.T) {
	stages, normalized, err := normalizeOpsPipelineStages(`stages:
  - id: checkout
    name: 代码拉取
    type: checkout
    timeoutSeconds: 600
    failurePolicy: stop
    config: {}
    env: {}
  - id: build
    name: 构建
    type: build
    config:
      script: go build ./...
`)
	if err != nil {
		t.Fatalf("normalize YAML: %v", err)
	}
	if len(stages) != 2 || stages[1].Config["script"] != "go build ./..." {
		t.Fatalf("unexpected stages: %#v", stages)
	}
	if normalized == "" {
		t.Fatal("normalized JSON is empty")
	}
}
