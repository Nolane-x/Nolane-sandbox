//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package control

func lockCatalogFile(uintptr) error   { return ErrCatalogLockUnsupported }
func unlockCatalogFile(uintptr) error { return nil }
