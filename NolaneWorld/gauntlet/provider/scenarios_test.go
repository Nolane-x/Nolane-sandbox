package providergauntlet

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func TestStandardSuiteCoversMandatoryProviderAuthorityScenarios(t *testing.T) {
	want := []string{
		"provider.v7.action-resource-collision",
		"provider.v7.broker-noncanonical",
		"provider.v7.broker-oversized",
		"provider.v7.broker-peer-mismatch",
		"provider.v7.comment-ambiguous-once",
		"provider.v7.comment-reconcile-observed",
		"provider.v7.comment-reconcile-unknown",
		"provider.v7.concurrent-secret-leases",
		"provider.v7.contents-ambiguous-once",
		"provider.v7.contents-reconcile-observed",
		"provider.v7.endpoint-rebinding",
		"provider.v7.generic-auth-http",
		"provider.v7.payload-canonicality",
		"provider.v7.provider-error-sanitized",
		"provider.v7.redirect-credential-contained",
		"provider.v7.resource-canonicality",
		"provider.v7.secret-evidence-absence",
		"provider.v7.stale-revoked-denied",
		"provider.v7.v4-evidence-stable",
		"provider.v7.v6-evidence-stable",
	}
	scenarios := Standard()
	if len(scenarios) != len(want) {
		t.Fatalf("scenario count=%d want=%d", len(scenarios), len(want))
	}
	seen := make(map[string]bool, len(scenarios))
	for _, scenario := range scenarios {
		seen[scenario.Spec().ID] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("missing mandatory scenario %s", id)
		}
	}
}

func TestStandardSuiteApprovedDeterministicAndSecretFree(t *testing.T) {
	policy := gauntlet.Policy{ProductID: gauntlet.ProductNolaneSandbox, ScenarioTimeout: 5 * time.Second}
	a, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunStandard(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Approved || !b.Approved {
		t.Fatalf("provider authority report rejected: a=%v b=%v", a.Approved, b.Approved)
	}
	ra, err := gauntlet.MarshalReport(a)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := gauntlet.MarshalReport(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ra, rb) {
		t.Fatal("v7 provider authority evidence is not byte deterministic")
	}
	forms := [][]byte{
		[]byte(SyntheticSecret),
		[]byte(base64.StdEncoding.EncodeToString([]byte(SyntheticSecret))),
		[]byte(hex.EncodeToString([]byte(SyntheticSecret))),
	}
	for _, form := range forms {
		if bytes.Contains(ra, form) {
			t.Fatalf("synthetic secret representation leaked into report: %q", form)
		}
	}
}
