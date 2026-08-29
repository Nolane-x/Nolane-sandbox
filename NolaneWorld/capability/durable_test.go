package capability

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableRegistryPersistsExactMaterialAcrossRestart(t *testing.T) {
	root := t.TempDir()
	r, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	req := request([]byte("durable tool"))
	receipt, err := r.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	r2, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	material, err := r2.Load("browser", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(material.Content) != "durable tool" || string(material.Manifest) != "manifest" || string(material.VerificationEvidence) != "verification" {
		t.Fatalf("wrong material=%q/%q/%q", material.Content, material.Manifest, material.VerificationEvidence)
	}
	if material.Record.Receipt != receipt {
		t.Fatal("receipt changed across restart")
	}
}

func TestDurableRegistryRejectsSecondWriter(t *testing.T) {
	root := t.TempDir()
	one, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	two, err := OpenDurableRegistry(root)
	if two != nil {
		_ = two.Close()
	}
	if !errors.Is(err, ErrRegistryLocked) {
		t.Fatalf("second writer=%v", err)
	}
}

func TestDurableRegistryDetectsJournalTamperAndMalformedTail(t *testing.T) {
	root := t.TempDir()
	r, _ := OpenDurableRegistry(root)
	_, _ = r.Promote(request([]byte("tool")))
	_ = r.Close()
	journal := filepath.Join(root, "promotions.jsonl")
	raw, _ := os.ReadFile(journal)
	changed := strings.Replace(string(raw), `"VerifierID":"fresh-validator-1"`, `"VerifierID":"evil-validator"`, 1)
	if changed == string(raw) {
		t.Fatal("test did not modify journal")
	}
	_ = os.WriteFile(journal, []byte(changed), 0600)
	if _, err := OpenDurableRegistry(root); !errors.Is(err, ErrRegistryCorrupt) {
		t.Fatalf("journal tamper=%v", err)
	}

	root2 := t.TempDir()
	r2, _ := OpenDurableRegistry(root2)
	_, _ = r2.Promote(request([]byte("tool")))
	_ = r2.Close()
	j2 := filepath.Join(root2, "promotions.jsonl")
	b, _ := os.ReadFile(j2)
	_ = os.WriteFile(j2, append(b, []byte("{broken")...), 0600)
	if _, err := OpenDurableRegistry(root2); !errors.Is(err, ErrRegistryCorrupt) {
		t.Fatalf("malformed tail=%v", err)
	}
}

func TestDurableRegistryDetectsMissingOrTamperedTrustedBlob(t *testing.T) {
	for _, mode := range []string{"tamper", "missing"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			r, _ := OpenDurableRegistry(root)
			req := request([]byte("tool"))
			_, _ = r.Promote(req)
			_ = r.Close()
			blob := filepath.Join(root, "blobs", "sha256", req.Candidate.ContentDigest[:2], req.Candidate.ContentDigest)
			if mode == "tamper" {
				_ = os.WriteFile(blob, []byte("evil"), 0600)
			} else {
				_ = os.Remove(blob)
			}
			if _, err := OpenDurableRegistry(root); !errors.Is(err, ErrRegistryCorrupt) {
				t.Fatalf("blob %s=%v", mode, err)
			}
		})
	}
}

func TestDurableRegistryCollisionRulesSurviveRestart(t *testing.T) {
	root := t.TempDir()
	r, _ := OpenDurableRegistry(root)
	first := request([]byte("one"))
	if _, err := r.Promote(first); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	r2, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	second := request([]byte("two"))
	second.Candidate.CandidateID = "cand-2"
	if _, err := r2.Promote(second); !errors.Is(err, ErrCapabilityCollision) {
		t.Fatalf("restart version rebound=%v", err)
	}
	third := request([]byte("one"))
	third.Candidate.Name = "different"
	if _, err := r2.Promote(third); !errors.Is(err, ErrCapabilityCollision) {
		t.Fatalf("restart candidate rebound=%v", err)
	}
}

func TestOrphanBlobAloneNeverCreatesTrustedCapability(t *testing.T) {
	root := t.TempDir()
	content := []byte("orphan bytes")
	digest := Digest(content)
	path := filepath.Join(root, "blobs", "sha256", digest[:2], digest)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, ok := r.Get("browser", "1.0.0"); ok {
		t.Fatal("unreferenced CAS blob became trusted capability")
	}
}

func TestDurableRegistryExactRetryAfterRestartIsIdempotent(t *testing.T) {
	root := t.TempDir()
	r, _ := OpenDurableRegistry(root)
	req := request([]byte("same"))
	first, err := r.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	journal := filepath.Join(root, "promotions.jsonl")
	before, _ := os.ReadFile(journal)

	r2, err := OpenDurableRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r2.Promote(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Close()
	after, _ := os.ReadFile(journal)
	if first != second {
		t.Fatal("idempotent receipt changed after restart")
	}
	if string(before) != string(after) {
		t.Fatal("exact retry appended a second trusted promotion")
	}
}
