package realm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

type callerOwnedV31Store struct {
	realm.Store
	realmRecord realm.RealmRecord
}

type embeddedV31Store struct {
	*realm.MemoryStore
}

func (s *callerOwnedV31Store) Realm(id realm.ID) (realm.RealmRecord, bool) {
	if s == nil || s.realmRecord.Spec.ID != id {
		return realm.RealmRecord{}, false
	}
	return s.realmRecord, true
}

func externalV31Spec(id realm.ID, limit uint64) realm.Spec {
	return realm.Spec{
		ID:                      id,
		MaxWorlds:               2,
		DefaultLease:            time.Minute,
		NetworkProfile:          realm.R0InternalOnly,
		ResourceBudget:          realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048},
		RuntimeCPULimitMilliCPU: limit,
	}
}

func TestV31CallerOwnedStoreCannotMintAuthority(t *testing.T) {
	spec := externalV31Spec(realm.ID("realm://wave31-caller-store"), 500)
	store := &callerOwnedV31Store{
		Store:       realm.NewMemoryStore(),
		realmRecord: realm.RealmRecord{Spec: spec, Revision: 1},
	}
	ctl, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(context.Background(), spec.ID)
	if !errors.Is(err, realm.ErrRuntimeCPUPolicyUnavailable) {
		t.Fatalf("caller-owned Store minted authority=%+v err=%v, want unavailable", authority, err)
	}
}

func TestV31EmbeddedPackageStoreCannotMintAuthority(t *testing.T) {
	ctx := context.Background()
	backing := realm.NewMemoryStore()
	spec := externalV31Spec(realm.ID("realm://wave31-embedded-store"), 500)
	if _, err := backing.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	ctl, err := realm.NewController(&embeddedV31Store{MemoryStore: backing})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(ctx, spec.ID)
	if !errors.Is(err, realm.ErrRuntimeCPUPolicyUnavailable) {
		t.Fatalf("embedded Store minted authority=%+v err=%v, want unavailable", authority, err)
	}
}

func TestV31AuthorityCannotCrossIdenticalTrustedStoreInstances(t *testing.T) {
	ctx := context.Background()
	spec := externalV31Spec(realm.ID("realm://wave31-store-instance"), 500)
	storeA := realm.NewMemoryStore()
	storeB := realm.NewMemoryStore()
	if _, err := storeA.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	controllerA, err := realm.NewController(storeA)
	if err != nil {
		t.Fatal(err)
	}
	controllerB, err := realm.NewController(storeB)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := controllerA.CurrentRuntimeCPUPolicyAuthority(ctx, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if binding, err := controllerB.ValidateRuntimeCPUPolicyAuthority(ctx, authority); !errors.Is(err, realm.ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("authority crossed Store instances binding=%+v err=%v, want invalid", binding, err)
	}
}

func TestV31SerializedOrDescriptiveBindingCannotRestoreAuthority(t *testing.T) {
	ctx := context.Background()
	store := realm.NewMemoryStore()
	ctl, err := realm.NewController(store)
	if err != nil {
		t.Fatal(err)
	}
	spec := externalV31Spec(realm.ID("realm://wave31-opacity"), 500)
	if _, err := ctl.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	authority, err := ctl.CurrentRuntimeCPUPolicyAuthority(ctx, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := authority.Binding()
	if !ok {
		t.Fatal("fresh authority has no binding")
	}

	authorityJSON, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	if string(authorityJSON) != "{}" {
		t.Fatalf("opaque authority serialized as %s, want {}", authorityJSON)
	}
	var restored realm.RuntimeCPUPolicyAuthority
	if err := json.Unmarshal(authorityJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if _, ok := restored.Binding(); ok {
		t.Fatal("serialized authority restored package-owned seal")
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(ctx, restored); !errors.Is(err, realm.ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("serialized authority validation err=%v, want invalid", err)
	}

	bindingJSON, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var reconstructed realm.RuntimeCPUPolicyAuthority
	if err := json.Unmarshal(bindingJSON, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if _, ok := reconstructed.Binding(); ok {
		t.Fatal("descriptive binding reconstructed package-owned seal")
	}
	if _, err := ctl.ValidateRuntimeCPUPolicyAuthority(ctx, reconstructed); !errors.Is(err, realm.ErrInvalidRuntimeCPUPolicyAuthority) {
		t.Fatalf("descriptive reconstruction validation err=%v, want invalid", err)
	}
}
