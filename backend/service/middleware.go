package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"slices"
	"strings"
	"time"
)

type MiddlewareOperation struct {
	ID        uint              `json:"id"`
	Operation string            `json:"operation"`
	Params    map[string]string `json:"params"`
	Confirmed bool              `json:"confirmed"`
}

// This allowlist is also the authorization boundary: the client cannot select
// a read endpoint and then submit a mutation operation.
var middlewareOperations = map[string]map[string]string{
	"rabbitmq":  {"overview": "query", "vhosts": "query", "queues": "query", "exchanges": "query", "connections": "query", "consumers": "query", "channels": "query", "queue.get": "query", "queue.bindings": "query", "exchange.get": "query", "exchange.bindings": "query", "queue.put": "write", "exchange.put": "write", "binding.put": "write", "queue.delete": "delete", "queue.purge": "delete", "exchange.delete": "delete", "connection.close": "delete"},
	"nacos":     {"namespaces": "query", "configs": "query", "config.get": "query", "services": "query", "instances": "query", "config.put": "write", "namespace.put": "write", "instance.put": "write", "config.delete": "delete", "namespace.delete": "delete", "instance.delete": "delete"},
	"etcd":      {"keys": "query", "key.get": "query", "members": "query", "key.put": "write", "key.delete": "delete"},
	"zookeeper": {"children": "query", "node.get": "query", "node.create": "write", "node.put": "write", "node.delete": "delete"},
	"kafka":     {"topics": "query", "brokers": "query", "groups": "query", "group.get": "query", "topic.overview": "query", "topic.messages": "query", "topic.consumers": "query", "topic.configs": "query", "topic.produce": "write", "topic.config.set": "write", "topic.partitions": "write", "topic.create": "write", "topic.delete": "delete", "topic.clear": "delete", "group.delete": "delete"},
}

func validateMiddleware(d *model.MiddlewareDatasource) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || len(d.Name) > 128 || len(d.Description) > 500 {
		return errors.New("请填写数据源名称，说明最多 500 字节")
	}
	if middlewareOperations[d.Type] == nil {
		return errors.New("不支持的中间件类型")
	}
	if len(d.Endpoints) == 0 || len(d.Endpoints) > 10 {
		return errors.New("请填写 1 至 10 个连接地址")
	}
	if d.Type != "kafka" && d.Type != "zookeeper" && len(d.Endpoints) != 1 {
		return errors.New("HTTP 数据源请填写一个服务地址或负载均衡地址")
	}
	if d.AuthType == "" {
		d.AuthType = "none"
	}
	allowedAuth := map[string][]string{"rabbitmq": {"none", "basic"}, "nacos": {"none", "basic"}, "etcd": {"none", "basic", "tls"}, "kafka": {"none", "plain", "scram256", "scram512"}, "zookeeper": {"none", "digest"}}
	if !slices.Contains(allowedAuth[d.Type], d.AuthType) {
		return errors.New("认证方式与中间件类型不匹配")
	}
	if d.AuthType == "none" {
		d.CredentialID = 0
	}
	if d.Type == "etcd" && d.AuthType == "tls" {
		d.TLS = true
	}
	for i, e := range d.Endpoints {
		e = strings.TrimSpace(e)
		if d.Type == "rabbitmq" || d.Type == "nacos" || d.Type == "etcd" {
			u, err := url.Parse(e)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return errors.New("HTTP 地址格式错误，请使用 http(s)://主机:端口，可包含根路径，不要在地址内填写密码")
			}
			// ETCD 开启 TLS 后统一使用 HTTPS，避免把证书和明文地址混在一起。
			if d.Type == "etcd" && d.TLS && u.Scheme == "http" {
				u.Scheme = "https"
				e = u.String()
			}
			d.Endpoints[i] = e
		} else {
			if _, port, err := net.SplitHostPort(e); err != nil || port == "" {
				return errors.New("原生协议地址请填写 主机:端口，多个地址分别填写")
			}
			d.Endpoints[i] = e
		}
	}
	if d.Type == "nacos" && d.Version != "2" && d.Version != "3" {
		return errors.New("请选择 Nacos 2.x 或 3.x")
	}
	if !slices.Contains([]string{"", "none", "basic", "plain", "scram256", "scram512", "digest", "tls"}, d.AuthType) {
		return errors.New("认证方式不支持")
	}
	if d.AuthType != "" && d.AuthType != "none" && d.CredentialID == 0 {
		return errors.New("请选择认证凭据")
	}
	if d.Type == "zookeeper" && d.TLS {
		return errors.New("当前 ZooKeeper 原生客户端暂不支持 TLS，请使用受信任内网连接")
	}
	return nil
}
func (s *Service) middlewareCredential(d model.MiddlewareDatasource) (model.AssetCredential, error) {
	var c model.AssetCredential
	if d.CredentialID == 0 {
		return c, nil
	}
	// A bound credential is used internally only after the datasource permission check.
	err := s.db.WithContext(access.Context(context.Background(), nil)).First(&c, d.CredentialID).Error
	if err != nil {
		return c, errors.New("绑定凭据不存在")
	}
	if c.Status != 1 {
		return c, errors.New("绑定凭据已停用")
	}
	switch c.AuthType {
	case "password":
		if d.AuthType == "tls" {
			return c, errors.New("TLS 证书认证需要选择 TLS 证书类型的凭据")
		}
	case "tls":
		if d.AuthType != "tls" {
			return c, errors.New("TLS 证书凭据只能用于 TLS 证书认证方式")
		}
		if strings.TrimSpace(c.CACert) == "" && (strings.TrimSpace(c.ClientCert) == "" || strings.TrimSpace(c.PrivateKey) == "") {
			return c, errors.New("TLS 凭据缺少 CA 证书或客户端证书，请在凭据管理中补全")
		}
	default:
		return c, errors.New("中间件认证需要用户名密码或 TLS 证书类型的凭据")
	}
	return c, nil
}
func (s *Service) ListMiddleware(kind string) ([]model.MiddlewareDatasource, error) {
	rows := []model.MiddlewareDatasource{}
	q := s.db.Order("id DESC")
	if kind != "" {
		q = q.Where("type = ?", kind)
	}
	err := q.Find(&rows).Error
	if p := s.resourcePolicy(); p != nil && !p.Super {
		for i := range rows {
			rows[i].CredentialID = 0
			rows[i].Endpoints = nil
		}
	}
	return rows, err
}
func (s *Service) SaveMiddleware(d model.MiddlewareDatasource) error {
	if err := s.commandFilterAdmin(); err != nil {
		return err
	}
	if err := validateMiddleware(&d); err != nil {
		return err
	}
	if _, err := s.middlewareCredential(d); err != nil {
		return err
	}
	if d.ID > 0 {
		var old model.MiddlewareDatasource
		if err := s.db.First(&old, d.ID).Error; err != nil {
			return err
		}
		if old.Type != d.Type {
			return errors.New("已有数据源不能更改类型")
		}
		d.CreatedAt = old.CreatedAt
	}
	return s.db.Save(&d).Error
}
func (s *Service) DeleteMiddleware(id uint) error {
	if err := s.commandFilterAdmin(); err != nil {
		return err
	}
	if id == 0 {
		return errors.New("请选择数据源")
	}
	return s.db.Delete(&model.MiddlewareDatasource{}, id).Error
}
func (s *Service) TestMiddleware(d model.MiddlewareDatasource) (any, error) {
	if err := s.commandFilterAdmin(); err != nil {
		return nil, err
	}
	if err := validateMiddleware(&d); err != nil {
		return nil, err
	}
	c, err := s.middlewareCredential(d)
	if err != nil {
		return nil, err
	}
	op := map[string]string{"rabbitmq": "overview", "nacos": "namespaces", "etcd": "members", "zookeeper": "children", "kafka": "brokers"}[d.Type]
	return runMiddleware(d, c, MiddlewareOperation{Operation: op, Params: map[string]string{"path": "/"}})
}
func (s *Service) OperateMiddleware(r MiddlewareOperation, action string) (any, error) {
	if r.ID == 0 {
		return nil, errors.New("请选择数据源")
	}
	if err := s.requireResource("middleware", r.ID, "", action); err != nil {
		return nil, err
	}
	var d model.MiddlewareDatasource
	if err := s.db.First(&d, r.ID).Error; err != nil {
		return nil, err
	}
	required := middlewareOperations[d.Type][r.Operation]
	if required == "" || required != action {
		return nil, access.ErrDenied
	}
	if action != "query" && !r.Confirmed {
		return nil, errors.New("请确认本次中间件变更操作")
	}
	if len(r.Params) > 24 {
		return nil, errors.New("参数过多")
	}
	for k, v := range r.Params {
		if len(k) > 64 || len(v) > 1<<20 {
			return nil, errors.New("参数内容过大")
		}
	}
	c, err := s.middlewareCredential(d)
	if err != nil {
		return nil, err
	}
	return runMiddleware(d, c, r)
}
func runMiddleware(d model.MiddlewareDatasource, c model.AssetCredential, r MiddlewareOperation) (any, error) {
	// 清空 Topic 需要等待 Broker 完成删除再重建，给更长的执行窗口。
	timeout := 12 * time.Second
	if r.Operation == "topic.clear" {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if r.Params == nil {
		r.Params = map[string]string{}
	}
	switch d.Type {
	case "rabbitmq", "nacos", "etcd":
		return runMiddlewareHTTP(ctx, d, c, r)
	case "kafka":
		return runMiddlewareKafka(ctx, d, c, r)
	case "zookeeper":
		return runMiddlewareZK(ctx, d, c, r)
	}
	return nil, fmt.Errorf("未知中间件类型 %s", d.Type)
}
