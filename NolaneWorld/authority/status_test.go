package authority

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestMemoryLedgerStatusMissingAndCompleted(t *testing.T) {
	l := NewMemoryLedger()
	status, _, err := l.Status(world.ID("w-status"), "a1", "digest-1")
	if err != nil {
		t.Fatal(err)
	}
	if status != ActionMissing {
		t.Fatalf("status=%v", status)
	}

	_, err = l.ExecuteOnce(world.ID("w-status"), "a1", "digest-1", func() (Receipt, error) {
		return Receipt{WorldID: world.ID("w-status"), AuthorityEpoch: 1, ActionID: "a1", RequestDigest: "digest-1", EffectDigest: "effect", CompletedAt: time.Unix(1, 0).UTC()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	status, receipt, err := l.Status(world.ID("w-status"), "a1", "digest-1")
	if err != nil {
		t.Fatal(err)
	}
	if status != ActionCompleted || receipt.ActionID != "a1" {
		t.Fatalf("status=%v receipt=%+v", status, receipt)
	}
}

func TestLedgerStatusRejectsDigestRebinding(t *testing.T) {
	l := NewMemoryLedger()
	_, err := l.ExecuteOnce(world.ID("w-status"), "a1", "digest-1", func() (Receipt, error) {
		return Receipt{WorldID: world.ID("w-status"), AuthorityEpoch: 1, ActionID: "a1", RequestDigest: "digest-1", EffectDigest: "effect", CompletedAt: time.Unix(1, 0).UTC()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = l.Status(world.ID("w-status"), "a1", "digest-2")
	if !errors.Is(err, ErrActionCollision) {
		t.Fatalf("err=%v", err)
	}
}

func TestJournalLedgerStatusPendingSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, err := OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.ExecuteOnce(world.ID("w-pending"), "a1", "digest-pending", func() (Receipt, error) {
		return Receipt{}, errors.New("transport lost after dispatch")
	})
	if err == nil {
		t.Fatal("expected uncertain execution error")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	l, err = OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	status, _, err := l.Status(world.ID("w-pending"), "a1", "digest-pending")
	if err != nil {
		t.Fatal(err)
	}
	if status != ActionPending {
		t.Fatalf("status=%v", status)
	}
}
