package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type fakeFabric struct {
	lease fabric.Lease
	handle substrate.Handle
	revision uint64
	handleCalls int
}
func (f *fakeFabric) Acquire(context.Context, fabric.AcquireRequest) (fabric.Lease,error) { return f.lease,nil }
func (f *fakeFabric) Spawn(context.Context, fabric.SpawnRequest) (fabric.Lease,error) { return f.lease,nil }
func (f *fakeFabric) Release(context.Context, realm.ID, world.ID, uint64) error { return nil }
func (f *fakeFabric) Handle(_ realm.ID, _ world.ID, generation uint64) (substrate.Handle,uint64,error) { f.handleCalls++; if generation != f.lease.Generation { return "",0,fabric.ErrStaleLease }; return f.handle,f.revision,nil }
func (f *fakeFabric) Checkpoint(context.Context, realm.ID, world.ID, uint64) (realm.CheckpointRecord,error) { return realm.CheckpointRecord{},errors.New("not used") }
func (f *fakeFabric) Resume(context.Context, realm.CheckpointID, uint64) (fabric.Lease,error) { return fabric.Lease{},errors.New("not used") }

type fakeGuest struct { calls int; err error }
func (g *fakeGuest) Exec(context.Context, substrate.Handle, substrate.ProcessRequest) (substrate.ProcessObservation,error) { g.calls++; if g.err != nil { return substrate.ProcessObservation{},g.err }; return substrate.ProcessObservation{ExitCode:0,Stdout:[]byte("ok"),ObservationDigest:"abcd"},nil }

func runtimeFixture(t *testing.T) (*Service,*fakeFabric,*fakeGuest,Session) {
	t.Helper()
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID:realm.ID("realm://test"),MaxWorlds:2,DefaultLease:time.Minute,NetworkProfile:realm.R0InternalOnly,ResourceBudget:realm.ResourceBudget{CPUUnits:2,MemoryMiB:1024,DiskMiB:2048}}
	if _,err:=store.CreateRealm(spec);err!=nil { t.Fatal(err) }
	ff:=&fakeFabric{lease:fabric.Lease{RealmID:spec.ID,WorldID:world.ID("world-a"),Generation:1,ExpiresUnix:time.Now().Add(time.Minute).Unix(),RealizationRevision:1},handle:"private-handle",revision:1}
	fg:=&fakeGuest{}
	svc,err:=New(store,ff,fg);if err!=nil { t.Fatal(err) }
	sess,err:=svc.Enter(context.Background(),EnterRequest{RealmID:spec.ID,ExpectedRevision:1,PolicyDigest:"policy-v1"});if err!=nil { t.Fatal(err) }
	return svc,ff,fg,sess
}

func TestExecExactReplayAndChangedPayloadCollision(t *testing.T) {
	svc,ff,guest,sess:=runtimeFixture(t)
	req:=ExecRequest{SessionID:sess.ID,RealmRevision:sess.RealmRevision,WorldID:world.ID("world-a"),LeaseGeneration:1,ActionID:"exec-1",Command:"printf ok",Timeout:time.Second,MaxOutputBytes:1024}
	first,err:=svc.Exec(context.Background(),req);if err!=nil { t.Fatal(err) }
	second,err:=svc.Exec(context.Background(),req);if err!=nil { t.Fatal(err) }
	if first!=second { t.Fatalf("replay changed receipt: %+v %+v",first,second) }
	if guest.calls!=1 || ff.handleCalls!=1 { t.Fatalf("calls guest=%d handle=%d",guest.calls,ff.handleCalls) }
	changed:=req;changed.Command="printf changed"
	if _,err:=svc.Exec(context.Background(),changed);!errors.Is(err,ErrOperationCollision) { t.Fatalf("collision err=%v",err) }
}

func TestExecFailureIsUncertainAndNotReplayed(t *testing.T) {
	svc,_,guest,sess:=runtimeFixture(t)
	guest.err=errors.New("transport after entry")
	req:=ExecRequest{SessionID:sess.ID,RealmRevision:sess.RealmRevision,WorldID:world.ID("world-a"),LeaseGeneration:1,ActionID:"exec-1",Command:"do thing",Timeout:time.Second,MaxOutputBytes:1024}
	if _,err:=svc.Exec(context.Background(),req);!errors.Is(err,ErrExecUncertain) { t.Fatalf("first err=%v",err) }
	if _,err:=svc.Exec(context.Background(),req);!errors.Is(err,ErrExecUncertain) { t.Fatalf("replay err=%v",err) }
	if guest.calls!=1 { t.Fatalf("guest calls=%d want 1",guest.calls) }
}

func TestStaleRealmSessionDeniedBeforeGuest(t *testing.T) {
	svc,_,guest,sess:=runtimeFixture(t)
	req:=ExecRequest{SessionID:sess.ID,RealmRevision:sess.RealmRevision+1,WorldID:world.ID("world-a"),LeaseGeneration:1,ActionID:"exec-1",Command:"x",Timeout:time.Second,MaxOutputBytes:1024}
	if _,err:=svc.Exec(context.Background(),req);!errors.Is(err,ErrStaleSession) { t.Fatalf("err=%v",err) }
	if guest.calls!=0 { t.Fatalf("guest calls=%d",guest.calls) }
}
