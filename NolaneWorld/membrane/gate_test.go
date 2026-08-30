package membrane

import (
	"errors"
	"testing"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/network"
)

func TestRealityCrossingClassificationNeverConfusesCapabilityWithAuthority(t *testing.T) {
	tests := []struct {
		class         network.Class
		lane          RealityLane
		gateway       bool
		delegation    bool
		typedProvider bool
	}{
		{network.N0None, LaneInternal, false, false, false},
		{network.N1PublicRead, LaneGovernedPublic, true, false, false},
		{network.N2PublicSupplyChain, LaneGovernedPublic, true, false, false},
		{network.N3AuthenticatedRead, LaneDelegatedProvider, false, true, true},
		{network.N4ReversibleWrite, LaneDelegatedProvider, false, true, true},
		{network.N5ConsequentialWrite, LaneDelegatedProvider, false, true, true},
	}
	for _, tt := range tests {
		plan, err := Classify(tt.class)
		if err != nil {
			t.Fatalf("class %s: %v", tt.class, err)
		}
		if plan.Class != tt.class || plan.Lane != tt.lane || plan.RequiresRealityGateway != tt.gateway || plan.RequiresDelegation != tt.delegation || plan.RequiresTypedProvider != tt.typedProvider {
			t.Fatalf("class %s plan=%+v", tt.class, plan)
		}
		if plan.PublicInboundAllowed || plan.AmbientCredentialsAllowed {
			t.Fatalf("class %s crossed unsafe authority boundary: %+v", tt.class, plan)
		}
	}
}

func TestRealityCrossingRejectsUnknownClass(t *testing.T) {
	if _, err := Classify(network.Class("N9_MAGIC")); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("err=%v", err)
	}
}
