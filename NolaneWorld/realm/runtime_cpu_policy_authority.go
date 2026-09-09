package realm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrRuntimeCPUPolicyUnavailable          = errors.New("realm: runtime CPU policy authority unavailable")
	ErrInvalidRuntimeCPUPolicyAuthority     = errors.New("realm: invalid runtime CPU policy authority")
	ErrStaleRuntimeCPUPolicyAuthority       = errors.New("realm: stale runtime CPU policy authority")
)

const (
	runtimeCPUPolicyDigestPrefix = "realm-runtime-cpu-policy-v31:"
	runtimeCPUPolicyDigestDomain = "nolane.realm-runtime-cpu-policy.v31\x00"
)

// RuntimeCPUPolicyBinding is a descriptive projection of a sealed current
// Realm runtime CPU bandwidth intent. It carries no authority by itself and
// cannot be converted back into RuntimeCPUPolicyAuthority.
type RuntimeCPUPolicyBinding struct {
	RealmID       ID
	RealmRevision uint64
	PolicyDigest  string
	LimitMilliCPU uint64
	Digest        string
}

type runtimeCPUPolicyAuthoritySeal struct{}

var currentRuntimeCPUPolicyAuthoritySeal = &runtimeCPUPolicyAuthoritySeal{}

// runtimeCPUPolicyStore is deliberately narrower than Store. The marker is
// package-owned, while trustedRuntimeCPUPolicyStore also uses an exact dynamic
// type switch so caller wrappers or embeddings cannot become minting roots.
type runtimeCPUPolicyStore interface {
	Store
	packageOwnedRuntimeCPUPolicyStore()
}

func (*MemoryStore) packageOwnedRuntimeCPUPolicyStore()  {}
func (*DurableStore) packageOwnedRuntimeCPUPolicyStore() {}

func (c *Controller) trustedRuntimeCPUPolicyStore() (runtimeCPUPolicyStore, bool) {
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

// RuntimeCPUPolicyAuthority is an opaque in-process capability. It certifies
// only that one exact current package-owned Realm policy explicitly carries a
// positive runtime CPU bandwidth intent in milliCPU. It does not certify
// propagation to a substrate or kernel enforcement.
type RuntimeCPUPolicyAuthority struct {
	binding RuntimeCPUPolicyBinding
	store   runtimeCPUPolicyStore
	seal    *runtimeCPUPolicyAuthoritySeal
}

func canonicalLowerHex64(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func deriveRuntimeCPUPolicyDigest(binding RuntimeCPUPolicyBinding) (string, error) {
	if !validRealmID(binding.RealmID) || binding.RealmRevision == 0 || !canonicalLowerHex64(binding.PolicyDigest) || binding.LimitMilliCPU == 0 {
		return "", ErrInvalidRuntimeCPUPolicyAuthority
	}
	type canonical struct {
		RealmID       ID     `json:"realm_id"`
		RealmRevision uint64 `json:"realm_revision"`
		PolicyDigest  string `json:"policy_digest"`
		LimitMilliCPU uint64 `json:"limit_millicpu"`
	}
	raw, err := json.Marshal(canonical{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision,
		PolicyDigest:  binding.PolicyDigest,
		LimitMilliCPU: binding.LimitMilliCPU,
	})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(append([]byte(runtimeCPUPolicyDigestDomain), raw...))
	return runtimeCPUPolicyDigestPrefix + hex.EncodeToString(h[:]), nil
}

func validRuntimeCPUPolicyBinding(binding RuntimeCPUPolicyBinding) bool {
	if !strings.HasPrefix(binding.Digest, runtimeCPUPolicyDigestPrefix) {
		return false
	}
	digestHex := strings.TrimPrefix(binding.Digest, runtimeCPUPolicyDigestPrefix)
	if !canonicalLowerHex64(digestHex) {
		return false
	}
	expected, err := deriveRuntimeCPUPolicyDigest(RuntimeCPUPolicyBinding{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision,
		PolicyDigest:  binding.PolicyDigest,
		LimitMilliCPU: binding.LimitMilliCPU,
	})
	return err == nil && expected == binding.Digest
}

// Binding validates only the in-process seal and internally bound digest. It
// is descriptive and does not re-read current Realm state; callers that need
// freshness must use Controller.ValidateRuntimeCPUPolicyAuthority.
func (a RuntimeCPUPolicyAuthority) Binding() (RuntimeCPUPolicyBinding, bool) {
	if a.seal != currentRuntimeCPUPolicyAuthoritySeal || a.store == nil || !validRuntimeCPUPolicyBinding(a.binding) {
		return RuntimeCPUPolicyBinding{}, false
	}
	return a.binding, true
}

// CurrentRuntimeCPUPolicyAuthority mints authority only from the Controller's
// exact current package-owned Realm record. The caller supplies no revision,
// policy digest, milliCPU value, or Store authority material.
func (c *Controller) CurrentRuntimeCPUPolicyAuthority(ctx context.Context, realmID ID) (RuntimeCPUPolicyAuthority, error) {
	if c == nil || c.store == nil {
		return RuntimeCPUPolicyAuthority{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RuntimeCPUPolicyAuthority{}, err
	}
	store, trusted := c.trustedRuntimeCPUPolicyStore()
	if !trusted {
		return RuntimeCPUPolicyAuthority{}, ErrRuntimeCPUPolicyUnavailable
	}
	rec, ok := store.Realm(realmID)
	if !ok || rec.Closed || rec.Revision == 0 || rec.Spec.ID != realmID || rec.Spec.Validate() != nil || rec.Spec.RuntimeCPULimitMilliCPU == 0 {
		return RuntimeCPUPolicyAuthority{}, ErrRuntimeCPUPolicyUnavailable
	}
	policyDigest, err := PolicyDigest(rec.Spec, rec.Revision)
	if err != nil || !canonicalLowerHex64(policyDigest) {
		return RuntimeCPUPolicyAuthority{}, ErrRuntimeCPUPolicyUnavailable
	}
	binding := RuntimeCPUPolicyBinding{
		RealmID:       realmID,
		RealmRevision: rec.Revision,
		PolicyDigest:  policyDigest,
		LimitMilliCPU: rec.Spec.RuntimeCPULimitMilliCPU,
	}
	binding.Digest, err = deriveRuntimeCPUPolicyDigest(binding)
	if err != nil || !validRuntimeCPUPolicyBinding(binding) {
		return RuntimeCPUPolicyAuthority{}, ErrRuntimeCPUPolicyUnavailable
	}
	return RuntimeCPUPolicyAuthority{
		binding: binding,
		store:   store,
		seal:    currentRuntimeCPUPolicyAuthoritySeal,
	}, nil
}

// ValidateRuntimeCPUPolicyAuthority re-reads the current package-owned Realm
// state. Any same-Store Realm revision or policy drift makes a previously
// valid authority stale; a malformed capability or Store-instance crossing is
// invalid.
func (c *Controller) ValidateRuntimeCPUPolicyAuthority(ctx context.Context, authority RuntimeCPUPolicyAuthority) (RuntimeCPUPolicyBinding, error) {
	if c == nil || c.store == nil {
		return RuntimeCPUPolicyBinding{}, ErrInvalidController
	}
	if err := ctx.Err(); err != nil {
		return RuntimeCPUPolicyBinding{}, err
	}
	binding, ok := authority.Binding()
	if !ok {
		return RuntimeCPUPolicyBinding{}, ErrInvalidRuntimeCPUPolicyAuthority
	}
	store, trusted := c.trustedRuntimeCPUPolicyStore()
	if !trusted {
		return RuntimeCPUPolicyBinding{}, ErrRuntimeCPUPolicyUnavailable
	}
	if authority.store != store {
		return RuntimeCPUPolicyBinding{}, ErrInvalidRuntimeCPUPolicyAuthority
	}
	rec, ok := store.Realm(binding.RealmID)
	if !ok || rec.Closed || rec.Revision != binding.RealmRevision || rec.Spec.ID != binding.RealmID || rec.Spec.Validate() != nil || rec.Spec.RuntimeCPULimitMilliCPU == 0 || rec.Spec.RuntimeCPULimitMilliCPU != binding.LimitMilliCPU {
		return RuntimeCPUPolicyBinding{}, ErrStaleRuntimeCPUPolicyAuthority
	}
	policyDigest, err := PolicyDigest(rec.Spec, rec.Revision)
	if err != nil || policyDigest != binding.PolicyDigest || !canonicalLowerHex64(policyDigest) {
		return RuntimeCPUPolicyBinding{}, ErrStaleRuntimeCPUPolicyAuthority
	}
	fresh := RuntimeCPUPolicyBinding{
		RealmID:       rec.Spec.ID,
		RealmRevision: rec.Revision,
		PolicyDigest:  policyDigest,
		LimitMilliCPU: rec.Spec.RuntimeCPULimitMilliCPU,
	}
	fresh.Digest, err = deriveRuntimeCPUPolicyDigest(fresh)
	if err != nil || fresh.Digest != binding.Digest || !validRuntimeCPUPolicyBinding(fresh) {
		return RuntimeCPUPolicyBinding{}, ErrStaleRuntimeCPUPolicyAuthority
	}
	return binding, nil
}
