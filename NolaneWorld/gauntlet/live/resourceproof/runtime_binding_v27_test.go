package resourceproof

import (
	"errors"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
)

func TestRuntimeBindingV27ZeroAuthorityFailsClosed(t *testing.T) {
	_, err := BindingFromRuntimeAuthority(cube.RealmResourceRuntimeAuthority{})
	if !errors.Is(err, ErrRuntimeBindingUnavailable) {
		t.Fatalf("zero authority error=%v, want ErrRuntimeBindingUnavailable", err)
	}
}

func TestRuntimeBindingV27DescriptiveBindingDoesNotMintTrustedReport(t *testing.T) {
	binding := validBinding()
	binding.RuntimeDigest = "runtime-realization-v27:" + strings.Repeat("a", 64)

	report := BuildReport(live.ModeRequireLive, binding, validCPUObservation(), validMemoryObservation())
	if report.Status != live.StatusUnavailable || report.Approved {
		t.Fatalf("descriptive Wave27-looking binding elevated public observations: %+v", report)
	}
	if err := VerifyReport(report); err != nil {
		t.Fatalf("canonical untrusted report rejected: %v", err)
	}
}
