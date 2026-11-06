package service

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"ops-admin/backend/model"
)

func k8sDryRunQuery(enabled bool) map[string]string {
	if enabled {
		return map[string]string{"dryRun": "All", "fieldValidation": "Strict"}
	}
	return nil
}

func applyK8sResourceVersion(submitted, current map[string]any) error {
	meta, ok := submitted["metadata"].(map[string]any)
	if !ok {
		return errors.New("resource metadata is required")
	}
	currentMeta, _ := current["metadata"].(map[string]any)
	version, _ := currentMeta["resourceVersion"].(string)
	if supplied, _ := meta["resourceVersion"].(string); supplied != "" && supplied != version {
		return errors.New("resource version conflict; refresh before editing")
	}
	if uid, _ := meta["uid"].(string); uid != "" && uid != currentMeta["uid"] {
		return errors.New("resource was replaced; refresh before editing")
	}
	if version != "" {
		meta["resourceVersion"] = version
	}
	return nil
}

func buildK8sDeleteOptions(p model.K8sResourceDeletePayload) ([]byte, error) {
	if p.GracePeriodSeconds != nil && (*p.GracePeriodSeconds < 0 || *p.GracePeriodSeconds > 86400) {
		return nil, errors.New("invalid grace period")
	}
	if strings.EqualFold(p.ResourceType, "pod") {
		if strings.TrimSpace(p.UID) == "" {
			return nil, errors.New("pod UID is required; refresh before deleting")
		}
		if (p.Force || (p.GracePeriodSeconds != nil && *p.GracePeriodSeconds == 0)) && !p.Confirmed {
			return nil, errors.New("force deletion requires explicit confirmation")
		}
	} else if p.Force || p.GracePeriodSeconds != nil {
		return nil, errors.New("termination options are supported only for pods")
	}
	body := map[string]any{"apiVersion": "v1", "kind": "DeleteOptions"}
	if p.UID != "" {
		body["preconditions"] = map[string]string{"uid": p.UID}
	}
	if p.GracePeriodSeconds != nil {
		body["gracePeriodSeconds"] = *p.GracePeriodSeconds
	}
	if p.Force {
		body["gracePeriodSeconds"] = 0
	}
	return json.Marshal(body)
}

func podDisplayStatus(p kubePod) string {
	if p.Metadata.DeletionTimestamp != "" {
		return "Terminating"
	}
	for _, state := range p.Status.InitContainerStatuses {
		if state.State.Waiting != nil && state.State.Waiting.Reason != "" {
			return "Init:" + state.State.Waiting.Reason
		}
		if state.State.Terminated != nil && state.State.Terminated.ExitCode != 0 {
			return "Init:" + fallbackText(state.State.Terminated.Reason)
		}
	}
	for _, state := range p.Status.ContainerStatuses {
		if state.State.Waiting != nil && state.State.Waiting.Reason != "" {
			return state.State.Waiting.Reason
		}
	}
	if p.Status.Reason != "" {
		return p.Status.Reason
	}
	return fallbackText(p.Status.Phase)
}

// Deletion status follows one UID; a recreated StatefulSet Pod is a different object.
func (s *Service) GetK8sPodDeletionStatus(clusterID uint, namespace, name, uid string) (map[string]any, error) {
	_, runtime, client, err := s.k8sClientForCluster(clusterID)
	if err != nil {
		return nil, err
	}
	path := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(name)
	var raw map[string]any
	if err := k8sGetJSON(client, runtime, path, &raw); err != nil {
		if isK8sNotFoundError(err) {
			return map[string]any{"exists": false}, nil
		}
		return nil, err
	}
	var p kubePod
	encoded, _ := json.Marshal(raw)
	if err := json.Unmarshal(encoded, &p); err != nil {
		return nil, err
	}
	if uid != "" && p.Metadata.UID != uid {
		return map[string]any{"exists": false, "replacementUID": p.Metadata.UID}, nil
	}
	result := map[string]any{"exists": true, "uid": p.Metadata.UID, "status": podDisplayStatus(p), "phase": p.Status.Phase, "reason": p.Status.Reason, "message": p.Status.Message, "deletionTimestamp": p.Metadata.DeletionTimestamp, "finalizers": p.Metadata.Finalizers, "node": p.Spec.NodeName, "containers": p.Status.ContainerStatuses, "initContainers": p.Status.InitContainerStatuses, "owners": p.Metadata.OwnerReferences}
	if spec, ok := raw["spec"].(map[string]any); ok {
		result["terminationGracePeriodSeconds"] = spec["terminationGracePeriodSeconds"]
		result["volumes"] = spec["volumes"]
		hooks := []map[string]any{}
		if containers, ok := spec["containers"].([]any); ok {
			for _, container := range containers {
				if c, ok := container.(map[string]any); ok && c["lifecycle"] != nil {
					hooks = append(hooks, map[string]any{"name": c["name"], "lifecycle": c["lifecycle"]})
				}
			}
		}
		result["lifecycle"] = hooks
	}
	if p.Spec.NodeName != "" {
		var node kubeNode
		if err := k8sGetJSON(client, runtime, "/api/v1/nodes/"+url.PathEscape(p.Spec.NodeName), &node); err == nil {
			result["nodeConditions"] = node.Status.Conditions
		} else {
			result["nodeError"] = err.Error()
		}
	}
	events, eventsErr := fetchNamespacedEvents(client, runtime, namespace, "involvedObject.uid="+p.Metadata.UID)
	if eventsErr == nil {
		result["events"] = events
	} else {
		result["eventsError"] = eventsErr.Error()
	}
	return result, nil
}
