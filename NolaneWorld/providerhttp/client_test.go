package providerhttp

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func rootsForServer(t *testing.T, server *httptest.Server) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return pool
}

func TestNewRejectsUnsafeProviderConfiguration(t *testing.T) {
	cases := []Config{
		{},
		{BaseURL: "http://example.com", Timeout: time.Second},
		{BaseURL: "https://user@example.com", Timeout: time.Second},
		{BaseURL: "https://example.com?x=1", Timeout: time.Second},
		{BaseURL: "https://example.com#frag", Timeout: time.Second},
		{BaseURL: "https://example.com/api/../v3", Timeout: time.Second},
		{BaseURL: "https://example.com/api/v3/", Timeout: time.Second},
		{BaseURL: "https://example.com", Timeout: -time.Second},
	}
	for i, cfg := range cases {
		if client, err := New(cfg); !errors.Is(err, ErrInvalidProviderConfig) || client != nil {
			t.Fatalf("case %d client=%v err=%v", i, client, err)
		}
	}
}

func TestNewOwnsTransportAndIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, RootCAs: rootsForServer(t, server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("provider runtime does not own a proxy-free transport")
	}
	status, _, err := client.Do(context.Background(), http.MethodGet, []string{"meta"}, nil, nil, 1024)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("status=%d err=%v proxy=%q", status, err, os.Getenv("HTTPS_PROXY"))
	}
}

func TestDoPinsBasePathAndRejectsTraversalSegments(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/Nolane-x/Nolane-sandbox" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL + "/api/v3", RootCAs: rootsForServer(t, server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Do(context.Background(), http.MethodGet, []string{"repos", "..", "escape"}, nil, nil, 1024); !errors.Is(err, ErrInvalidProviderRoute) {
		t.Fatalf("traversal err=%v", err)
	}
	status, _, err := client.Do(context.Background(), http.MethodGet, []string{"repos", "Nolane-x", "Nolane-sandbox"}, nil, nil, 1024)
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestDoDoesNotFollowRedirectOrForwardAuthorization(t *testing.T) {
	var targetCalls atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Fatal("authorization forwarded across redirect")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer SYNTHETIC-V7-SECRET" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Location", target.URL+"/steal")
		w.WriteHeader(http.StatusFound)
	}))
	defer origin.Close()

	pool := x509.NewCertPool()
	pool.AddCert(origin.Certificate())
	pool.AddCert(target.Certificate())
	client, err := New(Config{BaseURL: origin.URL, RootCAs: pool, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"Authorization": []string{"Bearer SYNTHETIC-V7-SECRET"}}
	status, _, err := client.Do(context.Background(), http.MethodGet, []string{"redirect"}, headers, nil, 1024)
	if err != nil || status != http.StatusFound {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if targetCalls.Load() != 0 {
		t.Fatalf("redirect target calls=%d", targetCalls.Load())
	}
}

func TestDoBoundsProviderResponseWithoutLeakingBody(t *testing.T) {
	secretText := "provider-error-SYNTHETIC-V7-SECRET"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(secretText, 100)))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, RootCAs: rootsForServer(t, server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := client.Do(context.Background(), http.MethodGet, []string{"large"}, nil, nil, 64)
	if !errors.Is(err, ErrProviderResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
	if len(body) != 0 || strings.Contains(err.Error(), secretText) {
		t.Fatalf("body=%q err=%v", body, err)
	}
}
