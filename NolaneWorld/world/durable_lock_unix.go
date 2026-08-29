//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package world

import (
	"errors"
	"syscall"
)

func lockStateFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrStateLocked
		}
		return errors.Join(ErrStateCorrupt, err)
	}
	return nil
}
func unlockStateFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_UN); err != nil {
		return errors.Join(ErrStateCorrupt, err)
	}
	return nil
}
