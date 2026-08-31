package capabilityproof

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func validCapabilityBinding() CapabilityEvidenceBinding {
	return CapabilityEvidenceBinding{
		RealmID:       realm.ID("realm://v10-evidence"),
		RealmRevision: 7,
		PolicyDigest:  strings.Repeat("p", 64),
	}
}

func TestEvidenceSourceRejectsUnavailableReport(t *testing.T) {
	report, err := defaultFusionRunner().Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCapabilityEvidenceSource(report, validCapabilityBinding()); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("err=%v", err)
	}
}

func TestEvidenceSourceRejectsTamperedReport(t *testing.T) {
	report := passingFusionReport(t)
	report.SubstrateProof.Digest = "tampered"
	if _, err := NewCapabilityEvidenceSource(report, validCapabilityBinding()); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("err=%v", err)
	}
}

func TestEvidenceSourceRejectsInvalidBinding(t *testing.T) {
	report := passingFusionReport(t)
	cases := []CapabilityEvidenceBinding{
		{},
		{RealmID: realm.ID("realm://v10"), RealmRevision: 0, PolicyDigest: "p"},
		{RealmID: realm.ID("realm://v10"), RealmRevision: 1, PolicyDigest: "   "},
	}
	for _, binding := range cases {
		if _, err := NewCapabilityEvidenceSource(report, binding); !errors.Is(err, ErrInvalidCapabilityEvidence) {
			t.Fatalf("binding=%+v err=%v", binding, err)
		}
	}
}

func TestEvidenceSourceReturnsNothingForBindingMismatch(t *testing.T) {
	report := passingFusionReport(t)
	binding := validCapabilityBinding()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	query := agentruntime.CapabilityEvidenceQuery{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision + 1,
		PolicyDigest:  binding.PolicyDigest,
	}
	snapshot, found, err := source.Snapshot(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if found || snapshot != (agentruntime.CapabilityEvidenceSnapshot{}) {
		t.Fatalf("mismatched binding returned evidence: found=%v snapshot=%+v", found, snapshot)
	}
}

func TestEvidenceSourceProjectsSnapshotNetworkAndGuestProof(t *testing.T) {
	report := passingFusionReport(t)
	binding := validCapabilityBinding()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	query := agentruntime.CapabilityEvidenceQuery{
		RealmID:       binding.RealmID,
		RealmRevision: binding.RealmRevision,
		PolicyDigest:  binding.PolicyDigest,
	}
	snapshot, found, err := source.Snapshot(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("exact-bound v10 evidence not found")
	}
	if snapshot.RealmID != binding.RealmID || snapshot.RealmRevision != binding.RealmRevision || snapshot.PolicyDigest != binding.PolicyDigest {
		t.Fatalf("snapshot binding=%+v", snapshot)
	}
	a := snapshot.Attestation
	if !a.GuestExecAvailable || !a.GuestExecVerified || a.GuestExecEvidence == "" {
		t.Fatalf("guest evidence=%+v", a)
	}
	if !a.SnapshotAvailable || !a.SnapshotVerified || a.SnapshotEvidence == "" {
		t.Fatalf("snapshot evidence=%+v", a)
	}
	if !a.PublicInboundDisabled || a.PublicInboundEvidence == "" {
		t.Fatalf("public inbound evidence=%+v", a)
	}
	if !a.NetworkIsolationVerified || a.NetworkIsolationEvidence == "" {
		t.Fatalf("network evidence=%+v", a)
	}
	for _, evidence := range []string{a.GuestExecEvidence, a.SnapshotEvidence, a.PublicInboundEvidence, a.NetworkIsolationEvidence} {
		if !strings.HasPrefix(evidence, "live-capability-v10:") {
			t.Fatalf("unexpected evidence reference %q", evidence)
		}
	}
}

func TestEvidenceSourceDoesNotInventUnprovedClaims(t *testing.T) {
	report := passingFusionReport(t)
	source, err := NewCapabilityEvidenceSource(report, validCapabilityBinding())
	if err != nil {
		t.Fatal(err)
	}
	binding := validCapabilityBinding()
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{
		RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest,
	})
	if err != nil || !found {
		t.Fatalf("err=%v found=%v", err, found)
	}
	a := snapshot.Attestation
	if a.PublicReadAvailable || a.PublicReadVerified || a.PublicReadEvidence != "" ||
		a.FilesystemIsolationVerified || a.FilesystemIsolationEvidence != "" ||
		a.ProcessIsolationVerified || a.ProcessIsolationEvidence != "" ||
		a.ResourceEnforcementAvailable || a.ResourceEnforcementVerified || a.ResourceEnforcementEvidence != "" {
		t.Fatalf("v10 invented an unproved claim: %+v", a)
	}
}

func TestEvidenceSourceCannotUpgradeInternalMeshWithoutV9MeshPass(t *testing.T) {
	report := passingFusionReport(t)
	if report.Capabilities.InternalMeshVerified || report.RealmProof.Capabilities.InternalMeshVerified {
		t.Fatal("fixture unexpectedly verifies mesh")
	}
	binding := validCapabilityBinding()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{
		RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest,
	})
	if err != nil || !found {
		t.Fatalf("err=%v found=%v", err, found)
	}
	if snapshot.Attestation.InternalMeshAvailable || snapshot.Attestation.InternalMeshVerified || snapshot.Attestation.InternalMeshEvidence != "" {
		t.Fatalf("mesh was upgraded without proof: %+v", snapshot.Attestation)
	}
}
