package service

import (
	"bufio"
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
)

type redisTestError string
type redisTestRequest struct {
	db   int
	args []string
}
type redisDBTestServer struct {
	listener                              net.Listener
	mu                                    sync.Mutex
	requests                              []redisTestRequest
	data                                  map[int]map[string]string
	denyConfig, denyInfo, cluster, sparse bool
}

func newRedisDBTestServer(t *testing.T) *redisDBTestServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &redisDBTestServer{listener: listener, data: map[int]map[string]string{0: {}, 10: {"demo:key": "only-in-db10"}}}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return s
}
func (s *redisDBTestServer) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	db := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if !strings.HasPrefix(line, "*") {
			return
		}
		count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return
		}
		args := make([]string, count)
		for i := range args {
			line, err = reader.ReadString('\n')
			if err != nil {
				return
			}
			n, e := strconv.Atoi(strings.TrimSpace(line[1:]))
			if e != nil {
				return
			}
			body := make([]byte, n+2)
			if _, e = io.ReadFull(reader, body); e != nil {
				return
			}
			args[i] = string(body[:n])
		}
		s.mu.Lock()
		s.requests = append(s.requests, redisTestRequest{db: db, args: args})
		reply := s.reply(&db, args)
		s.mu.Unlock()
		if _, err := io.WriteString(conn, redisTestRESP(reply)); err != nil {
			return
		}
	}
}
func redisTestRESP(value any) string {
	switch v := value.(type) {
	case redisTestError:
		return "-" + string(v) + "\r\n"
	case nil:
		return "$-1\r\n"
	case string:
		return fmt.Sprintf("$%d\r\n%s\r\n", len(v), v)
	case int:
		return fmt.Sprintf(":%d\r\n", v)
	case []any:
		out := fmt.Sprintf("*%d\r\n", len(v))
		for _, part := range v {
			out += redisTestRESP(part)
		}
		return out
	}
	panic(fmt.Sprintf("unsupported response %T", value))
}
func (s *redisDBTestServer) reply(db *int, args []string) any {
	if len(args) == 0 {
		return redisTestError("ERR empty")
	}
	command := strings.ToUpper(args[0])
	switch command {
	case "HELLO":
		return redisTestError("ERR unknown command 'hello'")
	case "CLIENT":
		return "OK"
	case "PING":
		return "PONG"
	case "SELECT":
		index, err := strconv.Atoi(args[1])
		if err != nil || index < 0 || index >= 16 || s.cluster && index != 0 {
			return redisTestError("ERR DB index is out of range")
		}
		*db = index
		return "OK"
	case "INFO":
		if s.denyInfo {
			return redisTestError("NOPERM INFO denied")
		}
		if args[1] == "cluster" {
			if s.cluster {
				return "cluster_enabled:1\r\n"
			}
			return "cluster_enabled:0\r\n"
		}
		return "# Keyspace\r\ndb10:keys=1,expires=0,avg_ttl=0\r\n"
	case "CONFIG":
		if s.denyConfig {
			return redisTestError("NOPERM CONFIG denied")
		}
		return []any{"databases", "16"}
	case "DBSIZE":
		return len(s.data[*db])
	case "SCAN":
		if *db != 10 {
			return []any{"0", []any{}}
		}
		if s.sparse && args[1] == "0" {
			return []any{"8", []any{}}
		}
		return []any{"0", []any{"demo:key"}}
	case "TYPE":
		if _, ok := s.data[*db][args[1]]; ok {
			return "string"
		}
		return "none"
	case "TTL":
		return -1
	case "GET":
		value, ok := s.data[*db][args[1]]
		if !ok {
			return nil
		}
		return value
	case "SET":
		if s.data[*db] == nil {
			s.data[*db] = map[string]string{}
		}
		s.data[*db][args[1]] = args[2]
		return "OK"
	default:
		return redisTestError("ERR unsupported test command " + command)
	}
}
func redisTestService(t *testing.T, server *redisDBTestServer, mode string) (*Service, *systemTestDB, *[]string) {
	t.Helper()
	host, portText, _ := net.SplitHostPort(server.listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	auditSchemas := []string{}
	state := &systemTestDB{query: func(q string, _ []driver.NamedValue) ([]string, [][]driver.Value, error) {
		if !strings.Contains(q, "asset_database") {
			t.Fatalf("unexpected query: %s", q)
		}
		return []string{"id", "name", "db_type", "host", "port", "db_name", "access_mode"}, [][]driver.Value{{int64(7), "test-redis", "redis", host, int64(port), "0", mode}}, nil
	}, exec: func(q string, args []driver.NamedValue) (driver.Result, error) {
		if strings.Contains(q, "dbms_sql_history") {
			for _, arg := range args {
				if text, ok := arg.Value.(string); ok && strings.HasPrefix(text, "db") {
					auditSchemas = append(auditSchemas, text)
				}
			}
		}
		return driver.RowsAffected(1), nil
	}}
	return &Service{db: systemDB(t, state)}, state, &auditSchemas
}
func TestRedisLogicalDatabaseParsing(t *testing.T) {
	for _, value := range []string{"0", "db0", "10", "db10", "DB10"} {
		index, err := redisDatabaseIndex("0", value)
		if err != nil || index != 0 && index != 10 {
			t.Fatalf("%s => %d %v", value, index, err)
		}
	}
	for _, value := range []string{"db", "-1", "db-1", "abc", "db1.5", "2147483648"} {
		if _, err := redisDatabaseIndex("0", value); err == nil {
			t.Fatalf("accepted invalid index %s", value)
		}
	}
}
func TestRedisDiscoversEmptyDatabasesAndPopulatedDB10(t *testing.T) {
	server := newRedisDBTestServer(t)
	s, state, _ := redisTestService(t, server, "readonly")
	result, err := s.GetDatabaseSchemaTree(7)
	if err != nil {
		t.Fatal(err)
	}
	schemas := result["schemas"].([]map[string]any)
	if len(schemas) != 16 || schemas[10]["name"] != "db10" || schemas[10]["keyCount"] != int64(1) || schemas[0]["keyCount"] != int64(0) {
		t.Fatalf("incorrect discovery: %+v", schemas)
	}
	if len(state.writes) > 0 {
		t.Fatal("discovery changed business database")
	}
}
func TestRedisDiscoveryDegradesWithoutConfigurationPermissions(t *testing.T) {
	server := newRedisDBTestServer(t)
	server.denyConfig = true
	s, _, _ := redisTestService(t, server, "readonly")
	result, err := s.GetDatabaseSchemaTree(7)
	if err != nil {
		t.Fatal(err)
	}
	schemas := result["schemas"].([]map[string]any)
	if len(schemas) != 2 || schemas[1]["name"] != "db10" || len(result["warnings"].([]string)) == 0 {
		t.Fatal("keyspace DB10 not preserved on denied CONFIG")
	}
	server.mu.Lock()
	server.denyInfo = true
	server.mu.Unlock()
	result, err = s.GetDatabaseSchemaTree(7)
	if err != nil {
		t.Fatal(err)
	}
	schemas = result["schemas"].([]map[string]any)
	if schemas[0]["keyCount"] != nil {
		t.Fatal("unknown count represented as zero")
	}
}
func TestRedisReadsAndCommandsUseSelectedDatabase(t *testing.T) {
	server := newRedisDBTestServer(t)
	s, _, audit := redisTestService(t, server, "readwrite")
	list, err := s.GetRedisKeys(RedisKeysQuery{DatabaseID: 7, Schema: "db10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list["keys"].([]map[string]any)) != 1 {
		t.Fatal("db10 key missing")
	}
	row, err := s.GetDatabaseResourceData(DBMSTableDataQueryPayload{DatabaseID: 7, Schema: "db10", Table: "demo:key"})
	if err != nil {
		t.Fatal(err)
	}
	if row["rows"].([]map[string]any)[0]["value"] != "only-in-db10" {
		t.Fatal("key read from wrong DB")
	}
	for _, schema := range []string{"db10", "db0"} {
		result, err := s.ExecuteRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: schema, CommandText: "GET demo:key"})
		if err != nil {
			t.Fatal(err)
		}
		value := result["rows"].([]map[string]any)[0]["result"]
		if schema == "db10" && value != "only-in-db10" || schema == "db0" && value != "(nil)" {
			t.Fatalf("%s returned %v", schema, value)
		}
	}
	if _, err := s.ExecuteRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: "db10", CommandText: "SET added value", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.data[10]["added"] != "value" || server.data[0]["added"] != "" {
		t.Fatal("write leaked into db0")
	}
	if len(*audit) != 3 || (*audit)[2] != "db10" {
		t.Fatalf("audit did not retain selected DB: %v", *audit)
	}
}
func TestRedisScanCursorDoesNotMisreportEmptyDatabase(t *testing.T) {
	server := newRedisDBTestServer(t)
	server.sparse = true
	s, _, _ := redisTestService(t, server, "readonly")
	first, err := s.GetRedisKeys(RedisKeysQuery{DatabaseID: 7, Schema: "db10"})
	if err != nil {
		t.Fatal(err)
	}
	if first["complete"] != false || first["nextCursor"] != "8" || first["total"] != int64(1) {
		t.Fatalf("empty scan batch lost cursor: %v", first)
	}
	next, err := s.GetRedisKeys(RedisKeysQuery{DatabaseID: 7, Schema: "db10", Cursor: "8"})
	if err != nil {
		t.Fatal(err)
	}
	if len(next["keys"].([]map[string]any)) != 1 {
		t.Fatal("continuation failed")
	}
}
func TestRedisWriteRequiresConfirmationAndPermission(t *testing.T) {
	server := newRedisDBTestServer(t)
	s, _, _ := redisTestService(t, server, "readonly")
	if _, err := s.ExecuteRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: "db10", CommandText: "SET bad value", Confirmed: true}); err == nil {
		t.Fatal("readonly write accepted")
	}
	s, _, _ = redisTestService(t, server, "readwrite")
	if _, err := s.ExecuteRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: "db10", CommandText: "SET bad value"}); err == nil {
		t.Fatal("unconfirmed write accepted")
	}
	if _, err := s.AnalyzeRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: "db10", CommandText: "SELECT 0"}); err == nil {
		t.Fatal("connection-local SELECT accepted")
	}
	s = s.WithAccess(context.Background(), &access.Policy{UserID: 2, Grants: []model.ResourceGrant{{ResourceType: "database", ResourceID: 7, Actions: []string{"query"}}}})
	if _, err := s.ExecuteRedisCommand(RedisCommandPayload{DatabaseID: 7, Schema: "db10", CommandText: "SET bad value", Confirmed: true}); err == nil {
		t.Fatal("query-only grant allowed write")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.requests) != 0 {
		t.Fatal("blocked writes reached Redis")
	}
}
