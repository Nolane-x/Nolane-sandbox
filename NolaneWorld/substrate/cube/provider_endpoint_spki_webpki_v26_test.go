package cube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"testing"
)

func TestV26PinnedEndpointStillRequiresTrustedCertificateChain(t *testing.T) {
	server := newV26UniqueTLSServer(t, v26HealthHandler(nil))
	pin := v26CertificatePin(server.Certificate())

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client, err := New(Config{
		APIURL:           server.URL,
		TemplateID:       "tpl-v26",
		HTTPClient:       &http.Client{Transport: transport},
		EndpointSPKIPins: []string{pin},
	})
	if err != nil {
		t.Fatalf("New pinned client: %v", err)
	}

	_, err = client.ObserveProviderEndpointSPKI(context.Background())
	if err == nil {
		t.Fatal("self-signed peer was trusted solely because its SPKI was pinned")
	}
	if !errors.Is(err, ErrEndpointTLSAuthorityUnavailable) {
		t.Fatalf("chain validation error = %v, want wrapped ErrEndpointTLSAuthorityUnavailable", err)
	}
}

func TestV26PinnedEndpointStillRequiresHostnameValidation(t *testing.T) {
	server := newV26UniqueTLSServer(t, v26HealthHandler(nil))
	pin := v26CertificatePin(server.Certificate())
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{
		RootCAs:    roots,
		ServerName: "wrong-host.invalid",
	}
	client, err := New(Config{
		APIURL:           server.URL,
		TemplateID:       "tpl-v26",
		HTTPClient:       &http.Client{Transport: transport},
		EndpointSPKIPins: []string{pin},
	})
	if err != nil {
		t.Fatalf("New pinned client: %v", err)
	}

	_, err = client.ObserveProviderEndpointSPKI(context.Background())
	if err == nil {
		t.Fatal("hostname-mismatched peer was trusted solely because its SPKI was pinned")
	}
	if !errors.Is(err, ErrEndpointTLSAuthorityUnavailable) {
		t.Fatalf("hostname validation error = %v, want wrapped ErrEndpointTLSAuthorityUnavailable", err)
	}
}

func TestV26PinnedEndpointSupportsExplicitPrivateRootCA(t *testing.T) {
	server := newV26UniqueTLSServer(t, v26HealthHandler(nil))
	pin := v26CertificatePin(server.Certificate())
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	client, err := New(Config{
		APIURL:           server.URL,
		TemplateID:       "tpl-v26",
		HTTPClient:       &http.Client{Transport: transport},
		EndpointSPKIPins: []string{pin},
	})
	if err != nil {
		t.Fatalf("New pinned private-PKI client: %v", err)
	}

	proof, err := client.ObserveProviderEndpointSPKI(context.Background())
	if err != nil {
		t.Fatalf("private RootCA observation failed: %v", err)
	}
	if !proof.Valid() {
		t.Fatal("private RootCA observation did not mint a valid endpoint proof")
	}
	got, ok := proof.SPKISHA256Hex()
	if !ok || got != pin {
		t.Fatalf("private RootCA proof = (%q,%v), want configured pin", got, ok)
	}
}
