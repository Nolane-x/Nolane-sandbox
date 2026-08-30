package agentruntime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/fabric"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestAgentFacingTypesDoNotExposeRealizationAuthority(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(Session{}),
		reflect.TypeOf(WorldLease{}),
		reflect.TypeOf(ExecReceipt{}),
		reflect.TypeOf(CheckpointReceipt{}),
		reflect.TypeOf(ServiceReceipt{}),
		reflect.TypeOf(CapabilityReport{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			text := strings.ToLower(f.Name + " " + f.Type.String())
			for _, forbidden := range []string{"handle", "sandbox", "token", "secret", "credential", "envd", "traffic", "cube"} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s exposes forbidden realization/authority field %s (%s)", typ, f.Name, f.Type)
				}
			}
		}
	}
}

func TestEnterRejectsCallerForgedPolicyDigest(t *testing.T) {
	store := realm.NewMemoryStore()
	spec := realm.Spec{ID: realm.ID("realm://policy"), MaxWorlds: 2, DefaultLease: time.Minute, NetworkProfile: realm.R0InternalOnly, ResourceBudget: realm.ResourceBudget{CPUUnits: 2, MemoryMiB: 1024, DiskMiB: 2048}}
	if _, err := store.CreateRealm(spec); err != nil {
		t.Fatal(err)
	}
	ff := &fakeFabric{lease: fabric.Lease{RealmID: spec.ID, WorldID: world.ID("world-a"), Generation: 1, ExpiresUnix: time.Now().Add(time.Minute).Unix(), RealizationRevision: 1}}
	svc, err := New(store, ff, &fakeGuest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enter(context.Background(), EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: "caller-forged-policy"}); !errors.Is(err, ErrPolicyMismatch) {
		t.Fatalf("forged policy digest err=%v", err)
	}
	policyDigest, err := realm.PolicyDigest(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Enter(context.Background(), EnterRequest{RealmID: spec.ID, ExpectedRevision: 1, PolicyDigest: policyDigest})
	if err != nil {
		t.Fatal(err)
	}
	if sess.PolicyDigest != policyDigest {
		t.Fatalf("session policy digest=%q want %q", sess.PolicyDigest, policyDigest)
	}
}

func TestRuntimeInterfaceIsCompleteAndHasNoRealmAdministration(t *testing.T) {
	typ := reflect.TypeOf((*Runtime)(nil)).Elem()
	required := map[string]bool{
		"Enter":           false,
		"Acquire":         false,
		"Exec":            false,
		"Spawn":           false,
		"Checkpoint":      false,
		"Resume":          false,
		"RegisterService": false,
		"Capabilities":    false,
		"Release":         false,
	}
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		lower := strings.ToLower(name)
		if strings.Contains(lower, "createrealm") || strings.Contains(lower, "updaterealm") || strings.Contains(lower, "closerealm") {
			t.Fatalf("agent Runtime exposes Realm administration: %s", name)
		}
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for method, present := range required {
		if !present {
			t.Errorf("Runtime missing semantic operation %s", method)
		}
	}
}
