package resourceproof_test

import (
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/agentruntime"
	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/resourceproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func TestPublicCallerCannotForgeLiveHostProvenance(t *testing.T) {
	binding := resourceproof.Binding{
		RealmID:             realm.ID("realm://forged-resource-proof"),
		RealmRevision:       11,
		PolicyDigest:        "forged-policy-digest",
		RealizationRevision: 12,
		RuntimeDigest:       "forged-runtime-digest",
	}
	cpu := resourceproof.CPUObservation{
		Source:                 resourceproof.SourceLiveHost,
		RequestedQuotaMicros:   50_000,
		RequestedPeriodMicros:  100_000,
		EffectiveQuotaMicros:   50_000,
		EffectivePeriodMicros:  100_000,
		PressureObserved:       true,
		NrThrottledBefore:      10,
		NrThrottledAfter:       11,
		ThrottledUsecBefore:    1_000,
		ThrottledUsecAfter:     1_001,
	}
	memory := resourceproof.MemoryObservation{
		Source:              resourceproof.SourceLiveHost,
		RequestedLimitBytes: 64 << 20,
		EffectiveLimitBytes: 64 << 20,
		AttemptedBytes:      96 << 20,
		OOMEventsBefore:     2,
		OOMEventsAfter:      3,
		ExitCode:            137,
		ExitReason:          "OOMKilled",
	}

	report := resourceproof.BuildReport(live.ModeRequireLive, binding, cpu, memory)
	if report.Status == live.StatusLivePass || report.Approved ||
		report.CPU.State == agentruntime.Verified || report.Memory.State == agentruntime.Verified {
		t.Fatalf("public caller forged live-host provenance into verified enforcement: %+v", report)
	}
}
