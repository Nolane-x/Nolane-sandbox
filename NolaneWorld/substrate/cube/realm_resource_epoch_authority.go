package cube

import (
	"context"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRealmResourceEpochAuthority = errors.New("cube: invalid Realm resource epoch authority")

type realmResourceEpochAuthoritySeal struct{}

var currentRealmResourceEpochAuthoritySeal = &realmResourceEpochAuthoritySeal{}

// RealmResourceEpochAuthority is Wave24's opaque cross-boundary capability. It
// proves only that a fresh Wave23 Realm realization, an exact package-owned
// Cube ResourceBinding and a freshly revalidated Cubelet-local realization
// epoch all identify the same sandbox. It does not imply OOM causality, exact
// task outcome, resource enforcement, provider-global incarnation or LIVE_PASS.
type RealmResourceEpochAuthority struct {
	realmResource RealmResourceAuthority
	epoch         RealizationEpochProof
	seal          *realmResourceEpochAuthoritySeal
}

func (a RealmResourceEpochAuthority) Valid() bool {
	if a.seal != currentRealmResourceEpochAuthoritySeal || !a.realmResource.Valid() || !a.epoch.Valid() {
		return false
	}
	sandboxID, ok := a.realmResource.SandboxID()
	return ok && sandboxID == a.epoch.sandboxID
}

func (a RealmResourceEpochAuthority) SandboxID() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.epoch.sandboxID, true
}

func (a RealmResourceEpochAuthority) RealmResource() (RealmResourceAuthority, bool) {
	if !a.Valid() {
		return RealmResourceAuthority{}, false
	}
	return a.realmResource, true
}

func (a RealmResourceEpochAuthority) Epoch() (RealizationEpochProof, bool) {
	if !a.Valid() {
		return RealizationEpochProof{}, false
	}
	return a.epoch, true
}

// ValidateRealmResourceEpochAuthority revalidates both freshness roots at the
// mint seam: Wave23 re-reads Realm state and Wave24 re-scrapes Cubelet's current
// epoch metric. A stale sealed proof is therefore insufficient on its own.
func ValidateRealmResourceEpochAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	resource ResourceBinding,
	epochObserver *RealizationEpochObserver,
	epochProof RealizationEpochProof,
) (RealmResourceEpochAuthority, error) {
	base, err := ValidateRealmResourceAuthority(ctx, controller, realization, resource)
	if err != nil {
		return RealmResourceEpochAuthority{}, err
	}
	if epochObserver == nil || !epochProof.Valid() {
		return RealmResourceEpochAuthority{}, ErrInvalidRealizationEpochProof
	}
	baseSandboxID, ok := base.SandboxID()
	if !ok || epochProof.sandboxID != baseSandboxID {
		return RealmResourceEpochAuthority{}, ErrInvalidRealmResourceEpochAuthority
	}
	if err := epochObserver.ValidateCurrent(ctx, resource, epochProof); err != nil {
		return RealmResourceEpochAuthority{}, err
	}
	return RealmResourceEpochAuthority{
		realmResource: base,
		epoch:         epochProof,
		seal:          currentRealmResourceEpochAuthoritySeal,
	}, nil
}
