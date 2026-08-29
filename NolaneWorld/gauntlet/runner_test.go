package gauntlet

import (
	"context"
	"errors"
	"testing"
	"time"
)

func passingScenario(id string) Scenario {
	return ScenarioFunc{
		Definition: ScenarioSpec{ID: id, Invariant: "invariant", Attack: "attack", ExpectedDefense: "deny", Severity: SeverityHigh, RequiredMarkers: []string{"attack", "boundary", "denial"}},
		Execute: func(_ context.Context, p *Probe) error {
			_ = p.Record(EventAttack, "attack", "attack attempted")
			_ = p.Record(EventBoundary, "boundary", "boundary exercised")
			_ = p.Record(EventDenial, "denial", "attack denied")
			return nil
		},
	}
}

func TestRunnerRejectsVacuousScenario(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	s := ScenarioFunc{Definition: ScenarioSpec{ID: "vacuous", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh, RequiredMarkers: []string{"attack"}}, Execute: func(context.Context, *Probe) error { return nil }}
	report, err := r.Run(context.Background(), []Scenario{s})
	if err != nil {
		t.Fatal(err)
	}
	if report.Approved || report.Scenarios[0].Outcome != OutcomeFail || report.Scenarios[0].FailureCode != FailureProofMissing {
		t.Fatalf("vacuous scenario passed: %+v", report.Scenarios[0])
	}
}

func TestRunnerRejectsMissingRequiredMarker(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	s := ScenarioFunc{Definition: ScenarioSpec{ID: "missing", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh, RequiredMarkers: []string{"required"}}, Execute: func(_ context.Context, p *Probe) error {
		_ = p.Record(EventAttack, "attack", "x")
		_ = p.Record(EventBoundary, "boundary", "x")
		_ = p.Record(EventDenial, "denial", "x")
		return nil
	}}
	report, err := r.Run(context.Background(), []Scenario{s})
	if err != nil {
		t.Fatal(err)
	}
	if report.Approved || report.Scenarios[0].FailureCode != FailureMarkerMissing {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerConvertsPanicAndTimeoutToFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		exec func(context.Context, *Probe) error
		want FailureCode
	}{
		{"panic", func(context.Context, *Probe) error { panic("boom") }, FailurePanic},
		{"timeout", func(ctx context.Context, _ *Probe) error { <-ctx.Done(); return ctx.Err() }, FailureTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: 10 * time.Millisecond})
			s := ScenarioFunc{Definition: ScenarioSpec{ID: tc.name, Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityHigh}, Execute: tc.exec}
			report, err := r.Run(context.Background(), []Scenario{s})
			if err != nil {
				t.Fatal(err)
			}
			if report.Approved || report.Scenarios[0].FailureCode != tc.want {
				t.Fatalf("scenario=%+v", report.Scenarios[0])
			}
		})
	}
}

func TestRunnerRejectsDuplicateScenarioIDs(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	_, err := r.Run(context.Background(), []Scenario{passingScenario("dup"), passingScenario("dup")})
	if !errors.Is(err, ErrDuplicateScenario) {
		t.Fatalf("got %v", err)
	}
}

func TestRunnerOneFailureRejectsWholeRelease(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	bad := ScenarioFunc{Definition: ScenarioSpec{ID: "bad", Invariant: "i", Attack: "a", ExpectedDefense: "d", Severity: SeverityCritical}, Execute: func(context.Context, *Probe) error { return errors.New("defense failed") }}
	report, err := r.Run(context.Background(), []Scenario{passingScenario("good"), bad})
	if err != nil {
		t.Fatal(err)
	}
	if report.Approved {
		t.Fatal("failed scenario did not veto release")
	}
}

func TestRunnerRegistrationOrderDoesNotChangeDigest(t *testing.T) {
	r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
	a, err := r.Run(context.Background(), []Scenario{passingScenario("b"), passingScenario("a")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Run(context.Background(), []Scenario{passingScenario("a"), passingScenario("b")})
	if err != nil {
		t.Fatal(err)
	}
	if a.ReportDigest == "" || a.ReportDigest != b.ReportDigest {
		t.Fatalf("digest differs %q %q", a.ReportDigest, b.ReportDigest)
	}
}
