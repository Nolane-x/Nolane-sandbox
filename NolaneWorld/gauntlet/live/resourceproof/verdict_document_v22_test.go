package resourceproof

import (
	"bytes"
	"errors"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

func trustedPassingVerdict(t *testing.T, dimensions ...ResourceDimension) Verdict {
	t.Helper()
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())
	verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: dimensions})
	if err != nil {
		t.Fatalf("trusted verdict setup failed: %v", err)
	}
	if err := VerifyVerdict(verdict); err != nil {
		t.Fatalf("trusted verdict setup produced invalid document: %v", err)
	}
	return verdict
}

func TestV22VerdictMarshalIsDeterministicAndVerified(t *testing.T) {
	verdict := trustedPassingVerdict(t, ResourceMemory, ResourceCPU)
	first, err := MarshalVerdict(verdict)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalVerdict(verdict)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("canonical verdict marshal is nondeterministic")
	}
	if len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatal("canonical verdict must end in newline")
	}
}

func TestV22VerdictTamperingIsDetected(t *testing.T) {
	base := trustedPassingVerdict(t, ResourceCPU, ResourceMemory)

	cases := map[string]func(Verdict) Verdict{
		"status": func(v Verdict) Verdict {
			v.Status = live.StatusUnavailable
			return v
		},
		"approved": func(v Verdict) Verdict {
			v.Approved = false
			return v
		},
		"binding": func(v Verdict) Verdict {
			v.Binding.RealizationRevision++
			return v
		},
		"digest": func(v Verdict) Verdict {
			v.Digest = "tampered"
			return v
		},
		"reordered request": func(v Verdict) Verdict {
			v.Requested = []ResourceDimension{ResourceMemory, ResourceCPU}
			return v
		},
		"missing verified evidence": func(v Verdict) Verdict {
			v.Results = append([]DimensionVerdict(nil), v.Results...)
			v.Results[0].Evidence = ""
			return v
		},
		"verified disk": func(v Verdict) Verdict {
			v.Requested = []ResourceDimension{ResourceDisk}
			v.Results = []DimensionVerdict{{Dimension: ResourceDisk, State: DimensionVerified, Reason: ReasonNone, Evidence: "forged"}}
			v.Digest = verdictDigest(v)
			return v
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := VerifyVerdict(mutate(base)); !errors.Is(err, ErrInvalidVerdict) {
				t.Fatalf("tampered verdict was accepted: %v", err)
			}
		})
	}
}

func TestV22UnavailableVerdictCannotCarryEvidence(t *testing.T) {
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())
	verdict, err := trusted.ProjectVerdict(VerdictRequest{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceDisk}})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyVerdict(verdict); err != nil {
		t.Fatalf("valid unavailable verdict rejected: %v", err)
	}
	verdict.Results = append([]DimensionVerdict(nil), verdict.Results...)
	verdict.Results[0].Evidence = "forged-evidence"
	verdict.Digest = verdictDigest(verdict)
	if err := VerifyVerdict(verdict); !errors.Is(err, ErrInvalidVerdict) {
		t.Fatalf("unavailable result accepted injected evidence: %v", err)
	}
}

func TestV22MarshalRejectsForbiddenMaterial(t *testing.T) {
	verdict := trustedPassingVerdict(t, ResourceCPU)
	if _, err := MarshalVerdict(verdict, string(verdict.Binding.RealmID)); !errors.Is(err, ErrInvalidVerdict) {
		t.Fatalf("forbidden material was not rejected: %v", err)
	}
}

func TestV22InvalidRequestsFailClosed(t *testing.T) {
	trusted := buildTrustedReport(live.ModeProbe, validBinding(), validCPUObservation(), validMemoryObservation())
	cases := []VerdictRequest{
		{Mode: live.ModeProbe},
		{Mode: "invalid", Dimensions: []ResourceDimension{ResourceCPU}},
		{Mode: live.ModeProbe, Dimensions: []ResourceDimension{ResourceCPU, ResourceCPU}},
		{Mode: live.ModeProbe, Dimensions: []ResourceDimension{"gpu"}},
	}
	for _, req := range cases {
		verdict, err := trusted.ProjectVerdict(req)
		if !errors.Is(err, ErrInvalidVerdictRequest) {
			t.Fatalf("invalid request did not fail closed: req=%+v verdict=%+v err=%v", req, verdict, err)
		}
		if verdict.Approved || verdict.Status == live.StatusLivePass {
			t.Fatalf("invalid request manufactured approval: %+v", verdict)
		}
	}
}
