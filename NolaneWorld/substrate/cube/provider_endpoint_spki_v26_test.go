package cube

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func v26CertificatePin(cert *x509.Certificate) string {
	digest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(digest[:])
}

func v26HealthHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reached != nil {
			*reached = true
		}
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

func newV26PinnedTLSClient(t *testing.T, server *httptest.Server, pins []string) *Client {
	t.Helper()
	client, err := New(Config{
		APIURL:           server.URL,
		APIKey:           "v26-secret",
		TemplateID:       "tpl-v26",
		HTTPClient:       server.Client(),
		EndpointSPKIPins: pins,
	})
	if err != nil {
		t.Fatalf("New pinned TLS client: %v", err)
	}
	return client
}

func TestV26EndpointSPKIObserveSealsExactPeerKey(t *testing.T) {
	server := httptest.NewTLSServer(v26HealthHandler(nil))
	defer server.Close()
	pin := v26CertificatePin(server.Certificate())
	client := newV26PinnedTLSClient(t, server, []string{pin})

	proof, err := client.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("ObserveProviderEndpointSPKI: %v", err)
	}
	if !proof.Valid() {
		t.Fatal("observed endpoint proof is invalid")
	}
	got, ok := proof.SPKISHA256Hex()
	if !ok || got != pin {
		t.Fatalf("SPKISHA256Hex = (%q,%v), want (%q,true)", got, ok, pin)
	}
}

func TestV26EndpointSPKIObserveRequiresConfiguredPins(t *testing.T) {
	server := httptest.NewTLSServer(v26HealthHandler(nil))
	defer server.Close()
	client, err := New(Config{
		APIURL:     server.URL,
		TemplateID: "tpl-v26",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("New unpinned TLS client: %v", err)
	}

	_, err = client.ObserveProviderEndpointSPKI(context.Background())
	if !errors.Is(err, ErrEndpointTLSAuthorityUnavailable) {
		t.Fatalf("error = %v, want ErrEndpointTLSAuthorityUnavailable", err)
	}
}

func TestV26EndpointSPKIMismatchFailsBeforeHTTPHandler(t *testing.T) {
	trustedServer := httptest.NewTLSServer(v26HealthHandler(nil))
	defer trustedServer.Close()
	trustedPin := v26CertificatePin(trustedServer.Certificate())

	reached := false
	wrongServer := httptest.NewTLSServer(v26HealthHandler(&reached))
	defer wrongServer.Close()
	client := newV26PinnedTLSClient(t, wrongServer, []string{trustedPin})

	_, err := client.ObserveProviderEndpointSPKI(context.Background())
	if !errors.Is(err, ErrEndpointSPKIMismatch) {
		t.Fatalf("error = %v, want ErrEndpointSPKIMismatch", err)
	}
	if reached {
		t.Fatal("HTTP handler ran even though TLS peer SPKI was not pinned")
	}
}

func TestV26EndpointSPKIProofCannotBeForgedOrSerialized(t *testing.T) {
	server := httptest.NewTLSServer(v26HealthHandler(nil))
	defer server.Close()
	pin := v26CertificatePin(server.Certificate())
	client := newV26PinnedTLSClient(t, server, []string{pin})
	proof, err := client.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("observe endpoint: %v", err)
	}

	digest := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	forged := ProviderEndpointSPKIProof{spkiSHA256: digest, client: client}
	if forged.Valid() {
		t.Fatal("descriptive endpoint fields forged authority without private seal")
	}

	raw, err := json.Marshal(proof)
	if err != nil {
		t.Fatalf("marshal endpoint proof: %v", err)
	}
	var reconstructed ProviderEndpointSPKIProof
	if err := json.Unmarshal(raw, &reconstructed); err != nil {
		t.Fatalf("unmarshal endpoint proof: %v", err)
	}
	if reconstructed.Valid() {
		t.Fatal("JSON round-trip reconstructed endpoint authority")
	}
}

func TestV26EndpointSPKIProofIsBoundToExactClient(t *testing.T) {
	server := httptest.NewTLSServer(v26HealthHandler(nil))
	defer server.Close()
	pin := v26CertificatePin(server.Certificate())
	clientA := newV26PinnedTLSClient(t, server, []string{pin})
	clientB := newV26PinnedTLSClient(t, server, []string{pin})

	proof, err := clientA.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("observe with client A: %v", err)
	}
	err = clientB.ValidateProviderEndpointSPKI(context.Background(), proof)
	if !errors.Is(err, ErrInvalidProviderEndpointSPKIProof) {
		t.Fatalf("cross-client error = %v, want ErrInvalidProviderEndpointSPKIProof", err)
	}
}

type v26SwitchingDialer struct {
	mu   sync.RWMutex
	addr string
}

func (d *v26SwitchingDialer) set(addr string) {
	d.mu.Lock()
	d.addr = addr
	d.mu.Unlock()
}

func (d *v26SwitchingDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	d.mu.RLock()
	addr := d.addr
	d.mu.RUnlock()
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, addr)
}

func TestV26EndpointSPKIOverlapRotationMakesOldProofStale(t *testing.T) {
	serverA := httptest.NewTLSServer(v26HealthHandler(nil))
	defer serverA.Close()
	serverB := httptest.NewTLSServer(v26HealthHandler(nil))
	defer serverB.Close()
	pinA := v26CertificatePin(serverA.Certificate())
	pinB := v26CertificatePin(serverB.Certificate())
	if pinA == pinB {
		t.Fatal("rotation fixture unexpectedly reused the same SPKI")
	}

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
		t.Fatalf("New rotating pinned client: %v", err)
	}

	proofA, err := client.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("observe A: %v", err)
	}
	if err := client.ValidateProviderEndpointSPKI(context.Background(), proofA); err != nil {
		t.Fatalf("fresh proof A rejected: %v", err)
	}

	switcher.set(serverB.Listener.Addr().String())
	err = client.ValidateProviderEndpointSPKI(context.Background(), proofA)
	if !errors.Is(err, ErrStaleProviderEndpointSPKIProof) {
		t.Fatalf("rotated old proof error = %v, want ErrStaleProviderEndpointSPKIProof", err)
	}

	proofB, err := client.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("observe B after rotation: %v", err)
	}
	gotB, ok := proofB.SPKISHA256Hex()
	if !ok || gotB != pinB {
		t.Fatalf("rotated proof = (%q,%v), want pin B", gotB, ok)
	}
	if err := client.ValidateProviderEndpointSPKI(context.Background(), proofB); err != nil {
		t.Fatalf("fresh proof B rejected: %v", err)
	}
}
