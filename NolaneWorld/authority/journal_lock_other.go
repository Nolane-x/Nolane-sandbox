//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package authority

func lockJournal(uintptr) error   { return ErrLedgerLockUnsupported }
func unlockJournal(uintptr) error { return nil }
