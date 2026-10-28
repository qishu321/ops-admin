package service

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

func TestInspectionMultipleSourcesUsesOneWorkerPoolAndPersistsSourceSnapshots(t *testing.T) {
	var active, maximum, queries atomic.Int32
	gate := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); count > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, count) {
				break
			}
		}
		queries.Add(1)
		if count == 4 {
			once.Do(func() { close(gate) })
		}
		select {
		case <-gate:
		case <-time.After(2 * time.Second):
			t.Error("inspection did not start its four workers")
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/3/"):
			http.Error(w, "source unavailable", http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, "/2/"):
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
		default:
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"instance":"test"},"values":[[100,"20"],[200,"40"]]}]}}`)
		}
	}))
	defer server.Close()
	var savedRun model.MonitorInspectionRun
	var savedResults []model.MonitorInspectionResult
	names := map[uint]string{1: "集群A", 2: "集群B", 3: "失败集群"}
	state := &systemTestDB{query: func(q string, args []driver.NamedValue) ([]string, [][]driver.Value, error) {
		switch {
		case strings.Contains(q, "monitor_dashboard_panel"):
			rows := [][]driver.Value{}
			for i := 1; i <= 3; i++ {
				rows = append(rows, []driver.Value{int64(i), int64(10), int64(1), fmt.Sprintf("规则%d", i), "vector(1)", int64(1), "rule", "max", "gte", float64(80), float64(90), "warning"})
			}
			return []string{"id", "dashboard_id", "datasource_id", "title", "prom_ql", "status", "inspection_kind", "reducer", "operator", "warning_value", "critical_value", "no_data_state"}, rows, nil
		case strings.Contains(q, "monitor_dashboard"):
			return []string{"id", "name", "layout"}, [][]driver.Value{{int64(10), "跨集群巡检", "list"}}, nil
		case strings.Contains(q, "monitor_datasource"):
			id, _ := strconv.Atoi(fmt.Sprint(args[0].Value))
			return []string{"id", "name", "type", "url", "status"}, [][]driver.Value{{int64(id), names[uint(id)], "prometheus", fmt.Sprintf("%s/%d", server.URL, id), int64(1)}}, nil
		case strings.Contains(q, "monitor_inspection_run"):
			ids, _ := json.Marshal(savedRun.DatasourceIDs)
			sourceNames, _ := json.Marshal(savedRun.DatasourceNames)
			return []string{"id", "dashboard_id", "dashboard_name", "datasource_id", "datasource_name", "datasource_ids", "datasource_names", "total_count", "status"}, [][]driver.Value{{int64(savedRun.ID), int64(savedRun.DashboardID), savedRun.DashboardName, int64(savedRun.DatasourceID), savedRun.DatasourceName, string(ids), string(sourceNames), int64(savedRun.TotalCount), savedRun.Status}}, nil
		case strings.Contains(q, "monitor_inspection_result"):
			rows := [][]driver.Value{}
			for _, item := range savedResults {
				rows = append(rows, []driver.Value{int64(item.ID), int64(item.RunID), int64(item.PanelID), int64(item.DatasourceID), item.DatasourceName, item.Title, item.Status, item.ValueText})
			}
			return []string{"id", "run_id", "panel_id", "datasource_id", "datasource_name", "title", "status", "value_text"}, rows, nil
		}
		return nil, nil, fmt.Errorf("unexpected database read: %s", q)
	}}
	db := systemDB(t, state)
	if err := db.Callback().Create().Replace("gorm:create", func(tx *gorm.DB) {
		switch item := tx.Statement.Dest.(type) {
		case *model.MonitorInspectionRun:
			item.ID = 5
			savedRun = *item
		case *model.MonitorInspectionResult:
			item.ID = uint(len(savedResults) + 1)
			savedResults = append(savedResults, *item)
		default:
			tx.AddError(errors.New("unexpected inspection write"))
		}
		tx.RowsAffected = 1
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("test:inspection-snapshot", func(tx *gorm.DB) {
		if run, ok := tx.Statement.Model.(*model.MonitorInspectionRun); ok {
			savedRun = *run
		}
	}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{db: db}
	data, err := svc.RunMonitorInspection(MonitorInspectionRunPayload{DashboardID: 10, DatasourceID: 1, DatasourceIDs: []uint{3, 1, 2, 1}, StartAt: 100, EndAt: 200})
	if err != nil {
		t.Fatal(err)
	}
	run := data["run"].(model.MonitorInspectionRun)
	results := data["results"].([]model.MonitorInspectionResult)
	if run.TotalCount != 9 || run.HealthyCount != 3 || run.WarningCount != 3 || run.DangerCount != 3 || run.NoDataCount != 3 {
		t.Fatalf("incorrect cross-source totals: %#v", run)
	}
	if queries.Load() != 9 || maximum.Load() != 4 {
		t.Fatalf("queries=%d maximum=%d", queries.Load(), maximum.Load())
	}
	for i, item := range results {
		wantSource := []uint{3, 1, 2}[i/3]
		if item.DatasourceID != wantSource || item.PanelID != uint(i%3+1) || item.DatasourceName != names[wantSource] {
			t.Fatalf("source/result mismatch: %#v", item)
		}
	}
	names[1] = "已改名"
	restored, err := svc.GetMonitorInspectionRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	restoredRun := restored["run"].(model.MonitorInspectionRun)
	if len(restoredRun.DatasourceIDs) != 3 || restoredRun.DatasourceNames[1] != "集群A" || len(restored["results"].([]model.MonitorInspectionResult)) != 9 {
		t.Fatal("report snapshots did not survive reload")
	}
}

func TestInspectionRejectsUnauthorizedSourcesAndHydratesLegacyReports(t *testing.T) {
	svc := policyTestService(t, &access.Policy{UserID: 42})
	if _, err := svc.inspectionDatasources(MonitorInspectionRunPayload{DatasourceIDs: []uint{2}}, 1); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("unauthorized data source was queried: %v", err)
	}
	if _, err := svc.inspectionDatasources(MonitorInspectionRunPayload{DatasourceIDs: []uint{0}}, 1); err == nil {
		t.Fatal("invalid explicit selection silently used a default")
	}
	run := model.MonitorInspectionRun{DatasourceID: 7, DatasourceName: "历史集群"}
	results := []model.MonitorInspectionResult{{PanelID: 1}}
	hydrateInspectionDatasourceSnapshots(&run, results)
	if len(run.DatasourceIDs) != 1 || results[0].DatasourceID != 7 || results[0].DatasourceName != "历史集群" {
		t.Fatal("legacy single-source report was not hydrated")
	}
	// History must enforce access to every source in a multi-source report.
	db := systemDB(t, &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "monitor_inspection_run") {
			t.Fatal("unauthorized report results were read")
		}
		return []string{"id", "datasource_id", "datasource_ids"}, [][]driver.Value{{int64(1), int64(1), "[1,2]"}}, nil
	}})
	restricted := (&Service{db: db}).WithAccess(context.Background(), &access.Policy{UserID: 42, Grants: []model.ResourceGrant{{ResourceType: "datasource", ResourceID: 1, Actions: []string{"query"}}}})
	if err := restricted.requireResource("datasource", 1, "", "query"); err != nil {
		t.Fatal("test must authorize the first source before rejecting the second")
	}
	if _, err := restricted.GetMonitorInspectionRun(1); !errors.Is(err, access.ErrDenied) {
		t.Fatalf("cross-source report escaped access boundary: %v", err)
	}
}
