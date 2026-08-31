package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestTypedResourceEvidenceDoesNotOverclaimAggregateEnforcement(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{
		ID:             realm.ID("realm://resource-v11"),
		MaxWorlds:      2,
		DefaultLease:   time.Minute,
		NetworkProfile: realm.R0InternalOnly,
		ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 512, DiskMiB: 1024},
	}
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
			CPUEnforcementAvailable:    true,
			CPUEnforcementVerified:     true,
			CPUEnforcementEvidence:     "live-resource-v11:cpu:proof",
			MemoryEnforcementAvailable: true,
			MemoryEnforcementVerified:  true,
			MemoryEnforcementEvidence:  "live-resource-v11:memory:proof",
		},
	}}
	ff := &fakeFabric{lease: fabric.Lease{
		RealmID: spec.ID, WorldID: world.ID("world-resource-v11"), Generation: 1,
		ExpiresUnix: time.Now().Add(time.Minute).Unix(), RealizationRevision: 1,
	}}
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
	if report.CPUEnforcement.State != Verified || report.CPUEnforcement.Evidence == "" {
		t.Fatalf("CPU enforcement must be independently verified from host proof: %+v", report)
	}
	if report.MemoryEnforcement.State != Verified || report.MemoryEnforcement.Evidence == "" {
		t.Fatalf("memory enforcement must be independently verified from host proof: %+v", report)
	}
	if report.DiskEnforcement.State != Unavailable || report.DiskEnforcement.Evidence != "" {
		t.Fatalf("disk enforcement must remain unavailable without direct proof: %+v", report)
	}
	if report.ResourceEnforcement.State == Verified {
		t.Fatalf("aggregate resource enforcement overclaimed without disk proof: %+v", report)
	}
	if report.ResourceEnforcement.State != AvailableUnproven {
		t.Fatalf("aggregate resource enforcement should expose partial availability as unproven: %+v", report)
	}
}

func TestCallerCannotForgeTypedResourceVerification(t *testing.T) {
	svc, _, _, sess := runtimeFixture(t)
	forged := ProviderAttestation{
		CPUEnforcementAvailable:    true,
		CPUEnforcementVerified:     true,
		CPUEnforcementEvidence:     "caller:forged:cpu",
		MemoryEnforcementAvailable: true,
		MemoryEnforcementVerified:  true,
		MemoryEnforcementEvidence:  "caller:forged:memory",
		DiskEnforcementAvailable:   true,
		DiskEnforcementVerified:    true,
		DiskEnforcementEvidence:    "caller:forged:disk",
		ResourceEnforcementAvailable: true,
		ResourceEnforcementVerified:  true,
		ResourceEnforcementEvidence:  "caller:forged:aggregate",
	}
	report, err := svc.Capabilities(context.Background(), CapabilityRequest{
		SessionID: sess.ID, RealmRevision: sess.RealmRevision, Attestation: forged,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, got := range map[string]Claim{
		"cpu":       report.CPUEnforcement,
		"memory":    report.MemoryEnforcement,
		"disk":      report.DiskEnforcement,
		"aggregate": report.ResourceEnforcement,
	} {
		if got.State == Verified || got.Evidence != "" {
			t.Fatalf("caller forged trusted %s resource state: %+v", name, report)
		}
	}
	if report.CPUEnforcement.State != AvailableUnproven ||
		report.MemoryEnforcement.State != AvailableUnproven ||
		report.DiskEnforcement.State != AvailableUnproven ||
		report.ResourceEnforcement.State != AvailableUnproven {
		t.Fatalf("caller availability hints must remain unproven: %+v", report)
	}
}
