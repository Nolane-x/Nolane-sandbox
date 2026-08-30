package fabric

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeManager struct {
	creates []world.ID
	states map[world.ID]world.AuthorityControl
	snaps map[world.ID]substrate.Snapshot
}
func newFakeManager() *fakeManager { return &fakeManager{states: map[world.ID]world.AuthorityControl{}, snaps: map[world.ID]substrate.Snapshot{}} }
func (m *fakeManager) Create(_ context.Context, id world.ID) (substrate.Handle, error) { m.creates = append(m.creates, id); st, err := world.NewState(id); if err != nil { return "", err }; m.states[id] = st; return substrate.Handle("handle-"+string(id)), nil }
func (m *fakeManager) Snapshot(_ context.Context, id world.ID) (substrate.Snapshot, error) { s := substrate.Snapshot("snap-"+string(id)); m.snaps[id] = s; return s, nil }
func (m *fakeManager) Rollback(_ context.Context, id world.ID, snap substrate.Snapshot) error { if m.snaps[id] != snap { return errors.New("wrong snapshot") }; _, err := m.states[id].AdvanceAuthority(); return err }
func (m *fakeManager) Clone(_ context.Context, _ world.ID, _ substrate.Snapshot, child world.ID) (substrate.Handle, error) { return m.Create(context.Background(), child) }
func (m *fakeManager) Destroy(_ context.Context, id world.ID) error { _, err := m.states[id].CloseAuthority(); return err }
func (m *fakeManager) AuthorityState(id world.ID) (world.AuthorityState, bool) { s, ok := m.states[id]; return s, ok }

func testLocal(t *testing.T) (*Local, *realm.MemoryStore, *fakeManager) {
	t.Helper()
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://test"), MaxWorlds: 4, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 4, MemoryMiB: 4096, DiskMiB: 8192}}
	if _, err := store.CreateRealm(spec); err != nil { t.Fatal(err) }
	cap := NewCapacity(); cap.Observe(spec.ResourceBudget)
	bases := NewBaselineCatalog(); if err := bases.Admit(Baseline{ID:"clean",Digest:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",TemplateRef:"clean-template",NetworkProfile:realm.R0InternalOnly,Sanitized:true}); err != nil { t.Fatal(err) }
	mgr := newFakeManager()
	local, err := NewLocal(store, mgr, cap, NewLeaseBook(), bases)
	if err != nil { t.Fatal(err) }
	return local, store, mgr
}

func TestAcquireCreatesFreshExactWorldAndReplaysIdempotently(t *testing.T) {
	local, _, mgr := testLocal(t)
	now := time.Now().Unix()
	req := AcquireRequest{RealmID:realm.ID("realm://test"),RealmRevision:1,WorldID:world.ID("world-a"),OperationID:"acquire-a",Units:realm.ResourceBudget{CPUUnits:1,MemoryMiB:512,DiskMiB:1024},ExpiresUnix:now+60}
	first, err := local.Acquire(context.Background(), req)
	if err != nil { t.Fatal(err) }
	second, err := local.Acquire(context.Background(), req)
	if err != nil { t.Fatal(err) }
	if first != second { t.Fatalf("replay changed lease: %+v %+v", first, second) }
	if len(mgr.creates) != 1 || mgr.creates[0] != req.WorldID { t.Fatalf("creates=%v", mgr.creates) }
	changed:=req; changed.Units.MemoryMiB++
	if _,err:=local.Acquire(context.Background(),changed);!errors.Is(err,ErrOperationCollision){t.Fatalf("collision err=%v",err)}
}

func TestAcquireEnforcesAggregateRealmResourceBudgetBeforeRealization(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://budget"), MaxWorlds: 4, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil { t.Fatal(err) }
	cap := NewCapacity()
	cap.Observe(realm.ResourceBudget{CPUUnits: 8, MemoryMiB: 8192, DiskMiB: 16384})
	mgr := newFakeManager()
	local, err := NewLocal(store, mgr, cap, NewLeaseBook(), NewBaselineCatalog())
	if err != nil { t.Fatal(err) }
	now := time.Now().Unix()
	units := realm.ResourceBudget{CPUUnits: 1, MemoryMiB: 768, DiskMiB: 1024}
	first, err := local.Acquire(context.Background(), AcquireRequest{RealmID: spec.ID, RealmRevision: 1, WorldID: world.ID("world-a"), OperationID: "budget-a", Units: units, ExpiresUnix: now + 60})
	if err != nil { t.Fatal(err) }
	if _, err := local.Acquire(context.Background(), AcquireRequest{RealmID: spec.ID, RealmRevision: 1, WorldID: world.ID("world-b"), OperationID: "budget-b", Units: units, ExpiresUnix: now + 60}); !errors.Is(err, ErrRealmBudgetExceeded) {
		t.Fatalf("over-budget admission err=%v", err)
	}
	if len(mgr.creates) != 1 { t.Fatalf("over-budget request entered realization: creates=%v", mgr.creates) }
	if err := local.Release(context.Background(), spec.ID, first.WorldID, first.Generation); err != nil { t.Fatal(err) }
	if _, err := local.Acquire(context.Background(), AcquireRequest{RealmID: spec.ID, RealmRevision: 1, WorldID: world.ID("world-b"), OperationID: "budget-b", Units: units, ExpiresUnix: now + 60}); err != nil {
		t.Fatalf("released Realm budget was not reclaimed: %v", err)
	}
}

func TestCheckpointResumeAdvancesAuthorityBeforeNewLease(t *testing.T) {
	local, _, mgr := testLocal(t)
	now := time.Now().Unix()
	lease, err := local.Acquire(context.Background(), AcquireRequest{RealmID:realm.ID("realm://test"),RealmRevision:1,WorldID:world.ID("world-a"),OperationID:"acquire-a",Units:realm.ResourceBudget{CPUUnits:1,MemoryMiB:512,DiskMiB:1024},ExpiresUnix:now+60})
	if err != nil { t.Fatal(err) }
	cp, err := local.Checkpoint(context.Background(), lease.RealmID, lease.WorldID, lease.Generation)
	if err != nil { t.Fatal(err) }
	before := cp.AuthorityEpoch
	resumed, err := local.Resume(context.Background(), cp.ID, 1)
	if err != nil { t.Fatal(err) }
	state, _ := mgr.AuthorityState(lease.WorldID)
	if state.CurrentEpoch() <= before { t.Fatalf("epoch=%d before=%d", state.CurrentEpoch(), before) }
	if resumed.Generation <= lease.Generation || resumed.RealizationRevision <= lease.RealizationRevision { t.Fatalf("resumed=%+v old=%+v", resumed, lease) }
	if err := local.ValidateLease(lease, time.Now().Unix()); !errors.Is(err, ErrStaleLease) { t.Fatalf("old lease err=%v", err) }
}
