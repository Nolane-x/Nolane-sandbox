package agentruntime

import (
	"reflect"
	"strings"
	"testing"
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

func TestRuntimeInterfaceIsCompleteAndHasNoRealmAdministration(t *testing.T) {
	typ := reflect.TypeOf((*Runtime)(nil)).Elem()
	required := map[string]bool{
		"Enter": false,
		"Acquire": false,
		"Exec": false,
		"Spawn": false,
		"Checkpoint": false,
		"Resume": false,
		"RegisterService": false,
		"Capabilities": false,
		"Release": false,
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
