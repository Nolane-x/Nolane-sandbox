package freedomgauntlet

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

var expectedFreedomIDs = []string{
	"freedom.acquire-collision",
	"freedom.acquire-idempotency",
	"freedom.agent-projection-secret-free",
	"freedom.authority-noninheritance",
	"freedom.baseline-fresh-create",
	"freedom.baseline-identity-isolation",
	"freedom.capability-fail-honest",
	"freedom.checkpoint-authority-nonrewind",
	"freedom.exec-bounded-output",
	"freedom.exec-uncertain-not-success",
	"freedom.persistence-tamper",
	"freedom.profile-no-n3-n5",
	"freedom.profile-no-public-ingress",
	"freedom.realm-policy-host-only",
	"freedom.restart-no-false-ready",
	"freedom.service-generation-stale",
	"freedom.stale-lease-denial",
	"freedom.terminal-world-denial",
	"freedom.v4-v6-v7-nondrift",
	"freedom.v5-unavailable-not-pass",
}

func TestStandardFreedomSuiteHasExactlyTwentyStableScenarios(t *testing.T) {
	suite := Standard()
	if len(suite) != len(expectedFreedomIDs) {
		t.Fatalf("scenario count=%d want=%d", len(suite), len(expectedFreedomIDs))
	}
	for i, scenario := range suite {
		if got := scenario.Spec().ID; got != expectedFreedomIDs[i] {
			t.Fatalf("scenario[%d]=%q want=%q", i, got, expectedFreedomIDs[i])
		}
	}
}

func TestFreedomGauntletApprovesAndIsByteDeterministic(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	first, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Approved || !second.Approved {
		t.Fatalf("freedom gauntlet not approved: first=%v failures=%s second=%v failures=%s", first.Approved, failureSummary(first), second.Approved, failureSummary(second))
	}
	if len(first.Scenarios) != 20 || len(second.Scenarios) != 20 {
		t.Fatalf("unexpected report size: %d %d", len(first.Scenarios), len(second.Scenarios))
	}
	one, err := gauntlet.MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := gauntlet.MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) {
		t.Fatal("Freedom Gauntlet report is not byte deterministic")
	}
	for _, ev := range first.Scenarios {
		if ev.Outcome != gauntlet.OutcomePass || len(ev.Events) < 3 {
			t.Fatalf("scenario lacks real proof: %+v", ev)
		}
	}
}

func failureSummary(report gauntlet.Report) string {
	parts := make([]string, 0)
	for _, ev := range report.Scenarios {
		if ev.Outcome != gauntlet.OutcomePass {
			parts = append(parts, fmt.Sprintf("%s[%s:%s events=%v]", ev.ID, ev.FailureCode, ev.FailureMessage, ev.Events))
		}
	}
	return strings.Join(parts, "; ")
}

func TestFreedomEvidenceContainsNoSyntheticCredentialOrReversibleEncoding(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	report, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatalf("cannot scan rejected report: %s", failureSummary(report))
	}
	raw, err := gauntlet.MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte(SyntheticSecret)
	for label, needle := range map[string][]byte{
		"plain": secret,
		"base64": []byte(base64.StdEncoding.EncodeToString(secret)),
		"hex": []byte(hex.EncodeToString(secret)),
	} {
		if bytes.Contains(bytes.ToLower(raw), bytes.ToLower(needle)) {
			t.Fatalf("Freedom evidence leaked synthetic credential via %s", label)
		}
	}
	if strings.Contains(strings.ToLower(string(raw)), "authorization: bearer") {
		t.Fatal("Freedom evidence contains credential-bearing header material")
	}
}
