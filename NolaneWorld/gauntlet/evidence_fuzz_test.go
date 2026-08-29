package gauntlet

import (
	"context"
	"testing"
	"time"
)

func FuzzVerifyReportRejectsMutatedEvent(f *testing.F) {
	f.Add("mutated")
	f.Add("x")
	f.Fuzz(func(t *testing.T, mutation string) {
		if mutation == "" || mutation == "attack attempted" {
			t.Skip()
		}
		r := NewRunner(Policy{ProductID: ProductNolaneSandbox, ScenarioTimeout: time.Second})
		report, err := r.Run(context.Background(), []Scenario{passingScenario("fuzz")})
		if err != nil {
			t.Fatal(err)
		}
		report.Scenarios[0].Events[0].Detail = mutation
		if err := VerifyReport(report); err == nil {
			t.Fatalf("mutated event verified: %q", mutation)
		}
	})
}
