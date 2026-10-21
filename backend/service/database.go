package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/model"

	_ "github.com/go-sql-driver/mysql"
)

type DBMSSQLExecutePayload struct {
	DatabaseID uint   `json:"databaseId"`
	Schema     string `json:"schema"`
	SQLText    string `json:"sqlText"`
	Confirmed  bool   `json:"confirmed"`
	Operator   string `json:"-"`
	ClientIP   string `json:"-"`
}

// DBMSCreateDatabasePayload creates a database on MySQL, or a schema in the
// currently connected PostgreSQL database. PostgreSQL's workbench tree is a
// schema tree, so creating a schema keeps the new object immediately usable.
type DBMSCreateDatabasePayload struct {
	DatabaseID uint   `json:"databaseId"`
	Name       string `json:"name"`
	Charset    string `json:"charset"`
	Collation  string `json:"collation"`
	Operator   string `json:"-"`
}

var mysqlNamedForeignKeyPattern = regexp.MustCompile("(?i)CONSTRAINT\\s+(?:`[^`]+`|[a-zA-Z0-9_]+)\\s+(FOREIGN\\s+KEY)")
var databaseObjectNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
var mysqlCharsetOptionPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// mysqlImportCreateTableSQL keeps foreign keys during an automatic table copy,
// but drops their source-side names. MySQL requires foreign key names to be
// unique within a schema, so reusing the source name can make the import fail.
func mysqlImportCreateTableSQL(createSQL, sourceTable, targetTable string) string {
	createSQL = strings.Replace(
		createSQL,
		"CREATE TABLE `"+sourceTable+"`",
		"CREATE TABLE IF NOT EXISTS "+quoteIdentifier(targetTable),
		1,
	)
	return mysqlNamedForeignKeyPattern.ReplaceAllString(createSQL, "$1")
}

type DBMSTableDataQueryPayload struct {
	DatabaseID uint   `json:"databaseId"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	PageNum    int    `json:"pageNum"`
	PageSize   int    `json:"pageSize"`
	FilterKey  string `json:"filterKey"`
	FilterText string `json:"filterText"`
}

type DBMSTableInsertPayload struct {
	DatabaseID uint           `json:"databaseId"`
	Schema     string         `json:"schema"`
	Table      string         `json:"table"`
	Row        map[string]any `json:"row"`
}

type DBMSTableUpdatePayload struct {
	DatabaseID uint           `json:"databaseId"`
	Schema     string         `json:"schema"`
	Table      string         `json:"table"`
	Original   map[string]any `json:"original"`
	Current    map[string]any `json:"current"`
}

type DBMSTableDeletePayload struct {
	DatabaseID uint           `json:"databaseId"`
	Schema     string         `json:"schema"`
	Table      string         `json:"table"`
	Row        map[string]any `json:"row"`
}

type DBMSImportPayload struct {
	SourceDatabaseID uint   `json:"sourceDatabaseId"`
	SourceSchema     string `json:"sourceSchema"`
	SourceTable      string `json:"sourceTable"`
	TargetDatabaseID uint   `json:"targetDatabaseId"`
	TargetSchema     string `json:"targetSchema"`
	TargetTable      string `json:"targetTable"`
	CreateIfMissing  bool   `json:"createIfMissing"`
	TruncateTarget   bool   `json:"truncateTarget"`
}

type DBMSExportPayload struct {
	DatabaseID  uint   `json:"databaseId"`
	Schema      string `json:"schema"`
	Table       string `json:"table"`
	IncludeData bool   `json:"includeData"`
}

type DBMSBatchSQLPayload struct {
	DatabaseID    uint   `json:"databaseId"`
	Schema        string `json:"schema"`
	SQLText       string `json:"sqlText"`
	FileName      string `json:"fileName"`
	ExecutionMode string `json:"executionMode"`
	Confirmed     bool   `json:"confirmed"`
	Operator      string `json:"-"`
	ClientIP      string `json:"-"`
}

type DBMSImportPrecheck struct {
	SourceDatabase string   `json:"sourceDatabase"`
	SourceSchema   string   `json:"sourceSchema"`
	SourceTable    string   `json:"sourceTable"`
	TargetDatabase string   `json:"targetDatabase"`
	TargetSchema   string   `json:"targetSchema"`
	TargetTable    string   `json:"targetTable"`
	EstimatedRows  int64    `json:"estimatedRows"`
	TargetExists   bool     `json:"targetExists"`
	CommonColumns  []string `json:"commonColumns"`
	MissingColumns []string `json:"missingColumns"`
	Warnings       []string `json:"warnings"`
	Ready          bool     `json:"ready"`
}

type DBMSSQLAnalysis struct {
	SQLType        string   `json:"sqlType"`
	StatementCount int      `json:"statementCount"`
	WriteOperation bool     `json:"writeOperation"`
	RiskLevel      string   `json:"riskLevel"`
	Reasons        []string `json:"reasons"`
	DatabaseName   string   `json:"databaseName"`
	Schema         string   `json:"schema"`
	Environment    string   `json:"environment"`
	AccessMode     string   `json:"accessMode"`
}

type databaseTableColumn struct {
	Name          string `json:"name"`
	DataType      string `json:"dataType"`
	ColumnType    string `json:"columnType"`
	ColumnKey     string `json:"columnKey"`
	IsNullable    string `json:"isNullable"`
	ColumnDefault any    `json:"columnDefault"`
	Extra         string `json:"extra"`
}

func normalizeDatabaseType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "postgres", "postgresql":
		return "postgresql"
	case "mongo", "mongodb":
		return "mongodb"
	case "redis":
		return "redis"
	default:
		return "mysql"
	}
}

func databasePort(v int) int {
	return databasePortByType("mysql", v)
}

func databaseCharset(v string) string {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" {
		return "utf8mb4"
	}
	if strings.HasPrefix(value, "utf8mb4_") {
		return "utf8mb4"
	}
	if strings.HasPrefix(value, "utf8_") {
		return "utf8"
	}
	return value
}

func normalizeDatabaseAccessMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "readonly") {
		return "readonly"
	}
	return "readwrite"
}

func ensureDatabaseWritable(item *model.AssetDatabase) error {
	if item != nil && normalizeDatabaseAccessMode(item.AccessMode) == "readonly" {
		return errors.New("当前数据库为只读模式，禁止新增、编辑、删除或导入数据")
	}
	return nil
}

func databaseColumnsHavePrimaryKey(columns []databaseTableColumn) bool {
	for _, column := range columns {
		if strings.EqualFold(column.ColumnKey, "PRI") {
			return true
		}
	}
	return false
}

func mysqlDSN(host string, port int, user, password, dbName, charset string) string {
	schema := strings.TrimSpace(dbName)
	if schema == "" {
		schema = "mysql"
	}
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&timeout=5s&readTimeout=20s&writeTimeout=20s&multiStatements=true&loc=Local",
		user,
		password,
		host,
		port,
		schema,
		charset,
	)
}

func inspectMySQLDatabase(host string, port int, user, password, dbName, charset string) (string, error) {
	item := model.AssetDatabase{
		Host:     host,
		Port:     port,
		Username: user,
		Password: password,
		DBName:   dbName,
		Charset:  charset,
	}
	db, cleanup, err := openMySQLDatabase(item, "")
	if err != nil {
		return "", err
	}
	defer cleanup()
	defer db.Close()
	if err := db.Ping(); err != nil {
		return "", err
	}
	var version string
	if err := db.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

func (s *Service) inspectAssetMySQLDatabase(item model.AssetDatabase) (string, error) {
	db, cleanup, err := s.openAssetMySQLDatabase(item, "")
	if err != nil {
		return "", err
	}
	defer cleanup()
	defer db.Close()
	if err := db.Ping(); err != nil {
		return "", err
	}
	var version string
	if err := db.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

func openMySQLDatabase(item model.AssetDatabase, schema string) (*sql.DB, func(), error) {
	db, err := sql.Open("mysql", mysqlDSN(item.Host, databasePort(item.Port), item.Username, item.Password, defaultSchema(&item, schema), databaseCharset(item.Charset)))
	return db, func() {}, err
}

func (s *Service) openAssetMySQLDatabase(item model.AssetDatabase, schema string) (*sql.DB, func(), error) {
	targetHost := strings.TrimSpace(item.Host)
	targetPort := databasePort(item.Port)
	cleanup := func() {}
	if normalizeConnectionMode(item.ConnectionMode) == "gateway" && item.GatewayID != nil && *item.GatewayID > 0 {
		localAddress, tunnelCleanup, err := s.startGatewayTunnel(*item.GatewayID, fmt.Sprintf("%s:%d", targetHost, targetPort))
		if err != nil {
			return nil, cleanup, err
		}
		cleanup = tunnelCleanup
		host, portText, err := net.SplitHostPort(localAddress)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		port, _ := strconv.Atoi(portText)
		item.Host = host
		item.Port = port
	}
	db, err := sql.Open("mysql", mysqlDSN(item.Host, databasePort(item.Port), item.Username, item.Password, defaultSchema(&item, schema), databaseCharset(item.Charset)))
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return db, cleanup, nil
}

func (s *Service) ListAssetDatabases(pageNum, pageSize int, keyword string, dbType string, status string, env string, tag string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.AssetDatabase{}).Preload("Gateway")
	if keyword != "" {
		query = query.Where("name like ? or host like ? or username like ? or db_name like ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if dbType != "" {
		query = query.Where("db_type = ?", normalizeDatabaseType(dbType))
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if env != "" {
		query = query.Where("env = ?", normalizeEnvCode(env))
	}
	if tag != "" {
		query = query.Where("tags LIKE ?", "%\""+strings.TrimSpace(tag)+"\"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AssetDatabase
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Password = ""
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetAssetDatabase(id uint) (*model.AssetDatabase, error) {
	item, err := s.getAssetDatabase(id)
	if err != nil {
		return nil, err
	}
	item.Password = ""
	return item, nil
}

func (s *Service) CreateAssetDatabase(payload AssetDatabasePayload) error {
	item := model.AssetDatabase{
		Name:           Trimmed(payload.Name),
		DBType:         normalizeDatabaseType(payload.DBType),
		Host:           Trimmed(payload.Host),
		Port:           databasePortByType(payload.DBType, payload.Port),
		Username:       Trimmed(payload.Username),
		Password:       payload.Password,
		ConnectionMode: normalizeConnectionMode(payload.ConnectionMode),
		GatewayID:      optionalGatewayID(payload.ConnectionMode, payload.GatewayID),
		DBName:         Trimmed(payload.DBName),
		Charset:        databaseCharsetByType(payload.DBType, payload.Charset),
		Env:            normalizeEnvCode(payload.Env),
		Tags:           normalizeAssetTags(payload.Tags),
		AccessMode:     normalizeDatabaseAccessMode(payload.AccessMode),
		MonitorEnabled: payload.MonitorEnabled,
		Status:         payload.Status,
		Description:    Trimmed(payload.Description),
	}
	if item.Name == "" {
		return errors.New("数据库名称不能为空")
	}
	if item.Host == "" {
		return errors.New("数据库地址不能为空")
	}
	if databaseRequiresUsername(item.DBType) && item.Username == "" {
		return errors.New("数据库用户名不能为空")
	}
	if item.Env == "" {
		return errors.New("请选择所属环境")
	}
	if err := validateGatewaySelection(item.ConnectionMode, item.GatewayID); err != nil {
		return err
	}
	if item.Status == 0 {
		item.Status = 1
	}
	now := time.Now()
	item.LastCheckTime = &now
	version, err := s.inspectAssetDatabase(item)
	if err == nil {
		item.Version = version
		item.ConnectStatus = 1
	} else {
		item.ConnectStatus = 2
	}
	if err := s.db.Create(&item).Error; err != nil {
		return err
	}
	s.recordAssetChange("database", item.ID, item.Name, "create", "新增数据库资产", payload.Operator)
	return nil
}

func (s *Service) UpdateAssetDatabase(payload AssetDatabasePayload) error {
	existing, err := s.getAssetDatabase(payload.ID)
	if err != nil {
		return err
	}
	password := payload.Password
	if password == "" {
		password = existing.Password
	}
	updates := map[string]any{
		"name":            Trimmed(payload.Name),
		"db_type":         normalizeDatabaseType(payload.DBType),
		"host":            Trimmed(payload.Host),
		"port":            databasePortByType(payload.DBType, payload.Port),
		"username":        Trimmed(payload.Username),
		"password":        password,
		"connection_mode": normalizeConnectionMode(payload.ConnectionMode),
		"gateway_id":      optionalGatewayID(payload.ConnectionMode, payload.GatewayID),
		"db_name":         Trimmed(payload.DBName),
		"charset":         databaseCharsetByType(payload.DBType, payload.Charset),
		"env":             normalizeEnvCode(payload.Env),
		"tags":            normalizeAssetTags(payload.Tags),
		"access_mode":     normalizeDatabaseAccessMode(payload.AccessMode),
		"monitor_enabled": payload.MonitorEnabled,
		"status":          payload.Status,
		"description":     Trimmed(payload.Description),
	}
	if Trimmed(payload.Name) == "" {
		return errors.New("数据库名称不能为空")
	}
	if Trimmed(payload.Host) == "" {
		return errors.New("数据库地址不能为空")
	}
	if databaseRequiresUsername(payload.DBType) && Trimmed(payload.Username) == "" {
		return errors.New("数据库用户名不能为空")
	}
	if normalizeEnvCode(payload.Env) == "" {
		return errors.New("请选择所属环境")
	}
	if err := validateGatewaySelection(normalizeConnectionMode(payload.ConnectionMode), optionalGatewayID(payload.ConnectionMode, payload.GatewayID)); err != nil {
		return err
	}
	if payload.Status == 0 {
		updates["status"] = 1
	}
	now := time.Now()
	updates["last_check_time"] = &now
	probeItem := *existing
	probeItem.Host = Trimmed(payload.Host)
	probeItem.DBType = normalizeDatabaseType(payload.DBType)
	probeItem.Port = databasePortByType(payload.DBType, payload.Port)
	probeItem.Username = Trimmed(payload.Username)
	probeItem.Password = password
	probeItem.ConnectionMode = normalizeConnectionMode(payload.ConnectionMode)
	probeItem.GatewayID = optionalGatewayID(payload.ConnectionMode, payload.GatewayID)
	probeItem.DBName = Trimmed(payload.DBName)
	probeItem.Charset = databaseCharsetByType(payload.DBType, payload.Charset)
	probeItem.Env = normalizeEnvCode(payload.Env)
	probeItem.AccessMode = normalizeDatabaseAccessMode(payload.AccessMode)
	version, err := s.inspectAssetDatabase(probeItem)
	if err == nil {
		updates["version"] = version
		updates["connect_status"] = 1
	} else {
		updates["version"] = ""
		updates["connect_status"] = 2
	}
	if err := s.db.Model(&model.AssetDatabase{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
		return err
	}
	s.recordAssetChange("database", payload.ID, payload.Name, "update", "更新数据库基础信息", payload.Operator)
	return nil
}

func (s *Service) DeleteAssetDatabase(id uint) error {
	var item model.AssetDatabase
	_ = s.db.First(&item, id).Error
	if err := s.db.Delete(&model.AssetDatabase{}, id).Error; err != nil {
		return err
	}
	s.recordAssetChange("database", id, item.Name, "delete", "删除数据库资产", "system")
	return nil
}

func (s *Service) TestAssetDatabaseConnection(payload AssetDatabasePayload) (map[string]any, error) {
	dbType := normalizeDatabaseType(payload.DBType)
	item := model.AssetDatabase{
		DBType:         dbType,
		Host:           Trimmed(payload.Host),
		Port:           databasePortByType(dbType, payload.Port),
		Username:       Trimmed(payload.Username),
		Password:       payload.Password,
		ConnectionMode: normalizeConnectionMode(payload.ConnectionMode),
		GatewayID:      optionalGatewayID(payload.ConnectionMode, payload.GatewayID),
		DBName:         Trimmed(payload.DBName),
		Charset:        databaseCharsetByType(dbType, payload.Charset),
		Env:            normalizeEnvCode(payload.Env),
		AccessMode:     normalizeDatabaseAccessMode(payload.AccessMode),
	}
	if err := validateGatewaySelection(item.ConnectionMode, item.GatewayID); err != nil {
		return nil, err
	}
	version, err := s.inspectAssetDatabase(item)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"dbType":        dbType,
		"version":       version,
		"connectStatus": 1,
		"checkedAt":     time.Now(),
	}, nil
}

func (s *Service) GetDatabaseWorkbench(databaseID uint) (map[string]any, error) {
	item, err := s.getAssetDatabase(databaseID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":            item.ID,
		"name":          item.Name,
		"dbType":        item.DBType,
		"host":          item.Host,
		"port":          item.Port,
		"username":      item.Username,
		"dbName":        item.DBName,
		"charset":       item.Charset,
		"version":       item.Version,
		"connectStatus": item.ConnectStatus,
		"accessMode":    item.AccessMode,
		"mode":          databaseMode(item.DBType),
		"capabilities":  databaseCapabilities(item.DBType),
	}, nil
}

func (s *Service) GetDatabaseSchemaTree(databaseID uint) (map[string]any, error) {
	item, err := s.getAssetDatabase(databaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(item.DBType) != "mysql" {
		return s.getNonMySQLSchemaTree(item)
	}
	item, db, cleanup, err := s.openDatabaseByID(databaseID, "")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	defer cleanup()

	schemas := make([]map[string]any, 0)
	rows, err := db.Query(`
		SELECT SCHEMA_NAME
		FROM INFORMATION_SCHEMA.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
		ORDER BY SCHEMA_NAME
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName string
		if err := rows.Scan(&schemaName); err != nil {
			return nil, err
		}
		tableRows, err := db.Query(`
			SELECT TABLE_NAME, TABLE_TYPE, TABLE_ROWS
			FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_SCHEMA = ?
			ORDER BY TABLE_NAME
		`, schemaName)
		if err != nil {
			return nil, err
		}
		tables := make([]map[string]any, 0)
		for tableRows.Next() {
			var tableName, tableType string
			var tableRowsCount sql.NullInt64
			if err := tableRows.Scan(&tableName, &tableType, &tableRowsCount); err != nil {
				tableRows.Close()
				return nil, err
			}
			tables = append(tables, map[string]any{
				"name":      tableName,
				"type":      tableType,
				"rows":      tableRowsCount.Int64,
				"schema":    schemaName,
				"fullName":  schemaName + "." + tableName,
				"isDefault": tableName == item.DBName,
			})
		}
		tableRows.Close()
		schemas = append(schemas, map[string]any{
			"name":       schemaName,
			"tableCount": len(tables),
			"tables":     tables,
			"isCurrent":  schemaName == item.DBName,
		})
	}
	return map[string]any{
		"schemas":       schemas,
		"defaultSchema": defaultSchema(item, ""),
	}, nil
}

func (s *Service) CreateDatabaseSchema(payload DBMSCreateDatabasePayload) (map[string]any, error) {
	name := strings.TrimSpace(payload.Name)
	if payload.DatabaseID == 0 || name == "" {
		return nil, errors.New("请输入数据库名称")
	}
	if !databaseObjectNamePattern.MatchString(name) {
		return nil, errors.New("名称须以字母或下划线开头，仅可包含字母、数字和下划线，长度不超过 63 个字符")
	}

	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	if err := ensureDatabaseWritable(item); err != nil {
		return nil, err
	}

	objectType := "database"
	sqlText := ""
	switch normalizeDatabaseType(item.DBType) {
	case "mysql":
		charset := strings.TrimSpace(payload.Charset)
		if charset == "" {
			charset = "utf8mb4"
		}
		collation := strings.TrimSpace(payload.Collation)
		if collation == "" {
			collation = "utf8mb4_unicode_ci"
		}
		if !mysqlCharsetOptionPattern.MatchString(charset) || !mysqlCharsetOptionPattern.MatchString(collation) {
			return nil, errors.New("字符集或排序规则格式不正确")
		}
		if !strings.HasPrefix(strings.ToLower(collation), strings.ToLower(charset)+"_") {
			return nil, errors.New("排序规则必须与所选字符集匹配")
		}
		sqlText = "CREATE DATABASE " + quoteIdentifier(name) + " CHARACTER SET " + charset + " COLLATE " + collation
	case "postgresql":
		objectType = "schema"
		sqlText = "CREATE SCHEMA " + postgresQuoteIdentifier(name)
	default:
		return nil, errors.New("当前数据库类型不支持新增数据库")
	}
	if _, err := db.Exec(sqlText); err != nil {
		return nil, err
	}

	summary := "SQL 工作台新增数据库：" + name
	if objectType == "schema" {
		summary = "SQL 工作台新增 PostgreSQL Schema：" + name
	}
	s.recordAssetChange("database", item.ID, item.Name, "create_schema", summary, payload.Operator)
	return map[string]any{"name": name, "objectType": objectType}, nil
}

func (s *Service) GetDatabaseCharsetOptions(databaseID uint, charset string) (map[string]any, error) {
	item, db, cleanup, err := s.openDatabaseByID(databaseID, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	if normalizeDatabaseType(item.DBType) != "mysql" {
		return nil, errors.New("当前数据库类型不支持字符集与排序规则设置")
	}

	charsetRows, err := db.Query(`
		SELECT CHARACTER_SET_NAME, DEFAULT_COLLATE_NAME
		FROM information_schema.CHARACTER_SETS
		ORDER BY CHARACTER_SET_NAME
	`)
	if err != nil {
		return nil, err
	}
	defer charsetRows.Close()
	charsets := make([]map[string]any, 0)
	for charsetRows.Next() {
		var name, defaultCollation string
		if err := charsetRows.Scan(&name, &defaultCollation); err != nil {
			return nil, err
		}
		charsets = append(charsets, map[string]any{"name": name, "defaultCollation": defaultCollation})
	}
	if err := charsetRows.Err(); err != nil {
		return nil, err
	}

	selectedCharset := strings.TrimSpace(charset)
	if selectedCharset == "" {
		selectedCharset = "utf8mb4"
	}
	collationRows, err := db.Query(`
		SELECT COLLATION_NAME, IS_DEFAULT
		FROM information_schema.COLLATIONS
		WHERE CHARACTER_SET_NAME = ?
		ORDER BY IS_DEFAULT DESC, COLLATION_NAME
	`, selectedCharset)
	if err != nil {
		return nil, err
	}
	defer collationRows.Close()
	collations := make([]map[string]any, 0)
	for collationRows.Next() {
		var name, isDefault string
		if err := collationRows.Scan(&name, &isDefault); err != nil {
			return nil, err
		}
		collations = append(collations, map[string]any{"name": name, "isDefault": strings.EqualFold(isDefault, "Yes")})
	}
	if err := collationRows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"charsets": charsets, "collations": collations}, nil
}

func (s *Service) GetDatabaseTableData(payload DBMSTableDataQueryPayload) (map[string]any, error) {
	if payload.DatabaseID == 0 || strings.TrimSpace(payload.Table) == "" {
		return nil, errors.New("请先选择数据表")
	}
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(asset.DBType) == "postgresql" {
		return s.getPostgresTableData(asset, payload)
	}
	if payload.DatabaseID == 0 || payload.Table == "" {
		return nil, errors.New("请先选择表")
	}
	if payload.PageNum < 1 {
		payload.PageNum = 1
	}
	if payload.PageSize < 1 {
		payload.PageSize = 25
	}
	schema := strings.TrimSpace(payload.Schema)
	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, schema)
	if err != nil {
		return nil, err
	}
	if err := ensureMySQLFeature(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	defer db.Close()
	defer cleanup()
	schema = defaultSchema(item, schema)

	columns, err := s.getTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}

	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdentifier(schema), quoteIdentifier(payload.Table))
	countArgs := make([]any, 0)
	if filterClause, filterArgs, err := buildTableFilterClause(columns, payload.FilterKey, payload.FilterText); err != nil {
		return nil, err
	} else if filterClause != "" {
		countSQL += " WHERE " + filterClause
		countArgs = append(countArgs, filterArgs...)
	}
	if err := db.QueryRow(countSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := fmt.Sprintf(
		"SELECT * FROM %s.%s LIMIT %d OFFSET %d",
		quoteIdentifier(schema),
		quoteIdentifier(payload.Table),
		payload.PageSize,
		(payload.PageNum-1)*payload.PageSize,
	)
	queryArgs := make([]any, 0)
	if filterClause, filterArgs, err := buildTableFilterClause(columns, payload.FilterKey, payload.FilterText); err != nil {
		return nil, err
	} else if filterClause != "" {
		querySQL = strings.Replace(querySQL, " LIMIT ", " WHERE "+filterClause+" LIMIT ", 1)
		queryArgs = append(queryArgs, filterArgs...)
	}
	rows, err := db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columnNames, dataRows, err := scanRows(rows)
	if err != nil {
		return nil, err
	}

	primaryKeys := make([]string, 0)
	for _, item := range columns {
		if strings.EqualFold(item.ColumnKey, "PRI") {
			primaryKeys = append(primaryKeys, item.Name)
		}
	}

	return map[string]any{
		"schema":      schema,
		"table":       payload.Table,
		"columns":     columns,
		"columnNames": columnNames,
		"rows":        dataRows,
		"primaryKeys": primaryKeys,
		"total":       total,
		"pageNum":     payload.PageNum,
		"pageSize":    payload.PageSize,
		"filterKey":   payload.FilterKey,
		"filterText":  payload.FilterText,
	}, nil
}

func (s *Service) ExecuteDatabaseSQL(payload DBMSSQLExecutePayload) (map[string]any, error) {
	sqlText := strings.TrimSpace(payload.SQLText)
	if payload.DatabaseID == 0 || sqlText == "" {
		return nil, errors.New("请输入 SQL")
	}
	analysis, err := s.AnalyzeDatabaseSQL(payload)
	if err != nil {
		return nil, err
	}
	if analysis.WriteOperation {
		if err := s.requireResource("database", payload.DatabaseID, "", "write"); err != nil {
			return nil, err
		}
	}
	if analysis.WriteOperation && analysis.AccessMode == "readonly" {
		return nil, errors.New("当前数据库为只读模式，禁止执行写入或结构变更 SQL")
	}
	if analysis.WriteOperation && !payload.Confirmed {
		return nil, errors.New("写操作必须完成执行前确认")
	}
	start := time.Now()
	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, payload.Schema)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	defer cleanup()

	trimmedSQL := strings.TrimSuffix(sqlText, ";")
	sqlType := analysis.SQLType
	history := model.DatabaseSQLHistory{
		DatabaseID:   item.ID,
		DatabaseName: item.Name,
		SchemaName:   defaultSchema(item, payload.Schema),
		SQLType:      sqlType,
		SQLText:      sqlText,
		ExecutionID:  newDBMSExecutionID(),
		Operator:     strings.TrimSpace(payload.Operator),
		ClientIP:     strings.TrimSpace(payload.ClientIP),
		Environment:  item.Env,
		AccessMode:   normalizeDatabaseAccessMode(item.AccessMode),
	}

	if isQuerySQL(sqlType) {
		rows, releaseQuery, err := s.queryDatabaseSQLRows(db, payload.DatabaseID, trimmedSQL)
		defer releaseQuery()
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			history.Status = 2
			history.DurationMs = durationMs
			history.ErrorMessage = err.Error()
			s.db.Create(&history)
			return nil, err
		}
		defer rows.Close()
		columnNames, dataRows, err := scanRows(rows)
		if err != nil {
			history.Status = 2
			history.DurationMs = durationMs
			history.ErrorMessage = err.Error()
			s.db.Create(&history)
			return nil, err
		}
		history.Status = 1
		history.DurationMs = durationMs
		history.RowsAffected = int64(len(dataRows))
		s.db.Create(&history)
		return map[string]any{
			"sqlType":      sqlType,
			"columns":      columnNames,
			"rows":         dataRows,
			"rowsAffected": len(dataRows),
			"durationMs":   durationMs,
			"historyId":    history.ID,
			"executionId":  history.ExecutionID,
			"analysis":     analysis,
		}, nil
	}

	result, err := db.Exec(trimmedSQL)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		history.Status = 2
		history.DurationMs = durationMs
		history.ErrorMessage = err.Error()
		s.db.Create(&history)
		return nil, err
	}
	rowsAffected, _ := result.RowsAffected()
	history.Status = 1
	history.DurationMs = durationMs
	history.RowsAffected = rowsAffected
	s.db.Create(&history)
	return map[string]any{
		"sqlType":      sqlType,
		"rowsAffected": rowsAffected,
		"durationMs":   durationMs,
		"historyId":    history.ID,
		"executionId":  history.ExecutionID,
		"analysis":     analysis,
	}, nil
}

func (s *Service) AnalyzeDatabaseSQL(payload DBMSSQLExecutePayload) (*DBMSSQLAnalysis, error) {
	if payload.DatabaseID == 0 || strings.TrimSpace(payload.SQLText) == "" {
		return nil, errors.New("请输入 SQL")
	}
	item, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	statements := splitDBMSSQLStatements(payload.SQLText)
	if len(statements) == 0 {
		return nil, errors.New("请输入有效 SQL")
	}
	sqlType := detectSQLType(statements[0])
	writeOperation := false
	riskLevel := "low"
	reasons := make([]string, 0)
	for _, statement := range statements {
		statementType := detectSQLType(statement)
		if !isReadOnlySQL(statement) {
			writeOperation = true
		}
		switch statementType {
		case "DROP", "TRUNCATE", "ALTER", "GRANT", "REVOKE", "RENAME":
			riskLevel = "high"
			reasons = append(reasons, statementType+" 属于高风险结构或权限变更")
		case "UPDATE", "DELETE":
			if !regexp.MustCompile(`(?i)\bWHERE\b`).MatchString(stripDBMSSQLComments(statement)) {
				riskLevel = "high"
				reasons = append(reasons, statementType+" 未包含 WHERE 条件")
			} else if riskLevel != "high" {
				riskLevel = "medium"
			}
		case "INSERT", "REPLACE", "CREATE":
			if riskLevel != "high" {
				riskLevel = "medium"
			}
		default:
			if !isReadOnlySQL(statement) {
				riskLevel = "high"
				reasons = append(reasons, "包含无法确认为只读的 SQL 语句")
			}
		}
	}
	if len(statements) > 1 {
		reasons = append(reasons, fmt.Sprintf("将连续执行 %d 条 SQL", len(statements)))
		if writeOperation && riskLevel == "low" {
			riskLevel = "medium"
		}
	}
	if writeOperation && normalizeEnvCode(item.Env) == "prod" {
		riskLevel = "high"
		reasons = append(reasons, "目标数据库属于生产环境")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "未发现明显高风险特征")
	}
	return &DBMSSQLAnalysis{
		SQLType:        sqlType,
		StatementCount: len(statements),
		WriteOperation: writeOperation,
		RiskLevel:      riskLevel,
		Reasons:        reasons,
		DatabaseName:   item.Name,
		Schema:         defaultSchema(item, payload.Schema),
		Environment:    item.Env,
		AccessMode:     normalizeDatabaseAccessMode(item.AccessMode),
	}, nil
}

func (s *Service) ListDatabaseSQLHistory(databaseID uint, pageNum, pageSize int) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	query := s.db.Model(&model.DatabaseSQLHistory{}).Where("database_id = ?", databaseID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.DatabaseSQLHistory
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) InsertDatabaseTableRow(payload DBMSTableInsertPayload) (map[string]any, error) {
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(asset.DBType) == "postgresql" {
		return s.insertPostgresTableRow(asset, payload)
	}
	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, payload.Schema)
	if err != nil {
		return nil, err
	}
	if err := ensureMySQLFeature(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	if err := ensureDatabaseWritable(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	defer db.Close()
	defer cleanup()
	schema := defaultSchema(item, payload.Schema)
	columns, err := s.getTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}

	insertColumns := make([]string, 0)
	values := make([]any, 0)
	for _, col := range columns {
		if _, exists := payload.Row[col.Name]; exists {
			insertColumns = append(insertColumns, col.Name)
			values = append(values, normalizeJSONValue(payload.Row[col.Name]))
		}
	}
	if len(insertColumns) == 0 {
		return nil, errors.New("没有可插入的数据")
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(insertColumns)), ",")
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s.%s (%s) VALUES (%s)",
		quoteIdentifier(schema),
		quoteIdentifier(payload.Table),
		joinIdentifiers(insertColumns),
		placeholders,
	)
	result, err := db.Exec(insertSQL, values...)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	insertedRow := make(map[string]any, len(payload.Row))
	for key, value := range payload.Row {
		insertedRow[key] = value
	}
	if lastInsertID, err := result.LastInsertId(); err == nil {
		for _, col := range columns {
			if strings.EqualFold(col.ColumnKey, "PRI") && strings.Contains(strings.ToLower(col.Extra), "auto_increment") {
				if _, exists := insertedRow[col.Name]; !exists || insertedRow[col.Name] == nil || insertedRow[col.Name] == "" {
					insertedRow[col.Name] = lastInsertID
				}
			}
		}
	}
	rollbackSQL := s.buildDeleteRollbackSQL(schema, payload.Table, columns, insertedRow)
	s.logDBSQLHistory(item, schema, payload.Table, "INSERT", insertSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) UpdateDatabaseTableRow(payload DBMSTableUpdatePayload) (map[string]any, error) {
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(asset.DBType) == "postgresql" {
		return s.updatePostgresTableRow(asset, payload)
	}
	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, payload.Schema)
	if err != nil {
		return nil, err
	}
	if err := ensureMySQLFeature(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	if err := ensureDatabaseWritable(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	defer db.Close()
	defer cleanup()
	schema := defaultSchema(item, payload.Schema)
	columns, err := s.getTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	if !databaseColumnsHavePrimaryKey(columns) {
		return nil, errors.New("当前表没有主键，禁止直接编辑结果集")
	}

	setParts := make([]string, 0)
	args := make([]any, 0)
	for _, col := range columns {
		newVal, exists := payload.Current[col.Name]
		if !exists {
			continue
		}
		oldVal := payload.Original[col.Name]
		if fmt.Sprintf("%v", normalizeJSONValue(newVal)) == fmt.Sprintf("%v", normalizeJSONValue(oldVal)) {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = ?", quoteIdentifier(col.Name)))
		args = append(args, normalizeJSONValue(newVal))
	}
	if len(setParts) == 0 {
		return map[string]any{"rowsAffected": 0, "rollbackSql": ""}, nil
	}

	whereSQL, whereArgs := buildRowWhereClause(columns, payload.Original)
	updateSQL := fmt.Sprintf(
		"UPDATE %s.%s SET %s WHERE %s",
		quoteIdentifier(schema),
		quoteIdentifier(payload.Table),
		strings.Join(setParts, ", "),
		whereSQL,
	)
	args = append(args, whereArgs...)
	result, err := db.Exec(updateSQL, args...)
	if err != nil {
		return nil, err
	}
	rollbackSQL := s.buildUpdateRollbackSQL(schema, payload.Table, columns, payload.Original)
	affected, _ := result.RowsAffected()
	s.logDBSQLHistory(item, schema, payload.Table, "UPDATE", updateSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) DeleteDatabaseTableRow(payload DBMSTableDeletePayload) (map[string]any, error) {
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(asset.DBType) == "postgresql" {
		return s.deletePostgresTableRow(asset, payload)
	}
	item, db, cleanup, err := s.openDatabaseByID(payload.DatabaseID, payload.Schema)
	if err != nil {
		return nil, err
	}
	if err := ensureMySQLFeature(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	if err := ensureDatabaseWritable(item); err != nil {
		db.Close()
		cleanup()
		return nil, err
	}
	defer db.Close()
	defer cleanup()
	schema := defaultSchema(item, payload.Schema)
	columns, err := s.getTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	if !databaseColumnsHavePrimaryKey(columns) {
		return nil, errors.New("当前表没有主键，禁止删除结果集数据")
	}
	whereSQL, args := buildRowWhereClause(columns, payload.Row)
	deleteSQL := fmt.Sprintf(
		"DELETE FROM %s.%s WHERE %s",
		quoteIdentifier(schema),
		quoteIdentifier(payload.Table),
		whereSQL,
	)
	result, err := db.Exec(deleteSQL, args...)
	if err != nil {
		return nil, err
	}
	rollbackSQL := s.buildInsertRollbackSQL(schema, payload.Table, columns, payload.Row)
	affected, _ := result.RowsAffected()
	s.logDBSQLHistory(item, schema, payload.Table, "DELETE", deleteSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) ExportDatabaseTable(databaseID uint, schema string, table string, includeData bool) ([]byte, string, error) {
	if databaseID == 0 || strings.TrimSpace(table) == "" {
		return nil, "", errors.New("请先选择表")
	}
	item, db, cleanup, err := s.openDatabaseByID(databaseID, schema)
	if err != nil {
		return nil, "", err
	}
	if err := ensureMySQLFeature(item); err != nil {
		db.Close()
		cleanup()
		return nil, "", err
	}
	defer db.Close()
	defer cleanup()
	schema = defaultSchema(item, schema)

	showSQL := fmt.Sprintf("SHOW CREATE TABLE %s.%s", quoteIdentifier(schema), quoteIdentifier(table))
	showRows, err := db.Query(showSQL)
	if err != nil {
		return nil, "", err
	}
	defer showRows.Close()
	_, dataRows, err := scanRows(showRows)
	if err != nil || len(dataRows) == 0 {
		return nil, "", errors.New("获取建表语句失败")
	}
	createSQL := fmt.Sprintf("%v", dataRows[0]["Create Table"])
	builder := &strings.Builder{}
	builder.WriteString("-- Ops Admin DBMS Export\n")
	builder.WriteString("-- Generated at " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")
	builder.WriteString("DROP TABLE IF EXISTS " + quoteIdentifier(table) + ";\n")
	builder.WriteString(createSQL + ";\n\n")

	if includeData {
		dataSQL := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdentifier(schema), quoteIdentifier(table))
		rows, err := db.Query(dataSQL)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		columnNames, list, err := scanRows(rows)
		if err != nil {
			return nil, "", err
		}
		for _, row := range list {
			values := make([]string, 0, len(columnNames))
			for _, name := range columnNames {
				values = append(values, sqlLiteral(row[name]))
			}
			builder.WriteString(fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s);\n",
				quoteIdentifier(table),
				joinIdentifiers(columnNames),
				strings.Join(values, ", "),
			))
		}
	}
	filename := fmt.Sprintf("%s_%s.sql", schema, table)
	return []byte(builder.String()), filename, nil
}

func (s *Service) ImportDatabaseTable(payload DBMSImportPayload) (map[string]any, error) {
	if payload.SourceDatabaseID == 0 || payload.TargetDatabaseID == 0 {
		return nil, errors.New("请选择源数据库和目标数据库")
	}
	if strings.TrimSpace(payload.SourceTable) == "" {
		return nil, errors.New("请选择源表")
	}
	sourceAsset, sourceDB, sourceCleanup, err := s.withResourceOperation("database", "export").openDatabaseByID(payload.SourceDatabaseID, payload.SourceSchema)
	if err != nil {
		return nil, err
	}
	if err := ensureRelationalImportFeature(sourceAsset); err != nil {
		sourceDB.Close()
		sourceCleanup()
		return nil, err
	}
	defer sourceDB.Close()
	defer sourceCleanup()

	targetAsset, targetDB, targetCleanup, err := s.openDatabaseByID(payload.TargetDatabaseID, payload.TargetSchema)
	if err != nil {
		return nil, err
	}
	if err := ensureRelationalImportFeature(targetAsset); err != nil {
		targetDB.Close()
		targetCleanup()
		return nil, err
	}
	defer targetDB.Close()
	defer targetCleanup()
	if err := ensureDatabaseWritable(targetAsset); err != nil {
		return nil, err
	}

	sourceSchema := defaultSchema(sourceAsset, payload.SourceSchema)
	targetSchema := defaultSchema(targetAsset, payload.TargetSchema)
	targetTable := strings.TrimSpace(payload.TargetTable)
	if targetTable == "" {
		targetTable = strings.TrimSpace(payload.SourceTable)
	}

	if payload.CreateIfMissing {
		sourceType := normalizeDatabaseType(sourceAsset.DBType)
		targetType := normalizeDatabaseType(targetAsset.DBType)
		if sourceType != targetType {
			return nil, errors.New("跨数据库类型导入暂不支持自动建表，请先创建目标表后再导入")
		}
		if sourceType == "postgresql" {
			if err := s.createPostgresImportTable(sourceDB, targetDB, sourceSchema, payload.SourceTable, targetSchema, targetTable); err != nil {
				return nil, err
			}
		} else {
			showSQL := fmt.Sprintf("SHOW CREATE TABLE %s", importTableName(sourceAsset, sourceSchema, payload.SourceTable))
			rows, err := sourceDB.Query(showSQL)
			if err != nil {
				return nil, err
			}
			_, list, err := scanRows(rows)
			rows.Close()
			if err != nil || len(list) == 0 {
				return nil, errors.New("获取源表结构失败")
			}
			createSQL := mysqlImportCreateTableSQL(fmt.Sprintf("%v", list[0]["Create Table"]), payload.SourceTable, targetTable)
			if _, err := targetDB.Exec(createSQL); err != nil {
				return nil, err
			}
		}
	}

	sourceColumns, err := s.getImportTableColumns(sourceAsset, sourceDB, sourceSchema, payload.SourceTable)
	if err != nil {
		return nil, err
	}
	targetColumns, err := s.getImportTableColumns(targetAsset, targetDB, targetSchema, targetTable)
	if err != nil {
		return nil, err
	}
	targetColumnSet := make(map[string]struct{}, len(targetColumns))
	for _, col := range targetColumns {
		targetColumnSet[col.Name] = struct{}{}
	}

	commonColumns := make([]string, 0)
	for _, col := range sourceColumns {
		if _, ok := targetColumnSet[col.Name]; ok {
			commonColumns = append(commonColumns, col.Name)
		}
	}
	if len(commonColumns) == 0 {
		return nil, errors.New("源表和目标表没有可匹配的字段")
	}

	if payload.TruncateTarget {
		if _, err := targetDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s", importTableName(targetAsset, targetSchema, targetTable))); err != nil {
			return nil, err
		}
	}

	selectSQL := fmt.Sprintf(
		"SELECT %s FROM %s",
		importJoinIdentifiers(sourceAsset, commonColumns),
		importTableName(sourceAsset, sourceSchema, payload.SourceTable),
	)
	rows, err := sourceDB.Query(selectSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	_, rowList, err := scanRows(rows)
	if err != nil {
		return nil, err
	}

	tx, err := targetDB.Begin()
	if err != nil {
		return nil, err
	}
	placeholders := importPlaceholders(targetAsset, len(commonColumns))
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		importTableName(targetAsset, targetSchema, targetTable),
		importJoinIdentifiers(targetAsset, commonColumns),
		placeholders,
	)
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	defer stmt.Close()

	var imported int64
	for _, row := range rowList {
		args := make([]any, 0, len(commonColumns))
		for _, col := range commonColumns {
			args = append(args, row[col])
		}
		if _, err := stmt.Exec(args...); err != nil {
			tx.Rollback()
			return nil, err
		}
		imported++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.logDBSQLHistory(targetAsset, targetSchema, targetTable, "IMPORT", insertSQL, 1, imported, 0, "", "")
	return map[string]any{"imported": imported, "targetTable": targetTable}, nil
}

func (s *Service) CreateExportTask(payload DBMSExportPayload) (map[string]any, error) {
	if payload.DatabaseID == 0 || strings.TrimSpace(payload.Table) == "" {
		return nil, errors.New("请先选择要导出的表")
	}
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	task := model.DatabaseTransferTask{
		TaskType:     "export",
		Status:       "pending",
		Progress:     0,
		Message:      "等待执行",
		DatabaseID:   asset.ID,
		DatabaseName: asset.Name,
		SchemaName:   defaultSchema(asset, payload.Schema),
		PrimaryTable: strings.TrimSpace(payload.Table),
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	go s.runExportTask(task.ID, payload)
	return map[string]any{"taskId": task.ID}, nil
}

func (s *Service) CreateImportTask(payload DBMSImportPayload) (map[string]any, error) {
	if payload.SourceDatabaseID == 0 || payload.TargetDatabaseID == 0 {
		return nil, errors.New("请选择源数据库和目标数据库")
	}
	precheck, err := s.PrecheckImportTask(payload)
	if err != nil {
		return nil, err
	}
	if !precheck.Ready {
		return nil, errors.New("导入预检查未通过，请处理风险项后重试")
	}
	sourceAsset, err := s.withResourceOperation("database", "export").getAssetDatabase(payload.SourceDatabaseID)
	if err != nil {
		return nil, err
	}
	targetAsset, err := s.getAssetDatabase(payload.TargetDatabaseID)
	if err != nil {
		return nil, err
	}
	if err := ensureDatabaseWritable(targetAsset); err != nil {
		return nil, err
	}
	task := model.DatabaseTransferTask{
		TaskType:         "import",
		Status:           "pending",
		Progress:         0,
		Message:          "等待执行",
		SourceDatabaseID: sourceAsset.ID,
		SourceDatabase:   sourceAsset.Name,
		SourceSchema:     defaultSchema(sourceAsset, payload.SourceSchema),
		SourceTable:      strings.TrimSpace(payload.SourceTable),
		TargetDatabaseID: targetAsset.ID,
		TargetDatabase:   targetAsset.Name,
		TargetSchema:     defaultSchema(targetAsset, payload.TargetSchema),
		TargetTable:      strings.TrimSpace(payload.TargetTable),
		DatabaseID:       targetAsset.ID,
		DatabaseName:     targetAsset.Name,
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	go s.runImportTask(task.ID, payload)
	return map[string]any{"taskId": task.ID}, nil
}

func (s *Service) PrecheckImportTask(payload DBMSImportPayload) (*DBMSImportPrecheck, error) {
	if payload.SourceDatabaseID == 0 || payload.TargetDatabaseID == 0 || strings.TrimSpace(payload.SourceTable) == "" {
		return nil, errors.New("请选择源数据库、源表和目标数据库")
	}
	sourceAsset, sourceDB, sourceCleanup, err := s.withResourceOperation("database", "export").openDatabaseByID(payload.SourceDatabaseID, payload.SourceSchema)
	if err != nil {
		return nil, err
	}
	if err := ensureRelationalImportFeature(sourceAsset); err != nil {
		sourceDB.Close()
		sourceCleanup()
		return nil, err
	}
	defer sourceDB.Close()
	defer sourceCleanup()
	targetAsset, targetDB, targetCleanup, err := s.openDatabaseByID(payload.TargetDatabaseID, payload.TargetSchema)
	if err != nil {
		return nil, err
	}
	if err := ensureRelationalImportFeature(targetAsset); err != nil {
		targetDB.Close()
		targetCleanup()
		return nil, err
	}
	defer targetDB.Close()
	defer targetCleanup()

	sourceSchema := defaultSchema(sourceAsset, payload.SourceSchema)
	targetSchema := defaultSchema(targetAsset, payload.TargetSchema)
	targetTable := strings.TrimSpace(payload.TargetTable)
	if targetTable == "" {
		targetTable = strings.TrimSpace(payload.SourceTable)
	}
	sourceColumns, err := s.getImportTableColumns(sourceAsset, sourceDB, sourceSchema, payload.SourceTable)
	if err != nil {
		return nil, err
	}
	if len(sourceColumns) == 0 {
		return nil, errors.New("源表不存在或没有可导入字段")
	}
	targetColumns, err := s.getImportTableColumns(targetAsset, targetDB, targetSchema, targetTable)
	if err != nil {
		return nil, err
	}
	var estimatedRows int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s", importTableName(sourceAsset, sourceSchema, payload.SourceTable))
	if err := sourceDB.QueryRow(countSQL).Scan(&estimatedRows); err != nil {
		return nil, err
	}

	targetNames := make(map[string]struct{}, len(targetColumns))
	for _, column := range targetColumns {
		targetNames[column.Name] = struct{}{}
	}
	commonColumns := make([]string, 0)
	missingColumns := make([]string, 0)
	for _, column := range sourceColumns {
		if _, exists := targetNames[column.Name]; exists || len(targetColumns) == 0 && payload.CreateIfMissing {
			commonColumns = append(commonColumns, column.Name)
		} else {
			missingColumns = append(missingColumns, column.Name)
		}
	}
	warnings := make([]string, 0)
	if normalizeDatabaseAccessMode(targetAsset.AccessMode) == "readonly" {
		warnings = append(warnings, "目标数据库为只读模式")
	}
	if len(targetColumns) == 0 && !payload.CreateIfMissing {
		warnings = append(warnings, "目标表不存在，且未启用自动建表")
	}
	if normalizeDatabaseType(sourceAsset.DBType) != normalizeDatabaseType(targetAsset.DBType) && payload.CreateIfMissing {
		warnings = append(warnings, "跨数据库类型导入不支持自动建表，请先创建目标表")
	}
	if payload.TruncateTarget {
		warnings = append(warnings, "导入前将清空目标表全部数据")
	}
	if len(missingColumns) > 0 {
		warnings = append(warnings, fmt.Sprintf("有 %d 个源字段无法映射到目标表", len(missingColumns)))
	}
	ready := normalizeDatabaseAccessMode(targetAsset.AccessMode) != "readonly" &&
		(len(targetColumns) > 0 || (payload.CreateIfMissing && normalizeDatabaseType(sourceAsset.DBType) == normalizeDatabaseType(targetAsset.DBType))) &&
		len(commonColumns) > 0
	return &DBMSImportPrecheck{
		SourceDatabase: sourceAsset.Name,
		SourceSchema:   sourceSchema,
		SourceTable:    payload.SourceTable,
		TargetDatabase: targetAsset.Name,
		TargetSchema:   targetSchema,
		TargetTable:    targetTable,
		EstimatedRows:  estimatedRows,
		TargetExists:   len(targetColumns) > 0,
		CommonColumns:  commonColumns,
		MissingColumns: missingColumns,
		Warnings:       warnings,
		Ready:          ready,
	}, nil
}

func (s *Service) CreateBatchSQLTask(payload DBMSBatchSQLPayload) (map[string]any, error) {
	if payload.DatabaseID == 0 || strings.TrimSpace(payload.SQLText) == "" {
		return nil, errors.New("请选择数据库并提供 SQL 内容")
	}
	analysis, err := s.AnalyzeDatabaseSQL(DBMSSQLExecutePayload{
		DatabaseID: payload.DatabaseID,
		Schema:     payload.Schema,
		SQLText:    payload.SQLText,
	})
	if err != nil {
		return nil, err
	}
	if analysis.WriteOperation && analysis.AccessMode == "readonly" {
		return nil, errors.New("当前数据库为只读模式，禁止执行批量 SQL")
	}
	if analysis.WriteOperation && !payload.Confirmed {
		return nil, errors.New("批量 SQL 写操作必须完成执行前确认")
	}
	executionMode := strings.ToLower(strings.TrimSpace(payload.ExecutionMode))
	if executionMode != "transaction" {
		executionMode = "sequential"
	}
	if executionMode == "transaction" {
		for _, statement := range splitDBMSSQLStatements(payload.SQLText) {
			switch detectSQLType(statement) {
			case "CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME", "GRANT", "REVOKE":
				return nil, errors.New("事务执行不支持 DDL 或权限语句，请改用顺序执行")
			}
		}
	}
	asset, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	fileName := strings.TrimSpace(payload.FileName)
	if fileName == "" {
		fileName = "batch.sql"
	}
	task := model.DatabaseTransferTask{
		TaskType:      "batch_sql",
		Status:        "pending",
		Progress:      0,
		Message:       "等待执行",
		DatabaseID:    asset.ID,
		DatabaseName:  asset.Name,
		SchemaName:    defaultSchema(asset, payload.Schema),
		FileName:      fileName,
		FileContent:   payload.SQLText,
		ExecutionMode: executionMode,
		Operator:      payload.Operator,
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	go s.runBatchSQLTask(task.ID, payload.ClientIP)
	return map[string]any{"taskId": task.ID, "analysis": analysis}, nil
}

func (s *Service) runBatchSQLTask(taskID uint, clientIP string) {
	var task model.DatabaseTransferTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return
	}
	startedAt := time.Now()
	_ = s.db.Model(&task).Updates(map[string]any{"status": "running", "progress": 5, "message": "正在执行 SQL", "started_at": &startedAt}).Error
	statements := splitDBMSSQLStatements(task.FileContent)
	var rowsAffected int64
	var runErr error
	if task.ExecutionMode == "transaction" {
		item, db, cleanup, err := s.openDatabaseByID(task.DatabaseID, task.SchemaName)
		if err != nil {
			runErr = err
		} else {
			defer db.Close()
			defer cleanup()
			tx, err := db.Begin()
			if err != nil {
				runErr = err
			} else {
				for index, statement := range statements {
					result, err := tx.Exec(statement)
					if err != nil {
						runErr = fmt.Errorf("第 %d 条 SQL 执行失败: %w", index+1, err)
						_ = tx.Rollback()
						break
					}
					affected, _ := result.RowsAffected()
					rowsAffected += affected
					progress := 5 + int(float64(index+1)/float64(len(statements))*85)
					_ = s.db.Model(&task).Updates(map[string]any{"progress": progress, "message": fmt.Sprintf("已执行 %d/%d 条", index+1, len(statements))}).Error
				}
				if runErr == nil {
					runErr = tx.Commit()
				}
				status := 1
				errText := ""
				if runErr != nil {
					status = 2
					errText = runErr.Error()
				}
				s.logDBSQLHistory(item, task.SchemaName, "", "BATCH", task.FileContent, status, rowsAffected, time.Since(startedAt).Milliseconds(), errText, "")
			}
		}
	} else {
		for index, statement := range statements {
			result, err := s.ExecuteDatabaseSQL(DBMSSQLExecutePayload{
				DatabaseID: task.DatabaseID,
				Schema:     task.SchemaName,
				SQLText:    statement,
				Confirmed:  true,
				Operator:   task.Operator,
				ClientIP:   clientIP,
			})
			if err != nil {
				runErr = fmt.Errorf("第 %d 条 SQL 执行失败: %w", index+1, err)
				break
			}
			switch value := result["rowsAffected"].(type) {
			case int64:
				rowsAffected += value
			case int:
				rowsAffected += int64(value)
			}
			progress := 5 + int(float64(index+1)/float64(len(statements))*85)
			_ = s.db.Model(&task).Updates(map[string]any{"progress": progress, "message": fmt.Sprintf("已执行 %d/%d 条", index+1, len(statements))}).Error
		}
	}
	finishedAt := time.Now()
	updates := map[string]any{"finished_at": &finishedAt, "rows_affected": rowsAffected}
	if runErr != nil {
		updates["status"] = "failed"
		updates["progress"] = 100
		updates["message"] = runErr.Error()
	} else {
		updates["status"] = "success"
		updates["progress"] = 100
		updates["message"] = fmt.Sprintf("执行完成，共 %d 条 SQL", len(statements))
	}
	_ = s.db.Model(&task).Updates(updates).Error
}

func (s *Service) ListTransferTasks(databaseID uint, taskType string, pageNum, pageSize int) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	query := s.db.Model(&model.DatabaseTransferTask{})
	if databaseID > 0 {
		query = query.Where("database_id = ? OR target_database_id = ? OR source_database_id = ?", databaseID, databaseID, databaseID)
	}
	if strings.TrimSpace(taskType) != "" {
		taskTypes := strings.Split(strings.TrimSpace(taskType), ",")
		query = query.Where("task_type IN ?", taskTypes)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.DatabaseTransferTask
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"list":     list,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}, nil
}

func (s *Service) GetTransferTaskFile(taskID uint) ([]byte, string, error) {
	var task model.DatabaseTransferTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(task.FileContent) == "" {
		return nil, "", errors.New("当前任务没有可下载文件")
	}
	filename := strings.TrimSpace(task.FileName)
	if filename == "" {
		filename = fmt.Sprintf("dbms-task-%d.sql", task.ID)
	}
	return []byte(task.FileContent), filename, nil
}

func (s *Service) getAssetDatabase(id uint) (*model.AssetDatabase, error) {
	if err := s.requireResource("database", id, "", s.resourceAction("database", "query")); err != nil {
		return nil, err
	}
	var item model.AssetDatabase
	if err := s.db.Preload("Gateway").Preload("Gateway.Credential").First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) openDatabaseByID(id uint, schema string) (*model.AssetDatabase, *sql.DB, func(), error) {
	item, err := s.getAssetDatabase(id)
	if err != nil {
		return nil, nil, func() {}, err
	}
	var db *sql.DB
	var cleanup func()
	switch normalizeDatabaseType(item.DBType) {
	case "mysql":
		db, cleanup, err = s.openAssetMySQLDatabase(*item, schema)
	case "postgresql":
		db, cleanup, err = s.openAssetPostgresDatabase(*item)
	default:
		return nil, nil, func() {}, errors.New("当前数据库类型不支持 SQL 工作台，可在左侧查看数据库结构与连接状态")
	}
	if err != nil {
		return nil, nil, cleanup, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		cleanup()
		return nil, nil, func() {}, err
	}
	return item, db, cleanup, nil
}

func defaultSchema(item *model.AssetDatabase, schema string) string {
	value := strings.TrimSpace(schema)
	if value != "" {
		return value
	}
	if normalizeDatabaseType(item.DBType) == "postgresql" {
		return "public"
	}
	if strings.TrimSpace(item.DBName) != "" {
		return strings.TrimSpace(item.DBName)
	}
	if normalizeDatabaseType(item.DBType) == "redis" {
		return "db0"
	}
	return "mysql"
}

func detectSQLType(sqlText string) string {
	fields := strings.Fields(stripDBMSSQLComments(sqlText))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

func isQuerySQL(sqlType string) bool {
	switch sqlType {
	case "SELECT", "SHOW", "DESC", "DESCRIBE", "EXPLAIN", "WITH":
		return true
	default:
		return false
	}
}

func stripDBMSSQLComments(sqlText string) string {
	lineComment := regexp.MustCompile(`(?m)--[^\r\n]*|#[^\r\n]*`)
	blockComment := regexp.MustCompile(`(?s)/\*.*?\*/`)
	return strings.TrimSpace(blockComment.ReplaceAllString(lineComment.ReplaceAllString(sqlText, " "), " "))
}

func splitDBMSSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var current strings.Builder
	var quote rune
	escaped := false
	for _, char := range sqlText {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != 0 {
			current.WriteRune(char)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(char)
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' || char == '`' {
			quote = char
			current.WriteRune(char)
			continue
		}
		if char == ';' {
			if statement := strings.TrimSpace(current.String()); stripDBMSSQLComments(statement) != "" {
				statements = append(statements, statement)
			}
			current.Reset()
			continue
		}
		current.WriteRune(char)
	}
	if statement := strings.TrimSpace(current.String()); stripDBMSSQLComments(statement) != "" {
		statements = append(statements, statement)
	}
	return statements
}

func isReadOnlySQL(sqlText string) bool {
	cleaned := strings.ToUpper(stripDBMSSQLComments(sqlText))
	sqlType := detectSQLType(cleaned)
	if sqlType == "WITH" {
		writePattern := regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|ALTER|DROP|TRUNCATE|GRANT|REVOKE|CALL)\b`)
		return !writePattern.MatchString(cleaned)
	}
	return isQuerySQL(sqlType)
}

func newDBMSExecutionID() string {
	now := time.Now()
	return fmt.Sprintf("SQL-%s-%06d", now.Format("20060102150405"), now.Nanosecond()/1000)
}

func scanRows(rows *sql.Rows) ([]string, []map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, nil, err
		}
		item := make(map[string]any, len(columns))
		for i, col := range columns {
			item[col] = normalizeScanValue(values[i])
		}
		result = append(result, item)
	}
	return columns, result, rows.Err()
}

func normalizeScanValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

func normalizeJSONValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case string:
		return v
	default:
		return v
	}
}

func buildTableFilterClause(columns []databaseTableColumn, filterKey, filterText string) (string, []any, error) {
	key := strings.TrimSpace(filterKey)
	text := strings.TrimSpace(filterText)
	if text == "" {
		return "", nil, nil
	}
	if key != "" {
		for _, col := range columns {
			if col.Name == key {
				return fmt.Sprintf("%s LIKE ?", quoteIdentifier(key)), []any{"%" + text + "%"}, nil
			}
		}
		return "", nil, errors.New("筛选字段不存在")
	}
	parts := make([]string, 0)
	args := make([]any, 0)
	for _, col := range columns {
		parts = append(parts, fmt.Sprintf("%s LIKE ?", quoteIdentifier(col.Name)))
		args = append(args, "%"+text+"%")
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args, nil
}

func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(strings.TrimSpace(name), "`", "``") + "`"
}

func joinIdentifiers(columns []string) string {
	items := make([]string, 0, len(columns))
	for _, col := range columns {
		items = append(items, quoteIdentifier(col))
	}
	return strings.Join(items, ", ")
}

func sqlLiteral(value any) string {
	if value == nil {
		return "NULL"
	}
	switch v := value.(type) {
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		text := fmt.Sprintf("%v", v)
		text = strings.ReplaceAll(text, "\\", "\\\\")
		text = strings.ReplaceAll(text, "'", "''")
		return "'" + text + "'"
	}
}

func buildRowWhereClause(columns []databaseTableColumn, row map[string]any) (string, []any) {
	primaryKeys := make([]string, 0)
	for _, col := range columns {
		if strings.EqualFold(col.ColumnKey, "PRI") {
			primaryKeys = append(primaryKeys, col.Name)
		}
	}
	keys := primaryKeys
	if len(keys) == 0 {
		keys = make([]string, 0)
		for _, col := range columns {
			if _, exists := row[col.Name]; exists {
				keys = append(keys, col.Name)
			}
		}
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		clauses = append(clauses, fmt.Sprintf("%s = ?", quoteIdentifier(key)))
		args = append(args, normalizeJSONValue(row[key]))
	}
	return strings.Join(clauses, " AND "), args
}

func (s *Service) buildDeleteRollbackSQL(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	whereSQL, args := buildRowWhereClause(columns, row)
	literals := make([]string, 0, len(args))
	for _, arg := range args {
		literals = append(literals, sqlLiteral(arg))
	}
	parts := strings.Split(whereSQL, "?")
	builder := &strings.Builder{}
	for index, part := range parts {
		builder.WriteString(part)
		if index < len(literals) {
			builder.WriteString(literals[index])
		}
	}
	return fmt.Sprintf("DELETE FROM %s.%s WHERE %s;", quoteIdentifier(schema), quoteIdentifier(table), builder.String())
}

func (s *Service) buildUpdateRollbackSQL(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	setParts := make([]string, 0)
	for _, col := range columns {
		if _, exists := row[col.Name]; exists {
			setParts = append(setParts, fmt.Sprintf("%s = %s", quoteIdentifier(col.Name), sqlLiteral(row[col.Name])))
		}
	}
	whereSQL, args := buildRowWhereClause(columns, row)
	literals := make([]string, 0, len(args))
	for _, arg := range args {
		literals = append(literals, sqlLiteral(arg))
	}
	parts := strings.Split(whereSQL, "?")
	builder := &strings.Builder{}
	for index, part := range parts {
		builder.WriteString(part)
		if index < len(literals) {
			builder.WriteString(literals[index])
		}
	}
	return fmt.Sprintf(
		"UPDATE %s.%s SET %s WHERE %s;",
		quoteIdentifier(schema),
		quoteIdentifier(table),
		strings.Join(setParts, ", "),
		builder.String(),
	)
}

func (s *Service) buildInsertRollbackSQL(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	insertColumns := make([]string, 0)
	values := make([]string, 0)
	for _, col := range columns {
		if _, exists := row[col.Name]; exists {
			insertColumns = append(insertColumns, col.Name)
			values = append(values, sqlLiteral(row[col.Name]))
		}
	}
	return fmt.Sprintf(
		"INSERT INTO %s.%s (%s) VALUES (%s);",
		quoteIdentifier(schema),
		quoteIdentifier(table),
		joinIdentifiers(insertColumns),
		strings.Join(values, ", "),
	)
}

func (s *Service) getTableColumns(db *sql.DB, schema, table string) ([]databaseTableColumn, error) {
	rows, err := db.Query(`
		SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, COLUMN_KEY, IS_NULLABLE, COLUMN_DEFAULT, EXTRA
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make([]databaseTableColumn, 0)
	for rows.Next() {
		var item databaseTableColumn
		if err := rows.Scan(&item.Name, &item.DataType, &item.ColumnType, &item.ColumnKey, &item.IsNullable, &item.ColumnDefault, &item.Extra); err != nil {
			return nil, err
		}
		columns = append(columns, item)
	}
	return columns, rows.Err()
}

func (s *Service) logDBSQLHistory(asset *model.AssetDatabase, schema, table, sqlType, sqlText string, status int, rowsAffected int64, durationMs int64, errMessage, rollbackSQL string) {
	rollbackConfidence := ""
	if strings.TrimSpace(rollbackSQL) != "" {
		rollbackConfidence = "high"
	}
	history := model.DatabaseSQLHistory{
		DatabaseID:         asset.ID,
		DatabaseName:       asset.Name,
		SchemaName:         schema,
		TargetTable:        table,
		SQLType:            sqlType,
		SQLText:            sqlText,
		ExecutionID:        newDBMSExecutionID(),
		Operator:           "系统操作",
		Environment:        asset.Env,
		AccessMode:         normalizeDatabaseAccessMode(asset.AccessMode),
		Status:             status,
		RowsAffected:       rowsAffected,
		DurationMs:         durationMs,
		ErrorMessage:       errMessage,
		RollbackSQL:        rollbackSQL,
		RollbackConfidence: rollbackConfidence,
	}
	_ = s.db.Create(&history).Error
}

func (s *Service) updateTransferTask(taskID uint, updates map[string]any) {
	_ = s.db.Model(&model.DatabaseTransferTask{}).Where("id = ?", taskID).Updates(updates).Error
}

func (s *Service) runExportTask(taskID uint, payload DBMSExportPayload) {
	startedAt := time.Now()
	s.updateTransferTask(taskID, map[string]any{
		"status":     "running",
		"progress":   10,
		"message":    "正在读取表结构",
		"started_at": &startedAt,
	})

	data, filename, err := s.ExportDatabaseTable(payload.DatabaseID, payload.Schema, payload.Table, payload.IncludeData)
	finishedAt := time.Now()
	if err != nil {
		s.updateTransferTask(taskID, map[string]any{
			"status":      "failed",
			"progress":    100,
			"message":     err.Error(),
			"finished_at": &finishedAt,
		})
		return
	}
	s.updateTransferTask(taskID, map[string]any{
		"status":       "success",
		"progress":     100,
		"message":      "导出完成",
		"file_name":    filename,
		"file_content": string(data),
		"finished_at":  &finishedAt,
	})
}

func (s *Service) runImportTask(taskID uint, payload DBMSImportPayload) {
	startedAt := time.Now()
	s.updateTransferTask(taskID, map[string]any{
		"status":     "running",
		"progress":   10,
		"message":    "准备导入任务",
		"started_at": &startedAt,
	})

	s.updateTransferTask(taskID, map[string]any{
		"progress": 35,
		"message":  "正在比对源表和目标表结构",
	})
	result, err := s.ImportDatabaseTable(payload)
	finishedAt := time.Now()
	if err != nil {
		s.updateTransferTask(taskID, map[string]any{
			"status":      "failed",
			"progress":    100,
			"message":     err.Error(),
			"finished_at": &finishedAt,
		})
		return
	}
	rowsAffected, _ := result["imported"].(int64)
	if rowsAffected == 0 {
		switch v := result["imported"].(type) {
		case int:
			rowsAffected = int64(v)
		case float64:
			rowsAffected = int64(v)
		}
	}
	s.updateTransferTask(taskID, map[string]any{
		"status":        "success",
		"progress":      100,
		"message":       "导入完成",
		"rows_affected": rowsAffected,
		"finished_at":   &finishedAt,
	})
}
