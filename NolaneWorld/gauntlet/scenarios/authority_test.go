package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func runApproved(t *testing.T, s gauntlet.Scenario) gauntlet.Report {
	t.Helper()
	r := gauntlet.NewRunner(gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: time.Second})
	report, err := r.Run(context.Background(), []gauntlet.Scenario{s})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatalf("scenario failed: %+v", report.Scenarios[0])
	}
	if err := gauntlet.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestAuthorityScenariosSurviveAttacks(t *testing.T) {
	for _, scenario := range []gauntlet.Scenario{StaleEpochScenario(), TerminalAuthorityScenario(), ActionCollisionScenario()} {
		runApproved(t, scenario)
	}
}
