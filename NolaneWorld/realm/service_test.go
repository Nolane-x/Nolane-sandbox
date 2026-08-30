package realm

import (
	"errors"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

func serviceStore(t *testing.T) *MemoryStore {
	t.Helper()
	s:=NewMemoryStore()
	spec:=Spec{ID:ID("realm://test"),MaxWorlds:2,DefaultLease:time.Minute,NetworkProfile:R0InternalOnly,ResourceBudget:ResourceBudget{CPUUnits:2,MemoryMiB:1024,DiskMiB:2048}}
	if _,err:=s.CreateRealm(spec);err!=nil { t.Fatal(err) }
	if err:=s.PutWorld(WorldRecord{RealmID:spec.ID,WorldID:world.ID("world-a"),RealizationRevision:1,Phase:WorldLeased,LeaseGeneration:1,LeaseExpiresUnix:time.Now().Add(time.Minute).Unix()});err!=nil { t.Fatal(err) }
	return s
}

func TestServiceCanonicalIdentityAndStaleness(t *testing.T) {
	store:=serviceStore(t)
	reg,err:=NewServiceRegistry(store);if err!=nil { t.Fatal(err) }
	rec,err:=reg.Register(ServiceRequest{RealmID:ID("realm://test"),WorldID:world.ID("world-a"),RealizationRevision:1,Name:"api",Protocol:ServiceHTTP,Port:8080,Ready:true});if err!=nil { t.Fatal(err) }
	if rec.ID!=ServiceID("service://test/api") { t.Fatalf("id=%q",rec.ID) }
	current,ok:=reg.Current(rec.ID);if !ok || current.Generation!=1 || !current.Ready { t.Fatalf("current=%+v ok=%v",current,ok) }

	wr,_:=store.World(ID("realm://test"),world.ID("world-a"));wr.RealizationRevision=2
	if err:=store.PutWorld(wr);err!=nil { t.Fatal(err) }
	if _,ok:=reg.Current(rec.ID);ok { t.Fatal("old service remained current after realization change") }

	next,err:=reg.Register(ServiceRequest{RealmID:ID("realm://test"),WorldID:world.ID("world-a"),RealizationRevision:2,Name:"api",Protocol:ServiceHTTP,Port:8080,Ready:true});if err!=nil { t.Fatal(err) }
	if next.ID!=rec.ID || next.Generation!=2 { t.Fatalf("next=%+v",next) }
}

func TestServiceRejectsAmbiguousNamesAndInvalidBindings(t *testing.T) {
	store:=serviceStore(t)
	reg,_:=NewServiceRegistry(store)
	for _,name:=range []string{"","../x","a/b","a%2fb","a@b","a?b","a#b"} {
		_,err:=reg.Register(ServiceRequest{RealmID:ID("realm://test"),WorldID:world.ID("world-a"),RealizationRevision:1,Name:name,Protocol:ServiceTCP,Port:80,Ready:true})
		if !errors.Is(err,ErrInvalidService) { t.Fatalf("name=%q err=%v",name,err) }
	}
	if _,err:=reg.Register(ServiceRequest{RealmID:ID("realm://test"),WorldID:world.ID("world-a"),RealizationRevision:1,Name:"api",Protocol:ServiceProtocol("smtp"),Port:25});!errors.Is(err,ErrInvalidService) { t.Fatalf("protocol err=%v",err) }
	if _,err:=reg.Register(ServiceRequest{RealmID:ID("realm://test"),WorldID:world.ID("world-a"),RealizationRevision:1,Name:"api",Protocol:ServiceTCP,Port:0});!errors.Is(err,ErrInvalidService) { t.Fatalf("port err=%v",err) }
}
