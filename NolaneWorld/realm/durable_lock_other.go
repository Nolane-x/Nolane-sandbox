//go:build !(linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package realm

func lockStoreFile(uintptr) error { return ErrLockUnsupported }
func unlockStoreFile(uintptr) error { return ErrLockUnsupported }
