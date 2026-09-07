package realm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func putV23World(t *testing.T, store *MemoryStore, realmID ID, phase WorldPhase, revision uint64, handle substrate.Handle) world.ID {
	t.Helper()
	id := world.ID("wave23-world")
	if err := store.PutWorld(WorldRecord{
		RealmID:             realmID,
		WorldID:             id,
		RealizationRevision: revision,
		Phase:               phase,
		LeaseGeneration:     1,
		LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
		Handle:              handle,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestV23CurrentRealizationAuthorityBindsHostOwnedRealmAndWorld(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	ctl, err := NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	realmRec, err := ctl.Create(ctx, validSpec())
	if err != nil {
		t.Fatal(err)
	}
	worldID := putV23World(t, store, realmRec.Spec.ID, WorldObservedReady, 7, substrate.Handle("cube-sandbox-a"))

	auth, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
	if err != nil {
		t.Fatalf("CurrentRealizationAuthority: %v", err)
	}
	binding, ok := auth.Binding()
	if !ok {
		t.Fatal("minted authority is invalid")
	}
	wantPolicy, err := PolicyDigest(realmRec.Spec, realmRec.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if binding.RealmID != realmRec.Spec.ID || binding.RealmRevision != realmRec.Revision || binding.PolicyDigest != wantPolicy || binding.WorldID != worldID || binding.RealizationRevision != 7 || binding.SubstrateHandle != substrate.Handle("cube-sandbox-a") {
		t.Fatalf("binding=%+v", binding)
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, auth); err != nil {
		t.Fatalf("fresh authority rejected: %v", err)
	}
}

func TestV23PreReadyAndTerminalWorldsCannotMintAuthority(t *testing.T) {
	for _, phase := range []WorldPhase{WorldRequested, WorldCreating, WorldTerminal} {
		t.Run(string(phase), func(t *testing.T) {
			store := NewMemoryStore()
			ctl, _ := NewController(store)
			realmRec, err := ctl.Create(context.Background(), validSpec())
			if err != nil {
				t.Fatal(err)
			}
			worldID := putV23World(t, store, realmRec.Spec.ID, phase, 1, substrate.Handle("cube-sandbox-a"))
			if _, err := ctl.CurrentRealizationAuthority(context.Background(), realmRec.Spec.ID, worldID); !errors.Is(err, ErrRealizationAuthorityUnavailable) {
				t.Fatalf("phase=%s err=%v, want unavailable", phase, err)
			}
		})
	}
}

func TestV23ZeroAndStaleAuthorityFailClosed(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	ctl, _ := NewController(store)
	if _, err := ctl.ValidateRealizationAuthority(ctx, RealizationAuthority{}); !errors.Is(err, ErrInvalidRealizationAuthority) {
		t.Fatalf("zero authority err=%v", err)
	}

	realmRec, err := ctl.Create(ctx, validSpec())
	if err != nil {
		t.Fatal(err)
	}
	worldID := putV23World(t, store, realmRec.Spec.ID, WorldObservedReady, 1, substrate.Handle("cube-sandbox-a"))
	auth, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
	if err != nil {
		t.Fatal(err)
	}

	changedSpec := realmRec.Spec
	changedSpec.MaxWorlds++
	if _, err := ctl.Update(ctx, realmRec.Spec.ID, realmRec.Revision, changedSpec); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, auth); !errors.Is(err, ErrStaleRealizationAuthority) {
		t.Fatalf("Realm revision drift err=%v", err)
	}
}

func TestV23WorldReRealizationAndHandleDriftStaleAuthority(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	ctl, _ := NewController(store)
	realmRec, err := ctl.Create(ctx, validSpec())
	if err != nil {
		t.Fatal(err)
	}
	worldID := putV23World(t, store, realmRec.Spec.ID, WorldObservedReady, 1, substrate.Handle("cube-sandbox-a"))
	auth, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.PutWorld(WorldRecord{
		RealmID: realmRec.Spec.ID, WorldID: worldID, RealizationRevision: 2,
		Phase: WorldObservedReady, LeaseGeneration: 1,
		LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(), Handle: substrate.Handle("cube-sandbox-b"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, auth); !errors.Is(err, ErrStaleRealizationAuthority) {
		t.Fatalf("re-realization err=%v", err)
	}
}
