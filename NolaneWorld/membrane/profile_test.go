package membrane

import (
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/network"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

func TestRealmProfilesNeverEncodeAuthenticatedOrConsequentialAuthority(t *testing.T) {
	tests:=[]struct{ profile realm.NetworkProfile; class network.Class }{
		{realm.R0InternalOnly,network.N0None},
		{realm.R1PublicRead,network.N1PublicRead},
		{realm.R2SupplyChain,network.N2PublicSupplyChain},
	}
	for _,tt:=range tests {
		plan,err:=Plan(tt.profile);if err!=nil { t.Fatal(err) }
		if plan.Class!=tt.class { t.Fatalf("%s class=%s",tt.profile,plan.Class) }
		if plan.PublicInboundAllowed || plan.AmbientCredentialsAllowed { t.Fatalf("unsafe plan=%+v",plan) }
	}
	for _,class:=range []network.Class{network.N3AuthenticatedRead,network.N4ReversibleWrite,network.N5ConsequentialWrite} {
		if AllowsRealmClass(class) { t.Fatalf("realm accepted %s",class) }
		if _,err:=ProfileForClass(class);!errors.Is(err,ErrRealityAuthorityRequired) { t.Fatalf("class=%s err=%v",class,err) }
	}
}

func TestPublicProfilesRequireGovernedRealityLane(t *testing.T) {
	for _,profile:=range []realm.NetworkProfile{realm.R1PublicRead,realm.R2SupplyChain} {
		plan,err:=Plan(profile);if err!=nil { t.Fatal(err) }
		if !plan.RequiresRealityGateway || plan.RawPublicInternetAllowed { t.Fatalf("profile=%s plan=%+v",profile,plan) }
	}
	plan,_:=Plan(realm.R0InternalOnly)
	if plan.RequiresRealityGateway || plan.RawPublicInternetAllowed { t.Fatalf("R0 plan=%+v",plan) }
}
