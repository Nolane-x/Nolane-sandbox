package delegation

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/authority"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Plane struct {
	state    world.AuthorityState
	grants   Resolver
	vault    Vault
	adapters *Registry
	ledger   authority.InspectableLedger
	now      func() time.Time
}

func NewPlane(state world.AuthorityState, grants Resolver, vault Vault, adapters *Registry, ledger authority.InspectableLedger, now func() time.Time) (*Plane, error) {
	if state == nil || state.ID() == "" || grants == nil || vault == nil || adapters == nil || ledger == nil {
		return nil, ErrInvalidPlane
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Plane{state: state, grants: grants, vault: vault, adapters: adapters, ledger: ledger, now: now}, nil
}

func (p *Plane) Execute(ctx context.Context, in Intent) (Receipt, error) {
	if p == nil || ctx == nil {
		return Receipt{}, ErrInvalidPlane
	}
	if err := validateIntent(in); err != nil {
		return Receipt{}, err
	}
	if in.WorldID != p.state.ID() {
		return Receipt{}, world.ErrInvalidWorld
	}

	var out authority.Receipt
	var grant Grant
	var grantDigest, handleDigest, reqDigest string
	err := p.state.WithEpoch(in.AuthorityEpoch, func() error {
		state, err := p.grants.Lookup(in.DelegationID)
		if err != nil {
			return ErrDelegationNotFound
		}
		grant = state.Grant
		if grant.WorldID != in.WorldID || grant.AuthorityEpoch != in.AuthorityEpoch {
			return ErrScopeDenied
		}
		if state.Revoked {
			return ErrDelegationRevoked
		}
		if !p.now().UTC().Before(grant.ExpiresAt) {
			return ErrDelegationExpired
		}
		if in.Resource != grant.Resource || !grantAllows(grant, in.Operation) {
			return ErrScopeDenied
		}
		adapter, err := p.adapters.Lookup(grant.Adapter)
		if err != nil {
			return ErrAdapterNotFound
		}
		reqDigest, grantDigest, handleDigest, err = requestDigest(in, grant)
		if err != nil {
			return ErrInvalidIntent
		}
		request := buildAdapterRequest(in, reqDigest)
		out, err = p.ledger.ExecuteOnce(in.WorldID, in.ActionID, reqDigest, func() (authority.Receipt, error) {
			evidence, err := p.executeAdapter(ctx, grant.SecretHandle, adapter, request)
			if err != nil {
				return authority.Receipt{}, err
			}
			return authority.Receipt{
				WorldID:        in.WorldID,
				AuthorityEpoch: in.AuthorityEpoch,
				ActionID:       in.ActionID,
				RequestDigest:  reqDigest,
				EffectDigest:   effectDigest(evidence),
				CompletedAt:    p.now().UTC(),
			}, nil
		})
		return err
	})
	if err != nil {
		return Receipt{}, err
	}
	return derivedReceipt(in, out, grantDigest, handleDigest), nil
}

func (p *Plane) Reconcile(ctx context.Context, in Intent) (Receipt, error) {
	if p == nil || ctx == nil {
		return Receipt{}, ErrInvalidPlane
	}
	if err := validateIntent(in); err != nil {
		return Receipt{}, err
	}
	if in.WorldID != p.state.ID() {
		return Receipt{}, world.ErrInvalidWorld
	}
	state, err := p.grants.Lookup(in.DelegationID)
	if err != nil {
		return Receipt{}, ErrDelegationNotFound
	}
	grant := state.Grant
	// Reconciliation is historical observation. Revocation, expiry, and a later
	// world epoch must not prevent checking an already-pending external effect.
	if grant.WorldID != in.WorldID || grant.AuthorityEpoch != in.AuthorityEpoch || in.Resource != grant.Resource || !grantAllows(grant, in.Operation) {
		return Receipt{}, ErrScopeDenied
	}
	adapter, err := p.adapters.Lookup(grant.Adapter)
	if err != nil {
		return Receipt{}, ErrAdapterNotFound
	}
	reqDigest, grantDigest, handleDigest, err := requestDigest(in, grant)
	if err != nil {
		return Receipt{}, ErrInvalidIntent
	}
	ledger, ok := p.ledger.(authority.ResolvingLedger)
	if !ok {
		return Receipt{}, ErrReconcileUnsupported
	}
	status, prior, err := ledger.Status(in.WorldID, in.ActionID, reqDigest)
	if err != nil {
		return Receipt{}, err
	}
	switch status {
	case authority.ActionMissing:
		return Receipt{}, ErrNoPendingAction
	case authority.ActionCompleted:
		return derivedReceipt(in, prior, grantDigest, handleDigest), nil
	case authority.ActionPending:
		// continue below
	default:
		return Receipt{}, ErrReconcileFailure
	}

	request := buildAdapterRequest(in, reqDigest)
	result, err := p.reconcileAdapter(ctx, grant.SecretHandle, adapter, request)
	if err != nil {
		return Receipt{}, err
	}
	switch result.State {
	case ReconcileObserved:
		receipt := authority.Receipt{
			WorldID:        in.WorldID,
			AuthorityEpoch: in.AuthorityEpoch,
			ActionID:       in.ActionID,
			RequestDigest:  reqDigest,
			EffectDigest:   effectDigest(result.Evidence),
			CompletedAt:    p.now().UTC(),
		}
		if err := ledger.Resolve(in.WorldID, in.ActionID, reqDigest, receipt); err != nil {
			return Receipt{}, ErrReconcileFailure
		}
		return derivedReceipt(in, receipt, grantDigest, handleDigest), nil
	case ReconcileAbsent:
		return Receipt{}, ErrEffectAbsent
	case ReconcileUnknown:
		return Receipt{}, authority.ErrActionUncertain
	default:
		return Receipt{}, ErrReconcileFailure
	}
}

func (p *Plane) executeAdapter(ctx context.Context, handle SecretHandle, adapter Adapter, request AdapterRequest) ([]byte, error) {
	var evidence []byte
	entered := false
	err := p.vault.Use(ctx, handle, func(secret Secret) error {
		entered = true
		effect, err := adapter.Execute(ctx, cloneAdapterRequest(request), secret)
		if err != nil {
			return ErrAdapterFailure
		}
		if containsSecret(effect.Evidence, secret.Bytes()) {
			return ErrSecretLeak
		}
		evidence = append([]byte(nil), effect.Evidence...)
		return nil
	})
	if err != nil {
		if !entered {
			// Vault resolution failed before provider execution. Join only stable
			// sentinels so the ledger may safely remove this pending row.
			return nil, errors.Join(ErrSecretUnavailable, authority.ErrPolicyFailure)
		}
		if errors.Is(err, ErrSecretLeak) {
			return nil, ErrSecretLeak
		}
		return nil, ErrAdapterFailure
	}
	return evidence, nil
}

func (p *Plane) reconcileAdapter(ctx context.Context, handle SecretHandle, adapter Adapter, request AdapterRequest) (ReconcileResult, error) {
	var out ReconcileResult
	entered := false
	err := p.vault.Use(ctx, handle, func(secret Secret) error {
		entered = true
		result, err := adapter.Reconcile(ctx, cloneAdapterRequest(request), secret)
		if err != nil {
			return ErrReconcileFailure
		}
		if containsSecret(result.Evidence, secret.Bytes()) {
			return ErrSecretLeak
		}
		out = ReconcileResult{State: result.State, Evidence: append([]byte(nil), result.Evidence...)}
		return nil
	})
	if err != nil {
		if !entered {
			return ReconcileResult{}, ErrSecretUnavailable
		}
		if errors.Is(err, ErrSecretLeak) {
			return ReconcileResult{}, ErrSecretLeak
		}
		return ReconcileResult{}, ErrReconcileFailure
	}
	return out, nil
}

func buildAdapterRequest(in Intent, requestDigest string) AdapterRequest {
	return AdapterRequest{
		WorldID:        in.WorldID,
		ActionID:       in.ActionID,
		Operation:      in.Operation,
		Resource:       in.Resource,
		Payload:        append([]byte(nil), in.Payload...),
		IdempotencyKey: requestDigest,
	}
}

func cloneAdapterRequest(in AdapterRequest) AdapterRequest {
	out := in
	out.Payload = append([]byte(nil), in.Payload...)
	return out
}

func containsSecret(evidence, secret []byte) bool {
	return len(secret) > 0 && bytes.Contains(evidence, secret)
}

func derivedReceipt(in Intent, base authority.Receipt, grantDigest, handleDigest string) Receipt {
	return Receipt{
		WorldID:            base.WorldID,
		AuthorityEpoch:     base.AuthorityEpoch,
		ActionID:           base.ActionID,
		DelegationID:       in.DelegationID,
		RequestDigest:      base.RequestDigest,
		GrantDigest:        grantDigest,
		SecretHandleDigest: handleDigest,
		EffectDigest:       base.EffectDigest,
		CompletedAt:        base.CompletedAt,
	}
}
