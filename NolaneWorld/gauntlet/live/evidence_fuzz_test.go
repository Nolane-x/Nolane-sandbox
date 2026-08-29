package live

import "testing"

func FuzzVerifyReportRejectsFabricatedGuestProof(f *testing.F) {
	f.Add("fabricated")
	f.Add("guest-canary")
	f.Fuzz(func(t *testing.T, marker string) {
		if marker == "control-plane" || marker == "guest-canary" || marker == "cleanup-observed" {
			return
		}
		r := validCoreReportForTest()
		r.Scenarios[0].Markers = []string{marker}
		sealReport(&r)
		if VerifyReport(r) == nil {
			t.Fatalf("fabricated marker accepted: %q", marker)
		}
	})
}
