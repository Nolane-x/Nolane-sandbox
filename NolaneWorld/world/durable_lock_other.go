//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package world

func lockStateFile(uintptr) error   { return ErrStateLockUnsupported }
func unlockStateFile(uintptr) error { return nil }
