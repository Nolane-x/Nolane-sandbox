package capabilityproof

import (
	"bytes"
	"context"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

func passingFusionReport(t *testing.T) Report {
	t.Helper()
	report, err := defaultFusionRunner().Run(context.Background(), newFusionDriver())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusLivePass || !report.Approved {
		t.Fatalf("report=%+v", report)
	}
	return report
}

func TestVerifyReportRejectsTamperedNestedSubstrateProof(t *testing.T) {
	report := passingFusionReport(t)
	report.SubstrateProof.Digest = "tampered"
	if err := VerifyReport(report); err == nil {
		t.Fatal("tampered substrate proof accepted")
	}
}

func TestVerifyReportRejectsTamperedNestedRealmProof(t *testing.T) {
	report := passingFusionReport(t)
	report.RealmProof.Digest = "tampered"
	if err := VerifyReport(report); err == nil {
		t.Fatal("tampered realm proof accepted")
	}
}

func TestVerifyReportRejectsForgedCapabilityBits(t *testing.T) {
	report := passingFusionReport(t)
	report.Capabilities.SnapshotRollback = false
	if err := VerifyReport(report); err == nil {
		t.Fatal("forged capability projection accepted")
	}
}

func TestVerifyReportRejectsFingerprintMismatch(t *testing.T) {
	report := passingFusionReport(t)
	report.EndpointDigest = strings.Repeat("x", 64)
	if err := VerifyReport(report); err == nil {
		t.Fatal("outer/nested fingerprint mismatch accepted")
	}
}

func TestMarshalReportIsDeterministicAndSecretSafe(t *testing.T) {
	const secret = "SYNTHETIC-V10-CREDENTIAL"
	runner := defaultFusionRunner()
	runner.RawPublicTarget.Address = "https://public.example.invalid/" + secret

	first, err := runner.Run(context.Background(), newFusionDriver())
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Run(context.Background(), newFusionDriver())
	if err != nil {
		t.Fatal(err)
	}

	firstBody, err := MarshalReport(first, secret)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := MarshalReport(second, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatalf("non-deterministic v10 evidence\nfirst=%s\nsecond=%s", firstBody, secondBody)
	}
	if bytes.Contains(firstBody, []byte(secret)) {
		t.Fatal("raw target secret leaked into v10 evidence")
	}
}
