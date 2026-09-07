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

// realizationAuthorityStore is deliberately narrower than the public Store
// interface. Only exact package-owned store implementations are accepted as
// mint/validation trust roots. The marker remains an internal contract, while
// trustedRealizationAuthorityStore additionally rejects external wrappers that
// would otherwise inherit this unexported method through embedding.
type realizationAuthorityStore interface {
	Store
	packageOwnedRealizationAuthorityStore()
}

func (*MemoryStore) packageOwnedRealizationAuthorityStore()  {}
func (*DurableStore) packageOwnedRealizationAuthorityStore() {}

func (c *Controller) trustedRealizationAuthorityStore() (realizationAuthorityStore, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}
	switch store := c.store.(type) {
	case *MemoryStore:
		if store == nil {
			return nil, false
		}
		return store, true
	case *DurableStore:
		if store == nil {
			return nil, false
		}
		return store, true
	default:
		return nil, false
	}
}

// RealizationAuthority is an opaque in-process capability minted only after a
// Controller reads its own current Realm and World records. Its zero value is
// invalid; the descriptive binding is intentionally insufficient to recreate
// the package-owned seal. The package-owned Store instance is retained as part
// of the capability provenance so an authority cannot cross otherwise
// identical authority contexts.
type RealizationAuthority struct {
	binding RealizationBinding
	store   realizationAuthorityStore
	seal    *realizationAuthoritySeal
}

func (a RealizationAuthority) Binding() (RealizationBinding, bool) {
	if a.seal != currentRealizationAuthoritySeal || a.store == nil || !validRealizationBinding(a.binding) {
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
// package-owned Store state. Caller-supplied stores, revisions, policy digests,
// handles, or generations are never accepted as authority inputs.
func (c *Controller) CurrentRealizationAuthority(ctx context.Context, realmID ID, worldID world.ID) (RealizationAuthority, error) {
	if c == nil || c.store == nil {
		return RealizationAuthority{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RealizationAuthority{}, err
	}
	store, trusted := c.trustedRealizationAuthorityStore()
	if !trusted {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	realmRec, ok := store.Realm(realmID)
	if !ok || realmRec.Closed || realmRec.Revision == 0 || realmRec.Spec.ID != realmID {
		return RealizationAuthority{}, ErrRealizationAuthorityUnavailable
	}
	worldRec, ok := store.World(realmID, worldID)
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
	return RealizationAuthority{
		binding: binding,
		store:   store,
		seal:    currentRealizationAuthoritySeal,
	}, nil
}

// ValidateRealizationAuthority re-reads current package-owned Store state
// before returning the sealed descriptive binding. Any drift of Realm
// revision/policy, World realization, live phase, substrate handle, or exact
// Store authority context invalidates the old authority.
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
	store, trusted := c.trustedRealizationAuthorityStore()
	if !trusted {
		return RealizationBinding{}, ErrRealizationAuthorityUnavailable
	}
	if authority.store != store {
		return RealizationBinding{}, ErrInvalidRealizationAuthority
	}
	realmRec, ok := store.Realm(binding.RealmID)
	if !ok || realmRec.Closed || realmRec.Revision != binding.RealmRevision || realmRec.Spec.ID != binding.RealmID {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	policyDigest, err := PolicyDigest(realmRec.Spec, realmRec.Revision)
	if err != nil || policyDigest != binding.PolicyDigest {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	worldRec, ok := store.World(binding.RealmID, binding.WorldID)
	if !ok || worldRec.RealmID != binding.RealmID || worldRec.WorldID != binding.WorldID || !authorityBearingWorldPhase(worldRec.Phase) || worldRec.RealizationRevision != binding.RealizationRevision || worldRec.Handle != binding.SubstrateHandle {
		return RealizationBinding{}, ErrStaleRealizationAuthority
	}
	return binding, nil
}
