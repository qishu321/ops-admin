package store

import (
	"strings"

	"ops-admin/backend/model"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Dept{},
		&model.Post{},
		&model.Role{},
		&model.Menu{},
		&model.Admin{},
		&model.AuthSession{},
		&model.PublicDNSAccount{},
		&model.PublicDomainSnapshot{},
		&model.InternalDNSSetting{},
		&model.InternalDNSZone{},
		&model.InternalDNSRecord{},
		&model.DNSAuditLog{},
		&model.SSLCertificate{},
		&model.SSLCertificateDomain{},
		&model.SSLCertificateVersion{},
		&model.SSLCertificateTask{},
		&model.SSLCertificateAuditLog{},
		&model.AdminRole{},
		&model.RoleMenu{},
		&model.LoginLog{},
		&model.OperationLog{},
		&model.SystemConfig{},
		&model.LDAPConfig{},
		&model.AssetHostGroup{},
		&model.AssetHostGroupRelation{},
		&model.AssetCredential{},
		&model.AssetCloudAccount{},
		&model.AssetGateway{},
		&model.AssetHost{},
		&model.AssetService{},
		&model.AssetServiceWorkload{},
		&model.AssetDatabase{},
		&model.AssetDatabaseMetricSnapshot{},
		&model.AssetChangeLog{},
		&model.DatabaseSQLHistory{},
		&model.DatabaseTransferTask{},
		&model.DatabaseBackupPlan{},
		&model.DatabaseBackupRecord{},
		&model.K8sCluster{},
		&model.OpsScript{},
		&model.OpsScriptVersion{},
		&model.OpsExecTask{},
		&model.OpsExecTargetResult{},
		&model.OpsScheduleTemplate{},
		&model.OpsScheduleTask{},
		&model.OpsScheduleTaskLog{},
		&model.OpsJobTemplate{},
		&model.OpsJob{},
		&model.OpsJobHistory{},
		&model.OpsJobHistoryStep{},
		&model.OpsEnvironment{},
		&model.OpsApplication{},
		&model.OpsApplicationEnvironmentBinding{},
		&model.OpsAppBuildTask{},
		&model.OpsAppRelease{},
		&model.OpsAppArtifact{},
		&model.OpsImageRegistry{},
		&model.OpsAppPipelineTemplate{},
		&model.OpsAppPipeline{},
		&model.OpsAppPipelineRun{},
		&model.OpsAppPipelineRunStage{},
		&model.NotifyTemplate{},
		&model.NotifyChannel{},
		&model.NotifyRule{},
		&model.NotifySendLog{},
		&model.MonitorDatasource{},
		&model.MonitorLogShortcut{},
		&model.MonitorAlertRule{},
		&model.MonitorAlertRuleDatasource{},
		&model.MonitorAlertTemplate{},
		&model.MonitorAlertTemplateGroup{},
		&model.MonitorAlertEvent{},
		&model.MonitorAlertEventTimeline{},
		&model.MonitorAlertAction{},
		&model.MonitorSilenceRule{},
		&model.MonitorAggregationRule{},
		&model.MonitorQueryHistory{},
		&model.MonitorDashboard{},
		&model.MonitorDashboardPanel{},
		&model.MonitorInspectionRun{},
		&model.MonitorInspectionResult{},
		&model.IntegrationNavigationGroup{},
		&model.IntegrationNavigation{},
		&model.IntegrationAIModel{},
		&model.IntegrationAIConversation{},
		&model.IntegrationAIMessage{},
		&model.IntegrationAIToolConfig{},
		&model.IntegrationAIToolAction{},
		&model.IntegrationAIKnowledgeDocument{},
		&model.IntegrationFinOpsAccount{},
		&model.IntegrationFinOpsCostRecord{},
		&model.IntegrationFinOpsRecommendation{},
		&model.IntegrationFinOpsSyncLog{},
	); err != nil {
		return err
	}

	// Structured script variables supersede the legacy command-line defaults.
	// Remove the retired columns after the models have been migrated so old
	// installations converge to the same schema as fresh installations.
	for _, target := range []any{&model.OpsScript{}, &model.OpsScriptVersion{}} {
		if db.Migrator().HasColumn(target, "default_params") {
			if err := db.Migrator().DropColumn(target, "default_params"); err != nil {
				return err
			}
		}
	}

	// Templates created before scope support are classified by their variables.
	// Explicit generic templates remain "all".
	legacyScopes := []struct {
		scope   string
		markers []string
	}{
		{scope: "job", markers: []string{"{{jobName}}", "{{jobHistoryId}}", "{{stepName}}"}},
		{scope: "schedule", markers: []string{"{{taskName}}", "{{cronExpr}}", "{{durationMs}}"}},
		{scope: "monitor", markers: []string{"{{alertName}}", "{{severity}}", "{{datasourceName}}"}},
	}
	for _, item := range legacyScopes {
		conditions := make([]string, 0, len(item.markers))
		args := make([]any, 0, len(item.markers)*2)
		for _, marker := range item.markers {
			conditions = append(conditions, "title LIKE ? OR content LIKE ?")
			args = append(args, "%"+marker+"%", "%"+marker+"%")
		}
		query := db.Model(&model.NotifyTemplate{}).
			Where("scope = ? OR scope = '' OR scope IS NULL", "all").
			Where("("+strings.Join(conditions, ") OR (")+")", args...)
		if err := query.Update("scope", item.scope).Error; err != nil {
			return err
		}
	}
	return nil
}
