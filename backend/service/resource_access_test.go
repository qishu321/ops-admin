package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func policyTestService(t *testing.T, p *access.Policy) *Service {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "unused:unused@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return (&Service{db: db, serviceRuntime: &serviceRuntime{}}).WithAccess(context.Background(), p)
}
func TestResourceBoundariesDenyBeforeSSHOrKubernetesConnection(t *testing.T) {
	s := policyTestService(t, &access.Policy{Actions: map[string]string{}, Grants: []model.ResourceGrant{{ResourceType: "host", ResourceID: 1, Actions: []string{"read"}}, {ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"read"}}}})
	if _, err := s.OpenAssetTerminal(1, 24, 80); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("read must not open SSH: %v", err)
	}
	if err := s.OpenK8sPodTerminal(2, "dev", "pod", "container", "sh", 24, 80, nil); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("read must not open exec: %v", err)
	}
	if _, err := s.UploadK8sPodFile(2, "dev", "pod", "c", "/tmp", "a.txt", 1, strings.NewReader("x")); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("read must not upload: %v", err)
	}
	if _, err := s.QueryMonitorLogs(MonitorLogQueryPayload{DatasourceID: 3, Index: "_all"}); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("log query must fail before network: %v", err)
	}
}
func TestRequestScopedServiceSharesRuntimeWithoutSharingIdentityOrCancellation(t *testing.T) {
	root := policyTestService(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	a := root.WithAccess(ctx, &access.Policy{UserID: 1})
	b := root.WithAccess(context.Background(), &access.Policy{UserID: 2})
	cancel()
	if a.serviceRuntime != b.serviceRuntime {
		t.Fatal("runtime locks/schedulers must be shared by pointer")
	}
	if a.resourcePolicy().UserID != 1 || b.resourcePolicy().UserID != 2 || root.resourcePolicy() != nil {
		t.Fatal("request identity leaked between services")
	}
	if a.db.Statement.Context.Err() != nil {
		t.Fatal("async tasks must retain authorization after the HTTP response")
	}
}
func TestKubernetesTransportFiltersCollectionsAndRejectsCrossNamespaceWrites(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"kind":"PodList","metadata":{"resourceVersion":"secret"},"items":[{"metadata":{"name":"dev-pod","namespace":"dev"}},{"metadata":{"name":"prod-pod","namespace":"prod"}}]}`)
	}))
	defer server.Close()
	p := &access.Policy{Actions: map[string]string{"k8s": "restart"}, Grants: []model.ResourceGrant{{ResourceType: "k8s", ResourceID: 2, Scope: "dev", Actions: []string{"read", "restart"}}}}
	client := &http.Client{Transport: resourceK8sTransport{base: http.DefaultTransport, policy: p, clusterID: 2}}
	resp, err := client.Get(server.URL + "/api/v1/pods")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "prod-pod") || strings.Contains(string(body), "resourceVersion") || !strings.Contains(string(body), "dev-pod") {
		t.Fatalf("unfiltered Kubernetes collection: %s", body)
	}
	for _, path := range []string{"/api/v1/namespaces/prod/pods/x", "/api/v1/namespaces/dev/../../secrets/x", "/apis/apps/v1/deployments/x"} {
		req, _ := http.NewRequest(http.MethodPatch, server.URL+path, strings.NewReader("{}"))
		if _, err = client.Do(req); !errors.Is(err, access.ErrDenied) {
			t.Errorf("write escaped namespace: %s: %v", path, err)
		}
	}
	if requests != 1 {
		t.Fatalf("denied requests reached Kubernetes: %d", requests)
	}
	filtered, err := filterK8sResourceCollection([]byte(`{"kind":"NamespaceList","items":[{"metadata":{"name":"dev"}},{"metadata":{"name":"prod"}}]}`), p, 2)
	if err != nil || strings.Contains(string(filtered), "prod") {
		t.Fatalf("namespace discovery leaked: %s %v", filtered, err)
	}
	var decoded map[string]any
	if json.Unmarshal(filtered, &decoded) != nil {
		t.Fatal("filtered result must remain valid JSON")
	}
}

// A tiny database/sql driver verifies that the engine, rather than the SQL
// keyword classifier alone, receives ReadOnly=true for query-only users.
var readonlyDriverOnce sync.Once
var readonlyOptions chan driver.TxOptions

type readonlyDriver struct{}
type readonlyConn struct{}
type readonlyTx struct{}
type readonlyRows struct{ done bool }

func (readonlyDriver) Open(string) (driver.Conn, error) { return readonlyConn{}, nil }
func (readonlyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (readonlyConn) Close() error              { return nil }
func (readonlyConn) Begin() (driver.Tx, error) { return nil, errors.New("expected BeginTx") }
func (readonlyConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	readonlyOptions <- opts
	return readonlyTx{}, nil
}
func (readonlyConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &readonlyRows{}, nil
}
func (readonlyTx) Commit() error        { return nil }
func (readonlyTx) Rollback() error      { return nil }
func (*readonlyRows) Columns() []string { return []string{"n"} }
func (*readonlyRows) Close() error      { return nil }
func (r *readonlyRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = int64(1)
	return nil
}
func TestSQLQueryOnlyAuthorizationUsesReadOnlyTransaction(t *testing.T) {
	readonlyDriverOnce.Do(func() { sql.Register("resource-readonly-test", readonlyDriver{}) })
	readonlyOptions = make(chan driver.TxOptions, 1)
	db, err := sql.Open("resource-readonly-test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := policyTestService(t, &access.Policy{Grants: []model.ResourceGrant{{ResourceType: "database", ResourceID: 3, Actions: []string{"read", "query"}}}})
	rows, cleanup, err := s.queryDatabaseSQLRows(db, 3, "SELECT potentially_mutating_function()")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	cleanup()
	select {
	case opts := <-readonlyOptions:
		if !opts.ReadOnly {
			t.Fatal("query-only authorization must reach the database as a read-only transaction")
		}
	default:
		t.Fatal("query ran without a transaction")
	}
}
