package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalCatalogRecoversLegalLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worlds.jsonl")
	c, err := OpenJournalCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.BeginCreate("w"); err != nil {
		t.Fatal(err)
	}
	if err := c.Ready("w", "sb-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Terminal("w"); err != nil {
		t.Fatal(err)
	}
	if err := c.Destroyed("w"); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c2, err := OpenJournalCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	rec, ok := c2.Get("w")
	if !ok || rec.Phase != PhaseDestroyed || rec.Handle != "sb-1" {
		t.Fatalf("record=%+v ok=%v", rec, ok)
	}
}

func TestJournalCatalogRejectsIllegalTransitionAndSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worlds.jsonl")
	c, err := OpenJournalCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.BeginCreate("w"); err != nil {
		t.Fatal(err)
	}
	if err := c.Destroyed("w"); !errors.Is(err, ErrCatalogTransition) {
		t.Fatalf("destroy creating=%v", err)
	}
	second, err := OpenJournalCatalog(path)
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, ErrCatalogLocked) {
		t.Fatalf("second writer=%v", err)
	}
}

func TestJournalCatalogDetectsTamperAndMalformedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worlds.jsonl")
	c, _ := OpenJournalCatalog(path)
	_ = c.BeginCreate("w")
	_ = c.Ready("w", "sb")
	_ = c.Close()
	raw, _ := os.ReadFile(path)
	changed := strings.Replace(string(raw), `"handle":"sb"`, `"handle":"evil"`, 1)
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournalCatalog(path); !errors.Is(err, ErrCatalogCorrupt) {
		t.Fatalf("tamper=%v", err)
	}

	path2 := filepath.Join(t.TempDir(), "worlds.jsonl")
	c2, _ := OpenJournalCatalog(path2)
	_ = c2.BeginCreate("x")
	_ = c2.Close()
	b, _ := os.ReadFile(path2)
	_ = os.WriteFile(path2, append(b, []byte("{bad")...), 0600)
	if _, err := OpenJournalCatalog(path2); !errors.Is(err, ErrCatalogCorrupt) {
		t.Fatalf("tail=%v", err)
	}
}
