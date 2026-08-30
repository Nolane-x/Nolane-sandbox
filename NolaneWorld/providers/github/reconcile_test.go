package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestReconcileIssueCommentFindsExactMarkerReadOnly(t *testing.T) {
	var getCalls, writeCalls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeCalls.Add(1)
			t.Fatalf("reconciliation entered write method %s", r.Method)
		}
		getCalls.Add(1)
		if r.URL.Path != "/repos/Nolane-x/Nolane-sandbox/issues/42/comments" || r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("url=%s", r.URL.String())
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		marker := actionMarker("reconcile-comment")
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "body": "other"}})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 77, "body": "hello\n\n<!-- " + marker + " -->"}})
	}))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{Operation: OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "reconcile-comment"}
	var result delegation.ReconcileResult
	withTestSecret(t, func(secret delegation.Secret) { result, err = adapter.Reconcile(context.Background(), request, secret) })
	if err != nil || result.State != delegation.ReconcileObserved {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if getCalls.Load() != 2 || writeCalls.Load() != 0 || !strings.Contains(string(result.Evidence), `"object_id":"77"`) {
		t.Fatalf("get=%d write=%d evidence=%s", getCalls.Load(), writeCalls.Load(), result.Evidence)
	}
}

func TestReconcileBoundedCommentNonObservationIsUnknown(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		items := make([]map[string]any, 100)
		for i := range items {
			items[i] = map[string]any{"id": i + 1, "body": fmt.Sprintf("unrelated-%d", i)}
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{Operation: OpPullComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:pull:7", Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "missing-marker"}
	var result delegation.ReconcileResult
	withTestSecret(t, func(secret delegation.Secret) { result, err = adapter.Reconcile(context.Background(), request, secret) })
	if err != nil || result.State != delegation.ReconcileUnknown || calls.Load() != 10 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls.Load())
	}
}

func TestReconcileContentsFindsMarkerAndNeverWrites(t *testing.T) {
	var writes atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
			t.Fatalf("write method=%s", r.Method)
		}
		q := r.URL.Query()
		if r.URL.Path != "/repos/Nolane-x/Nolane-sandbox/commits" || q.Get("sha") != "release/v7" || q.Get("path") != "docs/spec-v7.md" || q.Get("per_page") != "100" {
			t.Fatalf("url=%s", r.URL.String())
		}
		marker := actionMarker("reconcile-content")
		_, _ = w.Write([]byte(`[{"sha":"89abcdef0123456789abcdef0123456789abcdef","commit":{"message":"update\n\n` + marker + `"}}]`))
	}))
	defer server.Close()
	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{Operation: OpContentsWrite, Resource: "github:repo:Nolane-x/Nolane-sandbox:contents:docs/spec-v7.md@release/v7", Payload: []byte(`{"content_b64":"aGVsbG8=","commit_message":"update spec"}`), IdempotencyKey: "reconcile-content"}
	var result delegation.ReconcileResult
	withTestSecret(t, func(secret delegation.Secret) { result, err = adapter.Reconcile(context.Background(), request, secret) })
	if err != nil || result.State != delegation.ReconcileObserved || writes.Load() != 0 || !strings.Contains(string(result.Evidence), `"object_id":"89abcdef0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("result=%+v err=%v writes=%d", result, err, writes.Load())
	}
}

func TestPlaneUncertainGitHubWriteReconcilesWithoutSecondWrite(t *testing.T) {
	var postCalls, getCalls atomic.Int64
	requestDigestMarker := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			postCalls.Add(1)
			var body struct{ Body string `json:"body"` }
			_ = json.NewDecoder(r.Body).Decode(&body)
			start := strings.Index(body.Body, markerPrefix)
			if start >= 0 {
				requestDigestMarker = strings.TrimSuffix(body.Body[start:], " -->")
			}
			w.WriteHeader(http.StatusInternalServerError)
		case http.MethodGet:
			getCalls.Add(1)
			_, _ = w.Write([]byte(`[{"id":909,"body":"hello\n\n<!-- ` + requestDigestMarker + ` -->"}]`))
		default:
			t.Fatalf("method=%s", r.Method)
		}
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := world.NewState("world-v7")
	store := delegation.NewMemoryStore()
	grant := delegation.Grant{
		ID: "grant-v7", WorldID: "world-v7", AuthorityEpoch: 1, Adapter: Kind,
		Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42", Operations: []delegation.Operation{OpIssueComment},
		SecretHandle: "github-token", IssuedAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Issue(grant); err != nil {
		t.Fatal(err)
	}
	vault := delegation.NewMemoryVault()
	if err := vault.Put(grant.SecretHandle, []byte(syntheticV7Secret)); err != nil {
		t.Fatal(err)
	}
	registry, err := delegation.NewRegistry(adapter)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := authority.OpenJournalLedger(filepath.Join(t.TempDir(), "effects.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	plane, err := delegation.NewPlane(state, store, vault, registry, ledger, func() time.Time { return time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	intent := delegation.Intent{WorldID: "world-v7", AuthorityEpoch: 1, ActionID: "action-v7", DelegationID: "grant-v7", Operation: OpIssueComment, Resource: grant.Resource, Payload: []byte(`{"body":"hello"}`)}
	if _, err := plane.Execute(context.Background(), intent); !errors.Is(err, delegation.ErrAdapterFailure) {
		t.Fatalf("execute err=%v", err)
	}
	if _, err := plane.Execute(context.Background(), intent); !errors.Is(err, authority.ErrActionUncertain) {
		t.Fatalf("retry err=%v", err)
	}
	if postCalls.Load() != 1 {
		t.Fatalf("post calls=%d", postCalls.Load())
	}
	receipt, err := plane.Reconcile(context.Background(), intent)
	if err != nil || receipt.EffectDigest == "" || postCalls.Load() != 1 || getCalls.Load() == 0 {
		t.Fatalf("receipt=%+v err=%v post=%d get=%d", receipt, err, postCalls.Load(), getCalls.Load())
	}
}
