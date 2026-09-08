package cube

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

type v26RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f v26RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func v26ValidPin(seed string) string {
	return strings.Repeat(seed, 64)
}

func TestV26EndpointSPKIConfigRejectsMalformedPins(t *testing.T) {
	valid := v26ValidPin("1")
	cases := []struct {
		name string
		pins []string
	}{
		{name: "short", pins: []string{"abcd"}},
		{name: "uppercase", pins: []string{strings.Repeat("A", 64)}},
		{name: "non hex", pins: []string{strings.Repeat("g", 64)}},
		{name: "all zero", pins: []string{strings.Repeat("0", 64)}},
		{name: "duplicate", pins: []string{valid, valid}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Config{
				APIURL:           "https://example.com",
				TemplateID:       "tpl-v26",
				EndpointSPKIPins: tc.pins,
			})
			if !errors.Is(err, ErrInvalidEndpointSPKIConfig) {
				t.Fatalf("error = %v, want ErrInvalidEndpointSPKIConfig", err)
			}
		})
	}
}

func TestV26EndpointSPKIConfigAcceptsCanonicalPin(t *testing.T) {
	client, err := New(Config{
		APIURL:           "https://example.com",
		TemplateID:       "tpl-v26",
		EndpointSPKIPins: []string{v26ValidPin("1")},
	})
	if err != nil {
		t.Fatalf("New canonical pinned client: %v", err)
	}
	if client == nil {
		t.Fatal("New returned nil client")
	}
}

func TestV26EndpointSPKIConfigRequiresHTTPS(t *testing.T) {
	_, err := New(Config{
		APIURL:           "http://127.0.0.1:8080",
		TemplateID:       "tpl-v26",
		EndpointSPKIPins: []string{v26ValidPin("1")},
	})
	if !errors.Is(err, ErrInvalidEndpointSPKIConfig) {
		t.Fatalf("error = %v, want ErrInvalidEndpointSPKIConfig", err)
	}
}

func TestV26UnpinnedLoopbackHTTPRemainsAvailable(t *testing.T) {
	client, err := New(Config{
		APIURL:     "http://127.0.0.1:8080",
		TemplateID: "tpl-v26",
	})
	if err != nil {
		t.Fatalf("unpinned loopback client regressed: %v", err)
	}
	if client == nil {
		t.Fatal("New returned nil client")
	}
}

func TestV26PinnedClientRejectsNonHTTPTransport(t *testing.T) {
	_, err := New(Config{
		APIURL:     "https://example.com",
		TemplateID: "tpl-v26",
		HTTPClient: &http.Client{Transport: v26RoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("must never be used")
		})},
		EndpointSPKIPins: []string{v26ValidPin("1")},
	})
	if !errors.Is(err, ErrInvalidEndpointSPKIConfig) {
		t.Fatalf("error = %v, want ErrInvalidEndpointSPKIConfig", err)
	}
}

func TestV26PinnedClientRejectsInsecureSkipVerify(t *testing.T) {
	_, err := New(Config{
		APIURL:     "https://example.com",
		TemplateID: "tpl-v26",
		HTTPClient: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}},
		EndpointSPKIPins: []string{v26ValidPin("1")},
	})
	if !errors.Is(err, ErrInvalidEndpointSPKIConfig) {
		t.Fatalf("error = %v, want ErrInvalidEndpointSPKIConfig", err)
	}
}

func TestV26PinnedClientRejectsTLSHandshakeBypassHook(t *testing.T) {
	_, err := New(Config{
		APIURL:     "https://example.com",
		TemplateID: "tpl-v26",
		HTTPClient: &http.Client{Transport: &http.Transport{
			DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
				return nil, errors.New("must never be used")
			},
		}},
		EndpointSPKIPins: []string{v26ValidPin("1")},
	})
	if !errors.Is(err, ErrInvalidEndpointSPKIConfig) {
		t.Fatalf("error = %v, want ErrInvalidEndpointSPKIConfig", err)
	}
}
