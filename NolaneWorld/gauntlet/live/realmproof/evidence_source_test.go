package realmproof

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func evidenceBindingFixture() CapabilityEvidenceBinding {
	return CapabilityEvidenceBinding{
		RealmID:       realm.ID("realm://live-proof-v9"),
		RealmRevision: 7,
		PolicyDigest:  strings.Repeat("c", 64),
	}
}

func sealedApprovedReport(t *testing.T) Report {
	t.Helper()
	report := approvedReportFixture()
	if err := SealReport(&report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestCapabilityEvidenceSourceMapsOnlyObservedRealmClaims(t *testing.T) {
	report := sealedApprovedReport(t)
	binding := evidenceBindingFixture()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{
		RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest,
	})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	att := snapshot.Attestation
	if !att.GuestExecAvailable || !att.GuestExecVerified || att.GuestExecEvidence == "" {
		t.Fatalf("guest evidence=%+v", att)
	}
	if !att.PublicInboundDisabled || att.PublicInboundEvidence == "" {
		t.Fatalf("public ingress evidence=%+v", att)
	}
	if !att.NetworkIsolationVerified || att.NetworkIsolationEvidence == "" {
		t.Fatalf("network evidence=%+v", att)
	}
	if att.InternalMeshAvailable || att.InternalMeshVerified || att.InternalMeshEvidence != "" {
		t.Fatalf("unavailable mesh was upgraded: %+v", att)
	}
	if att.PublicReadAvailable || att.PublicReadVerified || att.PublicReadEvidence != "" || att.SnapshotAvailable || att.SnapshotVerified || att.FilesystemIsolationVerified || att.ProcessIsolationVerified || att.ResourceEnforcementAvailable || att.ResourceEnforcementVerified {
		t.Fatalf("realm proof inferred unrelated capability: %+v", att)
	}
}

func TestCapabilityEvidenceSourceRefusesCrossBindingReuse(t *testing.T) {
	report := sealedApprovedReport(t)
	binding := evidenceBindingFixture()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	queries := []agentruntime.CapabilityEvidenceQuery{
		{RealmID: realm.ID("realm://other"), RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest},
		{RealmID: binding.RealmID, RealmRevision: binding.RealmRevision + 1, PolicyDigest: binding.PolicyDigest},
		{RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: strings.Repeat("d", 64)},
	}
	for _, query := range queries {
		if snapshot, found, err := source.Snapshot(context.Background(), query); err != nil || found || snapshot.RealmID != "" {
			t.Fatalf("cross-binding query=%+v found=%v err=%v snapshot=%+v", query, found, err, snapshot)
		}
	}
}

func TestCapabilityEvidenceSourceRejectsTamperedOrUnapprovedReport(t *testing.T) {
	binding := evidenceBindingFixture()
	tampered := sealedApprovedReport(t)
	tampered.Capabilities.RawPublicDenied = false
	if _, err := NewCapabilityEvidenceSource(tampered, binding); err == nil {
		t.Fatal("tampered live report became capability evidence")
	}
	unapproved := approvedReportFixture()
	unapproved.Status = live.StatusUnavailable
	unapproved.Reason = ReasonConfigMissing
	unapproved.Approved = false
	unapproved.Capabilities = Capabilities{}
	unapproved.Scenarios = []ScenarioEvidence{}
	if err := SealReport(&unapproved); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCapabilityEvidenceSource(unapproved, binding); err == nil {
		t.Fatal("UNAVAILABLE report became trusted capability evidence")
	}
}

func TestCapabilityEvidenceSourceMapsMeshOnlyFromGenuineMeshPass(t *testing.T) {
	report := approvedReportFixture()
	report.Capabilities.InternalMeshVerified = true
	for i := range report.Scenarios {
		if report.Scenarios[i].ID == ScenarioInternalMesh {
			report.Scenarios[i] = ScenarioEvidence{ID: ScenarioInternalMesh, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"private-mesh-observed"}}
		}
	}
	if err := SealReport(&report); err != nil {
		t.Fatal(err)
	}
	binding := evidenceBindingFixture()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest})
	if err != nil || !found || !snapshot.Attestation.InternalMeshAvailable || !snapshot.Attestation.InternalMeshVerified || snapshot.Attestation.InternalMeshEvidence == "" {
		t.Fatalf("found=%v err=%v snapshot=%+v", found, err, snapshot)
	}
}

func TestCapabilityEvidenceSourceIsImmutableUnderConcurrentReads(t *testing.T) {
	report := sealedApprovedReport(t)
	binding := evidenceBindingFixture()
	source, err := NewCapabilityEvidenceSource(report, binding)
	if err != nil {
		t.Fatal(err)
	}
	query := agentruntime.CapabilityEvidenceQuery{RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest}
	var wg sync.WaitGroup
	errs := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot, found, err := source.Snapshot(context.Background(), query)
			if err != nil || !found || !snapshot.Attestation.GuestExecVerified || snapshot.PolicyDigest != binding.PolicyDigest {
				errs <- "unstable concurrent snapshot"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for message := range errs {
		t.Fatal(message)
	}
}
