package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet"
	freedomgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/freedom"
)

func TestRunWritesApprovedDeterministicSecretFreeFreedomEvidence(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "freedom-v8-a.json")
	second := filepath.Join(dir, "freedom-v8-b.json")
	for _, path := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--out", path}, &stdout, &stderr); code != 0 {
			t.Fatalf("run code=%d stderr=%s", code, stderr.String())
		}
		if stdout.Len() != 0 { t.Fatalf("unexpected stdout=%q", stdout.String()) }
	}
	a, err := os.ReadFile(first); if err != nil { t.Fatal(err) }
	b, err := os.ReadFile(second); if err != nil { t.Fatal(err) }
	if !bytes.Equal(a, b) { t.Fatal("Freedom v8 CLI evidence is not byte deterministic") }
	var report gauntlet.Report
	if err := json.Unmarshal(a, &report); err != nil { t.Fatal(err) }
	if !report.Approved || len(report.Scenarios) != 20 { t.Fatalf("approved=%v scenarios=%d", report.Approved, len(report.Scenarios)) }
	forms := [][]byte{[]byte(freedomgauntlet.SyntheticSecret), []byte(base64.StdEncoding.EncodeToString([]byte(freedomgauntlet.SyntheticSecret))), []byte(hex.EncodeToString([]byte(freedomgauntlet.SyntheticSecret)))}
	for _, form := range forms { if bytes.Contains(a, form) { t.Fatalf("secret representation leaked: %q", form) } }
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"unexpected"}, &stdout, &stderr); code != 2 { t.Fatalf("code=%d", code) }
}
