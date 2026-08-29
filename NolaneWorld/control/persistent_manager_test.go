package control

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func newPersistentParts(t *testing.T) (*world.DurableFactory, *JournalCatalog) {
	t.Helper()
	root := t.TempDir()
	f, err := world.NewDurableFactory(filepath.Join(root, "authority"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenJournalCatalog(filepath.Join(root, "worlds.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func TestPersistentRollbackEpochSurvivesManagerRestart(t *testing.T) {
	f, c := newPersistentParts(t)
	fs := &fakeSubstrate{}
	m, issues, err := NewPersistentManager(fs, f, c)
	if err != nil || len(issues) != 0 {
		t.Fatalf("new err=%v issues=%v", err, issues)
	}
	if _, err := m.Create(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	fs.rollbackFn = func(substrate.Handle, substrate.Snapshot) error { return errors.New("rollback failed") }
	if err := m.Rollback(context.Background(), "w", "snap"); err == nil {
		t.Fatal("expected failure")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	c2, err := OpenJournalCatalog(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	m2, issues, err := NewPersistentManager(&fakeSubstrate{}, f, c2)
	if err != nil || len(issues) != 0 {
		t.Fatalf("recover err=%v issues=%v", err, issues)
	}
	defer m2.Close()
	st, ok := m2.AuthorityState("w")
	if !ok || st.CurrentEpoch() != 2 {
		t.Fatalf("state ok=%v epoch=%v", ok, st)
	}
	if err := st.ValidateEpoch(1); !errors.Is(err, world.ErrStaleEpoch) {
		t.Fatalf("stale=%v", err)
	}
}

func TestPersistentDestroyRevocationSurvivesFailureAndRestart(t *testing.T) {
	f, c := newPersistentParts(t)
	fs := &fakeSubstrate{destroyFn: func(substrate.Handle) error { return errors.New("host down") }}
	m, _, _ := NewPersistentManager(fs, f, c)
	_, _ = m.Create(context.Background(), "w")
	if err := m.Destroy(context.Background(), "w"); err == nil {
		t.Fatal("expected destroy error")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	c2, _ := OpenJournalCatalog(c.Path())
	m2, issues, err := NewPersistentManager(&fakeSubstrate{}, f, c2)
	if err != nil || len(issues) != 0 {
		t.Fatalf("recover err=%v issues=%v", err, issues)
	}
	defer m2.Close()
	st, ok := m2.AuthorityState("w")
	if !ok {
		t.Fatal("terminal state should remain inspectable")
	}
	if err := st.ValidateEpoch(st.CurrentEpoch()); !errors.Is(err, world.ErrClosedWorld) {
		t.Fatalf("revocation lost=%v", err)
	}
	if _, err := m2.Snapshot(context.Background(), "w"); !errors.Is(err, ErrWorldClosed) {
		t.Fatalf("terminal snapshot=%v", err)
	}
}

func TestRecoveryQuarantinesIncompleteCreate(t *testing.T) {
	f, c := newPersistentParts(t)
	st, err := f.Create("uncertain")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Release()
	if err := c.BeginCreate("uncertain"); err != nil {
		t.Fatal(err)
	}
	path := c.Path()
	_ = c.Close()

	c2, _ := OpenJournalCatalog(path)
	m, issues, err := NewPersistentManager(&fakeSubstrate{}, f, c2)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if len(issues) != 1 || issues[0].Kind != RecoveryIssuePossibleOrphan {
		t.Fatalf("issues=%+v", issues)
	}
	st2, ok := m.AuthorityState("uncertain")
	if !ok {
		t.Fatal("incomplete create state missing")
	}
	if err := st2.ValidateEpoch(st2.CurrentEpoch()); !errors.Is(err, world.ErrClosedWorld) {
		t.Fatalf("incomplete create not terminal: %v", err)
	}
}

func TestRecoveryFailsIfCatalogAuthorityStateMissing(t *testing.T) {
	f, c := newPersistentParts(t)
	if err := c.BeginCreate("missing"); err != nil {
		t.Fatal(err)
	}
	if err := c.Ready("missing", "sb"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewPersistentManager(&fakeSubstrate{}, f, c); err == nil {
		t.Fatal("missing authority state must fail closed")
	}
}

type failingCatalog struct {
	readyErr    error
	terminalErr error
}

func (f *failingCatalog) BeginCreate(world.ID) error                  { return nil }
func (f *failingCatalog) Ready(world.ID, substrate.Handle) error      { return f.readyErr }
func (f *failingCatalog) Terminal(world.ID) error                     { return f.terminalErr }
func (f *failingCatalog) Quarantine(world.ID, substrate.Handle) error { return f.terminalErr }
func (f *failingCatalog) Destroyed(world.ID) error                    { return nil }
func (f *failingCatalog) Get(world.ID) (CatalogEntry, bool)           { return CatalogEntry{}, false }
func (f *failingCatalog) Entries() map[world.ID]CatalogEntry          { return map[world.ID]CatalogEntry{} }
func (f *failingCatalog) Close() error                                { return nil }

type memoryFactoryForTest struct{}

func (memoryFactoryForTest) Create(id world.ID) (world.AuthorityControl, error) {
	return world.NewState(id)
}
func (memoryFactoryForTest) Open(world.ID) (world.AuthorityControl, error) {
	return nil, world.ErrStateNotFound
}

func TestReadyPersistenceFailureDoesNotDestroyBeforeTerminalIsDurable(t *testing.T) {
	var destroys int
	fs := &fakeSubstrate{destroyFn: func(substrate.Handle) error { destroys++; return nil }}
	catalog := &failingCatalog{readyErr: errors.New("disk full"), terminalErr: errors.New("still full")}
	m, _, err := NewPersistentManager(fs, memoryFactoryForTest{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.Create(context.Background(), "w"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if destroys != 0 {
		t.Fatalf("destroy called %d times before terminal state was durable", destroys)
	}
	st, ok := m.AuthorityState("w")
	if !ok {
		t.Fatal("authority state missing after ready persistence failure")
	}
	if err := st.ValidateEpoch(st.CurrentEpoch()); !errors.Is(err, world.ErrClosedWorld) {
		t.Fatalf("authority not closed after ready persistence failure: %v", err)
	}
}

func TestManagerCloseWaitsForInFlightCreate(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fs := &fakeSubstrate{createFn: func(id world.ID) (substrate.Handle, error) {
		close(started)
		<-release
		return substrate.Handle("h-" + string(id)), nil
	}}
	m, _ := NewManager(fs)
	createDone := make(chan error, 1)
	go func() { _, err := m.Create(context.Background(), "w"); createDone <- err }()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- m.Close() }()
	select {
	case <-closeDone:
		t.Fatal("Manager.Close returned while Create was still in flight")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-createDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}

type faultAuthorityState struct {
	id        world.ID
	epoch     world.Epoch
	closeErrs int
	closed    bool
}

func (s *faultAuthorityState) ID() world.ID              { return s.id }
func (s *faultAuthorityState) CurrentEpoch() world.Epoch { return s.epoch }
func (s *faultAuthorityState) Closed() bool              { return s.closed }
func (s *faultAuthorityState) ValidateEpoch(e world.Epoch) error {
	if s.closed {
		return world.ErrClosedWorld
	}
	if e < s.epoch {
		return world.ErrStaleEpoch
	}
	if e > s.epoch || e == 0 {
		return world.ErrInvalidEpoch
	}
	return nil
}
func (s *faultAuthorityState) WithEpoch(e world.Epoch, fn func() error) error {
	if err := s.ValidateEpoch(e); err != nil {
		return err
	}
	return fn()
}
func (s *faultAuthorityState) AdvanceAuthority() (world.Epoch, error) {
	if s.closed {
		return s.epoch, world.ErrClosedWorld
	}
	s.epoch++
	return s.epoch, nil
}
func (s *faultAuthorityState) CloseAuthority() (world.Epoch, error) {
	if s.closeErrs > 0 {
		s.closeErrs--
		return s.epoch, errors.New("authority journal unavailable")
	}
	if !s.closed {
		s.epoch++
		s.closed = true
	}
	return s.epoch, nil
}
func (s *faultAuthorityState) Release() error { return nil }

type fixedAuthorityFactory struct{ state *faultAuthorityState }

func (f fixedAuthorityFactory) Create(id world.ID) (world.AuthorityControl, error) {
	f.state.id = id
	if f.state.epoch == 0 {
		f.state.epoch = 1
	}
	return f.state, nil
}
func (f fixedAuthorityFactory) Open(id world.ID) (world.AuthorityControl, error) {
	if f.state.id != id {
		return nil, world.ErrStateNotFound
	}
	return f.state, nil
}

type memoryCatalog struct{ entries map[world.ID]CatalogEntry }

func newMemoryCatalog() *memoryCatalog { return &memoryCatalog{entries: map[world.ID]CatalogEntry{}} }
func (c *memoryCatalog) BeginCreate(id world.ID) error {
	c.entries[id] = CatalogEntry{WorldID: id, Phase: PhaseCreating}
	return nil
}
func (c *memoryCatalog) Ready(id world.ID, h substrate.Handle) error {
	c.entries[id] = CatalogEntry{WorldID: id, Handle: h, Phase: PhaseReady}
	return nil
}
func (c *memoryCatalog) Terminal(id world.ID) error {
	e := c.entries[id]
	e.Phase = PhaseTerminal
	c.entries[id] = e
	return nil
}
func (c *memoryCatalog) Quarantine(id world.ID, h substrate.Handle) error {
	c.entries[id] = CatalogEntry{WorldID: id, Handle: h, Phase: PhaseTerminal}
	return nil
}
func (c *memoryCatalog) Destroyed(id world.ID) error {
	e := c.entries[id]
	e.Phase = PhaseDestroyed
	c.entries[id] = e
	return nil
}
func (c *memoryCatalog) Get(id world.ID) (CatalogEntry, bool) { e, ok := c.entries[id]; return e, ok }
func (c *memoryCatalog) Entries() map[world.ID]CatalogEntry {
	out := map[world.ID]CatalogEntry{}
	for k, v := range c.entries {
		out[k] = v
	}
	return out
}
func (c *memoryCatalog) Close() error { return nil }

func TestBrokerFacingAuthorityIsReadOnlyManagedView(t *testing.T) {
	m, err := NewManager(&fakeSubstrate{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	view, ok := m.AuthorityState("w")
	if !ok {
		t.Fatal("missing authority view")
	}
	if _, mutable := view.(world.AuthorityControl); mutable {
		t.Fatal("broker-facing authority must not expose host mutation methods")
	}
}

func TestDurableLifecycleTerminalFencesExistingViewWhenAuthorityCloseFails(t *testing.T) {
	st := &faultAuthorityState{epoch: 1, closeErrs: 1}
	cat := newMemoryCatalog()
	destroys := 0
	fs := &fakeSubstrate{destroyFn: func(substrate.Handle) error { destroys++; return nil }}
	m, _, err := NewPersistentManager(fs, fixedAuthorityFactory{state: st}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	view, ok := m.AuthorityState("w")
	if !ok {
		t.Fatal("missing authority view")
	}
	epoch := view.CurrentEpoch()
	if err := m.Destroy(context.Background(), "w"); err == nil {
		t.Fatal("expected authority close persistence failure")
	}
	if destroys != 0 {
		t.Fatalf("substrate destroyed before authority close recovered: %d", destroys)
	}
	if err := view.ValidateEpoch(epoch); !errors.Is(err, world.ErrClosedWorld) {
		t.Fatalf("existing broker view remained live after durable terminal fence: %v", err)
	}
	if cat.entries["w"].Phase != PhaseTerminal {
		t.Fatalf("terminal lifecycle was not durable before close retry: %+v", cat.entries["w"])
	}
	if err := m.Destroy(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	if destroys != 1 {
		t.Fatalf("destroy count=%d want 1", destroys)
	}
}

func TestAuthorityViewRejectsAfterManagerClose(t *testing.T) {
	m, err := NewManager(&fakeSubstrate{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	view, ok := m.AuthorityState("w")
	if !ok {
		t.Fatal("missing authority view")
	}
	epoch := view.CurrentEpoch()
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := view.ValidateEpoch(epoch); !errors.Is(err, world.ErrClosedWorld) {
		t.Fatalf("view survived manager shutdown: %v", err)
	}
}
