//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package capability

import (
	"errors"
	"syscall"
)

func lockRegistryFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrRegistryLocked
		}
		return errors.Join(ErrRegistryCorrupt, err)
	}
	return nil
}

func unlockRegistryFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_UN); err != nil {
		return errors.Join(ErrRegistryCorrupt, err)
	}
	return nil
}
