package live

import (
	"context"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type Mode string
type Profile string
type Status string
type Outcome string
type ReasonCode string

const (
	ModeProbe       Mode = "probe"
	ModeRequireLive Mode = "require-live"

	ProfileCore       Profile = "core"
	ProfileFullEgress Profile = "full-egress"

	StatusLivePass    Status = "LIVE_PASS"
	StatusLiveFail    Status = "LIVE_FAIL"
	StatusUnavailable Status = "UNAVAILABLE"

	OutcomePass        Outcome = "pass"
	OutcomeFail        Outcome = "fail"
	OutcomeUnavailable Outcome = "unavailable"

	ReasonNone             ReasonCode = "none"
	ReasonConfigMissing    ReasonCode = "config_missing"
	ReasonControlUnhealthy ReasonCode = "control_unhealthy"
	ReasonCreateFailed     ReasonCode = "create_failed"
	ReasonGuestFailed      ReasonCode = "guest_execution_failed"
	ReasonSnapshotFailed   ReasonCode = "snapshot_failed"
	ReasonRollbackFailed   ReasonCode = "rollback_failed"
	ReasonAuthorityFailed  ReasonCode = "authority_monotonicity_failed"
	ReasonCleanupFailed    ReasonCode = "cleanup_failed"
	ReasonTargetMissing    ReasonCode = "target_missing"
	ReasonTargetPreflight  ReasonCode = "target_preflight_failed"
	ReasonEgressViolation  ReasonCode = "egress_violation"
	ReasonVerification     ReasonCode = "verification_failed"

	ScenarioGuestExecution    = "live.cube.guest-execution"
	ScenarioSnapshotAuthority = "live.cube.snapshot-authority-monotonicity"
	ScenarioEgressHTTP        = "live.cube.egress-http"
	ScenarioEgressTCP         = "live.cube.egress-tcp"
	ScenarioEgressUDP         = "live.cube.egress-udp"
	ScenarioEgressDNS         = "live.cube.egress-dns"
)

var (
	ErrInvalidLiveReport = errors.New("live gauntlet: invalid report")
	ErrLiveUnavailable   = errors.New("live gauntlet: unavailable")
	ErrLiveFailed        = errors.New("live gauntlet: failed")
	ErrProbeUnsupported  = errors.New("live gauntlet: probe unsupported")
	ErrCleanupFailed     = errors.New("live gauntlet: cleanup failed")
)

type Fingerprint struct {
	EndpointDigest string `json:"endpoint_digest"`
	TemplateDigest string `json:"template_digest"`
}

type Capabilities struct {
	ControlPlane     bool `json:"control_plane"`
	GuestExecution   bool `json:"guest_execution"`
	SnapshotRollback bool `json:"snapshot_rollback"`
	CleanupObserved  bool `json:"cleanup_observed"`
	EgressHTTP       bool `json:"egress_http"`
	EgressTCP        bool `json:"egress_tcp"`
	EgressUDP        bool `json:"egress_udp"`
	EgressDNS        bool `json:"egress_dns"`
}

type ScenarioEvidence struct {
	ID            string     `json:"id"`
	Outcome       Outcome    `json:"outcome"`
	Reason        ReasonCode `json:"reason"`
	RuntimeDigest string     `json:"runtime_digest,omitempty"`
	Markers       []string   `json:"markers"`
	Digest        string     `json:"digest"`
}

type Report struct {
	SchemaVersion  int                `json:"schema_version"`
	Profile        Profile            `json:"profile"`
	Mode           Mode               `json:"mode"`
	Substrate      string             `json:"substrate"`
	Status         Status             `json:"status"`
	Reason         ReasonCode         `json:"reason"`
	Approved       bool               `json:"approved"`
	EndpointDigest string             `json:"endpoint_digest,omitempty"`
	TemplateDigest string             `json:"template_digest,omitempty"`
	Capabilities   Capabilities       `json:"capabilities"`
	Scenarios      []ScenarioEvidence `json:"scenarios"`
	Digest         string             `json:"digest"`
}

type Sandbox interface {
	Digest() string
	Canary(context.Context) error
	PutSentinel(context.Context, string) error
	ReadSentinel(context.Context) (string, error)
	Snapshot(context.Context) (substrate.Snapshot, error)
	Rollback(context.Context, substrate.Snapshot) error
	DestroyObserved(context.Context) error
	ProbeEgress(context.Context, Target) (EgressObservation, error)
}

type Driver interface {
	Fingerprint() Fingerprint
	Health(context.Context) error
	Create(context.Context, world.ID) (Sandbox, error)
}

type TargetKind string

const (
	TargetHTTP TargetKind = "http"
	TargetTCP  TargetKind = "tcp"
	TargetUDP  TargetKind = "udp"
	TargetDNS  TargetKind = "dns"
)

type Target struct {
	Kind    TargetKind `json:"kind"`
	Address string     `json:"address"`
	Expect  string     `json:"expect,omitempty"`
}

type EgressObservation struct {
	Reached bool
	Marker  string
}
