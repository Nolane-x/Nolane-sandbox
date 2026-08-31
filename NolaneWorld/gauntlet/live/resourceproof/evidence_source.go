package resourceproof

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidCapabilityEvidence = errors.New("live resource proof: invalid capability evidence")

type CapabilityEvidenceBinding struct {
	RealmID             realm.ID
	RealmRevision       uint64
	PolicyDigest        string
	RealizationRevision uint64
	RuntimeDigest       string
}

type capabilityEvidenceSource struct {
	query    agentruntime.CapabilityEvidenceQuery
	snapshot agentruntime.CapabilityEvidenceSnapshot
}

// NewCapabilityEvidenceSource converts one sealed v11 LIVE_PASS into immutable
// host-owned capability evidence bound to the exact Realm policy and runtime
// realization that produced the causal CPU and memory observations.
func NewCapabilityEvidenceSource(report Report, binding CapabilityEvidenceBinding) (agentruntime.CapabilityEvidenceSource, error) {
	if err := VerifyReport(report); err != nil || report.Status != live.StatusLivePass || !report.Approved {
		return nil, ErrInvalidCapabilityEvidence
	}
	if binding.RealmID == "" || binding.RealmRevision == 0 || strings.TrimSpace(binding.PolicyDigest) == "" ||
		binding.RealizationRevision == 0 || strings.TrimSpace(binding.RuntimeDigest) == "" {
		return nil, ErrInvalidCapabilityEvidence
	}
	if binding.RealmID != report.Binding.RealmID || binding.RealmRevision != report.Binding.RealmRevision ||
		binding.PolicyDigest != report.Binding.PolicyDigest || binding.RealizationRevision != report.Binding.RealizationRevision ||
		binding.RuntimeDigest != report.Binding.RuntimeDigest {
		return nil, ErrInvalidCapabilityEvidence
	}
	if report.CPU.State != agentruntime.Verified || strings.TrimSpace(report.CPU.Evidence) == "" ||
		report.Memory.State != agentruntime.Verified || strings.TrimSpace(report.Memory.Evidence) == "" ||
		report.Disk.State == agentruntime.Verified {
		return nil, ErrInvalidCapabilityEvidence
	}

	attestation := agentruntime.ProviderAttestation{
		CPUEnforcementAvailable:       true,
		CPUEnforcementVerified:        true,
		CPUEnforcementEvidence:        report.CPU.Evidence,
		MemoryEnforcementAvailable:    true,
		MemoryEnforcementVerified:     true,
		MemoryEnforcementEvidence:     report.Memory.Evidence,
		ResourceEnforcementAvailable: true,
	}
	query := agentruntime.CapabilityEvidenceQuery{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision,
		PolicyDigest:  binding.PolicyDigest,
	}
	snapshot := agentruntime.CapabilityEvidenceSnapshot{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision,
		PolicyDigest:  binding.PolicyDigest,
		Attestation:   attestation,
	}
	return &capabilityEvidenceSource{query: query, snapshot: snapshot}, nil
}

func (s *capabilityEvidenceSource) Snapshot(ctx context.Context, query agentruntime.CapabilityEvidenceQuery) (agentruntime.CapabilityEvidenceSnapshot, bool, error) {
	if s == nil {
		return agentruntime.CapabilityEvidenceSnapshot{}, false, ErrInvalidCapabilityEvidence
	}
	if err := ctx.Err(); err != nil {
		return agentruntime.CapabilityEvidenceSnapshot{}, false, err
	}
	if query != s.query {
		return agentruntime.CapabilityEvidenceSnapshot{}, false, nil
	}
	return s.snapshot, true, nil
}
