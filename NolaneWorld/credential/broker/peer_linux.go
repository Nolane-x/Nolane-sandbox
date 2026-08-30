//go:build linux

package broker

import (
	"net"
	"syscall"
)

func verifyPeer(conn net.Conn, expectedUID uint32) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return ErrBrokerUnsupported
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return ErrBrokerUnavailable
	}
	var cred *syscall.Ucred
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		cred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || sockErr != nil || cred == nil {
		return ErrBrokerUnavailable
	}
	if cred.Uid != expectedUID {
		return ErrBrokerPeerMismatch
	}
	return nil
}
