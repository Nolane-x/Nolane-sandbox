package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func TestStorageBoundaryScenariosSurviveAttacks(t *testing.T) {
	for _, scenario := range []gauntlet.Scenario{ArtifactTraversalScenario(), CapabilityBlobTamperScenario(), CapabilityJournalTamperScenario()} {
		runApproved(t, scenario)
	}
}

func TestStandardSuiteHasStableUniqueIDs(t *testing.T) {
	suite := StandardSuite()
	want := []string{
		"artifact.path-traversal",
		"authority.action-id-rebinding",
		"authority.stale-epoch",
		"authority.terminal-world",
		"capability.cas-tamper",
		"capability.journal-tamper",
	}
	if len(suite) != len(want) {
		t.Fatalf("suite size=%d", len(suite))
	}
	seen := map[string]bool{}
	for _, s := range suite {
		seen[s.Spec().ID] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
}

func TestStandardSuiteReportIsDeterministicAcrossRuns(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 2 * time.Second}
	first, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Approved || !second.Approved {
		t.Fatal("standard suite did not approve")
	}
	if first.ReportDigest != second.ReportDigest {
		t.Fatalf("report digest changed %q != %q", first.ReportDigest, second.ReportDigest)
	}
	one, err := gauntlet.MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := gauntlet.MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(one) != string(two) {
		t.Fatal("verified report bytes are not deterministic")
	}
}
