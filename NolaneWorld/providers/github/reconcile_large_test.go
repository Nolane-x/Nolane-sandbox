package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

func TestReconcileAcceptsBoundedPageLargerThanWriteResponseLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marker := actionMarker("large-reconcile")
		body := strings.Repeat("x", 70*1024) + "\n<!-- " + marker + " -->"
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 808, "body": body}})
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, RootCAs: githubRoots(server), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := delegation.AdapterRequest{
		Operation: OpIssueComment, Resource: "github:repo:Nolane-x/Nolane-sandbox:issue:42",
		Payload: []byte(`{"body":"hello"}`), IdempotencyKey: "large-reconcile",
	}
	var result delegation.ReconcileResult
	withTestSecret(t, func(secret delegation.Secret) { result, err = adapter.Reconcile(context.Background(), request, secret) })
	if err != nil || result.State != delegation.ReconcileObserved {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
