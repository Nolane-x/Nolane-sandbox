package realmproof

import (
	"errors"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

type ReasonCode string

const (
	ReasonNone              ReasonCode = "none"
	ReasonInvalidReport     ReasonCode = "invalid_report"
	ReasonConfigMissing     ReasonCode = "config_missing"
	ReasonControlUnhealthy  ReasonCode = "control_unhealthy"
	ReasonDriverUnsupported ReasonCode = "driver_unsupported"
	ReasonTargetMissing     ReasonCode = "target_missing"
	ReasonTargetPreflight   ReasonCode = "target_preflight_failed"
	ReasonCreateFailed      ReasonCode = "create_failed"
	ReasonProfileApply      ReasonCode = "profile_apply_failed"
	ReasonGuestFailed       ReasonCode = "guest_after_profile_failed"
	ReasonRawPublicReachable ReasonCode = "raw_public_reachable"
	ReasonIngressViolation  ReasonCode = "public_ingress_violation"
	ReasonIngressUnavailable ReasonCode = "public_ingress_unavailable"
	ReasonMeshUnsupported   ReasonCode = "private_mesh_unsupported"
	ReasonMeshFailed        ReasonCode = "private_mesh_failed"
	ReasonCleanupFailed     ReasonCode = "cleanup_failed"
)

const (
	ScenarioProfileApply        = "live.realm.profile-apply"
	ScenarioGuestAfterProfile   = "live.realm.guest-after-profile"
	ScenarioRawPublicDenied     = "live.realm.raw-public-denied"
	ScenarioPublicIngressDenied = "live.realm.public-ingress-denied"
	ScenarioInternalMesh        = "live.realm.internal-mesh"
	ScenarioCleanup             = "live.realm.cleanup-observed"
)

var (
	ErrInvalidReport = errors.New("live realm proof: invalid report")
	ErrUnavailable   = errors.New("live realm proof: unavailable")
	ErrFailed        = errors.New("live realm proof: failed")
)

type Capabilities struct {
	GuestExecution       bool `json:"guest_execution"`
	RawPublicDenied      bool `json:"raw_public_denied"`
	PublicIngressDenied  bool `json:"public_ingress_denied"`
	InternalMeshVerified bool `json:"internal_mesh_verified"`
}

type ScenarioEvidence struct {
	ID      string          `json:"id"`
	Outcome live.Outcome    `json:"outcome"`
	Reason  ReasonCode      `json:"reason"`
	Markers []string        `json:"markers"`
	Digest  string          `json:"digest"`
}

type Report struct {
	SchemaVersion  int                  `json:"schema_version"`
	Profile        realm.NetworkProfile `json:"profile"`
	Mode           live.Mode            `json:"mode"`
	Substrate      string               `json:"substrate"`
	Status         live.Status          `json:"status"`
	Reason         ReasonCode            `json:"reason"`
	Approved       bool                 `json:"approved"`
	EndpointDigest string               `json:"endpoint_digest,omitempty"`
	TargetDigest   string               `json:"target_digest,omitempty"`
	Capabilities   Capabilities         `json:"capabilities"`
	Scenarios      []ScenarioEvidence   `json:"scenarios"`
	Digest         string               `json:"digest"`
}
