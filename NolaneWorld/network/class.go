package network

import "errors"

type Class string

const (
	N0None               Class = "N0_NONE"
	N1PublicRead         Class = "N1_PUBLIC_READ"
	N2PublicSupplyChain  Class = "N2_PUBLIC_SUPPLY_CHAIN"
	N3AuthenticatedRead  Class = "N3_AUTHENTICATED_READ"
	N4ReversibleWrite    Class = "N4_REVERSIBLE_WRITE"
	N5ConsequentialWrite Class = "N5_CONSEQUENTIAL_WRITE"
)

var ErrInvalidNetworkClass = errors.New("network: invalid class")

func (c Class) Valid() bool {
	switch c {
	case N0None, N1PublicRead, N2PublicSupplyChain, N3AuthenticatedRead, N4ReversibleWrite, N5ConsequentialWrite:
		return true
	default:
		return false
	}
}

func Parse(raw string) (Class, error) {
	c := Class(raw)
	if !c.Valid() {
		return "", ErrInvalidNetworkClass
	}
	return c, nil
}
