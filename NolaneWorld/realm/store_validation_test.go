package realm

import (
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestRealmStoresRejectMalformedServiceAndOperationRows(t *testing.T) {
	factories := []struct {
		name string
		open func(*testing.T) Store
	}{
		{name: "memory", open: func(t *testing.T) Store { return NewMemoryStore() }},
		{name: "durable", open: func(t *testing.T) Store {
			store, err := OpenDurableStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return store
		}},
	}

	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			store := factory.open(t)
			defer store.Close()
			spec := validSpec()
			if _, err := store.CreateRealm(spec); err != nil {
				t.Fatal(err)
			}
			wr := WorldRecord{
				RealmID: spec.ID, WorldID: world.ID("world-a"), RealizationRevision: 1,
				Phase: WorldLeased, LeaseGeneration: 1, LeaseExpiresUnix: time.Now().Add(time.Hour).Unix(),
			}
			if err := store.PutWorld(wr); err != nil {
				t.Fatal(err)
			}

			badProtocol := ServiceRecord{
				ID: serviceID(spec.ID, "api"), RealmID: spec.ID, WorldID: wr.WorldID,
				RealizationRevision: wr.RealizationRevision, Protocol: ServiceProtocol("raw"), Port: 8080, Generation: 1, Ready: true,
			}
			if err := store.PutService(badProtocol); !errors.Is(err, ErrInvalidService) {
				t.Fatalf("unsupported protocol accepted: %v", err)
			}

			badID := ServiceRecord{
				ID: ServiceID("service://other/api"), RealmID: spec.ID, WorldID: wr.WorldID,
				RealizationRevision: wr.RealizationRevision, Protocol: ServiceHTTP, Port: 8080, Generation: 1, Ready: true,
			}
			if err := store.PutService(badID); !errors.Is(err, ErrInvalidService) {
				t.Fatalf("cross-realm service ID accepted: %v", err)
			}

			if err := store.RecordOperation(OperationRecord{RealmID: spec.ID, OperationID: "op-a", RequestDigest: "digest-a", Status: "forged-success"}); !errors.Is(err, ErrInvalidOperation) {
				t.Fatalf("unknown operation status accepted: %v", err)
			}
		})
	}
}

func TestPersistedRowsRejectUnknownNestedFields(t *testing.T) {
	var service ServiceRecord
	if err := jsonUnmarshalStrict([]byte(`{"id":"service://test/api","realm_id":"realm://test","world_id":"world-a","realization_revision":1,"protocol":"http","port":8080,"generation":1,"ready":true,"unknown":true}`), &service); err == nil {
		t.Fatal("unknown service field accepted")
	}
	var operation OperationRecord
	if err := jsonUnmarshalStrict([]byte(`{"realm_id":"realm://test","operation_id":"op-a","request_digest":"digest-a","status":"pending","unknown":true}`), &operation); err == nil {
		t.Fatal("unknown operation field accepted")
	}
}

func jsonUnmarshalStrict(raw []byte, dst any) error {
	return strictDecodeRecord(raw, dst)
}
