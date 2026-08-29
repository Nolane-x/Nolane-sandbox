package authority

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func sampleReceipt(w world.ID, action, digest string) Receipt {
	return Receipt{WorldID: w, AuthorityEpoch: 1, ActionID: action, RequestDigest: digest, EffectDigest: "effect", CompletedAt: time.Unix(10, 0).UTC()}
}

func TestJournalLedgerSurvivesRestartWithoutReexecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, err := OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	want := sampleReceipt("w", "a", "d")
	got, err := l.ExecuteOnce("w", "a", "d", func() (Receipt, error) { calls.Add(1); return want, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got=%+v", got)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l2, err := OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	got, err = l2.ExecuteOnce("w", "a", "d", func() (Receipt, error) { calls.Add(1); return Receipt{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got != want || calls.Load() != 1 {
		t.Fatalf("restart got=%+v calls=%d", got, calls.Load())
	}
}

func TestJournalLedgerExecutorErrorBecomesUncertainAndNeverReexecutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, _ := OpenJournalLedger(path)
	var calls atomic.Int32
	_, err := l.ExecuteOnce("w", "a", "d", func() (Receipt, error) {
		calls.Add(1)
		return Receipt{}, errors.Join(ErrExecutionFailure, errors.New("timeout"))
	})
	if !errors.Is(err, ErrExecutionFailure) {
		t.Fatalf("first error=%v", err)
	}
	_ = l.Close()
	l2, err := OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	_, err = l2.ExecuteOnce("w", "a", "d", func() (Receipt, error) { calls.Add(1); return sampleReceipt("w", "a", "d"), nil })
	if !errors.Is(err, ErrActionUncertain) {
		t.Fatalf("retry error=%v want uncertain", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("executor re-ran after uncertain crash state: %d", calls.Load())
	}
}

func TestJournalLedgerPolicyDenialDoesNotPoisonAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, _ := OpenJournalLedger(path)
	defer l.Close()
	_, err := l.ExecuteOnce("w", "a", "d", func() (Receipt, error) { return Receipt{}, ErrDenied })
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("error=%v", err)
	}
	var calls int
	_, err = l.ExecuteOnce("w", "a", "d", func() (Receipt, error) { calls++; return sampleReceipt("w", "a", "d"), nil })
	if err != nil || calls != 1 {
		t.Fatalf("retry err=%v calls=%d", err, calls)
	}
}

func TestJournalLedgerResolveUncertainAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, _ := OpenJournalLedger(path)
	defer l.Close()
	_, _ = l.ExecuteOnce("w", "a", "d", func() (Receipt, error) {
		return Receipt{}, errors.Join(ErrExecutionFailure, errors.New("lost response"))
	})
	want := sampleReceipt("w", "a", "d")
	if err := l.Resolve("w", "a", "d", want); err != nil {
		t.Fatal(err)
	}
	got, err := l.ExecuteOnce("w", "a", "d", func() (Receipt, error) { t.Fatal("must not execute"); return Receipt{}, nil })
	if err != nil || got != want {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestJournalLedgerRejectsCollisionAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	l, _ := OpenJournalLedger(path)
	_, _ = l.ExecuteOnce("w", "a", "d1", func() (Receipt, error) { return sampleReceipt("w", "a", "d1"), nil })
	if _, err := l.ExecuteOnce("w", "a", "d2", func() (Receipt, error) { return Receipt{}, nil }); !errors.Is(err, ErrActionCollision) {
		t.Fatalf("collision=%v", err)
	}
	_ = l.Close()
	if err := os.WriteFile(path, append(mustRead(t, path), []byte("{bad")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournalLedger(path); !errors.Is(err, ErrLedgerCorrupt) {
		t.Fatalf("corruption error=%v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestJournalLedgerEnforcesSingleWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effects.jsonl")
	first, err := OpenJournalLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenJournalLedger(path)
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, ErrLedgerLocked) {
		t.Fatalf("second open error=%v want ErrLedgerLocked", err)
	}
}
