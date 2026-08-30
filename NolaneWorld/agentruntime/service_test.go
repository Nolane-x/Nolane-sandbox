package agentruntime

import (
	"context"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func TestRegisterServiceReturnsSemanticIdentityOnly(t *testing.T) {
	svc,_,_,sess:=runtimeFixture(t)
	// The fixture fabric is semantic, but the Realm store also needs the World realization.
	if err:=svc.store.PutWorld(realm.WorldRecord{RealmID:sess.RealmID,WorldID:world.ID("world-a"),RealizationRevision:1,Phase:realm.WorldLeased,LeaseGeneration:1,LeaseExpiresUnix:1<<62});err!=nil { t.Fatal(err) }
	rec,err:=svc.RegisterService(context.Background(),ServiceRequest{SessionID:sess.ID,RealmRevision:sess.RealmRevision,WorldID:world.ID("world-a"),LeaseGeneration:1,Name:"api",Protocol:realm.ServiceHTTP,Port:8080,Ready:true})
	if err!=nil { t.Fatal(err) }
	if rec.ID!=realm.ServiceID("service://test/api") || rec.WorldID!=world.ID("world-a") || rec.Generation!=1 { t.Fatalf("receipt=%+v",rec) }
}
