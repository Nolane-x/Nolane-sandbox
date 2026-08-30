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

func TestRuntimeInterfaceHasNoRealmAdministration(t *testing.T) {
	typ := reflect.TypeOf((*Runtime)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		name := strings.ToLower(typ.Method(i).Name)
		if strings.Contains(name, "createrealm") || strings.Contains(name, "updaterealm") || strings.Contains(name, "closerealm") {
			t.Fatalf("agent Runtime exposes Realm administration: %s", typ.Method(i).Name)
		}
	}
}
