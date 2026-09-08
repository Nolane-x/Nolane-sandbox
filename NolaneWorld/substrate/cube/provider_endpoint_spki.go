package cube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrInvalidProviderEndpointSPKIProof = errors.New("cube: invalid provider endpoint SPKI proof")
	ErrStaleProviderEndpointSPKIProof   = errors.New("cube: stale provider endpoint SPKI proof")
)

type providerEndpointSPKIProofSeal struct{}

var currentProviderEndpointSPKIProofSeal = &providerEndpointSPKIProofSeal{}

// ProviderEndpointSPKIProof is Wave26's sealed endpoint-key capability. It
// proves only that the exact configured Cube Client completed a normally
// verified TLS handshake with a peer whose leaf SPKI SHA-256 digest is in the
// client's explicit pin-set. It does not prove sandbox state, OOM causality,
// resource enforcement, hardware attestation or LIVE_PASS.
type ProviderEndpointSPKIProof struct {
	spkiSHA256 [32]byte
	client     *Client
	seal       *providerEndpointSPKIProofSeal
}

func (p ProviderEndpointSPKIProof) Valid() bool {
	if p.seal != currentProviderEndpointSPKIProofSeal || p.client == nil || len(p.client.endpointSPKIPins) == 0 {
		return false
	}
	var zero [32]byte
	if p.spkiSHA256 == zero {
		return false
	}
	_, ok := p.client.endpointSPKIPins[p.spkiSHA256]
	return ok
}

func (p ProviderEndpointSPKIProof) SPKISHA256Hex() (string, bool) {
	if !p.Valid() {
		return "", false
	}
	return hex.EncodeToString(p.spkiSHA256[:]), true
}

// ObserveProviderEndpointSPKI performs a fresh, non-mutating CubeAPI health
// request through the exact hardened control-plane client. The health payload
// is liveness only; endpoint authority comes exclusively from the verified TLS
// peer state produced by the handshake-time SPKI pin check.
func (c *Client) ObserveProviderEndpointSPKI(ctx context.Context) (ProviderEndpointSPKIProof, error) {
	if c == nil || c.http == nil || len(c.endpointSPKIPins) == 0 || !strings.HasPrefix(c.apiURL, "https://") {
		return ProviderEndpointSPKIProof{}, ErrEndpointTLSAuthorityUnavailable
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/health", nil)
	if err != nil {
		return ProviderEndpointSPKIProof{}, errors.Join(ErrEndpointTLSAuthorityUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ProviderEndpointSPKIProof{}, errors.Join(ErrEndpointTLSAuthorityUnavailable, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return ProviderEndpointSPKIProof{}, errors.Join(ErrEndpointTLSAuthorityUnavailable, err)
	}
	if int64(len(raw)) > c.maxBytes {
		return ProviderEndpointSPKIProof{}, errors.Join(ErrEndpointTLSAuthorityUnavailable, ErrResponseTooLarge)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProviderEndpointSPKIProof{}, errors.Join(
			ErrEndpointTLSAuthorityUnavailable,
			fmt.Errorf("%w: status=%d", ErrRequestFailed, resp.StatusCode),
		)
	}
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return ProviderEndpointSPKIProof{}, ErrEndpointTLSAuthorityUnavailable
	}

	digest := sha256.Sum256(resp.TLS.PeerCertificates[0].RawSubjectPublicKeyInfo)
	if _, ok := c.endpointSPKIPins[digest]; !ok {
		// This is a defense-in-depth recheck. The handshake-time verifier should
		// already have rejected the request before HTTP bytes were exchanged.
		return ProviderEndpointSPKIProof{}, ErrEndpointSPKIMismatch
	}

	return ProviderEndpointSPKIProof{
		spkiSHA256: digest,
		client:     c,
		seal:       currentProviderEndpointSPKIProofSeal,
	}, nil
}

// ValidateProviderEndpointSPKI re-observes the endpoint before accepting a
// sealed proof. During an explicit overlap rotation {A,B}, transport may trust
// both keys, but a proof for A becomes stale as soon as the current peer is B.
func (c *Client) ValidateProviderEndpointSPKI(
	ctx context.Context,
	proof ProviderEndpointSPKIProof,
) error {
	if c == nil || !proof.Valid() || proof.client != c {
		return ErrInvalidProviderEndpointSPKIProof
	}
	current, err := c.ObserveProviderEndpointSPKI(ctx)
	if err != nil {
		return err
	}
	if !sameProviderEndpointSPKIProof(current, proof) {
		return ErrStaleProviderEndpointSPKIProof
	}
	return nil
}

func sameProviderEndpointSPKIProof(a, b ProviderEndpointSPKIProof) bool {
	return a.Valid() && b.Valid() && a.client == b.client && a.spkiSHA256 == b.spkiSHA256
}
