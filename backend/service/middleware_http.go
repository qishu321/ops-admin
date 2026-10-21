package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"ops-admin/backend/model"
	"strconv"
	"strings"
)

type middlewareHTTP struct {
	ctx        context.Context
	base       string
	client     *http.Client
	credential model.AssetCredential
	token      string
	basic      bool
}

func (h *middlewareHTTP) request(method, path string, q url.Values, body any, form bool) (any, error) {
	if q == nil {
		q = url.Values{}
	}
	var data []byte
	var err error
	ctype := "application/json"
	if body != nil {
		if form {
			data = []byte(body.(url.Values).Encode())
			ctype = "application/x-www-form-urlencoded"
		} else {
			data, err = json.Marshal(body)
			if err != nil {
				return nil, err
			}
		}
	}
	target := strings.TrimRight(h.base, "/") + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(h.ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ctype)
	if h.basic {
		req.SetBasicAuth(h.credential.Username, h.credential.Password)
	}
	if h.token != "" {
		req.Header.Set("Authorization", h.token)
		req.Header.Set("accessToken", h.token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, errors.New("连接中间件失败，请检查地址、网络和 TLS 证书")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 8<<20 {
		return nil, errors.New("返回内容超过 8 MiB，请缩小查询范围")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("中间件返回 HTTP %d，请检查权限和接口地址", resp.StatusCode)
	}
	if len(raw) == 0 {
		return true, nil
	}
	var result any
	if json.Unmarshal(raw, &result) != nil {
		return string(raw), nil
	}
	if success, ok := result.(bool); ok && !success {
		return nil, errors.New("中间件拒绝本次操作，请检查参数和远端日志")
	}
	if m, ok := result.(map[string]any); ok {
		if code, ok := m["code"].(float64); ok && code != 0 && code != 200 {
			return nil, fmt.Errorf("中间件拒绝请求（code %.0f）：%v", code, m["message"])
		}
		if e, ok := m["error"].(string); ok && e != "" {
			return nil, fmt.Errorf("中间件拒绝请求：%s", e)
		}
	}
	return result, nil
}
func runMiddlewareHTTP(ctx context.Context, d model.MiddlewareDatasource, c model.AssetCredential, r MiddlewareOperation) (any, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	h := &middlewareHTTP{ctx: ctx, base: d.Endpoints[0], credential: c, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	switch d.Type {
	case "rabbitmq":
		h.basic = d.CredentialID > 0
		return h.rabbit(r)
	case "nacos":
		return h.nacos(d.Version, r)
	case "etcd":
		return h.etcd(r)
	}
	return nil, errors.New("不支持的 HTTP 中间件")
}
func requiredMiddleware(p map[string]string, keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(p[k]) == "" {
			return fmt.Errorf("请填写 %s", k)
		}
	}
	return nil
}
func (h *middlewareHTTP) rabbit(r MiddlewareOperation) (any, error) {
	p := r.Params
	v := p["vhost"]
	if v == "" {
		v = "/"
	}
	esc := url.PathEscape
	method, path := http.MethodGet, ""
	var body any
	switch r.Operation {
	case "overview", "vhosts", "connections", "channels":
		path = "/api/" + r.Operation
	case "queues", "exchanges", "consumers":
		path = "/api/" + r.Operation + "/" + esc(v)
	case "queue.put", "queue.delete", "queue.purge", "exchange.put", "exchange.delete":
		if err := requiredMiddleware(p, "name"); err != nil {
			return nil, err
		}
		kind := "queues"
		if strings.HasPrefix(r.Operation, "exchange.") {
			kind = "exchanges"
		}
		path = "/api/" + kind + "/" + esc(v) + "/" + esc(p["name"])
		if strings.HasSuffix(r.Operation, "put") {
			method = "PUT"
			body = map[string]any{"durable": p["durable"] != "false", "auto_delete": false, "arguments": map[string]any{}}
			if kind == "exchanges" {
				t := p["type"]
				if t == "" {
					t = "direct"
				}
				body.(map[string]any)["type"] = t
			}
		} else {
			method = "DELETE"
			if r.Operation == "queue.purge" {
				path += "/contents"
			}
		}
	case "binding.put":
		if err := requiredMiddleware(p, "exchange", "queue"); err != nil {
			return nil, err
		}
		method = "POST"
		path = "/api/bindings/" + esc(v) + "/e/" + esc(p["exchange"]) + "/q/" + esc(p["queue"])
		body = map[string]any{"routing_key": p["routingKey"], "arguments": map[string]any{}}
	case "connection.close":
		if err := requiredMiddleware(p, "name"); err != nil {
			return nil, err
		}
		method = "DELETE"
		kind := "connections"
		path = "/api/" + kind + "/" + esc(p["name"])
	default:
		return nil, errors.New("不支持的 RabbitMQ 操作")
	}
	return h.request(method, path, nil, body, false)
}
func unwrapMiddleware(v any) any {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"]; ok {
			return d
		}
	}
	return v
}
func (h *middlewareHTTP) nacos(version string, r MiddlewareOperation) (any, error) {
	prefix := "/v1"
	login := "/v1/auth/login"
	if version == "3" {
		prefix = "/v3/console"
		login = "/v3/auth/user/login"
	}
	if h.credential.ID > 0 {
		v, e := h.request("POST", login, nil, url.Values{"username": {h.credential.Username}, "password": {h.credential.Password}}, true)
		if e != nil {
			return nil, e
		}
		m, _ := v.(map[string]any)
		h.token, _ = m["accessToken"].(string)
		if h.token == "" {
			if data, ok := m["data"].(map[string]any); ok {
				h.token, _ = data["accessToken"].(string)
			}
		}
		if h.token == "" {
			return nil, errors.New("Nacos 登录未返回 accessToken")
		}
	}
	p := r.Params
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	if q.Get("pageNo") == "" {
		q.Set("pageNo", "1")
	}
	if n, e := strconv.Atoi(q.Get("pageSize")); e != nil || n < 1 || n > 500 {
		q.Set("pageSize", "100")
	}
	if q.Get("namespaceId") == "" && version == "3" {
		q.Set("namespaceId", "public")
	}
	if q.Get("groupName") == "" {
		q.Set("groupName", "DEFAULT_GROUP")
	}
	method, path := "GET", ""
	namespace := "/console/namespaces"
	if version == "3" {
		namespace = "/core/namespace"
	}
	switch r.Operation {
	case "namespaces":
		path = prefix + namespace
		if version == "3" {
			path += "/list"
		}
	case "namespace.put", "namespace.delete":
		path = prefix + namespace
		method = "POST"
		if r.Operation == "namespace.delete" {
			method = "DELETE"
			if err := requiredMiddleware(p, "namespaceId"); err != nil {
				return nil, err
			}
		} else {
			if err := requiredMiddleware(p, "namespaceName"); err != nil {
				return nil, err
			}
			q.Set("customNamespaceId", p["namespaceId"])
		}
	case "configs":
		path = prefix + "/cs/config/list"
		q.Set("search", "blur")
		if version == "2" {
			path = "/v1/cs/configs"
			q.Set("tenant", p["namespaceId"])
			q.Set("group", q.Get("groupName"))
			q.Del("groupName")
			q.Del("namespaceId")
		}
	case "config.get", "config.put", "config.delete":
		if err := requiredMiddleware(p, "dataId"); err != nil {
			return nil, err
		}
		path = prefix + "/cs/config"
		if version == "2" {
			path = "/v1/cs/configs"
			q.Set("group", q.Get("groupName"))
			q.Set("tenant", p["namespaceId"])
			q.Del("groupName")
			q.Del("namespaceId")
		}
		if r.Operation == "config.put" {
			method = "POST"
		}
		if r.Operation == "config.delete" {
			method = "DELETE"
		}
	case "services":
		path = prefix + "/ns/service/list"
	case "instances":
		if err := requiredMiddleware(p, "serviceName"); err != nil {
			return nil, err
		}
		path = prefix + "/ns/instance/list"
	case "instance.put", "instance.delete":
		if err := requiredMiddleware(p, "serviceName", "ip", "port"); err != nil {
			return nil, err
		}
		path = prefix + "/ns/instance"
		method = "PUT"
		if r.Operation == "instance.delete" {
			method = "DELETE"
		}
	default:
		return nil, errors.New("不支持的 Nacos 操作")
	}
	if h.token != "" {
		q.Set("accessToken", h.token)
	}
	var body any
	if method == "POST" || method == "PUT" {
		body = q
		q = nil
	}
	v, e := h.request(method, path, q, body, true)
	return unwrapMiddleware(v), e
}
func prefixEnd(b []byte) []byte {
	out := append([]byte(nil), b...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 255 {
			out[i]++
			return out[:i+1]
		}
	}
	return []byte{0}
}
func (h *middlewareHTTP) etcd(r MiddlewareOperation) (any, error) {
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	if h.credential.ID > 0 {
		v, e := h.request("POST", "/v3/auth/authenticate", nil, map[string]string{"name": h.credential.Username, "password": h.credential.Password}, false)
		if e != nil {
			return nil, e
		}
		m, _ := v.(map[string]any)
		h.token, _ = m["token"].(string)
		if h.token == "" {
			return nil, errors.New("etcd 登录未返回 token")
		}
	}
	p := r.Params
	body := map[string]any{}
	path := ""
	switch r.Operation {
	case "members":
		path = "/v3/cluster/member/list"
	case "keys", "key.get":
		path = "/v3/kv/range"
		key := p["key"]
		if r.Operation == "key.get" && key == "" {
			return nil, errors.New("请填写 key")
		}
		body["key"] = enc(key)
		body["limit"] = 200
		if r.Operation == "keys" {
			if key == "" {
				body["key"] = enc(string([]byte{0}))
				body["range_end"] = enc(string([]byte{0}))
			} else {
				body["range_end"] = base64.StdEncoding.EncodeToString(prefixEnd([]byte(key)))
			}
		}
	case "key.put", "key.delete":
		if err := requiredMiddleware(p, "key"); err != nil {
			return nil, err
		}
		body["key"] = enc(p["key"])
		if r.Operation == "key.put" {
			path = "/v3/kv/put"
			body["value"] = enc(p["value"])
		} else {
			path = "/v3/kv/deleterange"
		}
	default:
		return nil, errors.New("不支持的 etcd 操作")
	}
	v, e := h.request("POST", path, nil, body, false)
	if e != nil {
		return nil, e
	}
	if m, ok := v.(map[string]any); ok {
		if list, ok := m["kvs"].([]any); ok {
			for _, entry := range list {
				if kv, ok := entry.(map[string]any); ok {
					for _, k := range []string{"key", "value"} {
						if raw, ok := kv[k].(string); ok {
							decoded, e := base64.StdEncoding.DecodeString(raw)
							if e == nil {
								kv[k] = string(decoded)
							}
						}
					}
				}
			}
		}
	}
	return v, nil
}
