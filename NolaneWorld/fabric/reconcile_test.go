package fabric

import (
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestFabricOwnsExplicitRecoveryFenceWithoutRewritingDurableHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := realm.OpenDurableStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := realm.Spec{ID: realm.ID("realm://recovery"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	wr := realm.WorldRecord{RealmID: spec.ID, WorldID: world.ID("world-a"), RealizationRevision: 7, Phase: realm.WorldLeased, LeaseGeneration: 4, LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle("sandbox-private")}
	if err := store.PutWorld(wr); err != nil {
		t.Fatal(err)
	}
	registry, err := realm.NewServiceRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	service, err := registry.Register(realm.ServiceRequest{RealmID: spec.ID, WorldID: wr.WorldID, RealizationRevision: wr.RealizationRevision, Name: "api", Protocol: realm.ServiceHTTP, Port: 8080, Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	recovered, err := realm.OpenDurableStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	capacity := NewCapacity()
	capacity.Observe(spec.ResourceBudget)
	local, err := NewLocal(recovered, newFakeManager(), capacity, NewLeaseBook(), NewBaselineCatalog())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := recovered.World(spec.ID, wr.WorldID)
	if before.Phase != realm.WorldLeased || before.Handle != wr.Handle {
		t.Fatalf("store did not replay durable history before fabric recovery: %+v", before)
	}
	if err := local.FenceRecoveredRealizations(); err != nil {
		t.Fatal(err)
	}
	after, _ := recovered.World(spec.ID, wr.WorldID)
	if after.Phase != realm.WorldCreating || after.Handle != "" {
		t.Fatalf("fabric recovery failed to fence stale realization: %+v", after)
	}
	staleService, ok := recovered.Service(service.ID)
	if !ok || staleService.Ready {
		t.Fatalf("service readiness survived recovery fence: %+v ok=%v", staleService, ok)
	}
	if after.LeaseGeneration != wr.LeaseGeneration || after.RealizationRevision != wr.RealizationRevision {
		t.Fatalf("recovery changed semantic fences: after=%+v before=%+v", after, wr)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}

	replayed, err := realm.OpenDurableStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer replayed.Close()
	historical, _ := replayed.World(spec.ID, wr.WorldID)
	historicalService, _ := replayed.Service(service.ID)
	if historical.Phase != realm.WorldLeased || historical.Handle != wr.Handle || !historicalService.Ready {
		t.Fatalf("fabric recovery projection mutated journal history: world=%+v service=%+v", historical, historicalService)
	}
}
