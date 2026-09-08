package cube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const v25ProviderIncarnationID = "550e8400-e29b-41d4-a716-446655440000"

func newV25ProviderClient(t *testing.T, handler http.Handler, maxBytes int64) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := New(Config{
		APIURL:           server.URL,
		APIKey:           "v25-secret",
		TemplateID:       "tpl-v25",
		MaxResponseBytes: maxBytes,
		HTTPClient:       server.Client(),
	})
	if err != nil {
		server.Close()
		t.Fatalf("new Cube client: %v", err)
	}
	t.Cleanup(server.Close)
	return client, server
}

func v25ProviderJSONHandler(t *testing.T, sandboxID, incarnationID string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/sandboxes/sandbox-v25" {
			t.Errorf("path = %q, want /sandboxes/sandbox-v25", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := r.Header.Get("X-API-Key"); got != "v25-secret" {
			t.Errorf("X-API-Key = %q, want configured key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sandboxID":     sandboxID,
			"incarnationID": incarnationID,
		})
	})
}

func TestV25ProviderIncarnationObserveSealsExactProviderIdentity(t *testing.T) {
	client, _ := newV25ProviderClient(
		t,
		v25ProviderJSONHandler(t, "sandbox-v25", v25ProviderIncarnationID),
		0,
	)
	binding := ResourceBinding{sandboxID: "sandbox-v25"}

	proof, err := client.ObserveProviderIncarnation(context.Background(), binding)
	if err != nil {
		t.Fatalf("observe provider incarnation: %v", err)
	}
	if !proof.Valid() {
		t.Fatal("observed provider incarnation proof is invalid")
	}
	sandboxID, ok := proof.SandboxID()
	incarnationID, ok2 := proof.IncarnationID()
	if !ok || !ok2 || sandboxID != "sandbox-v25" || incarnationID != v25ProviderIncarnationID {
		t.Fatalf("proof diagnostics = (%q,%v,%q,%v), want exact provider identity", sandboxID, ok, incarnationID, ok2)
	}
}

func TestV25ProviderIncarnationObserveRejectsMalformedOrMismatchedIdentity(t *testing.T) {
	cases := []struct {
		name          string
		sandboxID     string
		incarnationID string
	}{
		{name: "missing incarnation", sandboxID: "sandbox-v25"},
		{name: "empty sandbox", incarnationID: v25ProviderIncarnationID},
		{name: "wrong sandbox", sandboxID: "sandbox-other", incarnationID: v25ProviderIncarnationID},
		{name: "uppercase", sandboxID: "sandbox-v25", incarnationID: "550E8400-E29B-41D4-A716-446655440000"},
		{name: "uuid v1", sandboxID: "sandbox-v25", incarnationID: "550e8400-e29b-11d4-a716-446655440000"},
		{name: "malformed", sandboxID: "sandbox-v25", incarnationID: "not-a-uuid"},
		{name: "non canonical", sandboxID: "sandbox-v25", incarnationID: "550e8400e29b41d4a716446655440000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newV25ProviderClient(t, v25ProviderJSONHandler(t, tc.sandboxID, tc.incarnationID), 0)
			proof, err := client.ObserveProviderIncarnation(
				context.Background(),
				ResourceBinding{sandboxID: "sandbox-v25"},
			)
			if err == nil {
				t.Fatalf("invalid provider identity minted proof: %#v", proof)
			}
			if proof.Valid() {
				t.Fatal("failed observation returned a valid proof")
			}
		})
	}
}

func TestV25ProviderIncarnationObservePreservesClientTransportLimits(t *testing.T) {
	t.Run("non 2xx", func(t *testing.T) {
		client, _ := newV25ProviderClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}), 0)
		if _, err := client.ObserveProviderIncarnation(context.Background(), ResourceBinding{sandboxID: "sandbox-v25"}); err == nil {
			t.Fatal("non-2xx provider response was accepted")
		}
	})

	t.Run("response size", func(t *testing.T) {
		client, _ := newV25ProviderClient(t, v25ProviderJSONHandler(t, "sandbox-v25", v25ProviderIncarnationID), 24)
		_, err := client.ObserveProviderIncarnation(context.Background(), ResourceBinding{sandboxID: "sandbox-v25"})
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("oversized response error = %v, want ErrResponseTooLarge", err)
		}
	})
}

func TestV25ProviderIncarnationProofCannotBeForgedOrReconstructed(t *testing.T) {
	client, _ := newV25ProviderClient(
		t,
		v25ProviderJSONHandler(t, "sandbox-v25", v25ProviderIncarnationID),
		0,
	)
	binding := ResourceBinding{sandboxID: "sandbox-v25"}
	proof, err := client.ObserveProviderIncarnation(context.Background(), binding)
	if err != nil {
		t.Fatalf("observe provider incarnation: %v", err)
	}

	forged := ProviderIncarnationProof{
		sandboxID:     "sandbox-v25",
		incarnationID: v25ProviderIncarnationID,
		client:        client,
	}
	if forged.Valid() {
		t.Fatal("descriptive fields forged provider authority without the private seal")
	}

	raw, err := json.Marshal(proof)
	if err != nil {
		t.Fatalf("marshal proof: %v", err)
	}
	var reconstructed ProviderIncarnationProof
	if err := json.Unmarshal(raw, &reconstructed); err != nil {
		t.Fatalf("unmarshal proof: %v", err)
	}
	if reconstructed.Valid() {
		t.Fatal("JSON round-trip reconstructed provider authority")
	}
}

func TestV25ProviderIncarnationProofIsBoundToExactClientContext(t *testing.T) {
	clientA, _ := newV25ProviderClient(
		t,
		v25ProviderJSONHandler(t, "sandbox-v25", v25ProviderIncarnationID),
		0,
	)
	clientB, _ := newV25ProviderClient(
		t,
		v25ProviderJSONHandler(t, "sandbox-v25", v25ProviderIncarnationID),
		0,
	)
	binding := ResourceBinding{sandboxID: "sandbox-v25"}
	proof, err := clientA.ObserveProviderIncarnation(context.Background(), binding)
	if err != nil {
		t.Fatalf("observe with client A: %v", err)
	}

	err = clientB.ValidateProviderIncarnation(context.Background(), binding, proof)
	if !errors.Is(err, ErrInvalidProviderIncarnationProof) {
		t.Fatalf("cross-client validation error = %v, want ErrInvalidProviderIncarnationProof", err)
	}
}

func TestV25ProviderIncarnationFreshValidationRejectsRemoteSameIDRecreation(t *testing.T) {
	var mu sync.RWMutex
	current := v25ProviderIncarnationID
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.RLock()
		incarnationID := current
		mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sandboxID":     "sandbox-v25",
			"incarnationID": incarnationID,
		})
	})
	client, _ := newV25ProviderClient(t, handler, 0)
	binding := ResourceBinding{sandboxID: "sandbox-v25"}
	proof, err := client.ObserveProviderIncarnation(context.Background(), binding)
	if err != nil {
		t.Fatalf("observe original incarnation: %v", err)
	}
	if err := client.ValidateProviderIncarnation(context.Background(), binding, proof); err != nil {
		t.Fatalf("fresh proof rejected before recreation: %v", err)
	}

	mu.Lock()
	current = "6f9619ff-8b86-4e2e-aad7-5bf05e8a0f4d"
	mu.Unlock()

	err = client.ValidateProviderIncarnation(context.Background(), binding, proof)
	if !errors.Is(err, ErrStaleProviderIncarnationProof) {
		t.Fatalf("stale proof validation error = %v, want ErrStaleProviderIncarnationProof", err)
	}
}
