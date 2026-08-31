package capabilityproof

import (
	"errors"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

type ReasonCode string

const (
	ReasonNone                 ReasonCode = "none"
	ReasonConfigMissing        ReasonCode = "config_missing"
	ReasonFingerprintInvalid   ReasonCode = "fingerprint_invalid"
	ReasonEvidenceMismatch     ReasonCode = "evidence_mismatch"
	ReasonSubstrateUnavailable ReasonCode = "substrate_unavailable"
	ReasonRealmUnavailable     ReasonCode = "realm_unavailable"
	ReasonSubstrateFailed      ReasonCode = "substrate_failed"
	ReasonRealmFailed          ReasonCode = "realm_failed"
)

var (
	ErrInvalidReport = errors.New("live capability proof: invalid report")
	ErrUnavailable   = errors.New("live capability proof: unavailable")
	ErrFailed        = errors.New("live capability proof: failed")
)

type Capabilities struct {
	GuestExecution       bool `json:"guest_execution"`
	SnapshotRollback     bool `json:"snapshot_rollback"`
	PublicIngressDenied  bool `json:"public_ingress_denied"`
	NetworkIsolation     bool `json:"network_isolation"`
	InternalMeshVerified bool `json:"internal_mesh_verified"`
}

type Report struct {
	SchemaVersion  int                  `json:"schema_version"`
	Profile        realm.NetworkProfile `json:"profile"`
	Mode           live.Mode            `json:"mode"`
	Substrate      string               `json:"substrate"`
	Status         live.Status          `json:"status"`
	Reason         ReasonCode           `json:"reason"`
	Approved       bool                 `json:"approved"`
	EndpointDigest string               `json:"endpoint_digest,omitempty"`
	TemplateDigest string               `json:"template_digest,omitempty"`
	SubstrateProof live.Report          `json:"substrate_proof"`
	RealmProof     realmproof.Report     `json:"realm_proof"`
	Capabilities   Capabilities         `json:"capabilities"`
	Digest         string               `json:"digest"`
}

type Runner struct {
	Mode            live.Mode
	Profile         realm.NetworkProfile
	RawPublicTarget live.Target
}
