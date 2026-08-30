package github

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

const syntheticV7Secret = "SYNTHETIC-V7-SECRET"

func githubRoots(server *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return pool
}

func withTestSecret(t *testing.T, fn func(delegation.Secret)) {
	t.Helper()
	if err := delegation.WithSecretLease([]byte(syntheticV7Secret), func(secret delegation.Secret) error {
		fn(secret)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterIssueCommentWritesOnceWithFixedRouteAndMarker(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/repos/Nolane-x/Nolane-sandbox/issues/42/comments" || r.URL.RawQuery != "" {
			t.Fatalf("method=%s url=%s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer "+syntheticV7Secret {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		var body struct{ Body string `json:"body"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		marker := actionMarker("request-digest-comment")
		if body.Body != "hello\n\n<!-- "+marker+" -->" {
			t.Fatalf("body=%q", body.Body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123,"body":"SYNTHETIC-V7-SECRET"}`))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{
		WorldID: "world-v7", ActionID: "action-comment", Operation: OpIssueComment,
		Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42",
		Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "request-digest-comment",
	}
	var effect delegation.Effect
	withTestSecret(t, func(secret delegation.Secret) {
		effect, err = adapter.Execute(context.Background(), request, secret)
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if strings.Contains(string(effect.Evidence), syntheticV7Secret) || strings.Contains(string(effect.Evidence), server.URL) {
		t.Fatalf("unsafe evidence=%s", effect.Evidence)
	}
	if !strings.Contains(string(effect.Evidence), `"object_id":"123"`) {
		t.Fatalf("evidence=%s", effect.Evidence)
	}
}

func TestAdapterContentsWriteUsesTypedPUTAndExpectedSHA(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPut || r.URL.Path != "/repos/Nolane-x/Nolane-sandbox/contents/docs/spec-v7.md" {
			t.Fatalf("method=%s path=%q", r.Method, r.URL.Path)
		}
		var body struct {
			Message string `json:"message"`
			Content string `json:"content"`
			Branch  string `json:"branch"`
			SHA     string `json:"sha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Branch != "release/v7" || body.SHA != "0123456789abcdef0123456789abcdef01234567" || !strings.Contains(body.Message, actionMarker("request-digest-content")) {
			t.Fatalf("body=%+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commit":{"sha":"89abcdef0123456789abcdef0123456789abcdef"}}`))
	}))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{
		WorldID: "world-v7", ActionID: "action-content", Operation: OpContentsWrite,
		Resource: "github:repo:Nolane-x/Nolane-sandbox:contents:docs/spec-v7.md@release/v7",
		Payload: []byte(`{"content_b64":"aGVsbG8=","commit_message":"update spec","expected_blob_sha":"0123456789abcdef0123456789abcdef01234567"}`),
		IdempotencyKey: "request-digest-content",
	}
	var effect delegation.Effect
	withTestSecret(t, func(secret delegation.Secret) { effect, err = adapter.Execute(context.Background(), request, secret) })
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !strings.Contains(string(effect.Evidence), `"object_id":"89abcdef0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("calls=%d evidence=%s", calls.Load(), effect.Evidence)
	}
}

func TestAdapterProviderFailureDoesNotRetryOrLeakProviderText(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("provider says " + syntheticV7Secret))
	}))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{Operation: OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "failure-id"}
	withTestSecret(t, func(secret delegation.Secret) {
		_, err = adapter.Execute(context.Background(), request, secret)
	})
	if !errors.Is(err, ErrProviderRejected) || strings.Contains(err.Error(), syntheticV7Secret) {
		t.Fatalf("err=%v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider was retried: calls=%d", calls.Load())
	}
}

func TestAdapterRejectsUnsupportedOperationBeforeNetwork(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{Operation: "github.admin.delete", Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"x"}`), IdempotencyKey: "bad-op"}
	withTestSecret(t, func(secret delegation.Secret) { _, err = adapter.Execute(context.Background(), request, secret) })
	if !errors.Is(err, ErrInvalidProviderPayload) || calls.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}
