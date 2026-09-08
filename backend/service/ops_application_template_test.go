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

func TestNormalizeOpsPipelineStagesAcceptsCompactYAML(t *testing.T) {
	stages, _, err := normalizeOpsPipelineStages(`stages:
  - checkout
  - test: go test ./...
  - build:
      script: go build ./...
      timeout: 1200
  - image
  - deploy:
      namespace: test
      workload: demo-api
`)
	if err != nil {
		t.Fatalf("normalize compact YAML: %v", err)
	}
	if len(stages) != 5 {
		t.Fatalf("expected 5 stages, got %#v", stages)
	}
	if stages[1].Type != "test" || stages[1].Config["script"] != "go test ./..." {
		t.Fatalf("unexpected test stage: %#v", stages[1])
	}
	if stages[2].TimeoutSeconds != 1200 || stages[3].Type != "dockerBuild" || stages[4].Type != "k8sDeploy" {
		t.Fatalf("unexpected compact stages: %#v", stages)
	}
}

func TestNormalizeOpsPipelineStagesAcceptsStepYAML(t *testing.T) {
	stages, _, err := normalizeOpsPipelineStages(`steps:
  - name: 拉取代码
    uses: checkout
  - name: 运行测试
    type: test
    run: |
      go test ./...
  - name: 发布应用
    uses: kubernetes-deploy
    with:
      namespace: test
      workload: demo-api
`)
	if err != nil {
		t.Fatalf("normalize step YAML: %v", err)
	}
	if len(stages) != 3 || stages[0].Name != "拉取代码" || stages[1].Config["script"] != "go test ./...\n" {
		t.Fatalf("unexpected step stages: %#v", stages)
	}
	if stages[2].Type != "k8sDeploy" || stages[2].Config["namespace"] != "test" {
		t.Fatalf("unexpected deploy step: %#v", stages[2])
	}
}
