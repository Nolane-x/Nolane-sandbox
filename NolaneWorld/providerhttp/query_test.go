package providerhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDoQueryUsesCanonicalTrustedQueryWithoutChangingOrigin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/repos/Nolane-x/Nolane-sandbox/commits" {
			t.Fatalf("method=%s path=%q", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "page=2&path=docs%2Fspec-v7.md&per_page=100&sha=release%2Fv7" {
			t.Fatalf("query=%q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL + "/api/v3", RootCAs: rootsForServer(t, server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"per_page": {"100"}, "page": {"2"}, "sha": {"release/v7"}, "path": {"docs/spec-v7.md"}}
	status, _, err := client.DoQuery(context.Background(), http.MethodGet, []string{"repos", "Nolane-x", "Nolane-sandbox", "commits"}, query, nil, nil, 1024)
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestDoQueryRejectsUnsafeKeysAndValues(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("network entered") }))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, RootCAs: rootsForServer(t, server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	cases := []url.Values{
		{"": {"x"}},
		{"key\n": {"x"}},
		{"key": {"x\x00y"}},
		{"key": {string(make([]byte, 4097))}},
	}
	for i, query := range cases {
		if _, _, err := client.DoQuery(context.Background(), http.MethodGet, []string{"meta"}, query, nil, nil, 1024); err != ErrInvalidProviderRoute {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
}
