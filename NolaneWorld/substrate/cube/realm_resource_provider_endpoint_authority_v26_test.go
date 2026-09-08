package cube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

const v26ProviderIncarnationID = "7f9619ff-8b86-4e2e-aad7-5bf05e8a0f4d"

type v26MutableProviderEndpointState struct {
	mu            sync.RWMutex
	sandboxID     string
	incarnationID string
}

func (s *v26MutableProviderEndpointState) SetIncarnation(id string) {
	s.mu.Lock()
	s.incarnationID = id
	s.mu.Unlock()
}

func (s *v26MutableProviderEndpointState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return
	}

	s.mu.RLock()
	sandboxID := s.sandboxID
	incarnationID := s.incarnationID
	s.mu.RUnlock()
	if r.Method != http.MethodGet || r.URL.Path != "/sandboxes/"+sandboxID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"sandboxID":     sandboxID,
		"incarnationID": incarnationID,
	})
}

type v26BridgeFixture struct {
	controller     *realm.Controller
	store          *realm.MemoryStore
	realization    realm.RealizationAuthority
	resource       ResourceBinding
	epochState     *v24MutableEpochMetrics
	epochObserver  *RealizationEpochObserver
	providerState  *v26MutableProviderEndpointState
	providerServer interface{ Close() }
	client         *Client
	wave25         RealmResourceProviderIncarnationAuthority
	endpoint       ProviderEndpointSPKIProof
	endpointPin    string
}

func newV26BridgeFixture(t *testing.T) v26BridgeFixture {
	t.Helper()
	const sandboxID = "sandbox-v26"
	ctx := context.Background()
	controller, store, realization, _ := mintV23RealmAuthority(t, sandboxID)
	resource := ResourceBinding{sandboxID: sandboxID}

	epochState := &v24MutableEpochMetrics{
		body: v24EpochMetric(sandboxID, "1", strings.Repeat("81", 32), "1"),
	}
	epochServer := httptestNewServer(t, epochState)
	epochObserver, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: epochServer.URL})
	if err != nil {
		t.Fatalf("new epoch observer: %v", err)
	}
	epochProof := observeV24Epoch(t, epochObserver, resource)
	local, err := ValidateRealmResourceEpochAuthority(ctx, controller, realization, resource, epochObserver, epochProof)
	if err != nil {
		t.Fatalf("mint Wave24 authority: %v", err)
	}

	providerState := &v26MutableProviderEndpointState{
		sandboxID:     sandboxID,
		incarnationID: v26ProviderIncarnationID,
	}
	providerServer := newV26UniqueTLSServer(t, providerState)
	pin := v26CertificatePin(providerServer.Certificate())
	client := newV26PinnedTLSClient(t, providerServer, []string{pin})
	provider, err := client.ObserveProviderIncarnation(ctx, resource)
	if err != nil {
		t.Fatalf("observe provider incarnation: %v", err)
	}
	wave25, err := ValidateRealmResourceProviderIncarnationAuthority(
		ctx, controller, realization, local, resource, epochObserver, client, provider,
	)
	if err != nil {
		t.Fatalf("mint Wave25 authority: %v", err)
	}
	endpoint, err := client.ObserveProviderEndpointSPKI(ctx)
	if err != nil {
		t.Fatalf("observe endpoint SPKI: %v", err)
	}

	return v26BridgeFixture{
		controller:     controller,
		store:          store,
		realization:    realization,
		resource:       resource,
		epochState:     epochState,
		epochObserver:  epochObserver,
		providerState:  providerState,
		providerServer: providerServer,
		client:         client,
		wave25:         wave25,
		endpoint:       endpoint,
		endpointPin:    pin,
	}
}

func TestV26ExactFreshWave25AndEndpointMintBridgeAuthority(t *testing.T) {
	f := newV26BridgeFixture(t)
	auth, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, f.endpoint,
	)
	if err != nil {
		t.Fatalf("ValidateRealmResourceProviderEndpointAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("exact Wave26 bridge authority is invalid")
	}
	if sandboxID, ok := auth.SandboxID(); !ok || sandboxID != "sandbox-v26" {
		t.Fatalf("SandboxID=(%q,%v)", sandboxID, ok)
	}
	if wave25, ok := auth.ProviderIncarnation(); !ok || !wave25.Valid() {
		t.Fatal("Wave26 bridge did not retain sealed Wave25 authority")
	}
	if endpoint, ok := auth.Endpoint(); !ok || !endpoint.Valid() {
		t.Fatal("Wave26 bridge did not retain sealed endpoint proof")
	}
}

func TestV26StaleProviderIncarnationCannotMintEndpointBridge(t *testing.T) {
	f := newV26BridgeFixture(t)
	f.providerState.SetIncarnation("8f9619ff-8b86-4e2e-aad7-5bf05e8a0f4d")

	_, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, f.endpoint,
	)
	if !errors.Is(err, ErrStaleProviderIncarnationProof) {
		t.Fatalf("stale provider error=%v, want ErrStaleProviderIncarnationProof", err)
	}
}

func TestV26StaleWave24EpochCannotMintEndpointBridge(t *testing.T) {
	f := newV26BridgeFixture(t)
	f.epochState.Set(v24EpochMetric("sandbox-v26", "1", strings.Repeat("82", 32), "1"))

	_, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, f.endpoint,
	)
	if !errors.Is(err, ErrStaleRealizationEpochProof) {
		t.Fatalf("stale epoch error=%v, want ErrStaleRealizationEpochProof", err)
	}
}

func TestV26StaleRealmCannotMintEndpointBridge(t *testing.T) {
	f := newV26BridgeFixture(t)
	binding, ok := f.realization.Binding()
	if !ok {
		t.Fatal("invalid realization fixture")
	}
	if err := f.store.PutWorld(realm.WorldRecord{
		RealmID:             binding.RealmID,
		WorldID:             binding.WorldID,
		RealizationRevision: binding.RealizationRevision + 1,
		Phase:               realm.WorldObservedReady,
		LeaseGeneration:     1,
		LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
		Handle:              substrate.Handle("sandbox-v26"),
	}); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, f.endpoint,
	)
	if !errors.Is(err, realm.ErrStaleRealizationAuthority) {
		t.Fatalf("stale Realm error=%v, want realm.ErrStaleRealizationAuthority", err)
	}
}

func TestV26WrongResourceOrCrossClientEndpointCannotMintBridge(t *testing.T) {
	f := newV26BridgeFixture(t)

	if _, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25,
		ResourceBinding{sandboxID: "sandbox-other"}, f.epochObserver, f.client, f.endpoint,
	); !errors.Is(err, ErrInvalidRealmResourceProviderEndpointAuthority) {
		t.Fatalf("wrong resource error=%v", err)
	}

	server := newV26UniqueTLSServer(t, f.providerState)
	pin := v26CertificatePin(server.Certificate())
	otherClient := newV26PinnedTLSClient(t, server, []string{pin})
	otherEndpoint, err := otherClient.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("observe other endpoint: %v", err)
	}
	if _, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, otherEndpoint,
	); !errors.Is(err, ErrInvalidProviderEndpointSPKIProof) {
		t.Fatalf("cross-client endpoint error=%v", err)
	}
}

func TestV26ForgedOrDeserializedEndpointCannotMintBridge(t *testing.T) {
	f := newV26BridgeFixture(t)
	digest := f.endpoint.spkiSHA256
	forged := ProviderEndpointSPKIProof{spkiSHA256: digest, client: f.client}
	for name, endpoint := range map[string]ProviderEndpointSPKIProof{
		"zero":   {},
		"forged": forged,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateRealmResourceProviderEndpointAuthority(
				context.Background(), f.controller, f.realization, f.wave25, f.resource,
				f.epochObserver, f.client, endpoint,
			); !errors.Is(err, ErrInvalidProviderEndpointSPKIProof) {
				t.Fatalf("%s endpoint error=%v", name, err)
			}
		})
	}

	raw, err := json.Marshal(f.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var deserialized ProviderEndpointSPKIProof
	if err := json.Unmarshal(raw, &deserialized); err != nil {
		t.Fatal(err)
	}
	if deserialized.Valid() {
		t.Fatal("JSON round-trip reconstructed endpoint authority")
	}
	if _, err := ValidateRealmResourceProviderEndpointAuthority(
		context.Background(), f.controller, f.realization, f.wave25, f.resource,
		f.epochObserver, f.client, deserialized,
	); !errors.Is(err, ErrInvalidProviderEndpointSPKIProof) {
		t.Fatalf("deserialized endpoint error=%v", err)
	}
}

func TestV26EndpointRotationMakesOldBridgeProofStale(t *testing.T) {
	const sandboxID = "sandbox-v26-rotate"
	ctx := context.Background()
	controller, _, realization, _ := mintV23RealmAuthority(t, sandboxID)
	resource := ResourceBinding{sandboxID: sandboxID}
	epochState := &v24MutableEpochMetrics{
		body: v24EpochMetric(sandboxID, "1", strings.Repeat("83", 32), "1"),
	}
	epochServer := httptestNewServer(t, epochState)
	epochObserver, err := NewRealizationEpochObserver(RealizationEpochConfig{BaseURL: epochServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	epochProof := observeV24Epoch(t, epochObserver, resource)
	local, err := ValidateRealmResourceEpochAuthority(ctx, controller, realization, resource, epochObserver, epochProof)
	if err != nil {
		t.Fatal(err)
	}

	providerState := &v26MutableProviderEndpointState{sandboxID: sandboxID, incarnationID: v26ProviderIncarnationID}
	serverA := newV26UniqueTLSServer(t, providerState)
	serverB := newV26UniqueTLSServer(t, providerState)
	pinA := v26CertificatePin(serverA.Certificate())
	pinB := v26CertificatePin(serverB.Certificate())
	roots := x509.NewCertPool()
	roots.AddCert(serverA.Certificate())
	roots.AddCert(serverB.Certificate())
	switcher := &v26SwitchingDialer{addr: serverA.Listener.Addr().String()}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.DialContext = switcher.DialContext
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	client, err := New(Config{
		APIURL:           serverA.URL,
		TemplateID:       "tpl-v26",
		HTTPClient:       &http.Client{Transport: transport},
		EndpointSPKIPins: []string{pinA, pinB},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := client.ObserveProviderIncarnation(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}
	wave25, err := ValidateRealmResourceProviderIncarnationAuthority(
		ctx, controller, realization, local, resource, epochObserver, client, provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	endpointA, err := client.ObserveProviderEndpointSPKI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	switcher.set(serverB.Listener.Addr().String())
	_, err = ValidateRealmResourceProviderEndpointAuthority(
		ctx, controller, realization, wave25, resource, epochObserver, client, endpointA,
	)
	if !errors.Is(err, ErrStaleProviderEndpointSPKIProof) {
		t.Fatalf("rotated endpoint error=%v, want ErrStaleProviderEndpointSPKIProof", err)
	}
}
