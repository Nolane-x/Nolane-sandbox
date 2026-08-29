package control

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeSubstrate struct {
	mu         sync.Mutex
	createFn   func(world.ID) (substrate.Handle, error)
	destroyFn  func(substrate.Handle) error
	rollbackFn func(substrate.Handle, substrate.Snapshot) error
	snapshotFn func(substrate.Handle) (substrate.Snapshot, error)
	cloneFn    func(substrate.Handle, substrate.Snapshot, world.ID) (substrate.Handle, error)
}

func (f *fakeSubstrate) Create(_ context.Context, id world.ID) (substrate.Handle, error) {
	if f.createFn != nil {
		return f.createFn(id)
	}
	return substrate.Handle("h-" + string(id)), nil
}
func (f *fakeSubstrate) Destroy(_ context.Context, h substrate.Handle) error {
	if f.destroyFn != nil {
		return f.destroyFn(h)
	}
	return nil
}
func (f *fakeSubstrate) Pause(context.Context, substrate.Handle) error  { return nil }
func (f *fakeSubstrate) Resume(context.Context, substrate.Handle) error { return nil }
func (f *fakeSubstrate) Snapshot(_ context.Context, h substrate.Handle) (substrate.Snapshot, error) {
	if f.snapshotFn != nil {
		return f.snapshotFn(h)
	}
	return "s1", nil
}
func (f *fakeSubstrate) Rollback(_ context.Context, h substrate.Handle, s substrate.Snapshot) error {
	if f.rollbackFn != nil {
		return f.rollbackFn(h, s)
	}
	return nil
}
func (f *fakeSubstrate) Clone(_ context.Context, h substrate.Handle, s substrate.Snapshot, id world.ID) (substrate.Handle, error) {
	if f.cloneFn != nil {
		return f.cloneFn(h, s, id)
	}
	return substrate.Handle("h-" + string(id)), nil
}

func TestRollbackAdvancesEpochBeforeSubstrateAndNeverRewindsOnFailure(t *testing.T) {
	fs := &fakeSubstrate{}
	m, err := NewManager(fs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "w1"); err != nil {
		t.Fatal(err)
	}
	st, ok := m.AuthorityState("w1")
	if !ok {
		t.Fatal("state missing")
	}
	fs.rollbackFn = func(substrate.Handle, substrate.Snapshot) error {
		if got := st.CurrentEpoch(); got != 2 {
			t.Fatalf("epoch during rollback=%d want 2", got)
		}
		return errors.New("rollback failed")
	}
	if err := m.Rollback(context.Background(), "w1", "snap"); err == nil {
		t.Fatal("expected rollback failure")
	}
	if got := st.CurrentEpoch(); got != 2 {
		t.Fatalf("epoch after failed rollback=%d want 2", got)
	}
	if err := st.ValidateEpoch(1); !errors.Is(err, world.ErrStaleEpoch) {
		t.Fatalf("old epoch error=%v", err)
	}
}

func TestDestroyClosesAuthorityBeforeSubstrateAndStaysClosedOnFailure(t *testing.T) {
	fs := &fakeSubstrate{}
	m, _ := NewManager(fs)
	_, _ = m.Create(context.Background(), "w1")
	st, _ := m.AuthorityState("w1")
	fs.destroyFn = func(substrate.Handle) error {
		if err := st.ValidateEpoch(st.CurrentEpoch()); !errors.Is(err, world.ErrClosedWorld) {
			t.Fatalf("authority not closed before destroy: %v", err)
		}
		return errors.New("destroy failed")
	}
	if err := m.Destroy(context.Background(), "w1"); err == nil {
		t.Fatal("expected destroy failure")
	}
	if !st.Closed() {
		t.Fatal("world authority reopened after destroy failure")
	}
	if _, err := m.Snapshot(context.Background(), "w1"); !errors.Is(err, ErrWorldClosed) {
		t.Fatalf("snapshot after terminal revoke=%v", err)
	}
}

func TestCloneGetsIndependentAuthorityState(t *testing.T) {
	fs := &fakeSubstrate{}
	m, _ := NewManager(fs)
	_, _ = m.Create(context.Background(), "src")
	src, _ := m.AuthorityState("src")
	src.AdvanceEpoch()
	h, err := m.Clone(context.Background(), "src", "snap", "child")
	if err != nil {
		t.Fatal(err)
	}
	if h != "h-child" {
		t.Fatalf("handle=%q", h)
	}
	child, _ := m.AuthorityState("child")
	if got := child.CurrentEpoch(); got != 1 {
		t.Fatalf("child epoch=%d want 1", got)
	}
	if got := src.CurrentEpoch(); got != 2 {
		t.Fatalf("src epoch changed=%d", got)
	}
}

func TestCreateRejectsDuplicateWorld(t *testing.T) {
	m, _ := NewManager(&fakeSubstrate{})
	if _, err := m.Create(context.Background(), "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "w1"); !errors.Is(err, ErrWorldExists) {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestAuthorityStateHiddenUntilCreateCompletes(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSubstrate{}
	fs.createFn = func(id world.ID) (substrate.Handle, error) {
		close(started)
		<-release
		return substrate.Handle("h-" + string(id)), nil
	}
	m, _ := NewManager(fs)
	done := make(chan error, 1)
	go func() {
		_, err := m.Create(context.Background(), "w-pending")
		done <- err
	}()
	<-started
	if _, ok := m.AuthorityState("w-pending"); ok {
		t.Fatal("authority state became available before substrate creation completed")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := m.AuthorityState("w-pending"); !ok {
		t.Fatal("authority state missing after create completed")
	}
}
