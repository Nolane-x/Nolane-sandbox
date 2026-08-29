//go:build !linux

package broker

import "net"

func verifyPeer(net.Conn, uint32) error {
	return ErrBrokerUnsupported
}
