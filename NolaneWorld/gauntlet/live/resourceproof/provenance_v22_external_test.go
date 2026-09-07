package resourceproof_test

import (
	"reflect"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/resourceproof"
)

func TestV22ExternalCallerCannotConstructTrustedVerdictAuthority(t *testing.T) {
	trustedType := reflect.TypeOf(resourceproof.TrustedReport{})
	for i := 0; i < trustedType.NumField(); i++ {
		if trustedType.Field(i).IsExported() {
			t.Fatalf("TrustedReport exposes constructible authority field %q", trustedType.Field(i).Name)
		}
	}

	verdict, err := (resourceproof.TrustedReport{}).ProjectVerdict(resourceproof.VerdictRequest{
		Mode:       live.ModeProbe,
		Dimensions: []resourceproof.ResourceDimension{resourceproof.ResourceCPU},
	})
	if err != nil {
		t.Fatalf("zero trusted authority probe should remain unavailable, got %v", err)
	}
	if verdict.Status == live.StatusLivePass || verdict.Approved {
		t.Fatalf("zero trusted wrapper manufactured public authority: %+v", verdict)
	}
}

func TestV22PublicReportAndVerdictDocumentsHaveNoAuthorityProjection(t *testing.T) {
	if method, ok := reflect.TypeOf(resourceproof.Report{}).MethodByName("ProjectVerdict"); ok {
		t.Fatalf("public Report unexpectedly gained trusted verdict projection: %v", method.Type)
	}
	for _, name := range []string{"TrustedReport", "AsTrusted", "CapabilityEvidenceSource", "AsCapabilityEvidence"} {
		if method, ok := reflect.TypeOf(resourceproof.Verdict{}).MethodByName(name); ok {
			t.Fatalf("serialized Verdict unexpectedly gained authority conversion %q: %v", name, method.Type)
		}
	}
}
