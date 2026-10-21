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
