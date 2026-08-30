package membrane

import "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/network"

type RealityLane string

const (
	LaneInternal          RealityLane = "internal"
	LaneGovernedPublic    RealityLane = "governed-public"
	LaneDelegatedProvider RealityLane = "delegated-provider"
)

type CrossingPlan struct {
	Class                     network.Class `json:"class"`
	Lane                      RealityLane   `json:"lane"`
	RequiresRealityGateway    bool          `json:"requires_reality_gateway"`
	RequiresDelegation        bool          `json:"requires_delegation"`
	RequiresTypedProvider     bool          `json:"requires_typed_provider"`
	PublicInboundAllowed      bool          `json:"public_inbound_allowed"`
	AmbientCredentialsAllowed bool          `json:"ambient_credentials_allowed"`
}

func Classify(class network.Class) (CrossingPlan, error) {
	plan := CrossingPlan{Class: class}
	switch class {
	case network.N0None:
		plan.Lane = LaneInternal
	case network.N1PublicRead, network.N2PublicSupplyChain:
		plan.Lane = LaneGovernedPublic
		plan.RequiresRealityGateway = true
	case network.N3AuthenticatedRead, network.N4ReversibleWrite, network.N5ConsequentialWrite:
		plan.Lane = LaneDelegatedProvider
		plan.RequiresDelegation = true
		plan.RequiresTypedProvider = true
	default:
		return CrossingPlan{}, ErrInvalidProfile
	}
	return plan, nil
}
