package access

import (
	"slices"

	"ops-admin/backend/model"
)

// Existing grants keep their original role gate, even when a new policy opens
// the same feature on another instance. This prevents a compatibility grant
// from acquiring extra operations through the global navigation permission set.
var operationFeatures = map[string]map[string][]string{
	"host": {
		"read":     {"assets:host:list", "assets:hostgroup:list", "assets:terminal", "ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:file"},
		"terminal": {"assets:host:terminal"}, "upload": {"assets:host:upload", "ops:quickexec:file:execute"},
		"download": {"assets:host:download", "ops:quickexec:file:execute"},
		"execute":  {"ops:quickexec:command:execute", "ops:quickexec:script:execute"}, "manage": {"assets:host:delete"},
	},
	"k8s": {
		"read": {"assets:k8s:cluster", "assets:k8s:overview", "assets:k8s:node", "assets:k8s:namespace", "assets:k8s:workload", "assets:k8s:pod", "assets:k8s:advancednetwork", "assets:k8s:configstorage", "assets:k8s:scalingpolicy"},
		"logs": {"assets:k8s:pod"}, "terminal": {"assets:k8s:pod:terminal"}, "upload": {"assets:k8s:pod:upload"},
		"restart": {"assets:k8s:workload:restart"}, "scale": {"assets:k8s:workload:scale"},
		"write": {"assets:k8s:workload:yaml", "assets:k8s:workload:image", "assets:k8s:workload:create"}, "delete": {"assets:k8s:pod:delete"},
	},
	"database": {
		"read":   {"assets:database:list", "assets:database:workbench", "assets:database:backup", "assets:database:import"},
		"query":  {"assets:database:workbench", "assets:database:sql:execute"},
		"write":  {"assets:database:sql:execute", "assets:database:data:edit", "assets:database:import:execute"},
		"export": {"assets:database:export", "assets:database:backup"}, "backup": {"assets:database:backup"}, "restore": {"assets:database:backup:restore"},
	},
	"datasource": {"read": {"monitor:datasource", "monitor:datasource:list", "monitor:query", "monitor:logs", "monitor:traces"}, "query": {"monitor:query", "monitor:logs"}},
	"middleware": {"read": {"assets:middleware:list", "assets:middleware:rabbitmq", "assets:middleware:nacos", "assets:middleware:etcd", "assets:middleware:zookeeper", "assets:middleware:kafka"}, "query": {"assets:middleware:query"}, "write": {"assets:middleware:write"}, "delete": {"assets:middleware:delete"}},
	"credential": {"read": {"assets:credential:list"}, "use": {"assets:credential:list"}, "manage": {"assets:credential:edit", "assets:credential:delete"}},
}

func FeatureAllows(permissions []string, kind, action string) bool {
	if kind == "host_group" {
		kind = "host"
	}
	for _, key := range operationFeatures[kind][action] {
		if slices.Contains(permissions, key) {
			return true
		}
	}
	return false
}

// Feature permissions only expose the relevant screens/buttons. The resource
// boundary is still checked independently on every request and execution.
var featureActions = map[string]map[string][]string{
	"host": {
		"read":     {"assets:host:list", "assets:hostgroup:list"},
		"terminal": {"assets:terminal", "assets:host:terminal"},
		"upload":   {"assets:terminal", "assets:host:upload", "ops:quickexec:file", "ops:quickexec:file:execute"},
		"download": {"assets:terminal", "assets:host:download"},
		"execute":  {"ops:quickexec:command", "ops:quickexec:script", "ops:quickexec:history", "ops:quickexec:command:execute", "ops:quickexec:script:execute"},
		"manage":   {"assets:host:delete"},
	},
	"k8s": {
		"read": {"assets:k8s:cluster", "assets:k8s:overview", "assets:k8s:node", "assets:k8s:namespace", "assets:k8s:workload", "assets:k8s:pod", "assets:k8s:advancednetwork", "assets:k8s:service", "assets:k8s:ingress", "assets:k8s:configstorage"},
		"logs": {"assets:k8s:pod"}, "terminal": {"assets:k8s:pod:terminal"}, "upload": {"assets:k8s:pod:upload"},
		"restart": {"assets:k8s:workload:restart"}, "scale": {"assets:k8s:workload:scale"},
		"write":  {"assets:k8s:workload:yaml", "assets:k8s:pod:yaml", "assets:k8s:workload:image", "assets:k8s:workload:create"},
		"delete": {"assets:k8s:pod:delete"},
	},
	"database": {
		"read": {"assets:database:list"}, "query": {"assets:database:workbench", "assets:database:sql:execute"},
		"write":  {"assets:database:workbench", "assets:database:sql:execute", "assets:database:data:edit", "assets:database:import", "assets:database:import:execute"},
		"export": {"assets:database:workbench", "assets:database:export", "assets:database:backup"},
		"backup": {"assets:database:backup", "assets:database:backup:create"}, "restore": {"assets:database:backup", "assets:database:backup:restore"},
	},
	"datasource": {"read": {"monitor:datasource:list", "monitor:datasource"}, "query": {"monitor:query", "monitor:logs"}},
	"middleware": {"read": {"assets:middleware:list", "assets:middleware:rabbitmq", "assets:middleware:nacos", "assets:middleware:etcd", "assets:middleware:zookeeper", "assets:middleware:kafka"}, "query": {"assets:middleware:query"}, "write": {"assets:middleware:write"}, "delete": {"assets:middleware:delete"}},
	"credential": {"read": {"assets:credential:list"}, "use": {"assets:credential:list"}, "manage": {"assets:credential:edit", "assets:credential:delete"}},
}

func FeaturePermissions(g model.ResourceGrant) []string {
	kind := g.ResourceType
	if kind == "host_group" {
		kind = "host"
	}
	result := []string{}
	for action, values := range featureActions[kind] {
		if !slices.Contains(g.Actions, action) && !slices.Contains(g.Actions, "manage") {
			continue
		}
		for _, value := range values {
			if !slices.Contains(result, value) {
				result = append(result, value)
			}
		}
	}
	return result
}

// Templates are a starting point copied into a new policy. Legacy role/menu
// definitions themselves remain unchanged and never broaden existing grants.
func TemplateActions(permissions []string, readOnly bool) map[string][]string {
	result := map[string][]string{}
	for kind, actions := range featureActions {
		result[kind] = []string{"read"}
		for _, action := range Actions[kind] {
			if action == "read" || action == "manage" || (readOnly && action != "query" && action != "logs") {
				continue
			}
			// Page entries alone must never imply an operation such as SQL writes.
			for _, key := range actions[action] {
				if key == "assets:k8s:pod" && action != "logs" {
					continue
				}
				if slices.Contains(permissions, key) && (slices.Contains([]string{"query", "logs"}, action) || slices.Contains([]string{"assets:middleware:write", "assets:middleware:delete", "assets:host:terminal", "assets:host:upload", "assets:host:download", "ops:quickexec:command:execute", "ops:quickexec:script:execute", "assets:database:data:edit", "assets:database:import:execute", "assets:database:export", "assets:database:backup:create", "assets:database:backup:restore", "assets:k8s:pod:terminal", "assets:k8s:pod:upload", "assets:k8s:workload:restart", "assets:k8s:workload:scale", "assets:k8s:workload:yaml", "assets:k8s:pod:delete"}, key)) {
					result[kind] = append(result[kind], action)
					break
				}
			}
		}
	}
	result["host_group"] = slices.Clone(result["host"])
	return result
}
