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

type v25MutableProviderState struct {
	mu            sync.RWMutex
	sandboxID     string
	incarnationID string
}

func (s *v25MutableProviderState) SetIncarnation(id string) {
	s.mu.Lock()
	s.incarnationID = id
	s.mu.Unlock()
}

func (s *v25MutableProviderState) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	sandboxID := s.sandboxID
	incarnationID := s.incarnationID
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"sandboxID":     sandboxID,
		"incarnationID": incarnationID,
	})
}

type v25BridgeFixture struct {
	controller    *realm.Controller
	store         *realm.MemoryStore
	realization   realm.RealizationAuthority
	resource      ResourceBinding
	epochState    *v24MutableEpochMetrics
	epochObserver *RealizationEpochObserver
	local         RealmResourceEpochAuthority
	providerState *v25MutableProviderState
	client        *Client
	provider      ProviderIncarnationProof
}

func newV25BridgeFixture(t *testing.T) v25BridgeFixture {
	t.Helper()
	const sandboxID = "sandbox-v25"
	ctx := context.Background()
	controller, store, realization, _ := mintV23RealmAuthority(t, sandboxID)
	resource := ResourceBinding{sandboxID: sandboxID}

	epochState := &v24MutableEpochMetrics{
		body: v24EpochMetric(sandboxID, "1", strings.Repeat("71", 32), "1"),
	}
	epochServer := httptestNewServer(t, epochState)
	epochObserver, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: epochServer.URL})
	if err != nil {
		t.Fatalf("new epoch observer: %v", err)
	}
	epochProof := observeV24Epoch(t, epochObserver, resource)
	local, err := ValidateRealmResourceEpochAuthority(ctx, controller, realization, resource, epochObserver, epochProof)
	if err != nil {
		t.Fatalf("mint Wave24 local authority: %v", err)
	}

	providerState := &v25MutableProviderState{
		sandboxID:     sandboxID,
		incarnationID: v25ProviderIncarnationID,
	}
	client, _ := newV25ProviderClient(t, providerState, 0)
	provider, err := client.ObserveProviderIncarnation(ctx, resource)
	if err != nil {
		t.Fatalf("observe provider proof: %v", err)
	}

	return v25BridgeFixture{
		controller:    controller,
		store:         store,
		realization:   realization,
		resource:      resource,
		epochState:    epochState,
		epochObserver: epochObserver,
		local:         local,
		providerState: providerState,
		client:        client,
		provider:      provider,
	}
}

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestV25ExactFreshLocalAndProviderMintBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)
	auth, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(),
		f.controller,
		f.realization,
		f.local,
		f.resource,
		f.epochObserver,
		f.client,
		f.provider,
	)
	if err != nil {
		t.Fatalf("ValidateRealmResourceProviderIncarnationAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("exact Wave25 bridge authority is invalid")
	}
	if sandboxID, ok := auth.SandboxID(); !ok || sandboxID != "sandbox-v25" {
		t.Fatalf("SandboxID=%q ok=%v", sandboxID, ok)
	}
	if local, ok := auth.Local(); !ok || !local.Valid() {
		t.Fatal("Wave25 bridge did not retain sealed Wave24 authority")
	}
	if provider, ok := auth.Provider(); !ok || !provider.Valid() {
		t.Fatal("Wave25 bridge did not retain sealed provider proof")
	}
}

func TestV25StaleProviderSameIDRecreationCannotMintBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)
	f.providerState.SetIncarnation("6f9619ff-8b86-4e2e-aad7-5bf05e8a0f4d")

	_, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, f.local, f.resource,
		f.epochObserver, f.client, f.provider,
	)
	if !errors.Is(err, ErrStaleProviderIncarnationProof) {
		t.Fatalf("stale provider error=%v, want ErrStaleProviderIncarnationProof", err)
	}
}

func TestV25StaleWave24EpochCannotMintProviderBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)
	f.epochState.Set(v24EpochMetric("sandbox-v25", "1", strings.Repeat("72", 32), "1"))

	_, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, f.local, f.resource,
		f.epochObserver, f.client, f.provider,
	)
	if !errors.Is(err, ErrStaleRealizationEpochProof) {
		t.Fatalf("stale Wave24 epoch error=%v, want ErrStaleRealizationEpochProof", err)
	}
}

func TestV25StaleRealmCannotMintProviderBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)
	binding, ok := f.realization.Binding()
	if !ok {
		t.Fatal("invalid Realm authority in fixture")
	}
	if err := f.store.PutWorld(realm.WorldRecord{
		RealmID:             binding.RealmID,
		WorldID:             binding.WorldID,
		RealizationRevision: binding.RealizationRevision + 1,
		Phase:               realm.WorldObservedReady,
		LeaseGeneration:     1,
		LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
		Handle:              substrate.Handle("sandbox-v25"),
	}); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, f.local, f.resource,
		f.epochObserver, f.client, f.provider,
	)
	if !errors.Is(err, realm.ErrStaleRealizationAuthority) {
		t.Fatalf("stale Realm error=%v, want realm.ErrStaleRealizationAuthority", err)
	}
}

func TestV25WrongResourceOrClientCannotMintProviderBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)

	if _, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, f.local,
		ResourceBinding{sandboxID: "sandbox-other"}, f.epochObserver, f.client, f.provider,
	); !errors.Is(err, ErrInvalidRealmResourceProviderIncarnationAuthority) {
		t.Fatalf("wrong resource error=%v", err)
	}

	otherClient, _ := newV25ProviderClient(t, f.providerState, 0)
	if _, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, f.local,
		f.resource, f.epochObserver, otherClient, f.provider,
	); !errors.Is(err, ErrInvalidProviderIncarnationProof) {
		t.Fatalf("wrong client error=%v", err)
	}
}

func TestV25ZeroForgedOrDeserializedInputsCannotMintProviderBridgeAuthority(t *testing.T) {
	f := newV25BridgeFixture(t)

	for name, local := range map[string]RealmResourceEpochAuthority{
		"zero": {},
		"forged": {
			realmResource: f.local.realmResource,
			epoch:         f.local.epoch,
		},
	} {
		t.Run("local-"+name, func(t *testing.T) {
			if _, err := ValidateRealmResourceProviderIncarnationAuthority(
				context.Background(), f.controller, f.realization, local, f.resource,
				f.epochObserver, f.client, f.provider,
			); !errors.Is(err, ErrInvalidRealmResourceProviderIncarnationAuthority) {
				t.Fatalf("%s local error=%v", name, err)
			}
		})
	}

	forgedProvider := ProviderIncarnationProof{
		sandboxID:     "sandbox-v25",
		incarnationID: v25ProviderIncarnationID,
		client:        f.client,
	}
	for name, provider := range map[string]ProviderIncarnationProof{
		"zero":   {},
		"forged": forgedProvider,
	} {
		t.Run("provider-"+name, func(t *testing.T) {
			if _, err := ValidateRealmResourceProviderIncarnationAuthority(
				context.Background(), f.controller, f.realization, f.local, f.resource,
				f.epochObserver, f.client, provider,
			); !errors.Is(err, ErrInvalidProviderIncarnationProof) {
				t.Fatalf("%s provider error=%v", name, err)
			}
		})
	}

	raw, err := json.Marshal(f.local)
	if err != nil {
		t.Fatal(err)
	}
	var deserialized RealmResourceEpochAuthority
	if err := json.Unmarshal(raw, &deserialized); err != nil {
		t.Fatal(err)
	}
	if deserialized.Valid() {
		t.Fatal("JSON round-trip reconstructed Wave24 authority")
	}
	if _, err := ValidateRealmResourceProviderIncarnationAuthority(
		context.Background(), f.controller, f.realization, deserialized, f.resource,
		f.epochObserver, f.client, f.provider,
	); !errors.Is(err, ErrInvalidRealmResourceProviderIncarnationAuthority) {
		t.Fatalf("deserialized local error=%v", err)
	}
}
