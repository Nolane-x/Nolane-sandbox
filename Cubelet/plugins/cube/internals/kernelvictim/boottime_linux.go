//go:build linux

package kernelvictim

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func BootTimeNS() (uint64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, fmt.Errorf("read CLOCK_BOOTTIME: %w", err)
	}
	return timespecToBootNS(ts.Sec, ts.Nsec)
}

func timespecToBootNS(seconds, nanoseconds int64) (uint64, error) {
	if seconds < 0 || nanoseconds < 0 || nanoseconds >= 1_000_000_000 {
		return 0, fmt.Errorf("invalid CLOCK_BOOTTIME timespec")
	}
	sec := uint64(seconds)
	if sec > math.MaxUint64/1_000_000_000 {
		return 0, fmt.Errorf("CLOCK_BOOTTIME seconds overflow nanoseconds")
	}
	base := sec * 1_000_000_000
	ns := uint64(nanoseconds)
	if base > math.MaxUint64-ns {
		return 0, fmt.Errorf("CLOCK_BOOTTIME nanoseconds overflow")
	}
	value := base + ns
	if value == 0 {
		return 0, fmt.Errorf("CLOCK_BOOTTIME is zero")
	}
	return value, nil
}
