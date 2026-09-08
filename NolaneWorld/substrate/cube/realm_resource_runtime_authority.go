package cube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidRealmResourceRuntimeAuthority = errors.New("cube: invalid Realm resource runtime authority")

type realmResourceRuntimeAuthoritySeal struct{}

var currentRealmResourceRuntimeAuthoritySeal = &realmResourceRuntimeAuthoritySeal{}

// RealmResourceRuntimeAuthority is Wave27's opaque runtime-provenance
// capability. It retains one freshly reconstructed Wave26 authority and one
// freshly revalidated same-scrape runtime realization proof. The runtime digest
// identifies this exact authority-owned runtime provenance; it is not a binary
// measurement, resource-enforcement proof, task-outcome proof or LIVE_PASS.
type RealmResourceRuntimeAuthority struct {
	endpoint RealmResourceProviderEndpointAuthority
	runtime  RuntimeRealizationProof
	digest   string
	seal     *realmResourceRuntimeAuthoritySeal
}

func (a RealmResourceRuntimeAuthority) Valid() bool {
	if a.seal != currentRealmResourceRuntimeAuthoritySeal || !a.endpoint.Valid() || !a.runtime.Valid() || !validRuntimeRealizationDigest(a.digest) {
		return false
	}
	epoch, ok := wave26RealizationEpoch(a.endpoint)
	if !ok || !sameRealizationEpochProof(epoch, a.runtime.epoch) {
		return false
	}
	expected, err := deriveRuntimeRealizationDigest(a.endpoint, a.runtime)
	return err == nil && expected == a.digest
}

func (a RealmResourceRuntimeAuthority) SandboxID() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.endpoint.SandboxID()
}

func (a RealmResourceRuntimeAuthority) RuntimeDigest() (string, bool) {
	if !a.Valid() {
		return "", false
	}
	return a.digest, true
}

// RealizationBinding returns the descriptive Realm binding already sealed into
// the nested Wave23 authority. It cannot reconstruct Realm or Wave27 authority.
func (a RealmResourceRuntimeAuthority) RealizationBinding() (realm.RealizationBinding, bool) {
	if !a.Valid() {
		return realm.RealizationBinding{}, false
	}
	return wave26RealizationBinding(a.endpoint)
}

// ValidateRealmResourceRuntimeAuthority closes Wave27 only after reconstructing
// all Wave26 freshness and revalidating the exact same-scrape runtime proof.
// Neither the supplied Wave26 capability nor the supplied runtime proof is
// accepted by shape alone.
func ValidateRealmResourceRuntimeAuthority(
	ctx context.Context,
	controller *realm.Controller,
	realization realm.RealizationAuthority,
	endpointAuthority RealmResourceProviderEndpointAuthority,
	resource ResourceBinding,
	epochObserver *RealizationEpochObserver,
	client *Client,
	runtimeObserver *RuntimeRealizationObserver,
	runtime RuntimeRealizationProof,
) (RealmResourceRuntimeAuthority, error) {
	if !endpointAuthority.Valid() {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}

	providerAuthority, ok := endpointAuthority.ProviderIncarnation()
	if !ok {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}
	endpointProof, ok := endpointAuthority.Endpoint()
	if !ok {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}

	freshEndpoint, err := ValidateRealmResourceProviderEndpointAuthority(
		ctx,
		controller,
		realization,
		providerAuthority,
		resource,
		epochObserver,
		client,
		endpointProof,
	)
	if err != nil {
		return RealmResourceRuntimeAuthority{}, err
	}
	if !sameRealmResourceProviderEndpointAuthority(freshEndpoint, endpointAuthority) {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}

	if runtimeObserver == nil || !runtime.Valid() || runtime.observer != runtimeObserver {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRuntimeRealizationProof
	}
	if err := runtimeObserver.ValidateCurrent(ctx, resource, runtime); err != nil {
		return RealmResourceRuntimeAuthority{}, err
	}

	embeddedEpoch, ok := wave26RealizationEpoch(freshEndpoint)
	if !ok || !sameRealizationEpochProof(embeddedEpoch, runtime.epoch) {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}

	digest, err := deriveRuntimeRealizationDigest(freshEndpoint, runtime)
	if err != nil {
		return RealmResourceRuntimeAuthority{}, ErrInvalidRealmResourceRuntimeAuthority
	}
	return RealmResourceRuntimeAuthority{
		endpoint: freshEndpoint,
		runtime:  runtime,
		digest:   digest,
		seal:     currentRealmResourceRuntimeAuthoritySeal,
	}, nil
}

func sameRealmResourceProviderEndpointAuthority(a, b RealmResourceProviderEndpointAuthority) bool {
	return a.Valid() && b.Valid() &&
		a.seal == b.seal &&
		sameRealmResourceProviderIncarnationAuthority(a.provider, b.provider) &&
		sameProviderEndpointSPKIProof(a.endpoint, b.endpoint)
}

func wave26RealizationEpoch(authority RealmResourceProviderEndpointAuthority) (RealizationEpochProof, bool) {
	if !authority.Valid() {
		return RealizationEpochProof{}, false
	}
	provider, ok := authority.ProviderIncarnation()
	if !ok {
		return RealizationEpochProof{}, false
	}
	local, ok := provider.Local()
	if !ok {
		return RealizationEpochProof{}, false
	}
	return local.Epoch()
}

func wave26RealizationBinding(authority RealmResourceProviderEndpointAuthority) (realm.RealizationBinding, bool) {
	if !authority.Valid() {
		return realm.RealizationBinding{}, false
	}
	provider, ok := authority.ProviderIncarnation()
	if !ok {
		return realm.RealizationBinding{}, false
	}
	local, ok := provider.Local()
	if !ok {
		return realm.RealizationBinding{}, false
	}
	base, ok := local.RealmResource()
	if !ok {
		return realm.RealizationBinding{}, false
	}
	return base.RealmBinding()
}

func deriveRuntimeRealizationDigest(endpoint RealmResourceProviderEndpointAuthority, runtime RuntimeRealizationProof) (string, error) {
	if !endpoint.Valid() || !runtime.Valid() {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	binding, ok := wave26RealizationBinding(endpoint)
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	embeddedEpoch, ok := wave26RealizationEpoch(endpoint)
	if !ok || !sameRealizationEpochProof(embeddedEpoch, runtime.epoch) {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	providerAuthority, ok := endpoint.ProviderIncarnation()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	providerProof, ok := providerAuthority.Provider()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	incarnationID, ok := providerProof.IncarnationID()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	endpointProof, ok := endpoint.Endpoint()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	spki, ok := endpointProof.SPKISHA256Hex()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	token, ok := runtime.epoch.TokenHex()
	if !ok {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}

	process := runtime.process
	document := struct {
		RealmID             string `json:"realm_id"`
		RealmRevision       uint64 `json:"realm_revision"`
		PolicyDigest        string `json:"policy_digest"`
		WorldID             string `json:"world_id"`
		RealizationRevision uint64 `json:"realization_revision"`
		SubstrateHandle     string `json:"substrate_handle"`
		SandboxID           string `json:"sandbox_id"`
		Generation          uint64 `json:"generation"`
		EpochToken          string `json:"realization_epoch_token"`
		ProviderIncarnation string `json:"provider_incarnation_id"`
		EndpointSPKI        string `json:"endpoint_spki_sha256"`
		HostPID             uint32 `json:"host_pid"`
		StartTimeTicks      uint64 `json:"starttime_ticks"`
		BootID              string `json:"boot_id"`
		CGroupPath          string `json:"cgroup_path"`
		RuntimeRole         string `json:"runtime_role"`
		Source              string `json:"source"`
		PlacedAt            string `json:"placed_at"`
		BoundAt             string `json:"bound_at"`
	}{
		RealmID:             string(binding.RealmID),
		RealmRevision:       binding.RealmRevision,
		PolicyDigest:        binding.PolicyDigest,
		WorldID:             string(binding.WorldID),
		RealizationRevision: binding.RealizationRevision,
		SubstrateHandle:     string(binding.SubstrateHandle),
		SandboxID:           process.SandboxID,
		Generation:          process.Generation,
		EpochToken:          token,
		ProviderIncarnation: incarnationID,
		EndpointSPKI:        spki,
		HostPID:             process.HostPID,
		StartTimeTicks:      process.StartTimeTicks,
		BootID:              process.BootID,
		CGroupPath:          process.CGroupPath,
		RuntimeRole:         process.RuntimeRole,
		Source:              process.Source,
		PlacedAt:            process.PlacedAt.UTC().Format(time.RFC3339Nano),
		BoundAt:             process.BoundAt.UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return "", ErrInvalidRealmResourceRuntimeAuthority
	}
	preimage := append([]byte("nolane.runtime-realization.v27\x00"), raw...)
	digest := sha256.Sum256(preimage)
	return "runtime-realization-v27:" + hex.EncodeToString(digest[:]), nil
}

func validRuntimeRealizationDigest(raw string) bool {
	const prefix = "runtime-realization-v27:"
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	hexDigest := strings.TrimPrefix(raw, prefix)
	if len(hexDigest) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(hexDigest)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == hexDigest
}
