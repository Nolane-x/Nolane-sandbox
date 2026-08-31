package resourceproof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

const SchemaVersion = 11

type SourceKind string
type ReasonCode string

const (
	// SourceLiveHost is descriptive provenance metadata only. It is deliberately
	// not an authority token: public callers may copy this value, so BuildReport
	// must never upgrade a claim merely because this label is present.
	SourceLiveHost SourceKind = "live-host"
	SourceFixture  SourceKind = "fixture"

	ReasonNone                ReasonCode = "none"
	ReasonInvalidBinding      ReasonCode = "invalid_binding"
	ReasonEvidenceUnavailable ReasonCode = "evidence_unavailable"
	ReasonCPULimitMismatch    ReasonCode = "cpu_limit_mismatch"
	ReasonCPUThrottleMissing  ReasonCode = "cpu_throttle_missing"
	ReasonMemoryLimitMismatch ReasonCode = "memory_limit_mismatch"
	ReasonMemoryOOMMissing    ReasonCode = "memory_oom_missing"
)

var ErrInvalidReport = errors.New("live resource proof: invalid report")

type Binding struct {
	RealmID             realm.ID `json:"realm_id"`
	RealmRevision       uint64   `json:"realm_revision"`
	PolicyDigest        string   `json:"policy_digest"`
	RealizationRevision uint64   `json:"realization_revision"`
	RuntimeDigest       string   `json:"runtime_digest"`
}

type CPUObservation struct {
	Source                SourceKind `json:"source"`
	RequestedQuotaMicros  int64      `json:"requested_quota_micros"`
	RequestedPeriodMicros uint64     `json:"requested_period_micros"`
	EffectiveQuotaMicros  int64      `json:"effective_quota_micros"`
	EffectivePeriodMicros uint64     `json:"effective_period_micros"`
	PressureObserved      bool       `json:"pressure_observed"`
	NrThrottledBefore     uint64     `json:"nr_throttled_before"`
	NrThrottledAfter      uint64     `json:"nr_throttled_after"`
	ThrottledUsecBefore   uint64     `json:"throttled_usec_before"`
	ThrottledUsecAfter    uint64     `json:"throttled_usec_after"`
}

type MemoryObservation struct {
	Source              SourceKind `json:"source"`
	RequestedLimitBytes uint64     `json:"requested_limit_bytes"`
	EffectiveLimitBytes uint64     `json:"effective_limit_bytes"`
	AttemptedBytes      uint64     `json:"attempted_bytes"`
	OOMEventsBefore     uint64     `json:"oom_events_before"`
	OOMEventsAfter      uint64     `json:"oom_events_after"`
	ExitCode            int        `json:"exit_code"`
	ExitReason          string     `json:"exit_reason"`
}

type CPUProof struct {
	State       agentruntime.ClaimState `json:"state"`
	Evidence    string                  `json:"evidence,omitempty"`
	Reason      ReasonCode              `json:"reason"`
	Observation CPUObservation          `json:"observation"`
}

type MemoryProof struct {
	State       agentruntime.ClaimState `json:"state"`
	Evidence    string                  `json:"evidence,omitempty"`
	Reason      ReasonCode              `json:"reason"`
	Observation MemoryObservation       `json:"observation"`
}

type DimensionProof struct {
	State    agentruntime.ClaimState `json:"state"`
	Evidence string                  `json:"evidence,omitempty"`
	Reason   ReasonCode              `json:"reason"`
}

type Report struct {
	SchemaVersion int            `json:"schema_version"`
	Mode          live.Mode      `json:"mode"`
	Status        live.Status    `json:"status"`
	Reason        ReasonCode     `json:"reason"`
	Approved      bool           `json:"approved"`
	Binding       Binding        `json:"binding"`
	CPU           CPUProof       `json:"cpu"`
	Memory        MemoryProof    `json:"memory"`
	Disk          DimensionProof `json:"disk"`
	Digest        string         `json:"digest"`
}

// TrustedReport separates a serializable report document from the authority to
// consume that document as live capability evidence. Its state is intentionally
// unexported, so code outside this package cannot mint one from caller-supplied
// observations. A future concrete host adapter must live inside this trust
// boundary and may call buildTrustedReport only after it has obtained the
// corresponding host-owned observations.
type TrustedReport struct {
	report Report
}

// Report returns a copy suitable for logging or artifact serialization. The
// returned document is evidence data, not an authority-bearing capability.
func (t TrustedReport) Report() Report { return t.report }

// BuildReport classifies public/caller-supplied observations. Even if a caller
// labels them "live-host", provenance is not established by a copyable string,
// so this path is fail-honest and can never mint VERIFIED live enforcement.
func BuildReport(mode live.Mode, binding Binding, cpu CPUObservation, memory MemoryObservation) Report {
	return buildReport(mode, binding, cpu, memory, false)
}

// buildTrustedReport is package-owned on purpose. It is the only path that may
// classify causal CPU/memory observations as live proof. Production callers
// must not receive a public constructor for this authority boundary.
func buildTrustedReport(mode live.Mode, binding Binding, cpu CPUObservation, memory MemoryObservation) TrustedReport {
	return TrustedReport{report: buildReport(mode, binding, cpu, memory, true)}
}

func buildReport(mode live.Mode, binding Binding, cpu CPUObservation, memory MemoryObservation, trusted bool) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		Mode:          mode,
		Status:        live.StatusLiveFail,
		Reason:        ReasonInvalidBinding,
		Binding:       binding,
		CPU:           CPUProof{State: agentruntime.Unavailable, Reason: ReasonInvalidBinding, Observation: cpu},
		Memory:        MemoryProof{State: agentruntime.Unavailable, Reason: ReasonInvalidBinding, Observation: memory},
		Disk:          DimensionProof{State: agentruntime.Unavailable, Reason: ReasonEvidenceUnavailable},
	}
	if !bindingValid(binding) || (mode != live.ModeProbe && mode != live.ModeRequireLive) {
		return seal(report)
	}
	if !trusted || cpu.Source != SourceLiveHost || memory.Source != SourceLiveHost {
		report.Status = live.StatusUnavailable
		report.Reason = ReasonEvidenceUnavailable
		report.CPU.Reason = ReasonEvidenceUnavailable
		report.Memory.Reason = ReasonEvidenceUnavailable
		return seal(report)
	}

	cpuReason := verifyCPU(cpu)
	memoryReason := verifyMemory(memory)
	if cpuReason != ReasonNone || memoryReason != ReasonNone {
		report.Status = live.StatusLiveFail
		report.Reason = ReasonNone
		report.CPU.Reason = cpuReason
		report.Memory.Reason = memoryReason
		if cpuReason != ReasonNone {
			report.Reason = cpuReason
		} else {
			report.Reason = memoryReason
		}
		return seal(report)
	}

	report.Status = live.StatusLivePass
	report.Reason = ReasonNone
	report.Approved = true
	report.CPU.State = agentruntime.Verified
	report.CPU.Reason = ReasonNone
	report.CPU.Evidence = evidenceDigest("cpu", binding, cpu)
	report.Memory.State = agentruntime.Verified
	report.Memory.Reason = ReasonNone
	report.Memory.Evidence = evidenceDigest("memory", binding, memory)
	return seal(report)
}

// VerifyReport validates only the public, untrusted report domain. In
// particular it rejects a forged LIVE_PASS document even when every numeric
// field is internally consistent.
func VerifyReport(report Report) error {
	if report.SchemaVersion != SchemaVersion {
		return ErrInvalidReport
	}
	expected := BuildReport(report.Mode, report.Binding, report.CPU.Observation, report.Memory.Observation)
	if !reflect.DeepEqual(report, expected) {
		return ErrInvalidReport
	}
	return nil
}

// VerifyTrustedReport validates a package-owned authority wrapper. This is the
// verification primitive used by capability projection; plain Report values
// are never accepted as proof of provenance.
func VerifyTrustedReport(trusted TrustedReport) error {
	return verifyTrustedDocument(trusted.report)
}

func verifyTrustedDocument(report Report) error {
	if report.SchemaVersion != SchemaVersion {
		return ErrInvalidReport
	}
	expected := buildReport(report.Mode, report.Binding, report.CPU.Observation, report.Memory.Observation, true)
	if !reflect.DeepEqual(report, expected) {
		return ErrInvalidReport
	}
	return nil
}

func bindingValid(binding Binding) bool {
	return binding.RealmID != "" && binding.RealmRevision > 0 && binding.RealizationRevision > 0 &&
		strings.TrimSpace(binding.PolicyDigest) != "" && strings.TrimSpace(binding.RuntimeDigest) != ""
}

func verifyCPU(cpu CPUObservation) ReasonCode {
	if cpu.RequestedQuotaMicros <= 0 || cpu.RequestedPeriodMicros == 0 ||
		cpu.EffectiveQuotaMicros != cpu.RequestedQuotaMicros || cpu.EffectivePeriodMicros != cpu.RequestedPeriodMicros {
		return ReasonCPULimitMismatch
	}
	if !cpu.PressureObserved || cpu.NrThrottledAfter < cpu.NrThrottledBefore || cpu.ThrottledUsecAfter < cpu.ThrottledUsecBefore ||
		(cpu.NrThrottledAfter == cpu.NrThrottledBefore && cpu.ThrottledUsecAfter == cpu.ThrottledUsecBefore) {
		return ReasonCPUThrottleMissing
	}
	return ReasonNone
}

func verifyMemory(memory MemoryObservation) ReasonCode {
	if memory.RequestedLimitBytes == 0 || memory.EffectiveLimitBytes != memory.RequestedLimitBytes {
		return ReasonMemoryLimitMismatch
	}
	if memory.AttemptedBytes <= memory.EffectiveLimitBytes || memory.OOMEventsAfter <= memory.OOMEventsBefore ||
		memory.ExitCode != 137 || memory.ExitReason != "OOMKilled" {
		return ReasonMemoryOOMMissing
	}
	return ReasonNone
}

func evidenceDigest(kind string, binding Binding, observation any) string {
	raw, _ := json.Marshal(struct {
		Binding     Binding `json:"binding"`
		Observation any     `json:"observation"`
	}{Binding: binding, Observation: observation})
	h := sha256.Sum256(append([]byte("nolane.live-resource-v11/"+kind+"\x00"), raw...))
	return "live-resource-v11:" + kind + ":" + hex.EncodeToString(h[:])
}

func seal(report Report) Report {
	report.Digest = ""
	raw, _ := json.Marshal(report)
	h := sha256.Sum256(append([]byte("nolane.live-resource-report.v11\x00"), raw...))
	report.Digest = hex.EncodeToString(h[:])
	return report
}
