package resourceproof

import (
	"errors"
	"reflect"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

func TestV22TrustedCPUAndMemoryCanPassIndependently(t *testing.T) {
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())

	for _, dimension := range []ResourceDimension{ResourceCPU, ResourceMemory} {
		verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{dimension}})
		if err != nil {
			t.Fatalf("%s projection returned error: %v", dimension, err)
		}
		if verdict.Status != live.StatusLivePass || !verdict.Approved {
			t.Fatalf("%s trusted dimension did not LIVE_PASS: %+v", dimension, verdict)
		}
		if len(verdict.Results) != 1 || verdict.Results[0].Dimension != dimension || verdict.Results[0].State != DimensionVerified || verdict.Results[0].Evidence == "" {
			t.Fatalf("%s verdict lacks verified evidence: %+v", dimension, verdict)
		}
	}
}

func TestV22DimensionLocalAuthorityDoesNotRequireUnrequestedDimension(t *testing.T) {
	badMemory := validMemoryObservation()
	badMemory.OOMEventsAfter = badMemory.OOMEventsBefore
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), badMemory)
	if trusted.Report().Status != live.StatusLiveFail {
		t.Fatal("fixture must contain a trusted memory contradiction")
	}

	cpu, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceCPU}})
	if err != nil || cpu.Status != live.StatusLivePass || !cpu.Approved || len(cpu.Results) != 1 || cpu.Results[0].State != DimensionVerified {
		t.Fatalf("valid trusted CPU dimension was poisoned by unrequested memory contradiction: verdict=%+v err=%v", cpu, err)
	}

	badCPU := validCPUObservation()
	badCPU.NrThrottledAfter = badCPU.NrThrottledBefore
	badCPU.ThrottledUsecAfter = badCPU.ThrottledUsecBefore
	trusted = buildTrustedReport(live.ModeProbe, validBinding(), badCPU, validMemoryObservation())
	memory, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceMemory}})
	if err != nil || memory.Status != live.StatusLivePass || !memory.Approved || len(memory.Results) != 1 || memory.Results[0].State != DimensionVerified {
		t.Fatalf("valid trusted memory dimension was poisoned by unrequested CPU contradiction: verdict=%+v err=%v", memory, err)
	}
}

func TestV22RequestedContradictionFailsClosed(t *testing.T) {
	badCPU := validCPUObservation()
	badCPU.NrThrottledAfter = badCPU.NrThrottledBefore
	badCPU.ThrottledUsecAfter = badCPU.ThrottledUsecBefore
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), badCPU, validMemoryObservation())

	verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceCPU, ResourceMemory}})
	if !errors.Is(err, live.ErrLiveFailed) {
		t.Fatalf("trusted contradiction must return ErrLiveFailed, got %v", err)
	}
	if verdict.Status != live.StatusLiveFail || verdict.Approved || verdict.Reason != VerdictReasonContradicted {
		t.Fatalf("trusted contradiction did not fail closed: %+v", verdict)
	}
	if len(verdict.Results) != 2 || verdict.Results[0].Dimension != ResourceCPU || verdict.Results[0].State != DimensionContradicted {
		t.Fatalf("CPU contradiction not preserved: %+v", verdict.Results)
	}
}

func TestV22DiskAndAggregateStayUnavailable(t *testing.T) {
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())

	for _, dimensions := range [][]ResourceDimension{
		{ResourceDisk},
		{ResourceCPU, ResourceMemory, ResourceDisk},
	} {
		verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: dimensions})
		if err != nil {
			t.Fatalf("probe unavailable must remain an ordinary result: %v", err)
		}
		if verdict.Status != live.StatusUnavailable || verdict.Approved {
			t.Fatalf("disk-containing request overclaimed LIVE_PASS: %+v", verdict)
		}
		foundDisk := false
		for _, result := range verdict.Results {
			if result.Dimension == ResourceDisk {
				foundDisk = true
				if result.State != DimensionUnavailable || result.Evidence != "" {
					t.Fatalf("disk must remain unavailable: %+v", result)
				}
			}
		}
		if !foundDisk {
			t.Fatalf("disk result missing from %+v", verdict)
		}
	}

	verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeRequireLive, Dimensions: []ResourceDimension{ResourceDisk}})
	if !errors.Is(err, live.ErrLiveUnavailable) || verdict.Status != live.StatusUnavailable || verdict.Approved {
		t.Fatalf("require-live disk request did not fail unavailable: verdict=%+v err=%v", verdict, err)
	}
}

func TestV22ExactExpectedBindingAndCanonicalDimensionOrder(t *testing.T) {
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())
	expected := validBinding()
	verdict, err := trusted.ProjectVerdict(VerdictRequest{
		Mode:            live.ModeProbe,
		Dimensions:      []ResourceDimension{ResourceMemory, ResourceCPU},
		ExpectedBinding: &expected,
	})
	if err != nil || verdict.Status != live.StatusLivePass || !verdict.Approved {
		t.Fatalf("exact binding did not pass: verdict=%+v err=%v", verdict, err)
	}
	if !reflect.DeepEqual(verdict.Requested, []ResourceDimension{ResourceCPU, ResourceMemory}) {
		t.Fatalf("dimensions are not canonical: %+v", verdict.Requested)
	}
	if verdict.Binding != expected {
		t.Fatalf("verdict did not preserve exact binding: got %+v want %+v", verdict.Binding, expected)
	}

	mismatch := expected
	mismatch.RealizationRevision++
	verdict, err = trusted.ProjectVerdict(VerdictRequest{
		Mode:            live.ModeProbe,
		Dimensions:      []ResourceDimension{ResourceCPU},
		ExpectedBinding: &mismatch,
	})
	if err != nil {
		t.Fatalf("probe binding mismatch should be unavailable, got %v", err)
	}
	if verdict.Status != live.StatusUnavailable || verdict.Approved || verdict.Reason != VerdictReasonBindingMismatch {
		t.Fatalf("binding mismatch did not become UNAVAILABLE: %+v", verdict)
	}
}

func TestV22ZeroTrustedAuthorityCannotPass(t *testing.T) {
	verdict, err := (TrustedReport{}).ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceCPU}})
	if err != nil {
		t.Fatalf("probe unavailable should not fabricate a hard failure: %v", err)
	}
	if verdict.Status != live.StatusUnavailable || verdict.Approved {
		t.Fatalf("zero trusted authority manufactured a pass: %+v", verdict)
	}
}
