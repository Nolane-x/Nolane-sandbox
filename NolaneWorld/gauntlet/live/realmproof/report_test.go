package realmproof

import (
	"bytes"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func approvedReportFixture() Report {
	return Report{
		SchemaVersion: 1,
		Profile:       realm.R0InternalOnly,
		Mode:          live.ModeProbe,
		Substrate:     "cubesandbox",
		Status:        live.StatusLivePass,
		Reason:        ReasonNone,
		Approved:      true,
		EndpointDigest: strings.Repeat("a", 64),
		TargetDigest:   strings.Repeat("b", 64),
		Capabilities: Capabilities{
			GuestExecution:       true,
			RawPublicDenied:      true,
			PublicIngressDenied:  true,
			InternalMeshVerified: false,
		},
		Scenarios: []ScenarioEvidence{
			{ID: ScenarioProfileApply, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"profile-applied"}},
			{ID: ScenarioGuestAfterProfile, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"guest-after-profile"}},
			{ID: ScenarioRawPublicDenied, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"raw-public-denied"}},
			{ID: ScenarioPublicIngressDenied, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"unauthenticated-ingress-denied"}},
			{ID: ScenarioInternalMesh, Outcome: live.OutcomeUnavailable, Reason: ReasonMeshUnsupported, Markers: []string{"private-mesh-unavailable"}},
			{ID: ScenarioCleanup, Outcome: live.OutcomePass, Reason: ReasonNone, Markers: []string{"cleanup-observed"}},
		},
	}
}

func TestApprovedReportSealsDeterministically(t *testing.T) {
	first := approvedReportFixture()
	second := approvedReportFixture()
	if err := SealReport(&first); err != nil {
		t.Fatal(err)
	}
	if err := SealReport(&second); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(first); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(second); err != nil {
		t.Fatal(err)
	}
	left, err := MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("deterministic report drift:\n%s\n%s", left, right)
	}
	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatalf("digest mismatch first=%q second=%q", first.Digest, second.Digest)
	}
}

func TestApprovedReportRequiresEveryMandatoryObservedBoundary(t *testing.T) {
	cases := []struct {
		name string
		drop string
	}{
		{"profile apply", ScenarioProfileApply},
		{"guest after profile", ScenarioGuestAfterProfile},
		{"raw public denial", ScenarioRawPublicDenied},
		{"public ingress denial", ScenarioPublicIngressDenied},
		{"cleanup", ScenarioCleanup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := approvedReportFixture()
			filtered := r.Scenarios[:0]
			for _, ev := range r.Scenarios {
				if ev.ID != tc.drop {
					filtered = append(filtered, ev)
				}
			}
			r.Scenarios = filtered
			if err := SealReport(&r); err == nil {
				t.Fatalf("approved report sealed without mandatory scenario %s", tc.drop)
			}
		})
	}
}

func TestUnavailableMeshCannotBecomeVerified(t *testing.T) {
	r := approvedReportFixture()
	r.Capabilities.InternalMeshVerified = true
	if err := SealReport(&r); err == nil {
		t.Fatal("mesh capability became verified while private mesh scenario was unavailable")
	}
}

func TestTamperedSealedReportIsRejected(t *testing.T) {
	r := approvedReportFixture()
	if err := SealReport(&r); err != nil {
		t.Fatal(err)
	}
	r.Capabilities.RawPublicDenied = false
	if err := VerifyReport(r); err == nil {
		t.Fatal("tampered report verified")
	}
}

func TestReportWireSurfaceContainsNoRawAuthorityOrTarget(t *testing.T) {
	r := approvedReportFixture()
	if err := SealReport(&r); err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalReport(r)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sandbox_id", "sandboxid", "handle", "envd", "traffic_access_token", "trafficaccesstoken", "authorization", "target_url", "target_address", "api_url"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("report leaks forbidden authority surface %q: %s", forbidden, raw)
		}
	}
}
