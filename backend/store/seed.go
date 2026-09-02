package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ops-admin/backend/model"
	"ops-admin/backend/util"

	"gorm.io/gorm"
)

func Seed(db *gorm.DB) error {
	steps := []func(*gorm.DB) error{
		seedSystemConfig,
		seedMonitorAlertTemplates,
		seedDept,
		seedPost,
		seedRole,
		seedMenus,
		seedAdmin,
		seedSuperRolePermissions,
	}

	for _, step := range steps {
		if err := step(db); err != nil {
			return err
		}
	}
	return nil
}

// seedMonitorAlertTemplates provides a reviewed starting library. They are
// platform-owned definitions only; no datasource is bound and no alert is
// enabled until an operator creates an alert rule from a template.
func seedMonitorAlertTemplates(db *gorm.DB) error {
	return db.Transaction(seedMonitorAlertTemplatesTx)
}

func seedMonitorAlertTemplatesTx(db *gorm.DB) error {
	groupIDs := map[string]uint{}
	for _, path := range [][]string{{"Linux", "node_exporter"}, {"Kubernetes", "kube-state-metrics"}, {"MySQL", "mysqld_exporter"}, {"Redis", "redis_exporter"}} {
		parentID := uint(0)
		for _, name := range path {
			key := fmt.Sprintf("%d/%s", parentID, name)
			var group model.MonitorAlertTemplateGroup
			err := db.Where("parent_id = ? AND name = ?", parentID, name).First(&group).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				group = model.MonitorAlertTemplateGroup{ParentID: parentID, Name: name}
				if err = db.Create(&group).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			groupIDs[key] = group.ID
			parentID = group.ID
		}
	}
	makeTemplate := func(name, collector, query, comparator string, threshold float64, duration int, severity, domain, description string) model.MonitorAlertTemplate {
		return model.MonitorAlertTemplate{Name: name, Category: "Linux", Collector: collector, ObjectType: "主机", DatasourceType: "prometheus", QueryText: query, Comparator: comparator, Threshold: threshold, ForSeconds: duration, EvalIntervalSeconds: 60, Severity: severity, LabelsJSON: fmt.Sprintf(`{"domain":%q}`, domain), AnnotationsJSON: fmt.Sprintf(`{"summary":%q}`, name), Description: description, Source: "platform", Status: 1}
	}
	makeDatabaseTemplate := func(category, collector, name, query, comparator string, threshold float64, duration int, severity, domain, description string) model.MonitorAlertTemplate {
		return model.MonitorAlertTemplate{Name: name, Category: category, Collector: collector, ObjectType: "数据库", DatasourceType: "prometheus", QueryText: query, Comparator: comparator, Threshold: threshold, ForSeconds: duration, EvalIntervalSeconds: 60, Severity: severity, LabelsJSON: fmt.Sprintf(`{"domain":%q}`, domain), AnnotationsJSON: fmt.Sprintf(`{"summary":%q}`, name), Description: description, Source: "platform", Status: 1}
	}
	templates := []model.MonitorAlertTemplate{
		makeTemplate("主机 Exporter 离线", "node_exporter", `up{job=~"node.*|node_exporter"}`, "==", 0, 120, "P0", "availability", "采集目标连续不可达，优先确认网络、Exporter 进程与采集配置。"),
		makeTemplate("主机 CPU 使用率过高", "node_exporter", `100 - (avg by(instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`, ">", 90, 300, "P1", "cpu", "CPU 使用率持续过高，结合系统负载和高消耗进程定位。"),
		makeTemplate("主机内存可用率过低", "node_exporter", `node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes * 100`, "<", 10, 300, "P1", "memory", "可用内存比例持续偏低，检查内存泄漏、缓存和容量水位。"),
		makeTemplate("主机磁盘使用率过高", "node_exporter", `(100 - ((node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"} * 100) / node_filesystem_size_bytes{fstype!~"tmpfs|overlay"}))`, ">", 85, 600, "P1", "disk", "磁盘分区容量接近上限，处理日志、临时文件或执行扩容。"),

		makeTemplate("主机 TCP 连接数过高", "node_exporter", "node_netstat_Tcp_CurrEstab", ">", 20000, 300, "P1", "connection", "TCP 已建立连接数持续过高，检查连接泄漏、流量突增与短连接配置。"),
		makeTemplate("主机 UDP 套接字数过高", "node_exporter", "node_sockstat_UDP_inuse", ">", 10000, 300, "P1", "connection", "UDP 正在使用的套接字数持续过高，检查异常流量与服务行为。"),
		makeTemplate("主机用户态 CPU 使用率过高", "node_exporter", "avg by(instance) (rate(node_cpu_seconds_total{mode=\"user\"}[5m])) * 100", ">", 70, 300, "P1", "cpu", "用户态 CPU 使用率持续过高，定位高消耗应用进程。"),
		makeTemplate("主机内核态 CPU 使用率过高", "node_exporter", "avg by(instance) (rate(node_cpu_seconds_total{mode=\"system\"}[5m])) * 100", ">", 50, 300, "P1", "cpu", "内核态 CPU 使用率持续过高，检查系统调用、网络和磁盘 I/O。"),
		makeTemplate("主机根分区 inode 使用率过高", "node_exporter", `100 - (node_filesystem_files_free{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_files{mountpoint="/",fstype!~"tmpfs|overlay"} * 100)`, ">", 85, 600, "P1", "disk", "根分区 inode 使用率接近上限，检查大量小文件。"),
		makeTemplate("主机磁盘读取速率过高", "node_exporter", "sum by(instance) (rate(node_disk_read_bytes_total[5m]))", ">", 104857600, 300, "P2", "disk_io", "磁盘读取速率持续超过 100 MiB/s。"),
		makeTemplate("主机磁盘写入速率过高", "node_exporter", "sum by(instance) (rate(node_disk_written_bytes_total[5m]))", ">", 104857600, 300, "P2", "disk_io", "磁盘写入速率持续超过 100 MiB/s。"),
		makeTemplate("主机磁盘读 IOPS 过高", "node_exporter", "sum by(instance) (rate(node_disk_reads_completed_total[5m]))", ">", 5000, 300, "P2", "disk_io", "磁盘读 IOPS 持续过高，检查热点请求与存储性能。"),
		makeTemplate("主机磁盘写 IOPS 过高", "node_exporter", "sum by(instance) (rate(node_disk_writes_completed_total[5m]))", ">", 5000, 300, "P2", "disk_io", "磁盘写 IOPS 持续过高，检查写放大与批处理任务。"),
		makeTemplate("主机 1 分钟平均负载过高", "node_exporter", "node_load1", ">", 8, 300, "P2", "load", "1 分钟平均负载持续偏高。"),
		makeTemplate("主机 5 分钟平均负载过高", "node_exporter", "node_load5", ">", 8, 300, "P2", "load", "5 分钟平均负载持续偏高。"),
		makeTemplate("主机 15 分钟平均负载过高", "node_exporter", "node_load15", ">", 8, 600, "P2", "load", "15 分钟平均负载持续偏高。"),
		makeTemplate("主机 1 分钟负载比过高", "node_exporter", "node_load1 / count by(instance) (node_cpu_seconds_total{mode=\"idle\"})", ">", 1, 300, "P2", "load", "1 分钟系统负载与 CPU 核数比值超过 1。"),
		makeTemplate("主机 5 分钟负载比过高", "node_exporter", "node_load5 / count by(instance) (node_cpu_seconds_total{mode=\"idle\"})", ">", 1, 300, "P2", "load", "5 分钟系统负载与 CPU 核数比值超过 1。"),
		makeTemplate("主机 15 分钟负载比过高", "node_exporter", "node_load15 / count by(instance) (node_cpu_seconds_total{mode=\"idle\"})", ">", 1, 600, "P2", "load", "15 分钟系统负载与 CPU 核数比值超过 1。"),
		makeTemplate("主机 Swap 使用率过高", "node_exporter", "(1 - (node_memory_SwapFree_bytes / node_memory_SwapTotal_bytes)) * 100", ">", 50, 300, "P2", "memory", "Swap 使用率持续偏高，检查内存压力。"),
		makeTemplate("主机网络接收速率过高", "node_exporter", "sum by(instance) (rate(node_network_receive_bytes_total{device!~\"lo\"}[5m]))", ">", 104857600, 300, "P2", "network", "网络接收速率持续超过 100 MiB/s。"),
		makeTemplate("主机网络发送速率过高", "node_exporter", "sum by(instance) (rate(node_network_transmit_bytes_total{device!~\"lo\"}[5m]))", ">", 104857600, 300, "P2", "network", "网络发送速率持续超过 100 MiB/s。"),
		makeTemplate("主机最近重启", "node_exporter", "time() - node_boot_time_seconds", "<", 86400, 0, "P2", "service", "主机运行时间不足一天，确认是否为计划内重启。"),
		makeTemplate("主机打开文件句柄数过高", "node_exporter", "node_filefd_allocated", ">", 100000, 300, "P2", "service", "打开文件句柄数持续过高，检查句柄泄漏与系统上限。"),
		{Name: "Kubernetes 节点未就绪", Category: "Kubernetes", Collector: "kube-state-metrics", ObjectType: "节点", DatasourceType: "prometheus", QueryText: "kube_node_status_condition{condition=\"Ready\",status=\"true\"}", Comparator: "==", Threshold: 0, ForSeconds: 300, EvalIntervalSeconds: 60, Severity: "P1", LabelsJSON: `{"domain":"kubernetes"}`, AnnotationsJSON: `{"summary":"Kubernetes 节点未就绪"}`, Description: "节点连续 5 分钟 NotReady，检查 kubelet、网络和节点资源。", Source: "platform", Status: 1},
		{Name: "Kubernetes Pod 异常", Category: "Kubernetes", Collector: "kube-state-metrics", ObjectType: "Pod", DatasourceType: "prometheus", QueryText: "sum by (namespace, pod, phase) (kube_pod_status_phase{phase=~\"Failed|Unknown|Pending\"})", Comparator: ">", Threshold: 0, ForSeconds: 300, EvalIntervalSeconds: 60, Severity: "P1", LabelsJSON: `{"domain":"kubernetes"}`, AnnotationsJSON: `{"summary":"Pod 处于异常状态"}`, Description: "聚合 Failed、Unknown 与长期 Pending 的 Pod。", Source: "platform", Status: 1},
		{Name: "Kubernetes Pod 重启频繁", Category: "Kubernetes", Collector: "kube-state-metrics", ObjectType: "Pod", DatasourceType: "prometheus", QueryText: "sum by (namespace, pod) (increase(kube_pod_container_status_restarts_total[10m]))", Comparator: ">", Threshold: 3, ForSeconds: 0, EvalIntervalSeconds: 60, Severity: "P2", LabelsJSON: `{"domain":"kubernetes"}`, AnnotationsJSON: `{"summary":"Pod 在 10 分钟内重启频繁"}`, Description: "超过 3 次时检查 OOM、探针失败和应用错误。", Source: "platform", Status: 1},
		{Name: "Deployment 副本不可用", Category: "Kubernetes", Collector: "kube-state-metrics", ObjectType: "Deployment", DatasourceType: "prometheus", QueryText: "kube_deployment_status_replicas_unavailable", Comparator: ">", Threshold: 0, ForSeconds: 300, EvalIntervalSeconds: 60, Severity: "P1", LabelsJSON: `{"domain":"kubernetes"}`, AnnotationsJSON: `{"summary":"Deployment 存在不可用副本"}`, Description: "工作负载副本无法就绪，检查镜像、资源、调度和事件。", Source: "platform", Status: 1},

		// MySQL templates are derived from the MySQL monitoring dashboard. Rates
		// use a five-minute window to avoid alerting on one-off spikes.
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 服务不可用", "mysql_up", "==", 0, 120, "P1", "mysql_availability", "采集目标连续不可达，确认 MySQL 进程、网络、账号和 Exporter。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 最近重启", "mysql_global_status_uptime", "<", 300, 0, "P2", "mysql_availability", "运行时长不足 5 分钟，确认是否为计划内重启或异常退出。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 连接数使用率过高", "(mysql_global_status_threads_connected / clamp_min(mysql_global_variables_max_connections, 1)) * 100", ">", 85, 300, "P1", "mysql_connection", "连接使用率持续超过 85%，检查连接泄漏、慢查询与连接池配置。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 活跃线程数过高", "mysql_global_status_threads_running", ">", 50, 300, "P2", "mysql_connection", "活跃执行线程持续偏高，检查慢 SQL、锁等待和突发流量。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 慢查询激增", "increase(mysql_global_status_slow_queries[5m])", ">", 10, 300, "P2", "mysql_query", "5 分钟内新增慢查询超过 10 条，排查执行计划、索引和热点 SQL。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 异常客户端断开", "increase(mysql_global_status_aborted_clients[5m])", ">", 10, 300, "P2", "mysql_connection", "客户端异常断开持续增加，检查网络、连接超时和应用连接池。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 异常连接失败", "increase(mysql_global_status_aborted_connects[5m])", ">", 10, 300, "P2", "mysql_connection", "连接失败持续增加，检查认证、网络、防火墙和连接数上限。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL InnoDB 脏页率过高", "(mysql_global_status_buffer_pool_bytes_dirty / clamp_min(mysql_global_status_buffer_pool_bytes_total, 1)) * 100", ">", 20, 300, "P2", "mysql_innodb", "Buffer Pool 脏页率持续偏高，检查磁盘 I/O、刷脏速度与写入压力。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL InnoDB 缓冲池命中率过低", "(1 - rate(mysql_global_status_innodb_buffer_pool_reads[5m]) / clamp_min(rate(mysql_global_status_innodb_buffer_pool_read_requests[5m]), 1)) * 100", "<", 99, 600, "P2", "mysql_innodb", "缓冲池命中率持续低于 99%，检查内存配置、工作集与异常全表扫描。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 行锁等待过多", "increase(mysql_global_status_innodb_row_lock_waits[5m])", ">", 10, 300, "P2", "mysql_lock", "5 分钟内行锁等待次数过多，定位长事务和锁冲突 SQL。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 锁等待耗时过长", "increase(mysql_global_status_innodb_row_lock_time[5m]) / 1000", ">", 10, 300, "P1", "mysql_lock", "5 分钟累计行锁等待超过 10 秒，优先检查阻塞事务。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 临时表落盘比例过高", "(rate(mysql_global_status_created_tmp_disk_tables[5m]) / clamp_min(rate(mysql_global_status_created_tmp_tables[5m]), 1)) * 100", ">", 25, 600, "P2", "mysql_query", "磁盘临时表占比持续超过 25%，检查排序、分组 SQL 与临时表参数。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL 打开文件使用率过高", "(mysql_global_status_open_files / clamp_min(mysql_global_variables_open_files_limit, 1)) * 100", ">", 85, 300, "P1", "mysql_capacity", "打开文件使用率持续超过 85%，检查表数量、连接数和文件句柄上限。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL Prepared Statement 使用率过高", "(mysql_global_status_prepared_stmt_count / clamp_min(mysql_global_variables_max_prepared_stmt_count, 1)) * 100", ">", 85, 300, "P2", "mysql_capacity", "预处理语句使用率持续超过 85%，检查语句关闭和最大预处理语句配置。"),
		makeDatabaseTemplate("MySQL", "mysqld_exporter", "MySQL Exporter 抓取耗时过长", "mysql_scrape_use_seconds", ">", 5, 300, "P2", "mysql_exporter", "Exporter 单次抓取持续超过 5 秒，检查数据库负载、网络与采集权限。"),

		// Redis templates are derived from the Redis monitoring dashboard.
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 服务不可用", "redis_up", "==", 0, 120, "P1", "redis_availability", "采集目标连续不可达，确认 Redis 进程、网络、认证与 Exporter。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 最近重启", "redis_uptime_in_seconds", "<", 300, 0, "P2", "redis_availability", "运行时长不足 5 分钟，确认是否为计划内重启或异常退出。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 内存使用率过高", "100 * redis_memory_used_bytes / clamp_min((redis_memory_max_bytes > 0) or on(instance) redis_memory_used_peak_bytes, 1)", ">", 85, 300, "P1", "redis_memory", "内存使用率持续超过 85%，检查 maxmemory、淘汰策略与大 Key。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 缓存命中率过低", "100 * rate(redis_keyspace_hits_total[5m]) / clamp_min(rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]), 1)", "<", 80, 600, "P2", "redis_cache", "缓存命中率持续低于 80%，检查缓存预热、过期策略和访问模式。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis P95 响应延迟过高", "histogram_quantile(0.95, sum by (le, instance) (rate(redis_commands_latencies_usec_bucket[5m]))) / 1000", ">", 50, 300, "P2", "redis_latency", "P95 命令延迟持续超过 50ms，检查慢命令、CPU、网络和持久化压力。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 错误响应速率过高", "rate(redis_total_error_replies[5m])", ">", 1, 300, "P2", "redis_error", "错误响应速率持续超过 1 次/秒，检查命令错误、权限与客户端行为。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 拒绝连接", "increase(redis_rejected_connections_total[5m])", ">", 0, 0, "P1", "redis_connection", "出现被拒绝的连接，检查 maxclients、文件句柄和连接泄漏。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 阻塞客户端过多", "redis_blocked_clients", ">", 50, 300, "P2", "redis_connection", "阻塞客户端持续超过 50，检查阻塞命令、慢 Lua 脚本和下游依赖。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis Key 被淘汰", "increase(redis_evicted_keys_total[5m])", ">", 0, 0, "P1", "redis_memory", "发生 Key 淘汰，表示内存水位或淘汰策略已影响业务缓存。"),
		makeDatabaseTemplate("Redis", "redis_exporter", "Redis 慢日志堆积", "redis_slowlog_length", ">", 10, 300, "P2", "redis_latency", "慢日志队列持续超过 10，检查慢命令、大 Key 与阻塞操作。"),
	}
	// The available-ratio definition replaces the old inverted usage-ratio name.
	// Alert rules created from the old template are independent and remain intact.
	if err := db.Where("source = ? AND name IN ?", "platform", []string{"主机内存使用率过高"}).Delete(&model.MonitorAlertTemplate{}).Error; err != nil {
		return err
	}
	// This platform uses SSH for host access and node_exporter-compatible metrics
	// from Prometheus/VictoriaMetrics for alert evaluation. It has no Agent data
	// collection path, so remove the mistakenly seeded Agent-only platform rules.
	if err := db.Where("source = ? AND collector = ?", "platform", "agent").Delete(&model.MonitorAlertTemplate{}).Error; err != nil {
		return err
	}
	if err := db.Where("source = ? AND collector = ?", "platform", "datasource-health").Delete(&model.MonitorAlertTemplate{}).Error; err != nil {
		return err
	}
	var platformGroup, datasourceHealthGroup model.MonitorAlertTemplateGroup
	if err := db.Where("parent_id = ? AND name = ?", 0, "平台").First(&platformGroup).Error; err == nil {
		if err := db.Where("parent_id = ? AND name = ?", platformGroup.ID, "datasource-health").First(&datasourceHealthGroup).Error; err == nil {
			var remaining int64
			if err := db.Model(&model.MonitorAlertTemplate{}).Where("group_id = ?", datasourceHealthGroup.ID).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				_ = db.Delete(&datasourceHealthGroup).Error
			}
		}
	}
	var linuxGroup, agentGroup model.MonitorAlertTemplateGroup
	if err := db.Where("parent_id = ? AND name = ?", 0, "Linux").First(&linuxGroup).Error; err == nil {
		if err := db.Where("parent_id = ? AND name = ?", linuxGroup.ID, "agent").First(&agentGroup).Error; err == nil {
			var remaining int64
			if err := db.Model(&model.MonitorAlertTemplate{}).Where("group_id = ?", agentGroup.ID).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				if err := db.Delete(&agentGroup).Error; err != nil {
					return err
				}
			}
		}
	}
	for _, item := range templates {
		root, collector := item.Category, item.Collector
		rootID := groupIDs[fmt.Sprintf("%d/%s", uint(0), root)]
		item.GroupID = groupIDs[fmt.Sprintf("%d/%s", rootID, collector)]
		if rootID == 0 || item.GroupID == 0 {
			return fmt.Errorf("resolve built-in alert template group failed: %s/%s", root, collector)
		}
		var count int64
		if err := db.Model(&model.MonitorAlertTemplate{}).Where("name = ? AND source = ?", item.Name, "platform").Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&item).Error; err != nil {
				return err
			}
		} else {
			updates := map[string]any{
				"group_id":              item.GroupID,
				"category":              item.Category,
				"collector":             item.Collector,
				"object_type":           item.ObjectType,
				"datasource_type":       item.DatasourceType,
				"query_text":            item.QueryText,
				"comparator":            item.Comparator,
				"threshold":             item.Threshold,
				"for_seconds":           item.ForSeconds,
				"eval_interval_seconds": item.EvalIntervalSeconds,
				"severity":              item.Severity,
				"labels_json":           item.LabelsJSON,
				"annotations_json":      item.AnnotationsJSON,
				"description":           item.Description,
				"status":                item.Status,
			}
			if err := db.Model(&model.MonitorAlertTemplate{}).Where("name = ? AND source = ?", item.Name, "platform").Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedSystemConfig(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.SystemConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		var cfg model.SystemConfig
		if err := db.First(&cfg).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if cfg.SiteSlogan == "Personal operations platform" {
			updates["site_slogan"] = "个人运维管理平台"
		}
		if cfg.LoginSubtitle == "System management and operation console" {
			updates["login_subtitle"] = "系统管理与运维控制台"
		}
		if len(updates) == 0 {
			return nil
		}
		return db.Model(&cfg).Updates(updates).Error
	}

	return db.Create(&model.SystemConfig{
		SiteName:           "Ops Admin",
		SiteSlogan:         "个人运维管理平台",
		LogoType:           "text",
		LogoValue:          "OA",
		LoginTitle:         "Ops Admin",
		LoginSubtitle:      "系统管理与运维控制台",
		UseLoginBackground: false,
		PrimaryColor:       "#5b6cf9",
		SidebarTheme:       "dark",
		CreatedAt:          time.Now(),
	}).Error
}

func seedDept(db *gorm.DB) error {
	var count int64
	db.Model(&model.Dept{}).Count(&count)
	if count > 0 {
		return nil
	}
	return db.Create(&model.Dept{
		ParentID:   0,
		DeptType:   1,
		DeptName:   "总部",
		DeptStatus: 1,
		CreatedAt:  time.Now(),
	}).Error
}

func seedPost(db *gorm.DB) error {
	var count int64
	db.Model(&model.Post{}).Count(&count)
	if count > 0 {
		return nil
	}
	return db.Create(&model.Post{
		PostCode:   "super-admin",
		PostName:   "超级管理员",
		PostStatus: 1,
		Remark:     "系统初始化岗位",
		CreatedAt:  time.Now(),
	}).Error
}

func seedRole(db *gorm.DB) error {
	var count int64
	db.Model(&model.Role{}).Count(&count)
	if count > 0 {
		return nil
	}
	return db.Create(&model.Role{
		RoleName:    "超级管理员",
		RoleKey:     "super-admin",
		Status:      1,
		Description: "系统初始化角色",
		CreatedAt:   time.Now(),
	}).Error
}

func seedMenus(db *gorm.DB) error {
	systemRoot, err := ensureMenu(db, model.Menu{
		ParentID:   0,
		MenuName:   "系统管理",
		MenuType:   1,
		URL:        "/system",
		Value:      "system",
		MenuStatus: 1,
		Sort:       1,
		Icon:       "Setting",
	})
	if err != nil {
		return err
	}
	businessTopologyMenu, err := ensureMenu(db, model.Menu{
		ParentID:   0,
		MenuName:   "业务拓扑图",
		MenuType:   2,
		URL:        "/business-topology",
		Value:      "console:business-topology",
		MenuStatus: 1,
		Sort:       0,
		Icon:       "Share",
	})
	if err != nil {
		return err
	}

	logRoot, err := ensureMenu(db, model.Menu{
		ParentID:   0,
		MenuName:   "操作审计",
		MenuType:   1,
		URL:        "/logs",
		Value:      "logs",
		MenuStatus: 1,
		Sort:       2,
		Icon:       "Document",
	})
	if err != nil {
		return err
	}

	adminMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "用户信息", MenuType: 2, URL: "/system/admin", Value: "system:admin:list", MenuStatus: 1, Sort: 1, Icon: "User"})
	if err != nil {
		return err
	}
	roleMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "角色信息", MenuType: 2, URL: "/system/role", Value: "system:role:list", MenuStatus: 1, Sort: 2, Icon: "Avatar"})
	if err != nil {
		return err
	}
	menuMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "菜单信息", MenuType: 2, URL: "/system/menu", Value: "system:menu:list", MenuStatus: 1, Sort: 3, Icon: "Grid"})
	if err != nil {
		return err
	}
	deptMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "部门信息", MenuType: 2, URL: "/system/dept", Value: "system:dept:list", MenuStatus: 1, Sort: 4, Icon: "OfficeBuilding"})
	if err != nil {
		return err
	}
	postMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "岗位信息", MenuType: 2, URL: "/system/post", Value: "system:post:list", MenuStatus: 1, Sort: 5, Icon: "Suitcase"})
	if err != nil {
		return err
	}
	configMenu, err := ensureMenu(db, model.Menu{ParentID: systemRoot.ID, MenuName: "系统设置", MenuType: 2, URL: "/system/settings", Value: "system:config:view", MenuStatus: 1, Sort: 6, Icon: "Tools"})
	if err != nil {
		return err
	}
	loginLogMenu, err := ensureMenu(db, model.Menu{ParentID: logRoot.ID, MenuName: "登录日志", MenuType: 2, URL: "/logs/login", Value: "system:loginlog:list", MenuStatus: 1, Sort: 1, Icon: "Tickets"})
	if err != nil {
		return err
	}
	operationLogMenu, err := ensureMenu(db, model.Menu{ParentID: logRoot.ID, MenuName: "操作日志", MenuType: 2, URL: "/logs/operation", Value: "system:operationlog:list", MenuStatus: 1, Sort: 2, Icon: "Memo"})
	if err != nil {
		return err
	}

	buttons := []model.Menu{
		{ParentID: businessTopologyMenu.ID, MenuName: "查看业务拓扑", MenuType: 3, Value: "console:business-topology:view", MenuStatus: 1, Sort: 1},
		{ParentID: adminMenu.ID, MenuName: "新增用户", MenuType: 3, Value: "system:admin:add", MenuStatus: 1, Sort: 1},
		{ParentID: adminMenu.ID, MenuName: "编辑用户", MenuType: 3, Value: "system:admin:edit", MenuStatus: 1, Sort: 2},
		{ParentID: adminMenu.ID, MenuName: "删除用户", MenuType: 3, Value: "system:admin:delete", MenuStatus: 1, Sort: 3},
		{ParentID: adminMenu.ID, MenuName: "切换状态", MenuType: 3, Value: "system:admin:status", MenuStatus: 1, Sort: 4},
		{ParentID: adminMenu.ID, MenuName: "重置密码", MenuType: 3, Value: "system:admin:resetpwd", MenuStatus: 1, Sort: 5},
		{ParentID: adminMenu.ID, MenuName: "同步 LDAP 用户", MenuType: 3, Value: "system:admin:ldapSync", MenuStatus: 1, Sort: 6},

		{ParentID: roleMenu.ID, MenuName: "新增角色", MenuType: 3, Value: "system:role:add", MenuStatus: 1, Sort: 1},
		{ParentID: roleMenu.ID, MenuName: "编辑角色", MenuType: 3, Value: "system:role:edit", MenuStatus: 1, Sort: 2},
		{ParentID: roleMenu.ID, MenuName: "删除角色", MenuType: 3, Value: "system:role:delete", MenuStatus: 1, Sort: 3},
		{ParentID: roleMenu.ID, MenuName: "切换状态", MenuType: 3, Value: "system:role:status", MenuStatus: 1, Sort: 4},
		{ParentID: roleMenu.ID, MenuName: "分配权限", MenuType: 3, Value: "system:role:assign", MenuStatus: 1, Sort: 5},

		{ParentID: menuMenu.ID, MenuName: "新增菜单", MenuType: 3, Value: "system:menu:add", MenuStatus: 1, Sort: 1},
		{ParentID: menuMenu.ID, MenuName: "编辑菜单", MenuType: 3, Value: "system:menu:edit", MenuStatus: 1, Sort: 2},
		{ParentID: menuMenu.ID, MenuName: "删除菜单", MenuType: 3, Value: "system:menu:delete", MenuStatus: 1, Sort: 3},

		{ParentID: deptMenu.ID, MenuName: "新增部门", MenuType: 3, Value: "system:dept:add", MenuStatus: 1, Sort: 1},
		{ParentID: deptMenu.ID, MenuName: "编辑部门", MenuType: 3, Value: "system:dept:edit", MenuStatus: 1, Sort: 2},
		{ParentID: deptMenu.ID, MenuName: "删除部门", MenuType: 3, Value: "system:dept:delete", MenuStatus: 1, Sort: 3},

		{ParentID: postMenu.ID, MenuName: "新增岗位", MenuType: 3, Value: "system:post:add", MenuStatus: 1, Sort: 1},
		{ParentID: postMenu.ID, MenuName: "编辑岗位", MenuType: 3, Value: "system:post:edit", MenuStatus: 1, Sort: 2},
		{ParentID: postMenu.ID, MenuName: "删除岗位", MenuType: 3, Value: "system:post:delete", MenuStatus: 1, Sort: 3},
		{ParentID: postMenu.ID, MenuName: "切换状态", MenuType: 3, Value: "system:post:status", MenuStatus: 1, Sort: 4},

		{ParentID: configMenu.ID, MenuName: "保存配置", MenuType: 3, Value: "system:config:save", MenuStatus: 1, Sort: 1},
		{ParentID: configMenu.ID, MenuName: "配置 LDAP 集成", MenuType: 3, Value: "system:config:ldap", MenuStatus: 1, Sort: 2},

		{ParentID: loginLogMenu.ID, MenuName: "删除登录日志", MenuType: 3, Value: "system:loginlog:delete", MenuStatus: 1, Sort: 1},
		{ParentID: loginLogMenu.ID, MenuName: "清空登录日志", MenuType: 3, Value: "system:loginlog:clean", MenuStatus: 1, Sort: 2},

		{ParentID: operationLogMenu.ID, MenuName: "删除操作日志", MenuType: 3, Value: "system:operationlog:delete", MenuStatus: 1, Sort: 1},
		{ParentID: operationLogMenu.ID, MenuName: "清空操作日志", MenuType: 3, Value: "system:operationlog:clean", MenuStatus: 1, Sort: 2},
	}

	for _, button := range buttons {
		if _, err := ensureMenu(db, button); err != nil {
			return err
		}
	}

	return seedApplicationMenus(db)
}

// seedApplicationMenus mirrors the application switcher navigation in sys_menu.
// This keeps menu administration and role permission assignment aware of every
// functional area, even though the five application sidebars are rendered by the
// frontend's application layout.
func seedApplicationMenus(db *gorm.DB) error {
	type menuSeed struct {
		name  string
		url   string
		value string
		icon  string
	}
	type appSeed struct {
		name     string
		url      string
		value    string
		icon     string
		children []menuSeed
	}
	type buttonSeed struct {
		parentValue string
		name        string
		value       string
	}

	// Application topology was retired. Remove the historical menu and its
	// role links so an existing deployment does not keep showing a stale entry.
	var retiredMenus []model.Menu
	if err := db.Where("value = ? OR url = ?", "applications:topology", "/applications/topology").Find(&retiredMenus).Error; err != nil {
		return err
	}
	if len(retiredMenus) > 0 {
		ids := make([]uint, 0, len(retiredMenus))
		for _, menu := range retiredMenus {
			ids = append(ids, menu.ID)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("menu_id IN ?", ids).Delete(&model.RoleMenu{}).Error; err != nil {
				return err
			}
			return tx.Where("id IN ?", ids).Delete(&model.Menu{}).Error
		}); err != nil {
			return err
		}
	}

	applications := []appSeed{
		{
			name: "资产管理", url: "/assets", value: "assets", icon: "Box",
			children: []menuSeed{
				{"资产概览", "/assets/overview", "assets:overview", "DataBoard"},
				{"环境模型", "/assets/environments", "ops:environment:list", "SetUp"},
				{"终端登录", "/assets/terminal", "assets:terminal", "Platform"},
				{"主机管理", "/assets/server/hosts", "assets:host:list", "Monitor"},
				{"主机组管理", "/assets/server/groups", "assets:hostgroup:list", "FolderOpened"},
				{"凭据管理", "/assets/server/credentials", "assets:credential:list", "Key"},
				{"云账号管理", "/assets/server/cloud-accounts", "assets:cloudaccount:list", "Cloudy"},
				{"数据库列表", "/assets/databases", "assets:database:list", "List"},
				{"DBMS 工作台", "/assets/databases/workbench", "assets:database:workbench", "EditPen"},
				{"数据导入", "/assets/databases/import", "assets:database:import", "Upload"},
				{"备份管理", "/assets/databases/backups", "assets:database:backup", "FolderOpened"},
				{"网关管理", "/assets/gateways", "assets:gateway:list", "Switch"},
			},
		},
		{
			name: "容器管理", url: "/containers", value: "containers", icon: "Box",
			children: []menuSeed{
				{"集群管理", "/containers/k8s/clusters", "assets:k8s:cluster", "FolderOpened"},
				{"集群概览", "/containers/k8s/overview", "assets:k8s:overview", "DataAnalysis"},
				{"节点管理", "/containers/k8s/nodes", "assets:k8s:node", "Monitor"},
				{"命名空间", "/containers/k8s/namespaces", "assets:k8s:namespace", "Grid"},
				{"工作负载", "/containers/k8s/workloads", "assets:k8s:workload", "SetUp"},
				{"Pod 管理", "/containers/k8s/pods", "assets:k8s:pod", "Box"},
				{"服务", "/containers/k8s/services", "assets:k8s:service", "Share"},
				{"Ingress", "/containers/k8s/ingresses", "assets:k8s:ingress", "Connection"},
				{"高级网络", "/containers/k8s/advanced-network", "assets:k8s:advancednetwork", "Connection"},
				{"配置与存储", "/containers/k8s/config-storage", "assets:k8s:configstorage", "Files"},
			},
		},
		{
			name: "标准运维", url: "/ops", value: "ops", icon: "Operation",
			children: []menuSeed{
				{"脚本库", "/ops/scripts/library", "ops:script:list", "Document"},
				{"命令执行", "/ops/quick-exec/command", "ops:quickexec:command", "CaretRight"},
				{"脚本执行", "/ops/quick-exec/script", "ops:quickexec:script", "EditPen"},
				{"文件分发", "/ops/quick-exec/file-dispatch", "ops:quickexec:file", "Files"},
				{"快速执行历史", "/ops/quick-exec/history", "ops:quickexec:history", "DocumentCopy"},
				{"定时任务", "/ops/schedule/tasks", "ops:schedule:task", "Clock"},
				{"任务日志", "/ops/schedule/logs", "ops:schedule:log", "Document"},
				{"任务模板", "/ops/schedule/templates", "ops:schedule:template", "Tickets"},
				{"作业编排", "/ops/jobs/designer", "ops:job:designer", "Grid"},
				{"作业列表", "/ops/jobs/list", "ops:job:list", "List"},
				{"人工确认", "/ops/jobs/approvals", "ops:job:approval", "Bell"},
				{"作业历史", "/ops/jobs/history", "ops:job:history", "Document"},
				{"作业模板", "/ops/jobs/templates", "ops:job:template", "Tickets"},
			},
		},
		{
			name: "应用中心", url: "/applications", value: "applications", icon: "Box",
			children: []menuSeed{
				{"应用管理", "/applications/projects", "applications:project:list", "Tickets"},
				{"构建任务", "/applications/build-tasks", "applications:buildtask:list", "Operation"},
				{"构建历史", "/applications/build-history", "applications:buildhistory:list", "Document"},
				{"CI/CD 流水线", "/applications/pipelines", "applications:pipeline:list", "Share"},
			},
		},
		{
			name: "消息通知", url: "/notify", value: "notify", icon: "Bell",
			children: []menuSeed{
				{"通知规则", "/notify/rules", "notify:rule:list", "Operation"},
				{"消息模板", "/notify/templates", "notify:template:list", "Document"},
				{"通知媒介", "/notify/channels", "notify:channel:list", "Connection"},
				{"发送日志", "/notify/send-logs", "notify:sendlog:list", "Tickets"},
			},
		},
		{
			name: "监控中心", url: "/monitor", value: "monitor", icon: "Histogram",
			children: []menuSeed{
				{"监控概览", "/monitor/overview", "monitor:overview", "TrendCharts"},
				{"智能大屏", "/monitor/command-center", "monitor:commandcenter", "DataBoard"},
				{"数据源管理", "/monitor/datasources", "monitor:datasource:list", "Connection"},
				{"即时查询", "/monitor/query", "monitor:query", "Search"},
				{"日志查询", "/monitor/logs", "monitor:logs", "Document"},
				{"链路追踪", "/monitor/traces", "monitor:traces", "Share"},
				{"告警模板", "/monitor/alert-templates", "monitor:alerttemplate:list", "CollectionTag"},
				{"告警规则", "/monitor/alert-rules", "monitor:alertrule:list", "Bell"},
				{"告警事件", "/monitor/alert-events", "monitor:alertevent:list", "Warning"},
				{"告警屏蔽", "/monitor/silences", "monitor:silence:list", "MuteNotification"},
				{"聚合收敛", "/monitor/aggregations", "monitor:aggregation:list", "Filter"},
				{"监控大屏", "/monitor/dashboards", "monitor:dashboard:list", "PieChart"},
				{"巡检大屏", "/monitor/inspections", "monitor:inspection:list", "Tickets"},
			},
		},
		{
			name: "域名管理", url: "/domains", value: "domains", icon: "Connection",
			children: []menuSeed{
				{"公网域名", "/domains/public", "domains:public:list", "Position"},
				{"公网 DNS 账号", "/domains/public/accounts", "domains:account:list", "Key"},
				{"SSL 证书", "/domains/public/certificates", "domains:ssl:view", "Lock"},
				{"内网域名", "/domains/internal", "domains:internal:list", "OfficeBuilding"},
				{"DNS 设置", "/domains/internal/settings", "domains:settings:view", "Setting"},
				{"解析测试", "/domains/query-test", "domains:query:test", "Search"},
				{"操作审计", "/domains/audit", "domains:audit:list", "Memo"},
			},
		},
	}

	menuByValue := make(map[string]model.Menu)
	for appIndex, application := range applications {
		root, err := ensureMenu(db, model.Menu{
			ParentID:   0,
			MenuName:   application.name,
			MenuType:   1,
			URL:        application.url,
			Value:      application.value,
			MenuStatus: 1,
			Sort:       10 + appIndex,
			Icon:       application.icon,
		})
		if err != nil {
			return err
		}
		for childIndex, child := range application.children {
			menu, err := ensureMenu(db, model.Menu{
				ParentID:   root.ID,
				MenuName:   child.name,
				MenuType:   2,
				URL:        child.url,
				Value:      child.value,
				MenuStatus: 1,
				Sort:       childIndex + 1,
				Icon:       child.icon,
			})
			if err != nil {
				return err
			}
			menuByValue[child.value] = menu
		}
	}

	buttons := []buttonSeed{
		// Asset management
		{"assets:host:list", "新增主机", "assets:host:add"}, {"assets:host:list", "编辑主机", "assets:host:edit"}, {"assets:host:list", "删除主机", "assets:host:delete"}, {"assets:host:list", "复制主机", "assets:host:copy"}, {"assets:host:list", "终端登录", "assets:host:terminal"},
		{"assets:hostgroup:list", "新增主机组", "assets:hostgroup:add"}, {"assets:hostgroup:list", "编辑主机组", "assets:hostgroup:edit"}, {"assets:hostgroup:list", "删除主机组", "assets:hostgroup:delete"},
		{"assets:credential:list", "新增凭据", "assets:credential:add"}, {"assets:credential:list", "编辑凭据", "assets:credential:edit"}, {"assets:credential:list", "删除凭据", "assets:credential:delete"},
		{"assets:cloudaccount:list", "新增云账号", "assets:cloudaccount:add"}, {"assets:cloudaccount:list", "编辑云账号", "assets:cloudaccount:edit"}, {"assets:cloudaccount:list", "删除云账号", "assets:cloudaccount:delete"},
		{"assets:database:list", "新增数据库", "assets:database:add"}, {"assets:database:list", "编辑数据库", "assets:database:edit"}, {"assets:database:list", "删除数据库", "assets:database:delete"}, {"assets:database:list", "测试连接", "assets:database:test"},
		{"assets:database:workbench", "执行 SQL", "assets:database:sql:execute"}, {"assets:database:workbench", "编辑数据", "assets:database:data:edit"}, {"assets:database:workbench", "导出数据", "assets:database:export"},
		{"assets:database:import", "执行数据导入", "assets:database:import:execute"}, {"assets:database:backup", "手动备份", "assets:database:backup:create"}, {"assets:database:backup", "恢复备份", "assets:database:backup:restore"}, {"assets:database:backup", "删除备份", "assets:database:backup:delete"},
		{"assets:gateway:list", "新增网关", "assets:gateway:add"}, {"assets:gateway:list", "编辑网关", "assets:gateway:edit"}, {"assets:gateway:list", "删除网关", "assets:gateway:delete"}, {"assets:gateway:list", "测试网关", "assets:gateway:test"},
		{"assets:k8s:cluster", "新增集群", "assets:k8s:cluster:add"}, {"assets:k8s:cluster", "编辑集群", "assets:k8s:cluster:edit"}, {"assets:k8s:cluster", "删除集群", "assets:k8s:cluster:delete"},
		{"assets:k8s:workload", "新增工作负载", "assets:k8s:workload:create"}, {"assets:k8s:workload", "伸缩工作负载", "assets:k8s:workload:scale"}, {"assets:k8s:workload", "重启工作负载", "assets:k8s:workload:restart"}, {"assets:k8s:workload", "更新镜像", "assets:k8s:workload:image"}, {"assets:k8s:workload", "编辑 YAML", "assets:k8s:workload:yaml"},
		{"assets:k8s:pod", "进入 Pod 终端", "assets:k8s:pod:terminal"}, {"assets:k8s:pod", "删除 Pod", "assets:k8s:pod:delete"}, {"assets:k8s:pod", "编辑 YAML", "assets:k8s:pod:yaml"},

		// Standard operations
		{"ops:script:list", "新增脚本", "ops:script:add"}, {"ops:script:list", "编辑脚本", "ops:script:edit"}, {"ops:script:list", "删除脚本", "ops:script:delete"}, {"ops:script:list", "启用或禁用脚本", "ops:script:status"},
		{"ops:quickexec:command", "执行命令", "ops:quickexec:command:execute"}, {"ops:quickexec:script", "执行脚本", "ops:quickexec:script:execute"}, {"ops:quickexec:file", "分发文件", "ops:quickexec:file:execute"},
		{"ops:schedule:task", "新增定时任务", "ops:schedule:task:add"}, {"ops:schedule:task", "编辑定时任务", "ops:schedule:task:edit"}, {"ops:schedule:task", "删除定时任务", "ops:schedule:task:delete"}, {"ops:schedule:task", "启用或禁用定时任务", "ops:schedule:task:status"},
		{"ops:job:designer", "保存作业模板", "ops:job:template:save"}, {"ops:job:list", "执行作业", "ops:job:execute"}, {"ops:job:list", "删除作业", "ops:job:delete"}, {"ops:job:approval", "确认作业步骤", "ops:job:approve"},

		// Application center
		{"applications:project:list", "新增项目", "applications:project:add"}, {"applications:project:list", "编辑项目", "applications:project:edit"}, {"applications:project:list", "删除项目", "applications:project:delete"},
		{"applications:buildtask:list", "新增构建任务", "applications:buildtask:add"}, {"applications:buildtask:list", "编辑构建任务", "applications:buildtask:edit"}, {"applications:buildtask:list", "删除构建任务", "applications:buildtask:delete"}, {"applications:buildtask:list", "立即构建", "applications:buildtask:run"},
		{"applications:pipeline:list", "新增流水线", "applications:pipeline:add"}, {"applications:pipeline:list", "编辑流水线", "applications:pipeline:edit"}, {"applications:pipeline:list", "删除流水线", "applications:pipeline:delete"}, {"applications:pipeline:list", "执行流水线", "applications:pipeline:run"},

		// Notification center
		{"notify:rule:list", "新增通知规则", "notify:rule:add"}, {"notify:rule:list", "编辑通知规则", "notify:rule:edit"}, {"notify:rule:list", "删除通知规则", "notify:rule:delete"},
		{"notify:template:list", "新增消息模板", "notify:template:add"}, {"notify:template:list", "编辑消息模板", "notify:template:edit"}, {"notify:template:list", "删除消息模板", "notify:template:delete"},
		{"notify:channel:list", "新增通知媒介", "notify:channel:add"}, {"notify:channel:list", "编辑通知媒介", "notify:channel:edit"}, {"notify:channel:list", "删除通知媒介", "notify:channel:delete"}, {"notify:channel:list", "测试通知媒介", "notify:channel:test"},

		// Monitoring center
		{"monitor:datasource:list", "新增数据源", "monitor:datasource:add"}, {"monitor:datasource:list", "编辑数据源", "monitor:datasource:edit"}, {"monitor:datasource:list", "删除数据源", "monitor:datasource:delete"}, {"monitor:datasource:list", "测试数据源", "monitor:datasource:test"},
		{"monitor:alerttemplate:list", "新增告警模板", "monitor:alerttemplate:add"}, {"monitor:alerttemplate:list", "编辑告警模板", "monitor:alerttemplate:edit"}, {"monitor:alerttemplate:list", "删除告警模板", "monitor:alerttemplate:delete"},
		{"monitor:alertrule:list", "新增告警规则", "monitor:alertrule:add"}, {"monitor:alertrule:list", "编辑告警规则", "monitor:alertrule:edit"}, {"monitor:alertrule:list", "删除告警规则", "monitor:alertrule:delete"}, {"monitor:alertrule:list", "批量更新告警规则", "monitor:alertrule:batch"},
		{"monitor:alertevent:list", "认领告警", "monitor:alertevent:claim"}, {"monitor:alertevent:list", "关闭告警", "monitor:alertevent:close"}, {"monitor:alertevent:list", "删除告警事件", "monitor:alertevent:delete"},
		{"monitor:silence:list", "新增屏蔽规则", "monitor:silence:add"}, {"monitor:silence:list", "编辑屏蔽规则", "monitor:silence:edit"}, {"monitor:silence:list", "删除屏蔽规则", "monitor:silence:delete"},
		{"monitor:aggregation:list", "新增收敛规则", "monitor:aggregation:add"}, {"monitor:aggregation:list", "编辑收敛规则", "monitor:aggregation:edit"}, {"monitor:aggregation:list", "删除收敛规则", "monitor:aggregation:delete"},
		{"monitor:dashboard:list", "创建监控大屏", "monitor:dashboard:add"}, {"monitor:dashboard:list", "编辑监控大屏", "monitor:dashboard:edit"}, {"monitor:dashboard:list", "删除监控大屏", "monitor:dashboard:delete"},

		// Domain management
		{"domains:account:list", "新增 DNS 账号", "domains:account:add"}, {"domains:account:list", "编辑 DNS 账号", "domains:account:edit"}, {"domains:account:list", "删除 DNS 账号", "domains:account:delete"}, {"domains:account:list", "测试 DNS 账号", "domains:account:test"},
		{"domains:public:list", "同步公网域名", "domains:public:sync"}, {"domains:public:list", "管理公网解析记录", "domains:public:record"}, {"domains:public:list", "批量操作公网记录", "domains:public:batch"},
		{"domains:ssl:view", "同步 SSL 证书", "domains:ssl:sync"}, {"domains:ssl:view", "申请 SSL 证书", "domains:ssl:apply"}, {"domains:ssl:view", "上传 SSL 证书", "domains:ssl:upload"}, {"domains:ssl:view", "续签 SSL 证书", "domains:ssl:renew"}, {"domains:ssl:view", "删除 SSL 证书", "domains:ssl:delete"}, {"domains:ssl:view", "下载 SSL 证书", "domains:ssl:download"}, {"domains:ssl:view", "下载证书私钥", "domains:ssl:download-key"}, {"domains:ssl:view", "修改自动续签", "domains:ssl:settings"},
		{"domains:internal:list", "新增内网 Zone", "domains:internal:zone:add"}, {"domains:internal:list", "编辑内网 Zone", "domains:internal:zone:edit"}, {"domains:internal:list", "删除内网 Zone", "domains:internal:zone:delete"}, {"domains:internal:list", "管理内网解析记录", "domains:internal:record"},
		{"domains:settings:view", "保存 DNS 设置", "domains:settings:save"},
	}

	for sort, button := range buttons {
		parent, found := menuByValue[button.parentValue]
		if !found {
			continue
		}
		if _, err := ensureMenu(db, model.Menu{
			ParentID:   parent.ID,
			MenuName:   button.name,
			MenuType:   3,
			Value:      button.value,
			MenuStatus: 1,
			Sort:       sort + 1,
		}); err != nil {
			return err
		}
	}

	return nil
}

func seedAdmin(db *gorm.DB) error {
	var count int64
	db.Model(&model.Admin{}).Count(&count)
	if count > 0 {
		return nil
	}

	initialPassword := strings.TrimSpace(os.Getenv("OPS_ADMIN_INITIAL_PASSWORD"))
	if initialPassword == "" {
		return errors.New("OPS_ADMIN_INITIAL_PASSWORD must be set before initializing the first administrator")
	}
	username := strings.TrimSpace(os.Getenv("OPS_ADMIN_INITIAL_USERNAME"))
	if username == "" {
		username = "admin"
	}

	var dept model.Dept
	var post model.Post
	var role model.Role
	if err := db.First(&dept).Error; err != nil {
		return err
	}
	if err := db.First(&post).Error; err != nil {
		return err
	}
	if err := db.First(&role).Error; err != nil {
		return err
	}

	admin := model.Admin{
		PostID:    post.ID,
		DeptID:    dept.ID,
		Username:  username,
		Password:  util.HashPassword(initialPassword),
		Nickname:  "系统管理员",
		Status:    1,
		Email:     "admin@example.com",
		Phone:     "13800000000",
		Note:      "初始化管理员",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}

	return db.Create(&model.AdminRole{
		AdminID: admin.ID,
		RoleID:  role.ID,
	}).Error
}

func seedSuperRolePermissions(db *gorm.DB) error {
	var role model.Role
	if err := db.Where("role_key = ?", "super-admin").First(&role).Error; err != nil {
		return err
	}

	var menus []model.Menu
	if err := db.Find(&menus).Error; err != nil {
		return err
	}

	for _, menu := range menus {
		var count int64
		if err := db.Model(&model.RoleMenu{}).Where("role_id = ? and menu_id = ?", role.ID, menu.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := db.Create(&model.RoleMenu{RoleID: role.ID, MenuID: menu.ID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureMenu(db *gorm.DB, menu model.Menu) (model.Menu, error) {
	var existing model.Menu
	err := db.Where("value = ? AND value <> ''", menu.Value).First(&existing).Error
	if err == nil {
		updates := map[string]any{
			"parent_id":   menu.ParentID,
			"menu_name":   menu.MenuName,
			"menu_type":   menu.MenuType,
			"url":         menu.URL,
			"menu_status": menu.MenuStatus,
			"sort":        menu.Sort,
			"icon":        menu.Icon,
		}
		if updateErr := db.Model(&existing).Updates(updates).Error; updateErr != nil {
			return existing, updateErr
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}

	menu.CreatedAt = time.Now()
	if err := db.Create(&menu).Error; err != nil {
		return existing, err
	}
	return menu, nil
}
