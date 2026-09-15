package middleware

import "testing"

func TestGlobalReadOnlyRequestAllowed(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
		want   bool
	}{
		{"GET", "/api/v1/monitor/overview", true},
		{"POST", "/api/v1/monitor/query/range", true},
		{"POST", "/api/v1/monitor/inspection/run", false},
		{"PUT", "/api/v1/asset/host/update", false},
		{"GET", "/api/v1/asset/credential/list", false},
		{"GET", "/api/v1/asset/service/diagnosis/run", false},
	} {
		if got := globalReadOnlyRequestAllowed(test.method, test.path); got != test.want {
			t.Errorf("globalReadOnlyRequestAllowed(%q, %q) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}
