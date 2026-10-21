package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	kafka "github.com/segmentio/kafka-go"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Kept narrow enough to exercise protocol outcomes without a production broker.
type kafkaTopicAPI interface {
	Metadata(context.Context, *kafka.MetadataRequest) (*kafka.MetadataResponse, error)
	ListOffsets(context.Context, *kafka.ListOffsetsRequest) (*kafka.ListOffsetsResponse, error)
	Fetch(context.Context, *kafka.FetchRequest) (*kafka.FetchResponse, error)
	Produce(context.Context, *kafka.ProduceRequest) (*kafka.ProduceResponse, error)
	DescribeConfigs(context.Context, *kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error)
	IncrementalAlterConfigs(context.Context, *kafka.IncrementalAlterConfigsRequest) (*kafka.IncrementalAlterConfigsResponse, error)
	CreatePartitions(context.Context, *kafka.CreatePartitionsRequest) (*kafka.CreatePartitionsResponse, error)
	CreateTopics(context.Context, *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error)
	DeleteTopics(context.Context, *kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error)
	ListGroups(context.Context, *kafka.ListGroupsRequest) (*kafka.ListGroupsResponse, error)
	DescribeGroups(context.Context, *kafka.DescribeGroupsRequest) (*kafka.DescribeGroupsResponse, error)
	OffsetFetch(context.Context, *kafka.OffsetFetchRequest) (*kafka.OffsetFetchResponse, error)
}
type kafkaTopicPartition struct {
	ID          int    `json:"id"`
	Leader      int    `json:"leader"`
	Replicas    []int  `json:"replicas"`
	ISR         []int  `json:"isr"`
	FirstOffset string `json:"firstOffset"`
	NextOffset  string `json:"nextOffset"`
	OffsetSpan  string `json:"offsetSpan"`
	Error       string `json:"error,omitempty"`
}
type kafkaTopicOverview struct {
	Name            string                `json:"name"`
	Internal        bool                  `json:"internal"`
	Partitions      []kafkaTopicPartition `json:"partitions"`
	OffsetSpan      string                `json:"offsetSpan"`
	UnderReplicated int                   `json:"underReplicated"`
	ISRCount        int                   `json:"isrCount"`
	ReplicaCount    int                   `json:"replicaCount"`
	FetchedAt       time.Time             `json:"fetchedAt"`
}

func kafkaTopicMetadata(ctx context.Context, c kafkaTopicAPI, name string) (kafka.Topic, error) {
	v, e := c.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{name}})
	if e != nil {
		return kafka.Topic{}, e
	}
	for _, t := range v.Topics {
		if t.Name == name {
			if t.Error != nil {
				return t, t.Error
			}
			return t, nil
		}
	}
	return kafka.Topic{}, errors.New("Topic 不存在或没有访问权限")
}
func kafkaTopicOffsets(ctx context.Context, c kafkaTopicAPI, name string, partitions []kafka.Partition) (map[int]kafka.PartitionOffsets, error) {
	reqs := []kafka.OffsetRequest{}
	for _, p := range partitions {
		reqs = append(reqs, kafka.FirstOffsetOf(p.ID), kafka.LastOffsetOf(p.ID))
	}
	v, e := c.ListOffsets(ctx, &kafka.ListOffsetsRequest{Topics: map[string][]kafka.OffsetRequest{name: reqs}, IsolationLevel: kafka.ReadCommitted})
	if e != nil {
		return nil, e
	}
	result := map[int]kafka.PartitionOffsets{}
	for _, p := range v.Topics[name] {
		result[p.Partition] = p
	}
	return result, nil
}
func kafkaOverview(ctx context.Context, c kafkaTopicAPI, name string) (kafkaTopicOverview, error) {
	result := kafkaTopicOverview{Name: name, Partitions: []kafkaTopicPartition{}, FetchedAt: time.Now()}
	topic, e := kafkaTopicMetadata(ctx, c, name)
	if e != nil {
		return result, e
	}
	result.Internal = topic.Internal
	offsets, e := kafkaTopicOffsets(ctx, c, name, topic.Partitions)
	if e != nil {
		return result, e
	}
	var total int64
	for _, p := range topic.Partitions {
		row := kafkaTopicPartition{ID: p.ID, Leader: p.Leader.ID, Replicas: []int{}, ISR: []int{}, FirstOffset: "—", NextOffset: "—", OffsetSpan: "—"}
		for _, b := range p.Replicas {
			row.Replicas = append(row.Replicas, b.ID)
		}
		for _, b := range p.Isr {
			row.ISR = append(row.ISR, b.ID)
		}
		result.ReplicaCount += len(row.Replicas)
		result.ISRCount += len(row.ISR)
		if len(row.ISR) < len(row.Replicas) {
			result.UnderReplicated++
		}
		if p.Error != nil {
			row.Error = p.Error.Error()
		} else if off, ok := offsets[p.ID]; !ok {
			row.Error = "未返回分区 Offset"
		} else if off.Error != nil {
			row.Error = off.Error.Error()
		} else if off.FirstOffset < 0 || off.LastOffset < 0 {
			row.Error = "分区 Offset 不可用"
		} else {
			span := max(int64(0), off.LastOffset-off.FirstOffset)
			row.FirstOffset = strconv.FormatInt(off.FirstOffset, 10)
			row.NextOffset = strconv.FormatInt(off.LastOffset, 10)
			row.OffsetSpan = strconv.FormatInt(span, 10)
			total += span
		}
		result.Partitions = append(result.Partitions, row)
	}
	sort.Slice(result.Partitions, func(i, j int) bool { return result.Partitions[i].ID < result.Partitions[j].ID })
	result.OffsetSpan = strconv.FormatInt(total, 10)
	return result, nil
}
func runKafkaTopic(ctx context.Context, c kafkaTopicAPI, r MiddlewareOperation) (any, error) {
	p := r.Params
	if e := requiredMiddleware(p, "name"); e != nil {
		return nil, e
	}
	name := p["name"]
	if len(name) > 249 || strings.ContainsAny(name, "/\x00") {
		return nil, errors.New("Topic 名称不正确")
	}
	switch r.Operation {
	case "topic.overview":
		return kafkaOverview(ctx, c, name)
	case "topic.messages":
		return kafkaMessages(ctx, c, name, p)
	case "topic.consumers":
		return kafkaConsumers(ctx, c, name, p)
	case "topic.configs":
		return kafkaConfigs(ctx, c, name)
	case "topic.produce":
		return kafkaProduce(ctx, c, name, p)
	case "topic.config.set":
		if e := requiredMiddleware(p, "key"); e != nil {
			return nil, e
		}
		entries, e := kafkaConfigs(ctx, c, name)
		if e != nil {
			return nil, e
		}
		found := false
		for _, entry := range entries {
			if entry.Name == p["key"] {
				found = true
				if entry.ReadOnly || entry.Sensitive {
					return nil, errors.New("该配置只读或敏感，不能在此修改")
				}
			}
		}
		if !found {
			return nil, errors.New("配置项不存在")
		}
		operation := kafka.ConfigOperationSet
		if p["reset"] == "true" {
			operation = kafka.ConfigOperationDelete
		}
		v, e := c.IncrementalAlterConfigs(ctx, &kafka.IncrementalAlterConfigsRequest{Resources: []kafka.IncrementalAlterConfigsRequestResource{{ResourceType: kafka.ResourceTypeTopic, ResourceName: name, Configs: []kafka.IncrementalAlterConfigsRequestConfig{{Name: p["key"], Value: p["value"], ConfigOperation: operation}}}}})
		if e != nil {
			return nil, e
		}
		if len(v.Resources) == 0 {
			return nil, errors.New("Broker 未返回配置变更结果")
		}
		for _, resource := range v.Resources {
			if resource.Error != nil {
				return nil, resource.Error
			}
		}
		return true, nil
	case "topic.clear":
		return kafkaClearTopic(ctx, c, name)
	case "topic.partitions":
		n, e := middlewareInt(p, "partitions", 0, 1, 10000)
		if e != nil {
			return nil, e
		}
		topic, e := kafkaTopicMetadata(ctx, c, name)
		if e != nil {
			return nil, e
		}
		if n <= len(topic.Partitions) {
			return nil, errors.New("分区只能增加，新数量必须大于当前分区数")
		}
		v, e := c.CreatePartitions(ctx, &kafka.CreatePartitionsRequest{Topics: []kafka.TopicPartitionsConfig{{Name: name, Count: int32(n)}}})
		if e != nil {
			return nil, e
		}
		if len(v.Errors) == 0 {
			return nil, errors.New("Broker 未返回扩容结果")
		}
		for _, err := range v.Errors {
			if err != nil {
				return nil, err
			}
		}
		return true, nil
	}
	return nil, errors.New("不支持的 Topic 操作")
}

type kafkaTopicConfig struct {
	Name          string `json:"name"`
	Value         string `json:"value"`
	ReadOnly      bool   `json:"readOnly"`
	Sensitive     bool   `json:"sensitive"`
	Source        int8   `json:"source"`
	Documentation string `json:"documentation"`
}

func kafkaConfigs(ctx context.Context, c kafkaTopicAPI, name string) ([]kafkaTopicConfig, error) {
	v, e := c.DescribeConfigs(ctx, &kafka.DescribeConfigsRequest{Resources: []kafka.DescribeConfigRequestResource{{ResourceType: kafka.ResourceTypeTopic, ResourceName: name}}, IncludeDocumentation: true})
	if e != nil {
		return nil, e
	}
	rows := []kafkaTopicConfig{}
	if len(v.Resources) == 0 {
		return nil, errors.New("Broker 未返回 Topic 配置")
	}
	for _, res := range v.Resources {
		if res.Error != nil {
			return nil, res.Error
		}
		for _, entry := range res.ConfigEntries {
			value := entry.ConfigValue
			if entry.IsSensitive {
				value = ""
			}
			rows = append(rows, kafkaTopicConfig{Name: entry.ConfigName, Value: value, ReadOnly: entry.ReadOnly, Sensitive: entry.IsSensitive, Source: entry.ConfigSource, Documentation: entry.ConfigDocumentation})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

type kafkaClearResult struct {
	Partitions        int      `json:"partitions"`
	ReplicationFactor int      `json:"replicationFactor"`
	RestoredConfigs   int      `json:"restoredConfigs"`
	Warnings          []string `json:"warnings"`
}

// Kafka 没有清空 Topic 的原子接口：这里用「删除 + 按原分区数与副本因子重建」实现，
// 重建后重新写入原有的 Topic 级配置覆盖值。消息与消费位点都会随之失效。
func kafkaClearTopic(ctx context.Context, c kafkaTopicAPI, name string) (any, error) {
	topic, e := kafkaTopicMetadata(ctx, c, name)
	if e != nil {
		return nil, e
	}
	if topic.Internal {
		return nil, errors.New("内部 Topic 由集群自身维护，不能清空")
	}
	if len(topic.Partitions) == 0 {
		return nil, errors.New("Topic 没有可读取的分区，无法清空")
	}
	partitions := len(topic.Partitions)
	replication := len(topic.Partitions[0].Replicas)
	if replication < 1 {
		replication = 1
	}
	configs, e := kafkaConfigs(ctx, c, name)
	if e != nil {
		return nil, e
	}
	overrides := []kafkaTopicConfig{}
	for _, cfg := range configs {
		// ConfigSource 1 表示 Topic 级覆盖值；只读、敏感项不写回。
		if cfg.Source == 1 && !cfg.ReadOnly && !cfg.Sensitive && strings.TrimSpace(cfg.Value) != "" {
			overrides = append(overrides, cfg)
		}
	}

	deleted, e := c.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{Topics: []string{name}})
	if e != nil {
		return nil, fmt.Errorf("提交 Topic 删除失败：%w", e)
	}
	if err := deleted.Errors[name]; err != nil {
		return nil, fmt.Errorf("删除 Topic 失败：%w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		md, err := c.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{name}})
		gone := err != nil
		if err == nil {
			gone = true
			for _, t := range md.Topics {
				if t.Name == name && t.Error == nil {
					gone = false
				}
			}
		}
		if gone {
			break
		}
		if time.Now().After(deadline) {
			return nil, errors.New("Topic 已提交删除，但 30 秒内 Broker 仍未完成，请稍后检查 Topic 状态")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	created, e := c.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: []kafka.TopicConfig{{Topic: name, NumPartitions: partitions, ReplicationFactor: replication}}})
	if e != nil {
		return nil, fmt.Errorf("Topic 已删除，但重建失败：%w", e)
	}
	if err := created.Errors[name]; err != nil {
		return nil, fmt.Errorf("Topic 已删除，但重建失败：%w", err)
	}

	result := kafkaClearResult{Partitions: partitions, ReplicationFactor: replication, Warnings: []string{}}
	for _, cfg := range overrides {
		v, err := c.IncrementalAlterConfigs(ctx, &kafka.IncrementalAlterConfigsRequest{Resources: []kafka.IncrementalAlterConfigsRequestResource{{ResourceType: kafka.ResourceTypeTopic, ResourceName: name, Configs: []kafka.IncrementalAlterConfigsRequestConfig{{Name: cfg.Name, Value: cfg.Value, ConfigOperation: kafka.ConfigOperationSet}}}}})
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("恢复配置 %s 失败：%v", cfg.Name, err))
			continue
		}
		restored := false
		for _, res := range v.Resources {
			if res.Error != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("恢复配置 %s 失败：%v", cfg.Name, res.Error))
				continue
			}
			restored = true
		}
		if restored {
			result.RestoredConfigs++
		}
	}
	return result, nil
}

type kafkaMessageBytes struct {
	Text     string `json:"text"`
	Base64   string `json:"base64"`
	Encoding string `json:"encoding"`
	Null     bool   `json:"null"`
	Size     int    `json:"size"`
}

func kafkaBytes(raw []byte) kafkaMessageBytes {
	v := kafkaMessageBytes{Null: raw == nil, Size: len(raw), Base64: base64.StdEncoding.EncodeToString(raw), Encoding: "utf8"}
	if utf8.Valid(raw) {
		v.Text = string(raw)
	} else {
		v.Encoding = "base64"
		v.Text = v.Base64
	}
	return v
}

type kafkaTopicMessage struct {
	Partition int               `json:"partition"`
	Offset    string            `json:"offset"`
	Timestamp time.Time         `json:"timestamp"`
	Key       kafkaMessageBytes `json:"key"`
	Value     kafkaMessageBytes `json:"value"`
	Headers   []map[string]any  `json:"headers"`
}

func closeKafkaRecords(v kafka.RecordReader) {
	if closer, ok := v.(io.Closer); ok {
		_ = closer.Close()
	}
}
func readKafkaBytes(v kafka.Bytes) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	defer v.Close()
	return io.ReadAll(io.LimitReader(v, (1<<20)+1))
}
func kafkaMessages(ctx context.Context, c kafkaTopicAPI, name string, p map[string]string) (any, error) {
	partition, e := middlewareInt(p, "partition", 0, 0, 2147483647)
	if e != nil {
		return nil, e
	}
	limit, e := middlewareInt(p, "limit", 50, 1, 200)
	if e != nil {
		return nil, e
	}
	topic, e := kafkaTopicMetadata(ctx, c, name)
	if e != nil {
		return nil, e
	}
	var selected []kafka.Partition
	for _, v := range topic.Partitions {
		if v.ID == partition {
			selected = []kafka.Partition{v}
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("分区不存在")
	}
	offsets, e := kafkaTopicOffsets(ctx, c, name, selected)
	if e != nil {
		return nil, e
	}
	off, ok := offsets[partition]
	if !ok {
		return nil, errors.New("未返回分区 Offset")
	}
	if off.Error != nil {
		return nil, off.Error
	}
	if off.FirstOffset < 0 || off.LastOffset < 0 {
		return nil, errors.New("分区 Offset 不可用")
	}
	start := off.FirstOffset
	mode := p["mode"]
	switch mode {
	case "", "latest":
		start = max(off.FirstOffset, off.LastOffset-int64(limit))
	case "earliest":
	case "offset":
		start, e = strconv.ParseInt(p["offset"], 10, 64)
		if e != nil || start < 0 {
			return nil, errors.New("Offset 必须是非负整数")
		}
		if start < off.FirstOffset {
			return nil, fmt.Errorf("该 Offset 已过期，当前最早 Offset 为 %d", off.FirstOffset)
		}
	case "time":
		at, e := time.Parse(time.RFC3339Nano, p["timestamp"])
		if e != nil {
			return nil, errors.New("时间格式不正确")
		}
		v, e := c.ListOffsets(ctx, &kafka.ListOffsetsRequest{Topics: map[string][]kafka.OffsetRequest{name: {kafka.TimeOffsetOf(partition, at)}}, IsolationLevel: kafka.ReadCommitted})
		if e != nil {
			return nil, e
		}
		start = off.LastOffset
		for _, v := range v.Topics[name] {
			if v.Error != nil {
				return nil, v.Error
			}
			for value := range v.Offsets {
				if value >= 0 {
					start = value
				}
			}
		}
	default:
		return nil, errors.New("不支持的消息定位方式")
	}
	rows := []kafkaTopicMessage{}
	next := start
	bytesRead := 0
	truncated := false
	// Direct Fetch never joins a consumer group or commits application offsets.
	for len(rows) < limit && next < off.LastOffset && bytesRead < 4<<20 {
		v, e := c.Fetch(ctx, &kafka.FetchRequest{Topic: name, Partition: partition, Offset: next, MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 300 * time.Millisecond, IsolationLevel: kafka.ReadCommitted})
		if e != nil {
			return nil, e
		}
		if v.Records == nil {
			if v.Error != nil {
				return nil, v.Error
			}
			break
		}
		if v.Error != nil {
			closeKafkaRecords(v.Records)
			return nil, v.Error
		}
		before := next
		readErr := func() error {
			defer closeKafkaRecords(v.Records)
			for len(rows) < limit {
				record, err := v.Records.ReadRecord()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				if record.Offset < next {
					continue
				}
				key, err := readKafkaBytes(record.Key)
				if err != nil {
					return err
				}
				value, err := readKafkaBytes(record.Value)
				if err != nil {
					return err
				}
				size := len(key) + len(value)
				for _, h := range record.Headers {
					size += len(h.Key) + len(h.Value)
				}
				if len(key) > 1<<20 || len(value) > 1<<20 || size > 1<<20 {
					return errors.New("单条消息超过 1 MiB 展示限制，请使用专用客户端读取")
				}
				if bytesRead+size > 4<<20 {
					truncated = true
					break
				}
				bytesRead += size
				headers := []map[string]any{}
				for _, h := range record.Headers {
					headers = append(headers, map[string]any{"key": h.Key, "value": kafkaBytes(h.Value)})
				}
				rows = append(rows, kafkaTopicMessage{Partition: partition, Offset: strconv.FormatInt(record.Offset, 10), Timestamp: record.Time, Key: kafkaBytes(key), Value: kafkaBytes(value), Headers: headers})
				next = record.Offset + 1
			}
			return nil
		}()
		if readErr != nil {
			return nil, readErr
		}
		if next == before || truncated {
			break
		}
	}
	return map[string]any{"items": rows, "nextOffset": strconv.FormatInt(next, 10), "firstOffset": strconv.FormatInt(off.FirstOffset, 10), "endOffset": strconv.FormatInt(off.LastOffset, 10), "hasMore": next < off.LastOffset, "truncated": truncated}, nil
}
func decodeKafkaInput(value, encoding string) ([]byte, error) {
	if encoding == "base64" {
		v, e := base64.StdEncoding.DecodeString(value)
		if e != nil {
			return nil, errors.New("Base64 内容不正确")
		}
		return v, nil
	}
	if encoding != "" && encoding != "utf8" {
		return nil, errors.New("消息编码不支持")
	}
	return []byte(value), nil
}
func kafkaProduce(ctx context.Context, c kafkaTopicAPI, name string, p map[string]string) (any, error) {
	partition, e := middlewareInt(p, "partition", 0, 0, 2147483647)
	if e != nil {
		return nil, e
	}
	topic, e := kafkaTopicMetadata(ctx, c, name)
	if e != nil {
		return nil, e
	}
	valid := false
	for _, v := range topic.Partitions {
		valid = valid || v.ID == partition
	}
	if !valid {
		return nil, errors.New("分区不存在")
	}
	key, e := decodeKafkaInput(p["key"], p["keyEncoding"])
	if e != nil {
		return nil, e
	}
	value, e := decodeKafkaInput(p["value"], p["valueEncoding"])
	if e != nil {
		return nil, e
	}
	if p["nullKey"] == "true" {
		key = nil
	}
	if p["tombstone"] == "true" {
		value = nil
	}
	headers := []kafka.Header{}
	if p["headers"] != "" {
		var input []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal([]byte(p["headers"]), &input) != nil || len(input) > 100 {
			return nil, errors.New("Headers 请填写最多 100 项的 JSON 数组，如 [{\"key\":\"trace-id\",\"value\":\"123\"}]")
		}
		for _, h := range input {
			headers = append(headers, kafka.Header{Key: h.Key, Value: []byte(h.Value)})
		}
	}
	size := len(key) + len(value)
	for _, h := range headers {
		size += len(h.Key) + len(h.Value)
	}
	if size > 1<<20 {
		return nil, errors.New("消息内容与 Headers 合计不能超过 1 MiB")
	}
	var recordKey, recordValue kafka.Bytes
	if key != nil {
		recordKey = kafka.NewBytes(key)
	}
	if value != nil {
		recordValue = kafka.NewBytes(value)
	}
	v, e := c.Produce(ctx, &kafka.ProduceRequest{Topic: name, Partition: partition, RequiredAcks: kafka.RequireAll, Records: kafka.NewRecordReader(kafka.Record{Key: recordKey, Value: recordValue, Headers: headers, Time: time.Now()})})
	if e != nil {
		return nil, fmt.Errorf("发送结果未确认：%w；请先查询消息，避免重复发送", e)
	}
	if v.Error != nil {
		return nil, v.Error
	}
	for _, err := range v.RecordErrors {
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"partition": partition, "offset": strconv.FormatInt(v.BaseOffset, 10)}, nil
}

type kafkaConsumerPartition struct {
	ID              int    `json:"id"`
	CommittedOffset string `json:"committedOffset"`
	EndOffset       string `json:"endOffset"`
	Lag             string `json:"lag"`
	Error           string `json:"error,omitempty"`
}
type kafkaTopicConsumer struct {
	GroupID     string                   `json:"groupId"`
	State       string                   `json:"state"`
	Members     int                      `json:"members"`
	Coordinator int                      `json:"coordinator"`
	Lag         string                   `json:"lag"`
	Partitions  []kafkaConsumerPartition `json:"partitions"`
	Clients     []map[string]any         `json:"clients"`
	Error       string                   `json:"error,omitempty"`
}

func kafkaConsumers(ctx context.Context, c kafkaTopicAPI, name string, p map[string]string) (any, error) {
	topic, e := kafkaTopicMetadata(ctx, c, name)
	if e != nil {
		return nil, e
	}
	offsets, e := kafkaTopicOffsets(ctx, c, name, topic.Partitions)
	if e != nil {
		return nil, e
	}
	metadata, e := c.Metadata(ctx, &kafka.MetadataRequest{})
	if e != nil {
		return nil, e
	}
	groups := []kafka.ListGroupsResponseGroup{}
	seen := map[string]bool{}
	for _, b := range metadata.Brokers {
		v, e := c.ListGroups(ctx, &kafka.ListGroupsRequest{Addr: kafka.TCP(net.JoinHostPort(b.Host, strconv.Itoa(b.Port)))})
		if e != nil {
			return nil, e
		}
		if v.Error != nil {
			return nil, v.Error
		}
		for _, g := range v.Groups {
			if !seen[g.GroupID] && (p["groupFilter"] == "" || strings.Contains(strings.ToLower(g.GroupID), strings.ToLower(p["groupFilter"]))) {
				groups = append(groups, g)
				seen[g.GroupID] = true
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupID < groups[j].GroupID })
	start, e := middlewareInt(p, "scanOffset", 0, 0, len(groups))
	if e != nil {
		return nil, e
	}
	end := min(start+100, len(groups))
	ids := []int{}
	for _, v := range topic.Partitions {
		ids = append(ids, v.ID)
	}
	rows := []kafkaTopicConsumer{}
	warnings := []string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, g := range groups[start:end] {
		wg.Add(1)
		go func(g kafka.ListGroupsResponseGroup) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				warnings = append(warnings, g.GroupID+": 查询超时")
				mu.Unlock()
				return
			}
			row, related, err := kafkaConsumer(ctx, c, name, g, ids, offsets)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				warnings = append(warnings, g.GroupID+": "+err.Error())
			}
			if related {
				rows = append(rows, row)
			}
		}(g)
	}
	wg.Wait()
	sort.Slice(rows, func(i, j int) bool { return rows[i].GroupID < rows[j].GroupID })
	return map[string]any{"items": rows, "warnings": warnings, "scanned": end, "totalGroups": len(groups), "nextScanOffset": end, "hasMore": end < len(groups)}, nil
}
func kafkaConsumer(ctx context.Context, c kafkaTopicAPI, name string, g kafka.ListGroupsResponseGroup, ids []int, ends map[int]kafka.PartitionOffsets) (kafkaTopicConsumer, bool, error) {
	row := kafkaTopicConsumer{GroupID: g.GroupID, Coordinator: g.Coordinator, Lag: "0", Partitions: []kafkaConsumerPartition{}, Clients: []map[string]any{}}
	v, e := c.OffsetFetch(ctx, &kafka.OffsetFetchRequest{GroupID: g.GroupID, Topics: map[string][]int{name: ids}})
	if e != nil {
		return row, false, e
	}
	if v.Error != nil {
		return row, false, v.Error
	}
	related := false
	unknown := false
	var total int64
	for _, p := range v.Topics[name] {
		part := kafkaConsumerPartition{ID: p.Partition, CommittedOffset: strconv.FormatInt(p.CommittedOffset, 10), EndOffset: "—", Lag: "—"}
		if p.Error != nil {
			part.Error = p.Error.Error()
			unknown = true
		}
		if p.CommittedOffset >= 0 {
			related = true
		}
		end, ok := ends[p.Partition]
		if ok && end.Error == nil && end.LastOffset >= 0 {
			part.EndOffset = strconv.FormatInt(end.LastOffset, 10)
			if p.CommittedOffset >= 0 && p.Error == nil {
				lag := max(int64(0), end.LastOffset-p.CommittedOffset)
				part.Lag = strconv.FormatInt(lag, 10)
				total += lag
			} else {
				unknown = true
			}
		} else {
			unknown = true
			if ok && end.Error != nil {
				part.Error = end.Error.Error()
			}
		}
		row.Partitions = append(row.Partitions, part)
	}
	details, e := c.DescribeGroups(ctx, &kafka.DescribeGroupsRequest{GroupIDs: []string{g.GroupID}})
	if e != nil {
		row.Error = e.Error()
		if related {
			return row, true, e
		}
		return row, false, e
	}
	for _, group := range details.Groups {
		if group.Error != nil {
			row.Error = group.Error.Error()
			return row, related, group.Error
		}
		row.State = group.GroupState
		for _, member := range group.Members {
			isRelated := false
			for _, assigned := range member.MemberAssignments.Topics {
				if assigned.Topic == name {
					isRelated = true
				}
			}
			for _, t := range member.MemberMetadata.Topics {
				if t == name {
					related = true
				}
			}
			if isRelated {
				related = true
				row.Members++
				row.Clients = append(row.Clients, map[string]any{"memberId": member.MemberID, "clientId": member.ClientID, "host": member.ClientHost})
			}
		}
	}
	if len(row.Partitions) == 0 {
		unknown = true
	}
	if unknown {
		row.Lag = "—"
	} else {
		row.Lag = strconv.FormatInt(total, 10)
	}
	sort.Slice(row.Partitions, func(i, j int) bool { return row.Partitions[i].ID < row.Partitions[j].ID })
	return row, related, nil
}
