//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package control

import (
	"errors"
	"syscall"
)

func lockCatalogFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrCatalogLocked
		}
		return errors.Join(ErrCatalogCorrupt, err)
	}
	return nil
}
func unlockCatalogFile(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_UN); err != nil {
		return errors.Join(ErrCatalogCorrupt, err)
	}
	return nil
}
