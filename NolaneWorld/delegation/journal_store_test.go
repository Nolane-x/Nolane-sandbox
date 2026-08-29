package delegation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalStoreSurvivesRestartAndRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegations.jsonl")
	s, err := OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	g := validGrant()
	if err := s.Issue(g); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, err := s.Lookup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Revoked || state.Grant.Resource != g.Resource {
		t.Fatalf("state=%+v", state)
	}
}

func TestJournalStoreRejectsSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegations.jsonl")
	s, err := OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := OpenJournalStore(path); !errors.Is(err, ErrStoreLocked) && !errors.Is(err, ErrStoreLockUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

func TestJournalStoreRejectsTamper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegations.jsonl")
	s, err := OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	g := validGrant()
	if err := s.Issue(g); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		if raw[i] == 'N' {
			raw[i] = 'M'
			break
		}
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournalStore(path); !errors.Is(err, ErrStoreCorrupt) {
		t.Fatalf("err=%v", err)
	}
}

func TestJournalStoreRejectsMalformedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegations.jsonl")
	s, err := OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Issue(validGrant()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{broken\n")
	_ = f.Close()
	if _, err := OpenJournalStore(path); !errors.Is(err, ErrStoreCorrupt) {
		t.Fatalf("err=%v", err)
	}
}

func TestJournalStoreRefusesIllegalTransitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegations.jsonl")
	s, err := OpenJournalStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Revoke(ID("missing")); !errors.Is(err, ErrDelegationNotFound) {
		t.Fatalf("err=%v", err)
	}
	g := validGrant()
	if err := s.Issue(g); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(g.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(g.ID); !errors.Is(err, ErrAlreadyRevoked) {
		t.Fatalf("err=%v", err)
	}
}
