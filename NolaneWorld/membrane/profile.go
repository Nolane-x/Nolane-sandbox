package membrane

import (
	"errors"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/network"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

var (
	ErrInvalidProfile          = errors.New("membrane: invalid realm profile")
	ErrRealityAuthorityRequired = errors.New("membrane: typed reality authority required")
)

type ProfilePlan struct {
	Profile                    realm.NetworkProfile `json:"profile"`
	Class                      network.Class        `json:"class"`
	PublicInboundAllowed       bool                 `json:"public_inbound_allowed"`
	AmbientCredentialsAllowed bool                 `json:"ambient_credentials_allowed"`
	RawPublicInternetAllowed   bool                 `json:"raw_public_internet_allowed"`
	RequiresRealityGateway     bool                 `json:"requires_reality_gateway"`
}

func Plan(profile realm.NetworkProfile) (ProfilePlan, error) {
	if !profile.Valid() {
		return ProfilePlan{}, ErrInvalidProfile
	}
	p := ProfilePlan{Profile: profile, PublicInboundAllowed: false, AmbientCredentialsAllowed: false, RawPublicInternetAllowed: false}
	switch profile {
	case realm.R0InternalOnly:
		p.Class = network.N0None
	case realm.R1PublicRead:
		p.Class = network.N1PublicRead
		p.RequiresRealityGateway = true
	case realm.R2SupplyChain:
		p.Class = network.N2PublicSupplyChain
		p.RequiresRealityGateway = true
	default:
		return ProfilePlan{}, ErrInvalidProfile
	}
	return p, nil
}

func AllowsRealmClass(class network.Class) bool {
	switch class {
	case network.N0None, network.N1PublicRead, network.N2PublicSupplyChain:
		return true
	default:
		return false
	}
}

func ProfileForClass(class network.Class) (realm.NetworkProfile, error) {
	switch class {
	case network.N0None:
		return realm.R0InternalOnly, nil
	case network.N1PublicRead:
		return realm.R1PublicRead, nil
	case network.N2PublicSupplyChain:
		return realm.R2SupplyChain, nil
	case network.N3AuthenticatedRead, network.N4ReversibleWrite, network.N5ConsequentialWrite:
		return "", ErrRealityAuthorityRequired
	default:
		return "", ErrInvalidProfile
	}
}
