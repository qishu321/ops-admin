package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/model"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func databasePortByType(dbType string, value int) int {
	if value > 0 {
		return value
	}
	switch normalizeDatabaseType(dbType) {
	case "postgresql":
		return 5432
	case "mongodb":
		return 27017
	case "redis":
		return 6379
	default:
		return 3306
	}
}

func databaseCharsetByType(dbType, value string) string {
	if normalizeDatabaseType(dbType) != "mysql" {
		return strings.TrimSpace(value)
	}
	return databaseCharset(value)
}

func databaseRequiresUsername(dbType string) bool {
	switch normalizeDatabaseType(dbType) {
	case "mongodb", "redis":
		return false
	default:
		return true
	}
}

func databaseCapabilities(dbType string) map[string]bool {
	switch normalizeDatabaseType(dbType) {
	case "postgresql":
		return map[string]bool{"sql": true, "schema": true, "tableData": true, "resourceData": false, "rowEdit": true, "transfer": false, "import": true, "export": false, "backup": true}
	case "mongodb":
		return map[string]bool{"sql": false, "schema": true, "tableData": false, "resourceData": true, "rowEdit": false, "transfer": false, "backup": false}
	case "redis":
		return map[string]bool{"sql": false, "schema": true, "tableData": false, "resourceData": true, "rowEdit": false, "keyEdit": true, "transfer": false, "backup": false}
	default:
		return map[string]bool{"sql": true, "schema": true, "tableData": true, "resourceData": false, "rowEdit": true, "transfer": true, "backup": true}
	}
}

func databaseMode(dbType string) string {
	switch normalizeDatabaseType(dbType) {
	case "mongodb":
		return "document"
	case "redis":
		return "keyvalue"
	default:
		return "sql"
	}
}

// ensureMySQLFeature prevents MySQL-specific endpoints from being applied to
// other database engines through a direct API call.
func ensureMySQLFeature(item *model.AssetDatabase) error {
	if normalizeDatabaseType(item.DBType) != "mysql" {
		return fmt.Errorf("%s 暂不支持该操作，请在工作台使用该类型支持的能力", databaseTypeDisplayName(item.DBType))
	}
	return nil
}

// ensureRelationalImportFeature limits table import to engines with a tabular
// schema. MongoDB and Redis use their own resource models and are excluded.
func ensureRelationalImportFeature(item *model.AssetDatabase) error {
	switch normalizeDatabaseType(item.DBType) {
	case "mysql", "postgresql":
		return nil
	default:
		return fmt.Errorf("%s 暂不支持表数据导入", databaseTypeDisplayName(item.DBType))
	}
}

func importTableName(item *model.AssetDatabase, schema, table string) string {
	if normalizeDatabaseType(item.DBType) == "postgresql" {
		return postgresTableName(schema, table)
	}
	return quoteIdentifier(schema) + "." + quoteIdentifier(table)
}

func importJoinIdentifiers(item *model.AssetDatabase, columns []string) string {
	if normalizeDatabaseType(item.DBType) == "postgresql" {
		return postgresJoinIdentifiers(columns)
	}
	return joinIdentifiers(columns)
}

func importPlaceholders(item *model.AssetDatabase, count int) string {
	items := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		if normalizeDatabaseType(item.DBType) == "postgresql" {
			items = append(items, fmt.Sprintf("$%d", index))
		} else {
			items = append(items, "?")
		}
	}
	return strings.Join(items, ",")
}

func (s *Service) getImportTableColumns(item *model.AssetDatabase, db *sql.DB, schema, table string) ([]databaseTableColumn, error) {
	if normalizeDatabaseType(item.DBType) == "postgresql" {
		return postgresTableColumns(db, schema, table)
	}
	return s.getTableColumns(db, schema, table)
}

func (s *Service) createPostgresImportTable(sourceDB, targetDB *sql.DB, sourceSchema, sourceTable, targetSchema, targetTable string) error {
	rows, err := sourceDB.Query(`
		SELECT a.attname,
			pg_catalog.format_type(a.atttypid, a.atttypmod),
			a.attnotnull,
			pg_get_expr(ad.adbin, ad.adrelid)
		FROM pg_catalog.pg_attribute AS a
		JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
		LEFT JOIN pg_catalog.pg_attrdef AS ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE n.nspname = $1 AND c.relname = $2
			AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, sourceSchema, sourceTable)
	if err != nil {
		return err
	}
	defer rows.Close()

	definitions := make([]string, 0)
	for rows.Next() {
		var name, dataType string
		var notNull bool
		var defaultExpr sql.NullString
		if err := rows.Scan(&name, &dataType, &notNull, &defaultExpr); err != nil {
			return err
		}
		definition := postgresQuoteIdentifier(name) + " " + dataType
		// Sequence defaults are tied to the source table. Imported data carries
		// explicit values, so omit them rather than retaining a broken reference.
		if defaultExpr.Valid && strings.TrimSpace(defaultExpr.String) != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(defaultExpr.String)), "nextval(") {
			definition += " DEFAULT " + defaultExpr.String
		}
		if notNull {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(definitions) == 0 {
		return fmt.Errorf("源表不存在或没有可导入字段")
	}

	primaryRows, err := sourceDB.Query(`
		SELECT a.attname
		FROM pg_catalog.pg_constraint AS con
		JOIN pg_catalog.pg_class AS c ON c.oid = con.conrelid
		JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_attribute AS a ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
		WHERE con.contype = 'p' AND n.nspname = $1 AND c.relname = $2
		ORDER BY array_position(con.conkey, a.attnum)`, sourceSchema, sourceTable)
	if err != nil {
		return err
	}
	primaryKeys := make([]string, 0)
	for primaryRows.Next() {
		var name string
		if err := primaryRows.Scan(&name); err != nil {
			primaryRows.Close()
			return err
		}
		primaryKeys = append(primaryKeys, postgresQuoteIdentifier(name))
	}
	if err := primaryRows.Err(); err != nil {
		primaryRows.Close()
		return err
	}
	primaryRows.Close()
	if len(primaryKeys) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primaryKeys, ", ")+")")
	}

	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", postgresTableName(targetSchema, targetTable), strings.Join(definitions, ", "))
	_, err = targetDB.Exec(createSQL)
	return err
}

func databaseTypeDisplayName(dbType string) string {
	switch normalizeDatabaseType(dbType) {
	case "postgresql":
		return "PostgreSQL"
	case "mongodb":
		return "MongoDB"
	case "redis":
		return "Redis"
	default:
		return "MySQL"
	}
}

func (s *Service) resolveAssetDatabaseTarget(item model.AssetDatabase) (model.AssetDatabase, func(), error) {
	target := item
	cleanup := func() {}
	if normalizeConnectionMode(item.ConnectionMode) != "gateway" || item.GatewayID == nil || *item.GatewayID == 0 {
		return target, cleanup, nil
	}
	address, tunnelCleanup, err := s.startGatewayTunnel(*item.GatewayID, net.JoinHostPort(strings.TrimSpace(item.Host), strconv.Itoa(databasePortByType(item.DBType, item.Port))))
	if err != nil {
		return target, cleanup, err
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		tunnelCleanup()
		return target, cleanup, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		tunnelCleanup()
		return target, cleanup, err
	}
	target.Host, target.Port, cleanup = host, port, tunnelCleanup
	return target, cleanup, nil
}

func postgresDSN(item model.AssetDatabase) string {
	database := strings.TrimSpace(item.DBName)
	if database == "" {
		database = "postgres"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable connect_timeout=5", item.Host, databasePortByType("postgresql", item.Port), item.Username, item.Password, database)
}

func (s *Service) openAssetPostgresDatabase(item model.AssetDatabase) (*sql.DB, func(), error) {
	target, cleanup, err := s.resolveAssetDatabaseTarget(item)
	if err != nil {
		return nil, cleanup, err
	}
	db, err := sql.Open("pgx", postgresDSN(target))
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return db, cleanup, nil
}

func (s *Service) inspectAssetDatabase(item model.AssetDatabase) (string, error) {
	switch normalizeDatabaseType(item.DBType) {
	case "postgresql":
		db, cleanup, err := s.openAssetPostgresDatabase(item)
		if err != nil {
			return "", err
		}
		defer cleanup()
		defer db.Close()
		if err := db.Ping(); err != nil {
			return "", err
		}
		var version string
		err = db.QueryRow("SHOW server_version").Scan(&version)
		return version, err
	case "mongodb":
		target, cleanup, err := s.resolveAssetDatabaseTarget(item)
		if err != nil {
			return "", err
		}
		defer cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%d", target.Host, databasePortByType("mongodb", target.Port))).SetAuth(options.Credential{Username: target.Username, Password: target.Password}))
		if err != nil {
			return "", err
		}
		defer client.Disconnect(context.Background())
		var info struct {
			Version string `bson:"version"`
		}
		err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&info)
		return info.Version, err
	case "redis":
		target, cleanup, err := s.resolveAssetDatabaseTarget(item)
		if err != nil {
			return "", err
		}
		defer cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		dbIndex, _ := strconv.Atoi(strings.TrimSpace(target.DBName))
		client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(target.Host, strconv.Itoa(databasePortByType("redis", target.Port))), Username: target.Username, Password: target.Password, DB: dbIndex})
		defer client.Close()
		if err := client.Ping(ctx).Err(); err != nil {
			return "", err
		}
		info, err := client.Info(ctx, "server").Result()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(info, "\n") {
			if strings.HasPrefix(line, "redis_version:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "redis_version:")), nil
			}
		}
		return "Redis", nil
	default:
		return s.inspectAssetMySQLDatabase(item)
	}
}

func (s *Service) getNonMySQLSchemaTree(item *model.AssetDatabase) (map[string]any, error) {
	switch normalizeDatabaseType(item.DBType) {
	case "postgresql":
		db, cleanup, err := s.openAssetPostgresDatabase(*item)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		defer db.Close()
		rows, err := db.Query(`
			SELECT schema_name
			FROM information_schema.schemata
			WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
			  AND schema_name NOT LIKE 'pg_%'
			ORDER BY schema_name
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		schemas := make([]map[string]any, 0)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			tableRows, err := db.Query(`SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name`, name)
			if err != nil {
				return nil, err
			}
			type tableMeta struct {
				name      string
				tableType string
			}
			metadata := make([]tableMeta, 0)
			for tableRows.Next() {
				var tableName, tableType string
				if err := tableRows.Scan(&tableName, &tableType); err != nil {
					tableRows.Close()
					return nil, err
				}
				metadata = append(metadata, tableMeta{name: tableName, tableType: tableType})
			}
			if err := tableRows.Err(); err != nil {
				tableRows.Close()
				return nil, err
			}
			tableRows.Close()

			tables := make([]map[string]any, 0, len(metadata))
			for _, table := range metadata {
				var rowCount any
				if strings.EqualFold(table.tableType, "BASE TABLE") {
					var count int64
					countQuery := "SELECT COUNT(*) FROM " + postgresQuoteIdentifier(name) + "." + postgresQuoteIdentifier(table.name)
					if err := db.QueryRow(countQuery).Scan(&count); err == nil {
						rowCount = count
					}
				}
				tables = append(tables, map[string]any{
					"name":     table.name,
					"type":     table.tableType,
					"rows":     rowCount,
					"schema":   name,
					"fullName": name + "." + table.name,
				})
			}
			schemas = append(schemas, map[string]any{"name": name, "tableCount": len(tables), "tables": tables, "isCurrent": name == defaultSchema(item, "")})
		}
		return map[string]any{"schemas": schemas, "defaultSchema": defaultSchema(item, "")}, nil
	case "mongodb":
		target, cleanup, err := s.resolveAssetDatabaseTarget(*item)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%d", target.Host, databasePortByType("mongodb", target.Port))).SetAuth(options.Credential{Username: target.Username, Password: target.Password}))
		if err != nil {
			return nil, err
		}
		defer client.Disconnect(context.Background())
		names, err := client.ListDatabaseNames(ctx, bson.D{})
		if err != nil {
			return nil, err
		}
		schemas := make([]map[string]any, 0, len(names))
		for _, name := range names {
			collections, _ := client.Database(name).ListCollectionNames(ctx, bson.D{})
			tables := make([]map[string]any, 0, len(collections))
			for _, collection := range collections {
				tables = append(tables, map[string]any{"name": collection, "type": "collection", "rows": 0, "schema": name, "fullName": name + "." + collection})
			}
			schemas = append(schemas, map[string]any{"name": name, "tableCount": len(tables), "tables": tables, "isCurrent": name == strings.TrimSpace(item.DBName)})
		}
		return map[string]any{"schemas": schemas, "defaultSchema": defaultSchema(item, "")}, nil
	case "redis":
		target, cleanup, err := s.resolveAssetDatabaseTarget(*item)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		dbIndex, _ := strconv.Atoi(strings.TrimSpace(target.DBName))
		client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(target.Host, strconv.Itoa(databasePortByType("redis", target.Port))), Username: target.Username, Password: target.Password, DB: dbIndex})
		defer client.Close()
		if err := client.Ping(ctx).Err(); err != nil {
			return nil, err
		}
		keys, _, err := client.Scan(ctx, 0, "*", 100).Result()
		if err != nil {
			return nil, err
		}
		tables := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			keyType, _ := client.Type(ctx, key).Result()
			tables = append(tables, map[string]any{"name": key, "type": keyType, "rows": 0, "schema": fmt.Sprintf("db%d", dbIndex), "fullName": key})
		}
		return map[string]any{"schemas": []map[string]any{{"name": fmt.Sprintf("db%d", dbIndex), "tableCount": len(tables), "tables": tables, "isCurrent": true}}, "defaultSchema": fmt.Sprintf("db%d", dbIndex)}, nil
	default:
		return nil, fmt.Errorf("unsupported database type")
	}
}

// GetDatabaseResourceData provides a read-only browser for database engines
// whose resources are not relational MySQL tables.
func (s *Service) GetDatabaseResourceData(payload DBMSTableDataQueryPayload) (map[string]any, error) {
	if payload.DatabaseID == 0 || strings.TrimSpace(payload.Table) == "" {
		return nil, fmt.Errorf("please select a resource first")
	}
	if payload.PageNum < 1 {
		payload.PageNum = 1
	}
	if payload.PageSize < 1 || payload.PageSize > 200 {
		payload.PageSize = 25
	}
	item, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	switch normalizeDatabaseType(item.DBType) {
	case "postgresql":
		return s.getPostgresResourceData(item, payload)
	case "mongodb":
		return s.getMongoResourceData(item, payload)
	case "redis":
		return s.getRedisResourceData(item, payload)
	default:
		return nil, fmt.Errorf("resource browser is only available for PostgreSQL, MongoDB, and Redis")
	}
}

func postgresQuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(name), `"`, `""`) + `"`
}

func (s *Service) getPostgresResourceData(item *model.AssetDatabase, payload DBMSTableDataQueryPayload) (map[string]any, error) {
	db, cleanup, err := s.openAssetPostgresDatabase(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	schema := strings.TrimSpace(payload.Schema)
	if schema == "" {
		schema = "public"
	}
	columnsRows, err := db.Query(`SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	columns := make([]databaseTableColumn, 0)
	columnNames := make([]string, 0)
	for columnsRows.Next() {
		var name, dataType string
		if err := columnsRows.Scan(&name, &dataType); err != nil {
			columnsRows.Close()
			return nil, err
		}
		columns = append(columns, databaseTableColumn{Name: name, DataType: dataType, ColumnType: dataType})
		columnNames = append(columnNames, name)
	}
	columnsRows.Close()
	if len(columnNames) == 0 {
		return nil, fmt.Errorf("resource does not exist or has no readable columns")
	}
	if payload.FilterKey != "" {
		valid := false
		for _, name := range columnNames {
			if name == payload.FilterKey {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("invalid filter column")
		}
	}
	from := postgresQuoteIdentifier(schema) + "." + postgresQuoteIdentifier(payload.Table)
	where := ""
	args := make([]any, 0, 3)
	if strings.TrimSpace(payload.FilterText) != "" {
		if payload.FilterKey != "" {
			where = " WHERE CAST(" + postgresQuoteIdentifier(payload.FilterKey) + " AS TEXT) ILIKE $1"
			args = append(args, "%"+strings.TrimSpace(payload.FilterText)+"%")
		} else {
			clauses := make([]string, 0, len(columnNames))
			for _, name := range columnNames {
				clauses = append(clauses, "CAST("+postgresQuoteIdentifier(name)+" AS TEXT) ILIKE $1")
			}
			where = " WHERE " + strings.Join(clauses, " OR ")
			args = append(args, "%"+strings.TrimSpace(payload.FilterText)+"%")
		}
	}
	var total int64
	if err := db.QueryRow("SELECT COUNT(*) FROM "+from+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]any{}, args...)
	limitPos := len(queryArgs) + 1
	offsetPos := len(queryArgs) + 2
	queryArgs = append(queryArgs, payload.PageSize, (payload.PageNum-1)*payload.PageSize)
	rows, err := db.Query("SELECT * FROM "+from+where+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	_, dataRows, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "table": payload.Table, "columns": columns, "rows": dataRows, "total": total, "pageNum": payload.PageNum, "pageSize": payload.PageSize, "readOnly": true, "resourceType": "table"}, nil
}

func postgresTableColumns(db *sql.DB, schema, table string) ([]databaseTableColumn, error) {
	rows, err := db.Query(`
		SELECT
			columns.column_name,
			columns.data_type,
			columns.udt_name,
			CASE WHEN primary_key.column_name IS NULL THEN '' ELSE 'PRI' END AS column_key,
			columns.is_nullable,
			columns.column_default,
			CASE WHEN columns.is_identity = 'YES' THEN 'identity' ELSE '' END AS extra
		FROM information_schema.columns AS columns
		LEFT JOIN (
			SELECT key_columns.table_schema, key_columns.table_name, key_columns.column_name
			FROM information_schema.table_constraints AS constraints
			JOIN information_schema.key_column_usage AS key_columns
				ON constraints.constraint_name = key_columns.constraint_name
				AND constraints.table_schema = key_columns.table_schema
				AND constraints.table_name = key_columns.table_name
			WHERE constraints.constraint_type = 'PRIMARY KEY'
		) AS primary_key
			ON primary_key.table_schema = columns.table_schema
			AND primary_key.table_name = columns.table_name
			AND primary_key.column_name = columns.column_name
		WHERE columns.table_schema = $1 AND columns.table_name = $2
		ORDER BY columns.ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]databaseTableColumn, 0)
	for rows.Next() {
		var column databaseTableColumn
		if err := rows.Scan(&column.Name, &column.DataType, &column.ColumnType, &column.ColumnKey, &column.IsNullable, &column.ColumnDefault, &column.Extra); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

func postgresTableName(schema, table string) string {
	return postgresQuoteIdentifier(schema) + "." + postgresQuoteIdentifier(table)
}

func postgresPrimaryKeys(columns []databaseTableColumn) []string {
	keys := make([]string, 0)
	for _, column := range columns {
		if strings.EqualFold(column.ColumnKey, "PRI") {
			keys = append(keys, column.Name)
		}
	}
	return keys
}

func postgresJoinIdentifiers(columns []string) string {
	items := make([]string, 0, len(columns))
	for _, column := range columns {
		items = append(items, postgresQuoteIdentifier(column))
	}
	return strings.Join(items, ", ")
}

func postgresFilterClause(columns []databaseTableColumn, key, text string) (string, []any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil, nil
	}
	key = strings.TrimSpace(key)
	if key != "" {
		for _, column := range columns {
			if column.Name == key {
				return " WHERE CAST(" + postgresQuoteIdentifier(key) + " AS TEXT) ILIKE $1", []any{"%" + text + "%"}, nil
			}
		}
		return "", nil, fmt.Errorf("筛选字段不存在")
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, "CAST("+postgresQuoteIdentifier(column.Name)+" AS TEXT) ILIKE $1")
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return " WHERE (" + strings.Join(parts, " OR ") + ")", []any{"%" + text + "%"}, nil
}

func postgresWhereByPrimaryKey(columns []databaseTableColumn, row map[string]any, offset int) (string, []any, error) {
	keys := postgresPrimaryKeys(columns)
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("当前表没有主键，不能直接修改或删除数据")
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for index, key := range keys {
		value, exists := row[key]
		if !exists {
			return "", nil, fmt.Errorf("缺少主键字段 %s", key)
		}
		clauses = append(clauses, fmt.Sprintf("%s = $%d", postgresQuoteIdentifier(key), offset+index+1))
		args = append(args, normalizeJSONValue(value))
	}
	return strings.Join(clauses, " AND "), args, nil
}

func postgresSQLLiteral(value any) string {
	if boolean, ok := value.(bool); ok {
		if boolean {
			return "TRUE"
		}
		return "FALSE"
	}
	return sqlLiteral(value)
}

func postgresRollbackWhere(columns []databaseTableColumn, row map[string]any) string {
	keys := postgresPrimaryKeys(columns)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, exists := row[key]; exists {
			parts = append(parts, postgresQuoteIdentifier(key)+" = "+postgresSQLLiteral(value))
		}
	}
	return strings.Join(parts, " AND ")
}

func postgresInsertRollback(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	where := postgresRollbackWhere(columns, row)
	if where == "" {
		return ""
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s;", postgresTableName(schema, table), where)
}

func postgresUpdateRollback(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	sets := make([]string, 0)
	for _, column := range columns {
		if value, exists := row[column.Name]; exists {
			sets = append(sets, postgresQuoteIdentifier(column.Name)+" = "+postgresSQLLiteral(value))
		}
	}
	where := postgresRollbackWhere(columns, row)
	if len(sets) == 0 || where == "" {
		return ""
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;", postgresTableName(schema, table), strings.Join(sets, ", "), where)
}

func postgresDeleteRollback(schema, table string, columns []databaseTableColumn, row map[string]any) string {
	keys := make([]string, 0)
	values := make([]string, 0)
	for _, column := range columns {
		if value, exists := row[column.Name]; exists {
			keys = append(keys, column.Name)
			values = append(values, postgresSQLLiteral(value))
		}
	}
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", postgresTableName(schema, table), postgresJoinIdentifiers(keys), strings.Join(values, ", "))
}

func (s *Service) getPostgresTableData(item *model.AssetDatabase, payload DBMSTableDataQueryPayload) (map[string]any, error) {
	if payload.PageNum < 1 {
		payload.PageNum = 1
	}
	if payload.PageSize < 1 || payload.PageSize > 200 {
		payload.PageSize = 25
	}
	schema := defaultSchema(item, payload.Schema)
	db, cleanup, err := s.openAssetPostgresDatabase(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()

	columns, err := postgresTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("数据表不存在或没有可读取的字段")
	}
	where, args, err := postgresFilterClause(columns, payload.FilterKey, payload.FilterText)
	if err != nil {
		return nil, err
	}
	from := postgresTableName(schema, payload.Table)
	var total int64
	if err := db.QueryRow("SELECT COUNT(*) FROM "+from+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]any{}, args...)
	limitPos := len(queryArgs) + 1
	offsetPos := len(queryArgs) + 2
	queryArgs = append(queryArgs, payload.PageSize, (payload.PageNum-1)*payload.PageSize)
	rows, err := db.Query("SELECT * FROM "+from+where+fmt.Sprintf(" LIMIT $%d OFFSET $%d", limitPos, offsetPos), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columnNames, dataRows, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":      schema,
		"table":       payload.Table,
		"columns":     columns,
		"columnNames": columnNames,
		"rows":        dataRows,
		"primaryKeys": postgresPrimaryKeys(columns),
		"total":       total,
		"pageNum":     payload.PageNum,
		"pageSize":    payload.PageSize,
		"filterKey":   payload.FilterKey,
		"filterText":  payload.FilterText,
	}, nil
}

func (s *Service) insertPostgresTableRow(item *model.AssetDatabase, payload DBMSTableInsertPayload) (map[string]any, error) {
	if err := ensureDatabaseWritable(item); err != nil {
		return nil, err
	}
	schema := defaultSchema(item, payload.Schema)
	db, cleanup, err := s.openAssetPostgresDatabase(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	columns, err := postgresTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	insertColumns := make([]string, 0)
	values := make([]any, 0)
	for _, column := range columns {
		if value, exists := payload.Row[column.Name]; exists {
			insertColumns = append(insertColumns, column.Name)
			values = append(values, normalizeJSONValue(value))
		}
	}
	if len(insertColumns) == 0 {
		return nil, fmt.Errorf("没有可插入的数据")
	}
	placeholders := make([]string, 0, len(insertColumns))
	for index := range insertColumns {
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", postgresTableName(schema, payload.Table), postgresJoinIdentifiers(insertColumns), strings.Join(placeholders, ", "))
	primaryKeys := postgresPrimaryKeys(columns)
	insertedRow := make(map[string]any, len(payload.Row))
	for key, value := range payload.Row {
		insertedRow[key] = value
	}
	var affected int64 = 1
	if len(primaryKeys) > 0 {
		returningSQL := insertSQL + " RETURNING " + postgresJoinIdentifiers(primaryKeys)
		scanTargets := make([]any, len(primaryKeys))
		for index := range scanTargets {
			scanTargets[index] = new(any)
		}
		if err := db.QueryRow(returningSQL, values...).Scan(scanTargets...); err != nil {
			return nil, err
		}
		for index, key := range primaryKeys {
			insertedRow[key] = *(scanTargets[index].(*any))
		}
	} else {
		result, err := db.Exec(insertSQL, values...)
		if err != nil {
			return nil, err
		}
		affected, _ = result.RowsAffected()
	}
	rollbackSQL := postgresInsertRollback(schema, payload.Table, columns, insertedRow)
	s.logDBSQLHistory(item, schema, payload.Table, "INSERT", insertSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) updatePostgresTableRow(item *model.AssetDatabase, payload DBMSTableUpdatePayload) (map[string]any, error) {
	if err := ensureDatabaseWritable(item); err != nil {
		return nil, err
	}
	schema := defaultSchema(item, payload.Schema)
	db, cleanup, err := s.openAssetPostgresDatabase(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	columns, err := postgresTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	if !databaseColumnsHavePrimaryKey(columns) {
		return nil, fmt.Errorf("当前表没有主键，不能直接编辑数据")
	}
	setParts := make([]string, 0)
	args := make([]any, 0)
	for _, column := range columns {
		current, exists := payload.Current[column.Name]
		if !exists || fmt.Sprintf("%v", normalizeJSONValue(current)) == fmt.Sprintf("%v", normalizeJSONValue(payload.Original[column.Name])) {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = $%d", postgresQuoteIdentifier(column.Name), len(args)+1))
		args = append(args, normalizeJSONValue(current))
	}
	if len(setParts) == 0 {
		return map[string]any{"rowsAffected": 0, "rollbackSql": ""}, nil
	}
	where, whereArgs, err := postgresWhereByPrimaryKey(columns, payload.Original, len(args))
	if err != nil {
		return nil, err
	}
	args = append(args, whereArgs...)
	updateSQL := fmt.Sprintf("UPDATE %s SET %s WHERE %s", postgresTableName(schema, payload.Table), strings.Join(setParts, ", "), where)
	result, err := db.Exec(updateSQL, args...)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	rollbackSQL := postgresUpdateRollback(schema, payload.Table, columns, payload.Original)
	s.logDBSQLHistory(item, schema, payload.Table, "UPDATE", updateSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) deletePostgresTableRow(item *model.AssetDatabase, payload DBMSTableDeletePayload) (map[string]any, error) {
	if err := ensureDatabaseWritable(item); err != nil {
		return nil, err
	}
	schema := defaultSchema(item, payload.Schema)
	db, cleanup, err := s.openAssetPostgresDatabase(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	defer db.Close()
	columns, err := postgresTableColumns(db, schema, payload.Table)
	if err != nil {
		return nil, err
	}
	where, args, err := postgresWhereByPrimaryKey(columns, payload.Row, 0)
	if err != nil {
		return nil, err
	}
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s", postgresTableName(schema, payload.Table), where)
	result, err := db.Exec(deleteSQL, args...)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	rollbackSQL := postgresDeleteRollback(schema, payload.Table, columns, payload.Row)
	s.logDBSQLHistory(item, schema, payload.Table, "DELETE", deleteSQL, 1, affected, 0, "", rollbackSQL)
	return map[string]any{"rowsAffected": affected, "rollbackSql": rollbackSQL}, nil
}

func (s *Service) getMongoResourceData(item *model.AssetDatabase, payload DBMSTableDataQueryPayload) (map[string]any, error) {
	target, cleanup, err := s.resolveAssetDatabaseTarget(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	clientOptions := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%d", target.Host, databasePortByType("mongodb", target.Port)))
	if strings.TrimSpace(target.Username) != "" || target.Password != "" {
		clientOptions.SetAuth(options.Credential{Username: target.Username, Password: target.Password})
	}
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())
	schema := strings.TrimSpace(payload.Schema)
	if schema == "" {
		schema = strings.TrimSpace(target.DBName)
	}
	collection := client.Database(schema).Collection(payload.Table)
	filter := bson.M{}
	if payload.FilterKey != "" && strings.TrimSpace(payload.FilterText) != "" {
		filter[payload.FilterKey] = bson.M{"$regex": strings.TrimSpace(payload.FilterText), "$options": "i"}
	}
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	cursor, err := collection.Find(ctx, filter, options.Find().SetSkip(int64((payload.PageNum-1)*payload.PageSize)).SetLimit(int64(payload.PageSize)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	rows := make([]map[string]any, 0)
	keys := map[string]bool{}
	for cursor.Next(ctx) {
		var document bson.M
		if err := cursor.Decode(&document); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for key, value := range document {
			row[key] = value
			keys[key] = true
		}
		rows = append(rows, row)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	indexesCursor, err := collection.Indexes().List(ctx)
	indexes := make([]map[string]any, 0)
	if err == nil {
		defer indexesCursor.Close(ctx)
		for indexesCursor.Next(ctx) {
			var index bson.M
			if indexesCursor.Decode(&index) == nil {
				indexes = append(indexes, index)
			}
		}
	}
	columnNames := make([]string, 0, len(keys))
	for key := range keys {
		columnNames = append(columnNames, key)
	}
	sort.Strings(columnNames)
	columns := make([]databaseTableColumn, 0, len(columnNames))
	for _, key := range columnNames {
		columns = append(columns, databaseTableColumn{Name: key, DataType: "document"})
	}
	return map[string]any{"schema": schema, "table": payload.Table, "columns": columns, "rows": rows, "total": total, "pageNum": payload.PageNum, "pageSize": payload.PageSize, "indexes": indexes, "readOnly": true, "resourceType": "collection"}, nil
}

func (s *Service) getRedisResourceData(item *model.AssetDatabase, payload DBMSTableDataQueryPayload) (map[string]any, error) {
	target, cleanup, err := s.resolveAssetDatabaseTarget(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	dbIndex, _ := strconv.Atoi(strings.TrimSpace(target.DBName))
	client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(target.Host, strconv.Itoa(databasePortByType("redis", target.Port))), Username: target.Username, Password: target.Password, DB: dbIndex})
	defer client.Close()
	key := payload.Table
	keyType, err := client.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	value := ""
	switch keyType {
	case "string":
		value, _ = client.Get(ctx, key).Result()
	case "list":
		items, _ := client.LRange(ctx, key, 0, 199).Result()
		encoded, _ := json.Marshal(items)
		value = string(encoded)
	case "set":
		items, _ := client.SMembers(ctx, key).Result()
		encoded, _ := json.Marshal(items)
		value = string(encoded)
	case "zset":
		items, _ := client.ZRangeWithScores(ctx, key, 0, 199).Result()
		encoded, _ := json.Marshal(items)
		value = string(encoded)
	case "hash":
		items, _ := client.HGetAll(ctx, key).Result()
		encoded, _ := json.Marshal(items)
		value = string(encoded)
	case "stream":
		count, _ := client.XLen(ctx, key).Result()
		value = fmt.Sprintf("stream (%d entries)", count)
	default:
		value = keyType
	}
	ttlSeconds := int64(ttl / time.Second)
	ttlText := fmt.Sprintf("%d 秒", ttlSeconds)
	if ttl == -1*time.Nanosecond {
		ttlSeconds, ttlText = -1, "永不过期"
	} else if ttl == -2*time.Nanosecond {
		ttlSeconds, ttlText = -2, "已过期"
	}
	return map[string]any{
		"schema": payload.Schema, "table": key, "columns": []databaseTableColumn{{Name: "key"}, {Name: "type"}, {Name: "ttl"}, {Name: "value"}},
		"rows": []map[string]any{{"key": key, "type": keyType, "ttl": ttlText, "ttlSeconds": ttlSeconds, "value": value}}, "total": 1, "pageNum": 1, "pageSize": 1,
		"readOnly": normalizeDatabaseAccessMode(item.AccessMode) == "readonly", "resourceType": "key",
	}, nil
}

type RedisCommandPayload struct {
	DatabaseID  uint   `json:"databaseId"`
	CommandText string `json:"commandText"`
	Confirmed   bool   `json:"confirmed"`
	Operator    string `json:"-"`
	ClientIP    string `json:"-"`
}

type RedisCommandAnalysis struct {
	Command        string `json:"command"`
	Arguments      int    `json:"arguments"`
	WriteOperation bool   `json:"writeOperation"`
	RiskLevel      string `json:"riskLevel"`
	Reason         string `json:"reason"`
	AccessMode     string `json:"accessMode"`
}

func splitRedisCommand(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("请输入 Redis 命令")
	}
	args := make([]string, 0)
	var current strings.Builder
	quote := rune(0)
	escaped := false
	for _, char := range input {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("Redis 命令中的引号未闭合")
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("请输入 Redis 命令")
	}
	return args, nil
}

func isRedisReadCommand(command string) bool {
	switch strings.ToUpper(command) {
	case "GET", "MGET", "EXISTS", "TTL", "PTTL", "TYPE", "KEYS", "SCAN", "HSCAN", "SSCAN", "ZSCAN", "LRANGE", "LLEN", "LINDEX", "SMEMBERS", "SCARD", "SISMEMBER", "ZRANGE", "ZRANGEBYSCORE", "ZSCORE", "ZCARD", "HGET", "HMGET", "HGETALL", "HLEN", "HEXISTS", "DBSIZE", "INFO", "MEMORY", "PING", "TIME":
		return true
	default:
		return false
	}
}

func isBlockedRedisCommand(command string) bool {
	switch strings.ToUpper(command) {
	case "FLUSHALL", "FLUSHDB", "SHUTDOWN", "DEBUG", "MODULE", "ACL", "REPLICAOF", "SLAVEOF", "CONFIG", "MIGRATE":
		return true
	default:
		return false
	}
}

func (s *Service) AnalyzeRedisCommand(payload RedisCommandPayload) (*RedisCommandAnalysis, error) {
	item, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(item.DBType) != "redis" {
		return nil, fmt.Errorf("所选数据库不是 Redis")
	}
	args, err := splitRedisCommand(payload.CommandText)
	if err != nil {
		return nil, err
	}
	command := strings.ToUpper(args[0])
	if isBlockedRedisCommand(command) {
		return nil, fmt.Errorf("Redis 命令控制台不允许执行 %s", command)
	}
	writeOperation := !isRedisReadCommand(command)
	reason := "只读命令，可直接执行"
	riskLevel := "low"
	if writeOperation {
		reason = "写入命令，需要二次确认"
		riskLevel = "medium"
	}
	if writeOperation && normalizeDatabaseAccessMode(item.AccessMode) == "readonly" {
		reason = "当前 Redis 连接为只读模式"
		riskLevel = "high"
	}
	return &RedisCommandAnalysis{Command: command, Arguments: len(args) - 1, WriteOperation: writeOperation, RiskLevel: riskLevel, Reason: reason, AccessMode: normalizeDatabaseAccessMode(item.AccessMode)}, nil
}

func redisReplyText(value any) string {
	if value == nil {
		return "(nil)"
	}
	switch v := value.(type) {
	case string:
		return v
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, redisReplyText(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		return "[" + strings.Join(v, ", ") + "]"
	default:
		return fmt.Sprint(value)
	}
}

func (s *Service) ExecuteRedisCommand(payload RedisCommandPayload) (map[string]any, error) {
	analysis, err := s.AnalyzeRedisCommand(payload)
	if err != nil {
		return nil, err
	}
	if analysis.WriteOperation {
		if err := s.requireResource("database", payload.DatabaseID, "", "write"); err != nil {
			return nil, err
		}
	}
	if analysis.WriteOperation && analysis.AccessMode == "readonly" {
		return nil, fmt.Errorf("当前 Redis 连接为只读模式，不能执行写入命令")
	}
	if analysis.WriteOperation && !payload.Confirmed {
		return nil, fmt.Errorf("写入命令需要确认后才能执行")
	}
	args, _ := splitRedisCommand(payload.CommandText)
	item, err := s.getAssetDatabase(payload.DatabaseID)
	if err != nil {
		return nil, err
	}
	target, cleanup, err := s.resolveAssetDatabaseTarget(*item)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dbIndex, _ := strconv.Atoi(strings.TrimSpace(target.DBName))
	client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(target.Host, strconv.Itoa(databasePortByType("redis", target.Port))), Username: target.Username, Password: target.Password, DB: dbIndex})
	defer client.Close()
	start := time.Now()
	commandArgs := make([]interface{}, 0, len(args))
	for _, arg := range args {
		commandArgs = append(commandArgs, arg)
	}
	reply, execErr := client.Do(ctx, commandArgs...).Result()
	durationMs := time.Since(start).Milliseconds()
	history := model.DatabaseSQLHistory{DatabaseID: item.ID, DatabaseName: item.Name, SchemaName: fmt.Sprintf("db%d", dbIndex), SQLType: "REDIS " + analysis.Command, SQLText: strings.TrimSpace(payload.CommandText), ExecutionID: newDBMSExecutionID(), Operator: strings.TrimSpace(payload.Operator), ClientIP: strings.TrimSpace(payload.ClientIP), Environment: item.Env, AccessMode: normalizeDatabaseAccessMode(item.AccessMode), DurationMs: durationMs}
	if execErr != nil {
		history.Status = 2
		history.ErrorMessage = execErr.Error()
		s.db.Create(&history)
		return nil, execErr
	}
	history.Status = 1
	history.RowsAffected = 1
	s.db.Create(&history)
	return map[string]any{"command": analysis.Command, "columns": []string{"command", "result"}, "rows": []map[string]any{{"command": strings.TrimSpace(payload.CommandText), "result": redisReplyText(reply)}}, "rowsAffected": 1, "durationMs": durationMs, "executionId": history.ExecutionID, "analysis": analysis}, nil
}
