package realmproof

import (
	"context"
	"errors"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var ErrInvalidCapabilityEvidence = errors.New("live realm proof: invalid capability evidence")

type CapabilityEvidenceBinding struct {
	RealmID       realm.ID
	RealmRevision uint64
	PolicyDigest  string
}

type capabilityEvidenceSource struct {
	query    agentruntime.CapabilityEvidenceQuery
	snapshot agentruntime.CapabilityEvidenceSnapshot
}

// NewCapabilityEvidenceSource converts a sealed successful live Realm proof
// into an immutable host-owned capability evidence source. The adapter is
// intentionally located with the evidence producer so agentruntime does not
// depend on the live gauntlet implementation.
func NewCapabilityEvidenceSource(report Report, binding CapabilityEvidenceBinding) (agentruntime.CapabilityEvidenceSource, error) {
	if err := VerifyReport(report); err != nil || report.Status != live.StatusLivePass || !report.Approved {
		return nil, ErrInvalidCapabilityEvidence
	}
	if binding.RealmID == "" || binding.RealmRevision == 0 || strings.TrimSpace(binding.PolicyDigest) == "" {
		return nil, ErrInvalidCapabilityEvidence
	}

	guest, guestOK := scenarioEvidenceByID(report, ScenarioGuestAfterProfile)
	rawPublic, rawOK := scenarioEvidenceByID(report, ScenarioRawPublicDenied)
	ingress, ingressOK := scenarioEvidenceByID(report, ScenarioPublicIngressDenied)
	if !guestOK || !rawOK || !ingressOK || guest.Outcome != live.OutcomePass || rawPublic.Outcome != live.OutcomePass || ingress.Outcome != live.OutcomePass {
		return nil, ErrInvalidCapabilityEvidence
	}

	attestation := agentruntime.ProviderAttestation{
		GuestExecAvailable:          true,
		GuestExecVerified:           true,
		GuestExecEvidence:           evidenceRef("guest", guest.Digest),
		PublicInboundDisabled:       true,
		PublicInboundEvidence:       evidenceRef("public-inbound", ingress.Digest),
		NetworkIsolationVerified:    true,
		NetworkIsolationEvidence:    evidenceRef("network", proofHash("nolane-live-realm-v9/network-evidence", rawPublic.Digest, ingress.Digest, report.Digest)),
	}
	if report.Capabilities.InternalMeshVerified {
		mesh, ok := scenarioEvidenceByID(report, ScenarioInternalMesh)
		if !ok || mesh.Outcome != live.OutcomePass {
			return nil, ErrInvalidCapabilityEvidence
		}
		attestation.InternalMeshAvailable = true
		attestation.InternalMeshVerified = true
		attestation.InternalMeshEvidence = evidenceRef("internal-mesh", mesh.Digest)
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

func scenarioEvidenceByID(report Report, id string) (ScenarioEvidence, bool) {
	for _, scenario := range report.Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return ScenarioEvidence{}, false
}

func evidenceRef(kind, digest string) string {
	return "live-realm-v9:" + kind + ":" + digest
}
