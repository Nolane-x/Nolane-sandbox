//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package realm

import (
	"errors"
	"syscall"
)

func lockStoreFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrStoreLocked
		}
		return errors.Join(ErrStoreCorrupt, err)
	}
	return nil
}

func unlockStoreFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_UN); err != nil {
		return errors.Join(ErrStoreCorrupt, err)
	}
	return nil
}
