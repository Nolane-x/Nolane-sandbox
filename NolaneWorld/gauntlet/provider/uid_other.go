//go:build !linux

package providergauntlet

func currentUID() (uint32, bool) { return 0, false }
