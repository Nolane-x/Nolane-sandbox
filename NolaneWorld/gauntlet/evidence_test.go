package gauntlet

import (
	"context"
	"errors"
	"testing"
	"time"
)

func validReport(t *testing.T) Report {
	t.Helper()
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	report, err := r.Run(context.Background(), []Scenario{passingScenario("alpha")})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestVerifyReportRejectsTrustBearingMutation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Report)
	}{
		{"approved", func(r *Report) { r.Approved = !r.Approved }},
		{"event", func(r *Report) { r.Scenarios[0].Events[0].Detail = "mutated" }},
		{"digest", func(r *Report) { r.Scenarios[0].EvidenceDigest = "00" }},
		{"scenario id", func(r *Report) { r.Scenarios[0].ID = "other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := validReport(t)
			tc.mutate(&report)
			if err := VerifyReport(report); err == nil {
				t.Fatal("mutated report verified")
			}
		})
	}
}

func TestMarshalReportOnlyEmitsVerifiedEvidence(t *testing.T) {
	report := validReport(t)
	raw, err := MarshalReport(report)
	if err != nil || len(raw) == 0 {
		t.Fatalf("marshal: %v", err)
	}
	report.ReportDigest = "bad"
	if _, err := MarshalReport(report); err == nil {
		t.Fatal("invalid report marshaled")
	}
}

func TestVerifyReportRechecksRequiredMarkersFromEvidence(t *testing.T) {
	report := validReport(t)
	report.Scenarios[0].RequiredMarkers = append(report.Scenarios[0].RequiredMarkers, "not-observed")
	report.Scenarios[0].EvidenceDigest = scenarioDigest(report.Scenarios[0])
	report.ReportDigest = reportDigest(report)
	if err := VerifyReport(report); err == nil {
		t.Fatal("report verified after adding an unobserved required marker")
	}
}

func TestVerifyReportRecomputesPolicyDigest(t *testing.T) {
	report := validReport(t)
	report.Policy.ScenarioTimeout++
	report.ReportDigest = reportDigest(report)
	if err := VerifyReport(report); err == nil {
		t.Fatal("report verified with policy that no longer matches policy digest")
	}
}

func TestVerifyReportRejectsUnknownFailureCode(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	bad := ScenarioFunc{
		Definition: ScenarioSpec{ID: "failure-code", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh},
		Execute:    func(context.Context, *Probe) error { return errors.New("boom") },
	}
	report, err := r.Run(context.Background(), []Scenario{bad})
	if err != nil {
		t.Fatal(err)
	}
	report.Scenarios[0].FailureCode = FailureCode("invented")
	report.Scenarios[0].EvidenceDigest = scenarioDigest(report.Scenarios[0])
	report.ReportDigest = reportDigest(report)
	if err := VerifyReport(report); err == nil {
		t.Fatal("report verified with unknown failure code")
	}
}
