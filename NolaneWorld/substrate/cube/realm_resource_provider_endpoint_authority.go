package cube

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRealmResourceProviderEndpointAuthority = errors.New("cube: invalid Realm resource provider endpoint authority")

type realmResourceProviderEndpointAuthoritySeal struct{}

var currentRealmResourceProviderEndpointAuthoritySeal = &realmResourceProviderEndpointAuthoritySeal{}

// RealmResourceProviderEndpointAuthority is Wave26's opaque cross-boundary
// capability. It retains the exact freshly revalidated Wave25 authority and a
// sealed proof of the exact TLS endpoint SPKI key observed by the same Cube
// Client. Endpoint identity remains additive: it does not replace Realm,
// Cubelet realization-epoch or provider-incarnation freshness.
//
// This authority does not prove task outcome, OOM causality, resource
// enforcement, filesystem/guest state, hardware attestation or LIVE_PASS.
type RealmResourceProviderEndpointAuthority struct {
	provider RealmResourceProviderIncarnationAuthority
	endpoint ProviderEndpointSPKIProof
	seal     *realmResourceProviderEndpointAuthoritySeal
}

func (a RealmResourceProviderEndpointAuthority) Valid() bool {
	if a.seal != currentRealmResourceProviderEndpointAuthoritySeal || !a.provider.Valid() || !a.endpoint.Valid() {
		return false
	}
	sandboxID, ok := a.provider.SandboxID()
	if !ok || sandboxID == "" {
		return false
	}
	return a.provider.provider.client == a.endpoint.client
}

func (a RealmResourceProviderEndpointAuthority) SandboxID() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.provider.SandboxID()
}

func (a RealmResourceProviderEndpointAuthority) ProviderIncarnation() (RealmResourceProviderIncarnationAuthority, bool) {
	if !a.Valid() {
		return RealmResourceProviderIncarnationAuthority{}, false
	}
	return a.provider, true
}

func (a RealmResourceProviderEndpointAuthority) Endpoint() (ProviderEndpointSPKIProof, bool) {
	if !a.Valid() {
		return ProviderEndpointSPKIProof{}, false
	}
	return a.endpoint, true
}

// ValidateRealmResourceProviderEndpointAuthority closes Wave26 only after the
// complete older authority chain and the endpoint key are fresh at this mint
// seam. The supplied Wave25 capability is never trusted by shape alone: its
// sealed Wave24 and provider-incarnation inputs are extracted and re-run
// through the Wave25 validator before endpoint freshness is checked.
func ValidateRealmResourceProviderEndpointAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	providerAuthority RealmResourceProviderIncarnationAuthority,
	resource ResourceBinding,
	epochObserver *RealizationEpochObserver,
	client *Client,
	endpoint ProviderEndpointSPKIProof,
) (RealmResourceProviderEndpointAuthority, error) {
	if !providerAuthority.Valid() {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidRealmResourceProviderEndpointAuthority
	}

	sandboxID := resource.sandboxID
	providerSandboxID, ok := providerAuthority.SandboxID()
	if sandboxID == "" || sandboxID != strings.TrimSpace(sandboxID) || !ok || providerSandboxID != sandboxID {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidRealmResourceProviderEndpointAuthority
	}

	local, ok := providerAuthority.Local()
	if !ok {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidRealmResourceProviderEndpointAuthority
	}
	providerProof, ok := providerAuthority.Provider()
	if !ok {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidRealmResourceProviderEndpointAuthority
	}

	freshProviderAuthority, err := ValidateRealmResourceProviderIncarnationAuthority(
		ctx,
		controller,
		realization,
		local,
		resource,
		epochObserver,
		client,
		providerProof,
	)
	if err != nil {
		return RealmResourceProviderEndpointAuthority{}, err
	}
	if !sameRealmResourceProviderIncarnationAuthority(freshProviderAuthority, providerAuthority) {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidRealmResourceProviderEndpointAuthority
	}

	if client == nil || !endpoint.Valid() || endpoint.client != client {
		return RealmResourceProviderEndpointAuthority{}, ErrInvalidProviderEndpointSPKIProof
	}
	if err := client.ValidateProviderEndpointSPKI(ctx, endpoint); err != nil {
		return RealmResourceProviderEndpointAuthority{}, err
	}

	return RealmResourceProviderEndpointAuthority{
		provider: freshProviderAuthority,
		endpoint: endpoint,
		seal:     currentRealmResourceProviderEndpointAuthoritySeal,
	}, nil
}

func sameRealmResourceProviderIncarnationAuthority(a, b RealmResourceProviderIncarnationAuthority) bool {
	return a.Valid() && b.Valid() &&
		a.seal == b.seal &&
		sameRealmResourceEpochAuthority(a.local, b.local) &&
		sameProviderIncarnationProof(a.provider, b.provider)
}
