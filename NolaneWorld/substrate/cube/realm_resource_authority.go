package cube

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var (
	ErrInvalidRealmResourceAuthority = errors.New("cube: invalid Realm resource authority")
	ErrRealmResourceAuthorityMismatch = errors.New("cube: Realm realization and resource sandbox mismatch")
)

type realmResourceAuthoritySeal struct{}

var currentRealmResourceAuthoritySeal = &realmResourceAuthoritySeal{}

// RealmResourceAuthority is an opaque proof that one freshness-checked Realm
// World realization and one package-owned Cube ResourceBinding identify the
// exact same sandbox. It does not itself assert any resource or OOM outcome.
type RealmResourceAuthority struct {
	realmBinding realm.RealizationBinding
	sandboxID    string
	seal         *realmResourceAuthoritySeal
}

func (a RealmResourceAuthority) Valid() bool {
	return a.seal == currentRealmResourceAuthoritySeal && a.sandboxID != "" && string(a.realmBinding.SubstrateHandle) == a.sandboxID
}

func (a RealmResourceAuthority) RealmBinding() (realm.RealizationBinding, bool) {
	if !a.Valid() {
		return realm.RealizationBinding{}, false
	}
	return a.realmBinding, true
}

func (a RealmResourceAuthority) SandboxID() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.sandboxID, true
}

// ValidateRealmResourceAuthority revalidates Realm freshness immediately
// before comparing the exact sealed World substrate handle with the concrete
// Cube ResourceBinding sandbox identity.
func ValidateRealmResourceAuthority(ctx context.Context, controller *realm.Controller, realization realm.RealizationAuthority, resource ResourceBinding) (RealmResourceAuthority, error) {
	if controller == nil {
		return RealmResourceAuthority{}, ErrInvalidRealmResourceAuthority
	}
	sandboxID := resource.sandboxID
	if sandboxID == "" || sandboxID != strings.TrimSpace(sandboxID) {
		return RealmResourceAuthority{}, ErrInvalidResourceBinding
	}
	binding, err := controller.ValidateRealizationAuthority(ctx, realization)
	if err != nil {
		return RealmResourceAuthority{}, err
	}
	if string(binding.SubstrateHandle) != sandboxID {
		return RealmResourceAuthority{}, ErrRealmResourceAuthorityMismatch
	}
	return RealmResourceAuthority{
		realmBinding: binding,
		sandboxID:    sandboxID,
		seal:         currentRealmResourceAuthoritySeal,
	}, nil
}
