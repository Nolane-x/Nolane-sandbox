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

func TestMemoryLedgerUnknownFailureBecomesPendingAndCannotReplay(t *testing.T) {
	l := NewMemoryLedger()
	calls := 0
	_, err := l.ExecuteOnce(world.ID("w-pending"), "a1", "digest-pending", func() (Receipt, error) {
		calls++
		return Receipt{}, errors.New("transport lost after dispatch")
	})
	if err == nil {
		t.Fatal("expected provider uncertainty")
	}
	status, _, err := l.Status(world.ID("w-pending"), "a1", "digest-pending")
	if err != nil {
		t.Fatal(err)
	}
	if status != ActionPending {
		t.Fatalf("status=%v", status)
	}
	_, err = l.ExecuteOnce(world.ID("w-pending"), "a1", "digest-pending", func() (Receipt, error) {
		calls++
		return Receipt{}, nil
	})
	if !errors.Is(err, ErrActionUncertain) {
		t.Fatalf("retry err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("uncertain action re-executed, calls=%d", calls)
	}
}

func TestMemoryLedgerDefinitelyNoEffectMayRetry(t *testing.T) {
	l := NewMemoryLedger()
	_, err := l.ExecuteOnce(world.ID("w-safe-retry"), "a1", "digest-safe", func() (Receipt, error) {
		return Receipt{}, ErrPolicyFailure
	})
	if !errors.Is(err, ErrPolicyFailure) {
		t.Fatalf("err=%v", err)
	}
	status, _, err := l.Status(world.ID("w-safe-retry"), "a1", "digest-safe")
	if err != nil {
		t.Fatal(err)
	}
	if status != ActionMissing {
		t.Fatalf("definitely-no-effect failure left pending status=%v", status)
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
