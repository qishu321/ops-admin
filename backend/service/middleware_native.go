package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/go-zookeeper/zk"
	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
	"net"
	"ops-admin/backend/model"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func middlewareInt(p map[string]string, key string, fallback, min, max int) (int, error) {
	v := fallback
	if p[key] != "" {
		n, e := strconv.Atoi(p[key])
		if e != nil {
			return 0, fmt.Errorf("%s 必须是整数", key)
		}
		v = n
	}
	if v < min || v > max {
		return 0, fmt.Errorf("%s 必须在 %d 至 %d 之间", key, min, max)
	}
	return v, nil
}
func runMiddlewareKafka(ctx context.Context, d model.MiddlewareDatasource, c model.AssetCredential, r MiddlewareOperation) (any, error) {
	t := &kafka.Transport{DialTimeout: 5 * time.Second}
	defer t.CloseIdleConnections()
	if d.TLS {
		t.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if d.CredentialID > 0 {
		switch d.AuthType {
		case "scram256", "scram512":
			algorithm := scram.SHA256
			if d.AuthType == "scram512" {
				algorithm = scram.SHA512
			}
			m, e := scram.Mechanism(algorithm, c.Username, c.Password)
			if e != nil {
				return nil, e
			}
			t.SASL = m
		default:
			t.SASL = plain.Mechanism{Username: c.Username, Password: c.Password}
		}
	}
	client := &kafka.Client{Addr: kafka.TCP(d.Endpoints...), Transport: t, Timeout: 10 * time.Second}
	p := r.Params
	if strings.HasPrefix(r.Operation, "topic.") && r.Operation != "topic.create" && r.Operation != "topic.delete" {
		return runKafkaTopic(ctx, client, r)
	}
	switch r.Operation {
	case "topics", "brokers":
		v, e := client.Metadata(ctx, &kafka.MetadataRequest{})
		if e != nil {
			return nil, e
		}
		if r.Operation == "brokers" {
			return v.Brokers, nil
		}
		return v.Topics, nil
	case "groups":
		metadata, e := client.Metadata(ctx, &kafka.MetadataRequest{})
		if e != nil {
			return nil, e
		}
		groups := []kafka.ListGroupsResponseGroup{}
		seen := map[string]bool{}
		for _, broker := range metadata.Brokers {
			v, e := client.ListGroups(ctx, &kafka.ListGroupsRequest{Addr: kafka.TCP(net.JoinHostPort(broker.Host, strconv.Itoa(broker.Port)))})
			if e != nil {
				return nil, e
			}
			if v.Error != nil {
				return nil, v.Error
			}
			for _, g := range v.Groups {
				if !seen[g.GroupID] {
					seen[g.GroupID] = true
					groups = append(groups, g)
				}
			}
		}
		return groups, nil
	case "group.get":
		if e := requiredMiddleware(p, "groupId"); e != nil {
			return nil, e
		}
		v, e := client.DescribeGroups(ctx, &kafka.DescribeGroupsRequest{GroupIDs: []string{p["groupId"]}})
		if e != nil {
			return nil, e
		}
		for _, g := range v.Groups {
			if g.Error != nil {
				return nil, g.Error
			}
		}
		return v.Groups, nil
	case "topic.create":
		if e := requiredMiddleware(p, "name"); e != nil {
			return nil, e
		}
		n, e := middlewareInt(p, "partitions", 1, 1, 10000)
		if e != nil {
			return nil, e
		}
		rep, e := middlewareInt(p, "replicationFactor", 1, 1, 100)
		if e != nil {
			return nil, e
		}
		v, e := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: []kafka.TopicConfig{{Topic: p["name"], NumPartitions: n, ReplicationFactor: rep}}})
		if e != nil {
			return nil, e
		}
		for _, err := range v.Errors {
			if err != nil {
				return nil, err
			}
		}
		return true, nil
	case "topic.delete":
		if e := requiredMiddleware(p, "name"); e != nil {
			return nil, e
		}
		v, e := client.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{Topics: []string{p["name"]}})
		if e != nil {
			return nil, e
		}
		for _, err := range v.Errors {
			if err != nil {
				return nil, err
			}
		}
		return true, nil
	case "group.delete":
		if e := requiredMiddleware(p, "groupId"); e != nil {
			return nil, e
		}
		v, e := client.DeleteGroups(ctx, &kafka.DeleteGroupsRequest{GroupIDs: []string{p["groupId"]}})
		if e != nil {
			return nil, e
		}
		for _, err := range v.Errors {
			if err != nil {
				return nil, err
			}
		}
		return true, nil
	}
	return nil, errors.New("不支持的 Kafka 操作")
}

type quietZKLogger struct{}

func (quietZKLogger) Printf(string, ...interface{}) {}

// 子节点摘要只读取 Stat，不传输节点数据；单次最多枚举 500 个子节点，避免大目录拖慢浏览。
const zkChildSummaryLimit = 500

type zkChildSummary struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	HasChildren    bool   `json:"hasChildren"`
	NumChildren    int32  `json:"numChildren"`
	DataLength     int32  `json:"dataLength"`
	EphemeralOwner int64  `json:"ephemeralOwner"`
	Mtime          int64  `json:"mtime"`
}

func zkChildPath(parent, name string) string {
	if parent == "/" {
		return "/" + name
	}
	return parent + "/" + name
}

func zkChildSummaries(conn *zk.Conn, parent string, children []string) ([]zkChildSummary, bool) {
	list, truncated := children, false
	if len(list) > zkChildSummaryLimit {
		list, truncated = list[:zkChildSummaryLimit], true
	}
	out := make([]zkChildSummary, len(list))
	sem := make(chan struct{}, 12)
	var wg sync.WaitGroup
	for i, name := range list {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			item := zkChildSummary{Name: name, Path: zkChildPath(parent, name)}
			if ok, stat, err := conn.Exists(item.Path); err == nil && ok && stat != nil {
				item.HasChildren = stat.NumChildren > 0
				item.NumChildren = stat.NumChildren
				item.DataLength = stat.DataLength
				item.EphemeralOwner = stat.EphemeralOwner
				item.Mtime = stat.Mtime
			}
			out[i] = item
		}(i, name)
	}
	wg.Wait()
	return out, truncated
}

func runMiddlewareZK(ctx context.Context, d model.MiddlewareDatasource, c model.AssetCredential, r MiddlewareOperation) (any, error) {
	conn, events, e := zk.Connect(d.Endpoints, 8*time.Second, zk.WithLogger(quietZKLogger{}))
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	// Closing on deadline also interrupts operations waiting on a disconnected server.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	connected := false
	for !connected {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil, errors.New("ZooKeeper 连接已关闭")
			}
			if ev.State == zk.StateHasSession {
				connected = true
			}
			if ev.State == zk.StateAuthFailed {
				return nil, errors.New("ZooKeeper 认证失败")
			}
		}
	}
	if d.CredentialID > 0 {
		if e := conn.AddAuth("digest", []byte(c.Username+":"+c.Password)); e != nil {
			return nil, e
		}
	}
	p := r.Params
	node := p["path"]
	if node == "" {
		node = "/"
	}
	if !strings.HasPrefix(node, "/") || path.Clean(node) != node {
		return nil, errors.New("节点路径必须是规范的绝对路径")
	}
	switch r.Operation {
	case "children":
		children, stat, e := conn.Children(node)
		if e != nil {
			return nil, e
		}
		slices.Sort(children)
		payload := map[string]any{"children": children, "stat": stat, "path": node}
		// detail=1 附带子节点摘要，供树形目录区分叶子与可展开节点；未展开的节点不额外请求。
		if p["detail"] == "1" {
			items, truncated := zkChildSummaries(conn, node, children)
			payload["items"] = items
			payload["truncated"] = truncated
		}
		return payload, nil
	case "node.get":
		data, stat, e := conn.Get(node)
		if e != nil {
			return nil, e
		}
		acl, _, e := conn.GetACL(node)
		return map[string]any{"path": node, "value": string(data), "stat": stat, "acl": acl}, e
	case "node.create":
		if node == "/" {
			return nil, errors.New("不能创建根节点")
		}
		acl, _, e := conn.GetACL(path.Dir(node))
		if e != nil {
			return nil, e
		}
		return conn.Create(node, []byte(p["value"]), 0, acl)
	case "node.put", "node.delete":
		if node == "/" && r.Operation == "node.delete" {
			return nil, errors.New("不能删除根节点")
		}
		ver, e := middlewareInt(p, "version", -2, 0, 2147483647)
		if e != nil {
			return nil, errors.New("请先读取节点并提供当前 version，避免覆盖其他人的变更")
		}
		if r.Operation == "node.put" {
			return conn.Set(node, []byte(p["value"]), int32(ver))
		}
		return true, conn.Delete(node, int32(ver))
	}
	return nil, errors.New("不支持的 ZooKeeper 操作")
}
