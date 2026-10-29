package service

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"ops-admin/backend/model"
)

func redisDatabaseIndex(configured, selected string) (int, error) {
	value := strings.TrimSpace(selected)
	if value == "" {
		value = strings.TrimSpace(configured)
	}
	if value == "" {
		return 0, nil
	}
	value = strings.TrimPrefix(strings.ToLower(value), "db")
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, fmt.Errorf("Redis 逻辑库须为非负整数或 dbN 格式")
	}
	index, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("Redis 逻辑库编号超出有效范围")
	}
	return int(index), nil
}

func (s *Service) openAssetRedis(item model.AssetDatabase, schema string) (*redis.Client, func(), int, error) {
	index, err := redisDatabaseIndex(item.DBName, schema)
	if err != nil {
		return nil, func() {}, 0, err
	}
	target, cleanup, err := s.resolveAssetDatabaseTarget(item)
	if err != nil {
		return nil, cleanup, 0, err
	}
	client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(target.Host, strconv.Itoa(databasePortByType("redis", target.Port))), Username: target.Username, Password: target.Password, DB: index})
	return client, func() { _ = client.Close(); cleanup() }, index, nil
}

func redisInfoValues(info string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			values[key] = value
		}
	}
	return values
}

// Discover databases from server configuration and keyspace metadata. Missing
// ACL permissions are explicit; an unknown key count is never presented as zero.
func (s *Service) getRedisSchemaTree(item *model.AssetDatabase) (map[string]any, error) {
	client, cleanup, index, err := s.openAssetRedis(*item, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	warnings := []string{}
	counts := map[int]int64{}
	indices := map[int]bool{0: true, index: true}
	info, infoErr := client.Info(ctx, "keyspace").Result()
	if infoErr == nil {
		for name, value := range redisInfoValues(info) {
			if !strings.HasPrefix(name, "db") {
				continue
			}
			i, e := redisDatabaseIndex("", name)
			if e != nil {
				continue
			}
			for _, part := range strings.Split(value, ",") {
				if strings.HasPrefix(part, "keys=") {
					n, e := strconv.ParseInt(strings.TrimPrefix(part, "keys="), 10, 64)
					if e == nil && n >= 0 {
						counts[i] = n
						indices[i] = true
					}
				}
			}
		}
	} else {
		warnings = append(warnings, "无法读取 INFO keyspace，部分逻辑库的 Key 数量未知。")
	}
	clusterMode := false
	if info, e := client.Info(ctx, "cluster").Result(); e == nil {
		clusterMode = redisInfoValues(info)["cluster_enabled"] == "1"
	}
	databaseCount := 0
	if clusterMode {
		databaseCount = 1
		indices = map[int]bool{0: true}
		warnings = append(warnings, "Redis Cluster 仅支持 db0。")
	} else if config, e := client.ConfigGet(ctx, "databases").Result(); e == nil {
		databaseCount, _ = strconv.Atoi(config["databases"])
	}
	if databaseCount > 0 {
		limit := databaseCount
		if limit > 4096 {
			limit = 4096
			warnings = append(warnings, "数据库数量较多，仅列出前 4096 个及有数据的库；其他库可输入编号切换。")
		}
		for i := 0; i < limit; i++ {
			indices[i] = true
		}
	} else {
		warnings = append(warnings, "无法读取数据库数量，仅显示默认库及已发现的库；可手动输入 DB 编号切换。")
	}
	ordered := []int{}
	for i := range indices {
		ordered = append(ordered, i)
	}
	sort.Ints(ordered)
	schemas := make([]map[string]any, 0, len(ordered))
	for _, i := range ordered {
		var count any
		if n, ok := counts[i]; ok {
			count = n
		} else if infoErr == nil {
			count = int64(0)
		}
		schemas = append(schemas, map[string]any{"name": fmt.Sprintf("db%d", i), "keyCount": count, "tables": []any{}, "tableCount": 0, "isCurrent": i == index})
	}
	return map[string]any{"schemas": schemas, "defaultSchema": fmt.Sprintf("db%d", index), "databaseCount": databaseCount, "clusterMode": clusterMode, "warnings": warnings}, nil
}

type RedisKeysQuery struct {
	DatabaseID uint
	Schema     string
	Cursor     string
	Pattern    string
}

func (s *Service) GetRedisKeys(query RedisKeysQuery) (map[string]any, error) {
	item, err := s.getAssetDatabase(query.DatabaseID)
	if err != nil {
		return nil, err
	}
	if normalizeDatabaseType(item.DBType) != "redis" {
		return nil, fmt.Errorf("所选数据库不是 Redis")
	}
	client, cleanup, index, err := s.openAssetRedis(*item, query.Schema)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	cursor := uint64(0)
	if query.Cursor != "" {
		cursor, err = strconv.ParseUint(query.Cursor, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的扫描游标")
		}
	}
	pattern := query.Pattern
	if pattern == "" {
		pattern = "*"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	keys, next, err := client.Scan(ctx, cursor, pattern, 100).Result()
	if err != nil {
		return nil, err
	}
	pipe := client.Pipeline()
	types := make([]*redis.StatusCmd, len(keys))
	ttls := make([]*redis.DurationCmd, len(keys))
	for i, key := range keys {
		types[i] = pipe.Type(ctx, key)
		ttls[i] = pipe.TTL(ctx, key)
	}
	if len(keys) > 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
	}
	rows := []map[string]any{}
	for i, key := range keys {
		kind, e := types[i].Result()
		if e != nil {
			return nil, e
		}
		if kind == "none" {
			continue
		}
		ttl, e := ttls[i].Result()
		if e != nil {
			return nil, e
		}
		seconds, text := redisTTLDisplay(ttl)
		rows = append(rows, map[string]any{"key": key, "type": kind, "ttl": text, "ttlSeconds": seconds})
	}
	var total any
	warnings := []string{}
	if n, e := client.DBSize(ctx).Result(); e == nil {
		total = n
	} else {
		warnings = append(warnings, "当前账号无法读取 Key 总数。")
	}
	return map[string]any{"schema": fmt.Sprintf("db%d", index), "keys": rows, "nextCursor": strconv.FormatUint(next, 10), "complete": next == 0, "total": total, "warnings": warnings}, nil
}

func redisTTLDisplay(ttl time.Duration) (int64, string) {
	if ttl == -1*time.Nanosecond {
		return -1, "永不过期"
	}
	if ttl == -2*time.Nanosecond {
		return -2, "已过期"
	}
	seconds := int64(ttl / time.Second)
	return seconds, fmt.Sprintf("%d 秒", seconds)
}
