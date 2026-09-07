package realm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestV23EstablishedHandleCannotRebindWithoutRealizationAdvance(t *testing.T) {
	factories := map[string]func(t *testing.T) Store{
		"memory": func(t *testing.T) Store {
			t.Helper()
			return NewMemoryStore()
		},
		"durable": func(t *testing.T) Store {
			t.Helper()
			store, err := OpenDurableStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
	}

	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := factory(t)
			t.Cleanup(func() { _ = store.Close() })
			ctl, err := NewController(store)
			if err != nil {
				t.Fatal(err)
			}
			realmRec, err := ctl.Create(ctx, validSpec())
			if err != nil {
				t.Fatal(err)
			}
			worldID := world.ID("wave23-handle-lifecycle")
			base := WorldRecord{
				RealmID:             realmRec.Spec.ID,
				WorldID:             worldID,
				RealizationRevision: 1,
				Phase:               WorldObservedReady,
				LeaseGeneration:     1,
				LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
				Handle:              substrate.Handle("cube-sandbox-a"),
			}
			if err := store.PutWorld(base); err != nil {
				t.Fatal(err)
			}
			authority, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
			if err != nil {
				t.Fatal(err)
			}

			sameRevisionRebind := base
			sameRevisionRebind.Handle = substrate.Handle("cube-sandbox-b")
			if err := store.PutWorld(sameRevisionRebind); !errors.Is(err, ErrInvalidWorld) {
				t.Fatalf("same realization revision handle rebind err=%v, want ErrInvalidWorld", err)
			}
			if _, err := ctl.ValidateRealizationAuthority(ctx, authority); err != nil {
				t.Fatalf("rejected transition changed current authority: %v", err)
			}

			nextRealization := base
			nextRealization.RealizationRevision = 2
			nextRealization.Handle = substrate.Handle("cube-sandbox-b")
			if err := store.PutWorld(nextRealization); err != nil {
				t.Fatalf("advanced realization handle rebind rejected: %v", err)
			}
			if _, err := ctl.ValidateRealizationAuthority(ctx, authority); !errors.Is(err, ErrStaleRealizationAuthority) {
				t.Fatalf("old authority after realization advance err=%v, want stale", err)
			}
		})
	}
}

func TestV23LeasedAndPausedWorldsCanMintAuthority(t *testing.T) {
	for _, phase := range []WorldPhase{WorldLeased, WorldPaused} {
		t.Run(string(phase), func(t *testing.T) {
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
			worldID := putV23World(t, store, realmRec.Spec.ID, phase, 3, substrate.Handle("cube-sandbox-live"))
			authority, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
			if err != nil {
				t.Fatalf("phase=%s mint err=%v", phase, err)
			}
			if _, err := ctl.ValidateRealizationAuthority(ctx, authority); err != nil {
				t.Fatalf("phase=%s validate err=%v", phase, err)
			}
		})
	}
}

func TestV23TerminalTransitionStalesMintedAuthority(t *testing.T) {
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
	worldID := putV23World(t, store, realmRec.Spec.ID, WorldObservedReady, 5, substrate.Handle("cube-sandbox-terminal"))
	authority, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
	if err != nil {
		t.Fatal(err)
	}
	worldRec, ok := store.World(realmRec.Spec.ID, worldID)
	if !ok {
		t.Fatal("world disappeared")
	}
	worldRec.Phase = WorldTerminal
	if err := store.PutWorld(worldRec); err != nil {
		t.Fatalf("terminal transition rejected: %v", err)
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, authority); !errors.Is(err, ErrStaleRealizationAuthority) {
		t.Fatalf("terminal authority err=%v, want stale", err)
	}
}
