package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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
	result, _, err := h.requestRaw(method, path, q, body, form)
	return result, err
}

// requestRaw also returns the response body. Nacos serves configuration content as
// plain text, so callers that need the untouched payload (JSON or YAML) use this.
func (h *middlewareHTTP) requestRaw(method, path string, q url.Values, body any, form bool) (any, string, error) {
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
				return nil, "", err
			}
		}
	}
	target := strings.TrimRight(h.base, "/") + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(h.ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return nil, "", err
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
		return nil, "", errors.New("连接中间件失败，请检查地址、网络和 TLS 证书")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > 8<<20 {
		return nil, "", errors.New("返回内容超过 8 MiB，请缩小查询范围")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", middlewareHTTPError(resp.StatusCode)
	}
	if len(raw) == 0 {
		return true, "", nil
	}
	var result any
	if json.Unmarshal(raw, &result) != nil {
		return string(raw), string(raw), nil
	}
	if success, ok := result.(bool); ok && !success {
		return nil, "", errors.New("中间件拒绝本次操作，请检查参数和远端日志")
	}
	if m, ok := result.(map[string]any); ok {
		if code, ok := m["code"].(float64); ok && code != 0 && code != 200 {
			return nil, "", fmt.Errorf("中间件拒绝请求（code %.0f）：%v", code, m["message"])
		}
		if e, ok := m["error"].(string); ok && e != "" {
			return nil, "", fmt.Errorf("中间件拒绝请求：%s", e)
		}
	}
	return result, string(raw), nil
}

func middlewareHTTPError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("中间件返回 HTTP %d，认证失败或账号权限不足", status)
	case http.StatusNotFound:
		return errors.New("中间件返回 HTTP 404，资源不存在或已被删除")
	case http.StatusTooManyRequests:
		return fmt.Errorf("中间件返回 HTTP %d，请求过于频繁，请稍后重试", status)
	}
	return fmt.Errorf("中间件返回 HTTP %d，请检查权限和接口地址", status)
}
func runMiddlewareHTTP(ctx context.Context, d model.MiddlewareDatasource, c model.AssetCredential, r MiddlewareOperation) (any, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	if c.AuthType == "tls" {
		cfg, err := middlewareTLSConfig(c)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = cfg
	}
	h := &middlewareHTTP{ctx: ctx, base: d.Endpoints[0], credential: c, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	switch d.Type {
	case "rabbitmq":
		h.basic = d.CredentialID > 0
		return h.rabbit(r)
	case "nacos":
		return h.nacos(d.Version, r)
	case "etcd":
		h.basic = d.AuthType == "basic"
		return h.etcd(r)
	}
	return nil, errors.New("不支持的 HTTP 中间件")
}

// middlewareTLSConfig builds the client side of a TLS / mTLS connection from a
// certificate credential. A missing CA falls back to the system trust store.
func middlewareTLSConfig(c model.AssetCredential) (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	if ca := strings.TrimSpace(c.CACert); ca != "" {
		if !pool.AppendCertsFromPEM([]byte(ca)) {
			return nil, errors.New("CA 证书无法解析，请确认上传的是 PEM 格式证书")
		}
	}
	cert, key := strings.TrimSpace(c.ClientCert), strings.TrimSpace(c.PrivateKey)
	if cert != "" || key != "" {
		if cert == "" || key == "" {
			return nil, errors.New("TLS 凭据需要同时提供客户端证书与私钥")
		}
		pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, errors.New("客户端证书与私钥不匹配，或不是 PEM 格式")
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg, nil
}
func requiredMiddleware(p map[string]string, keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(p[k]) == "" {
			return fmt.Errorf("请填写 %s", k)
		}
	}
	return nil
}

// rabbitArguments parses the optional JSON argument map used by queue/exchange
// declarations. Keeping it JSON avoids inventing a second parameter format.
func rabbitArguments(p map[string]string) (map[string]any, error) {
	args := map[string]any{}
	raw := strings.TrimSpace(p["arguments"])
	if raw == "" {
		return args, nil
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, errors.New("扩展参数必须是 JSON 对象，例如 {\"x-queue-type\":\"quorum\"}")
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func (h *middlewareHTTP) rabbit(r MiddlewareOperation) (any, error) {
	p := r.Params
	v := p["vhost"]
	if v == "" {
		v = "/"
	}
	// "*" 表示跨虚拟主机查询，由前端“全部虚拟主机”使用。
	all := v == "*"
	esc := url.PathEscape
	method, path := http.MethodGet, ""
	var body any
	switch r.Operation {
	case "overview", "vhosts", "connections", "channels":
		path = "/api/" + r.Operation
	case "queues", "exchanges", "consumers":
		path = "/api/" + r.Operation
		if !all {
			path += "/" + esc(v)
		}
	case "queue.get", "queue.bindings", "exchange.get", "exchange.bindings":
		if err := requiredMiddleware(p, "name"); err != nil {
			return nil, err
		}
		kind := "queues"
		if strings.HasPrefix(r.Operation, "exchange.") {
			kind = "exchanges"
		}
		path = "/api/" + kind + "/" + esc(v) + "/" + esc(p["name"])
		if strings.HasSuffix(r.Operation, "bindings") {
			path += "/bindings"
		}
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
			args, err := rabbitArguments(p)
			if err != nil {
				return nil, err
			}
			body = map[string]any{"durable": p["durable"] != "false", "auto_delete": p["auto_delete"] == "true", "arguments": args}
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
		path = "/api/connections/" + esc(p["name"])
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

// nacosPage applies bounded pagination so a list request cannot pull the whole
// configuration table in one call.
func nacosPage(q url.Values, p map[string]string) {
	q.Set("pageNo", "1")
	if n, err := strconv.Atoi(strings.TrimSpace(p["pageNo"])); err == nil && n > 0 {
		q.Set("pageNo", strconv.Itoa(n))
	}
	q.Set("pageSize", "100")
	if n, err := strconv.Atoi(strings.TrimSpace(p["pageSize"])); err == nil && n >= 1 && n <= 500 {
		q.Set("pageSize", strconv.Itoa(n))
	}
}

func (h *middlewareHTTP) nacos(version string, r MiddlewareOperation) (any, error) {
	prefix, login := "/v1", "/v1/auth/login"
	if version == "3" {
		prefix, login = "/v3/console", "/v3/auth/user/login"
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
	ns := strings.TrimSpace(p["namespaceId"])
	group := strings.TrimSpace(p["groupName"])
	if group == "" {
		group = "DEFAULT_GROUP"
	}
	if version == "3" && ns == "" {
		ns = "public"
	}
	q := url.Values{}
	method, path := http.MethodGet, ""
	switch r.Operation {
	case "namespaces":
		path = prefix + "/console/namespaces"
		if version == "3" {
			path = prefix + "/core/namespace/list"
		}
	case "namespace.put", "namespace.delete":
		path = prefix + "/console/namespaces"
		if version == "3" {
			path = prefix + "/core/namespace"
		}
		if r.Operation == "namespace.delete" {
			if err := requiredMiddleware(p, "namespaceId"); err != nil {
				return nil, err
			}
			method = http.MethodDelete
			q.Set("namespaceId", ns)
			break
		}
		if err := requiredMiddleware(p, "namespaceName"); err != nil {
			return nil, err
		}
		method = http.MethodPost
		q.Set("namespaceName", strings.TrimSpace(p["namespaceName"]))
		if desc := strings.TrimSpace(p["namespaceDesc"]); desc != "" {
			q.Set("namespaceDesc", desc)
		}
		if ns != "" {
			q.Set("customNamespaceId", ns)
		}
	case "configs":
		nacosPage(q, p)
		q.Set("search", "blur")
		if dataID := strings.TrimSpace(p["dataId"]); dataID != "" {
			q.Set("dataId", dataID)
		}
		if appName := strings.TrimSpace(p["appName"]); appName != "" {
			q.Set("appName", appName)
		}
		if version == "3" {
			path = prefix + "/cs/config/list"
			q.Set("namespaceId", ns)
			q.Set("groupName", group)
		} else {
			path = "/v1/cs/configs"
			q.Set("tenant", ns)
			q.Set("group", group)
		}
	case "config.get", "config.put", "config.delete":
		if err := requiredMiddleware(p, "dataId"); err != nil {
			return nil, err
		}
		if version == "3" {
			path = prefix + "/cs/config"
			q.Set("namespaceId", ns)
			q.Set("groupName", group)
		} else {
			path = "/v1/cs/configs"
			q.Set("tenant", ns)
			q.Set("group", group)
		}
		q.Set("dataId", strings.TrimSpace(p["dataId"]))
		switch r.Operation {
		case "config.put":
			method = http.MethodPost
			q.Set("content", p["content"])
			format := strings.TrimSpace(p["type"])
			if format == "" {
				format = "text"
			}
			q.Set("type", format)
			if appName := strings.TrimSpace(p["appName"]); appName != "" {
				q.Set("appName", appName)
			}
			if desc := strings.TrimSpace(p["desc"]); desc != "" {
				q.Set("desc", desc)
			}
		case "config.delete":
			method = http.MethodDelete
		}
	case "services":
		nacosPage(q, p)
		service := strings.TrimSpace(p["serviceName"])
		q.Set("namespaceId", ns)
		if version == "3" {
			path = prefix + "/ns/service/list"
			q.Set("groupNameParam", group)
		} else {
			// v1 目录接口同时返回实例数与健康实例数，比只返回名称的 service/list 更适合清单展示。
			path = "/v1/ns/catalog/services"
			q.Set("groupNameParam", group)
			q.Set("withInstances", "false")
		}
		if service != "" {
			q.Set("serviceNameParam", service)
		}
	case "instances":
		if err := requiredMiddleware(p, "serviceName"); err != nil {
			return nil, err
		}
		path = prefix + "/ns/instance/list"
		q.Set("namespaceId", ns)
		q.Set("groupName", group)
		q.Set("serviceName", strings.TrimSpace(p["serviceName"]))
		if cluster := strings.TrimSpace(p["clusterName"]); cluster != "" {
			q.Set("clusterName", cluster)
		}
		if p["healthyOnly"] == "true" {
			q.Set("healthyOnly", "true")
		}
	case "instance.put", "instance.delete":
		if err := requiredMiddleware(p, "serviceName", "ip", "port"); err != nil {
			return nil, err
		}
		path = prefix + "/ns/instance"
		q.Set("namespaceId", ns)
		q.Set("groupName", group)
		q.Set("serviceName", strings.TrimSpace(p["serviceName"]))
		q.Set("ip", strings.TrimSpace(p["ip"]))
		q.Set("port", strings.TrimSpace(p["port"]))
		for _, key := range []string{"weight", "enabled", "ephemeral", "clusterName", "metadata"} {
			if value := strings.TrimSpace(p[key]); value != "" {
				q.Set(key, value)
			}
		}
		method = http.MethodPut
		if r.Operation == "instance.delete" {
			method = http.MethodDelete
		}
	default:
		return nil, errors.New("不支持的 Nacos 操作")
	}
	if h.token != "" {
		q.Set("accessToken", h.token)
	}
	var body any
	if method == http.MethodPost || method == http.MethodPut {
		body = q
		q = nil
	}
	v, raw, e := h.requestRaw(method, path, q, body, true)
	if e != nil {
		return nil, e
	}
	// 配置内容以原始文本返回，避免 JSON 格式的配置被解析后再序列化。
	if r.Operation == "config.get" {
		// Nacos 3.x 返回配置详情对象（content 在 data 或根节点）；2.x 直接返回内容文本，
		// 即使内容本身是 JSON 也不能再拆包，否则会破坏以 content 为字段的配置。
		if version == "3" {
			var payload map[string]any
			if json.Unmarshal([]byte(raw), &payload) == nil {
				if content, ok := nacosConfigContent(payload); ok {
					return content, nil
				}
			}
		}
		return raw, nil
	}
	return unwrapMiddleware(v), nil
}

func nacosConfigContent(payload map[string]any) (string, bool) {
	if content, ok := payload["content"].(string); ok {
		return content, true
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if content, ok := data["content"].(string); ok {
			return content, true
		}
	}
	return "", false
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
	if h.basic {
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
