package cube

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRealmResourceProviderIncarnationAuthority = errors.New("cube: invalid Realm resource provider incarnation authority")

type realmResourceProviderIncarnationAuthoritySeal struct{}

var currentRealmResourceProviderIncarnationAuthoritySeal = &realmResourceProviderIncarnationAuthoritySeal{}

// RealmResourceProviderIncarnationAuthority is Wave25's opaque cross-boundary
// identity capability. It retains the exact Wave24 local authority and the
// provider-persisted incarnation proof that were both freshly revalidated at
// the mint seam. It does not prove endpoint cryptographic attestation, task
// outcome, OOM causality, resource enforcement, filesystem or guest state, or
// LIVE_PASS.
type RealmResourceProviderIncarnationAuthority struct {
	local    RealmResourceEpochAuthority
	provider ProviderIncarnationProof
	seal     *realmResourceProviderIncarnationAuthoritySeal
}

func (a RealmResourceProviderIncarnationAuthority) Valid() bool {
	if a.seal != currentRealmResourceProviderIncarnationAuthoritySeal || !a.local.Valid() || !a.provider.Valid() {
		return false
	}
	localSandboxID, ok := a.local.SandboxID()
	return ok && localSandboxID == a.provider.sandboxID
}

func (a RealmResourceProviderIncarnationAuthority) SandboxID() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.provider.sandboxID, true
}

func (a RealmResourceProviderIncarnationAuthority) Local() (RealmResourceEpochAuthority, bool) {
	if !a.Valid() {
		return RealmResourceEpochAuthority{}, false
	}
	return a.local, true
}

func (a RealmResourceProviderIncarnationAuthority) Provider() (ProviderIncarnationProof, bool) {
	if !a.Valid() {
		return ProviderIncarnationProof{}, false
	}
	return a.provider, true
}

// ValidateRealmResourceProviderIncarnationAuthority closes the Wave25 identity
// seam only after all three authority roots are fresh at one validation point:
// Realm state, Cubelet-local realization epoch and provider-persisted sandbox
// incarnation. A previously sealed Wave24 authority is not accepted by shape
// alone: its exact epoch proof is revalidated through the original Wave23/24
// mint path before provider freshness is checked.
func ValidateRealmResourceProviderIncarnationAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	local RealmResourceEpochAuthority,
	resource ResourceBinding,
	epochObserver *RealizationEpochObserver,
	client *Client,
	provider ProviderIncarnationProof,
) (RealmResourceProviderIncarnationAuthority, error) {
	if !local.Valid() {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidRealmResourceProviderIncarnationAuthority
	}

	sandboxID := resource.sandboxID
	localSandboxID, ok := local.SandboxID()
	if sandboxID == "" || sandboxID != strings.TrimSpace(sandboxID) || !ok || localSandboxID != sandboxID {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidRealmResourceProviderIncarnationAuthority
	}

	epochProof, ok := local.Epoch()
	if !ok || epochObserver == nil {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidRealmResourceProviderIncarnationAuthority
	}
	freshLocal, err := ValidateRealmResourceEpochAuthority(
		ctx,
		controller,
		realization,
		resource,
		epochObserver,
		epochProof,
	)
	if err != nil {
		return RealmResourceProviderIncarnationAuthority{}, err
	}
	if !sameRealmResourceEpochAuthority(freshLocal, local) {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidRealmResourceProviderIncarnationAuthority
	}

	if client == nil || !provider.Valid() || provider.client != client {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidProviderIncarnationProof
	}
	if provider.sandboxID != sandboxID {
		return RealmResourceProviderIncarnationAuthority{}, ErrInvalidRealmResourceProviderIncarnationAuthority
	}
	if err := client.ValidateProviderIncarnation(ctx, resource, provider); err != nil {
		return RealmResourceProviderIncarnationAuthority{}, err
	}

	return RealmResourceProviderIncarnationAuthority{
		local:    freshLocal,
		provider: provider,
		seal:     currentRealmResourceProviderIncarnationAuthoritySeal,
	}, nil
}

func sameRealmResourceEpochAuthority(a, b RealmResourceEpochAuthority) bool {
	return a.Valid() && b.Valid() &&
		a.seal == b.seal &&
		a.realmResource == b.realmResource &&
		sameRealizationEpochProof(a.epoch, b.epoch)
}
