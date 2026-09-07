package realm

import (
	"context"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrRealizationAuthorityUnavailable = errors.New("realm: realization authority unavailable")
	ErrInvalidRealizationAuthority     = errors.New("realm: invalid realization authority")
	ErrStaleRealizationAuthority       = errors.New("realm: stale realization authority")
)

// RealizationBinding is a descriptive projection of the identity sealed into
// RealizationAuthority. It carries no authority by itself and callers may not
// convert it back into RealizationAuthority.
type RealizationBinding struct {
	RealmID             ID
	RealmRevision       uint64
	PolicyDigest        string
	WorldID             world.ID
	RealizationRevision uint64
	SubstrateHandle     substrate.Handle
}

type realizationAuthoritySeal struct{}

var currentRealizationAuthoritySeal = &realizationAuthoritySeal{}

// RealizationAuthority is an opaque in-process capability minted only after a
// Controller reads its own current Realm and World records. Its zero value is
// invalid; the descriptive binding is intentionally insufficient to recreate
// the package-owned seal.
type RealizationAuthority struct {
	binding RealizationBinding
	seal    *realizationAuthoritySeal
}

func (a RealizationAuthority) Binding() (RealizationBinding, bool) {
	if a.seal != currentRealizationAuthoritySeal || !validRealizationBinding(a.binding) {
		return RealizationBinding{}, false
	}
	return a.binding, true
}

func validRealizationBinding(b RealizationBinding) bool {
	return validRealmID(b.RealmID) && b.RealmRevision > 0 && b.PolicyDigest != "" && b.WorldID != "" && b.RealizationRevision > 0 && b.SubstrateHandle != ""
}

func authorityBearingWorldPhase(phase WorldPhase) bool {
	switch phase {
	case WorldObservedReady, WorldLeased, WorldPaused:
		return true
	default:
		return false
	}
}

// CurrentRealizationAuthority mints authority from the Controller's current
// host-owned Store state. Caller-supplied revisions, policy digests, handles,
// or generations are never accepted as inputs.
func (c *Controller) CurrentRealizationAuthority(ctx context.Context, realmID ID, worldID world.ID) (RealizationAuthority, error) {
	if c == nil || c.store == nil {
		return RealizationAuthority{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RealizationAuthority{}, err
	}
	realmRec, ok := c.store.Realm(realmID)
	if !ok || realmRec.Closed || realmRec.Revision == 0 || realmRec.Spec.ID != realmID {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	worldRec, ok := c.store.World(realmID, worldID)
	if !ok || worldRec.RealmID != realmID || worldRec.WorldID != worldID || !authorityBearingWorldPhase(worldRec.Phase) || worldRec.RealizationRevision == 0 || worldRec.Handle == "" {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	policyDigest, err := PolicyDigest(realmRec.Spec, realmRec.Revision)
	if err != nil {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	binding := RealizationBinding{
		RealmID:             realmID,
		RealmRevision:       realmRec.Revision,
		PolicyDigest:        policyDigest,
		WorldID:             worldID,
		RealizationRevision: worldRec.RealizationRevision,
		SubstrateHandle:     worldRec.Handle,
	}
	if !validRealizationBinding(binding) {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	return RealizationAuthority{binding: binding, seal: currentRealizationAuthoritySeal}, nil
}

// ValidateRealizationAuthority re-reads current Store state before returning
// the sealed descriptive binding. Any drift of Realm revision/policy, World
// realization, live phase, or substrate handle invalidates the old authority.
func (c *Controller) ValidateRealizationAuthority(ctx context.Context, authority RealizationAuthority) (RealizationBinding, error) {
	if c == nil || c.store == nil {
		return RealizationBinding{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RealizationBinding{}, err
	}
	binding, ok := authority.Binding()
	if !ok {
		return RealizationBinding{}, ErrInvalidRealizationAuthority
	}
	realmRec, ok := c.store.Realm(binding.RealmID)
	if !ok || realmRec.Closed || realmRec.Revision != binding.RealmRevision || realmRec.Spec.ID != binding.RealmID {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	policyDigest, err := PolicyDigest(realmRec.Spec, realmRec.Revision)
	if err != nil || policyDigest != binding.PolicyDigest {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	worldRec, ok := c.store.World(binding.RealmID, binding.WorldID)
	if !ok || worldRec.RealmID != binding.RealmID || worldRec.WorldID != binding.WorldID || !authorityBearingWorldPhase(worldRec.Phase) || worldRec.RealizationRevision != binding.RealizationRevision || worldRec.Handle != binding.SubstrateHandle {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	return binding, nil
}
