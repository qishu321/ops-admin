package service

import (
	"context"
	"encoding/json"
	"errors"
	kafka "github.com/segmentio/kafka-go"
	"io"
	"strings"
	"testing"
	"time"
)

type topicFake struct {
	kafkaTopicAPI
	metadata    func(*kafka.MetadataRequest) (*kafka.MetadataResponse, error)
	offsets     func(*kafka.ListOffsetsRequest) (*kafka.ListOffsetsResponse, error)
	fetch       func(*kafka.FetchRequest) (*kafka.FetchResponse, error)
	produce     func(*kafka.ProduceRequest) (*kafka.ProduceResponse, error)
	configs     func(*kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error)
	alter       func(*kafka.IncrementalAlterConfigsRequest) (*kafka.IncrementalAlterConfigsResponse, error)
	create      func(*kafka.CreatePartitionsRequest) (*kafka.CreatePartitionsResponse, error)
	createTopic func(*kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error)
	deleteTopic func(*kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error)
	describe    func(*kafka.DescribeGroupsRequest) (*kafka.DescribeGroupsResponse, error)
	committed   func(*kafka.OffsetFetchRequest) (*kafka.OffsetFetchResponse, error)
}

func (f topicFake) Metadata(_ context.Context, r *kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
	return f.metadata(r)
}
func (f topicFake) ListOffsets(_ context.Context, r *kafka.ListOffsetsRequest) (*kafka.ListOffsetsResponse, error) {
	return f.offsets(r)
}
func (f topicFake) Fetch(_ context.Context, r *kafka.FetchRequest) (*kafka.FetchResponse, error) {
	return f.fetch(r)
}
func (f topicFake) Produce(_ context.Context, r *kafka.ProduceRequest) (*kafka.ProduceResponse, error) {
	return f.produce(r)
}
func (f topicFake) DescribeConfigs(_ context.Context, r *kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error) {
	return f.configs(r)
}
func (f topicFake) IncrementalAlterConfigs(_ context.Context, r *kafka.IncrementalAlterConfigsRequest) (*kafka.IncrementalAlterConfigsResponse, error) {
	return f.alter(r)
}
func (f topicFake) CreatePartitions(_ context.Context, r *kafka.CreatePartitionsRequest) (*kafka.CreatePartitionsResponse, error) {
	return f.create(r)
}
func (f topicFake) CreateTopics(_ context.Context, r *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
	if f.createTopic == nil {
		return nil, errors.New("unexpected CreateTopics call")
	}
	return f.createTopic(r)
}
func (f topicFake) DeleteTopics(_ context.Context, r *kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error) {
	if f.deleteTopic == nil {
		return nil, errors.New("unexpected DeleteTopics call")
	}
	return f.deleteTopic(r)
}
func (f topicFake) DescribeGroups(_ context.Context, r *kafka.DescribeGroupsRequest) (*kafka.DescribeGroupsResponse, error) {
	return f.describe(r)
}
func (f topicFake) OffsetFetch(_ context.Context, r *kafka.OffsetFetchRequest) (*kafka.OffsetFetchResponse, error) {
	return f.committed(r)
}
func basicTopicFake() topicFake {
	return topicFake{
		metadata: func(r *kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
			return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Partitions: []kafka.Partition{{ID: 0, Leader: kafka.Broker{ID: 1}, Replicas: []kafka.Broker{{ID: 1}, {ID: 2}}, Isr: []kafka.Broker{{ID: 1}}}}}}}, nil
		},
		offsets: func(r *kafka.ListOffsetsRequest) (*kafka.ListOffsetsResponse, error) {
			return &kafka.ListOffsetsResponse{Topics: map[string][]kafka.PartitionOffsets{"orders": {{Partition: 0, FirstOffset: 9007199254741000, LastOffset: 9007199254741010}}}}, nil
		},
	}
}
func TestKafkaTopicOverviewPreservesOffsetsAndISR(t *testing.T) {
	f := basicTopicFake()
	v, e := kafkaOverview(context.Background(), f, "orders")
	if e != nil {
		t.Fatal(e)
	}
	if v.UnderReplicated != 1 || v.ISRCount != 1 || v.ReplicaCount != 2 || v.OffsetSpan != "10" || v.Partitions[0].FirstOffset != "9007199254741000" {
		t.Fatalf("bad overview: %+v", v)
	}
	raw, _ := json.Marshal(v)
	var data map[string]any
	json.Unmarshal(raw, &data)
	if data["partitions"].([]any)[0].(map[string]any)["nextOffset"] != "9007199254741010" {
		t.Fatal("offset precision lost")
	}
	f.metadata = func(*kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
		return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Error: kafka.TopicAuthorizationFailed}}}, nil
	}
	if _, e = kafkaOverview(context.Background(), f, "orders"); !errors.Is(e, kafka.TopicAuthorizationFailed) {
		t.Fatal("topic authorization error hidden", e)
	}
}

type trackedRecords struct {
	kafka.RecordReader
	closed bool
}

func (r *trackedRecords) Close() error { r.closed = true; return nil }
func TestKafkaTopicMessagesCursorBinaryAndTombstone(t *testing.T) {
	f := basicTopicFake()
	start := int64(9007199254741006)
	records := &trackedRecords{RecordReader: kafka.NewRecordReader(
		kafka.Record{Offset: start - 1, Value: kafka.NewBytes([]byte("skip old batch prefix"))},
		kafka.Record{Offset: start, Key: kafka.NewBytes([]byte{0xff}), Value: nil},
		kafka.Record{Offset: start + 2, Value: kafka.NewBytes([]byte(`{"ok":true}`)), Headers: []kafka.Header{{Key: "trace", Value: []byte("123")}}},
	)}
	f.fetch = func(r *kafka.FetchRequest) (*kafka.FetchResponse, error) {
		if r.Offset != start || r.IsolationLevel != kafka.ReadCommitted || r.MaxBytes != 1<<20 {
			t.Fatalf("bad fetch %+v", r)
		}
		return &kafka.FetchResponse{Records: records}, nil
	}
	result, e := kafkaMessages(context.Background(), f, "orders", map[string]string{"partition": "0", "mode": "offset", "offset": "9007199254741006", "limit": "2"})
	if e != nil {
		t.Fatal(e)
	}
	v := result.(map[string]any)
	rows := v["items"].([]kafkaTopicMessage)
	if len(rows) != 2 || rows[0].Offset != "9007199254741006" || !rows[0].Value.Null || rows[0].Key.Encoding != "base64" || rows[0].Key.Text != "/w==" || v["nextOffset"] != "9007199254741009" || !records.closed {
		t.Fatalf("bad message result %+v", v)
	}
	if v["hasMore"] != true {
		t.Fatal("pagination lost")
	}
}
func TestKafkaMessagesRejectExpiredOffsetsAndInvalidLimits(t *testing.T) {
	f := basicTopicFake()
	f.fetch = func(*kafka.FetchRequest) (*kafka.FetchResponse, error) {
		t.Fatal("must reject before fetch")
		return nil, nil
	}
	for _, p := range []map[string]string{{"mode": "offset", "offset": "1"}, {"limit": "201"}, {"partition": "7"}, {"mode": "time", "timestamp": "bad"}} {
		if _, e := kafkaMessages(context.Background(), f, "orders", p); e == nil {
			t.Fatal("invalid message request accepted", p)
		}
	}
}
func TestKafkaProduceConfirmsBrokerErrorsAndPayload(t *testing.T) {
	f := basicTopicFake()
	called := false
	f.produce = func(r *kafka.ProduceRequest) (*kafka.ProduceResponse, error) {
		called = true
		if r.RequiredAcks != kafka.RequireAll || r.Partition != 0 || r.Topic != "orders" {
			t.Fatal("wrong produce destination", r)
		}
		record, e := r.Records.ReadRecord()
		if e != nil {
			t.Fatal(e)
		}
		key, _ := io.ReadAll(record.Key)
		value, _ := io.ReadAll(record.Value)
		if string(key) != "id" || string(value) != "payload" || len(record.Headers) != 1 {
			t.Fatal("wrong payload")
		}
		return &kafka.ProduceResponse{BaseOffset: 9007199254741010}, nil
	}
	v, e := kafkaProduce(context.Background(), f, "orders", map[string]string{"partition": "0", "key": "aWQ=", "keyEncoding": "base64", "value": "payload", "headers": "[{\"key\":\"trace\",\"value\":\"123\"}]"})
	if e != nil || !called || v.(map[string]any)["offset"] != "9007199254741010" {
		t.Fatal(v, e)
	}
	called = false
	if _, e = kafkaProduce(context.Background(), f, "orders", map[string]string{"keyEncoding": "base64", "key": "!!"}); e == nil || called {
		t.Fatal("invalid base64 produced")
	}
	f.produce = func(*kafka.ProduceRequest) (*kafka.ProduceResponse, error) {
		return &kafka.ProduceResponse{Error: kafka.TopicAuthorizationFailed}, nil
	}
	if _, e = kafkaProduce(context.Background(), f, "orders", map[string]string{}); !errors.Is(e, kafka.TopicAuthorizationFailed) {
		t.Fatal("broker failure hidden", e)
	}
}
func TestKafkaTopicConfigIncrementalAndSensitive(t *testing.T) {
	f := basicTopicFake()
	f.configs = func(*kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error) {
		return &kafka.DescribeConfigsResponse{Resources: []kafka.DescribeConfigResponseResource{{ConfigEntries: []kafka.DescribeConfigResponseConfigEntry{{ConfigName: "retention.ms", ConfigValue: "1000"}, {ConfigName: "secret", ConfigValue: "do-not-return", IsSensitive: true}, {ConfigName: "fixed", ReadOnly: true}}}}}, nil
	}
	entries, e := kafkaConfigs(context.Background(), f, "orders")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.Sensitive && entry.Value != "" {
			t.Fatal("sensitive config leaked")
		}
	}
	calls := 0
	f.alter = func(r *kafka.IncrementalAlterConfigsRequest) (*kafka.IncrementalAlterConfigsResponse, error) {
		calls++
		if len(r.Resources) != 1 || len(r.Resources[0].Configs) != 1 || r.Resources[0].Configs[0].ConfigOperation != kafka.ConfigOperationDelete {
			t.Fatal("must only reset the selected property")
		}
		return &kafka.IncrementalAlterConfigsResponse{Resources: []kafka.IncrementalAlterConfigsResponseResource{{}}}, nil
	}
	_, e = runKafkaTopic(context.Background(), f, MiddlewareOperation{Operation: "topic.config.set", Params: map[string]string{"name": "orders", "key": "retention.ms", "reset": "true"}})
	if e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	for _, key := range []string{"secret", "fixed", "missing"} {
		_, e = runKafkaTopic(context.Background(), f, MiddlewareOperation{Operation: "topic.config.set", Params: map[string]string{"name": "orders", "key": key}})
		if e == nil || calls != 1 {
			t.Fatal("forbidden config changed", key)
		}
	}
}
func TestKafkaPartitionsCannotShrink(t *testing.T) {
	f := basicTopicFake()
	_, e := runKafkaTopic(context.Background(), f, MiddlewareOperation{Operation: "topic.partitions", Params: map[string]string{"name": "orders", "partitions": "1"}})
	if e == nil {
		t.Fatal("partition reduction permitted")
	}
}
func TestKafkaConsumerLagDoesNotTreatMissingCommitAsZero(t *testing.T) {
	f := basicTopicFake()
	f.committed = func(*kafka.OffsetFetchRequest) (*kafka.OffsetFetchResponse, error) {
		return &kafka.OffsetFetchResponse{Topics: map[string][]kafka.OffsetFetchPartition{"orders": {{Partition: 0, CommittedOffset: 9007199254741005}}}}, nil
	}
	f.describe = func(*kafka.DescribeGroupsRequest) (*kafka.DescribeGroupsResponse, error) {
		return &kafka.DescribeGroupsResponse{Groups: []kafka.DescribeGroupsResponseGroup{{GroupState: "Stable"}}}, nil
	}
	ends := map[int]kafka.PartitionOffsets{0: {LastOffset: 9007199254741010}}
	row, related, e := kafkaConsumer(context.Background(), f, "orders", kafka.ListGroupsResponseGroup{GroupID: "orders-app"}, []int{0}, ends)
	if e != nil || !related || row.Lag != "5" {
		t.Fatal(row, related, e)
	}
	f.committed = func(*kafka.OffsetFetchRequest) (*kafka.OffsetFetchResponse, error) {
		return &kafka.OffsetFetchResponse{Topics: map[string][]kafka.OffsetFetchPartition{"orders": {{Partition: 0, CommittedOffset: -1}}}}, nil
	}
	row, related, e = kafkaConsumer(context.Background(), f, "orders", kafka.ListGroupsResponseGroup{GroupID: "orders-app"}, []int{0}, ends)
	if e != nil || related || row.Lag != "—" {
		t.Fatal("missing commit reported as zero", row, related, e)
	}
}
func TestKafkaTopicTimeSeekUsesTimestampOffset(t *testing.T) {
	f := basicTopicFake()
	base := f.offsets
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	f.offsets = func(r *kafka.ListOffsetsRequest) (*kafka.ListOffsetsResponse, error) {
		if r.Topics["orders"][0].Timestamp == at.UnixMilli() {
			return &kafka.ListOffsetsResponse{Topics: map[string][]kafka.PartitionOffsets{"orders": {{Partition: 0, Offsets: map[int64]time.Time{9007199254741010: at}}}}}, nil
		}
		return base(r)
	}
	v, e := kafkaMessages(context.Background(), f, "orders", map[string]string{"mode": "time", "timestamp": at.Format(time.RFC3339)})
	if e != nil || v.(map[string]any)["nextOffset"] != "9007199254741010" {
		t.Fatal(v, e)
	}
}

func TestKafkaClearTopicRebuildsShapeAndConfig(t *testing.T) {
	f := basicTopicFake()
	reads := 0
	f.metadata = func(*kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
		reads++
		if reads == 1 {
			return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Partitions: []kafka.Partition{
				{ID: 0, Leader: kafka.Broker{ID: 1}, Replicas: []kafka.Broker{{ID: 1}, {ID: 2}}},
				{ID: 1, Leader: kafka.Broker{ID: 2}, Replicas: []kafka.Broker{{ID: 2}, {ID: 1}}},
				{ID: 2, Leader: kafka.Broker{ID: 1}, Replicas: []kafka.Broker{{ID: 1}, {ID: 2}}},
			}}}}, nil
		}
		return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Error: kafka.UnknownTopicOrPartition}}}, nil
	}
	f.configs = func(*kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error) {
		return &kafka.DescribeConfigsResponse{Resources: []kafka.DescribeConfigResponseResource{{ConfigEntries: []kafka.DescribeConfigResponseConfigEntry{
			{ConfigName: "retention.ms", ConfigValue: "3600000", ConfigSource: 1},
			{ConfigName: "cleanup.policy", ConfigValue: "delete", ConfigSource: 5},
			{ConfigName: "secret", ConfigValue: "do-not-return", ConfigSource: 1, IsSensitive: true},
		}}}}, nil
	}
	deleted, created := false, false
	f.deleteTopic = func(r *kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error) {
		if len(r.Topics) != 1 || r.Topics[0] != "orders" {
			t.Fatal("wrong delete target", r.Topics)
		}
		deleted = true
		return &kafka.DeleteTopicsResponse{Errors: map[string]error{"orders": nil}}, nil
	}
	f.createTopic = func(r *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
		if len(r.Topics) != 1 {
			t.Fatal("wrong create payload", r.Topics)
		}
		cfg := r.Topics[0]
		if cfg.Topic != "orders" || cfg.NumPartitions != 3 || cfg.ReplicationFactor != 2 {
			t.Fatalf("wrong shape %+v", cfg)
		}
		created = true
		return &kafka.CreateTopicsResponse{Errors: map[string]error{"orders": nil}}, nil
	}
	alters := []string{}
	f.alter = func(r *kafka.IncrementalAlterConfigsRequest) (*kafka.IncrementalAlterConfigsResponse, error) {
		alters = append(alters, r.Resources[0].Configs[0].Name)
		return &kafka.IncrementalAlterConfigsResponse{Resources: []kafka.IncrementalAlterConfigsResponseResource{{}}}, nil
	}
	v, e := kafkaClearTopic(context.Background(), f, "orders")
	if e != nil || !deleted || !created {
		t.Fatal(v, e)
	}
	result, ok := v.(kafkaClearResult)
	if !ok || result.Partitions != 3 || result.ReplicationFactor != 2 || result.RestoredConfigs != 1 || len(alters) != 1 || alters[0] != "retention.ms" {
		t.Fatalf("bad clear result %+v %v", v, alters)
	}
}

func TestKafkaClearTopicRejectsInternalAndReportsRebuildFailure(t *testing.T) {
	f := basicTopicFake()
	f.metadata = func(*kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
		return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "__consumer_offsets", Internal: true, Partitions: []kafka.Partition{{ID: 0, Replicas: []kafka.Broker{{ID: 1}}}}}}}, nil
	}
	if _, e := kafkaClearTopic(context.Background(), f, "__consumer_offsets"); e == nil {
		t.Fatal("internal topic cleared")
	}
	reads := 0
	f.metadata = func(*kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
		reads++
		if reads == 1 {
			return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Partitions: []kafka.Partition{{ID: 0, Replicas: []kafka.Broker{{ID: 1}}}}}}}, nil
		}
		return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Error: kafka.UnknownTopicOrPartition}}}, nil
	}
	f.configs = func(*kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error) {
		return &kafka.DescribeConfigsResponse{Resources: []kafka.DescribeConfigResponseResource{{}}}, nil
	}
	f.deleteTopic = func(*kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error) {
		return &kafka.DeleteTopicsResponse{Errors: map[string]error{"orders": nil}}, nil
	}
	f.createTopic = func(*kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
		return &kafka.CreateTopicsResponse{Errors: map[string]error{"orders": kafka.TopicAuthorizationFailed}}, nil
	}
	if _, e := kafkaClearTopic(context.Background(), f, "orders"); e == nil || !strings.Contains(e.Error(), "重建失败") {
		t.Fatal("rebuild failure hidden", e)
	}
}

func TestKafkaClearTopicDoesNotRebuildWhenDeletionCannotBeConfirmed(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{
		{name: "metadata transport failure", err: errors.New("connection reset")},
		{name: "metadata authorization failure", err: kafka.TopicAuthorizationFailed},
	} {
		t.Run(failure.name, func(t *testing.T) {
			f := basicTopicFake()
			initialMetadata := f.metadata
			reads := 0
			f.metadata = func(r *kafka.MetadataRequest) (*kafka.MetadataResponse, error) {
				reads++
				if reads == 1 {
					return initialMetadata(r)
				}
				if errors.Is(failure.err, kafka.TopicAuthorizationFailed) {
					return &kafka.MetadataResponse{Topics: []kafka.Topic{{Name: "orders", Error: failure.err}}}, nil
				}
				return nil, failure.err
			}
			f.configs = func(*kafka.DescribeConfigsRequest) (*kafka.DescribeConfigsResponse, error) {
				return &kafka.DescribeConfigsResponse{Resources: []kafka.DescribeConfigResponseResource{{}}}, nil
			}
			deleted, created := false, false
			f.deleteTopic = func(*kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error) {
				deleted = true
				return &kafka.DeleteTopicsResponse{Errors: map[string]error{"orders": nil}}, nil
			}
			f.createTopic = func(*kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
				created = true
				return &kafka.CreateTopicsResponse{Errors: map[string]error{"orders": nil}}, nil
			}
			_, err := kafkaClearTopic(context.Background(), f, "orders")
			if !deleted || created || !errors.Is(err, failure.err) || !strings.Contains(err.Error(), "无法确认删除完成") {
				t.Fatalf("unconfirmed deletion continued: deleted %v, created %v, error %v", deleted, created, err)
			}
		})
	}
}
