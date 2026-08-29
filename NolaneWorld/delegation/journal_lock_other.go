//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package delegation

func lockGrantJournal(uintptr) error   { return ErrStoreLockUnsupported }
func unlockGrantJournal(uintptr) error { return nil }
