package realm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func validSpec() Spec {
	return Spec{
		ID:             ID("realm://test"),
		MaxWorlds:      4,
		DefaultLease:   time.Minute,
		NetworkProfile: R0InternalOnly,
		ResourceBudget: ResourceBudget{CPUUnits: 4, MemoryMiB: 4096, DiskMiB: 8192},
	}
}

func TestSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Spec)
	}{
		{"empty-id", func(s *Spec) { s.ID = "" }},
		{"bad-id", func(s *Spec) { s.ID = "realm://../escape" }},
		{"zero-worlds", func(s *Spec) { s.MaxWorlds = 0 }},
		{"zero-lease", func(s *Spec) { s.DefaultLease = 0 }},
		{"bad-profile", func(s *Spec) { s.NetworkProfile = NetworkProfile("N5_CONSEQUENTIAL_WRITE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec()
			tt.mut(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("Validate() err=%v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestPolicyDigestBindsExactSpecAndRevision(t *testing.T) {
	spec := validSpec()
	first, err := PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || replay != first {
		t.Fatalf("policy digest is not deterministic: first=%q replay=%q", first, replay)
	}
	changedSpec := spec
	changedSpec.MaxWorlds++
	changed, err := PolicyDigest(changedSpec, 1)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("policy digest did not change when the host-owned Realm spec changed")
	}
	advanced, err := PolicyDigest(spec, 2)
	if err != nil {
		t.Fatal(err)
	}
	if advanced == first {
		t.Fatal("policy digest did not bind the exact Realm revision")
	}
	if _, err := PolicyDigest(spec, 0); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("zero revision err=%v", err)
	}
}

func TestControllerRevisionFenceAndIdentityImmutability(t *testing.T) {
	store := NewMemoryStore()
	ctl, err := NewController(store)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := ctl.Create(context.Background(), validSpec())
	if err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 1 {
		t.Fatalf("revision=%d, want 1", rec.Revision)
	}

	changed := rec.Spec
	changed.MaxWorlds = 8
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, 99, changed); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale update err=%v", err)
	}

	changed.ID = ID("realm://other")
	if _, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, changed); !errors.Is(err, ErrIdentityRebind) {
		t.Fatalf("identity rebind err=%v", err)
	}

	changed.ID = rec.Spec.ID
	updated, err := ctl.Update(context.Background(), rec.Spec.ID, rec.Revision, changed)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision=%d, want 2", updated.Revision)
	}

	if err := ctl.Close(context.Background(), rec.Spec.ID, rec.Revision); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale close err=%v", err)
	}
	if err := ctl.Close(context.Background(), rec.Spec.ID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	closed, ok := store.Realm(rec.Spec.ID)
	if !ok || !closed.Closed {
		t.Fatalf("closed=%+v ok=%v", closed, ok)
	}
}
