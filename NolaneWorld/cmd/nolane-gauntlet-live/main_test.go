package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
)

func TestProbeWithoutLiveConfigEmitsUnavailableAndExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--mode", "probe"}, func(string) string { return "" }, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	var r live.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Status != live.StatusUnavailable || r.Approved {
		t.Fatalf("report=%+v", r)
	}
	if err := live.VerifyReport(r); err != nil {
		t.Fatal(err)
	}
}
func TestRequireLiveWithoutConfigEmitsUnavailableAndFailsGate(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--mode", "require-live"}, func(string) string { return "" }, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero report=%s", out.String())
	}
	var r live.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Status != live.StatusUnavailable || r.Approved {
		t.Fatalf("report=%+v", r)
	}
}
func TestProbeOutputNeverContainsConfiguredAPIKeyWhenOtherConfigMissing(t *testing.T) {
	var out, errOut bytes.Buffer
	getenv := func(k string) string {
		if k == "NOLANE_CUBE_API_KEY" {
			return "SUPER-SECRET-CONTROL-KEY"
		}
		return ""
	}
	code := run([]string{"--mode", "probe"}, getenv, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if strings.Contains(out.String(), "SUPER-SECRET-CONTROL-KEY") {
		t.Fatal("API key leaked")
	}
}

func TestWave29InternalHelperDispatchPrecedesNormalGauntletRun(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	helper := strings.Index(text, "MaybeRunInternalCgroupHelper()")
	normal := strings.Index(text, "run(os.Args[1:]")
	if helper < 0 {
		t.Fatal("main does not invoke Wave29 internal helper entrypoint")
	}
	if normal < 0 {
		t.Fatal("normal gauntlet main flow not found")
	}
	if helper > normal {
		t.Fatal("internal helper dispatch occurs after normal gauntlet flow")
	}
}
