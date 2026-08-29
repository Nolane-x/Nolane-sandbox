package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	delegationgauntlet "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/delegation"
)

func TestRunWritesDeterministicSecretFreeEvidence(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	for _, path := range []string{a, b} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--out", path}, &stdout, &stderr); code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr.String())
		}
	}
	ra, err := os.ReadFile(a); if err != nil { t.Fatal(err) }
	rb, err := os.ReadFile(b); if err != nil { t.Fatal(err) }
	if !bytes.Equal(ra, rb) { t.Fatal("authority CLI output is not byte deterministic") }
	if bytes.Contains(ra, []byte(delegationgauntlet.SyntheticSecret)) { t.Fatal("synthetic secret leaked") }
}
