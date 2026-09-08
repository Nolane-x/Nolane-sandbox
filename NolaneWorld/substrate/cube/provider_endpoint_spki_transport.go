package cube

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalidEndpointSPKIConfig       = errors.New("cube: invalid endpoint SPKI config")
	ErrEndpointSPKIMismatch            = errors.New("cube: endpoint SPKI mismatch")
	ErrEndpointTLSAuthorityUnavailable = errors.New("cube: endpoint TLS authority unavailable")
)

func parseEndpointSPKIPins(rawPins []string) (map[[32]byte]struct{}, error) {
	if len(rawPins) == 0 {
		return nil, nil
	}

	pins := make(map[[32]byte]struct{}, len(rawPins))
	var zero [32]byte
	for _, raw := range rawPins {
		if len(raw) != 64 || raw != strings.ToLower(raw) {
			return nil, ErrInvalidEndpointSPKIConfig
		}
		decoded, err := hex.DecodeString(raw)
		if err != nil || len(decoded) != sha256.Size {
			return nil, ErrInvalidEndpointSPKIConfig
		}
		var digest [32]byte
		copy(digest[:], decoded)
		if digest == zero {
			return nil, ErrInvalidEndpointSPKIConfig
		}
		if _, exists := pins[digest]; exists {
			return nil, ErrInvalidEndpointSPKIConfig
		}
		pins[digest] = struct{}{}
	}
	return pins, nil
}

func cloneEndpointSPKIPins(src map[[32]byte]struct{}) map[[32]byte]struct{} {
	if len(src) == 0 {
		return nil
	}
	out := make(map[[32]byte]struct{}, len(src))
	for digest := range src {
		out[digest] = struct{}{}
	}
	return out
}

// hardenedPinnedHTTPClient installs the SPKI allow-set into the standard TLS
// handshake. Standard certificate-chain and hostname verification remain
// enabled because InsecureSkipVerify is rejected rather than overridden.
func hardenedPinnedHTTPClient(
	src *http.Client,
	timeout time.Duration,
	pins map[[32]byte]struct{},
) (*http.Client, error) {
	if len(pins) == 0 {
		return nil, ErrInvalidEndpointSPKIConfig
	}

	hc := &http.Client{Timeout: timeout}
	if src != nil {
		copyClient := *src
		hc = &copyClient
		if hc.Timeout == 0 {
			hc.Timeout = timeout
		}
	}

	var transport *http.Transport
	switch candidate := hc.Transport.(type) {
	case nil:
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, ErrInvalidEndpointSPKIConfig
		}
		transport = defaultTransport.Clone()
	case *http.Transport:
		transport = candidate.Clone()
	default:
		return nil, ErrInvalidEndpointSPKIConfig
	}

	// These hooks can replace the standard TLS handshake completely, which
	// would bypass tls.Config.VerifyConnection and could expose credentials
	// before endpoint identity is established.
	if transport.DialTLS != nil || transport.DialTLSContext != nil {
		return nil, ErrInvalidEndpointSPKIConfig
	}

	tlsConfig := &tls.Config{}
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
	}
	if tlsConfig.InsecureSkipVerify {
		return nil, ErrInvalidEndpointSPKIConfig
	}

	originalVerifyConnection := tlsConfig.VerifyConnection
	trustedPins := cloneEndpointSPKIPins(pins)
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if originalVerifyConnection != nil {
			if err := originalVerifyConnection(state); err != nil {
				return err
			}
		}
		if len(state.PeerCertificates) == 0 {
			return ErrEndpointTLSAuthorityUnavailable
		}
		digest := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
		if _, ok := trustedPins[digest]; !ok {
			return fmt.Errorf("%w: sha256=%s", ErrEndpointSPKIMismatch, hex.EncodeToString(digest[:]))
		}
		return nil
	}
	transport.TLSClientConfig = tlsConfig
	hc.Transport = transport
	hc.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return hc, nil
}
