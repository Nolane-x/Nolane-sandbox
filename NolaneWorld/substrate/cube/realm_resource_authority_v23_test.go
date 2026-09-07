package cube

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func v23RealmSpec() realm.Spec {
	return realm.Spec{
		ID: realm.ID("realm://wave23-cube"), MaxWorlds: 2, DefaultLease: time.Minute,
		NetworkProfile: realm.R0InternalOnly,
		ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048},
	}
}

func mintV23RealmAuthority(t *testing.T, sandboxID string) (*realm.Controller, *realm.MemoryStore, realm.RealizationAuthority, world.ID) {
	t.Helper()
	ctx := context.Background()
	store := realm.NewMemoryStore()
	ctl, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ctl.Create(ctx, v23RealmSpec())
	if err != nil {
		t.Fatal(err)
	}
	worldID := world.ID("wave23-cube-world")
	if err := store.PutWorld(realm.WorldRecord{
		RealmID: rec.Spec.ID, WorldID: worldID, RealizationRevision: 11,
		Phase: realm.WorldObservedReady, LeaseGeneration: 1,
		LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle(sandboxID),
	}); err != nil {
		t.Fatal(err)
	}
	auth, err := ctl.CurrentRealizationAuthority(ctx, rec.Spec.ID, worldID)
	if err != nil {
		t.Fatal(err)
	}
	return ctl, store, auth, worldID
}

func TestV23ExactRealmCubeSandboxBindingMintsCrossBoundaryAuthority(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	resource := ResourceBinding{sandboxID: "sandbox-exact"}

	auth, err := ValidateRealmResourceAuthority(context.Background(), ctl, realmAuth, resource)
	if err != nil {
		t.Fatalf("ValidateRealmResourceAuthority: %v", err)
	}
	if !auth.Valid() {
		t.Fatal("exact binding returned invalid authority")
	}
	binding, ok := auth.RealmBinding()
	if !ok || binding.SubstrateHandle != substrate.Handle("sandbox-exact") {
		t.Fatalf("realm binding=%+v ok=%v", binding, ok)
	}
}

func TestV23WrongCubeSandboxCannotMintCrossBoundaryAuthority(t *testing.T) {
	ctl, _, realmAuth, _ := mintV23RealmAuthority(t, "sandbox-exact")
	if _, err := ValidateRealmResourceAuthority(context.Background(), ctl, realmAuth, ResourceBinding{sandboxID: "sandbox-other"}); !errors.Is(err, ErrRealmResourceAuthorityMismatch) {
		t.Fatalf("sandbox mismatch err=%v", err)
	}
	if _, err := ValidateRealmResourceAuthority(context.Background(), ctl, realmAuth, ResourceBinding{}); !errors.Is(err, ErrInvalidResourceBinding) {
		t.Fatalf("zero resource binding err=%v", err)
	}
}

func TestV23StaleRealmAuthorityCannotSurviveCubeValidation(t *testing.T) {
	ctl, store, realmAuth, worldID := mintV23RealmAuthority(t, "sandbox-exact")
	binding, ok := realmAuth.Binding()
	if !ok {
		t.Fatal("invalid realm authority")
	}
	if err := store.PutWorld(realm.WorldRecord{
		RealmID: binding.RealmID, WorldID: worldID, RealizationRevision: binding.RealizationRevision + 1,
		Phase: realm.WorldObservedReady, LeaseGeneration: 1,
		LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle("sandbox-exact"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRealmResourceAuthority(context.Background(), ctl, realmAuth, ResourceBinding{sandboxID: "sandbox-exact"}); !errors.Is(err, realm.ErrStaleRealizationAuthority) {
		t.Fatalf("stale Realm authority err=%v", err)
	}
}
