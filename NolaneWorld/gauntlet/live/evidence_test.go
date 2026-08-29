package live

import (
	"strings"
	"testing"
)

func validCoreReportForTest() Report {
	r := Report{
		SchemaVersion:  1,
		Profile:        ProfileCore,
		Mode:           ModeRequireLive,
		Substrate:      "cubesandbox",
		Status:         StatusLivePass,
		Reason:         ReasonNone,
		Approved:       true,
		EndpointDigest: digestString("https://cube.example"),
		TemplateDigest: digestString("tpl"),
		Capabilities:   Capabilities{ControlPlane: true, GuestExecution: true, SnapshotRollback: true, CleanupObserved: true},
		Scenarios: []ScenarioEvidence{
			{ID: ScenarioGuestExecution, Outcome: OutcomePass, Reason: ReasonNone, Markers: []string{"control-plane", "guest-canary", "cleanup-observed"}},
			{ID: ScenarioSnapshotAuthority, Outcome: OutcomePass, Reason: ReasonNone, Markers: []string{"control-plane", "snapshot-observed", "rollback-restored-a", "stale-authority-denied", "cleanup-observed"}},
		},
	}
	sealReport(&r)
	return r
}

func TestVerifyReportRejectsFakeLivePassWithoutMaterialCapabilities(t *testing.T) {
	r := validCoreReportForTest()
	r.Capabilities.GuestExecution = false
	sealReport(&r)
	if err := VerifyReport(r); err == nil {
		t.Fatal("fake LIVE_PASS accepted")
	}
	r = validCoreReportForTest()
	r.Capabilities.CleanupObserved = false
	sealReport(&r)
	if err := VerifyReport(r); err == nil {
		t.Fatal("LIVE_PASS without cleanup accepted")
	}
}

func TestVerifyReportRejectsScenarioOrDigestMutation(t *testing.T) {
	r := validCoreReportForTest()
	r.Scenarios[0].Markers[0] = "fabricated"
	if err := VerifyReport(r); err == nil {
		t.Fatal("scenario mutation accepted")
	}
	r = validCoreReportForTest()
	r.Digest = strings.Repeat("0", 64)
	if err := VerifyReport(r); err == nil {
		t.Fatal("digest mutation accepted")
	}
}

func TestUnavailableIsValidDiagnosticButNeverApproved(t *testing.T) {
	r := newUnavailableReport(ProfileCore, ModeProbe, ReasonConfigMissing, "", "")
	if err := VerifyReport(r); err != nil {
		t.Fatal(err)
	}
	if r.Approved || r.Status != StatusUnavailable {
		t.Fatalf("report=%+v", r)
	}
}

func TestMarshalReportDoesNotLeakForbiddenSecrets(t *testing.T) {
	r := validCoreReportForTest()
	b, err := MarshalReport(r, "control-secret", "envd-secret", "traffic-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"control-secret", "envd-secret", "traffic-secret"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("leaked %q", secret)
		}
	}
}

func TestVerifyReportRejectsResealedPassWithFabricatedMarkers(t *testing.T) {
	r := validCoreReportForTest()
	r.Scenarios[0].Markers = []string{"made-up-proof"}
	sealReport(&r)
	if err := VerifyReport(r); err == nil {
		t.Fatal("resealed pass with fabricated proof markers accepted")
	}
}
