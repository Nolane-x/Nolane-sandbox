package capabilityproof

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidCapabilityEvidence = errors.New("live capability proof: invalid capability evidence")

type CapabilityEvidenceBinding struct {
	RealmID       realm.ID
	RealmRevision uint64
	PolicyDigest  string
}

type capabilityEvidenceSource struct {
	query    agentruntime.CapabilityEvidenceQuery
	snapshot agentruntime.CapabilityEvidenceSnapshot
}

// NewCapabilityEvidenceSource converts one sealed v10 LIVE_PASS into immutable
// host-owned evidence for exactly one Realm identity, revision, and policy.
func NewCapabilityEvidenceSource(report Report, binding CapabilityEvidenceBinding) (agentruntime.CapabilityEvidenceSource, error) {
	if err := VerifyReport(report); err != nil || report.Status != live.StatusLivePass || !report.Approved {
		return nil, ErrInvalidCapabilityEvidence
	}
	if binding.RealmID == "" || binding.RealmRevision == 0 || strings.TrimSpace(binding.PolicyDigest) == "" {
		return nil, ErrInvalidCapabilityEvidence
	}

	substrateGuest, substrateGuestOK := liveScenario(report.SubstrateProof, live.ScenarioGuestExecution)
	snapshotProof, snapshotOK := liveScenario(report.SubstrateProof, live.ScenarioSnapshotAuthority)
	realmGuest, realmGuestOK := realmScenario(report.RealmProof, realmproof.ScenarioGuestAfterProfile)
	rawPublic, rawOK := realmScenario(report.RealmProof, realmproof.ScenarioRawPublicDenied)
	ingress, ingressOK := realmScenario(report.RealmProof, realmproof.ScenarioPublicIngressDenied)
	if !substrateGuestOK || !snapshotOK || !realmGuestOK || !rawOK || !ingressOK ||
		substrateGuest.Outcome != live.OutcomePass || snapshotProof.Outcome != live.OutcomePass ||
		realmGuest.Outcome != live.OutcomePass || rawPublic.Outcome != live.OutcomePass || ingress.Outcome != live.OutcomePass {
		return nil, ErrInvalidCapabilityEvidence
	}

	attestation := agentruntime.ProviderAttestation{
		GuestExecAvailable:       true,
		GuestExecVerified:        true,
		GuestExecEvidence:        capabilityEvidenceRef("guest", proofHash("nolane-live-capability-v10/guest-evidence", substrateGuest.Digest, realmGuest.Digest, report.Digest)),
		SnapshotAvailable:        true,
		SnapshotVerified:         true,
		SnapshotEvidence:         capabilityEvidenceRef("snapshot", snapshotProof.Digest),
		PublicInboundDisabled:    true,
		PublicInboundEvidence:    capabilityEvidenceRef("public-inbound", ingress.Digest),
		NetworkIsolationVerified: true,
		NetworkIsolationEvidence: capabilityEvidenceRef("network", proofHash("nolane-live-capability-v10/network-evidence", rawPublic.Digest, ingress.Digest, report.Digest)),
	}

	if report.Capabilities.InternalMeshVerified {
		mesh, ok := realmScenario(report.RealmProof, realmproof.ScenarioInternalMesh)
		if !ok || mesh.Outcome != live.OutcomePass {
			return nil, ErrInvalidCapabilityEvidence
		}
		attestation.InternalMeshAvailable = true
		attestation.InternalMeshVerified = true
		attestation.InternalMeshEvidence = capabilityEvidenceRef("internal-mesh", mesh.Digest)
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

func liveScenario(report live.Report, id string) (live.ScenarioEvidence, bool) {
	for _, scenario := range report.Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return live.ScenarioEvidence{}, false
}

func realmScenario(report realmproof.Report, id string) (realmproof.ScenarioEvidence, bool) {
	for _, scenario := range report.Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return realmproof.ScenarioEvidence{}, false
}

func capabilityEvidenceRef(kind, digest string) string {
	return "live-capability-v10:" + kind + ":" + digest
}
