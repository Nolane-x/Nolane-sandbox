//go:build !linux

package kernelvictim

import (
	"context"
	"fmt"
)

func BootTimeNS() (uint64, error) {
	return 0, fmt.Errorf("CLOCK_BOOTTIME is unsupported on this platform")
}

func timespecToBootNS(seconds, nanoseconds int64) (uint64, error) {
	return 0, fmt.Errorf("CLOCK_BOOTTIME is unsupported on this platform")
}

func ResolveCgroupV2ID(string) (uint64, bool) {
	return 0, false
}

func StartBestEffortCollector(context.Context) (Source, error) {
	return nil, fmt.Errorf("kernel OOM victim collection is unsupported on this platform")
}
