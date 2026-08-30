//go:build linux

package providergauntlet

import "os"

func currentUID() (uint32, bool) { return uint32(os.Getuid()), true }
