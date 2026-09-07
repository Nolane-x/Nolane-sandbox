package cube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

type v24MutableEpochMetrics struct {
	mu   sync.RWMutex
	body string
}

func (m *v24MutableEpochMetrics) Set(body string) {
	m.mu.Lock()
	m.body = body
	m.mu.Unlock()
}

func (m *v24MutableEpochMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	body := m.body
	m.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func observeV24Epoch(t *testing.T, observer *RealizationEpochObserver, resource ResourceBinding) RealizationEpochProof {
	t.Helper()
	proof, ok, err := observer.Observe(context.Background(), resource)
	if err != nil {
		t.Fatalf("Observe epoch: %v", err)
	}
	if !ok || !proof.Valid() {
		t.Fatalf("epoch proof invalid: ok=%v proof=%+v", ok, proof)
	}
	return proof
}

func TestV24ExactFreshRealmResourceAndCurrentEpochMintBridgeAuthority(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-exact"}
	metrics := &v24MutableEpochMetrics{body: v24EpochMetric("sandbox-exact", "1", strings.Repeat("71", 32), "1")}
	server := httptest.NewServer(metrics)
	defer server.Close()
	observer, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	proof := observeV24Epoch(t, observer, resource)

	auth, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, proof)
	if err != nil {
		t.Fatalf("ValidateRealmResourceEpochAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("exact Wave24 tuple returned invalid bridge authority")
	}
	if sandboxID, ok := auth.SandboxID(); !ok || sandboxID != "sandbox-exact" {
		t.Fatalf("SandboxID=%q ok=%v", sandboxID, ok)
	}
	epoch, ok := auth.Epoch()
	if !ok || !epoch.Valid() {
		t.Fatal("bridge did not retain sealed epoch proof")
	}
}

func TestV24WrongSandboxCannotMintBridgeAuthority(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-other"}
	metrics := &v24MutableEpochMetrics{body: v24EpochMetric("sandbox-other", "1", strings.Repeat("71", 32), "1")}
	server := httptest.NewServer(metrics)
	defer server.Close()
	observer, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	proof := observeV24Epoch(t, observer, resource)

	if _, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, proof); !errors.Is(err, ErrRealmResourceAuthorityMismatch) {
		t.Fatalf("wrong sandbox err=%v", err)
	}
}

func TestV24StaleRealmAuthorityCannotMintBridgeAuthority(t *testing.T) {
	ctl, store, realmAuth, worldID := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-exact"}
	metrics := &v24MutableEpochMetrics{body: v24EpochMetric("sandbox-exact", "1", strings.Repeat("71", 32), "1")}
	server := httptest.NewServer(metrics)
	defer server.Close()
	observer, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	proof := observeV24Epoch(t, observer, resource)

	binding, ok := realmAuth.Binding()
	if !ok {
		t.Fatal("invalid Realm authority")
	}
	if err := store.PutWorld(realm.WorldRecord{
		RealmID: binding.RealmID, WorldID: worldID, RealizationRevision: binding.RealizationRevision + 1,
		Phase: realm.WorldObservedReady, LeaseGeneration: 1,
		LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle("sandbox-exact"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, proof); !errors.Is(err, realm.ErrStaleRealizationAuthority) {
		t.Fatalf("stale Realm authority err=%v", err)
	}
}

func TestV24OldEpochFailsAfterProducerReRealization(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-exact"}
	metrics := &v24MutableEpochMetrics{body: v24EpochMetric("sandbox-exact", "1", strings.Repeat("71", 32), "1")}
	server := httptest.NewServer(metrics)
	defer server.Close()
	observer, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	oldProof := observeV24Epoch(t, observer, resource)

	// Simulate Clear -> Start for the same literal sandbox. Numeric generation
	// may reset to 1, so only the new producer-owned token distinguishes it.
	metrics.Set(v24EpochMetric("sandbox-exact", "1", strings.Repeat("72", 32), "1"))
	if _, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, oldProof); !errors.Is(err, ErrStaleRealizationEpochProof) {
		t.Fatalf("old epoch after re-realization err=%v", err)
	}
}

func TestV24ForgedZeroOrDeserializedEpochCannotMintBridgeAuthority(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-exact"}
	metrics := &v24MutableEpochMetrics{body: v24EpochMetric("sandbox-exact", "1", strings.Repeat("71", 32), "1")}
	server := httptest.NewServer(metrics)
	defer server.Close()
	observer, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	valid := observeV24Epoch(t, observer, resource)

	var token [32]byte
	for i := range token {
		token[i] = byte(i + 1)
	}
	forged := RealizationEpochProof{sandboxID: "sandbox-exact", generation: 1, token: token}
	for name, proof := range map[string]RealizationEpochProof{
		"zero":   {},
		"forged": forged,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, proof); !errors.Is(err, ErrInvalidRealizationEpochProof) {
				t.Fatalf("%s proof err=%v", name, err)
			}
		})
	}

	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var deserialized RealizationEpochProof
	if err := json.Unmarshal(raw, &deserialized); err != nil {
		t.Fatal(err)
	}
	if deserialized.Valid() {
		t.Fatal("JSON round-trip reconstructed Wave24 epoch authority")
	}
	if _, err := ValidateRealmResourceEpochAuthority(context.Background(), ctl, realmAuth, resource, observer, deserialized); !errors.Is(err, ErrInvalidRealizationEpochProof) {
		t.Fatalf("deserialized proof err=%v", err)
	}
}
