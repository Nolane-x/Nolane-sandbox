//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package capability

func lockRegistryFile(uintptr) error   { return ErrRegistryLockUnsupported }
func unlockRegistryFile(uintptr) error { return nil }
