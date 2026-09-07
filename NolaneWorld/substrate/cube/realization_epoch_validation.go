package cube

import (
	"context"
	"errors"
)

var (
	ErrInvalidRealizationEpochProof = errors.New("cube: invalid realization epoch proof")
	ErrStaleRealizationEpochProof   = errors.New("cube: stale realization epoch proof")
)

func sameRealizationEpochProof(a, b RealizationEpochProof) bool {
	return a.Valid() && b.Valid() &&
		a.sandboxID == b.sandboxID &&
		a.generation == b.generation &&
		a.token == b.token
}

// ValidateCurrent performs a fresh scrape of the exact ResourceBinding before
// accepting a previously minted epoch proof. This is the freshness fence that
// prevents a sealed old token from surviving Clear -> Start even when the
// literal sandbox ID and numeric generation are reused.
func (o *RealizationEpochObserver) ValidateCurrent(ctx context.Context, binding ResourceBinding, proof RealizationEpochProof) error {
	if !proof.Valid() {
		return ErrInvalidRealizationEpochProof
	}
	if binding.sandboxID == "" || binding.sandboxID != proof.sandboxID {
		return ErrStaleRealizationEpochProof
	}
	current, ok, err := o.Observe(ctx, binding)
	if err != nil {
		return err
	}
	if !ok || !sameRealizationEpochProof(current, proof) {
		return ErrStaleRealizationEpochProof
	}
	return nil
}
