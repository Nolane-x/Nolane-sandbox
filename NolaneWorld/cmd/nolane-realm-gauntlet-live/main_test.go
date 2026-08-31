package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func emptyEnv(string) string { return "" }

func TestProbeWithoutLiveConfigEmitsDeterministicUnavailableArtifact(t *testing.T) {
	args := []string{"--mode", "probe", "--profile", string(realm.R0InternalOnly), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"}
	var out1, out2, err1, err2 bytes.Buffer
	if code := run(args, emptyEnv, &out1, &err1); code != 0 {
		t.Fatalf("first code=%d stderr=%s", code, err1.String())
	}
	if code := run(args, emptyEnv, &out2, &err2); code != 0 {
		t.Fatalf("second code=%d stderr=%s", code, err2.String())
	}
	if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
		t.Fatalf("probe artifact drift\nfirst=%s\nsecond=%s", out1.String(), out2.String())
	}
	var report realmproof.Report
	if err := json.Unmarshal(out1.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Profile != realm.R0InternalOnly || report.Mode != live.ModeProbe {
		t.Fatalf("binding=%+v", report)
	}
	if report.Status != live.StatusUnavailable || report.Approved || report.Reason != realmproof.ReasonConfigMissing {
		t.Fatalf("report=%+v", report)
	}
	if err := realmproof.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestRequireLiveWithoutConfigFailsClosedButStillEmitsVerifiedUnavailableArtifact(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--mode", "require-live", "--profile", string(realm.R1PublicRead), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"}, emptyEnv, &out, &errOut)
	if code == 0 {
		t.Fatalf("require-live unexpectedly succeeded artifact=%s", out.String())
	}
	var report realmproof.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != live.StatusUnavailable || report.Approved {
		t.Fatalf("report=%+v", report)
	}
	if err := realmproof.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestAllRealmProfilesAreAcceptedByParser(t *testing.T) {
	for _, profile := range []realm.NetworkProfile{realm.R0InternalOnly, realm.R1PublicRead, realm.R2SupplyChain} {
		var out, errOut bytes.Buffer
		code := run([]string{"--mode", "probe", "--profile", string(profile), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"}, emptyEnv, &out, &errOut)
		if code != 0 {
			t.Fatalf("profile=%s code=%d stderr=%s", profile, code, errOut.String())
		}
		var report realmproof.Report
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.Profile != profile {
			t.Fatalf("got profile=%s want=%s", report.Profile, profile)
		}
	}
}

func TestInvalidModeProfileAndTargetKindAreUsageErrors(t *testing.T) {
	cases := [][]string{
		{"--mode", "maybe", "--profile", string(realm.R0InternalOnly), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"},
		{"--mode", "probe", "--profile", "R9_UNKNOWN", "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"},
		{"--mode", "probe", "--profile", string(realm.R0InternalOnly), "--raw-public-kind", "smtp", "--raw-public-target", "mail.example.test:25"},
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		if code := run(args, emptyEnv, &out, &errOut); code != 2 {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, out.String(), errOut.String())
		}
		if out.Len() != 0 {
			t.Fatalf("usage error emitted artifact args=%v stdout=%s", args, out.String())
		}
	}
}

func TestProbeArtifactNeverContainsCubeCredentialPlainBase64OrHex(t *testing.T) {
	secret := "V9-SYNTHETIC-CUBE-CREDENTIAL"
	getenv := func(key string) string {
		if key == "NOLANE_CUBE_API_KEY" {
			return secret
		}
		return ""
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--mode", "probe", "--profile", string(realm.R2SupplyChain), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe"}, getenv, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	for _, forbidden := range credentialEncodings(secret) {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("credential representation leaked: %q", forbidden)
		}
	}
}

func TestOutWritesCanonicalArtifactWithoutDuplicatingStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm-v9.json")
	var out, errOut bytes.Buffer
	code := run([]string{"--mode", "probe", "--profile", string(realm.R0InternalOnly), "--raw-public-kind", string(live.TargetHTTP), "--raw-public-target", "https://example.test/probe", "--out", path}, emptyEnv, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty with --out: %s", out.String())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report realmproof.Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if err := realmproof.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
}
