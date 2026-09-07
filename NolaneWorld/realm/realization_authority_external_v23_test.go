package realm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type callerOwnedAuthorityStore struct {
	realm.Store
	realmRecord realm.RealmRecord
	worldRecord realm.WorldRecord
}

func (s *callerOwnedAuthorityStore) Realm(id realm.ID) (realm.RealmRecord, bool) {
	if s == nil || s.realmRecord.Spec.ID != id {
		return realm.RealmRecord{}, false
	}
	return s.realmRecord, true
}

func (s *callerOwnedAuthorityStore) World(id realm.ID, worldID world.ID) (realm.WorldRecord, bool) {
	if s == nil || s.worldRecord.RealmID != id || s.worldRecord.WorldID != worldID {
		return realm.WorldRecord{}, false
	}
	return s.worldRecord, true
}

func TestV23CallerOwnedStoreCannotMintPackageAuthority(t *testing.T) {
	spec := realm.Spec{
		ID:             realm.ID("realm://wave23-caller-store"),
		MaxWorlds:      2,
		DefaultLease:   time.Minute,
		NetworkProfile: realm.R0InternalOnly,
		ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048},
	}
	worldID := world.ID("wave23-caller-store-world")
	store := &callerOwnedAuthorityStore{
		Store:       realm.NewMemoryStore(),
		realmRecord: realm.RealmRecord{Spec: spec, Revision: 1},
		worldRecord: realm.WorldRecord{
			RealmID:             spec.ID,
			WorldID:             worldID,
			RealizationRevision: 1,
			Phase:               realm.WorldObservedReady,
			LeaseGeneration:     1,
			LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
			Handle:              substrate.Handle("cube-sandbox-forged-store"),
		},
	}
	ctl, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	if authority, err := ctl.CurrentRealizationAuthority(context.Background(), spec.ID, worldID); !errors.Is(err, realm.ErrRealizationAuthorityUnavailable) {
		t.Fatalf("caller-owned Store minted authority=%+v err=%v, want unavailable", authority, err)
	}
}

func TestV23SerializedOrDescriptiveBindingCannotRestoreAuthority(t *testing.T) {
	ctx := context.Background()
	store := realm.NewMemoryStore()
	ctl, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	spec := realm.Spec{
		ID:             realm.ID("realm://wave23-external-opacity"),
		MaxWorlds:      2,
		DefaultLease:   time.Minute,
		NetworkProfile: realm.R0InternalOnly,
		ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048},
	}
	realmRec, err := ctl.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	worldID := world.ID("wave23-external-world")
	if err := store.PutWorld(realm.WorldRecord{
		RealmID:             realmRec.Spec.ID,
		WorldID:             worldID,
		RealizationRevision: 9,
		Phase:               realm.WorldObservedReady,
		LeaseGeneration:     1,
		LeaseExpiresUnix:    time.Now().Add(time.Hour).Unix(),
		Handle:              substrate.Handle("cube-sandbox-external"),
	}); err != nil {
		t.Fatal(err)
	}
	authority, err := ctl.CurrentRealizationAuthority(ctx, realmRec.Spec.ID, worldID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok {
		t.Fatal("fresh authority has no descriptive binding")
	}

	authorityJSON, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	if string(authorityJSON) != "{}" {
		t.Fatalf("opaque authority serialized as %s, want {}", authorityJSON)
	}
	var restored realm.RealizationAuthority
	if err := json.Unmarshal(authorityJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if _, ok := restored.Binding(); ok {
		t.Fatal("serialized authority restored package-owned seal")
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, restored); !errors.Is(err, realm.ErrInvalidRealizationAuthority) {
		t.Fatalf("serialized authority validation err=%v, want invalid", err)
	}

	bindingJSON, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var reconstructed realm.RealizationAuthority
	if err := json.Unmarshal(bindingJSON, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if _, ok := reconstructed.Binding(); ok {
		t.Fatal("descriptive binding reconstructed package-owned seal")
	}
	if _, err := ctl.ValidateRealizationAuthority(ctx, reconstructed); !errors.Is(err, realm.ErrInvalidRealizationAuthority) {
		t.Fatalf("descriptive reconstruction validation err=%v, want invalid", err)
	}
}
