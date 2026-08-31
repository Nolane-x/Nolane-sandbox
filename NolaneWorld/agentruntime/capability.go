package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/membrane"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

type ClaimState string

const (
	Verified          ClaimState = "verified"
	AvailableUnproven ClaimState = "available-unproven"
	Unavailable       ClaimState = "unavailable"
	NotApplicable     ClaimState = "not-applicable"
)

var (
	ErrCapabilityEvidenceMismatch    = errors.New("agentruntime: capability evidence binding mismatch")
	ErrCapabilityEvidenceUnavailable = errors.New("agentruntime: capability evidence unavailable")
)

type Claim struct {
	State    ClaimState `json:"state"`
	Evidence string     `json:"evidence,omitempty"`
}

type ProviderAttestation struct {
	GuestExecAvailable              bool
	GuestExecVerified               bool
	GuestExecEvidence               string
	SnapshotAvailable               bool
	SnapshotVerified                bool
	SnapshotEvidence                string
	PublicReadAvailable             bool
	PublicReadVerified              bool
	PublicReadEvidence              string
	PublicInboundDisabled           bool
	PublicInboundEvidence           string
	InternalMeshAvailable           bool
	InternalMeshVerified            bool
	InternalMeshEvidence            string
	FilesystemIsolationVerified     bool
	FilesystemIsolationEvidence     string
	ProcessIsolationVerified        bool
	ProcessIsolationEvidence        string
	NetworkIsolationVerified        bool
	NetworkIsolationEvidence        string
	CPUEnforcementAvailable         bool
	CPUEnforcementVerified          bool
	CPUEnforcementEvidence          string
	MemoryEnforcementAvailable      bool
	MemoryEnforcementVerified       bool
	MemoryEnforcementEvidence       string
	DiskEnforcementAvailable        bool
	DiskEnforcementVerified         bool
	DiskEnforcementEvidence         string
	ResourceEnforcementAvailable    bool
	ResourceEnforcementVerified     bool
	ResourceEnforcementEvidence     string
}

type CapabilityEvidenceQuery struct {
	RealmID       realm.ID
	RealmRevision uint64
	PolicyDigest  string
}

type CapabilityEvidenceSnapshot struct {
	RealmID       realm.ID
	RealmRevision uint64
	PolicyDigest  string
	Attestation   ProviderAttestation
}

type CapabilityEvidenceSource interface {
	Snapshot(context.Context, CapabilityEvidenceQuery) (CapabilityEvidenceSnapshot, bool, error)
}

type CapabilityRequest struct {
	SessionID     realm.SessionID
	RealmRevision uint64
	// Attestation is a legacy compatibility hint surface. Caller-supplied
	// verification bits and evidence are never trusted; only availability
	// booleans may be downgraded to AvailableUnproven.
	Attestation ProviderAttestation
}

type CapabilityReport struct {
	RealmID             realm.ID             `json:"realm_id"`
	RealmRevision       uint64               `json:"realm_revision"`
	NetworkProfile      realm.NetworkProfile `json:"network_profile"`
	GuestExec           Claim                `json:"guest_exec"`
	SnapshotRollback    Claim                `json:"snapshot_rollback"`
	PublicRead          Claim                `json:"public_read"`
	PublicInbound       Claim                `json:"public_inbound"`
	InternalMesh        Claim                `json:"internal_mesh"`
	FilesystemIsolation Claim                `json:"filesystem_isolation"`
	ProcessIsolation    Claim                `json:"process_isolation"`
	NetworkIsolation    Claim                `json:"network_isolation"`
	CPUEnforcement      Claim                `json:"cpu_enforcement"`
	MemoryEnforcement   Claim                `json:"memory_enforcement"`
	DiskEnforcement     Claim                `json:"disk_enforcement"`
	ResourceEnforcement Claim                `json:"resource_enforcement"`
	AccountingBudget    realm.ResourceBudget `json:"accounting_budget"`
	EvidenceDigest      string               `json:"evidence_digest"`
}

func (s *Service) Capabilities(ctx context.Context, req CapabilityRequest) (CapabilityReport, error) {
	if s == nil {
		return CapabilityReport{}, ErrInvalidRuntime
	}
	sess, err := s.validateSession(req.SessionID, req.RealmRevision)
	if err != nil {
		return CapabilityReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return CapabilityReport{}, err
	}
	rr, ok := s.store.Realm(sess.RealmID)
	if !ok {
		return CapabilityReport{}, ErrStaleSession
	}
	plan, err := membrane.Plan(rr.Spec.NetworkProfile)
	if err != nil {
		return CapabilityReport{}, err
	}

	a := untrustedAvailabilityHints(req.Attestation)
	if s.capabilityEvidence != nil {
		query := CapabilityEvidenceQuery{RealmID: sess.RealmID, RealmRevision: sess.RealmRevision, PolicyDigest: sess.PolicyDigest}
		snapshot, found, sourceErr := s.capabilityEvidence.Snapshot(ctx, query)
		if sourceErr != nil {
			return CapabilityReport{}, errors.Join(ErrCapabilityEvidenceUnavailable, sourceErr)
		}
		if found {
			if snapshot.RealmID != query.RealmID || snapshot.RealmRevision != query.RealmRevision || snapshot.PolicyDigest != query.PolicyDigest {
				return CapabilityReport{}, ErrCapabilityEvidenceMismatch
			}
			a = snapshot.Attestation
		}
	}

	cpu := claim(a.CPUEnforcementAvailable, a.CPUEnforcementVerified, a.CPUEnforcementEvidence)
	memory := claim(a.MemoryEnforcementAvailable, a.MemoryEnforcementVerified, a.MemoryEnforcementEvidence)
	disk := claim(a.DiskEnforcementAvailable, a.DiskEnforcementVerified, a.DiskEnforcementEvidence)
	report := CapabilityReport{
		RealmID:               sess.RealmID,
		RealmRevision:         sess.RealmRevision,
		NetworkProfile:        rr.Spec.NetworkProfile,
		GuestExec:             claim(a.GuestExecAvailable, a.GuestExecVerified, a.GuestExecEvidence),
		SnapshotRollback:      claim(a.SnapshotAvailable, a.SnapshotVerified, a.SnapshotEvidence),
		InternalMesh:          claim(a.InternalMeshAvailable, a.InternalMeshVerified, a.InternalMeshEvidence),
		FilesystemIsolation:   verifiedOnly(a.FilesystemIsolationVerified, a.FilesystemIsolationEvidence),
		ProcessIsolation:      verifiedOnly(a.ProcessIsolationVerified, a.ProcessIsolationEvidence),
		NetworkIsolation:      verifiedOnly(a.NetworkIsolationVerified, a.NetworkIsolationEvidence),
		CPUEnforcement:        cpu,
		MemoryEnforcement:     memory,
		DiskEnforcement:       disk,
		ResourceEnforcement:   aggregateResourceEnforcement(a.ResourceEnforcementAvailable, cpu, memory, disk),
		AccountingBudget:      rr.Spec.ResourceBudget,
	}
	if plan.RequiresRealityGateway {
		report.PublicRead = claim(a.PublicReadAvailable, a.PublicReadVerified, a.PublicReadEvidence)
	} else {
		report.PublicRead = Claim{State: NotApplicable}
	}
	if a.PublicInboundDisabled && a.PublicInboundEvidence != "" {
		report.PublicInbound = Claim{State: Verified, Evidence: a.PublicInboundEvidence}
	} else if !plan.PublicInboundAllowed {
		report.PublicInbound = Claim{State: AvailableUnproven}
	} else {
		report.PublicInbound = Claim{State: Unavailable}
	}
	report.EvidenceDigest = capabilityDigest(report)
	return report, nil
}

func untrustedAvailabilityHints(a ProviderAttestation) ProviderAttestation {
	return ProviderAttestation{
		GuestExecAvailable:           a.GuestExecAvailable,
		SnapshotAvailable:            a.SnapshotAvailable,
		PublicReadAvailable:          a.PublicReadAvailable,
		InternalMeshAvailable:        a.InternalMeshAvailable,
		CPUEnforcementAvailable:      a.CPUEnforcementAvailable,
		MemoryEnforcementAvailable:   a.MemoryEnforcementAvailable,
		DiskEnforcementAvailable:     a.DiskEnforcementAvailable,
		ResourceEnforcementAvailable: a.ResourceEnforcementAvailable,
	}
}

func claim(available, verified bool, evidence string) Claim {
	if verified && evidence != "" {
		return Claim{State: Verified, Evidence: evidence}
	}
	if available {
		return Claim{State: AvailableUnproven}
	}
	return Claim{State: Unavailable}
}

func verifiedOnly(verified bool, evidence string) Claim {
	if verified && evidence != "" {
		return Claim{State: Verified, Evidence: evidence}
	}
	return Claim{State: Unavailable}
}

func aggregateResourceEnforcement(legacyAvailable bool, cpu, memory, disk Claim) Claim {
	if cpu.State == Verified && memory.State == Verified && disk.State == Verified {
		raw := []byte(cpu.Evidence + "\x00" + memory.Evidence + "\x00" + disk.Evidence)
		h := sha256.Sum256(append([]byte("nolane.resource-enforcement.aggregate.v11\x00"), raw...))
		return Claim{State: Verified, Evidence: "resource-v11:aggregate:" + hex.EncodeToString(h[:])}
	}
	if legacyAvailable || cpu.State != Unavailable || memory.State != Unavailable || disk.State != Unavailable {
		return Claim{State: AvailableUnproven}
	}
	return Claim{State: Unavailable}
}

func capabilityDigest(report CapabilityReport) string {
	report.EvidenceDigest = ""
	raw, _ := json.Marshal(report)
	h := sha256.Sum256(append([]byte("nolane.capability-report.v11\x00"), raw...))
	return hex.EncodeToString(h[:])
}
