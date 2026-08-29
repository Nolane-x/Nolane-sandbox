package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
)

func TestRunWritesVerifiedReportToStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	var report gauntlet.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Approved {
		t.Fatal("CLI emitted unapproved report")
	}
	if err := gauntlet.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestRunWritesDeterministicReportFile(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.json")
	two := filepath.Join(dir, "two.json")
	for _, path := range []string{one, two} {
		var out, errOut bytes.Buffer
		if code := run([]string{"--out", path}, &out, &errOut); code != 0 {
			t.Fatalf("code=%d stderr=%s", code, errOut.String())
		}
		if out.Len() != 0 {
			t.Fatal("--out also wrote report to stdout")
		}
	}
	a, err := os.ReadFile(one)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(two)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("CLI report changed across identical runs")
	}
}

func TestRunRejectsUnknownArgument(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--unknown"}, &out, &errOut); code == 0 {
		t.Fatal("unknown flag accepted")
	}
}
