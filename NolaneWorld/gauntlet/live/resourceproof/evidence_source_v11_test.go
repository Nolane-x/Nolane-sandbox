package resourceproof

import (
	"context"
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

func passingResourceReport() TrustedReport {
	return buildTrustedReport(live.ModeRequireLive, validBinding(), validCPUObservation(), validMemoryObservation())
}

func exactCapabilityBinding() CapabilityEvidenceBinding {
	b := validBinding()
	return CapabilityEvidenceBinding{
		RealmID:             b.RealmID,
		RealmRevision:       b.RealmRevision,
		PolicyDigest:        b.PolicyDigest,
		RealizationRevision: b.RealizationRevision,
		RuntimeDigest:       b.RuntimeDigest,
	}
}

func TestResourceEvidenceSourceProjectsOnlyProvedDimensions(t *testing.T) {
	trusted := passingResourceReport()
	source, err := NewCapabilityEvidenceSource(trusted, exactCapabilityBinding())
	if err != nil {
		t.Fatal(err)
	}
	binding := exactCapabilityBinding()
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{
		RealmID: binding.RealmID, RealmRevision: binding.RealmRevision, PolicyDigest: binding.PolicyDigest,
	})
	if err != nil || !found {
		t.Fatalf("err=%v found=%v", err, found)
	}
	a := snapshot.Attestation
	if !a.CPUEnforcementAvailable || !a.CPUEnforcementVerified || a.CPUEnforcementEvidence == "" {
		t.Fatalf("CPU proof not projected: %+v", a)
	}
	if !a.MemoryEnforcementAvailable || !a.MemoryEnforcementVerified || a.MemoryEnforcementEvidence == "" {
		t.Fatalf("memory proof not projected: %+v", a)
	}
	if a.DiskEnforcementAvailable || a.DiskEnforcementVerified || a.DiskEnforcementEvidence != "" {
		t.Fatalf("disk proof invented: %+v", a)
	}
	if !a.ResourceEnforcementAvailable || a.ResourceEnforcementVerified || a.ResourceEnforcementEvidence != "" {
		t.Fatalf("aggregate proof overclaimed: %+v", a)
	}
}

func TestResourceEvidenceSourceRejectsStaleRealizationOrRuntimeBinding(t *testing.T) {
	trusted := passingResourceReport()
	binding := exactCapabilityBinding()
	binding.RealizationRevision++
	if _, err := NewCapabilityEvidenceSource(trusted, binding); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("stale realization binding err=%v", err)
	}
	binding = exactCapabilityBinding()
	binding.RuntimeDigest = "different-runtime"
	if _, err := NewCapabilityEvidenceSource(trusted, binding); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("stale runtime binding err=%v", err)
	}
}

func TestResourceEvidenceSourceRejectsUnavailableAndTamperedTrustedReports(t *testing.T) {
	cpu := validCPUObservation()
	cpu.Source = SourceFixture
	memory := validMemoryObservation()
	memory.Source = SourceFixture
	unavailable := buildTrustedReport(live.ModeRequireLive, validBinding(), cpu, memory)
	if _, err := NewCapabilityEvidenceSource(unavailable, exactCapabilityBinding()); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("unavailable report err=%v", err)
	}

	trusted := passingResourceReport()
	tampered := trusted.Report()
	tampered.Memory.Observation.OOMEventsAfter++
	forgedWrapper := TrustedReport{report: tampered}
	if _, err := NewCapabilityEvidenceSource(forgedWrapper, exactCapabilityBinding()); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("tampered trusted report err=%v", err)
	}

	if _, err := NewCapabilityEvidenceSource(TrustedReport{}, exactCapabilityBinding()); !errors.Is(err, ErrInvalidCapabilityEvidence) {
		t.Fatalf("zero trusted report err=%v", err)
	}
}

func TestResourceEvidenceSourceReturnsNothingForRealmPolicyMismatch(t *testing.T) {
	trusted := passingResourceReport()
	binding := exactCapabilityBinding()
	source, err := NewCapabilityEvidenceSource(trusted, binding)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := source.Snapshot(context.Background(), agentruntime.CapabilityEvidenceQuery{
		RealmID: binding.RealmID, RealmRevision: binding.RealmRevision + 1, PolicyDigest: binding.PolicyDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found || snapshot != (agentruntime.CapabilityEvidenceSnapshot{}) {
		t.Fatalf("stale Realm query returned resource evidence: found=%v snapshot=%+v", found, snapshot)
	}
}
