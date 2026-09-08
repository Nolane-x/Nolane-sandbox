package resourceproof

import (
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

var ErrRuntimeBindingUnavailable = errors.New("live resource proof: runtime binding unavailable")

// BindingFromRuntimeAuthority projects the complete descriptive resourceproof
// binding from an already-valid Wave27 runtime provenance authority. Binding is
// still ordinary serializable data: this helper does not mint TrustedReport,
// capability authority, resource enforcement or LIVE_PASS.
func BindingFromRuntimeAuthority(authority cube.RealmResourceRuntimeAuthority) (Binding, error) {
	realization, ok := authority.RealizationBinding()
	if !ok {
		return Binding{}, ErrRuntimeBindingUnavailable
	}
	runtimeDigest, ok := authority.RuntimeDigest()
	if !ok {
		return Binding{}, ErrRuntimeBindingUnavailable
	}
	return Binding{
		RealmID:             realization.RealmID,
		RealmRevision:       realization.RealmRevision,
		PolicyDigest:        realization.PolicyDigest,
		RealizationRevision: realization.RealizationRevision,
		RuntimeDigest:       runtimeDigest,
	}, nil
}
