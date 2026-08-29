package delegationgauntlet

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func TestStandardSuiteIsMandatoryApprovedAndSecretFree(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 3 * time.Second}
	report, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatalf("report rejected: %+v", report)
	}
	if len(report.Scenarios) != 15 {
		t.Fatalf("scenario count=%d", len(report.Scenarios))
	}
	for _, scenario := range report.Scenarios {
		if scenario.Outcome != gauntlet.OutcomePass {
			t.Fatalf("scenario %s outcome=%s", scenario.ID, scenario.Outcome)
		}
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(SyntheticSecret)) {
		t.Fatal("synthetic credential leaked into gauntlet report")
	}
}

func TestStandardSuiteIsByteDeterministic(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 3 * time.Second}
	a, err := RunStandard(context.Background(), policy)
	if err != nil { t.Fatal(err) }
	b, err := RunStandard(context.Background(), policy)
	if err != nil { t.Fatal(err) }
	ra, err := gauntlet.MarshalReport(a); if err != nil { t.Fatal(err) }
	rb, err := gauntlet.MarshalReport(b); if err != nil { t.Fatal(err) }
	if !bytes.Equal(ra, rb) {
		t.Fatal("v6 authority evidence is not byte deterministic")
	}
}
