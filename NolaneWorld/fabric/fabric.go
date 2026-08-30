package fabric

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var (
	ErrInvalidFabric      = errors.New("fabric: invalid local fabric")
	ErrStaleRealmRevision = errors.New("fabric: stale realm revision")
	ErrWorldLimit         = errors.New("fabric: realm world limit reached")
	ErrWorldTerminal      = errors.New("fabric: world terminal")
	ErrOutcomeUncertain   = errors.New("fabric: outcome uncertain")
	ErrWorldUnavailable   = errors.New("fabric: world unavailable")
)

type WorldManager interface {
	Create(context.Context, world.ID) (substrate.Handle, error)
	Snapshot(context.Context, world.ID) (substrate.Snapshot, error)
	Rollback(context.Context, world.ID, substrate.Snapshot) error
	Clone(context.Context, world.ID, substrate.Snapshot, world.ID) (substrate.Handle, error)
	Destroy(context.Context, world.ID) error
	AuthorityState(world.ID) (world.AuthorityState, bool)
}

type AcquireRequest struct {
	RealmID       realm.ID             `json:"realm_id"`
	RealmRevision uint64               `json:"realm_revision"`
	WorldID       world.ID             `json:"world_id"`
	OperationID   string               `json:"operation_id"`
	Units         realm.ResourceBudget `json:"units"`
	ExpiresUnix   int64                `json:"expires_unix"`
}

type SpawnRequest = AcquireRequest

type Local struct {
	mu        sync.Mutex
	store     realm.Store
	manager   WorldManager
	capacity  *Capacity
	leases    *LeaseBook
	baselines *BaselineCatalog
	now       func() time.Time
}

func NewLocal(store realm.Store, manager WorldManager, capacity *Capacity, leases *LeaseBook, baselines *BaselineCatalog) (*Local, error) {
	if store == nil || manager == nil || capacity == nil || leases == nil || baselines == nil {
		return nil, ErrInvalidFabric
	}
	return &Local{store: store, manager: manager, capacity: capacity, leases: leases, baselines: baselines, now: time.Now}, nil
}

func (f *Local) Acquire(ctx context.Context, req AcquireRequest) (Lease, error) {
	if f == nil || f.store == nil || req.RealmID == "" || req.WorldID == "" || req.OperationID == "" || !req.Units.Valid() || req.ExpiresUnix <= 0 {
		return Lease{}, ErrInvalidFabric
	}
	if err := ctx.Err(); err != nil { return Lease{}, err }
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.store.Realm(req.RealmID)
	if !ok { return Lease{}, realm.ErrRealmNotFound }
	if rec.Closed { return Lease{}, realm.ErrRealmClosed }
	if rec.Revision != req.RealmRevision { return Lease{}, ErrStaleRealmRevision }
	digest := acquireDigest(req)
	if op, ok := f.store.Operation(req.RealmID, req.OperationID); ok {
		if op.RequestDigest != digest { return Lease{}, ErrOperationCollision }
		switch op.Status {
		case "completed":
			wr, ok := f.store.World(req.RealmID, req.WorldID)
			if !ok || wr.Phase == realm.WorldTerminal { return Lease{}, ErrWorldUnavailable }
			l := Lease{RealmID: wr.RealmID, WorldID: wr.WorldID, Generation: wr.LeaseGeneration, ExpiresUnix: wr.LeaseExpiresUnix, RealizationRevision: wr.RealizationRevision}
			_ = f.leases.Restore(l)
			return l, nil
		case "pending", "uncertain":
			return Lease{}, ErrOutcomeUncertain
		default:
			return Lease{}, ErrOutcomeUncertain
		}
	}
	active := uint32(0)
	for _, wr := range f.store.Worlds(req.RealmID) {
		if wr.Phase != realm.WorldTerminal { active++ }
	}
	if active >= rec.Spec.MaxWorlds { return Lease{}, ErrWorldLimit }
	if _, exists := f.store.World(req.RealmID, req.WorldID); exists { return Lease{}, realm.ErrInvalidWorld }
	res, err := f.capacity.Reserve(ReservationRequest{OperationID:req.OperationID,RealmID:req.RealmID,Units:req.Units,ExpiresUnix:req.ExpiresUnix})
	if err != nil { return Lease{}, err }
	baselineID := ""
	if b, ok := f.baselines.Select(rec.Spec.NetworkProfile); ok { baselineID = b.ID }
	wr := realm.WorldRecord{RealmID:req.RealmID,WorldID:req.WorldID,RealizationRevision:1,Phase:realm.WorldCreating,LeaseGeneration:1,LeaseExpiresUnix:req.ExpiresUnix,AcquireOperationID:req.OperationID,BaselineID:baselineID}
	if err := f.store.PutWorld(wr); err != nil { _ = f.capacity.Release(req.OperationID); return Lease{}, err }
	if err := f.store.RecordOperation(realm.OperationRecord{RealmID:req.RealmID,OperationID:req.OperationID,RequestDigest:digest,Status:"pending",ReceiptDigest:res.ID}); err != nil { _ = f.capacity.Release(req.OperationID); return Lease{}, err }
	h, createErr := f.manager.Create(ctx, req.WorldID)
	if createErr != nil || h == "" {
		_ = f.store.RecordOperation(realm.OperationRecord{RealmID:req.RealmID,OperationID:req.OperationID,RequestDigest:digest,Status:"uncertain",ReceiptDigest:res.ID})
		return Lease{}, ErrOutcomeUncertain
	}
	lease, err := f.leases.Issue(req.RealmID, req.WorldID, 1, req.ExpiresUnix)
	if err != nil { return Lease{}, ErrOutcomeUncertain }
	wr.Handle = h
	wr.Phase = realm.WorldLeased
	wr.LeaseGeneration = lease.Generation
	if err := f.store.PutWorld(wr); err != nil { return Lease{}, ErrOutcomeUncertain }
	if err := f.store.RecordOperation(realm.OperationRecord{RealmID:req.RealmID,OperationID:req.OperationID,RequestDigest:digest,Status:"completed",ReceiptDigest:leaseDigest(lease)}); err != nil { return Lease{}, ErrOutcomeUncertain }
	return lease, nil
}

func (f *Local) Spawn(ctx context.Context, req SpawnRequest) (Lease, error) { return f.Acquire(ctx, AcquireRequest(req)) }

func (f *Local) ValidateLease(l Lease, nowUnix int64) error { return f.leases.Validate(l, nowUnix) }

func (f *Local) Handle(realmID realm.ID, worldID world.ID, generation uint64) (substrate.Handle, uint64, error) {
	if f == nil { return "", 0, ErrInvalidFabric }
	f.mu.Lock(); defer f.mu.Unlock()
	wr, ok := f.store.World(realmID, worldID)
	if !ok { return "", 0, ErrWorldUnavailable }
	if wr.Phase == realm.WorldTerminal { return "", 0, ErrWorldTerminal }
	if wr.Phase != realm.WorldLeased || wr.Handle == "" { return "", 0, ErrWorldUnavailable }
	l := Lease{RealmID:wr.RealmID,WorldID:wr.WorldID,Generation:wr.LeaseGeneration,ExpiresUnix:wr.LeaseExpiresUnix,RealizationRevision:wr.RealizationRevision}
	if _, ok := f.leases.Current(realmID, worldID); !ok { _ = f.leases.Restore(l) }
	if generation != l.Generation { return "", 0, ErrStaleLease }
	if err := f.leases.Validate(l, f.now().Unix()); err != nil { return "", 0, err }
	return wr.Handle, wr.RealizationRevision, nil
}

func (f *Local) Checkpoint(ctx context.Context, realmID realm.ID, worldID world.ID, generation uint64) (realm.CheckpointRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	wr, ok := f.store.World(realmID, worldID)
	if !ok || wr.Phase != realm.WorldLeased { return realm.CheckpointRecord{}, ErrWorldUnavailable }
	l := Lease{RealmID:wr.RealmID,WorldID:wr.WorldID,Generation:wr.LeaseGeneration,ExpiresUnix:wr.LeaseExpiresUnix,RealizationRevision:wr.RealizationRevision}
	if _, ok := f.leases.Current(realmID, worldID); !ok { _ = f.leases.Restore(l) }
	if generation != l.Generation { return realm.CheckpointRecord{}, ErrStaleLease }
	if err := f.leases.Validate(l, f.now().Unix()); err != nil { return realm.CheckpointRecord{}, err }
	snap, err := f.manager.Snapshot(ctx, worldID)
	if err != nil || snap == "" { return realm.CheckpointRecord{}, ErrOutcomeUncertain }
	state, ok := f.manager.AuthorityState(worldID)
	if !ok || state.CurrentEpoch() == 0 { return realm.CheckpointRecord{}, ErrOutcomeUncertain }
	rec, ok := f.store.Realm(realmID)
	if !ok { return realm.CheckpointRecord{}, realm.ErrRealmNotFound }
	cp := realm.CheckpointRecord{ID:checkpointID(realmID,worldID,wr.RealizationRevision,state.CurrentEpoch(),snap),RealmID:realmID,RealmRevision:rec.Revision,WorldID:worldID,RealizationRevision:wr.RealizationRevision,AuthorityEpoch:state.CurrentEpoch(),Snapshot:snap,CapabilityDigest:wr.CapabilityDigest}
	if err := f.store.PutCheckpoint(cp); err != nil { return realm.CheckpointRecord{}, err }
	return cp, nil
}

func (f *Local) Resume(ctx context.Context, checkpoint realm.CheckpointID, expectedRealmRevision uint64) (Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp, ok := f.store.Checkpoint(checkpoint)
	if !ok { return Lease{}, realm.ErrInvalidCheckpoint }
	rr, ok := f.store.Realm(cp.RealmID)
	if !ok { return Lease{}, realm.ErrRealmNotFound }
	if rr.Closed { return Lease{}, realm.ErrRealmClosed }
	if rr.Revision != expectedRealmRevision || rr.Revision != cp.RealmRevision { return Lease{}, ErrStaleRealmRevision }
	wr, ok := f.store.World(cp.RealmID, cp.WorldID)
	if !ok || wr.Phase == realm.WorldTerminal { return Lease{}, ErrWorldTerminal }
	if err := f.manager.Rollback(ctx, cp.WorldID, cp.Snapshot); err != nil { return Lease{}, ErrOutcomeUncertain }
	state, ok := f.manager.AuthorityState(cp.WorldID)
	if !ok || state.CurrentEpoch() <= cp.AuthorityEpoch { return Lease{}, ErrOutcomeUncertain }
	wr.RealizationRevision++
	lease, err := f.leases.Issue(cp.RealmID, cp.WorldID, wr.RealizationRevision, f.now().Add(rr.Spec.DefaultLease).Unix())
	if err != nil { return Lease{}, err }
	wr.Phase = realm.WorldLeased
	wr.LeaseGeneration = lease.Generation
	wr.LeaseExpiresUnix = lease.ExpiresUnix
	if err := f.store.PutWorld(wr); err != nil { return Lease{}, err }
	return lease, nil
}

func (f *Local) Release(ctx context.Context, realmID realm.ID, worldID world.ID, generation uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	wr, ok := f.store.World(realmID, worldID)
	if !ok { return ErrWorldUnavailable }
	if wr.Phase == realm.WorldTerminal { return nil }
	l := Lease{RealmID:wr.RealmID,WorldID:wr.WorldID,Generation:wr.LeaseGeneration,ExpiresUnix:wr.LeaseExpiresUnix,RealizationRevision:wr.RealizationRevision}
	if _, ok := f.leases.Current(realmID, worldID); !ok { _ = f.leases.Restore(l) }
	if generation != l.Generation { return ErrStaleLease }
	if err := f.manager.Destroy(ctx, worldID); err != nil { return ErrOutcomeUncertain }
	wr.Phase = realm.WorldTerminal
	if err := f.store.PutWorld(wr); err != nil { return err }
	if wr.AcquireOperationID != "" { _ = f.capacity.Release(wr.AcquireOperationID) }
	return nil
}

func acquireDigest(req AcquireRequest) string {
	raw, _ := json.Marshal(req)
	h := sha256.Sum256(append([]byte("nolane.fabric.acquire.v1\x00"), raw...))
	return hex.EncodeToString(h[:])
}
func leaseDigest(l Lease) string { raw,_:=json.Marshal(l); h:=sha256.Sum256(append([]byte("nolane.fabric.lease.v1\x00"),raw...)); return hex.EncodeToString(h[:]) }
func checkpointID(r realm.ID, w world.ID, rev uint64, epoch world.Epoch, snap substrate.Snapshot) realm.CheckpointID { raw,_:=json.Marshal([]any{r,w,rev,epoch,snap}); h:=sha256.Sum256(append([]byte("nolane.realm.checkpoint.v1\x00"),raw...)); return realm.CheckpointID("checkpoint://"+hex.EncodeToString(h[:])) }
