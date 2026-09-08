package cube

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var (
	ErrProviderIncarnationUnavailable  = errors.New("cube provider incarnation observation unavailable")
	ErrInvalidProviderIncarnationProof = errors.New("cube: invalid provider incarnation proof")
	ErrStaleProviderIncarnationProof   = errors.New("cube: stale provider incarnation proof")
)

type providerIncarnationProofSeal struct{}

var currentProviderIncarnationProofSeal = &providerIncarnationProofSeal{}

// ProviderIncarnationProof is Wave25's sealed provider-persistence capability.
// It proves only that the exact configured Cube Client freshly observed a
// canonical provider incarnation for the exact ResourceBinding sandbox. It does
// not prove endpoint cryptographic attestation, OOM causality, task outcome,
// resource enforcement, Realm continuity or LIVE_PASS.
type ProviderIncarnationProof struct {
	sandboxID     string
	incarnationID string
	client        *Client
	seal          *providerIncarnationProofSeal
}

func (p ProviderIncarnationProof) Valid() bool {
	return p.seal == currentProviderIncarnationProofSeal &&
		p.client != nil &&
		p.sandboxID != "" &&
		p.sandboxID == strings.TrimSpace(p.sandboxID) &&
		isCanonicalProviderIncarnationID(p.incarnationID)
}

func (p ProviderIncarnationProof) SandboxID() (string, bool) {
	if !p.Valid() {
		return "", false
	}
	return p.sandboxID, true
}

func (p ProviderIncarnationProof) IncarnationID() (string, bool) {
	if !p.Valid() {
		return "", false
	}
	return p.incarnationID, true
}

// ObserveProviderIncarnation reads the provider-persisted incarnation through
// the existing hardened Cube API client. Reusing doJSON preserves the exact API
// key, redirect, response-size and status handling policy already established by
// Client instead of creating another transport trust root.
func (c *Client) ObserveProviderIncarnation(
	ctx context.Context,
	resource ResourceBinding,
) (ProviderIncarnationProof, error) {
	if c == nil || c.http == nil {
		return ProviderIncarnationProof{}, ErrProviderIncarnationUnavailable
	}
	sandboxID := resource.sandboxID
	if sandboxID == "" || sandboxID != strings.TrimSpace(sandboxID) {
		return ProviderIncarnationProof{}, ErrInvalidResourceBinding
	}

	var out struct {
		SandboxID     string `json:"sandboxID"`
		IncarnationID string `json:"incarnationID"`
	}
	if err := c.doJSON(
		ctx,
		http.MethodGet,
		"/sandboxes/"+url.PathEscape(sandboxID),
		nil,
		&out,
		false,
	); err != nil {
		return ProviderIncarnationProof{}, errors.Join(ErrProviderIncarnationUnavailable, err)
	}
	if out.SandboxID != sandboxID || !isCanonicalProviderIncarnationID(out.IncarnationID) {
		return ProviderIncarnationProof{}, ErrProviderIncarnationUnavailable
	}

	return ProviderIncarnationProof{
		sandboxID:     sandboxID,
		incarnationID: out.IncarnationID,
		client:        c,
		seal:          currentProviderIncarnationProofSeal,
	}, nil
}

// ValidateProviderIncarnation re-observes provider state before accepting a
// sealed proof. A structurally valid proof from another Client context or an
// older remote incarnation is insufficient.
func (c *Client) ValidateProviderIncarnation(
	ctx context.Context,
	resource ResourceBinding,
	proof ProviderIncarnationProof,
) error {
	if c == nil || !proof.Valid() || proof.client != c || proof.sandboxID != resource.sandboxID {
		return ErrInvalidProviderIncarnationProof
	}
	current, err := c.ObserveProviderIncarnation(ctx, resource)
	if err != nil {
		return err
	}
	if !sameProviderIncarnationProof(current, proof) {
		return ErrStaleProviderIncarnationProof
	}
	return nil
}

func sameProviderIncarnationProof(a, b ProviderIncarnationProof) bool {
	return a.Valid() &&
		b.Valid() &&
		a.client == b.client &&
		a.sandboxID == b.sandboxID &&
		a.incarnationID == b.incarnationID
}

func isCanonicalProviderIncarnationID(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	if raw[14] != '4' {
		return false
	}
	switch raw[19] {
	case '8', '9', 'a', 'b':
	default:
		return false
	}

	compact := strings.ReplaceAll(raw, "-", "")
	if len(compact) != 32 {
		return false
	}
	for _, ch := range compact {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}
