//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package authority

import (
	"errors"
	"syscall"
)

func lockJournal(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrLedgerLocked
		}
		return errors.Join(ErrLedgerCorrupt, err)
	}
	return nil
}

func unlockJournal(fd uintptr) error {
	if err := syscall.Flock(int(fd), syscall.LOCK_UN); err != nil {
		return errors.Join(ErrLedgerCorrupt, err)
	}
	return nil
}
