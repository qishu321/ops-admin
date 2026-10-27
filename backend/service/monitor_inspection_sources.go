package service

import (
	"errors"
	"fmt"

	"ops-admin/backend/model"
)

// Resolve and authorize every requested source before creating a run or issuing
// any metrics request. The legacy single-source field remains a fallback.
func (s *Service) inspectionDatasources(payload MonitorInspectionRunPayload, fallback uint) ([]model.MonitorDatasource, error) {
	ids := uniqueMonitorDatasourceIDs(payload.DatasourceIDs, payload.DatasourceID)
	if len(ids) == 0 {
		if len(payload.DatasourceIDs) > 0 {
			return nil, errors.New("请选择有效的巡检数据源")
		}
		ids = uniqueMonitorDatasourceIDs(nil, fallback)
	}
	if len(ids) == 0 {
		return nil, errors.New("请先选择巡检数据源")
	}
	result := make([]model.MonitorDatasource, 0, len(ids))
	for _, id := range ids {
		if err := s.requireResource("datasource", id, "", "query"); err != nil {
			return nil, err
		}
		ds, err := s.GetMonitorDatasource(id)
		if err != nil {
			return nil, fmt.Errorf("巡检数据源 #%d 不可用: %w", id, err)
		}
		if ds.Status != 1 {
			return nil, fmt.Errorf("巡检数据源 %s 已停用", ds.Name)
		}
		if ds.Type != "prometheus" && ds.Type != "victoriametrics" {
			return nil, errors.New("巡检仅支持 Prometheus 或 VictoriaMetrics 指标数据源")
		}
		result = append(result, *ds)
	}
	return result, nil
}

// Older report rows do not have per-result source snapshots. Hydrate the JSON
// response from the immutable report header without rewriting historical rows.
func hydrateInspectionDatasourceSnapshots(run *model.MonitorInspectionRun, results []model.MonitorInspectionResult) {
	if len(run.DatasourceIDs) == 0 && run.DatasourceID > 0 {
		run.DatasourceIDs = []uint{run.DatasourceID}
		run.DatasourceNames = []string{run.DatasourceName}
	}
	names := map[uint]string{}
	for index, id := range run.DatasourceIDs {
		if index < len(run.DatasourceNames) {
			names[id] = run.DatasourceNames[index]
		}
	}
	for index := range results {
		if results[index].DatasourceID == 0 {
			results[index].DatasourceID = run.DatasourceID
		}
		if results[index].DatasourceName == "" {
			results[index].DatasourceName = names[results[index].DatasourceID]
		}
	}
}
