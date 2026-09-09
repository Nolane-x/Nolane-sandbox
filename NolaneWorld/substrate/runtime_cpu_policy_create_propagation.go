package substrate

import (
	"encoding/hex"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const RuntimeCPUPolicyCreatePropagationDigestPrefix = "runtime-cpu-policy-create-propagation-v32:"

const runtimeCPUPolicyAuthorityDigestPrefix = "realm-runtime-cpu-policy-v31:"

// RuntimeCPUPolicyCreatePropagation is descriptive evidence that one exact
// Wave31 runtime CPU policy binding was represented in one World provider
// create request. It is not an authority and makes no enforcement claim.
type RuntimeCPUPolicyCreatePropagation struct {
	RealmID         string
	RealmRevision   uint64
	PolicyDigest    string
	LimitMilliCPU   uint64
	AuthorityDigest string
	WorldID         world.ID
	RequestDigest   string
}

func canonicalLowerHex64(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func prefixedCanonicalDigest(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	return canonicalLowerHex64(strings.TrimPrefix(value, prefix))
}

func (p RuntimeCPUPolicyCreatePropagation) Valid() bool {
	if p.RealmID == "" || p.RealmID != strings.TrimSpace(p.RealmID) || p.RealmRevision == 0 || p.LimitMilliCPU == 0 {
		return false
	}
	if p.WorldID == "" || string(p.WorldID) != strings.TrimSpace(string(p.WorldID)) {
		return false
	}
	if !canonicalLowerHex64(p.PolicyDigest) {
		return false
	}
	if !prefixedCanonicalDigest(p.AuthorityDigest, runtimeCPUPolicyAuthorityDigestPrefix) {
		return false
	}
	return prefixedCanonicalDigest(p.RequestDigest, RuntimeCPUPolicyCreatePropagationDigestPrefix)
}
