package main

import (
	"bytes"
	"encoding/json"
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
