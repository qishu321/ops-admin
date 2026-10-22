package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"ops-admin/backend/model"
	"strings"
	"testing"
	"time"
)

type testCertificateAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T) *testCertificateAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ops-admin-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCertificateAuthority{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (ca *testCertificateAuthority) issue(t *testing.T, commonName string, usage x509.ExtKeyUsage, ips []net.IP) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func TestMiddlewareTLSConfigAcceptsCAAndClientPair(t *testing.T) {
	ca := newTestCA(t)
	clientCert, clientKey := ca.issue(t, "ops-admin", x509.ExtKeyUsageClientAuth, nil)
	cfg, err := middlewareTLSConfig(model.AssetCredential{AuthType: "tls", CACert: string(ca.pem), ClientCert: string(clientCert), PrivateKey: string(clientKey)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS12 || cfg.RootCAs == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("unexpected tls config %+v", cfg)
	}
	// CA-only credentials are valid for one-way TLS.
	oneWay, err := middlewareTLSConfig(model.AssetCredential{AuthType: "tls", CACert: string(ca.pem)})
	if err != nil || len(oneWay.Certificates) != 0 {
		t.Fatalf("one-way tls rejected: %v %+v", err, oneWay)
	}
}

func TestMiddlewareTLSConfigRejectsBrokenMaterial(t *testing.T) {
	if _, err := middlewareTLSConfig(model.AssetCredential{AuthType: "tls", CACert: "not a certificate"}); err == nil || !strings.Contains(err.Error(), "CA 证书") {
		t.Fatalf("broken CA accepted: %v", err)
	}
	ca := newTestCA(t)
	clientCert, _ := ca.issue(t, "ops-admin", x509.ExtKeyUsageClientAuth, nil)
	if _, err := middlewareTLSConfig(model.AssetCredential{AuthType: "tls", CACert: string(ca.pem), ClientCert: string(clientCert)}); err == nil {
		t.Fatal("certificate without private key accepted")
	}
	if _, err := middlewareTLSConfig(model.AssetCredential{AuthType: "tls", CACert: string(ca.pem), ClientCert: string(ca.pem), PrivateKey: string(ca.pem)}); err == nil {
		t.Fatal("mismatched key pair accepted")
	}
}

func TestRunMiddlewareHTTPEtcdMutualTLS(t *testing.T) {
	ca := newTestCA(t)
	serverCert, serverKey := ca.issue(t, "etcd.test", x509.ExtKeyUsageServerAuth, []net.IP{net.ParseIP("127.0.0.1")})
	clientCert, clientKey := ca.issue(t, "ops-admin", x509.ExtKeyUsageClientAuth, nil)
	pair, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	trusted := x509.NewCertPool()
	if !trusted.AppendCertsFromPEM(ca.pem) {
		t.Fatal("cannot build client CA pool")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/cluster/member/list" {
			t.Errorf("unexpected etcd path %s", r.URL.Path)
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Error("etcd received no client certificate")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"members":[{"ID":"a1","name":"etcd-1"}]}`))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trusted}
	server.StartTLS()
	defer server.Close()

	source := model.MiddlewareDatasource{Type: "etcd", AuthType: "tls", TLS: true, Endpoints: []string{server.URL}}
	credential := model.AssetCredential{AuthType: "tls", CACert: string(ca.pem), ClientCert: string(clientCert), PrivateKey: string(clientKey)}
	value, err := runMiddlewareHTTP(context.Background(), source, credential, MiddlewareOperation{Operation: "members", Params: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(map[string]any)
	if !ok || result["members"] == nil {
		t.Fatalf("unexpected etcd members payload %#v", value)
	}

	// Without the client certificate the mTLS handshake must fail, which proves
	// the credential material is what authenticates the request.
	if _, err := runMiddlewareHTTP(context.Background(), source, model.AssetCredential{AuthType: "tls", CACert: string(ca.pem)}, MiddlewareOperation{Operation: "members"}); err == nil {
		t.Fatal("etcd accepted a connection without client certificate")
	}
}

func TestValidateMiddlewareEtcdTLSUpgradesEndpoint(t *testing.T) {
	source := model.MiddlewareDatasource{Name: "生产 etcd", Type: "etcd", AuthType: "tls", CredentialID: 7, Endpoints: []string{"http://etcd.internal:2379"}}
	if err := validateMiddleware(&source); err != nil {
		t.Fatal(err)
	}
	if source.Endpoints[0] != "https://etcd.internal:2379" || !source.TLS {
		t.Fatalf("endpoint not upgraded: %+v", source)
	}
	// Plain HTTP sources keep their scheme when TLS is off.
	plain := model.MiddlewareDatasource{Name: "测试 etcd", Type: "etcd", AuthType: "none", Endpoints: []string{"http://etcd.internal:2379"}}
	if err := validateMiddleware(&plain); err != nil || plain.Endpoints[0] != "http://etcd.internal:2379" {
		t.Fatalf("plain endpoint changed: %+v %v", plain, err)
	}
	bad := model.MiddlewareDatasource{Name: "证书 etcd", Type: "etcd", AuthType: "ssl", CredentialID: 7, Endpoints: []string{"https://etcd.internal:2379"}}
	if err := validateMiddleware(&bad); err == nil {
		t.Fatal("unknown auth type accepted")
	}
}

func TestValidateTLSCredentialPayload(t *testing.T) {
	caPEM := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"
	keyPEM := "-----BEGIN EC PRIVATE KEY-----\nMIIB\n-----END EC PRIVATE KEY-----"
	cases := []struct {
		name    string
		payload AssetCredentialPayload
		old     model.AssetCredential
		want    string
	}{
		{"ca only", AssetCredentialPayload{AuthType: "tls", CACert: caPEM}, model.AssetCredential{}, ""},
		{"pair", AssetCredentialPayload{AuthType: "tls", ClientCert: caPEM, PrivateKey: keyPEM}, model.AssetCredential{}, ""},
		{"empty", AssetCredentialPayload{AuthType: "tls"}, model.AssetCredential{}, "至少填写 CA 证书"},
		{"cert without key", AssetCredentialPayload{AuthType: "tls", ClientCert: caPEM}, model.AssetCredential{}, "必须同时填写"},
		{"key without cert", AssetCredentialPayload{AuthType: "tls", PrivateKey: keyPEM}, model.AssetCredential{}, "必须同时填写"},
		{"bad ca", AssetCredentialPayload{AuthType: "tls", CACert: "hello"}, model.AssetCredential{}, "PEM 格式"},
		{"bad key", AssetCredentialPayload{AuthType: "tls", ClientCert: caPEM, PrivateKey: "hello"}, model.AssetCredential{}, "私钥应为 PEM"},
		{"masked", AssetCredentialPayload{AuthType: "tls", CACert: "******"}, model.AssetCredential{}, "不会回显"},
		{"keep stored", AssetCredentialPayload{AuthType: "tls", Name: "x"}, model.AssetCredential{AuthType: "tls", CACert: caPEM}, ""},
		{"password credential", AssetCredentialPayload{AuthType: "password"}, model.AssetCredential{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTLSCredentialPayload(tc.payload, tc.old)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
