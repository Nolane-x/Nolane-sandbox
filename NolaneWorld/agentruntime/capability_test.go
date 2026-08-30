package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestCapabilityReportDoesNotUpgradeRequestsIntoProof(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://test"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R1PublicRead, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	ff := &fakeFabric{lease: fabric.Lease{RealmID: spec.ID, WorldID: world.ID("world-a"), Generation: 1, ExpiresUnix: time.Now().Add(time.Minute).Unix(), RealizationRevision: 1}}
	fg := &fakeGuest{}
	svc, err := New(store, ff, fg)
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := realm.PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Enter(context.Background(), EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: policyDigest})
	if err != nil {
		t.Fatal(err)
	}

	report, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: 1, Attestation: ProviderAttestation{GuestExecAvailable: true, SnapshotAvailable: true, PublicInboundDisabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if report.GuestExec.State != AvailableUnproven {
		t.Fatalf("guest exec=%+v", report.GuestExec)
	}
	if report.SnapshotRollback.State != AvailableUnproven {
		t.Fatalf("snapshot=%+v", report.SnapshotRollback)
	}
	if report.PublicRead.State == Verified {
		t.Fatalf("public read falsely verified: %+v", report.PublicRead)
	}
	if report.InternalMesh.State == Verified || report.ResourceEnforcement.State == Verified {
		t.Fatalf("unproven claims: mesh=%+v resource=%+v", report.InternalMesh, report.ResourceEnforcement)
	}
	if report.PublicInbound.State != AvailableUnproven || report.PublicInbound.Evidence != "" {
		t.Fatalf("public inbound denial was upgraded without evidence: %+v", report.PublicInbound)
	}
}

func TestCapabilityReportCanCarryExplicitVerifiedEvidence(t *testing.T) {
	svc, _, _, sess := runtimeFixture(t)
	att := ProviderAttestation{
		GuestExecAvailable: true, GuestExecVerified: true, GuestExecEvidence: "live:v5:commit",
		PublicInboundDisabled: true, PublicInboundEvidence: "policy:observed",
		FilesystemIsolationVerified: true, FilesystemIsolationEvidence: "isolation:filesystem:observed",
		ProcessIsolationVerified: true, ProcessIsolationEvidence: "isolation:process:observed",
		NetworkIsolationVerified: true, NetworkIsolationEvidence: "isolation:network:observed",
	}
	report, err := svc.Capabilities(context.Background(), CapabilityRequest{SessionID: sess.ID, RealmRevision: sess.RealmRevision, Attestation: att})
	if err != nil {
		t.Fatal(err)
	}
	if report.GuestExec.State != Verified || report.GuestExec.Evidence == "" {
		t.Fatalf("guest=%+v", report.GuestExec)
	}
	if report.FilesystemIsolation.State != Verified || report.ProcessIsolation.State != Verified || report.NetworkIsolation.State != Verified {
		t.Fatalf("isolation claims not verified: %+v", report)
	}
	if report.EvidenceDigest == "" {
		t.Fatal("missing deterministic report digest")
	}
}
