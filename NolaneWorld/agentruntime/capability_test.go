package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fixedCapabilityEvidenceSource struct {
	snapshot CapabilityEvidenceSnapshot
	ok       bool
	err      error
}

func (f fixedCapabilityEvidenceSource) Snapshot(context.Context, CapabilityEvidenceQuery) (CapabilityEvidenceSnapshot, bool, error) {
	return f.snapshot, f.ok, f.err
}

func TestCallerCapabilityAttestationCannotManufactureVerifiedEvidence(t *testing.T) {
	svc, _, _, sess := runtimeFixture(t)
	forged := ProviderAttestation{
		GuestExecAvailable: true, GuestExecVerified: true, GuestExecEvidence: "caller:forged:guest",
		SnapshotAvailable: true, SnapshotVerified: true, SnapshotEvidence: "caller:forged:snapshot",
		PublicReadAvailable: true, PublicReadVerified: true, PublicReadEvidence: "caller:forged:public-read",
		PublicInboundDisabled: true, PublicInboundEvidence: "caller:forged:inbound",
		InternalMeshAvailable: true, InternalMeshVerified: true, InternalMeshEvidence: "caller:forged:mesh",
		FilesystemIsolationVerified: true, FilesystemIsolationEvidence: "caller:forged:filesystem",
		ProcessIsolationVerified: true, ProcessIsolationEvidence: "caller:forged:process",
		NetworkIsolationVerified: true, NetworkIsolationEvidence: "caller:forged:network",
		ResourceEnforcementAvailable: true, ResourceEnforcementVerified: true, ResourceEnforcementEvidence: "caller:forged:resource",
	}
	report, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: sess.RealmRevision, Attestation: forged})
	if err != nil {
		t.Fatal(err)
	}
	claims := []Claim{report.GuestExec, report.SnapshotRollback, report.PublicRead, report.PublicInbound, report.InternalMesh, report.FilesystemIsolation, report.ProcessIsolation, report.NetworkIsolation, report.ResourceEnforcement}
	for _, claim := range claims {
		if claim.State == Verified || claim.Evidence != "" {
			t.Fatalf("caller-controlled attestation forged trusted capability state: %+v", report)
		}
	}
	if report.GuestExec.State != AvailableUnproven || report.InternalMesh.State != AvailableUnproven || report.ResourceEnforcement.State != AvailableUnproven {
		t.Fatalf("legacy availability hints were not downgraded to unproven: %+v", report)
	}
}

func TestCapabilityReportWithoutHostEvidenceCannotBeVerified(t *testing.T) {
	svc, _, _, sess := runtimeFixture(t)
	report, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: sess.RealmRevision})
	if err != nil {
		t.Fatal(err)
	}
	claims := []Claim{report.GuestExec, report.SnapshotRollback, report.PublicRead, report.PublicInbound, report.InternalMesh, report.FilesystemIsolation, report.ProcessIsolation, report.NetworkIsolation, report.ResourceEnforcement}
	for _, claim := range claims {
		if claim.State == Verified {
			t.Fatalf("capability became verified without host evidence: %+v", report)
		}
	}
	if report.EvidenceDigest == "" {
		t.Fatal("missing deterministic report digest")
	}
}

func TestCapabilityReportCanCarryHostVerifiedEvidence(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://host-evidence"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R1PublicRead, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	policyDigest, err := realm.PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := fixedCapabilityEvidenceSource{ok: true, snapshot: CapabilityEvidenceSnapshot{
		RealmID: spec.ID, RealmRevision: 1, PolicyDigest: policyDigest,
		Attestation: ProviderAttestation{
			GuestExecAvailable: true, GuestExecVerified: true, GuestExecEvidence: "live:v5:commit",
			PublicReadAvailable: true, PublicReadVerified: true, PublicReadEvidence: "reality:read:observed",
			PublicInboundDisabled: true, PublicInboundEvidence: "network:inbound:denied",
			FilesystemIsolationVerified: true, FilesystemIsolationEvidence: "isolation:filesystem:observed",
			ProcessIsolationVerified: true, ProcessIsolationEvidence: "isolation:process:observed",
			NetworkIsolationVerified: true, NetworkIsolationEvidence: "isolation:network:observed",
		},
	}}
	ff := &fakeFabric{lease: fabric.Lease{RealmID: spec.ID, WorldID: world.ID("world-a"), Generation: 1, ExpiresUnix: time.Now().Add(time.Minute).Unix(), RealizationRevision: 1}}
	svc, err := NewWithCapabilityEvidenceSource(store, ff, &fakeGuest{}, source)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Enter(context.Background(), EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: policyDigest})
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: sess.RealmRevision})
	if err != nil {
		t.Fatal(err)
	}
	if report.GuestExec.State != Verified || report.PublicRead.State != Verified || report.PublicInbound.State != Verified {
		t.Fatalf("host evidence was not projected as verified: %+v", report)
	}
	if report.FilesystemIsolation.State != Verified || report.ProcessIsolation.State != Verified || report.NetworkIsolation.State != Verified {
		t.Fatalf("host isolation evidence was not projected as verified: %+v", report)
	}
}

func TestCapabilityEvidenceBindingMismatchFailsClosed(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://binding"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	policyDigest, err := realm.PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := fixedCapabilityEvidenceSource{ok: true, snapshot: CapabilityEvidenceSnapshot{RealmID: spec.ID, RealmRevision: 1, PolicyDigest: "wrong-policy", Attestation: ProviderAttestation{GuestExecAvailable: true, GuestExecVerified: true, GuestExecEvidence: "stale-evidence"}}}
	ff := &fakeFabric{lease: fabric.Lease{RealmID: spec.ID, WorldID: world.ID("world-a"), Generation: 1, ExpiresUnix: time.Now().Add(time.Minute).Unix(), RealizationRevision: 1}}
	svc, err := NewWithCapabilityEvidenceSource(store, ff, &fakeGuest{}, source)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Enter(context.Background(), EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: policyDigest})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: sess.RealmRevision}); !errors.Is(err, ErrCapabilityEvidenceMismatch) {
		t.Fatalf("binding mismatch err=%v", err)
	}
}
