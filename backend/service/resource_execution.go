package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ops-admin/backend/internal/access"
)

type resourceK8sTransport struct {
	base      http.RoundTripper
	policy    *access.Policy
	clusterID uint
}

func (t resourceK8sTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	namespace := ""
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	for _, part := range parts {
		if part == "." || part == ".." || strings.ContainsAny(part, "\\\x00") {
			return nil, access.ErrDenied
		}
	}
	for i, part := range parts {
		if part == "namespaces" && i+1 < len(parts) {
			namespace = parts[i+1]
			break
		}
	}
	action := t.policy.Action("k8s")
	if req.Method == http.MethodGet {
		action = "read"
		if strings.HasSuffix(req.URL.Path, "/log") {
			action = "logs"
		}
	}
	if req.Method != http.MethodGet && action == "read" {
		action = "write"
	}
	// Cluster-wide GET collections are filtered below; all mutations and
	// namespaced requests must pass before any Kubernetes network request.
	if namespace != "" || req.Method != http.MethodGet {
		if err := t.policy.Require("k8s", t.clusterID, namespace, action); err != nil {
			return nil, err
		}
	} else if !t.policy.Visible("k8s", t.clusterID) {
		return nil, access.ErrDenied
	}
	response, err := t.base.RoundTrip(req)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || namespace != "" || req.Method != http.MethodGet {
		return response, err
	}
	if t.policy.Allows("k8s", t.clusterID, "", "read") {
		return response, nil
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "json") {
		response.Body.Close()
		return nil, access.ErrDenied
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	filtered, err := filterK8sResourceCollection(data, t.policy, t.clusterID)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(strings.NewReader(string(filtered)))
	response.ContentLength = int64(len(filtered))
	response.Header.Del("Content-Length")
	return response, nil
}
func filterK8sResourceCollection(data []byte, p *access.Policy, clusterID uint) ([]byte, error) {
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return nil, access.ErrDenied
	}
	items, ok := result["items"].([]any)
	if !ok {
		// API discovery and version information contain no workload data.
		if result["kind"] == "APIResourceList" || result["kind"] == "APIGroupList" || result["gitVersion"] != nil {
			return data, nil
		}
		return nil, access.ErrDenied
	}
	filtered := []any{}
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		meta, _ := item["metadata"].(map[string]any)
		namespace, _ := meta["namespace"].(string)
		if result["kind"] == "NamespaceList" {
			namespace, _ = meta["name"].(string)
		}
		if namespace != "" && p.Allows("k8s", clusterID, namespace, "read") {
			filtered = append(filtered, raw)
		}
	}
	result["items"] = filtered
	// Kubernetes continuation/resource counts describe the unfiltered set.
	delete(result, "metadata")
	return json.Marshal(result)
}

func (s *Service) guardK8sHTTPClient(client *http.Client, clusterID uint) {
	p := s.resourcePolicy()
	if p == nil || p.Super {
		return
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = resourceK8sTransport{base: base, policy: p, clusterID: clusterID}
}

// The SQL classifier is a UX/risk aid. Read-only SQL authorization also uses
// an engine-enforced read-only transaction, including SELECT functions and
// EXPLAIN ANALYZE, which cannot be classified safely by a keyword alone.
func (s *Service) queryDatabaseSQLRows(db *sql.DB, id uint, statement string) (*sql.Rows, func(), error) {
	p := s.resourcePolicy()
	if p == nil || p.Allows("database", id, "", "write") {
		rows, err := db.Query(statement)
		return rows, func() {}, err
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = tx.Rollback() }
	rows, err := tx.Query(statement)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return rows, cleanup, nil
}
