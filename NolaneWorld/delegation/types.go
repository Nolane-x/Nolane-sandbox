package delegation

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type ID string
type SecretHandle string
type AdapterKind string
type Operation string

type Grant struct {
	ID             ID
	WorldID        world.ID
	AuthorityEpoch world.Epoch
	Adapter        AdapterKind
	Resource       string
	Operations     []Operation
	SecretHandle   SecretHandle
	IssuedAt       time.Time
	ExpiresAt      time.Time
}

type GrantState struct {
	Grant   Grant
	Revoked bool
}

type Intent struct {
	WorldID        world.ID
	AuthorityEpoch world.Epoch
	ActionID       string
	DelegationID   ID
	Operation      Operation
	Resource       string
	Payload        []byte
}

type Receipt struct {
	WorldID            world.ID    `json:"world_id"`
	AuthorityEpoch     world.Epoch `json:"authority_epoch"`
	ActionID           string      `json:"action_id"`
	DelegationID       ID          `json:"delegation_id"`
	RequestDigest      string      `json:"request_digest"`
	GrantDigest        string      `json:"grant_digest"`
	SecretHandleDigest string      `json:"secret_handle_digest"`
	EffectDigest       string      `json:"effect_digest"`
	CompletedAt        time.Time   `json:"completed_at"`
}

type ReconcileState string

const (
	ReconcileObserved ReconcileState = "observed"
	ReconcileAbsent   ReconcileState = "absent"
	ReconcileUnknown  ReconcileState = "unknown"
)

var (
	ErrInvalidGrant          = errors.New("delegation: invalid grant")
	ErrInvalidIntent         = errors.New("delegation: invalid intent")
	ErrInvalidPlane          = errors.New("delegation: invalid plane")
	ErrGrantCollision        = errors.New("delegation: grant id collision")
	ErrDelegationNotFound    = errors.New("delegation: not found")
	ErrAlreadyRevoked        = errors.New("delegation: already revoked")
	ErrDelegationRevoked     = errors.New("delegation: revoked")
	ErrDelegationExpired     = errors.New("delegation: expired")
	ErrScopeDenied           = errors.New("delegation: scope denied")
	ErrAdapterNotFound       = errors.New("delegation: adapter not found")
	ErrAdapterCollision      = errors.New("delegation: adapter collision")
	ErrGenericAdapter        = errors.New("delegation: generic authenticated transport forbidden")
	ErrSecretUnavailable     = errors.New("delegation: secret unavailable")
	ErrSecretHandleCollision = errors.New("delegation: secret handle collision")
	ErrSecretLeak            = errors.New("delegation: secret material leaked into adapter evidence")
	ErrAdapterFailure        = errors.New("delegation: adapter execution failed")
	ErrReconcileFailure      = errors.New("delegation: reconciliation failed")
	ErrReconcileUnsupported  = errors.New("delegation: ledger does not support reconciliation")
	ErrEffectAbsent          = errors.New("delegation: reconciler confirmed effect absent")
	ErrNoPendingAction       = errors.New("delegation: no pending action")
	ErrStoreCorrupt          = errors.New("delegation: journal store corrupt")
	ErrStoreClosed           = errors.New("delegation: journal store closed")
	ErrStoreLocked           = errors.New("delegation: journal store locked")
	ErrStoreLockUnsupported  = errors.New("delegation: journal locking unsupported")
)

func canonicalGrant(in Grant) (Grant, error) {
	out := in
	if !strict(string(out.ID), 256) || out.WorldID == "" || out.AuthorityEpoch == 0 || !strict(string(out.Adapter), 128) || !strict(out.Resource, 1024) || !strict(string(out.SecretHandle), 512) {
		return Grant{}, ErrInvalidGrant
	}
	if out.IssuedAt.IsZero() || out.ExpiresAt.IsZero() || !out.ExpiresAt.After(out.IssuedAt) {
		return Grant{}, ErrInvalidGrant
	}
	if len(out.Operations) == 0 || len(out.Operations) > 128 {
		return Grant{}, ErrInvalidGrant
	}
	seen := make(map[Operation]struct{}, len(out.Operations))
	ops := make([]Operation, 0, len(out.Operations))
	for _, op := range out.Operations {
		if !strict(string(op), 256) {
			return Grant{}, ErrInvalidGrant
		}
		if _, ok := seen[op]; ok {
			continue
		}
		seen[op] = struct{}{}
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i] < ops[j] })
	out.Operations = ops
	out.IssuedAt = out.IssuedAt.UTC()
	out.ExpiresAt = out.ExpiresAt.UTC()
	return out, nil
}

func validateIntent(in Intent) error {
	if in.WorldID == "" || in.AuthorityEpoch == 0 || !strict(in.ActionID, 256) || !strict(string(in.DelegationID), 256) || !strict(string(in.Operation), 256) || !strict(in.Resource, 1024) || len(in.Payload) > 4*1024*1024 {
		return ErrInvalidIntent
	}
	return nil
}

func grantAllows(g Grant, op Operation) bool {
	i := sort.Search(len(g.Operations), func(i int) bool { return g.Operations[i] >= op })
	return i < len(g.Operations) && g.Operations[i] == op
}

func strict(s string, max int) bool {
	return s != "" && len(s) <= max && strings.TrimSpace(s) == s
}
