package resourceproof

import (
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func validBinding() Binding {
	return Binding{
		RealmID:             realm.ID("realm://resource-proof"),
		RealmRevision:       3,
		PolicyDigest:        "policy-digest-v11",
		RealizationRevision: 7,
		RuntimeDigest:       "runtime-digest-v11",
	}
}

func validCPUObservation() CPUObservation {
	return CPUObservation{
		Source:                 SourceLiveHost,
		RequestedQuotaMicros:   50_000,
		RequestedPeriodMicros:  100_000,
		EffectiveQuotaMicros:   50_000,
		EffectivePeriodMicros:  100_000,
		PressureObserved:       true,
		NrThrottledBefore:      10,
		NrThrottledAfter:       14,
		ThrottledUsecBefore:    1_000,
		ThrottledUsecAfter:     1_750,
	}
}

func validMemoryObservation() MemoryObservation {
	return MemoryObservation{
		Source:              SourceLiveHost,
		RequestedLimitBytes: 64 << 20,
		EffectiveLimitBytes: 64 << 20,
		AttemptedBytes:      96 << 20,
		OOMEventsBefore:     2,
		OOMEventsAfter:      3,
		ExitCode:            137,
		ExitReason:          "OOMKilled",
	}
}

func TestTrustedReportVerifiesCPUAndMemoryWithoutOverclaimingDisk(t *testing.T) {
	trusted := buildTrustedReport(live.ModeRequireLive, validBinding(), validCPUObservation(), validMemoryObservation())
	report := trusted.Report()
	if report.Status != live.StatusLivePass || !report.Approved {
		t.Fatalf("causal live resource proof did not pass: %+v", report)
	}
	if report.CPU.State != agentruntime.Verified || report.CPU.Evidence == "" {
		t.Fatalf("CPU proof not verified: %+v", report.CPU)
	}
	if report.Memory.State != agentruntime.Verified || report.Memory.Evidence == "" {
		t.Fatalf("memory proof not verified: %+v", report.Memory)
	}
	if report.Disk.State != agentruntime.Unavailable || report.Disk.Evidence != "" {
		t.Fatalf("disk must remain unavailable without direct proof: %+v", report.Disk)
	}
	if report.Digest == "" {
		t.Fatal("missing canonical report digest")
	}
	if err := VerifyTrustedReport(trusted); err != nil {
		t.Fatalf("valid trusted report rejected: %v", err)
	}
	if err := VerifyReport(report); err == nil {
		t.Fatal("plain report document retained trusted provenance authority")
	}
}

func TestVoluntaryExit137CannotManufactureMemoryProof(t *testing.T) {
	memory := validMemoryObservation()
	memory.OOMEventsAfter = memory.OOMEventsBefore
	report := buildTrustedReport(live.ModeRequireLive, validBinding(), validCPUObservation(), memory).Report()
	if report.Status == live.StatusLivePass || report.Approved || report.Memory.State == agentruntime.Verified {
		t.Fatalf("exit 137 without authoritative OOM delta manufactured proof: %+v", report)
	}
}

func TestCPUPressureWithoutThrottleDeltaCannotManufactureProof(t *testing.T) {
	cpu := validCPUObservation()
	cpu.NrThrottledAfter = cpu.NrThrottledBefore
	cpu.ThrottledUsecAfter = cpu.ThrottledUsecBefore
	report := buildTrustedReport(live.ModeRequireLive, validBinding(), cpu, validMemoryObservation()).Report()
	if report.Status == live.StatusLivePass || report.Approved || report.CPU.State == agentruntime.Verified {
		t.Fatalf("CPU pressure without authoritative throttle delta manufactured proof: %+v", report)
	}
}

func TestEffectiveLimitMismatchFailsClosed(t *testing.T) {
	cpu := validCPUObservation()
	cpu.EffectiveQuotaMicros++
	report := buildTrustedReport(live.ModeRequireLive, validBinding(), cpu, validMemoryObservation()).Report()
	if report.Status != live.StatusLiveFail || report.Approved {
		t.Fatalf("CPU readback mismatch did not fail closed: %+v", report)
	}

	memory := validMemoryObservation()
	memory.EffectiveLimitBytes++
	report = buildTrustedReport(live.ModeRequireLive, validBinding(), validCPUObservation(), memory).Report()
	if report.Status != live.StatusLiveFail || report.Approved {
		t.Fatalf("memory readback mismatch did not fail closed: %+v", report)
	}
}

func TestFixtureEvidenceIsUnavailableNotPass(t *testing.T) {
	cpu := validCPUObservation()
	cpu.Source = SourceFixture
	memory := validMemoryObservation()
	memory.Source = SourceFixture
	report := buildTrustedReport(live.ModeRequireLive, validBinding(), cpu, memory).Report()
	if report.Status != live.StatusUnavailable || report.Approved {
		t.Fatalf("fixture evidence must remain UNAVAILABLE, got %+v", report)
	}
	if report.CPU.State == agentruntime.Verified || report.Memory.State == agentruntime.Verified {
		t.Fatalf("fixture evidence elevated a live capability: %+v", report)
	}
}

func TestTamperedTrustedResourceReportDigestIsRejected(t *testing.T) {
	trusted := buildTrustedReport(live.ModeRequireLive, validBinding(), validCPUObservation(), validMemoryObservation())
	if err := VerifyTrustedReport(trusted); err != nil {
		t.Fatal(err)
	}
	tampered := trusted.Report()
	tampered.CPU.Observation.NrThrottledAfter++
	if err := verifyTrustedDocument(tampered); err == nil {
		t.Fatal("tampered trusted report document was accepted")
	}
}

func TestPublicBuildReportTreatsLiveHostLabelAsUntrustedMetadata(t *testing.T) {
	report := BuildReport(live.ModeRequireLive, validBinding(), validCPUObservation(), validMemoryObservation())
	if report.Status != live.StatusUnavailable || report.Approved {
		t.Fatalf("public report builder elevated a copyable provenance label: %+v", report)
	}
	if report.CPU.State == agentruntime.Verified || report.Memory.State == agentruntime.Verified {
		t.Fatalf("public report builder minted verified enforcement: %+v", report)
	}
	if err := VerifyReport(report); err != nil {
		t.Fatalf("canonical untrusted report was rejected: %v", err)
	}
}
