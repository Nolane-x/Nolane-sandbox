package authority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Broker struct {
	state    *world.State
	policy   Policy
	executor Executor
	ledger   Ledger
	now      func() time.Time
}

func NewBroker(state *world.State, policy Policy, executor Executor, ledger Ledger) (*Broker, error) {
	if state == nil || state.ID() == "" || policy == nil || executor == nil || ledger == nil {
		return nil, ErrInvalidAction
	}
	return &Broker{state: state, policy: policy, executor: executor, ledger: ledger, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (b *Broker) Execute(ctx context.Context, in Intent) (Receipt, error) {
	if err := validateIntent(in); err != nil {
		return Receipt{}, err
	}
	if in.WorldID != b.state.ID() {
		return Receipt{}, world.ErrInvalidWorld
	}

	digest := requestDigest(in)
	var receipt Receipt
	err := b.state.WithEpoch(in.AuthorityEpoch, func() error {
		var err error
		receipt, err = b.ledger.ExecuteOnce(in.WorldID, in.ActionID, digest, func() (Receipt, error) {
			decision, err := b.policy.Evaluate(ctx, cloneIntent(in))
			if err != nil {
				return Receipt{}, errors.Join(ErrPolicyFailure, err)
			}
			if decision != Allow {
				return Receipt{}, ErrDenied
			}

			effect, err := b.executor.Execute(ctx, cloneIntent(in))
			if err != nil {
				return Receipt{}, errors.Join(ErrExecutionFailure, err)
			}
			return Receipt{
				WorldID: in.WorldID, AuthorityEpoch: in.AuthorityEpoch, ActionID: in.ActionID,
				RequestDigest: digest, EffectDigest: digestBytes(effect), CompletedAt: b.now(),
			}, nil
		})
		return err
	})
	if err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func validateIntent(in Intent) error {
	if in.WorldID == "" || in.AuthorityEpoch == 0 || in.ActionID == "" || in.Kind == "" || in.Target == "" {
		return ErrInvalidAction
	}
	return nil
}

func cloneIntent(in Intent) Intent {
	out := in
	out.Payload = append([]byte(nil), in.Payload...)
	return out
}

func requestDigest(in Intent) string {
	h := sha256.New()
	writeField := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	writeField([]byte(in.WorldID))
	var epoch [8]byte
	binary.BigEndian.PutUint64(epoch[:], uint64(in.AuthorityEpoch))
	writeField(epoch[:])
	writeField([]byte(in.ActionID))
	writeField([]byte(in.Kind))
	writeField([]byte(in.Target))
	writeField(in.Payload)
	return hex.EncodeToString(h.Sum(nil))
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
