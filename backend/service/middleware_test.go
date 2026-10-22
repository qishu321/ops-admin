package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ops-admin/backend/model"
	"strings"
	"testing"
)

func TestMiddlewareNacosVersions(t *testing.T) {
	for _, version := range []string{"2", "3"} {
		t.Run(version, func(t *testing.T) {
			logins := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				login := "/nacos/v1/auth/login"
				config := "/nacos/v1/cs/configs"
				list := "/nacos/v1/cs/configs"
				if version == "3" {
					login = "/v3/auth/user/login"
					config = "/v3/console/cs/config"
					list = "/v3/console/cs/config/list"
				}
				if r.URL.Path == login {
					r.ParseForm()
					if r.Form.Get("username") != "operator" || r.Form.Get("password") != "secret" {
						t.Error("credential missing")
					}
					logins++
					w.Write([]byte(`{"accessToken":"test-token"}`))
					return
				}
				if r.Header.Get("accessToken") != "test-token" {
					t.Error("token missing")
				}
				r.ParseForm()
				if r.URL.Path == config && r.Method == "POST" {
					if r.Method != "POST" || r.Form.Get("dataId") != "app.yaml" || r.Form.Get("content") != "a: 1\nb: 2" {
						t.Errorf("bad publish: %s %v", r.Method, r.Form)
					}
					key := "group"
					if version == "3" {
						key = "groupName"
					}
					if r.Form.Get(key) != "OPS" {
						t.Error("group not adapted")
					}
					w.Write([]byte(`{"code":0,"data":true}`))
					return
				}
				if r.URL.Path == list {
					if version == "2" && r.URL.Query().Get("tenant") != "test" {
						t.Error("namespace not adapted")
					}
					w.Write([]byte(`{"code":0,"data":{"pageItems":[],"totalCount":0}}`))
					return
				}
				t.Errorf("unexpected route %s", r.URL.Path)
				w.WriteHeader(404)
			}))
			defer server.Close()
			base := server.URL
			if version == "2" {
				base += "/nacos"
			}
			d := model.MiddlewareDatasource{Type: "nacos", Endpoints: []string{base}, Version: version}
			c := model.AssetCredential{ID: 1, Username: "operator", Password: "secret"}
			_, e := runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "config.put", Params: map[string]string{"dataId": "app.yaml", "groupName": "OPS", "content": "a: 1\nb: 2"}})
			if e != nil {
				t.Fatal(e)
			}
			_, e = runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "configs", Params: map[string]string{"namespaceId": "test"}})
			if e != nil {
				t.Fatal(e)
			}
			if logins != 2 {
				t.Fatal("expected authentication for each operation")
			}
		})
	}
}
func TestMiddlewareRabbitEscapesAndCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "ops" || p != "secret" {
			t.Error("missing basic auth")
		}
		if r.URL.EscapedPath() != "/api/queues/%2F/team%2Fqueue" {
			t.Errorf("unsafe resource path %s", r.URL.EscapedPath())
		}
		if r.Method != "DELETE" {
			t.Error("expected delete")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	_, e := runMiddlewareHTTP(context.Background(), model.MiddlewareDatasource{Type: "rabbitmq", Endpoints: []string{server.URL}, CredentialID: 1}, model.AssetCredential{Username: "ops", Password: "secret"}, MiddlewareOperation{Operation: "queue.delete", Params: map[string]string{"name": "team/queue"}})
	if e != nil {
		t.Fatal(e)
	}
}
func TestMiddlewareEtcdDeleteIsExactAndDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["range_end"] != nil {
			t.Error("exact key operation must not use prefix delete")
		}
		if body["key"] != base64.StdEncoding.EncodeToString([]byte("/app/key")) {
			t.Error("key encoding")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "range") {
			w.Write([]byte(`{"kvs":[{"key":"L2FwcC9rZXk=","value":"aGVsbG8="}]}`))
		} else {
			w.Write([]byte(`{"deleted":"1"}`))
		}
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "etcd", Endpoints: []string{server.URL}}
	p := map[string]string{"key": "/app/key"}
	_, e := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "key.delete", Params: p})
	if e != nil {
		t.Fatal(e)
	}
	v, e := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "key.get", Params: p})
	if e != nil {
		t.Fatal(e)
	}
	kv := v.(map[string]any)["kvs"].([]any)[0].(map[string]any)
	if kv["key"] != "/app/key" || kv["value"] != "hello" {
		t.Fatal(kv)
	}
}
func TestMiddlewareNoRedirectAndRemoteError(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	_, e := runMiddlewareHTTP(context.Background(), model.MiddlewareDatasource{Type: "rabbitmq", Endpoints: []string{server.URL}, CredentialID: 1}, model.AssetCredential{Username: "ops", Password: "secret"}, MiddlewareOperation{Operation: "overview"})
	if e == nil || leaked {
		t.Fatal("must not redirect authenticated requests")
	}
}
func TestMiddlewareValidation(t *testing.T) {
	for _, d := range []model.MiddlewareDatasource{
		{Name: "a", Type: "nacos", Version: "1", Endpoints: []string{"http://host:8848/nacos"}},
		{Name: "a", Type: "rabbitmq", Endpoints: []string{"http://user:password@host:15672"}},
		{Name: "a", Type: "kafka", Endpoints: []string{"kafka://host:9092"}},
		{Name: "a", Type: "zookeeper", TLS: true, Endpoints: []string{"host:2181"}},
	} {
		if validateMiddleware(&d) == nil {
			t.Fatalf("accepted invalid datasource: %+v", d)
		}
	}
}

func TestMiddlewareRabbitQueueDetailAndArguments(t *testing.T) {
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/bindings"):
			w.Write([]byte(`[{"source":"orders","routing_key":"paid"}]`))
		case r.Method == http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			args, _ := body["arguments"].(map[string]any)
			if args["x-queue-type"] != "quorum" || body["auto_delete"] != true || body["durable"] != true {
				t.Errorf("unexpected declaration %v", body)
			}
			w.WriteHeader(204)
		default:
			w.Write([]byte(`{"name":"team/queue","messages":3}`))
		}
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "rabbitmq", Endpoints: []string{server.URL}, CredentialID: 1}
	c := model.AssetCredential{Username: "ops", Password: "secret"}
	params := map[string]string{"vhost": "/", "name": "team/queue"}
	if _, err := runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "queue.get", Params: params}); err != nil {
		t.Fatal(err)
	}
	if _, err := runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "queue.bindings", Params: params}); err != nil {
		t.Fatal(err)
	}
	declare := map[string]string{"vhost": "/", "name": "team/queue", "arguments": `{"x-queue-type":"quorum"}`, "auto_delete": "true"}
	if _, err := runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "queue.put", Params: declare}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /api/queues/%2F/team%2Fqueue",
		"GET /api/queues/%2F/team%2Fqueue/bindings",
		"PUT /api/queues/%2F/team%2Fqueue",
	}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected requests %v", seen)
	}
	if _, err := runMiddlewareHTTP(context.Background(), d, c, MiddlewareOperation{Operation: "queue.put", Params: map[string]string{"name": "bad", "arguments": "[]"}}); err == nil {
		t.Fatal("non-object arguments must be rejected")
	}
}

func TestMiddlewareNacosConfigContentAndParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v1/cs/configs" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			if q.Get("dataId") != "app.json" || q.Get("tenant") != "dev" || q.Get("group") != "OPS" {
				t.Errorf("missing config identity: %v", q)
			}
			if q.Get("pageNo") != "" || q.Get("search") != "" {
				t.Errorf("read request leaked list parameters: %v", q)
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(`{"feature":{"enabled":true}}`))
		case http.MethodPost:
			r.ParseForm()
			if r.Form.Get("pageNo") != "" || r.Form.Get("search") != "" {
				t.Errorf("publish leaked list parameters: %v", r.Form)
			}
			if r.Form.Get("appName") != "order" || r.Form.Get("desc") != "订单配置" || r.Form.Get("type") != "json" {
				t.Errorf("missing publish metadata: %v", r.Form)
			}
			w.Write([]byte(`true`))
		}
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "nacos", Version: "2", Endpoints: []string{server.URL + "/nacos"}}
	p := map[string]string{"namespaceId": "dev", "groupName": "OPS", "dataId": "app.json"}
	v, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.get", Params: p})
	if err != nil {
		t.Fatal(err)
	}
	if v != `{"feature":{"enabled":true}}` {
		t.Fatalf("config content must stay raw: %v", v)
	}
	publish := map[string]string{"namespaceId": "dev", "groupName": "OPS", "dataId": "app.json", "content": `{"feature":true}`, "type": "json", "appName": "order", "desc": "订单配置"}
	if _, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.put", Params: publish}); err != nil {
		t.Fatal(err)
	}
}

func TestMiddlewareNacosV3ServiceGroupParameter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/console/ns/service/list" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("groupNameParam") != "OPS" || q.Get("serviceNameParam") != "order" || q.Get("namespaceId") != "dev" {
			t.Errorf("v3 service query mismatch: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"data":{"pageItems":[{"name":"order"}],"totalCount":1}}`))
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "nacos", Version: "3", Endpoints: []string{server.URL}}
	p := map[string]string{"namespaceId": "dev", "groupName": "OPS", "serviceName": "order"}
	if _, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "services", Params: p}); err != nil {
		t.Fatal(err)
	}
}

func TestMiddlewareNacosV3ConfigContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/console/cs/config" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"data":{"dataId":"app.json","content":"{\"feature\":true}","type":"json"}}`))
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "nacos", Version: "3", Endpoints: []string{server.URL}}
	v, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.get", Params: map[string]string{"dataId": "app.json", "namespaceId": "public"}})
	if err != nil {
		t.Fatal(err)
	}
	if v != `{"feature":true}` {
		t.Fatalf("v3 config content not extracted: %v", v)
	}
}

func TestMiddlewareNacosV2ConfigContentKeepsJSONField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(`{"content":"business-value","enabled":true}`))
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "nacos", Version: "2", Endpoints: []string{server.URL + "/nacos"}}
	v, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.get", Params: map[string]string{"dataId": "app.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if v != `{"content":"business-value","enabled":true}` {
		t.Fatalf("v2 json config must not be unwrapped: %v", v)
	}
}

func TestMiddlewareNacosV2ConfigContentIsNotAnAPIEnvelope(t *testing.T) {
	for _, content := range []string{`{"code":500,"message":"business state"}`, `{"error":"business error value"}`, `false`, `{"code":0,"data":{"content":"business field"}}`, ""} {
		t.Run(content, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(content))
			}))
			defer server.Close()
			d := model.MiddlewareDatasource{Type: "nacos", Version: "2", Endpoints: []string{server.URL}}
			v, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.get", Params: map[string]string{"dataId": "app.json"}})
			if err != nil || v != content {
				t.Fatalf("configuration content changed or rejected: got %v, error %v", v, err)
			}
		})
	}
}

func TestMiddlewareNacosConfigContentStillRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "nacos", Version: "2", Endpoints: []string{server.URL}}
	if _, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "config.get", Params: map[string]string{"dataId": "app.json"}}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatal("HTTP error was returned as configuration content", err)
	}
}

func TestMiddlewareRabbitExchangeBindingsUseSourceEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/exchanges/%2F/team%2Fexchange/bindings/source" {
			t.Errorf("unexpected exchange bindings request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"source":"team/exchange","destination":"orders","destination_type":"queue"}]`))
	}))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "rabbitmq", Endpoints: []string{server.URL}}
	v, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "exchange.bindings", Params: map[string]string{"name": "team/exchange", "vhost": "/"}})
	if err != nil {
		t.Fatal(err)
	}
	if rows, ok := v.([]any); !ok || len(rows) != 1 {
		t.Fatalf("exchange bindings missing: %v", v)
	}
}

func TestMiddlewareMissingResourceMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer server.Close()
	d := model.MiddlewareDatasource{Type: "rabbitmq", Endpoints: []string{server.URL}}
	_, err := runMiddlewareHTTP(context.Background(), d, model.AssetCredential{}, MiddlewareOperation{Operation: "queue.get", Params: map[string]string{"name": "gone"}})
	if err == nil || !strings.Contains(err.Error(), "资源不存在") {
		t.Fatalf("expected not-found guidance, got %v", err)
	}
}

func TestMiddlewareEtcdKeyPagination(t *testing.T) {
	for _, prefix := range []string{"", "/registry/"} {
		t.Run(prefix, func(t *testing.T) {
			after := prefix + "last"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				decode := func(field string) string {
					value, _ := body[field].(string)
					raw, _ := base64.StdEncoding.DecodeString(value)
					return string(raw)
				}
				if decode("key") != after+"\x00" {
					t.Errorf("cursor must skip previous key: %v", body)
				}
				expectedEnd := "\x00"
				if prefix != "" {
					expectedEnd = string(prefixEnd([]byte(prefix)))
				}
				if decode("range_end") != expectedEnd || body["sort_order"] != "ASCEND" || body["keys_only"] != true {
					t.Errorf("bad pagination range: %v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"kvs":[],"more":false}`))
			}))
			defer server.Close()
			_, err := runMiddlewareHTTP(context.Background(), model.MiddlewareDatasource{Type: "etcd", AuthType: "none", Endpoints: []string{server.URL}}, model.AssetCredential{}, MiddlewareOperation{Operation: "keys", Params: map[string]string{"key": prefix, "after": after, "keysOnly": "true"}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
