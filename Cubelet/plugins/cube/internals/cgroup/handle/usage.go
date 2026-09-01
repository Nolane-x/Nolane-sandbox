package handle

import (
	"math"
	"os"
)

func IsCgroupV1UnlimitedMemoryLimit(limit uint64) bool {
	pageMask := uint64(os.Getpagesize() - 1)
	return limit == uint64(math.MaxInt64)&^pageMask
}

type ResourceLimit struct {
	Value     uint64
	Unlimited bool
}

type CPULimit struct {
	QuotaUS   uint64
	PeriodUS  uint64
	Unlimited bool
}

type UsageSnapshot struct {
	CPUUsageTotalNS          uint64
	CPUUserTotalNS           uint64
	CPUSystemTotalNS         uint64
	CPUThrottledTotalNS      uint64
	CPUPeriodsTotal          uint64
	CPUThrottledPeriodsTotal uint64
	CPULimit                 CPULimit

	MemoryCurrentBytes  uint64
	MemoryLimit         ResourceLimit
	MemoryFailuresTotal uint64

	// MemoryOOMKillsTotal is a kernel-observed OOM-kill counter. The value is
	// meaningful only when MemoryOOMKillsKnown is true. Keeping presence
	// explicit prevents a missing cgroup signal from being promoted to a false
	// zero-OOM claim.
	MemoryOOMKillsKnown bool
	MemoryOOMKillsTotal uint64
}
